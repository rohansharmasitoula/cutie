// Package agents decides how cutie names, starts, prompts and wakes the omp agent
// that works on a PR.
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
// herdr names must match [a-z][a-z0-9_-]{0,31}, so the repo part is trimmed to fit.
func Name(p github.PR) string {
	prefix := "rev"
	if p.Mine {
		prefix = "fix"
	}
	num := fmt.Sprintf("%d", p.Number)
	repo := strings.Trim(nameInvalid.ReplaceAllString(strings.ToLower(p.RepoName), "-"), "-")
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

// Baseline is the value a watcher compares against to detect new activity.
func Baseline(p github.PR) string {
	if p.Mine {
		if p.LastFeedback.IsZero() {
			return ""
		}
		return p.LastFeedback.UTC().Format(time.RFC3339)
	}
	return p.HeadOID
}

// Triggered reports whether the PR has activity newer than the watch baseline:
// new reviewer feedback on your PR, or new commits on a PR you review.
func Triggered(p github.PR, baseline string) bool {
	cur := Baseline(p)
	if p.Mine {
		return cur != "" && cur > baseline // RFC3339 UTC strings sort chronologically
	}
	return cur != baseline
}

//go:embed prompts/*.md
var defaultPrompts embed.FS

// promptNames are the four messages cutie sends: start or follow up, on your PR (fix)
// or someone else's (review).
var promptNames = []string{"fix-start", "fix-followup", "review-start", "review-followup"}

// Prompts are the message templates sent to agents. Each is a Go text/template over
// PromptData; a file <name>.md in the override folder replaces the built-in one.
type Prompts struct {
	t map[string]*template.Template
}

// PromptData is what a prompt template can use, e.g. {{.URL}} or {{.Branch}}.
type PromptData struct {
	URL, Repo, Branch, Base, Author, HeadShort string
	Number                                     int
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
		t, err := template.New(name).Option("missingkey=error").Parse(strings.TrimSpace(string(src)))
		if err != nil {
			return ps, fmt.Errorf("prompt %s: %w", file, err)
		}
		ps.t[name] = t
	}
	return ps, nil
}

// Start is the first message to a new agent for the PR.
func (ps Prompts) Start(p github.PR) (string, error) { return ps.render(p, "start") }

// FollowUp is what a watcher sends the PR's existing agent when new activity arrives.
func (ps Prompts) FollowUp(p github.PR) (string, error) { return ps.render(p, "followup") }

func (ps Prompts) render(p github.PR, stage string) (string, error) {
	side := "review"
	if p.Mine {
		side = "fix"
	}
	head := p.HeadOID
	if len(head) > 7 {
		head = head[:7]
	}
	var b strings.Builder
	err := ps.t[side+"-"+stage].Execute(&b, PromptData{
		URL: p.URL, Repo: p.Repo, Branch: p.Branch, Base: p.Base, Author: p.Author, HeadShort: head, Number: p.Number,
	})
	return b.String(), err
}
