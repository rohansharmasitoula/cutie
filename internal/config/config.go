// Package config reads cutie's settings from the environment.
package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

type Config struct {
	Root      string        // folder that holds one clone per repo, named after the repo
	Interval  time.Duration // GitHub refresh interval
	Guard     float64       // pause watchers when the 5 hour Claude window passes this fraction
	WatchGap  time.Duration // minimum time between watcher runs
	AgentKind string        // herdr agent kind started for PRs: omp, claude, codex, …
	HasOmp    bool          // omp is installed, so Claude usage and costs can be shown
	Demo      bool          // CUTIE_DEMO=1: made-up data, no GitHub, herdr or omp calls
	DemoHerdr bool          // CUTIE_DEMO=herdr: made-up data, but real herdr panes with scripted agents

	// Set by main, not the environment.
	Version      string // this build's version, e.g. v0.2.0
	ReleaseBuild bool   // a release binary (GoReleaser), so updates are downloaded, not go installed
}

// Load reads CUTIE_* variables, falling back to defaults.
func Load() (Config, error) {
	c := Config{
		Root:      os.Getenv("CUTIE_ROOT"),
		Interval:  60 * time.Second,
		Guard:     0.85,
		WatchGap:  30 * time.Minute,
		AgentKind: os.Getenv("CUTIE_AGENT"),
	}
	if c.Root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return c, err
		}
		c.Root = filepath.Join(home, "Development")
	}
	if c.AgentKind == "" {
		c.AgentKind = "omp"
	}
	_, err := exec.LookPath("omp")
	c.HasOmp = err == nil
	switch os.Getenv("CUTIE_DEMO") {
	case "1":
		c.Demo, c.HasOmp = true, true
	case "herdr":
		// It creates workspaces and panes, so it only runs inside herdr; use a throwaway
		// session (herdr --session cutie-demo) to keep them out of your own.
		if os.Getenv("HERDR_PANE_ID") == "" {
			return c, fmt.Errorf("CUTIE_DEMO=herdr runs inside a throwaway herdr session: start `herdr --session cutie-demo` and run it in a pane there")
		}
		c.Demo, c.DemoHerdr, c.HasOmp = true, true, true
	}

	if s := os.Getenv("CUTIE_INTERVAL"); s != "" {
		secs, err := strconv.Atoi(s)
		if err != nil || secs < 10 {
			return c, fmt.Errorf("CUTIE_INTERVAL must be a number of seconds ≥ 10, got %q", s)
		}
		c.Interval = time.Duration(secs) * time.Second
	}
	if s := os.Getenv("CUTIE_WATCH_MINUTES"); s != "" {
		mins, err := strconv.Atoi(s)
		if err != nil || mins < 1 {
			return c, fmt.Errorf("CUTIE_WATCH_MINUTES must be a number of minutes ≥ 1, got %q", s)
		}
		c.WatchGap = time.Duration(mins) * time.Minute
	}
	if s := os.Getenv("CUTIE_GUARD"); s != "" {
		pct, err := strconv.Atoi(s)
		if err != nil || pct < 1 || pct > 100 {
			return c, fmt.Errorf("CUTIE_GUARD must be a percentage 1–100, got %q", s)
		}
		c.Guard = float64(pct) / 100
	}
	return c, nil
}
