package ui

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/rohansharmasitoula/cutie/internal/agents"
	"github.com/rohansharmasitoula/cutie/internal/herdr"
)

// CUTIE_DEMO=herdr keeps the made-up PRs, issues and usage of demo mode but drives real
// herdr: agents open in real workspaces and tabs, run a scripted transcript instead of
// an AI, and report their state to herdr like real ones. It records docs/demo.gif in a
// throwaway herdr session, so no AI, GitHub or private repo is involved.

// demoHerdrBackend is the demo backend with herdr calls made for real.
type demoHerdrBackend struct {
	*demoBackend
	seed    sync.Once
	seedErr error
}

func newDemoHerdrBackend() *demoHerdrBackend {
	return &demoHerdrBackend{demoBackend: newDemoBackend()}
}

// demoCosts gives the scripted agents the costs the plain demo shows.
var demoCosts = map[string]float64{"fix-api-412": 3.12, "rev-api-409": 1.48, "fix-api-418": 6.35, "iss-api-430": 0.87}

// Agents lists herdr's agents, starting the demo's agents on the first call so the
// dashboard opens with work already running.
func (b *demoHerdrBackend) Agents() ([]herdr.Agent, error) {
	b.seed.Do(func() { b.seedErr = b.startSeeds() })
	if b.seedErr != nil {
		return nil, b.seedErr
	}
	list, err := herdr.ListAgents()
	for i := range list {
		list[i].Cost = 0.21
		if c, ok := demoCosts[list[i].Name]; ok {
			list[i].Cost = c
		}
	}
	return list, err
}

// startSeeds starts the agents the plain demo lists, unless a previous run in this
// herdr session left them running.
func (b *demoHerdrBackend) startSeeds() error {
	live, err := herdr.ListAgents()
	if err != nil {
		return err
	}
	running := map[string]bool{}
	for _, a := range live {
		running[a.Name] = true
	}
	mine, review, issue := b.mine, b.review, b.issues[0]
	seeds := []struct{ ws, tab, name string }{
		{mine[0].RepoName, agents.TabLabel(mine[0]), agents.Name(mine[0])},
		{review[1].RepoName, agents.TabLabel(review[1]), agents.Name(review[1])},
		{mine[1].RepoName, agents.TabLabel(mine[1]), agents.Name(mine[1])},
		{issue.RepoName, agents.IssueTabLabel(issue), agents.IssueName(issue)},
	}
	for _, s := range seeds {
		if !running[s.name] {
			if err := b.Start(s.ws, s.tab, "", s.name, "omp", ""); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *demoHerdrBackend) Tail(agent string, lines int) ([]string, error) {
	return herdr.AgentTail(agent, lines)
}

// Start opens the pane like the live backend, then runs a scripted agent in it
// (`cutie demo-agent`) and waits until herdr lists it under its name.
func (b *demoHerdrBackend) Start(ws, tab, dir, agent, _, prompt string) error {
	if dir == "" {
		dir = os.TempDir()
	}
	pane, err := herdr.FreePane(ws, tab, dir)
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf("clear; '%s' demo-agent %s", exe, agent)
	if prompt != "" {
		f := filepath.Join(os.TempDir(), "cutie-demo-"+agent+".prompt")
		if err := os.WriteFile(f, []byte(prompt), 0o600); err != nil {
			return err
		}
		cmd += " '" + f + "'"
	}
	if err := herdr.RunInPane(pane, cmd); err != nil {
		return err
	}
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(300 * time.Millisecond) {
		list, err := herdr.ListAgents()
		if err != nil {
			return err
		}
		for _, a := range list {
			if a.Name == agent {
				return nil
			}
		}
	}
	return fmt.Errorf("%s did not start in pane %s", agent, pane)
}

func (b *demoHerdrBackend) Prompt(agent, text string) error { return herdr.PromptAgent(agent, text) }
func (b *demoHerdrBackend) Focus(agent string) error        { return herdr.FocusAgent(agent) }
func (b *demoHerdrBackend) Close(pane string) error         { return herdr.ClosePane(pane) }
func (b *demoHerdrBackend) Notify(title, body string) error { return herdr.Notify(title, body) }

// ---- the scripted agent ----

var (
	agentHead = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))
	agentUser = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	agentStep = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	agentNote = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	agentAsk  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
)

// RunDemoAgent is `cutie demo-agent NAME [PROMPT_FILE]`: a stand-in agent for
// CUTIE_DEMO=herdr. It plays a transcript for its PR or issue and reports its state to
// herdr, so cutie and herdr show it like a real agent, then answers later prompts.
func RunDemoAgent(name, promptFile string) error {
	pane := os.Getenv("HERDR_PANE_ID")
	if pane == "" {
		return fmt.Errorf("demo-agent runs in a herdr pane, started by CUTIE_DEMO=herdr")
	}
	report := func(state, msg string) {
		_ = herdr.ReportAgent(pane, "omp", state, msg)
	}
	report("working", "starting")
	if err := herdr.RenameAgent(pane, name); err != nil {
		return err
	}
	prompt := ""
	if promptFile != "" {
		b, err := os.ReadFile(promptFile)
		if err != nil {
			return err
		}
		prompt = string(b)
	}
	in := bufio.NewScanner(os.Stdin)
	say := func(style lipgloss.Style, s string, pause time.Duration) {
		fmt.Println(style.Render(s))
		time.Sleep(pause)
	}

	say(agentHead, "π omp · "+name, 300*time.Millisecond)
	if prompt != "" {
		say(agentUser, "❯ "+prompt, 900*time.Millisecond)
	}
	fmt.Println()
	switch {
	case strings.HasPrefix(name, "rev-"):
		reviewScript(name, strings.Contains(prompt, "Autopilot:"), say, report, in)
	case name == "fix-api-418": // finished before the demo opens
		for _, l := range demoTail(name) {
			say(agentStep, "● "+l, 0)
		}
		report("idle", "done")
	default: // fix and issue agents keep working through the demo
		lines := demoTail(name)
		if name == "iss-infra-110" {
			lines = []string{
				"Read the issue: page on-call when the queue is over 1,000 jobs for 10 minutes",
				"Branch you/110-queue-alert from main",
				"Added a queue_depth alert rule in alerts/queue.yaml",
				"terraform validate ok",
			}
		}
		for _, l := range lines {
			say(agentStep, "● "+l, 1400*time.Millisecond)
		}
		say(agentNote, "  thinking…", 0)
	}
	// Later prompts from cutie, e.g. a review mode change; a working agent stays working.
	for in.Scan() {
		say(agentUser, "❯ "+in.Text(), 300*time.Millisecond)
		say(agentStep, "● Noted.", 0)
	}
	return nil
}

// reviewScript reviews the PR, then posts by itself on autopilot or asks first on copilot.
func reviewScript(name string, autopilot bool, say func(lipgloss.Style, string, time.Duration), report func(string, string), in *bufio.Scanner) {
	pr := strings.TrimPrefix(name, "rev-")
	repo, number, _ := strings.Cut(pr, "-")
	ref := "acme/" + repo + "#" + number
	findings := map[string][]string{
		"rev-api-409": {
			"search/index.go:51  the batch size is still hardcoded; read it from config",
			"search/index.go:88  flush on shutdown, or the last batch is lost",
			"search/index_test.go  no test for a partial batch",
		},
		"rev-web-240": {
			"settings/roles.tsx:42  an admin can demote themselves; keep at least one admin",
			"settings/roles.tsx:97  role names are hardcoded; read them from the API",
		},
	}[name]
	if findings == nil {
		findings = []string{"no blocking issues; one naming nit"}
	}
	verdict, issue := "request changes", "flush the search index on shutdown"
	if name != "rev-api-409" {
		verdict, issue = "approve", "keep at least one admin on a team"
	}

	say(agentStep, "● gh pr view "+number+" · gh pr diff "+number, 1100*time.Millisecond)
	say(agentStep, "● Checked the diff against the PR description and the linked issue", 1300*time.Millisecond)
	for _, f := range findings {
		say(agentNote, "  "+f, 900*time.Millisecond)
	}
	say(agentStep, fmt.Sprintf("● Verdict: %s, %d comments, 1 follow-up issue", verdict, len(findings)), 900*time.Millisecond)
	if !autopilot {
		say(agentAsk, "✋ Copilot: post this review and open the follow-up issue? [y/n]", 0)
		report("blocked", "waiting for your OK")
		for in.Scan() {
			answer := strings.TrimSpace(in.Text())
			if strings.Contains(answer, "Autopilot:") { // switched to autopilot mid-review
				say(agentUser, "❯ switched to autopilot", 400*time.Millisecond)
				break
			}
			if strings.EqualFold(answer, "y") {
				break
			}
			say(agentNote, "  Kept as a draft. Tell me what to change, or y to post.", 0)
		}
		report("working", "posting")
	} else {
		say(agentAsk, "✈ Autopilot: posting without asking", 600*time.Millisecond)
	}
	say(agentStep, fmt.Sprintf("● Posted the review on %s: %s, %d comments", ref, verdict, len(findings)), 900*time.Millisecond)
	say(agentStep, fmt.Sprintf("● Opened acme/%s#%d: %s", repo, 433+len(repo), issue), 600*time.Millisecond)
	report("idle", "done")
}
