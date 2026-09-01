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

// RerunCheckMsg asks the root to rerun the selected failed Actions job. The
// concrete job id makes the write precise; the logical key remains selected
// when GitHub replaces it with the new attempt.
type RerunCheckMsg struct {
	Repo  string
	JobID int64
	Name  string
}

// RerunRunMsg asks the root to rerun a whole workflow run, either its failed
// jobs or all of them. It carries the job ids it marked rather than deriving
// them again on the way back: the rollup can be replaced while the write is
// out, and a refusal has to release the marks it actually made.
type RerunRunMsg struct {
	Repo   string
	RunID  int64
	Name   string
	All    bool
	JobIDs []int64
}

// checkGroup is one workflow and the jobs that ran under it. A group with no
// workflow is the status contexts posted directly against the commit.
type checkGroup struct {
	name   string
	checks []gh.Check
	state  gh.CheckState

	// runID is the run the jobs below belong to. A workflow that fires on both
	// push and pull_request reports one name over two runs, and grouped on the
	// name alone one parent row stood over both: R reran the first and marked
	// the jobs of the second, which nothing then retired.
	runID int64
}

// checkTreeRow is one visible line in the Checks column. A parent folds a
// multi-job workflow; every other row is the same logical check the details
// rail lists.
const jobSettleDelay = 150 * time.Millisecond

type rerunPending struct {
	jobID       int64
	startedAt   time.Time
	completedAt time.Time
	acceptedAt  time.Time

	// check is the attempt the write replaces and at is where it sat in the
	// collapsed rollup. GitHub drops the old attempt the moment it queues the
	// rerun, so without these the row leaves the column until the new attempt is
	// reported, and comes back wherever its group happens to end.
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

	// runID is the workflow run behind a parent row, which is what the bulk
	// rerun endpoints take. Job rows leave it zero: those reach their run
	// through the check the selection already names.
	runID int64
}

// checks owns the stable logical selection and the concrete attempt loaded for
// it. selected survives a rerun because Check.Key does; wanted and job use the
// new JobID so an earlier attempt's log can never appear under it.
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

	// shown is what the column draws: the fetched rollup collapsed to one
	// attempt per logical check, with a rerun's own attempt held in place while
	// GitHub has dropped it and not yet reported the new one. It is not the
	// fetched rollup and must never be read as evidence about it, which is why
	// rerunReplacement and checkForLogical stay on Detail.Rollup.
	shown []gh.Check
}

// rerunHoldFor bounds how long a mark survives its check vanishing from the
// rollup. GitHub drops the old attempt when it queues the rerun and reports the
// new one a poll or two later, so the gap is seconds; past this it is a check
// that is gone rather than one on its way, and holding forever would leave a
// row nothing can retire.
const rerunHoldFor = time.Minute

// newestAttempt collapses the rollup to one check per logical key. GitHub keeps
// every attempt, so a job that was skipped and later rerun arrives twice and
// draws two rows for one check. Key tells the attempts apart by DistinctID,
// which is what makes them two rows; LogicalKey is the identity a reader has,
// and the newest attempt is the answer to it.
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
		// The kept attempt holds its place. A newer attempt taking the later
		// slot would walk the row down its group each time it reran.
		if laterAttempt(c, out[i]) {
			out[i] = c
		}
	}
	return out
}

// renderedChecks is the rollup as the column draws it. The hold runs after the
// collapse, because a held attempt is one the collapse found nothing to keep.
func (m *Model) renderedChecks() []gh.Check {
	out := newestAttempt(m.detail.Detail.Rollup.Checks)

	// A marked check reads as running wherever it is read, which is the row and
	// the workflow state above it. Forced at the row alone, worst() went on
	// ranking the attempt being replaced and left a parent marked failing over
	// a job that was already running again.
	for i := range out {
		if _, marked := m.check.reruns[out[i].LogicalKey()]; marked {
			out[i].State = gh.CheckStatePending
			out[i].StartedAt, out[i].CompletedAt = time.Time{}, time.Time{}
		}
	}

	// Ranged straight off the map these went in in whatever order Go handed
	// them over, each one inserted against a slice the one before it had grown:
	// a bulk rerun holds every job of a run at once, and the column drew them in
	// a different order on each sync. Ascending order is what makes a slot mean
	// the same thing to every insert after it.
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
		// Pending rather than the state it failed with. The row is reporting a
		// rerun that is out, and its last verdict is one the reader already
		// acted on by pressing the key.
		held.State = gh.CheckStatePending
		held.StartedAt, held.CompletedAt = time.Time{}, time.Time{}
		out = slices.Insert(out, min(pending.at, len(out)), held)
	}
	return out
}

// groupChecks keeps workflow order from the rollup. Status contexts are flat
// leaves in the tree, but collecting them here and moving them to the end keeps
// the ordering rule in one place.
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
	// The status contexts, which carry neither a workflow nor a run.
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

// flattenChecks makes single-job workflows one row and gives only multi-job
// workflows a parent. Status contexts are never a synthetic workflow.
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

// syncChecks rebuilds the visible tree after a detail or fold changes. Both
// cursor and selected job are restored by stable keys, never by indexes that a
// poll may have moved.
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

	// The marks settle first: both the collapse and the hold under it read the
	// map, and a mark this fetch retired must not hold a row for a frame.
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
		// The same check under a new attempt before dropping to row one. The
		// shown set is keyed on the attempt, so a check rerun anywhere else,
		// in the browser or by a job depending on it, takes the reader's
		// selection out from under them, and dropping straight to the first row
		// landed them on a check they had not been reading.
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

// shownSlot is where a check sits in the set the column is drawing, which is the
// slot a hold puts it back into. A check the set does not carry goes to the end
// rather than to row zero, which is a workflow it does not belong to.
func (m *Model) shownSlot(c gh.Check) int {
	at := slices.IndexFunc(m.check.shown, func(s gh.Check) bool { return s.Key() == c.Key() })
	if at < 0 {
		return len(m.check.shown)
	}
	return at
}

// checkForKey answers off the shown set rather than the fetched rollup, so a
// held rerun keeps its selection through the gap. Anything asking what GitHub
// last reported wants checkForLogical beside it.
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
		// The check is not in the rollup at all. While the write is young that
		// is the gap between GitHub dropping the old attempt and reporting the
		// new one, and dropping the mark here is what took the row with it.
		// Past the window it is a check that is gone, and a mark nothing can
		// retire would hold a row for the rest of the session.
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
	// statusCheckRollup reports the current attempts. Where either side lacks a
	// timestamp, a changed concrete job id is the only ordering evidence GitHub
	// exposes and must not leave the optimistic write locked forever.
	if oldAt.IsZero() || newAt.IsZero() || newAt.After(oldAt) {
		return newest, true
	}
	return nil, true
}

// laterAttempt is whether a is the attempt to draw over b. A queued rerun
// carries neither timestamp, so comparing times alone answered no and the
// column went on drawing the failure the rerun was replacing. GitHub runs one
// attempt of a logical check at a time, so a live one is always the current
// one; past that it is the clock, and past that the id, which counts up.
func laterAttempt(a, b gh.Check) bool {
	if live(a) != live(b) {
		return live(a)
	}
	if at, bt := attemptTime(a), attemptTime(b); !at.Equal(bt) {
		return at.After(bt)
	}
	return a.DistinctID > b.DistinctID
}

// logicalOf takes a check key back to the logical check under it. Key is
// LogicalKey with a distinct id appended where GitHub gave two attempts the
// same display identity, and LogicalKey is three fields, so the first three are
// the check whatever attempt the key named.
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

// canRerunCheck is whether r means the one job the selection names. A parent
// row is where it does not: r means the run there, and the selection under a
// parent is still whichever job the reader last stood on, so without this the
// job case matched first and the key reran one job of a run the reader had
// aimed the key at whole. It is the same question twice on the hint line, which
// carried r against both.
func (m *Model) canRerunCheck() bool {
	if m.tab != tabChecks || m.checkRerunning(m.check.selected) {
		return false
	}
	if _, onRun := m.selectedRun(); onRun {
		return false
	}
	return m.checkHasJob() && m.checkFailed()
}

// checkHasJob is whether the check under the cursor has an Actions job behind
// it, which is what the log keys act on.
//
// It reads the rollup rather than the fetched job, and that is the whole point.
// Moving the cursor empties that job and refills it a debounce and a round trip
// later, so a line derived from it dropped the log keys on every step through
// the column and put them back when the reader stopped, which on a held j is
// the line gone for the length of the walk. The rollup has the answer the whole
// time, the way the list keeps its rows through a reload. The other three tabs
// build their line from the tab; this was the one building it from a fetch.
func (m *Model) checkHasJob() bool {
	check := m.selectedCheck()
	return check != nil && check.JobID != 0
}

// checkFailed is whether the check under the cursor failed, which is half of
// what f jumps into and what r may rerun. The check's own state answers before
// its log has arrived, where walking the fetched steps for a failing one
// cannot.
//
// It is only ever half. A status context carries no job at all, so a failing
// Codecov or Vercel row is a failure with nothing to jump into and nothing to
// rerun, and both keys read checkHasJob beside this.
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
	// The mark decides what the rows and the workflow above them read as, and
	// nothing else rebuilds them until the next fetch lands.
	m.syncChecks()
	// The log under the pane is the attempt being replaced. Held, it reads as
	// this rerun's output and its search and folds answer lines that are on
	// their way out. syncChecks will not do it: the mark is exactly what stops
	// it resetting through the gap.
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

// selectedRun is the workflow run under the cursor, and it answers only on a
// parent row. flattenChecks gives a parent to multi-job workflows alone, so a
// single-job run is a job row and r there is the one-job rerun: the two calls
// do the same thing to a run of one, and the key that is already there is the
// one the reader has.
//
// It reads the column's cursor, so it needs the column to have the keys. That
// is checkFoldable's rule and for its reason: the single-job rerun beside it
// acts on the logical selection and is right from either pane, because the log
// pane is showing that job, where a run is a row and the pane that is not
// drawing rows cannot be aimed at one. Ungated, R from the log made a bulk
// write against a row nothing on the screen was pointing at.
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

// runRerunTargets is which jobs of the run a rerun would replace: the failed
// ones, or every one that has a job behind it. A status context has no job and
// is never a target, the way it is never one for the single-job key.
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

// canRerunRun is whether the run under the cursor has anything the key would
// replace. Rerunning failed jobs where none failed is a call GitHub answers
// with a refusal, so the key goes quiet rather than spending a request to be
// told there was nothing to do.
func (m *Model) canRerunRun(all bool) bool {
	row, ok := m.selectedRun()
	if !ok {
		return false
	}
	targets := m.runRerunTargets(row, all)
	if len(targets) == 0 {
		return false
	}
	// GitHub refuses a whole-run rerun while the run is still going, and the
	// optimistic path would have marked every job and dropped the log the
	// reader was watching before the refusal arrived.
	if all {
		for _, c := range targets {
			if live(c) {
				return false
			}
		}
	}
	// A second press while the first is out settles in whatever order the
	// responses arrive, which is the rule one key over.
	for _, c := range targets {
		if m.checkRerunning(c.Key()) {
			return false
		}
	}
	return true
}

// rerunRun marks every job the write will replace and asks the root for the
// call. The marks are the same ones the single-job key makes, so a bulk rerun
// reads on the column exactly the way one job's does.
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

// RunRerunSettled releases every mark a refused bulk write made. The ids come
// back from the write rather than off the rollup, which may have been replaced
// under it while the call was out.
func (m *Model) RunRerunSettled(jobIDs []int64) {
	for _, id := range jobIDs {
		for key, pending := range m.check.reruns {
			if pending.jobID == id {
				delete(m.check.reruns, key)
			}
		}
	}
	// The mark is what the rows and the workflow above them are computed from,
	// so releasing one has to rebuild them. Left alone the check stayed pending
	// in the shown set and r was dead on a failure GitHub had just refused to
	// rerun.
	m.syncChecks()
	m.syncContent()
}

// RunRerunAccepted stamps every mark the bulk write made, the way the one-job
// answer stamps its own. They stay marked until polling publishes the
// replacements.
func (m *Model) RunRerunAccepted(jobIDs []int64, acceptedAt time.Time) {
	for _, id := range jobIDs {
		m.RerunAccepted(id, acceptedAt)
	}
}

// RerunSettled releases a refused write wherever the reader has navigated.
// Accepted writes stay marked until polling publishes their replacement.
func (m *Model) RerunAccepted(jobID int64, acceptedAt time.Time) {
	for key, pending := range m.check.reruns {
		if pending.jobID == jobID {
			pending.acceptedAt = acceptedAt
			m.check.reruns[key] = pending
		}
	}
}

func (m *Model) RerunSettled(jobID int64) {
	for key, pending := range m.check.reruns {
		if pending.jobID == jobID {
			delete(m.check.reruns, key)
		}
	}
	m.syncChecks()
	m.syncContent()
}

// moveCheck walks visible tree rows. A parent leaves the selected job in the
// pane, the same way a directory row leaves the shown file alone.
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

// armJob starts a short wait for the concrete attempt selected now. A status
// context has no Actions job and deliberately asks for nothing.
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

// settleJob spends only the wait that still names the selected job. Holding a
// movement key can arm one timer per row without fetching any row passed over.
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

// PollJob refreshes volatile step metadata and retries a selected job whose
// last answer failed or contradicted the terminal rollup. Logs remain deferred
// until the metadata says the job is complete.
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

// RefreshJob is the explicit-sync form: the reader asked for the selected job
// regardless of its current state.
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

// SetJobAsync keeps maximum-size log parsing off Bubble Tea's update loop.
// Running jobs and fetch failures remain cheap enough to settle immediately.
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

// SetJob is the synchronous seam used by focused view tests. Production uses
// SetJobAsync so parsing cannot block the application update loop.
func (m *Model) SetJob(id int64, job store.Job) tea.Cmd {
	c := m.selectedCheck()
	if c == nil || c.JobID != id {
		return nil
	}
	jobPending := job.Job.State == gh.CheckStatePending || job.Job.State == gh.CheckStateExpected
	checkTerminal := c.State != gh.CheckStatePending && c.State != gh.CheckStateExpected
	if job.Loaded && jobPending && checkTerminal {
		// A pulse can overtake the REST request. Do not let that stale pending
		// response overwrite the terminal rollup or suppress the now-ready log.
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

// checkBody is the selected job, not another rollup of everything in the
// column. On narrow frames the selected job remains named by its summary even
// though the column is hidden; switching jobs there follows in a later slice.
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
