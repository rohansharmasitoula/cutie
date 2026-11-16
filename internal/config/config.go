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
	if os.Getenv("CUTIE_DEMO") == "1" {
		c.Demo, c.HasOmp = true, true
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
