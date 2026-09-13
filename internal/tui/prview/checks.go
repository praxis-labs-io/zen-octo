package prview

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/paint"
)

const (
	checkParentPrefix = "workflow\x00"
	checkJobPrefix    = "job\x00"
)

type RerunCheckMsg struct {
	Repo  string
	JobID int64
	Name  string
}

// RerunRunMsg asks the root to rerun run RunID's failed jobs, or every job when All is set.
// JobIDs are the jobs it marked, for a refusal to release.
type RerunRunMsg struct {
	Repo   string
	RunID  int64
	Name   string
	All    bool
	JobIDs []int64
}

type checkGroup struct {
	name   string
	checks []gh.Check
	state  gh.CheckState

	// Grouped by run, not name: a workflow on both push and pull_request reports one name over two runs.
	runID int64
}

const jobSettleDelay = 150 * time.Millisecond

type rerunPending struct {
	jobID       int64
	startedAt   time.Time
	completedAt time.Time
	acceptedAt  time.Time

	// GitHub drops the old attempt on queueing a rerun; these hold its row until the new one arrives.
	check gh.Check
	at    int
}

type checkTreeRow struct {
	key      string
	label    string
	checkKey string
	state    gh.CheckState
	count    int
	depth    int
	parent   bool
	folded   bool

	runID int64
}

// selected survives a rerun via Check.Key; wanted and job use JobID so an old attempt's log never shows.
type checks struct {
	cursor   int
	groups   []checkGroup
	rows     []checkTreeRow
	selected string
	folded   map[string]bool

	wanted          int64
	refreshJob      bool
	job             store.Job
	jobStale        bool
	parsing         bool
	sections        []jobSection
	rendered        []string
	renderedBody    string
	renderWidth     int
	renderQuery     string
	rendering       bool
	renderToken     uint64
	renderWantWidth int
	renderWantQuery string

	step       int
	line       int
	stepLines  int
	stepStarts []int
	stepOpen   map[int]bool
	stepSeen   map[int]bool

	searching    bool
	search       comp.Search
	matchLines   []int
	searchStep   int
	searchWithin int

	reruns map[string]rerunPending

	// Holds rerun attempts GitHub has dropped, so it is never evidence of the fetched rollup.
	shown []gh.Check
}

// GitHub reports a rerun's new attempt within seconds; a mark past this is a check that is gone.
const rerunHoldFor = time.Minute

// GitHub keeps every attempt, so a rerun job arrives once per attempt.
func newestAttempt(checks []gh.Check) []gh.Check {
	at := make(map[string]int, len(checks))
	out := make([]gh.Check, 0, len(checks))
	for _, c := range checks {
		i, seen := at[c.LogicalKey()]
		if !seen {
			at[c.LogicalKey()] = len(out)
			out = append(out, c)
			continue
		}
		if laterAttempt(c, out[i]) {
			out[i] = c
		}
	}
	return out
}

func (m *Model) renderedChecks() []gh.Check {
	out := newestAttempt(m.detail.Detail.Rollup.Checks)

	for i := range out {
		if _, marked := m.check.reruns[out[i].LogicalKey()]; marked {
			out[i].State = gh.CheckStatePending
			out[i].StartedAt, out[i].CompletedAt = time.Time{}, time.Time{}
		}
	}

	var holds []rerunPending
	for _, pending := range m.check.reruns {
		if pending.check.Name == "" {
			continue
		}
		if slices.ContainsFunc(out, func(c gh.Check) bool {
			return c.LogicalKey() == pending.check.LogicalKey()
		}) {
			continue
		}
		holds = append(holds, pending)
	}
	slices.SortFunc(holds, func(a, b rerunPending) int { return cmp.Compare(a.at, b.at) })

	for _, pending := range holds {
		held := pending.check
		held.State = gh.CheckStatePending
		held.StartedAt, held.CompletedAt = time.Time{}, time.Time{}
		out = slices.Insert(out, min(pending.at, len(out)), held)
	}
	return out
}

func groupChecks(r gh.CheckRollup) []checkGroup {
	at := make(map[string]int, len(r.Checks))
	var out []checkGroup
	for _, c := range r.Checks {
		group := c.Workflow + "\x00" + strconv.FormatInt(c.RunID, 10)
		i, ok := at[group]
		if !ok {
			i = len(out)
			at[group] = i
			out = append(out, checkGroup{name: c.Workflow, runID: c.RunID})
		}
		out[i].checks = append(out[i].checks, c)
	}
	if i, ok := at["\x000"]; ok && i != len(out)-1 {
		g := out[i]
		out = append(append(out[:i:i], out[i+1:]...), g)
	}
	for i := range out {
		out[i].state = worst(out[i].checks)
	}
	return out
}

func worst(list []gh.Check) gh.CheckState {
	out := gh.CheckStateNone
	for _, c := range list {
		if rank(c.State) > rank(out) {
			out = c.State
		}
	}
	return out
}

func rank(s gh.CheckState) int {
	switch s {
	case gh.CheckStateFailure, gh.CheckStateError:
		return 4
	case gh.CheckStatePending, gh.CheckStateExpected:
		return 3
	case gh.CheckStateSuccess:
		return 2
	case gh.CheckStateSkipped:
		return 1
	}
	return 0
}

func checkParentKey(workflow string, runID int64) string {
	return checkParentPrefix + workflow + "\x00" + strconv.FormatInt(runID, 10)
}
func checkRowKey(c gh.Check) string { return checkJobPrefix + c.Key() }

func flattenChecks(groups []checkGroup, folded map[string]bool) []checkTreeRow {
	var out []checkTreeRow
	for _, g := range groups {
		switch {
		case g.name == "":
			for _, c := range g.checks {
				out = append(out, checkTreeRow{
					key: checkRowKey(c), label: c.Name, checkKey: c.Key(), state: c.State,
				})
			}
		case len(g.checks) == 1:
			c := g.checks[0]
			out = append(out, checkTreeRow{
				key: checkRowKey(c), label: g.name + " / " + c.Name, checkKey: c.Key(), state: c.State,
			})
		default:
			key := checkParentKey(g.name, g.runID)
			closed := folded[key]
			out = append(out, checkTreeRow{
				key: key, label: g.name, state: g.state, count: len(g.checks), parent: true, folded: closed,
				runID: g.runID,
			})
			if closed {
				continue
			}
			for _, c := range g.checks {
				out = append(out, checkTreeRow{
					key: checkRowKey(c), label: c.Name, checkKey: c.Key(), state: c.State, depth: 1,
				})
			}
		}
	}
	return out
}

func (m *Model) syncChecks() {
	var cursorKey string
	if m.check.cursor < len(m.check.rows) {
		cursorKey = m.check.rows[m.check.cursor].key
	}
	if m.check.folded == nil {
		m.check.folded = make(map[string]bool)
	}
	if m.check.reruns == nil {
		m.check.reruns = make(map[string]rerunPending)
	}

	for logical, pending := range m.check.reruns {
		replacement, any := m.rerunReplacement(logical, pending)
		if !any {
			delete(m.check.reruns, logical)
			continue
		}
		if replacement == nil {
			continue
		}
		delete(m.check.reruns, logical)
		if m.check.selected == logical || strings.HasPrefix(m.check.selected, logical+"\x00") {
			m.check.selected = replacement.Key()
		}
	}

	m.check.shown = m.renderedChecks()
	m.check.groups = groupChecks(gh.CheckRollup{Checks: m.check.shown})
	m.check.rows = flattenChecks(m.check.groups, m.check.folded)

	if m.checkForKey(m.check.selected) == nil {
		want := logicalOf(m.check.selected)
		m.check.selected = ""
		if want != "" {
			if at := slices.IndexFunc(m.check.shown, func(c gh.Check) bool {
				return c.LogicalKey() == want
			}); at >= 0 {
				m.check.selected = m.check.shown[at].Key()
			}
		}
		if m.check.selected == "" {
			for _, r := range m.check.rows {
				if r.checkKey != "" {
					m.check.selected = r.checkKey
					break
				}
			}
		}
	}

	m.check.cursor = 0
	found := false
	if cursorKey != "" {
		for i, r := range m.check.rows {
			if r.key == cursorKey {
				m.check.cursor, found = i, true
				break
			}
		}
	}
	if !found {
		for i, r := range m.check.rows {
			if r.checkKey == m.check.selected {
				m.check.cursor = i
				break
			}
		}
	}
	if c := m.selectedCheck(); c == nil {
		m.resetCheckJob()
	} else {
		stateChanged := m.check.job.Loaded && m.check.job.Job.ID == c.JobID && m.check.job.Job.State != c.State
		changed := c.JobID != m.check.wanted || stateChanged
		_, rerunning := m.check.reruns[c.LogicalKey()]
		if changed && !rerunning {
			m.resetCheckJob()
			m.check.refreshJob = stateChanged
		}
	}
	showRow(&m.sideView, m.check.cursor)
}

func (m *Model) shownSlot(c gh.Check) int {
	at := slices.IndexFunc(m.check.shown, func(s gh.Check) bool { return s.Key() == c.Key() })
	if at < 0 {
		return len(m.check.shown)
	}
	return at
}

// Reads the shown set so a held rerun keeps its selection.
func (m *Model) checkForKey(key string) *gh.Check {
	for i := range m.check.shown {
		if m.check.shown[i].Key() == key {
			return &m.check.shown[i]
		}
	}
	return nil
}

func (m *Model) selectedCheck() *gh.Check { return m.checkForKey(m.check.selected) }

func (m Model) rerunReplacement(logical string, pending rerunPending) (*gh.Check, bool) {
	var newest *gh.Check
	for i := range m.detail.Detail.Rollup.Checks {
		check := &m.detail.Detail.Rollup.Checks[i]
		if check.LogicalKey() != logical {
			continue
		}
		if check.JobID == pending.jobID {
			continue
		}
		if newest == nil || attemptTime(*check).After(attemptTime(*newest)) {
			newest = check
		}
	}
	if newest == nil {
		if m.checkForLogical(logical) != nil {
			return nil, true
		}
		if pending.acceptedAt.IsZero() || time.Since(pending.acceptedAt) < rerunHoldFor {
			return nil, true
		}
		return nil, false
	}
	if newest.State == gh.CheckStatePending || newest.State == gh.CheckStateExpected {
		return newest, true
	}
	if !pending.acceptedAt.IsZero() && !newest.StartedAt.IsZero() {
		if newest.StartedAt.Before(pending.acceptedAt.Add(-5 * time.Second)) {
			return nil, true
		}
		return newest, true
	}
	oldAt, newAt := pending.completedAt, attemptTime(*newest)
	if oldAt.IsZero() {
		oldAt = pending.startedAt
	}
	if oldAt.IsZero() || newAt.IsZero() || newAt.After(oldAt) {
		return newest, true
	}
	return nil, true
}

// A live attempt always wins: a queued rerun has no timestamps, and GitHub runs one attempt at a time.
func laterAttempt(a, b gh.Check) bool {
	if live(a) != live(b) {
		return live(a)
	}
	if at, bt := attemptTime(a), attemptTime(b); !at.Equal(bt) {
		return at.After(bt)
	}
	return a.DistinctID > b.DistinctID
}

func logicalOf(key string) string {
	parts := strings.SplitN(key, "\x00", 4)
	if len(parts) < 3 {
		return ""
	}
	return strings.Join(parts[:3], "\x00")
}

func live(c gh.Check) bool {
	return c.State == gh.CheckStatePending || c.State == gh.CheckStateExpected
}

func attemptTime(check gh.Check) time.Time {
	if !check.CompletedAt.IsZero() {
		return check.CompletedAt
	}
	return check.StartedAt
}

func (m *Model) checkForLogical(logical string) *gh.Check {
	for i := range m.detail.Detail.Rollup.Checks {
		if m.detail.Detail.Rollup.Checks[i].LogicalKey() == logical {
			return &m.detail.Detail.Rollup.Checks[i]
		}
	}
	return nil
}

func (m Model) checkRerunning(key string) bool {
	check := m.checkForKey(key)
	if check == nil {
		return false
	}
	_, ok := m.check.reruns[check.LogicalKey()]
	return ok
}

func (m *Model) canRerunCheck() bool {
	if m.tab != tabChecks || m.checkRerunning(m.check.selected) {
		return false
	}
	if _, onRun := m.selectedRun(); onRun {
		return false
	}
	return m.checkHasJob() && m.checkFailed()
}

// Reads the rollup, not the fetched job, which empties on every cursor step.
func (m *Model) checkHasJob() bool {
	check := m.selectedCheck()
	return check != nil && check.JobID != 0
}

func (m *Model) checkFailed() bool {
	check := m.selectedCheck()
	return check != nil &&
		(check.State == gh.CheckStateFailure || check.State == gh.CheckStateError)
}

func (m *Model) rerunCheck() tea.Cmd {
	if !m.canRerunCheck() {
		return nil
	}
	check := *m.selectedCheck()
	if m.check.reruns == nil {
		m.check.reruns = make(map[string]rerunPending)
	}
	m.check.reruns[check.LogicalKey()] = rerunPending{
		jobID: check.JobID, startedAt: check.StartedAt, completedAt: check.CompletedAt,
		check: check, at: m.shownSlot(check),
	}
	m.syncChecks()
	m.resetCheckJob()
	m.syncContent()
	name := cleanJobLabel(check.Name)
	if check.Workflow != "" {
		name = cleanJobLabel(check.Workflow) + " / " + name
	}
	return func() tea.Msg {
		return RerunCheckMsg{Repo: m.pr.Repository, JobID: check.JobID, Name: name}
	}
}

// Needs the column to have the keys, or R from the log pane targets a row nothing points at.
func (m *Model) selectedRun() (checkTreeRow, bool) {
	if m.tab != tabChecks || m.focus != paneSide || m.check.cursor >= len(m.check.rows) {
		return checkTreeRow{}, false
	}
	row := m.check.rows[m.check.cursor]
	if !row.parent || row.runID == 0 {
		return checkTreeRow{}, false
	}
	return row, true
}

func (m *Model) runRerunTargets(row checkTreeRow, all bool) []gh.Check {
	var out []gh.Check
	for _, g := range m.check.groups {
		if checkParentKey(g.name, g.runID) != row.key {
			continue
		}
		for _, c := range g.checks {
			if c.JobID == 0 {
				continue
			}
			if all || c.State == gh.CheckStateFailure || c.State == gh.CheckStateError {
				out = append(out, c)
			}
		}
	}
	return out
}

func (m *Model) canRerunRun(all bool) bool {
	row, ok := m.selectedRun()
	if !ok {
		return false
	}
	targets := m.runRerunTargets(row, all)
	if len(targets) == 0 {
		return false
	}
	if all {
		for _, c := range targets {
			if live(c) {
				return false
			}
		}
	}
	for _, c := range targets {
		if m.checkRerunning(c.Key()) {
			return false
		}
	}
	return true
}

func (m *Model) rerunRun(all bool) tea.Cmd {
	if !m.canRerunRun(all) {
		return nil
	}
	row, _ := m.selectedRun()
	targets := m.runRerunTargets(row, all)

	if m.check.reruns == nil {
		m.check.reruns = make(map[string]rerunPending)
	}
	ids := make([]int64, 0, len(targets))
	for _, c := range targets {
		m.check.reruns[c.LogicalKey()] = rerunPending{
			jobID: c.JobID, startedAt: c.StartedAt, completedAt: c.CompletedAt,
			check: c, at: m.shownSlot(c),
		}
		ids = append(ids, c.JobID)
		if selected := m.selectedCheck(); selected != nil && selected.LogicalKey() == c.LogicalKey() {
			m.resetCheckJob()
		}
	}
	m.syncChecks()
	m.syncContent()

	msg := RerunRunMsg{
		Repo: m.pr.Repository, RunID: row.runID, Name: cleanJobLabel(row.label),
		All: all, JobIDs: ids,
	}
	return func() tea.Msg { return msg }
}

// RunRerunSettled releases the rerun marks on jobIDs after a refused bulk write.
func (m *Model) RunRerunSettled(jobIDs []int64) {
	for _, id := range jobIDs {
		for key, pending := range m.check.reruns {
			if pending.jobID == id {
				delete(m.check.reruns, key)
			}
		}
	}
	m.syncChecks()
	m.syncContent()
}

func (m *Model) RunRerunAccepted(jobIDs []int64, acceptedAt time.Time) {
	for _, id := range jobIDs {
		m.RerunAccepted(id, acceptedAt)
	}
}

func (m *Model) RerunAccepted(jobID int64, acceptedAt time.Time) {
	for key, pending := range m.check.reruns {
		if pending.jobID == jobID {
			pending.acceptedAt = acceptedAt
			m.check.reruns[key] = pending
		}
	}
}

// RerunSettled releases the mark on jobID after a refused rerun.
func (m *Model) RerunSettled(jobID int64) {
	for key, pending := range m.check.reruns {
		if pending.jobID == jobID {
			delete(m.check.reruns, key)
		}
	}
	m.syncChecks()
	m.syncContent()
}

func (m *Model) moveCheck(delta int) {
	if len(m.check.rows) == 0 {
		return
	}
	m.check.cursor = min(max(m.check.cursor+delta, 0), len(m.check.rows)-1)
	droppedHeader := false
	if key := m.check.rows[m.check.cursor].checkKey; key != "" && key != m.check.selected {
		droppedHeader = m.check.searching || !m.check.search.Empty()
		m.check.selected = key
		m.resetCheckJob()
		m.view.SetYOffset(0)
	}
	showRow(&m.sideView, m.check.cursor)
	if droppedHeader {
		m.layout()
	} else {
		m.syncContent()
	}
}

func (m *Model) resetCheckJob() {
	m.check.wanted = 0
	m.check.refreshJob = false
	m.check.job = store.Job{}
	m.check.jobStale = false
	m.check.parsing = false
	m.check.sections = nil
	m.invalidateJobRender()
	m.check.step = 0
	m.check.line = 0
	m.check.stepLines = 0
	m.check.stepStarts = nil
	m.check.stepOpen = nil
	m.check.stepSeen = nil
	m.check.searching = false
	m.check.search = comp.Search{}
	m.check.matchLines = nil
	m.check.searchStep = 0
	m.check.searchWithin = 0
}

func (m *Model) toggleCheckFold() {
	if m.check.cursor >= len(m.check.rows) || !m.check.rows[m.check.cursor].parent {
		return
	}
	key := m.check.rows[m.check.cursor].key
	m.check.folded[key] = !m.check.folded[key]
	m.syncChecks()
	m.syncContent()
}

func (m Model) checkFoldable() bool {
	return m.tab == tabChecks && m.focus == paneSide && m.check.cursor < len(m.check.rows) &&
		m.check.rows[m.check.cursor].parent
}

func (m Model) armJob() tea.Cmd {
	if m.tab != tabChecks {
		return nil
	}
	c := m.selectedCheck()
	if c == nil || c.JobID == 0 || c.JobID == m.check.wanted {
		return nil
	}
	msg := JobSettleMsg{Key: c.Key(), JobID: c.JobID, Refresh: m.check.refreshJob}
	return tea.Tick(jobSettleDelay, func(time.Time) tea.Msg { return msg })
}

func (m *Model) settleJob(msg JobSettleMsg) tea.Cmd {
	c := m.selectedCheck()
	if m.tab != tabChecks || c == nil || c.Key() != msg.Key || c.JobID != msg.JobID ||
		c.JobID == m.check.wanted {
		return nil
	}
	m.check.wanted = c.JobID
	refresh := m.check.refreshJob || msg.Refresh
	m.check.refreshJob = false
	return func() tea.Msg { return NeedJobMsg{JobID: c.JobID, Refresh: refresh} }
}

// PollJob asks again for the selected job while it is pending, failed or stale, on the Checks tab.
func (m *Model) PollJob() tea.Cmd {
	check := m.selectedCheck()
	if m.tab != tabChecks || check == nil || check.JobID == 0 {
		return nil
	}
	pending := check.State == gh.CheckStatePending || check.State == gh.CheckStateExpected
	if !pending && m.check.job.Status != store.StatusFailed && !m.check.jobStale {
		return nil
	}
	return m.refreshJob(check.JobID)
}

// RefreshJob asks for the selected job whatever its state, on the Checks tab only.
func (m *Model) RefreshJob() tea.Cmd {
	check := m.selectedCheck()
	if m.tab != tabChecks || check == nil || check.JobID == 0 {
		return nil
	}
	return m.refreshJob(check.JobID)
}

func (m *Model) refreshJob(id int64) tea.Cmd {
	m.check.wanted = id
	m.check.refreshJob = false
	return func() tea.Msg { return NeedJobMsg{JobID: id, Refresh: true} }
}

type jobParsedMsg struct {
	id       int64
	sections []jobSection
}

// SetJobAsync applies job, returning a command that parses a completed log off the update loop.
func (m *Model) SetJobAsync(id int64, job store.Job) tea.Cmd {
	if !job.Loaded || job.Status == store.StatusFailed ||
		job.Job.State == gh.CheckStatePending || job.Job.State == gh.CheckStateExpected {
		return m.SetJob(id, job)
	}
	c := m.selectedCheck()
	if c == nil || c.JobID != id {
		return nil
	}
	m.takeJob(id, job)
	m.check.jobStale = false
	m.check.parsing = true
	m.check.sections = nil
	m.check.stepLines = 0
	m.check.stepStarts = nil
	m.syncContent()
	jobValue, log := job.Job, job.Log
	return func() tea.Msg {
		return jobParsedMsg{id: id, sections: splitJobLog(jobValue, log)}
	}
}

func (m *Model) jobParsed(msg jobParsedMsg) tea.Cmd {
	c := m.selectedCheck()
	if c == nil || c.JobID != msg.id || m.check.wanted != msg.id {
		return nil
	}
	m.check.parsing = false
	m.check.sections = msg.sections
	m.invalidateJobRender()
	m.syncContent()
	return m.armJobRender()
}

func (m *Model) SetJob(id int64, job store.Job) tea.Cmd {
	c := m.selectedCheck()
	if c == nil || c.JobID != id {
		return nil
	}
	jobPending := job.Job.State == gh.CheckStatePending || job.Job.State == gh.CheckStateExpected
	checkTerminal := c.State != gh.CheckStatePending && c.State != gh.CheckStateExpected
	if job.Loaded && jobPending && checkTerminal {
		m.check.wanted = id
		m.check.jobStale = true
		return nil
	}
	m.takeJob(id, job)
	m.check.parsing = false
	if job.Status != store.StatusFailed {
		m.check.jobStale = false
	}
	if job.Loaded {
		m.check.sections = splitJobLog(job.Job, job.Log)
		if width := m.bodyWidth(); width > 0 {
			m.renderJobSteps(width)
		}
	} else {
		m.check.sections = nil
	}
	m.syncContent()
	if job.Status == store.StatusFailed {
		return nil
	}
	return m.Init()
}

func (m *Model) takeJob(id int64, job store.Job) {
	if m.check.job.Job.ID != id && job.Loaded {
		m.check.step = 0
		m.check.line = 0
		m.check.stepLines = 0
		m.check.stepOpen = make(map[int]bool)
		m.check.stepSeen = make(map[int]bool)
	}
	m.check.wanted = id
	m.check.job = job
	m.invalidateJobRender()
}

func (m *Model) staleJobRender() {
	m.check.renderWidth = 0
	m.check.renderQuery = ""
	m.check.rendering = false
	m.check.renderToken++
}

func (m *Model) invalidateJobRender() {
	m.check.rendered = nil
	m.check.renderedBody = ""
	m.check.renderWidth = 0
	m.check.renderQuery = ""
	m.check.rendering = false
	m.check.renderToken++
}

func (m Model) checkColumn(width int) string {
	if !m.detail.Loaded {
		return ""
	}
	if len(m.check.rows) == 0 {
		return m.faint().Render("No checks.")
	}
	lines := make([]string, len(m.check.rows))
	for i, r := range m.check.rows {
		lines[i] = m.checkTreeLine(r, width, i == m.check.cursor)
	}
	return strings.Join(lines, "\n")
}

func (m Model) checkTreeLine(r checkTreeRow, width int, selected bool) string {
	base := lipgloss.NewStyle()
	if selected {
		base = base.Background(m.theme.SelectedBackground)
	}
	glyph, c := comp.CheckStateIcon(m.theme, r.state)
	fold := ""
	if r.parent {
		fold = "▾ "
		if r.folded {
			fold = "▸ "
		}
	}
	indent := strings.Repeat("  ", r.depth)
	lead := base.Render(indent+fold) + base.Foreground(c).Render(glyph) + base.Render(" ") +
		base.Foreground(m.theme.Text).Render(cleanJobLabel(r.label))
	right := ""
	if r.parent {
		right = base.Foreground(m.theme.Subtle).Render(strconv.Itoa(r.count))
	}
	return m.padTo(m.checkLine(lead, right, width, base), width, base)
}

func (m Model) checkLine(lead, right string, width int, base lipgloss.Style) string {
	room := max(0, width-lipgloss.Width(right)-1)
	if lipgloss.Width(lead) > room {
		lead = paint.Clip(lead, room, base.Foreground(m.theme.Subtle))
	}
	gap := max(1, width-lipgloss.Width(lead)-lipgloss.Width(right))
	return lead + base.Render(strings.Repeat(" ", gap)) + right
}

func (m *Model) checkBody() string {
	if !m.detail.Loaded {
		if m.detail.Status == store.StatusFailed {
			return m.faint().Render("Could not load the checks: " + m.detail.Err.Error())
		}
		return m.spinner.Render("Loading the checks")
	}
	c := m.selectedCheck()
	if c == nil {
		return m.faint().Render("No checks have reported.")
	}
	return m.jobBody(*c, m.bodyWidth())
}
