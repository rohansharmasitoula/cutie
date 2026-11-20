// Package state keeps cutie's files: watched PRs, the review queue and panel sizes.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Watches maps PR key → baseline. For your PRs the baseline is the newest feedback
// time already handed to an agent; for review PRs it is the head commit already reviewed.
type Watches map[string]string

// Dir is cutie's folder for state and prompt overrides, e.g. ~/Library/Application Support/cutie.
func Dir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "cutie")
}

func statePath(name string) string { return filepath.Join(Dir(), name) }

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

// Queue is the PR keys waiting for a review agent, first in line first.
type Queue []string

func LoadQueue() (Queue, error) {
	var q Queue
	return q, loadState("queue.json", &q)
}

func (q Queue) Save() error { return saveState("queue.json", q) }

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
