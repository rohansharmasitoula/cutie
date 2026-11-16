package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	cAccent = lipgloss.Color("12")
	cDim    = lipgloss.Color("8")
	cGood   = lipgloss.Color("10")
	cWarn   = lipgloss.Color("11")
	cBad    = lipgloss.Color("9")
	cPurple = lipgloss.Color("13")
	cCyan   = lipgloss.Color("14")

	bold   = lipgloss.NewStyle().Bold(true)
	dim    = lipgloss.NewStyle().Foreground(cDim)
	accent = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	good   = lipgloss.NewStyle().Foreground(cGood)
	warn   = lipgloss.NewStyle().Foreground(cWarn)
	bad    = lipgloss.NewStyle().Foreground(cBad)
	sel    = lipgloss.NewStyle().Reverse(true)

	statusColor = map[Status]lipgloss.Color{
		StatusDraft:           cDim,
		StatusInReview:        cWarn,
		StatusNeedsWork:       cBad,
		StatusApproved:        cGood,
		StatusToReview:        cPurple,
		StatusReviewedUpdated: cBad,
		StatusReviewed:        cDim,
	}
	agentColor = map[string]lipgloss.Color{
		"working": cGood, "blocked": cBad, "idle": cWarn, "done": cCyan, "unknown": cDim,
	}
)

func fg(c lipgloss.Color, s string) string { return lipgloss.NewStyle().Foreground(c).Render(s) }

// fit truncates s to w cells and pads it with spaces to exactly w cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "…")
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}

// spread puts left and right on one w-cell line, truncating left to make room.
func spread(left, right string, w int) string {
	rw := ansi.StringWidth(right)
	if rw >= w {
		return fit(right, w)
	}
	return fit(left, w-rw-1) + " " + right
}

func ago(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return dur(time.Since(t))
}

func dur(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func until(t time.Time) string {
	d := time.Until(t)
	if d <= 0 {
		return "now"
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh%02d", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd%dh", int(d.Hours()/24), int(d.Hours())%24)
}

func money(v float64) string {
	if v < 0 {
		return ""
	}
	if v >= 100 {
		return fmt.Sprintf("$%.0f", v)
	}
	return fmt.Sprintf("$%.2f", v)
}

func tokens(n int64) string {
	switch {
	case n >= 1e9:
		return fmt.Sprintf("%.1fB", float64(n)/1e9)
	case n >= 1e6:
		return fmt.Sprintf("%.0fM", float64(n)/1e6)
	case n >= 1e3:
		return fmt.Sprintf("%.0fK", float64(n)/1e3)
	}
	return fmt.Sprint(n)
}

func usageColor(frac float64) lipgloss.Color {
	switch {
	case frac >= 0.85:
		return cBad
	case frac >= 0.6:
		return cWarn
	}
	return cGood
}

func bar(frac float64, w int) string {
	n := min(max(int(frac*float64(w)+0.5), 0), w)
	return fg(usageColor(frac), strings.Repeat("█", n)) + dim.Render(strings.Repeat("░", w-n))
}

func sparkline(vals []int64, w int) string {
	if len(vals) > w {
		vals = vals[len(vals)-w:]
	}
	var top int64 = 1
	for _, v := range vals {
		top = max(top, v)
	}
	const ticks = "▁▂▃▄▅▆▇█"
	r := []rune(ticks)
	var b strings.Builder
	for _, v := range vals {
		b.WriteRune(r[int(v*int64(len(r)-1)/top)])
	}
	return fg(cCyan, b.String())
}

// panel draws a rounded box of exactly w×h cells with a title in the top border.
func panel(title string, body []string, w, h int, focused bool) string {
	if w < 4 || h < 2 {
		return ""
	}
	bc := cDim
	if focused {
		bc = cAccent
	}
	b := lipgloss.NewStyle().Foreground(bc)
	inner := w - 2
	t := ""
	if title != "" {
		t = " " + ansi.Truncate(title, inner-3, "…") + " "
	}
	top := b.Render("╭─") + bold.Render(t) + b.Render(strings.Repeat("─", max(inner-1-ansi.StringWidth(t), 0))+"╮")
	lines := []string{top}
	for i := range h - 2 {
		l := ""
		if i < len(body) {
			l = body[i]
		}
		lines = append(lines, b.Render("│")+fit(l, inner)+b.Render("│"))
	}
	lines = append(lines, b.Render("╰"+strings.Repeat("─", inner)+"╯"))
	return strings.Join(lines, "\n")
}

// ---- layout ----

const (
	headerH = 1
	footerH = 2
)

func clamp(v, lo, hi int) int { return max(lo, min(v, max(lo, hi))) }

func (m model) activityH() int {
	h := 4
	if m.height >= 34 {
		h = 6
	}
	if m.sizes.Activity > 0 {
		h = m.sizes.Activity
	}
	return clamp(h, 3, m.height-headerH-footerH-8)
}

func (m model) mainH() int { return max(m.height-headerH-footerH-m.activityH(), 6) }

func (m model) wide() bool { return m.width >= 110 }

func (m model) leftW() int {
	if !m.wide() {
		return m.width
	}
	return clamp(int(float64(m.width)*m.sizes.List), 40, m.width-40)
}

func (m model) rightW() int { return m.width - m.leftW() }

// detailH is the height of the detail panel; the agents and Claude panels get the rest.
func (m model) detailH() int {
	return clamp(int(float64(m.mainH())*m.sizes.Detail), 4, m.mainH()-4)
}

// agentsW is the width of the agents panel; the Claude panel gets the rest.
func (m model) agentsW() int {
	return clamp(int(float64(m.rightW())*m.sizes.Agents), 14, m.rightW()-14)
}

// border is a draggable edge between panels.
type border int

const (
	borderNone     border = iota
	borderList            // list | right column
	borderDetail          // detail / agents+Claude
	borderAgents          // agents | Claude
	borderActivity        // everything above / activity
)

// borderAt reports which panel edge (either of its two border cells) is at x, y.
func (m model) borderAt(x, y int) border {
	near := func(v, edge int) bool { return v == edge-1 || v == edge }
	mainTop, mainEnd := headerH, headerH+m.mainH()
	switch {
	case near(y, mainEnd):
		return borderActivity
	case !m.wide() || y < mainTop || y >= mainEnd:
		return borderNone
	case near(x, m.leftW()):
		return borderList
	case x > m.leftW() && near(y, mainTop+m.detailH()):
		return borderDetail
	case y >= mainTop+m.detailH() && near(x, m.leftW()+m.agentsW()):
		return borderAgents
	}
	return borderNone
}

// dragTo moves the dragged border to x, y.
func (m *model) dragTo(x, y int) {
	frac := func(v, total int) float64 { return float64(v) / float64(max(total, 1)) }
	switch m.drag {
	case borderList:
		m.sizes.List = frac(x, m.width)
	case borderDetail:
		m.sizes.Detail = frac(y-headerH, m.mainH())
	case borderAgents:
		m.sizes.Agents = frac(x-m.leftW(), m.rightW())
	case borderActivity:
		m.sizes.Activity = clamp(m.height-footerH-y, 3, m.height-headerH-footerH-8)
	}
}

// listHeight is the number of item rows the list panel shows (borders and tab bar excluded).
func (m model) listHeight() int { return max(m.mainH()-3, 1) }

// scroll returns the first visible item, keeping the cursor on screen and showing the
// group header above the first row of a group.
func (m model) scroll() int {
	h := m.listHeight()
	n := len(m.items())
	off := m.offset
	cur := m.cursor
	if cur <= off {
		off = cur
		if its := m.items(); cur > 0 && cur < len(its) && !its[cur].header && its[cur-1].header {
			off = cur - 1
		}
	}
	if cur >= off+h {
		off = cur - h + 1
	}
	return max(min(off, n-h), 0)
}

func (m model) itemAt(x, y int) (int, bool) {
	top := headerH + 2 // panel border + tab bar
	if x >= m.leftW()-1 || y < top || y >= top+m.listHeight() {
		return 0, false
	}
	i := y - top + m.offset
	if i >= len(m.items()) {
		return 0, false
	}
	return i, true
}

// ---- view ----

func (m model) View() string {
	if m.width == 0 {
		return ""
	}
	mainH := m.mainH()
	left := panel(m.listTitle(), m.listBody(m.leftW()-2), m.leftW(), mainH, m.drag == borderList || m.drag == borderNone)
	body := left
	if m.wide() {
		rw, aw, detailH := m.rightW(), m.agentsW(), m.detailH()
		bottomH := mainH - detailH
		bottom := lipgloss.JoinHorizontal(lipgloss.Top,
			panel(m.agentsTitle(), m.agentsBody(aw-2), aw, bottomH, m.drag == borderAgents),
			panel("Claude · 24h", m.tokensBody(rw-aw-2), rw-aw, bottomH, m.drag == borderAgents))
		detailTitle, detail := m.detail(rw - 2)
		right := lipgloss.JoinVertical(lipgloss.Left, panel(detailTitle, detail, rw, detailH, m.drag == borderList || m.drag == borderDetail), bottom)
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}
	activity := panel("Activity", m.activityBody(m.width-2), m.width, m.activityH(), m.drag == borderActivity)
	return lipgloss.JoinVertical(lipgloss.Left, m.headerBar(), body, activity, m.footer())
}

func (m model) headerBar() string {
	parts := []string{accent.Render("✿ cutie")}
	if m.login != "" {
		parts = append(parts, dim.Render("@"+m.login))
	}
	if m.rate.Limit > 0 {
		used := 1 - float64(m.rate.Remaining)/float64(m.rate.Limit)
		parts = append(parts, "gh "+fg(usageColor(used), fmt.Sprintf("%d left", m.rate.Remaining))+dim.Render(" ↺"+until(m.rate.ResetAt)))
	}
	for _, l := range m.limits {
		parts = append(parts, fmt.Sprintf("%s %s %s %s", l.Window, bar(l.Used, 10), fg(usageColor(l.Used), fmt.Sprintf("%.0f%%", l.Used*100)), dim.Render("↺"+until(l.ResetsAt))))
	}
	if m.stats != nil {
		parts = append(parts, "24h "+bold.Render(money(m.stats.Cost)))
	}
	counts := map[string]int{}
	for _, a := range m.agents {
		counts[a.Status]++
	}
	if n := counts["working"]; n > 0 {
		parts = append(parts, good.Render(fmt.Sprintf("● %d working", n)))
	}
	if n := counts["blocked"]; n > 0 {
		parts = append(parts, bad.Bold(true).Render(fmt.Sprintf("▲ %d need you", n)))
	}
	if m.paused {
		parts = append(parts, bad.Render("⏸ watchers paused"))
	}
	right := dim.Render("⟳ refreshing")
	if !m.loading && !m.refreshed.IsZero() {
		right = dim.Render("⟳ " + ago(m.refreshed))
	}
	return spread(" "+strings.Join(parts, dim.Render("  │  ")), right+" ", m.width)
}

func (m model) footer() string {
	var line1 string
	switch m.mode {
	case inputFilter:
		line1 = accent.Render(" / ") + m.input.View() + dim.Render("   enter keep · esc clear")
	case inputPrompt:
		name := ""
		if p := m.selectedPR(); p != nil {
			name = AgentName(*p)
		}
		line1 = accent.Render(" → "+name+" ") + m.input.View() + dim.Render("   enter send · esc cancel")
	default:
		if m.statusErr {
			line1 = bad.Render(" " + m.status)
		} else {
			line1 = dim.Render(" " + m.status)
		}
	}
	keys := " enter agent · w watch · p prompt · x close agent · o open · tab mine/review · / filter · s sort · g group · b bots · d drafts · space fold · R force · drag borders to resize · = reset · ? help · q quit"
	return fit(line1, m.width) + "\n" + dim.Render(fit(keys, m.width))
}

// ---- list ----

func (m model) listTitle() string {
	t := fmt.Sprintf("PRs · sort %s", m.sort)
	if m.grouped {
		t += " · by repo"
	}
	if m.filter != "" {
		t += " · filter “" + m.filter + "”"
	}
	return t
}

func (m model) listBody(w int) []string {
	tab := func(label string, n int, on bool) string {
		s := fmt.Sprintf(" %s %d ", label, n)
		if on {
			return lipgloss.NewStyle().Reverse(true).Bold(true).Foreground(cAccent).Render(s)
		}
		return dim.Render(s)
	}
	countNeed := func(prs []PR) int {
		n := 0
		for _, p := range prs {
			if p.Status.Actionable() && !(m.hideBots && p.IsBot) {
				n++
			}
		}
		return n
	}
	tabs := tab("Mine", len(m.mine), m.tab == 0) + " " + tab("Review", len(m.review), m.tab == 1)
	var notes []string
	if n := countNeed(m.mine); n > 0 {
		notes = append(notes, bad.Render(fmt.Sprintf("%d need you", n)))
	}
	if n := countNeed(m.review); n > 0 {
		notes = append(notes, fg(cPurple, fmt.Sprintf("%d to review", n)))
	}
	if h := m.hiddenCount(); h > 0 {
		notes = append(notes, dim.Render(fmt.Sprintf("%d hidden", h)))
	}
	lines := []string{spread(tabs, strings.Join(notes, dim.Render(" · ")), w)}

	its := m.items()
	if len(its) == 0 {
		msg := "nothing here"
		if m.refreshed.IsZero() {
			msg = "loading from GitHub…"
		}
		return append(lines, dim.Render("  "+msg))
	}
	end := min(m.offset+m.listHeight(), len(its))
	for i := m.offset; i < end; i++ {
		lines = append(lines, m.renderItem(its[i], i == m.cursor, w))
	}
	return lines
}

func (m model) renderItem(it item, selected bool, w int) string {
	if it.header {
		arrow := "▾"
		if m.collapsed[m.collapseKey(it.repo)] {
			arrow = "▸"
		}
		line := accent.Render(arrow+" "+it.repo) + dim.Render(fmt.Sprintf("  %d", it.count))
		if selected {
			return sel.Render(fit(ansi.Strip(line), w))
		}
		return line
	}
	p := *it.pr
	watch := " "
	if _, ok := m.watches[p.Key()]; ok {
		watch = fg(cCyan, "◉")
	}
	ci := map[string]string{"SUCCESS": good.Render("✓"), "FAILURE": bad.Render("✗"), "ERROR": bad.Render("✗"), "PENDING": warn.Render("•"), "EXPECTED": warn.Render("•")}[p.CI]
	if ci == "" {
		ci = " "
	}
	badge := fit(fg(statusColor[p.Status], p.Status.Label()), 11)
	ref := fmt.Sprintf("#%d", p.Number)
	if !m.grouped {
		ref = p.RepoName + " " + ref
	}
	indent := ""
	if it.depth > 0 {
		indent = strings.Repeat("  ", it.depth-1) + dim.Render("└ ")
	}

	var right []string
	if a, ok := m.byName[AgentName(p)]; ok {
		cell := "● " + a.Status
		if c := money(a.Cost); c != "" {
			cell += " " + c
		}
		right = append(right, fg(agentColor[a.Status], cell))
	} else if m.inflight[p.Key()] {
		right = append(right, dim.Render("◌ starting"))
	}
	if n := p.Open(); n > 0 {
		right = append(right, bad.Render(fmt.Sprintf("%d open", n)))
	}
	if n := p.Unresolved() - p.Open(); n > 0 {
		right = append(right, dim.Render(fmt.Sprintf("%d replied", n)))
	}
	if !p.Mine {
		right = append(right, dim.Render("@"+p.Author))
	}
	if wait := ago(p.LastCommit); wait != "" {
		right = append(right, fg(waitColor(time.Since(p.LastCommit)), wait))
	}

	left := fmt.Sprintf(" %s %s %s %s%s %s", watch, badge, ci, indent, bold.Render(ref), p.Title)
	line := spread(left, strings.Join(right, "  "), w)
	if selected {
		return sel.Render(ansi.Strip(line))
	}
	return line
}

// waitColor warms with time since the last push: fresh, a day, several days.
func waitColor(d time.Duration) lipgloss.Color {
	switch {
	case d >= 3*24*time.Hour:
		return cBad
	case d >= 24*time.Hour:
		return cWarn
	}
	return cDim
}

// ---- detail ----

func (m model) detail(w int) (string, []string) {
	if m.help {
		return "Help", helpLines()
	}
	it := m.selectedItem()
	if it == nil {
		return "Detail", []string{dim.Render("select a PR")}
	}
	if it.header {
		return it.repo, m.repoSummary(it.repo)
	}
	p := *it.pr
	var l []string
	add := func(s ...string) { l = append(l, s...) }

	add(bold.Render(p.Title))
	meta := fmt.Sprintf("%s → %s · @%s · updated %s ago", p.Branch, p.Base, p.Author, ago(p.UpdatedAt))
	add(dim.Render(meta))
	state := fg(statusColor[p.Status], p.Status.Label())
	if p.Decision != "" {
		state += dim.Render(" · " + strings.ToLower(strings.ReplaceAll(p.Decision, "_", " ")))
	}
	if !p.LastCommit.IsZero() {
		state += dim.Render(" · last push " + ago(p.LastCommit) + " ago")
	}
	if _, ok := m.watches[p.Key()]; ok {
		state += fg(cCyan, " · ◉ watching")
	}
	add(state, "")

	// Checks
	var pass, fail, pend int
	var failed []string
	for _, c := range p.Checks {
		switch c.State {
		case CheckPass:
			pass++
		case CheckFail:
			fail++
			failed = append(failed, c.Name)
		case CheckPending:
			pend++
		}
	}
	if len(p.Checks) > 0 {
		cl := bold.Render("Checks ") + good.Render(fmt.Sprintf("✓ %d", pass))
		if fail > 0 {
			cl += "  " + bad.Render(fmt.Sprintf("✗ %d", fail))
		}
		if pend > 0 {
			cl += "  " + warn.Render(fmt.Sprintf("• %d", pend))
		}
		if len(failed) > 0 {
			cl += dim.Render("  " + strings.Join(failed, ", "))
		}
		add(cl)
	}

	// Reviewers
	if len(p.Reviewers) > 0 {
		var rs []string
		for _, r := range p.Reviewers {
			mark := map[string]string{"APPROVED": good.Render("✓"), "CHANGES_REQUESTED": bad.Render("✗"), "COMMENTED": dim.Render("💬"), "REQUESTED": warn.Render("⏳"), "DISMISSED": dim.Render("–")}[r.State]
			rs = append(rs, r.Login+" "+mark)
		}
		add(bold.Render("Reviewers ") + strings.Join(rs, dim.Render(" · ")))
	}

	// Agent
	name := AgentName(p)
	add("")
	if a, ok := m.byName[name]; ok {
		head := bold.Render("Agent ") + name + " " + fg(agentColor[a.Status], "● "+a.Status)
		if c := money(a.Cost); c != "" {
			head += dim.Render(" · ") + c
		}
		add(head)
		if m.tailFor == name {
			for _, t := range m.tail {
				add(dim.Render("  › " + t))
			}
		}
	} else if m.inflight[p.Key()] {
		add(dim.Render("Agent " + name + " starting…"))
	} else if p.Status.Actionable() {
		what := "an orchestrator"
		if !p.Mine {
			what = "a reviewer"
		}
		add(accent.Render("⏎ enter") + dim.Render(" starts "+what+" as "+name))
	}

	// Threads go last: they can be long, and the panel cuts what does not fit.
	if n := p.Unresolved(); n > 0 {
		head := fmt.Sprintf("Unresolved threads (%d)", n)
		if r := n - p.Open(); r > 0 {
			head += fmt.Sprintf(" · %d replied by @%s, waiting on the reviewer", r, p.Author)
		}
		add("", bold.Render(head))
		for _, t := range p.Threads {
			loc := t.Path
			if t.Line > 0 {
				loc = fmt.Sprintf("%s:%d", t.Path, t.Line)
			}
			mark := bad.Render("● ")
			if t.Replied {
				mark = dim.Render("↩ ")
			}
			add(mark + warn.Render(t.Author) + dim.Render(" · ") + loc)
			body := strings.Join(strings.Fields(t.Body), " ")
			add(dim.Render("  “" + ansi.Truncate(body, max(w-6, 10), "…") + "”"))
		}
	}
	return fmt.Sprintf("%s #%d", p.RepoName, p.Number), l
}

func (m model) repoSummary(repo string) []string {
	counts := map[Status]int{}
	var cost float64
	for _, p := range m.visiblePRs() {
		if p.RepoName != repo {
			continue
		}
		counts[p.Status]++
		if a, ok := m.byName[AgentName(p)]; ok && a.Cost > 0 {
			cost += a.Cost
		}
	}
	var l []string
	for _, s := range []Status{StatusNeedsWork, StatusReviewedUpdated, StatusToReview, StatusInReview, StatusApproved, StatusReviewed, StatusDraft} {
		if counts[s] > 0 {
			l = append(l, fmt.Sprintf("%s %d", fit(fg(statusColor[s], s.Label()), 12), counts[s]))
		}
	}
	if cost > 0 {
		l = append(l, "", "agents on this repo's PRs: "+money(cost))
	}
	return append(l, "", dim.Render("space folds the group"))
}

func helpLines() []string {
	rows := [][2]string{
		{"↑↓ j k, wheel", "move"},
		{"click", "select; click again = enter"},
		{"enter", "jump to the PR's agent, or start one"},
		{"p", "send a prompt to the PR's agent"},
		{"x x", "close the PR's agent pane (asks first)"},
		{"w", "watch: wake the agent on new feedback / commits"},
		{"o", "open the PR in the browser"},
		{"tab, 1, 2", "Mine / Review"},
		{"/", "filter by text (esc clears)"},
		{"s", "sort: urgent → updated → waiting"},
		{"g", "group by repo on/off"},
		{"space", "fold or unfold a repo"},
		{"b / d", "hide or show bots / drafts"},
		{"r", "refresh now"},
		{"R", "force: refresh and run watchers now"},
		{"drag a border", "resize panels (saved)"},
		{"=", "reset panel sizes"},
		{"? / esc", "close help"},
	}
	var l []string
	for _, r := range rows {
		l = append(l, accent.Render(fit(r[0], 15))+r[1])
	}
	return append(l, "", dim.Render("Watchers pause when the 5h Claude window passes CUTIE_GUARD (default 85%)."))
}

// ---- agents & tokens ----

func (m model) agentsTitle() string {
	return fmt.Sprintf("Agents · %d", len(m.agents))
}

func (m model) agentsBody(w int) []string {
	rank := map[string]int{"blocked": 0, "working": 1, "idle": 2, "done": 3}
	as := append([]Agent(nil), m.agents...)
	sort.SliceStable(as, func(i, j int) bool {
		ri, ok := rank[as[i].Status]
		if !ok {
			ri = 4
		}
		rj, ok := rank[as[j].Status]
		if !ok {
			rj = 4
		}
		return ri < rj
	})
	if len(as) == 0 {
		return []string{dim.Render("no agents running")}
	}
	var l []string
	total := 0.0
	for _, a := range as {
		if a.Cost > 0 {
			total += a.Cost
		}
		label := a.Label()
		if a.Name == "" {
			label = dim.Render(label)
		}
		l = append(l, spread(fg(agentColor[a.Status], "●")+" "+label, dim.Render(money(a.Cost)), w))
	}
	return append([]string{spread(dim.Render("blocked first"), dim.Render("total "+money(total)), w)}, l...)
}

func (m model) tokensBody(w int) []string {
	var l []string
	bw := max(w-20, 6)
	for _, lim := range m.limits {
		l = append(l, fmt.Sprintf("%-3s%s %s %s", lim.Window, bar(lim.Used, bw), fg(usageColor(lim.Used), fmt.Sprintf("%3.0f%%", lim.Used*100)), dim.Render("↺"+until(lim.ResetsAt))))
	}
	if len(m.limits) == 0 {
		l = append(l, dim.Render("plan limits loading…"))
	}
	if m.paused {
		l = append(l, bad.Render(fmt.Sprintf("⏸ watchers paused (5h ≥ %.0f%%)", m.cfg.guard*100)))
	} else {
		next := "next check soon"
		if !m.watchedAt.IsZero() {
			next = "next check in " + dur(time.Until(m.watchedAt.Add(m.cfg.watchGap)))
		}
		l = append(l, dim.Render(fmt.Sprintf("watchers: %s · pause at 5h ≥ %.0f%%", next, m.cfg.guard*100)))
	}
	s := m.stats
	if s == nil {
		return append(l, "", dim.Render("omp stats loading…"))
	}
	l = append(l, "", bold.Render(money(s.Cost))+dim.Render(" · "+tokens(s.Tokens)+" tokens"))
	l = append(l, sparkline(s.HourlyTokens, w))
	l = append(l, spread(dim.Render("-24h"), dim.Render("now"), min(w, len(s.HourlyTokens))))
	if s.Cost > 0 {
		var ms []string
		for _, sh := range s.Models {
			if pct := sh.Cost / s.Cost * 100; pct >= 1 {
				ms = append(ms, fmt.Sprintf("%s %.0f%%", strings.SplitN(sh.Name, "-", 2)[0], pct))
			}
		}
		l = append(l, dim.Render(strings.Join(ms, " · ")))
	}
	for i, f := range s.Folders {
		if i == 3 {
			break
		}
		l = append(l, spread(dim.Render(f.Name), money(f.Cost), w))
	}
	return l
}

// ---- activity ----

func (m model) activityBody(w int) []string {
	if len(m.events) == 0 {
		return []string{dim.Render("quiet so far: new feedback, CI changes, pushes and agent finishes show up here")}
	}
	var l []string
	for _, e := range m.events {
		c := cDim
		switch e.Kind {
		case EventGood:
			c = cGood
		case EventBad:
			c = cBad
		}
		l = append(l, dim.Render(fit(ago(e.At), 4))+fg(c, e.Text))
	}
	return l
}
