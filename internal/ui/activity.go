package ui

import (
	"fmt"
	"github.com/rohansharmasitoula/cutie/internal/github"
	"github.com/rohansharmasitoula/cutie/internal/herdr"
	"time"
)

type EventKind int

const (
	EventInfo EventKind = iota
	EventGood
	EventBad
)

type Event struct {
	At   time.Time
	Kind EventKind
	Text string
}

const maxEvents = 100

func ref(p github.PR) string { return fmt.Sprintf("%s #%d", p.RepoName, p.Number) }

// PREvents compares two GitHub snapshots and describes what changed.
func PREvents(prev, next map[string]github.PR, now time.Time) []Event {
	var evs []Event
	add := func(k EventKind, format string, a ...any) {
		evs = append(evs, Event{At: now, Kind: k, Text: fmt.Sprintf(format, a...)})
	}
	for key, n := range next {
		o, existed := prev[key]
		if !existed {
			if !n.Mine {
				add(EventInfo, "review wanted on %s by @%s", ref(n), n.Author)
			}
			continue
		}
		if n.LastFeedback.After(o.LastFeedback) && n.Mine {
			add(EventBad, "new review feedback on %s", ref(n))
		}
		if n.HeadOID != o.HeadOID && !n.Mine {
			add(EventInfo, "@%s pushed to %s", n.Author, ref(n))
		}
		if n.CI != o.CI {
			switch n.CI {
			case "FAILURE", "ERROR":
				add(EventBad, "CI failed on %s", ref(n))
			case "SUCCESS":
				add(EventGood, "CI passed on %s", ref(n))
			}
		}
		if n.Status != o.Status && n.Status == github.StatusApproved {
			add(EventGood, "%s approved", ref(n))
		}
	}
	for key, o := range prev {
		if _, ok := next[key]; !ok {
			add(EventInfo, "%s merged or closed", ref(o))
		}
	}
	return evs
}

// AgentEvents compares agent lists by pane and reports finished and blocked agents.
// blocked holds the agents that just became blocked, for notifications.
func AgentEvents(prev, next []herdr.Agent, now time.Time) (evs []Event, blocked []herdr.Agent) {
	before := make(map[string]string, len(prev))
	for _, a := range prev {
		before[a.PaneID] = a.Status
	}
	for _, a := range next {
		was, ok := before[a.PaneID]
		if !ok || was == a.Status {
			continue
		}
		switch {
		case a.Status == "blocked":
			evs = append(evs, Event{now, EventBad, a.Label() + " is waiting for you"})
			blocked = append(blocked, a)
		case was == "working" && (a.Status == "idle" || a.Status == "done"):
			evs = append(evs, Event{now, EventGood, a.Label() + " finished"})
		}
	}
	return evs, blocked
}
