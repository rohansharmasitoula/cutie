// Package agents decides how cutie names, starts, prompts and wakes the agent that
// works on a PR or an issue.
package agents

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/rohansharmasitoula/cutie/internal/github"
)

var nameInvalid = regexp.MustCompile(`[^a-z0-9-]+`)

// Name is the herdr agent name that ties a pane to a PR, e.g. "fix-my-service-42".
func Name(p github.PR) string {
	if p.Mine {
		return name("fix", p.RepoName, p.Number)
	}
	return name(reviewPrefix, p.RepoName, p.Number)
}

const reviewPrefix = "rev"

// IsReview reports whether a herdr agent name is one cutie gave a review agent.
func IsReview(agentName string) bool { return strings.HasPrefix(agentName, reviewPrefix+"-") }

// IssueName is the herdr agent name for an issue, e.g. "iss-my-service-7".
func IssueName(i github.Issue) string { return name("iss", i.RepoName, i.Number) }

// name joins prefix, repo and number. herdr names must match [a-z][a-z0-9_-]{0,31},
// so the repo part is trimmed to fit.
func name(prefix, repoName string, number int) string {
	num := fmt.Sprintf("%d", number)
	repo := strings.Trim(nameInvalid.ReplaceAllString(strings.ToLower(repoName), "-"), "-")
	if room := 32 - len(prefix) - len(num) - 2; len(repo) > room {
		repo = strings.TrimRight(repo[:room], "-")
	}
	return prefix + "-" + repo + "-" + num
}

// TabLabel is the herdr tab label used for a PR inside its repo workspace.
func TabLabel(p github.PR) string {
	if p.Mine {
		return fmt.Sprintf("fix #%d", p.Number)
	}
	return fmt.Sprintf("review #%d", p.Number)
}

// IssueTabLabel is the herdr tab label used for an issue inside its repo workspace.
func IssueTabLabel(i github.Issue) string { return fmt.Sprintf("issue #%d", i.Number) }

// Baseline is the value a watcher compares against to detect new activity. For a PR
// you review it marks a re-request: your last review's time while GitHub asks you to
// review again, else empty. Each review you submit gives the next re-request a new value.
func Baseline(p github.PR) string {
	if p.Mine {
		if p.LastFeedback.IsZero() {
			return ""
		}
		return p.LastFeedback.UTC().Format(time.RFC3339)
	}
	if p.Status != github.StatusReRequested {
		return ""
	}
	return p.MyLastReview.UTC().Format(time.RFC3339)
}

// Triggered reports whether the PR has activity newer than the watch baseline:
// new reviewer feedback on your PR, or a new re-request on a PR you review.
func Triggered(p github.PR, baseline string) bool {
	cur := Baseline(p)
	if p.Mine {
		return cur != "" && cur > baseline // RFC3339 UTC strings sort chronologically
	}
	return cur != "" && cur != baseline
}

//go:embed prompts/*.md
var defaultPrompts embed.FS

// promptNames are the messages cutie sends: start or follow up, on your PR (fix) or
// someone else's (review), and start on an issue assigned to you.
var promptNames = []string{"fix-start", "fix-followup", "review-start", "review-followup", "issue-start"}

// Prompts are the message templates sent to agents. Each is a Go text/template over
// PromptData; a file <name>.md in the override folder replaces the built-in one.
type Prompts struct {
	t map[string]*template.Template
}

// PromptData is what a PR prompt template can use, e.g. {{.URL}} or {{.Branch}}.
// Posting is the review mode's rule on posting to GitHub, for the review prompts.
type PromptData struct {
	URL, Repo, Branch, Base, Author, HeadShort, Posting string
	Number                                              int
}

// ReviewMode is how a review agent treats GitHub, chosen per PR: copilot drafts and
// waits for your approval, autopilot acts by itself.
type ReviewMode string

const (
	Copilot   ReviewMode = "copilot"
	Autopilot ReviewMode = "autopilot"
)

// Posting is the instruction a review prompt carries as {{.Posting}}: whether the agent
// posts the review and opens the follow-up issues its reviewer calls for by itself, or
// waits for your approval first. What to post and where is the reviewer's call.
func (r ReviewMode) Posting() string {
	if r == Autopilot {
		return "Autopilot: act without waiting for me. Post the review to GitHub with `gh` with the verdict the reviewer gives (approve, request changes or comment), and open the follow-up issues the reviewer calls for, in the repos it names. Then report here what you posted and the issues you opened."
	}
	return "Copilot: draft the review, the reviewer's verdict and the follow-up issues it calls for here, and wait for my approval. Post the review and open the issues with `gh` only after I approve them in this pane, with any edits I ask for."
}

// ModeChange is sent to a PR's live review agent, working or idle, when you switch its
// mode, so the switch also applies to a review in progress.
func ModeChange(r ReviewMode) string {
	return "The review mode for this PR changed; this replaces the earlier posting instructions. " + r.Posting()
}

// IssueData is what the issue prompt template can use, e.g. {{.URL}} or {{.Title}}.
type IssueData struct {
	URL, Repo, Title, Author string
	Number                   int
}

// LoadPrompts reads the built-in templates, replacing any that dir overrides.
func LoadPrompts(dir string) (Prompts, error) {
	ps := Prompts{t: map[string]*template.Template{}}
	for _, name := range promptNames {
		file := name + ".md"
		src, err := os.ReadFile(filepath.Join(dir, file))
		if errors.Is(err, fs.ErrNotExist) {
			src, err = defaultPrompts.ReadFile("prompts/" + file)
		}
		if err != nil {
			return ps, err
		}
		// Review prompts must carry the mode's posting rule, or autopilot and copilot
		// would both get whatever the file says about posting.
		if strings.HasPrefix(name, "review-") && !strings.Contains(string(src), "{{.Posting}}") {
			return ps, fmt.Errorf("prompt %s: must contain {{.Posting}}, the review mode's rule on posting to GitHub (replace any \"do not post\" sentence with it)", filepath.Join(dir, file))
		}
		t, err := template.New(name).Option("missingkey=error").Parse(strings.TrimSpace(string(src)))
		if err != nil {
			return ps, fmt.Errorf("prompt %s: %w", file, err)
		}
		ps.t[name] = t
	}
	return ps, nil
}

// Start is the first message to a new agent for the PR; mode applies to reviews.
func (ps Prompts) Start(p github.PR, mode ReviewMode) (string, error) {
	return ps.render(p, "start", mode)
}

// FollowUp is what a watcher sends the PR's existing agent when new activity arrives.
func (ps Prompts) FollowUp(p github.PR, mode ReviewMode) (string, error) {
	return ps.render(p, "followup", mode)
}

func (ps Prompts) render(p github.PR, stage string, mode ReviewMode) (string, error) {
	side := "review"
	if p.Mine {
		side = "fix"
	}
	head := p.HeadOID
	if len(head) > 7 {
		head = head[:7]
	}
	return ps.execute(side+"-"+stage, PromptData{
		URL: p.URL, Repo: p.Repo, Branch: p.Branch, Base: p.Base, Author: p.Author, HeadShort: head, Number: p.Number,
		Posting: mode.Posting(),
	})
}

// IssueStart is the first message to a new agent for the issue.
func (ps Prompts) IssueStart(i github.Issue) (string, error) {
	return ps.execute("issue-start", IssueData{URL: i.URL, Repo: i.Repo, Title: i.Title, Author: i.Author, Number: i.Number})
}

func (ps Prompts) execute(name string, data any) (string, error) {
	var b strings.Builder
	err := ps.t[name].Execute(&b, data)
	return b.String(), err
}
