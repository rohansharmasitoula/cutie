// Package herdr drives the herdr CLI: workspaces, panes, agents and notifications.
package herdr

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rohansharmasitoula/cutie/internal/utils"
	"os"
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

// Error is a failure herdr reported as JSON, e.g. code "agent_not_ready".
type Error struct {
	Cmd     string
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Cmd + ": " + e.Code + ": " + e.Message }

// IsCode reports whether err is a herdr error with the given code.
func IsCode(err error, code string) bool {
	var he *Error
	return errors.As(err, &he) && he.Code == code
}

// herdr runs a herdr CLI command and decodes its `.result` into out (if non-nil).
func herdr(out any, args ...string) error {
	stdout, err := utils.Run("herdr", args...)
	if ce, ok := utils.AsCmdError(err); ok {
		var body struct{ Error Error }
		if json.Unmarshal(ce.Stderr, &body) == nil && body.Error.Code != "" {
			body.Error.Cmd = "herdr " + args[0]
			return &body.Error
		}
		return err
	}
	if err != nil || out == nil {
		return err
	}
	var env struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(stdout, &env); err != nil {
		return utils.Wrap("herdr "+args[0]+": decoding", err)
	}
	return json.Unmarshal(env.Result, out)
}

// ListAgents returns every live agent herdr recognizes, except the caller's own pane.
// Cost is left at -1 (unknown) for the caller to fill.
func ListAgents() ([]Agent, error) {
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
	}
	return res.Agents, nil
}

// AgentTail returns the agent pane's last n lines of real output, without the
// agent UI's rules, prompt and status bar.
func AgentTail(name string, n int) ([]string, error) {
	// `agent read` prints the pane text itself, not JSON.
	out, err := utils.Run("herdr", "agent", "read", name, "--source", "recent-unwrapped", "--lines", "80")
	if err != nil {
		return nil, err
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

// FreePane finds or creates a shell pane: the workspace labelled wsLabel (created at
// dir), the tab labelled tabLabel in it, and the first pane in that tab with no agent.
func FreePane(wsLabel, tabLabel, dir string) (string, error) {
	var wsList struct {
		Workspaces []workspace `json:"workspaces"`
	}
	if err := herdr(&wsList, "workspace", "list"); err != nil {
		return "", err
	}
	wsID := ""
	for _, w := range wsList.Workspaces {
		if w.Label == wsLabel {
			wsID = w.ID
			break
		}
	}
	label := tabLabel
	if wsID == "" {
		var c created
		if err := herdr(&c, "workspace", "create", "--cwd", dir, "--label", wsLabel, "--no-focus"); err != nil {
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

// StartAgent starts a named agent of the given herdr kind (omp, claude, codex, …) in a
// shell pane and submits the prompt. A just-created pane may still be starting its
// shell, so the start is retried briefly. An agent that stops at a startup question
// (e.g. Claude asking whether to trust the folder) gets the prompt once the user has
// answered it, waiting up to startupWait.
func StartAgent(name, kind, paneID, prompt string) error {
	var err error
	for range 5 {
		err = herdr(nil, "agent", "start", name, "--kind", kind, "--pane", paneID, "--timeout", "60000")
		if err == nil || IsCode(err, "agent_not_ready") {
			break
		}
		time.Sleep(time.Second)
	}
	if IsCode(err, "agent_not_ready") {
		err = herdr(nil, "agent", "wait", name, "--until", "idle", "--timeout", fmt.Sprint(startupWait.Milliseconds()))
		if err != nil {
			return fmt.Errorf("%s is waiting for you (answer it in its pane, then press p to send the PR): %w", name, err)
		}
	}
	if err != nil {
		return err
	}
	return PromptAgent(name, prompt)
}

const startupWait = 10 * time.Minute

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
