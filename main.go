package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	agentPollInterval = 3 * time.Second
	limitPollInterval = 60 * time.Second
	statsPollInterval = 5 * time.Minute
	tailLines         = 12
)

type config struct {
	root     string        // folder that holds one clone per repo, named after the repo
	interval time.Duration // GitHub refresh interval
	guard    float64       // pause watchers when the 5 hour window passes this fraction
	watchGap time.Duration // minimum time between watcher runs
}

func loadConfig() (config, error) {
	c := config{root: os.Getenv("CUTIE_ROOT"), interval: 60 * time.Second, guard: 0.85, watchGap: 30 * time.Minute}
	if c.root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return c, err
		}
		c.root = filepath.Join(home, "Development")
	}
	if s := os.Getenv("CUTIE_INTERVAL"); s != "" {
		secs, err := strconv.Atoi(s)
		if err != nil || secs < 10 {
			return c, fmt.Errorf("CUTIE_INTERVAL must be a number of seconds ≥ 10, got %q", s)
		}
		c.interval = time.Duration(secs) * time.Second
	}
	if s := os.Getenv("CUTIE_WATCH_MINUTES"); s != "" {
		mins, err := strconv.Atoi(s)
		if err != nil || mins < 1 {
			return c, fmt.Errorf("CUTIE_WATCH_MINUTES must be a number of minutes ≥ 1, got %q", s)
		}
		c.watchGap = time.Duration(mins) * time.Minute
	}
	if s := os.Getenv("CUTIE_GUARD"); s != "" {
		pct, err := strconv.Atoi(s)
		if err != nil || pct < 1 || pct > 100 {
			return c, fmt.Errorf("CUTIE_GUARD must be a percentage 1–100, got %q", s)
		}
		c.guard = float64(pct) / 100
	}
	return c, nil
}

// ---- messages ----

type prsMsg struct {
	login        string
	mine, review []PR
	rate         RateLimit
	err          error
}
type rateMsg struct{ rate RateLimit }
type agentsMsg struct {
	agents []Agent
	err    error
}
type tailMsg struct {
	name  string
	lines []string
}
type limitsMsg struct {
	limits []Limit
	err    error
}
type statsMsg struct {
	stats Stats
	err   error
}
type ghTickMsg struct{}
type agentTickMsg struct{}
type limitTickMsg struct{}
type statsTickMsg struct{}
type dispatchMsg struct {
	key, note string
	err       error
}

// ---- model ----

type sortMode int

const (
	sortUrgent sortMode = iota
	sortUpdated
	sortWaiting
)

func (s sortMode) String() string {
	return [...]string{"urgent", "updated", "waiting"}[s]
}

type inputMode int

const (
	inputNone inputMode = iota
	inputFilter
	inputPrompt
)

type model struct {
	cfg   config
	meter *CostMeter
	gh    *Fetcher
	rate  RateLimit // GitHub GraphQL budget, from the last answer

	login   string
	mine    []PR
	review  []PR
	agents  []Agent
	byName  map[string]Agent
	limits  []Limit
	stats   *Stats
	watches Watches
	// inflight marks PR keys with an agent start or prompt running.
	inflight map[string]bool
	events   []Event

	tail    []string // output of the selected PR's agent
	tailFor string   // agent name the tail belongs to

	// view state
	tab        int // 0 mine, 1 review
	grouped    bool
	sort       sortMode
	hideBots   bool
	hideDrafts bool
	collapsed  map[string]bool // "tab/repo"
	filter     string
	mode       inputMode
	input      textinput.Model
	help       bool

	cursor    int
	offset    int
	width     int
	height    int
	loading   bool
	refreshed time.Time
	status    string
	statusErr bool
	paused    bool      // watchers paused by the spending guard
	agentBusy bool      // an agent poll is running (it owns the cost meter)
	watchedAt time.Time // last watcher run
	sizes     Sizes
	drag      border // the border being dragged with the mouse
	// confirmClose is the agent name awaiting a second `x` to close its pane.
	confirmClose string
}

func newModel(cfg config, w Watches, s Sizes) model {
	in := textinput.New()
	in.Prompt = ""
	return model{
		cfg: cfg, meter: NewCostMeter(), gh: NewFetcher(), watches: w, sizes: s,
		inflight: map[string]bool{}, byName: map[string]Agent{}, collapsed: map[string]bool{},
		grouped: true, hideBots: true, loading: true, input: in,
	}
}

// Init loads everything once and starts one tick loop per data source. Ticks are only
// rescheduled by their own handler, so manual refreshes never add loops.
func (m model) Init() tea.Cmd {
	return tea.Batch(m.fetchPRs(), fetchLimits, m.fetchStats(),
		func() tea.Msg { return agentTickMsg{} },
		m.tick(ghTickMsg{}), m.tick(limitTickMsg{}), m.tick(statsTickMsg{}))
}

func (m model) tick(msg tea.Msg) tea.Cmd {
	d := agentPollInterval
	switch msg.(type) {
	case ghTickMsg:
		d = m.cfg.interval
	case limitTickMsg:
		d = limitPollInterval
	case statsTickMsg:
		d = statsPollInterval
	}
	return tea.Tick(d, func(time.Time) tea.Msg { return msg })
}

// fetchPRs polls GitHub. Callers set m.loading first and never start a second poll
// while one runs, so the fetcher's cache needs no lock.
func (m model) fetchPRs() tea.Cmd {
	gh := m.gh
	return func() tea.Msg {
		login, mine, review, rate, err := gh.Fetch()
		return prsMsg{login, mine, review, rate, err}
	}
}

// rateLow reports whether polling should wait for the GitHub budget to reset.
// The reserve leaves room for gh calls from your agents.
func (m model) rateLow() bool {
	return m.rate.Limit > 0 && m.rate.Remaining < 300 && time.Now().Before(m.rate.ResetAt)
}

// fetchAgents lists agents with their session costs. Only the agent tick runs it, and
// never while a poll is in flight, so the cost meter needs no lock.
func (m model) fetchAgents() tea.Cmd {
	meter := m.meter
	return func() tea.Msg {
		agents, err := ListAgents(meter)
		return agentsMsg{agents, err}
	}
}

// tailCmd reads the selected PR's agent output, if it has a live agent.
func (m model) tailCmd() tea.Cmd {
	p := m.selectedPR()
	if p == nil {
		return nil
	}
	name := AgentName(*p)
	if _, live := m.byName[name]; !live {
		return nil
	}
	return func() tea.Msg {
		lines, err := AgentTail(name, tailLines)
		if err != nil {
			lines = []string{err.Error()}
		}
		return tailMsg{name, lines}
	}
}

func fetchLimits() tea.Msg {
	l, err := FetchLimits()
	return limitsMsg{l, err}
}

func (m model) fetchStats() tea.Cmd {
	root := m.cfg.root
	return func() tea.Msg {
		s, err := FetchStats(root)
		return statsMsg{s, err}
	}
}

func (m *model) setStatus(s string, isErr bool) {
	m.status, m.statusErr = s, isErr
}

func (m *model) addEvents(evs ...Event) {
	m.events = append(evs, m.events...)
	if len(m.events) > maxEvents {
		m.events = m.events[:maxEvents]
	}
}

func (m model) repoDir(p PR) (string, error) {
	dir := filepath.Join(m.cfg.root, p.RepoName)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return "", fmt.Errorf("no clone of %s at %s", p.Repo, dir)
	}
	return dir, nil
}

// ---- list items ----

type item struct {
	header bool
	repo   string
	count  int
	pr     *PR
	depth  int // stack depth inside a repo group
}

func (m model) collapseKey(repo string) string { return fmt.Sprintf("%d/%s", m.tab, repo) }

func (m model) visiblePRs() []PR {
	src := m.mine
	if m.tab == 1 {
		src = m.review
	}
	f := strings.ToLower(m.filter)
	var out []PR
	for _, p := range src {
		if m.hideBots && p.IsBot || m.hideDrafts && p.IsDraft {
			continue
		}
		if f != "" && !strings.Contains(strings.ToLower(fmt.Sprintf("%s #%d %s @%s %s", p.RepoName, p.Number, p.Title, p.Author, p.Status.Label())), f) {
			continue
		}
		out = append(out, p)
	}
	m.sortPRs(out)
	return out
}

func (m model) hiddenCount() int {
	src := m.mine
	if m.tab == 1 {
		src = m.review
	}
	n := 0
	for _, p := range src {
		if m.hideBots && p.IsBot || m.hideDrafts && p.IsDraft {
			n++
		}
	}
	return n
}

func (m model) sortPRs(prs []PR) {
	sort.SliceStable(prs, func(i, j int) bool {
		a, b := prs[i], prs[j]
		switch m.sort {
		case sortUpdated:
			return a.UpdatedAt.After(b.UpdatedAt)
		case sortWaiting:
			return a.LastCommit.Before(b.LastCommit)
		}
		if a.Status.urgency() != b.Status.urgency() {
			return a.Status.urgency() < b.Status.urgency()
		}
		return a.UpdatedAt.After(b.UpdatedAt)
	})
}

// longLived are shared branches that release PRs come from; PRs targeting them are
// never shown as stacked.
var longLived = map[string]bool{
	"main": true, "master": true, "staging": true, "development": true, "develop": true,
	"dev": true, "production": true, "prod": true, "release": true,
}

// items is the list as displayed: repo headers (when grouped) and PRs, with stacked
// PRs (base branch = another listed PR's branch) indented under their parent.
func (m model) items() []item {
	prs := m.visiblePRs()
	if !m.grouped {
		out := make([]item, len(prs))
		for i := range prs {
			out[i] = item{pr: &prs[i]}
		}
		return out
	}
	byRepo := map[string][]*PR{}
	var repos []string
	for i := range prs {
		r := prs[i].RepoName
		if _, ok := byRepo[r]; !ok {
			repos = append(repos, r)
		}
		byRepo[r] = append(byRepo[r], &prs[i])
	}
	sort.Strings(repos)
	var out []item
	for _, r := range repos {
		group := byRepo[r]
		out = append(out, item{header: true, repo: r, count: len(group)})
		if m.collapsed[m.collapseKey(r)] {
			continue
		}
		branch := map[string]*PR{}
		for _, p := range group {
			branch[p.Branch] = p
		}
		children := map[string][]*PR{}
		var roots []*PR
		for _, p := range group {
			// A stack is the same author's PR built on another of their PR branches; a PR
			// that merely targets a shared branch (staging) is not stacked on its release PR.
			if parent, ok := branch[p.Base]; ok && parent != p && parent.Author == p.Author && !longLived[parent.Branch] {
				children[parent.Branch] = append(children[parent.Branch], p)
			} else {
				roots = append(roots, p)
			}
		}
		seen := map[*PR]bool{}
		var walk func(p *PR, depth int)
		walk = func(p *PR, depth int) {
			if seen[p] { // a branch cycle cannot happen on GitHub, but never loop
				return
			}
			seen[p] = true
			out = append(out, item{repo: r, pr: p, depth: depth})
			for _, c := range children[p.Branch] {
				walk(c, depth+1)
			}
		}
		for _, p := range roots {
			walk(p, 0)
		}
	}
	return out
}

func (m model) selectedItem() *item {
	its := m.items()
	if m.cursor < 0 || m.cursor >= len(its) {
		return nil
	}
	return &its[m.cursor]
}

func (m model) selectedPR() *PR {
	if it := m.selectedItem(); it != nil {
		return it.pr
	}
	return nil
}

// allPRs is both sections, for watchers and change detection.
func (m model) allPRs() []PR {
	out := make([]PR, 0, len(m.mine)+len(m.review))
	out = append(out, m.mine...)
	return append(out, m.review...)
}

// ---- agents ----

func (m model) startCmd(p PR, prompt, note string) tea.Cmd {
	dir, dirErr := m.repoDir(p)
	name := AgentName(p)
	return func() tea.Msg {
		if dirErr != nil {
			return dispatchMsg{key: p.Key(), err: dirErr}
		}
		paneID, err := FreePane(p, dir)
		if err == nil {
			err = StartAgent(name, paneID, prompt)
		}
		return dispatchMsg{key: p.Key(), note: note, err: err}
	}
}

func promptCmd(key, name, prompt, note string) tea.Cmd {
	return func() tea.Msg {
		return dispatchMsg{key: key, note: note, err: PromptAgent(name, prompt)}
	}
}

func notifyCmd(title, body string) tea.Cmd {
	return func() tea.Msg {
		_ = Notify(title, body) // a missed notification is not worth an error line
		return nil
	}
}

func (m model) fiveHour() (Limit, bool) {
	for _, l := range m.limits {
		if l.Window == "5h" {
			return l, true
		}
	}
	return Limit{}, false
}

// runWatchers wakes or starts agents for watched PRs with new activity. A watch whose
// agent is busy, or that the spending guard holds back, keeps its baseline and is
// retried on the next refresh.
func (m *model) runWatchers() []tea.Cmd {
	var cmds []tea.Cmd
	open := map[string]bool{}
	changed := false
	checked, quiet, waiting, held := 0, 0, 0, 0
	for _, p := range m.allPRs() {
		open[p.Key()] = true
		baseline, watched := m.watches[p.Key()]
		if !watched {
			continue
		}
		checked++
		if m.inflight[p.Key()] || !Triggered(p, baseline) {
			quiet++
			continue
		}
		if m.paused {
			held++
			continue
		}
		name := AgentName(p)
		a, live := m.byName[name]
		switch {
		case live && (a.Status == "working" || a.Status == "blocked"):
			waiting++
			continue
		case live:
			cmds = append(cmds, promptCmd(p.Key(), name, FollowUpPrompt(p), "watcher woke "+name))
		default:
			cmds = append(cmds, m.startCmd(p, StartPrompt(p), "watcher started "+name))
		}
		m.addEvents(Event{time.Now(), EventInfo, "watcher → " + name + " for " + ref(p)})
		if p.Mine {
			cmds = append(cmds, notifyCmd("cutie: new feedback", ref(p)+": "+p.Title))
		}
		m.inflight[p.Key()] = true
		m.watches[p.Key()] = Baseline(p)
		changed = true
	}
	// Every run leaves a line, so a check that finds nothing is visible too.
	if checked > 0 {
		parts := []string{fmt.Sprintf("watchers checked %d PR(s)", checked)}
		if fired := checked - quiet - waiting - held; fired > 0 {
			parts = append(parts, fmt.Sprintf("%d woken", fired))
		}
		if quiet > 0 {
			parts = append(parts, fmt.Sprintf("%d with nothing new from reviewers", quiet))
		}
		if waiting > 0 {
			parts = append(parts, fmt.Sprintf("%d waiting for a busy agent", waiting))
		}
		if held > 0 {
			parts = append(parts, fmt.Sprintf("%d held by the spending guard", held))
		}
		m.addEvents(Event{time.Now(), EventInfo, strings.Join(parts, " · ")})
	}
	for k := range m.watches {
		if !open[k] { // merged or closed
			delete(m.watches, k)
			changed = true
		}
	}
	if changed {
		if err := m.watches.Save(); err != nil {
			m.setStatus("saving watches: "+err.Error(), true)
		}
	}
	return cmds
}

func (m model) activate() (model, tea.Cmd) {
	it := m.selectedItem()
	if it == nil {
		return m, nil
	}
	if it.header {
		k := m.collapseKey(it.repo)
		m.collapsed[k] = !m.collapsed[k]
		return m, nil
	}
	p := *it.pr
	name := AgentName(p)
	if _, live := m.byName[name]; live {
		if err := FocusAgent(name); err != nil {
			m.setStatus(err.Error(), true)
		} else {
			m.setStatus("jumped to "+name, false)
		}
		return m, nil
	}
	if m.inflight[p.Key()] {
		m.setStatus(name+" is already starting", false)
		return m, nil
	}
	if !p.Status.Actionable() {
		m.setStatus(fmt.Sprintf("#%d is %s: nothing for an agent to do", p.Number, p.Status.Label()), false)
		return m, nil
	}
	m.inflight[p.Key()] = true
	m.setStatus("starting "+name+"…", false)
	return m, m.startCmd(p, StartPrompt(p), "started "+name)
}

func (m model) toggleWatch() model {
	p := m.selectedPR()
	if p == nil {
		return m
	}
	if _, ok := m.watches[p.Key()]; ok {
		delete(m.watches, p.Key())
		m.setStatus(fmt.Sprintf("stopped watching #%d", p.Number), false)
	} else {
		// Baseline is the current state: only activity from now on triggers the agent.
		m.watches[p.Key()] = Baseline(*p)
		what := "new review feedback"
		if !p.Mine {
			what = "new commits"
		}
		m.setStatus(fmt.Sprintf("watching #%d for %s", p.Number, what), false)
	}
	if err := m.watches.Save(); err != nil {
		m.setStatus("saving watches: "+err.Error(), true)
	}
	return m
}

// ---- update ----

// Update applies msg, then keeps the cursor in range and the scroll offset following it.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	n := len(next.items())
	next.cursor = max(min(next.cursor, n-1), 0)
	next.offset = next.scroll()
	return next, cmd
}

func (m model) update(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case rateMsg:
		m.rate = msg.rate
		m.setStatus(fmt.Sprintf("GitHub rate limit reached: polling resumes at %s", m.rate.ResetAt.Local().Format("15:04")), true)

	case prsMsg:
		m.loading = false
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
			if strings.Contains(strings.ToLower(msg.err.Error()), "rate limit") {
				// Learn the reset time over REST so polling waits instead of failing each minute.
				return m, func() tea.Msg {
					r, err := FetchRateLimit()
					if err != nil {
						return nil
					}
					return rateMsg{r}
				}
			}
			return m, nil
		}
		m.rate = msg.rate
		var cmds []tea.Cmd
		if !m.refreshed.IsZero() {
			prev := map[string]PR{}
			for _, p := range m.allPRs() {
				prev[p.Key()] = p
			}
			m.login, m.mine, m.review = msg.login, msg.mine, msg.review
			next := map[string]PR{}
			for _, p := range m.allPRs() {
				next[p.Key()] = p
			}
			m.addEvents(PREvents(prev, next, time.Now())...)
		} else {
			m.login, m.mine, m.review = msg.login, msg.mine, msg.review
		}
		m.refreshed = time.Now()
		// The list refreshes every minute; watchers act at most every watchGap, so
		// agents are woken once per batch of feedback instead of per comment.
		if m.agents != nil && time.Since(m.watchedAt) >= m.cfg.watchGap {
			m.watchedAt = time.Now()
			cmds = append(cmds, m.runWatchers()...)
		}
		return m, tea.Batch(cmds...)

	case agentsMsg:
		m.agentBusy = false
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
			return m, nil
		}
		var cmds []tea.Cmd
		if m.agents != nil {
			evs, blocked := AgentEvents(m.agents, msg.agents, time.Now())
			m.addEvents(evs...)
			for _, a := range blocked {
				cmds = append(cmds, notifyCmd("cutie: agent needs you", a.Label()))
			}
		}
		m.agents = msg.agents
		m.byName = map[string]Agent{}
		for _, a := range msg.agents {
			if a.Name != "" {
				m.byName[a.Name] = a
			}
		}
		if m.tailFor != "" {
			if _, live := m.byName[m.tailFor]; !live {
				m.tail, m.tailFor = nil, ""
			}
		}
		return m, tea.Batch(append(cmds, m.tailCmd())...)

	case tailMsg:
		m.tail, m.tailFor = msg.lines, msg.name

	case limitsMsg:
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
			return m, nil
		}
		m.limits = msg.limits
		if l, ok := m.fiveHour(); ok {
			was := m.paused
			m.paused = l.Used >= m.cfg.guard
			switch {
			case m.paused && !was:
				m.addEvents(Event{time.Now(), EventBad, fmt.Sprintf("watchers paused: 5h window at %.0f%%", l.Used*100)})
			case was && !m.paused:
				m.addEvents(Event{time.Now(), EventGood, "watchers resumed"})
			}
		}
		return m, nil

	case statsMsg:
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
			return m, nil
		}
		m.stats = &msg.stats

	case ghTickMsg:
		if m.loading {
			return m, m.tick(msg) // previous poll still running
		}
		if m.rateLow() {
			m.setStatus(fmt.Sprintf("GitHub budget low (%d left): polling resumes at %s", m.rate.Remaining, m.rate.ResetAt.Local().Format("15:04")), true)
			return m, m.tick(msg)
		}
		m.loading = true
		return m, tea.Batch(m.fetchPRs(), m.tick(msg))
	case agentTickMsg:
		if m.agentBusy {
			return m, m.tick(msg)
		}
		m.agentBusy = true
		return m, tea.Batch(m.fetchAgents(), m.tick(msg))
	case limitTickMsg:
		return m, tea.Batch(fetchLimits, m.tick(msg))
	case statsTickMsg:
		return m, tea.Batch(m.fetchStats(), m.tick(msg))

	case dispatchMsg:
		delete(m.inflight, msg.key)
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
		} else {
			m.setStatus(msg.note, false)
			m.addEvents(Event{time.Now(), EventInfo, msg.note})
		}
		return m, nil

	case tea.MouseMsg:
		if m.mode != inputNone {
			return m, nil
		}
		switch {
		case m.drag != borderNone && msg.Action == tea.MouseActionMotion:
			m.dragTo(msg.X, msg.Y)
		case m.drag != borderNone && msg.Action == tea.MouseActionRelease:
			m.dragTo(msg.X, msg.Y)
			m.drag = borderNone
			if err := m.sizes.Save(); err != nil {
				m.setStatus("saving sizes: "+err.Error(), true)
			} else {
				m.setStatus("layout saved · = resets it", false)
			}
		case msg.Button == tea.MouseButtonWheelUp:
			m.cursor--
		case msg.Button == tea.MouseButtonWheelDown:
			m.cursor++
		case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
			if b := m.borderAt(msg.X, msg.Y); b != borderNone {
				m.drag = b
				m.setStatus("resizing: drag, then release", false)
				return m, nil
			}
			if row, ok := m.itemAt(msg.X, msg.Y); ok {
				if row == m.cursor {
					return m.activate() // click a selected row = Enter
				}
				m.cursor = row
				return m, m.tailCmd()
			}
		}

	case tea.KeyMsg:
		if m.mode != inputNone {
			return m.updateInput(msg)
		}
		return m.updateKey(msg)
	}
	return m, nil
}

func (m model) updateInput(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.mode == inputFilter {
			m.filter = ""
		}
		m.mode = inputNone
		m.input.Blur()
		return m, nil
	case "enter":
		mode, text := m.mode, strings.TrimSpace(m.input.Value())
		m.mode = inputNone
		m.input.Blur()
		if mode == inputPrompt && text != "" {
			p := m.selectedPR()
			if p == nil {
				return m, nil
			}
			name := AgentName(*p)
			m.setStatus("sending to "+name+"…", false)
			return m, promptCmd(p.Key(), name, text, "sent to "+name)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.mode == inputFilter {
		m.filter = m.input.Value()
		m.cursor = 0
	}
	return m, cmd
}

// closeAgent closes the pane of the selected PR's agent. The first `x` asks for
// confirmation (it ends the agent's session); a second `x` on the same PR closes it.
func (m model) closeAgent() (model, tea.Cmd) {
	p := m.selectedPR()
	if p == nil {
		return m, nil
	}
	name := AgentName(*p)
	a, live := m.byName[name]
	if !live {
		m.confirmClose = ""
		m.setStatus(fmt.Sprintf("#%d has no agent pane to close", p.Number), true)
		return m, nil
	}
	if m.confirmClose != name {
		m.confirmClose = name
		warning := ""
		if a.Status == "working" || a.Status == "blocked" {
			warning = " It is " + a.Status + "!"
		}
		m.setStatus(fmt.Sprintf("Close %s's pane (%s)?%s Press x again to close, any other key to keep it.", name, a.PaneID, warning), true)
		return m, nil
	}
	m.confirmClose = ""
	m.setStatus("closing "+name+"…", false)
	key, pane := p.Key(), a.PaneID
	return m, func() tea.Msg {
		return dispatchMsg{key: key, note: "closed " + name + "'s pane", err: ClosePane(pane)}
	}
}

func (m model) updateKey(msg tea.KeyMsg) (model, tea.Cmd) {
	if msg.String() != "x" && m.confirmClose != "" {
		m.setStatus("kept "+m.confirmClose+"'s pane", false) // any other key cancels a pending close
		m.confirmClose = ""
	}
	moved := true
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "x":
		return m.closeAgent()
	case "up", "k":
		m.cursor--
	case "down", "j":
		m.cursor++
	case "pgup":
		m.cursor -= m.listHeight()
	case "pgdown":
		m.cursor += m.listHeight()
	case "home":
		m.cursor = 0
	case "end":
		m.cursor = len(m.items()) - 1
	case "tab", "shift+tab":
		m.tab = 1 - m.tab
		m.cursor = 0
	case "1":
		m.tab, m.cursor = 0, 0
	case "2":
		m.tab, m.cursor = 1, 0
	default:
		moved = false
	}
	if moved {
		m.cursor = max(min(m.cursor, len(m.items())-1), 0)
		return m, m.tailCmd() // refresh the preview for the new selection
	}

	switch msg.String() {
	case "enter":
		return m.activate()
	case " ":
		if it := m.selectedItem(); it != nil {
			k := m.collapseKey(it.repo)
			m.collapsed[k] = !m.collapsed[k]
			if !it.header { // keep the cursor on the group being folded
				for i, x := range m.items() {
					if x.header && x.repo == it.repo {
						m.cursor = i
					}
				}
			}
		}
	case "w":
		m = m.toggleWatch()
	case "o":
		if p := m.selectedPR(); p != nil {
			if err := exec.Command("open", p.URL).Start(); err != nil {
				m.setStatus(err.Error(), true)
			}
		}
	case "s":
		m.sort = (m.sort + 1) % 3
		m.setStatus("sort: "+m.sort.String(), false)
	case "g":
		m.grouped = !m.grouped
		m.cursor = 0
	case "b":
		m.hideBots = !m.hideBots
		m.cursor = 0
	case "d":
		m.hideDrafts = !m.hideDrafts
		m.cursor = 0
	case "/":
		m.mode = inputFilter
		m.input.SetValue(m.filter)
		m.input.CursorEnd()
		return m, m.input.Focus()
	case "p":
		p := m.selectedPR()
		if p == nil {
			return m, nil
		}
		if _, live := m.byName[AgentName(*p)]; !live {
			m.setStatus(fmt.Sprintf("#%d has no agent to prompt", p.Number), true)
			return m, nil
		}
		m.mode = inputPrompt
		m.input.SetValue("")
		return m, m.input.Focus()
	case "=":
		m.sizes = DefaultSizes()
		if err := m.sizes.Save(); err != nil {
			m.setStatus("saving sizes: "+err.Error(), true)
		} else {
			m.setStatus("layout reset", false)
		}
	case "?":
		m.help = !m.help
	case "esc":
		m.help = false
		if m.filter != "" {
			m.filter = ""
			m.cursor = 0
		}
	case "R":
		// Force: refresh everything and run the watchers on the result, without waiting
		// for the watcher interval. The spending guard still applies.
		m.watchedAt = time.Time{}
		m.setStatus("force refresh: watchers run when GitHub answers", false)
		if m.loading {
			return m, nil // the fetch in flight will run them
		}
		m.loading = true
		return m, tea.Batch(m.fetchPRs(), fetchLimits, m.fetchStats())
	case "r":
		if !m.loading {
			m.loading = true
			return m, tea.Batch(m.fetchPRs(), fetchLimits, m.fetchStats())
		}
	}
	return m, nil
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	watches, err := LoadWatches()
	if err != nil {
		fmt.Fprintln(os.Stderr, "loading watches:", err)
		os.Exit(1)
	}
	sizes, err := LoadSizes()
	if err != nil {
		fmt.Fprintln(os.Stderr, "loading sizes:", err)
		os.Exit(1)
	}
	if _, err := tea.NewProgram(newModel(cfg, watches, sizes), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
