// Package ui is the Bubble Tea dashboard.
package ui

import (
	"errors"
	"fmt"
	"github.com/rohansharmasitoula/cutie/internal/agents"
	"github.com/rohansharmasitoula/cutie/internal/config"
	"github.com/rohansharmasitoula/cutie/internal/github"
	"github.com/rohansharmasitoula/cutie/internal/herdr"
	"github.com/rohansharmasitoula/cutie/internal/omp"
	"github.com/rohansharmasitoula/cutie/internal/state"
	"os"
	"path/filepath"
	"sort"
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
	// queueSettle is how long the review queue waits after any agent start or prompt,
	// so the agent list shows the new agent working before the next review is picked.
	queueSettle = 15 * time.Second
)

// ---- messages ----

type prsMsg struct {
	snap github.Snapshot
	err  error
}
type rateMsg struct{ rate github.RateLimit }
type agentsMsg struct {
	agents []herdr.Agent
	err    error
}
type tailMsg struct {
	name  string
	lines []string
}
type limitsMsg struct {
	limits []omp.Limit
	err    error
}
type statsMsg struct {
	stats omp.Stats
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
	cfg     config.Config
	prompts agents.Prompts
	be      backend
	rate    github.RateLimit // GitHub GraphQL budget, from the last answer

	login   string
	mine    []github.PR
	review  []github.PR
	issues  []github.Issue
	agents  []herdr.Agent
	byName  map[string]herdr.Agent
	limits  []omp.Limit
	stats   *omp.Stats
	watches state.Watches
	queue   state.Queue // review PRs waiting for the running review to finish
	// queueAfter is the earliest time the queue may pick its next review.
	queueAfter time.Time
	modes      state.Modes // review PR key → "autopilot"; others are copilot
	// inflight marks PR and issue keys with an agent start or prompt running.
	inflight map[string]bool
	events   []Event

	tail    []string // output of the selected row's agent
	tailFor string   // agent name the tail belongs to

	// view state
	tab        int // tabMine, tabReview or tabIssues
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
	sizes     state.Sizes
	drag      border // the border being dragged with the mouse
	// confirmClose is the agent name awaiting a second `x` to close its pane.
	confirmClose string
}

func newModel(cfg config.Config, ps agents.Prompts, w state.Watches, q state.Queue, md state.Modes, s state.Sizes) model {
	in := textinput.New()
	in.Prompt = ""
	m := model{
		cfg: cfg, prompts: ps, be: newBackend(cfg), watches: w, queue: q, modes: md, sizes: s,
		inflight: map[string]bool{}, byName: map[string]herdr.Agent{}, collapsed: map[string]bool{},
		grouped: true, hideBots: true, loading: true, input: in,
	}
	if cfg.Demo { // never show or overwrite the user's real watches, queue and modes
		mine, review := demoPRs()
		m.watches = state.Watches{mine[0].Key(): agents.Baseline(mine[0]), mine[1].Key(): agents.Baseline(mine[1])}
		m.queue = nil
		m.modes = state.Modes{review[1].Key(): string(agents.Autopilot)}
		m.events = demoEvents()
	}
	return m
}

// modeFor is PR p's review mode: copilot unless you switched it to autopilot.
func (m model) modeFor(p github.PR) agents.ReviewMode {
	if agents.ReviewMode(m.modes[p.Key()]) == agents.Autopilot {
		return agents.Autopilot
	}
	return agents.Copilot
}

// toggleMode switches the selected review PR between copilot and autopilot, queued or
// not. A live review agent, even one mid-review, is told at once; herdr only refuses
// an agent that is waiting on a question of its own.
func (m model) toggleMode() (model, tea.Cmd) {
	p := m.selectedPR()
	if p == nil || p.Mine {
		m.setStatus("review mode is for PRs you review; your own PRs and issues never post", false)
		return m, nil
	}
	next := agents.Autopilot
	if m.modeFor(*p) == agents.Autopilot {
		next = agents.Copilot
		delete(m.modes, p.Key())
	} else {
		m.modes[p.Key()] = string(next)
	}
	what := map[agents.ReviewMode]string{
		agents.Autopilot: "posts its review with a verdict and opens follow-up issues by itself",
		agents.Copilot:   "drafts the review and follow-up issues and waits for your OK",
	}[next]
	m.addEvents(Event{time.Now(), EventInfo, fmt.Sprintf("%s → %s", ref(*p), next)})
	name, a, live := m.agentFor(*p)
	if !live {
		m.save("review modes", m.modes.Save, fmt.Sprintf("#%d → %s: its reviewer %s", p.Number, next, what))
		return m, nil
	}
	m.save("review modes", m.modes.Save, "")
	if a.Status == "blocked" {
		m.setStatus(fmt.Sprintf("#%d → %s, but %s is waiting for you: answer it in its pane, then press m twice to tell it", p.Number, next, name), true)
		return m, nil
	}
	m.setStatus(fmt.Sprintf("telling %s: %s…", name, next), false)
	return m, m.promptCmd(p.Key(), name, agents.ModeChange(next), fmt.Sprintf("%s switched to %s", name, next))
}

// pruneModes forgets the modes of PRs that are no longer open.
func (m *model) pruneModes() {
	open := map[string]bool{}
	for _, p := range m.review {
		open[p.Key()] = true
	}
	changed := false
	for k := range m.modes {
		if !open[k] {
			delete(m.modes, k)
			changed = true
		}
	}
	if changed {
		m.save("review modes", m.modes.Save, "")
	}
}

// Init loads everything once and starts one tick loop per data source. Ticks are only
// rescheduled by their own handler, so manual refreshes never add loops.
func (m model) Init() tea.Cmd {
	return tea.Batch(m.fetchPRs(), m.fetchLimits(), m.fetchStats(),
		func() tea.Msg { return agentTickMsg{} },
		m.tick(ghTickMsg{}), m.tick(limitTickMsg{}), m.tick(statsTickMsg{}))
}

func (m model) tick(msg tea.Msg) tea.Cmd {
	d := agentPollInterval
	switch msg.(type) {
	case ghTickMsg:
		d = m.cfg.Interval
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
	be := m.be
	return func() tea.Msg {
		snap, err := be.Snapshot()
		return prsMsg{snap, err}
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
	be := m.be
	return func() tea.Msg {
		list, err := be.Agents()
		return agentsMsg{list, err}
	}
}

// tailCmd reads the selected row's agent output, if it has a live agent.
func (m model) tailCmd() tea.Cmd {
	w, ok := m.selectedWork()
	if !ok {
		return nil
	}
	name := w.agent
	if _, live := m.byName[name]; !live {
		return nil
	}
	be := m.be
	return func() tea.Msg {
		lines, err := be.Tail(name, tailLines)
		if err != nil {
			lines = []string{err.Error()}
		}
		return tailMsg{name, lines}
	}
}

// fetchLimits and fetchStats return nil without omp; tea.Batch skips nil commands.
func (m model) fetchLimits() tea.Cmd {
	if !m.cfg.HasOmp {
		return nil
	}
	be := m.be
	return func() tea.Msg {
		l, err := be.Limits()
		return limitsMsg{l, err}
	}
}

func (m model) fetchStats() tea.Cmd {
	if !m.cfg.HasOmp {
		return nil
	}
	be := m.be
	return func() tea.Msg {
		s, err := be.Stats()
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

// repoDir finds a repo's clone: a folder named after the repo, directly in the root or
// one folder down (e.g. ~/Development/acme/api), so clones grouped by org work as is.
func (m model) repoDir(repo, repoName string) (string, error) {
	if m.cfg.Demo {
		return "", nil
	}
	nested, _ := filepath.Glob(filepath.Join(m.cfg.Root, "*", repoName))
	for _, dir := range append([]string{filepath.Join(m.cfg.Root, repoName)}, nested...) {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return dir, nil
		}
	}
	return "", fmt.Errorf("no clone of %s in %s or one folder below it (set CUTIE_ROOT to the folder holding your clones)", repo, m.cfg.Root)
}

// ---- list items ----

const (
	tabMine = iota
	tabReview
	tabIssues
	tabCount
)

// item is one list row: a repo header, a PR or an issue.
type item struct {
	header bool
	repo   string
	count  int
	pr     *github.PR
	issue  *github.Issue
	depth  int // stack depth inside a repo group
}

// work is what an agent works on, a PR or an issue, as the agent actions need it.
type work struct {
	key, url, repo, repoName string
	number                   int
	agent, tab               string // herdr agent name and tab label
}

func prWork(p github.PR) work {
	return work{p.Key(), p.URL, p.Repo, p.RepoName, p.Number, agents.Name(p), agents.TabLabel(p)}
}

func issueWork(i github.Issue) work {
	return work{i.Key(), i.URL, i.Repo, i.RepoName, i.Number, agents.IssueName(i), agents.IssueTabLabel(i)}
}

// work is the row's PR or issue; repo headers have none.
func (it item) work() (work, bool) {
	switch {
	case it.pr != nil:
		return prWork(*it.pr), true
	case it.issue != nil:
		return issueWork(*it.issue), true
	}
	return work{}, false
}

func (m model) collapseKey(repo string) string { return fmt.Sprintf("%d/%s", m.tab, repo) }

func (m model) visiblePRs() []github.PR {
	src := m.mine
	if m.tab == tabReview {
		src = m.review
	}
	f := strings.ToLower(m.filter)
	var out []github.PR
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
	if m.tab == tabIssues {
		return 0 // bots and drafts are PR filters
	}
	src := m.mine
	if m.tab == tabReview {
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

func (m model) sortPRs(prs []github.PR) {
	sort.SliceStable(prs, func(i, j int) bool {
		a, b := prs[i], prs[j]
		switch m.sort {
		case sortUpdated:
			return a.UpdatedAt.After(b.UpdatedAt)
		case sortWaiting:
			return a.LastCommit.Before(b.LastCommit)
		}
		if a.Status.Urgency() != b.Status.Urgency() {
			return a.Status.Urgency() < b.Status.Urgency()
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
	if m.tab == tabIssues {
		return m.issueItems()
	}
	prs := m.visiblePRs()
	if !m.grouped {
		out := make([]item, len(prs))
		for i := range prs {
			out[i] = item{pr: &prs[i]}
		}
		return out
	}
	byRepo := map[string][]*github.PR{}
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
		branch := map[string]*github.PR{}
		for _, p := range group {
			branch[p.Branch] = p
		}
		children := map[string][]*github.PR{}
		var roots []*github.PR
		for _, p := range group {
			// A stack is the same author's PR built on another of their PR branches; a PR
			// that merely targets a shared branch (staging) is not stacked on its release PR.
			if parent, ok := branch[p.Base]; ok && parent != p && parent.Author == p.Author && !longLived[parent.Branch] {
				children[parent.Branch] = append(children[parent.Branch], p)
			} else {
				roots = append(roots, p)
			}
		}
		seen := map[*github.PR]bool{}
		var walk func(p *github.PR, depth int)
		walk = func(p *github.PR, depth int) {
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

// visibleIssues is the Issues tab after the filter, newest activity first (oldest
// first when sorting by waiting).
func (m model) visibleIssues() []github.Issue {
	f := strings.ToLower(m.filter)
	var out []github.Issue
	for _, i := range m.issues {
		if f != "" && !strings.Contains(strings.ToLower(fmt.Sprintf("%s #%d %s @%s %s", i.RepoName, i.Number, i.Title, i.Author, strings.Join(i.Labels, " "))), f) {
			continue
		}
		out = append(out, i)
	}
	sort.SliceStable(out, func(a, b int) bool {
		if m.sort == sortWaiting {
			return out[a].UpdatedAt.Before(out[b].UpdatedAt)
		}
		return out[a].UpdatedAt.After(out[b].UpdatedAt)
	})
	return out
}

// issueItems lists issues, under repo headers when grouped.
func (m model) issueItems() []item {
	is := m.visibleIssues()
	if !m.grouped {
		out := make([]item, len(is))
		for k := range is {
			out[k] = item{issue: &is[k]}
		}
		return out
	}
	byRepo := map[string][]*github.Issue{}
	var repos []string
	for k := range is {
		r := is[k].RepoName
		if _, ok := byRepo[r]; !ok {
			repos = append(repos, r)
		}
		byRepo[r] = append(byRepo[r], &is[k])
	}
	sort.Strings(repos)
	var out []item
	for _, r := range repos {
		out = append(out, item{header: true, repo: r, count: len(byRepo[r])})
		if m.collapsed[m.collapseKey(r)] {
			continue
		}
		for _, i := range byRepo[r] {
			out = append(out, item{repo: r, issue: i})
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

func (m model) selectedPR() *github.PR {
	if it := m.selectedItem(); it != nil {
		return it.pr
	}
	return nil
}

// selectedWork is the selected row's PR or issue.
func (m model) selectedWork() (work, bool) {
	if it := m.selectedItem(); it != nil {
		return it.work()
	}
	return work{}, false
}

// allPRs is both sections, for watchers and change detection.
func (m model) allPRs() []github.PR {
	out := make([]github.PR, 0, len(m.mine)+len(m.review))
	out = append(out, m.mine...)
	return append(out, m.review...)
}

// ---- agents ----

// agentFor is the herdr agent name cutie uses for p, and that agent if it is live.
func (m model) agentFor(p github.PR) (string, herdr.Agent, bool) {
	name := agents.Name(p)
	a, live := m.byName[name]
	return name, a, live
}

// startCmd finds or creates the PR's pane and starts an agent there with the start prompt.
func (m model) startCmd(p github.PR, note string) tea.Cmd {
	prompt, err := m.prompts.Start(p, m.modeFor(p))
	return m.startWorkCmd(prWork(p), prompt, err, note)
}

// startWorkCmd finds or creates w's pane in its repo workspace and starts an agent
// there with prompt.
func (m model) startWorkCmd(w work, prompt string, promptErr error, note string) tea.Cmd {
	dir, dirErr := m.repoDir(w.repo, w.repoName)
	kind, be := m.cfg.AgentKind, m.be
	return func() tea.Msg {
		err := errors.Join(dirErr, promptErr)
		if err == nil {
			err = be.Start(w.repoName, w.tab, dir, w.agent, kind, prompt)
		}
		return dispatchMsg{key: w.key, note: note, err: err}
	}
}

// followUpCmd sends the PR's live agent the follow-up prompt.
func (m model) followUpCmd(p github.PR, name, note string) tea.Cmd {
	prompt, err := m.prompts.FollowUp(p, m.modeFor(p))
	if err != nil {
		return func() tea.Msg { return dispatchMsg{key: p.Key(), err: err} }
	}
	return m.promptCmd(p.Key(), name, prompt, note)
}

func (m model) promptCmd(key, name, prompt, note string) tea.Cmd {
	be := m.be
	return func() tea.Msg {
		return dispatchMsg{key: key, note: note, err: be.Prompt(name, prompt)}
	}
}

func (m model) notifyCmd(title, body string) tea.Cmd {
	be := m.be
	return func() tea.Msg {
		_ = be.Notify(title, body) // a missed notification is not worth an error line
		return nil
	}
}

// fiveHour is the 5 hour window the spending guard watches. With several Claude accounts
// logged in, it is the one with the most room left, since agents can still run on it.
func (m model) fiveHour() (omp.Limit, bool) {
	var best omp.Limit
	found := false
	for _, l := range m.limits {
		if l.Window == "5h" && (!found || l.Used < best.Used) {
			best, found = l, true
		}
	}
	return best, found
}

// runWatchers wakes or starts agents for watched PRs with new activity. A watch whose
// agent is busy, or that the spending guard holds back, keeps its baseline and is
// retried on the next refresh.
func (m *model) runWatchers() []tea.Cmd {
	var cmds []tea.Cmd
	open := map[string]bool{}
	changed := false
	checked, quiet, waiting, held, queued := 0, 0, 0, 0, 0
	for _, p := range m.allPRs() {
		open[p.Key()] = true
		baseline, watched := m.watches[p.Key()]
		if !watched {
			continue
		}
		checked++
		if m.inflight[p.Key()] || !agents.Triggered(p, baseline) {
			quiet++
			continue
		}
		// A re-request on a PR you review joins the review queue, which runs reviews
		// one at a time; the new baseline marks this re-request as handled.
		if !p.Mine {
			if m.queuePos(p.Key()) == 0 {
				m.queue = append(m.queue, p.Key())
				m.save("review queue", m.queue.Save, "")
				m.addEvents(Event{time.Now(), EventInfo, fmt.Sprintf("watcher queued %s (%d in queue)", ref(p), len(m.queue))})
			}
			queued++
			m.watches[p.Key()] = agents.Baseline(p)
			changed = true
			continue
		}
		if m.paused {
			held++
			continue
		}
		name, a, live := m.agentFor(p)
		switch {
		case live && (a.Status == "working" || a.Status == "blocked"):
			waiting++
			continue
		case live:
			cmds = append(cmds, m.followUpCmd(p, name, "watcher woke "+name))
		default:
			cmds = append(cmds, m.startCmd(p, "watcher started "+name))
		}
		m.addEvents(Event{time.Now(), EventInfo, "watcher → " + name + " for " + ref(p)})
		if p.Mine {
			cmds = append(cmds, m.notifyCmd("cutie: new feedback", ref(p)+": "+p.Title))
		}
		m.inflight[p.Key()] = true
		m.watches[p.Key()] = agents.Baseline(p)
		changed = true
	}
	// Every run leaves a line, so a check that finds nothing is visible too.
	if checked > 0 {
		parts := []string{fmt.Sprintf("watchers checked %d PR(s)", checked)}
		if fired := checked - quiet - waiting - held - queued; fired > 0 {
			parts = append(parts, fmt.Sprintf("%d woken", fired))
		}
		if queued > 0 {
			parts = append(parts, fmt.Sprintf("%d sent to the review queue", queued))
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
		m.save("watches", m.watches.Save, "")
	}
	return cmds
}

// queueHold says why the review queue cannot start its next review now, or "" when it
// can: reviews run one at a time, so it waits for any review agent that is starting,
// working or waiting for you, then for queueSettle, and holds under the spending guard.
func (m model) queueHold() string {
	switch {
	case m.refreshed.IsZero():
		return "loading"
	case m.paused:
		return "held by the spending guard"
	}
	for _, p := range m.review {
		if m.inflight[p.Key()] {
			return "starting " + agents.Name(p)
		}
	}
	for _, a := range m.agents {
		if agents.IsReview(a.Name) && (a.Status == "working" || a.Status == "blocked") {
			return "waiting for " + a.Name + " (" + a.Status + ")"
		}
	}
	if wait := time.Until(m.queueAfter); wait > 0 {
		return fmt.Sprintf("next in %ds", int(wait.Seconds())+1)
	}
	return ""
}

// runQueue starts the next queued review when queueHold allows it. Queued PRs that were
// merged, closed or no longer need a review leave the queue with an Activity line.
func (m *model) runQueue() tea.Cmd {
	if len(m.queue) == 0 || m.queueHold() != "" {
		return nil
	}
	byKey := map[string]github.PR{}
	for _, p := range m.review {
		byKey[p.Key()] = p
	}
	defer m.save("review queue", m.queue.Save, "")
	for len(m.queue) > 0 {
		key := m.queue[0]
		m.queue = m.queue[1:]
		p, ok := byKey[key]
		if !ok {
			m.addEvents(Event{time.Now(), EventInfo, key + " left the review queue: merged or closed"})
			continue
		}
		if !p.Status.Actionable() {
			m.addEvents(Event{time.Now(), EventInfo, ref(p) + " left the review queue: " + p.Status.Label()})
			continue
		}
		name, _, live := m.agentFor(p)
		note := fmt.Sprintf("review queue started %s (%d left)", name, len(m.queue))
		var cmd tea.Cmd
		switch {
		case !live:
			cmd = m.startCmd(p, note)
		case p.Status == github.StatusReRequested:
			cmd = m.followUpCmd(p, name, note)
		default: // an idle agent from an earlier session: hand it the review
			prompt, err := m.prompts.Start(p, m.modeFor(p))
			if err != nil {
				return func() tea.Msg { return dispatchMsg{key: key, err: err} }
			}
			cmd = m.promptCmd(key, name, prompt, note)
		}
		m.inflight[key] = true
		return cmd
	}
	return nil
}

// toggleQueue adds the selected review PR to the end of the review queue, or takes it out.
func (m model) toggleQueue() model {
	it := m.selectedItem()
	if it == nil || it.header {
		return m
	}
	if it.pr == nil || it.pr.Mine {
		m.setStatus("the queue is for PRs you review; enter starts an agent here", false)
		return m
	}
	p := *it.pr
	for i, k := range m.queue {
		if k == p.Key() {
			m.queue = append(m.queue[:i:i], m.queue[i+1:]...)
			m.save("review queue", m.queue.Save, fmt.Sprintf("took #%d out of the review queue", p.Number))
			return m
		}
	}
	if !p.Status.Actionable() {
		m.setStatus(fmt.Sprintf("#%d is %s: nothing for an agent to do", p.Number, p.Status.Label()), false)
		return m
	}
	m.queue = append(m.queue, p.Key())
	m.save("review queue", m.queue.Save, fmt.Sprintf("queued #%d for review (%d in queue)", p.Number, len(m.queue)))
	return m
}

// queuePos is the PR's 1-based place in the review queue, or 0 when it is not queued.
func (m model) queuePos(key string) int {
	for i, k := range m.queue {
		if k == key {
			return i + 1
		}
	}
	return 0
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
	w, _ := it.work()
	if _, live := m.byName[w.agent]; live {
		if err := m.be.Focus(w.agent); err != nil {
			m.setStatus(err.Error(), true)
		} else {
			m.setStatus("jumped to "+w.agent, false)
		}
		return m, nil
	}
	if m.inflight[w.key] {
		m.setStatus(w.agent+" is already starting", false)
		return m, nil
	}
	var cmd tea.Cmd
	if it.issue != nil {
		prompt, err := m.prompts.IssueStart(*it.issue)
		cmd = m.startWorkCmd(w, prompt, err, "started "+w.agent)
	} else {
		p := *it.pr
		if !p.Status.Actionable() {
			m.setStatus(fmt.Sprintf("#%d is %s: nothing for an agent to do", p.Number, p.Status.Label()), false)
			return m, nil
		}
		cmd = m.startCmd(p, "started "+w.agent)
	}
	m.inflight[w.key] = true
	m.setStatus("starting "+w.agent+"…", false)
	return m, cmd
}

func (m model) toggleWatch() model {
	p := m.selectedPR()
	if p == nil {
		if it := m.selectedItem(); it != nil && it.issue != nil {
			m.setStatus("watchers follow PRs; issues have none", false)
		}
		return m
	}
	if _, ok := m.watches[p.Key()]; ok {
		delete(m.watches, p.Key())
		m.setStatus(fmt.Sprintf("stopped watching #%d", p.Number), false)
	} else {
		// Baseline is the current state: only activity from now on triggers the agent.
		m.watches[p.Key()] = agents.Baseline(*p)
		what := "new review feedback"
		if !p.Mine {
			what = "a re-request"
		}
		m.setStatus(fmt.Sprintf("watching #%d for %s", p.Number, what), false)
	}
	m.save("watches", m.watches.Save, m.status)
	return m
}

// save writes a state file (not in demo mode) and reports the error, or ok when non-empty.
func (m *model) save(what string, write func() error, ok string) {
	var err error
	if !m.cfg.Demo {
		err = write()
	}
	switch {
	case err != nil:
		m.setStatus("saving "+what+": "+err.Error(), true)
	case ok != "":
		m.setStatus(ok, false)
	}
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
					r, err := github.FetchRateLimit()
					if err != nil {
						return nil
					}
					return rateMsg{r}
				}
			}
			return m, nil
		}
		m.rate = msg.snap.Rate
		var cmds []tea.Cmd
		if !m.refreshed.IsZero() {
			prev := map[string]github.PR{}
			for _, p := range m.allPRs() {
				prev[p.Key()] = p
			}
			m.login, m.mine, m.review, m.issues = msg.snap.Login, msg.snap.Mine, msg.snap.Review, msg.snap.Issues
			next := map[string]github.PR{}
			for _, p := range m.allPRs() {
				next[p.Key()] = p
			}
			m.addEvents(PREvents(prev, next, time.Now())...)
		} else {
			m.login, m.mine, m.review, m.issues = msg.snap.Login, msg.snap.Mine, msg.snap.Review, msg.snap.Issues
		}
		m.refreshed = time.Now()
		m.pruneModes()
		// The list refreshes every minute; watchers act at most every watchGap, so
		// agents are woken once per batch of feedback instead of per comment.
		if m.agents != nil && time.Since(m.watchedAt) >= m.cfg.WatchGap {
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
				cmds = append(cmds, m.notifyCmd("cutie: agent needs you", a.Label()))
			}
		}
		m.agents = msg.agents
		m.byName = map[string]herdr.Agent{}
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
		cmds = append(cmds, m.runQueue()) // before the return, which copies m
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
			m.paused = l.Used >= m.cfg.Guard
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
		return m, tea.Batch(m.fetchLimits(), m.tick(msg))
	case statsTickMsg:
		return m, tea.Batch(m.fetchStats(), m.tick(msg))

	case dispatchMsg:
		delete(m.inflight, msg.key)
		m.queueAfter = time.Now().Add(queueSettle)
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
			m.save("sizes", m.sizes.Save, "layout saved · = resets it")
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
			w, ok := m.selectedWork()
			if !ok {
				return m, nil
			}
			m.setStatus("sending to "+w.agent+"…", false)
			return m, m.promptCmd(w.key, w.agent, text, "sent to "+w.agent)
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

// closeAgent closes the pane of the selected row's agent. The first `x` asks for
// confirmation (it ends the agent's session); a second `x` on the same row closes it.
func (m model) closeAgent() (model, tea.Cmd) {
	w, ok := m.selectedWork()
	if !ok {
		return m, nil
	}
	name := w.agent
	a, live := m.byName[name]
	if !live {
		m.confirmClose = ""
		m.setStatus(fmt.Sprintf("#%d has no agent pane to close", w.number), true)
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
	key, pane, be := w.key, a.PaneID, m.be
	return m, func() tea.Msg {
		return dispatchMsg{key: key, note: "closed " + name + "'s pane", err: be.Close(pane)}
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
	case "tab":
		m.tab, m.cursor = (m.tab+1)%tabCount, 0
	case "shift+tab":
		m.tab, m.cursor = (m.tab+tabCount-1)%tabCount, 0
	case "1":
		m.tab, m.cursor = tabMine, 0
	case "2":
		m.tab, m.cursor = tabReview, 0
	case "3":
		m.tab, m.cursor = tabIssues, 0
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
	case "m":
		return m.toggleMode()
	case "a":
		m = m.toggleQueue()
	case "o":
		if w, ok := m.selectedWork(); ok {
			if err := m.be.Open(w.url); err != nil {
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
		w, ok := m.selectedWork()
		if !ok {
			return m, nil
		}
		if _, live := m.byName[w.agent]; !live {
			m.setStatus(fmt.Sprintf("#%d has no agent to prompt", w.number), true)
			return m, nil
		}
		m.mode = inputPrompt
		m.input.SetValue("")
		return m, m.input.Focus()
	case "=":
		m.sizes = state.DefaultSizes()
		m.save("sizes", m.sizes.Save, "layout reset")
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
		return m, tea.Batch(m.fetchPRs(), m.fetchLimits(), m.fetchStats())
	case "r":
		if !m.loading {
			m.loading = true
			return m, tea.Batch(m.fetchPRs(), m.fetchLimits(), m.fetchStats())
		}
	}
	return m, nil
}

// Run shows the dashboard until the user quits.
func Run(cfg config.Config, ps agents.Prompts, w state.Watches, q state.Queue, md state.Modes, s state.Sizes) error {
	_, err := tea.NewProgram(newModel(cfg, ps, w, q, md, s), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}
