package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Agent is a live coding agent that herdr recognizes in a pane.
type Agent struct {
	Name    string `json:"name"` // empty unless started with a name (cutie names its agents)
	Kind    string `json:"agent"`
	Status  string `json:"agent_status"` // idle, working, blocked, done, unknown
	PaneID  string `json:"pane_id"`
	Cwd     string `json:"cwd"`
	Title   string `json:"terminal_title_stripped"`
	Session *struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
	} `json:"agent_session"`
	Cost float64 `json:"-"` // filled from the session files, -1 when unknown
}

// Label names the agent for display: its herdr name, else its terminal title's text.
func (a Agent) Label() string {
	if a.Name != "" {
		return a.Name
	}
	// omp titles look like "π ⠏ <icon> TOPIC: summary"; keep the text from the first letter.
	if i := strings.IndexFunc(a.Title, func(r rune) bool { return r < 128 && (r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z') }); i >= 0 {
		return a.Title[i:]
	}
	return a.Kind + " " + a.PaneID
}

// SessionPath is the omp session file, when herdr knows it.
func (a Agent) SessionPath() string {
	if a.Session != nil && a.Session.Kind == "path" {
		return a.Session.Value
	}
	return ""
}

type herdrError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// herdr runs a herdr CLI command and decodes its `.result` into out (if non-nil).
func herdr(out any, args ...string) error {
	cmd := exec.Command("herdr", args...)
	stdout, err := cmd.Output()
	if err != nil {
		msg := err.Error()
		if ee, ok := err.(*exec.ExitError); ok {
			var he herdrError
			if json.Unmarshal(ee.Stderr, &he) == nil && he.Error.Message != "" {
				msg = he.Error.Code + ": " + he.Error.Message
			} else if s := strings.TrimSpace(string(ee.Stderr)); s != "" {
				msg = s
			}
		}
		return fmt.Errorf("herdr %s: %s", args[0]+" "+args[1], msg)
	}
	if out == nil {
		return nil
	}
	var env struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(stdout, &env); err != nil {
		return fmt.Errorf("herdr %s: decoding: %w", args[0]+" "+args[1], err)
	}
	return json.Unmarshal(env.Result, out)
}

// ListAgents returns every live agent herdr recognizes, with session costs from meter.
func ListAgents(meter *CostMeter) ([]Agent, error) {
	var res struct {
		Agents []Agent `json:"agents"`
	}
	if err := herdr(&res, "agent", "list"); err != nil {
		return nil, err
	}
	self := os.Getenv("HERDR_PANE_ID") // cutie's own pane is not an agent
	agents := res.Agents[:0]
	for _, a := range res.Agents {
		if a.PaneID != self {
			agents = append(agents, a)
		}
	}
	res.Agents = agents
	for i := range res.Agents {
		res.Agents[i].Cost = -1
		if p := res.Agents[i].SessionPath(); p != "" {
			if c, err := meter.SessionCost(p); err == nil {
				res.Agents[i].Cost = c
			}
		}
	}
	return res.Agents, nil
}

// AgentTail returns the agent pane's last n lines of real output, without the
// agent UI's rules, prompt and status bar.
func AgentTail(name string, n int) ([]string, error) {
	// `agent read` prints the pane text itself, not JSON.
	out, err := exec.Command("herdr", "agent", "read", name, "--source", "recent-unwrapped", "--lines", "80").Output()
	if err != nil {
		return nil, fmt.Errorf("herdr agent read %s: %w", name, err)
	}
	var keep []string
	for _, l := range strings.Split(string(out), "\n") {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "❯") || strings.Count(t, "─") > 10 || startsWithIcon(t) {
			continue
		}
		keep = append(keep, t)
	}
	if len(keep) > n {
		keep = keep[len(keep)-n:]
	}
	return keep, nil
}

// startsWithIcon spots status-bar lines, which open with a Nerd Font icon
// (Unicode private use areas).
func startsWithIcon(s string) bool {
	for _, r := range s {
		return r >= 0xE000 && r <= 0xF8FF || r >= 0xF0000
	}
	return false
}

// Notify shows a herdr notification with a sound.
func Notify(title, body string) error {
	return herdr(nil, "notification", "show", title, "--body", body, "--sound", "request")
}

var nameInvalid = regexp.MustCompile(`[^a-z0-9-]+`)

// AgentName is the herdr agent name that ties a pane to a PR, e.g. "fix-my-service-42".
// herdr names must match [a-z][a-z0-9_-]{0,31}, so the repo part is trimmed to fit.
func AgentName(p PR) string {
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
func TabLabel(p PR) string {
	if p.Mine {
		return fmt.Sprintf("fix #%d", p.Number)
	}
	return fmt.Sprintf("review #%d", p.Number)
}

type workspace struct {
	ID    string `json:"workspace_id"`
	Label string `json:"label"`
}

type tab struct {
	ID    string `json:"tab_id"`
	Label string `json:"label"`
}

type pane struct {
	ID    string `json:"pane_id"`
	TabID string `json:"tab_id"`
	Agent string `json:"agent"`
}

type created struct {
	Workspace workspace `json:"workspace"`
	Tab       tab       `json:"tab"`
	RootPane  pane      `json:"root_pane"`
}

// FreePane finds or creates a shell pane for the PR: workspace labelled with the repo
// name (rooted at dir), tab labelled TabLabel(p), first pane in that tab with no agent.
func FreePane(p PR, dir string) (string, error) {
	var wsList struct {
		Workspaces []workspace `json:"workspaces"`
	}
	if err := herdr(&wsList, "workspace", "list"); err != nil {
		return "", err
	}
	wsID := ""
	for _, w := range wsList.Workspaces {
		if w.Label == p.RepoName {
			wsID = w.ID
			break
		}
	}
	label := TabLabel(p)
	if wsID == "" {
		var c created
		if err := herdr(&c, "workspace", "create", "--cwd", dir, "--label", p.RepoName, "--no-focus"); err != nil {
			return "", err
		}
		if err := herdr(nil, "tab", "rename", c.Tab.ID, label); err != nil {
			return "", err
		}
		return c.RootPane.ID, nil
	}

	var tabs struct {
		Tabs []tab `json:"tabs"`
	}
	if err := herdr(&tabs, "tab", "list", "--workspace", wsID); err != nil {
		return "", err
	}
	for _, t := range tabs.Tabs {
		if t.Label != label {
			continue
		}
		var panes struct {
			Panes []pane `json:"panes"`
		}
		if err := herdr(&panes, "pane", "list", "--workspace", wsID); err != nil {
			return "", err
		}
		for _, pn := range panes.Panes {
			if pn.TabID == t.ID && pn.Agent == "" {
				return pn.ID, nil
			}
		}
	}
	var c created
	if err := herdr(&c, "tab", "create", "--workspace", wsID, "--cwd", dir, "--label", label, "--no-focus"); err != nil {
		return "", err
	}
	return c.RootPane.ID, nil
}

// StartAgent starts a named omp agent in a shell pane and submits the prompt.
// A just-created pane may still be starting its shell, so the start is retried briefly.
func StartAgent(name, paneID, prompt string) error {
	var err error
	for range 5 {
		if err = herdr(nil, "agent", "start", name, "--kind", "omp", "--pane", paneID, "--timeout", "60000"); err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		return err
	}
	return PromptAgent(name, prompt)
}

func PromptAgent(name, prompt string) error {
	return herdr(nil, "agent", "prompt", name, prompt)
}

func FocusAgent(name string) error {
	return herdr(nil, "agent", "focus", name)
}

// ClosePane closes a herdr pane, ending whatever runs in it.
func ClosePane(paneID string) error {
	return herdr(nil, "pane", "close", paneID)
}
