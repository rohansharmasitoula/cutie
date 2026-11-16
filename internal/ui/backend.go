package ui

import (
	"os/exec"
	"runtime"

	"github.com/rohansharmasitoula/cutie/internal/config"
	"github.com/rohansharmasitoula/cutie/internal/github"
	"github.com/rohansharmasitoula/cutie/internal/herdr"
	"github.com/rohansharmasitoula/cutie/internal/omp"
)

// backend is everything the dashboard reads from or does to the outside world.
// The live one talks to gh, herdr and omp; the demo one serves made-up data.
// Calls run inside tea.Cmd goroutines, one poll of each kind at a time.
type backend interface {
	PRs() (login string, mine, review []github.PR, rate github.RateLimit, err error)
	Agents() ([]herdr.Agent, error)
	Tail(agent string, lines int) ([]string, error)
	Limits() ([]omp.Limit, error)
	Stats() (omp.Stats, error)

	// Start opens a pane in workspace ws, tab tab (rooted at dir) and starts the agent.
	Start(ws, tab, dir, agent, kind, prompt string) error
	Prompt(agent, text string) error
	Focus(agent string) error
	Close(pane string) error
	Notify(title, body string) error
	Open(url string) error
}

func newBackend(cfg config.Config) backend {
	if cfg.Demo {
		return newDemoBackend()
	}
	return &liveBackend{root: cfg.Root, gh: github.NewFetcher(), meter: omp.NewCostMeter()}
}

type liveBackend struct {
	root  string
	gh    *github.Fetcher
	meter *omp.CostMeter
}

func (b *liveBackend) PRs() (string, []github.PR, []github.PR, github.RateLimit, error) {
	return b.gh.Fetch()
}

// Agents lists herdr's agents and fills in the cost of omp sessions.
func (b *liveBackend) Agents() ([]herdr.Agent, error) {
	list, err := herdr.ListAgents()
	for i := range list {
		if p := list[i].SessionPath(); p != "" && list[i].Kind == "omp" { // cost is read from omp session files
			if c, err := b.meter.SessionCost(p); err == nil {
				list[i].Cost = c
			}
		}
	}
	return list, err
}

func (b *liveBackend) Tail(agent string, lines int) ([]string, error) {
	return herdr.AgentTail(agent, lines)
}
func (b *liveBackend) Limits() ([]omp.Limit, error) { return omp.FetchLimits() }
func (b *liveBackend) Stats() (omp.Stats, error)    { return omp.FetchStats(b.root) }

func (b *liveBackend) Start(ws, tab, dir, agent, kind, prompt string) error {
	pane, err := herdr.FreePane(ws, tab, dir)
	if err != nil {
		return err
	}
	return herdr.StartAgent(agent, kind, pane, prompt)
}

func (b *liveBackend) Prompt(agent, text string) error { return herdr.PromptAgent(agent, text) }
func (b *liveBackend) Focus(agent string) error        { return herdr.FocusAgent(agent) }
func (b *liveBackend) Close(pane string) error         { return herdr.ClosePane(pane) }
func (b *liveBackend) Notify(title, body string) error { return herdr.Notify(title, body) }

// Open shows url in the default browser.
func (b *liveBackend) Open(url string) error {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	return exec.Command(opener, url).Start()
}

// demoBackend serves the made-up data in demo.go and only pretends to act.
type demoBackend struct {
	mine, review []github.PR
}

func newDemoBackend() *demoBackend {
	mine, review := demoPRs()
	return &demoBackend{mine: mine, review: review}
}

func (b *demoBackend) PRs() (string, []github.PR, []github.PR, github.RateLimit, error) {
	_, _, rate := demoUsage()
	return "you", b.mine, b.review, rate, nil
}
func (b *demoBackend) Agents() ([]herdr.Agent, error)      { return demoAgents(b.mine, b.review), nil }
func (b *demoBackend) Tail(string, int) ([]string, error)  { return demoTail(), nil }
func (b *demoBackend) Limits() ([]omp.Limit, error)        { l, _, _ := demoUsage(); return l, nil }
func (b *demoBackend) Stats() (omp.Stats, error)           { _, s, _ := demoUsage(); return s, nil }
func (b *demoBackend) Start(_, _, _, _, _, _ string) error { return nil }
func (b *demoBackend) Prompt(string, string) error         { return nil }
func (b *demoBackend) Focus(string) error                  { return nil }
func (b *demoBackend) Close(string) error                  { return nil }
func (b *demoBackend) Notify(string, string) error         { return nil }
func (b *demoBackend) Open(string) error                   { return nil }
