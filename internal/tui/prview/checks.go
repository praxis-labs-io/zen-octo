package prview

import (
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

// checkGroup is one workflow and the jobs that ran under it. A group with no
// workflow is the status contexts posted directly against the commit.
type checkGroup struct {
	name   string
	checks []gh.Check
	state  gh.CheckState
}

// checkTreeRow is one visible line in the Checks column. A parent folds a
// multi-job workflow; every other row is the same logical check the details
// rail lists.
type checkTreeRow struct {
	key      string
	label    string
	checkKey string
	state    gh.CheckState
	count    int
	depth    int
	parent   bool
	folded   bool
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

	wanted      int64
	job         store.Job
	sections    []jobSection
	rendered    []string
	renderWidth int
	renderQuery string

	step       int
	line       int
	stepLines  int
	stepStarts []int
	stepOpen   map[int]bool
	stepSeen   map[int]bool

	searching  bool
	search     comp.Search
	matchLines []int
	rerunning  bool
	rerunAt    time.Time
}

// groupChecks keeps workflow order from the rollup. Status contexts are flat
// leaves in the tree, but collecting them here and moving them to the end keeps
// the ordering rule in one place.
func groupChecks(r gh.CheckRollup) []checkGroup {
	at := make(map[string]int, len(r.Checks))
	var out []checkGroup
	for _, c := range r.Checks {
		i, ok := at[c.Workflow]
		if !ok {
			i = len(out)
			at[c.Workflow] = i
			out = append(out, checkGroup{name: c.Workflow})
		}
		out[i].checks = append(out[i].checks, c)
	}
	if i, ok := at[""]; ok && i != len(out)-1 {
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

func checkParentKey(workflow string) string { return checkParentPrefix + workflow }
func checkRowKey(c gh.Check) string         { return checkJobPrefix + c.Key() }

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
			key := checkParentKey(g.name)
			closed := folded[key]
			out = append(out, checkTreeRow{
				key: key, label: g.name, state: g.state, count: len(g.checks), parent: true, folded: closed,
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

	m.check.groups = groupChecks(m.detail.Detail.Rollup)
	m.check.rows = flattenChecks(m.check.groups, m.check.folded)

	if m.checkForKey(m.check.selected) == nil {
		m.check.selected = ""
		for _, r := range m.check.rows {
			if r.checkKey != "" {
				m.check.selected = r.checkKey
				break
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
		changed := c.JobID != m.check.wanted ||
			(m.check.job.Loaded && m.check.job.Job.ID == c.JobID && m.check.job.Job.State != c.State)
		if changed && (!m.check.rerunning || m.rerunAttemptVisible(*c)) {
			m.resetCheckJob()
		}
	}
	showRow(&m.sideView, m.check.cursor)
}

func (m *Model) checkForKey(key string) *gh.Check {
	for i := range m.detail.Detail.Rollup.Checks {
		if m.detail.Detail.Rollup.Checks[i].Key() == key {
			return &m.detail.Detail.Rollup.Checks[i]
		}
	}
	return nil
}

func (m *Model) selectedCheck() *gh.Check { return m.checkForKey(m.check.selected) }

func (m Model) rerunAttemptVisible(check gh.Check) bool {
	if check.State == gh.CheckStatePending || check.State == gh.CheckStateExpected {
		return true
	}
	// The rollup can briefly fold an older completed attempt over the failed
	// one after GitHub accepts a rerun. A completed job is the replacement only
	// when it started after this write, not merely because its id differs.
	return check.JobID != m.check.wanted && !check.StartedAt.IsZero() &&
		!check.StartedAt.Before(m.check.rerunAt.Add(-5*time.Second))
}

func (m Model) canRerunCheck() bool {
	if m.tab != tabChecks || m.check.rerunning {
		return false
	}
	check := m.selectedCheck()
	return check != nil && check.JobID != 0 &&
		(check.State == gh.CheckStateFailure || check.State == gh.CheckStateError)
}

func (m *Model) rerunCheck() tea.Cmd {
	if !m.canRerunCheck() {
		return nil
	}
	check := *m.selectedCheck()
	m.check.rerunning = true
	m.check.rerunAt = time.Now()
	m.syncContent()
	name := check.Name
	if check.Workflow != "" {
		name = check.Workflow + " / " + check.Name
	}
	return func() tea.Msg {
		return RerunCheckMsg{Repo: m.pr.Repository, JobID: check.JobID, Name: name}
	}
}

// RerunSettled releases the key only if the answer belongs to the attempt still
// on screen. Polling replaces that attempt when GitHub publishes the rerun.
func (m *Model) RerunSettled(jobID int64) {
	check := m.selectedCheck()
	if check == nil || check.JobID != jobID {
		return
	}
	m.check.rerunning = false
	m.check.rerunAt = time.Time{}
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
	m.check.job = store.Job{}
	m.check.sections = nil
	m.check.rendered = nil
	m.check.renderWidth = 0
	m.check.renderQuery = ""
	m.check.step = 0
	m.check.line = 0
	m.check.stepLines = 0
	m.check.stepStarts = nil
	m.check.stepOpen = nil
	m.check.stepSeen = nil
	m.check.searching = false
	m.check.search = comp.Search{}
	m.check.matchLines = nil
	m.check.rerunning = false
	m.check.rerunAt = time.Time{}
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

// armJob asks the root for the concrete attempt selected now. A status context
// has no Actions job and deliberately asks for nothing.
func (m *Model) armJob() tea.Cmd {
	if m.tab != tabChecks {
		return nil
	}
	c := m.selectedCheck()
	if c == nil || c.JobID == 0 || c.JobID == m.check.wanted {
		return nil
	}
	m.check.wanted = c.JobID
	id := c.JobID
	return func() tea.Msg { return NeedJobMsg{JobID: id} }
}

// SetJob takes the fetched state from the root. A response for a job the cursor
// has left stays in the store and does not repaint this pane.
func (m *Model) SetJob(id int64, job store.Job) tea.Cmd {
	c := m.selectedCheck()
	if c == nil || c.JobID != id {
		return nil
	}
	if m.check.job.Job.ID != id && job.Loaded {
		m.check.step = 0
		m.check.line = 0
		m.check.stepLines = 0
		m.check.stepOpen = make(map[int]bool)
		m.check.stepSeen = make(map[int]bool)
	}
	m.check.wanted = id
	m.check.job = job
	m.check.rendered = nil
	m.check.renderWidth = 0
	m.check.renderQuery = ""
	if job.Loaded {
		// Parsing and sanitizing a real Actions log can mean walking megabytes.
		// Do it when the response lands, not on every j or k over its steps.
		m.check.sections = splitJobLog(job.Job, job.Log)
	} else {
		m.check.sections = nil
	}
	m.syncContent()
	return m.Init()
}

func (m Model) checkTitle() string {
	if !m.detail.Loaded {
		return "Checks"
	}
	return comp.Plural(len(m.detail.Detail.Rollup.Checks), "check")
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
		if m.check.rerunning && r.checkKey == m.check.selected {
			r.state = gh.CheckStatePending
		}
		lines[i] = m.checkTreeLine(r, width, i == m.check.cursor)
	}
	return strings.Join(lines, "\n")
}

func (m Model) checkTreeLine(r checkTreeRow, width int, selected bool) string {
	base := lipgloss.NewStyle()
	if selected {
		base = base.Background(m.theme.SelectedBackground)
	}
	_, c := comp.CheckStateIcon(m.theme, r.state)
	fold := ""
	if r.parent {
		fold = "▾ "
		if r.folded {
			fold = "▸ "
		}
	}
	indent := strings.Repeat("  ", r.depth)
	lead := base.Render(indent+fold) + base.Foreground(c).Render(glyphCheck) + base.Render(" ") +
		base.Foreground(m.theme.Text).Render(r.label)
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
