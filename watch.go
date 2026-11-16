package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Watches maps PR key → baseline. For your PRs the baseline is the newest feedback
// time already handed to an agent; for review PRs it is the head commit already reviewed.
type Watches map[string]string

// statePath is where cutie keeps a state file, e.g. ~/Library/Application Support/cutie/watches.json.
func statePath(name string) string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "cutie", name)
}

// loadState decodes a state file into v; a missing file leaves v unchanged.
func loadState(name string, v any) error {
	b, err := os.ReadFile(statePath(name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", statePath(name), err)
	}
	return nil
}

func saveState(name string, v any) error {
	p := statePath(name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

func LoadWatches() (Watches, error) {
	w := Watches{}
	return w, loadState("watches.json", &w)
}

func (w Watches) Save() error { return saveState("watches.json", w) }

// Sizes are the panel proportions set by dragging their borders.
type Sizes struct {
	List     float64 `json:"list"`     // list width, fraction of the screen
	Detail   float64 `json:"detail"`   // detail height, fraction of the main area
	Agents   float64 `json:"agents"`   // agents width, fraction of the right column
	Activity int     `json:"activity"` // activity rows incl. borders; 0 = by screen height
}

func DefaultSizes() Sizes { return Sizes{List: 0.56, Detail: 0.55, Agents: 0.5} }

func LoadSizes() (Sizes, error) {
	s := DefaultSizes()
	return s, loadState("sizes.json", &s)
}

func (s Sizes) Save() error { return saveState("sizes.json", s) }

// Baseline is the value a watcher compares against to detect new activity.
func Baseline(p PR) string {
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
func Triggered(p PR, baseline string) bool {
	cur := Baseline(p)
	if p.Mine {
		return cur != "" && cur > baseline // RFC3339 UTC strings sort chronologically
	}
	return cur != baseline
}

func StartPrompt(p PR) string {
	if p.Mine {
		return fmt.Sprintf("Spawn an `orchestrator` task agent to address the review feedback on my PR %s (%s #%d, branch %s into %s). "+
			"It must read every unresolved review thread and every review and PR comment, make the fixes on that branch in its own worktree, "+
			"run the repo's checks and commit locally. Draft a reply for each thread. Do not push and do not post anything to GitHub; "+
			"report back here with what changed and the drafted replies.",
			p.URL, p.Repo, p.Number, p.Branch, p.Base)
	}
	return fmt.Sprintf("Spawn a `reviewer` task agent to review PR %s (%s #%d by @%s, branch %s into %s). "+
		"Use the PR description and any linked issue as the source of truth. Report the findings here with comment text ready to post. "+
		"Do not post anything to GitHub.",
		p.URL, p.Repo, p.Number, p.Author, p.Branch, p.Base)
}

func FollowUpPrompt(p PR) string {
	if p.Mine {
		return fmt.Sprintf("New review feedback arrived on %s. Spawn an `orchestrator` task agent to address the new unresolved threads and comments, "+
			"reusing the existing worktree for branch %s. Same rules: commit locally, draft replies, do not push or post to GitHub.",
			p.URL, p.Branch)
	}
	return fmt.Sprintf("New commits were pushed to %s since the last review (head is now %.7s). Spawn a `reviewer` task agent to re-review "+
		"what changed since the last review and whether earlier findings were addressed. Do not post anything to GitHub.",
		p.URL, p.HeadOID)
}
