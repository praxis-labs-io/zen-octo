package prview

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/paint"
)

// statusGroup names the bucket for checks with no workflow behind them. A
// status context is posted straight against the commit, so there is no run to
// file it under.
const statusGroup = "Status checks"

// checkGroup is one workflow and the jobs that ran under it.
type checkGroup struct {
	name   string
	checks []gh.Check
	state  gh.CheckState
}

// checks is the Checks tab: the rollup grouped by workflow, which group the
// column is pointing at, and the line each group's card opens on.
//
// starts is what lets the cursor scroll the pane instead of replacing what is
// in it, the same way the file column scrolls the diff.
type checks struct {
	cursor int
	groups []checkGroup
	starts []int
}

// groupChecks buckets the rollup by the workflow behind each check, in the
// order they first appear. The rail lists the same checks in the same order, so
// the two columns read down together, and a rerun keeps the slot it had rather
// than jumping the list under the cursor.
func groupChecks(r gh.CheckRollup) []checkGroup {
	at := make(map[string]int, len(r.Checks))
	var out []checkGroup

	// Keyed on the workflow rather than on the label, so a repository with a
	// workflow actually called "Status checks" keeps its own group instead of
	// swallowing every status context posted against the commit.
	for _, c := range r.Checks {
		i, ok := at[c.Workflow]
		if !ok {
			i = len(out)
			at[c.Workflow] = i

			name := c.Workflow
			if name == "" {
				name = statusGroup
			}
			out = append(out, checkGroup{name: name})
		}
		out[i].checks = append(out[i].checks, c)
	}

	// The status contexts go last whatever order they arrived in. They are what
	// something outside the repository posted, and they read as a footnote to
	// the workflows this repository runs rather than as one of them.
	if i, ok := at[""]; ok && i != len(out)-1 {
		g := out[i]
		out = append(append(out[:i:i], out[i+1:]...), g)
	}

	for i := range out {
		out[i].state = worst(out[i].checks)
	}
	return out
}

// worst is the one state a workflow reads as: whichever of its jobs most wants
// looking at. One failure among nine passes is a failed run, and a marker that
// said otherwise would be the reason to miss it.
func worst(list []gh.Check) gh.CheckState {
	out := gh.CheckStateNone
	for _, c := range list {
		if rank(c.State) > rank(out) {
			out = c.State
		}
	}
	return out
}

// rank orders the states by how much attention they want. Skipped sits above
// nothing-reported, so a workflow entirely skipped reads as skipped rather than
// as the pass an empty rollup reads as.
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

// syncChecks rebuilds the groups from a rollup that has just landed or changed
// under the cursor. The cursor holds its workflow by name rather than by index:
// a refetch that adds a run would otherwise leave it pointing at a different
// one.
func (m *Model) syncChecks() {
	var was string
	if m.check.cursor < len(m.check.groups) {
		was = m.check.groups[m.check.cursor].name
	}

	m.check.groups = groupChecks(m.detail.Detail.Rollup)
	m.check.cursor = min(m.check.cursor, max(0, len(m.check.groups)-1))

	for i, g := range m.check.groups {
		if g.name == was {
			m.check.cursor = i
			break
		}
	}

	// A run that starts while the tab is open lands above the cursor as often
	// as below it. Holding the workflow moves the cursor's index, and a window
	// left where it was is then a column with nothing marked in it.
	showRow(&m.sideView, m.check.cursor)
}

// moveCheck walks the column, and the pane beside it scrolls to the workflow
// the cursor lands on. The pane holds every workflow either way: the jobs came
// with the detail, so there is nothing to ask for and nothing to swap out.
func (m *Model) moveCheck(delta int) {
	if len(m.check.groups) == 0 {
		return
	}
	m.check.cursor = min(max(m.check.cursor+delta, 0), len(m.check.groups)-1)

	showRow(&m.sideView, m.check.cursor)
	m.syncContent()
	m.showCursorCard()
}

// showCursorCard opens the pane on the selected workflow's heading. The starts
// come from the last render, so this runs after the resync rather than before.
func (m *Model) showCursorCard() {
	if m.check.cursor >= len(m.check.starts) {
		return
	}
	m.view.SetYOffset(contentLead + m.check.starts[m.check.cursor])
}

// checkTitle counts the jobs rather than the workflows. The status contexts are
// in that count and are no one's workflow, so counting groups would name them
// something they are not.
func (m Model) checkTitle() string {
	if !m.detail.Loaded {
		return "Checks"
	}
	return comp.Plural(len(m.detail.Detail.Rollup.Checks), "check")
}

// checkColumn is the workflow list. It paints its own selection, so it hands
// the viewport lines already the full inner width.
func (m Model) checkColumn(width int) string {
	if !m.detail.Loaded {
		return ""
	}
	if len(m.check.groups) == 0 {
		return m.faint().Render("No checks.")
	}

	lines := make([]string, len(m.check.groups))
	for i, g := range m.check.groups {
		lines[i] = m.checkRow(g, width, i == m.check.cursor)
	}
	return strings.Join(lines, "\n")
}

// checkRow is one workflow on one line: its state, its name, and how many jobs
// ran under it. One line rather than the commit column's two, which is what
// keeps the window arithmetic a clamp instead of a multiply.
//
// The marker is the dot the rail uses. Color carries the state, and a column of
// one shape reads down faster than a column of four.
//
// Selection is painted cell by cell, the same way the other two columns paint
// it. Every styled run ends in a reset that clears the background with it, so a
// joined row wrapped in the background style afterwards paints only its first
// cell.
func (m Model) checkRow(g checkGroup, width int, selected bool) string {
	base := lipgloss.NewStyle()
	if selected {
		base = base.Background(m.theme.SelectedBackground)
	}

	_, c := comp.CheckStateIcon(m.theme, g.state)
	lead := base.Foreground(c).Render(glyphCheck) + base.Render(" ") +
		base.Foreground(m.theme.Text).Render(g.name)

	count := base.Foreground(m.theme.Subtle).Render(strconv.Itoa(len(g.checks)))
	return m.padTo(m.checkLine(lead, count, width, base), width, base)
}

// checkLine lays a row out as a lead on the left and a short word on the right,
// the lead giving way when the two will not fit. Every row on this tab has that
// shape, and the count and the state word are each a few cells: clipped, they
// read as real ones.
func (m Model) checkLine(lead, right string, width int, base lipgloss.Style) string {
	room := max(0, width-lipgloss.Width(right)-1)
	if lipgloss.Width(lead) > room {
		lead = paint.Clip(lead, room, base.Foreground(m.theme.Subtle))
	}

	gap := max(1, width-lipgloss.Width(lead)-lipgloss.Width(right))
	return lead + base.Render(strings.Repeat(" ", gap)) + right
}

// checkBody is every workflow's card, in the order the column lists them. The
// pane holds the lot at every width and the cursor scrolls it, the way the file
// column scrolls the diff. Rendering only the selected one would mean a resize
// across the column's floor changed what was on screen without a keypress.
func (m *Model) checkBody() string {
	if !m.detail.Loaded {
		if m.detail.Status == store.StatusFailed {
			return m.faint().Render("Could not load the checks: " + m.detail.Err.Error())
		}
		return m.spinner.Render("Loading the checks")
	}
	if len(m.check.groups) == 0 {
		return m.faint().Render("No checks have reported.")
	}

	width := m.bodyWidth()
	cards := make([]string, len(m.check.groups))
	m.check.starts = make([]int, len(m.check.groups))

	at := 0
	for i, g := range m.check.groups {
		cards[i] = m.checkCard(g, width)
		m.check.starts[i] = at
		// The card's own lines, plus the blank that separates it from the next.
		at += strings.Count(cards[i], "\n") + 2
	}
	return strings.Join(cards, "\n\n")
}

// checkCard is one workflow's jobs in a box of its own, headed by the workflow
// and what its jobs came to.
func (m Model) checkCard(g checkGroup, width int) string {
	inner := max(1, width-2)

	rows := make([]string, len(g.checks))
	for i, c := range g.checks {
		rows[i] = m.jobRow(c, inner)
	}

	pane := comp.NewPane(m.theme).Header(" " + m.checkHead(g, inner-1))
	return pane.Size(width, len(rows)+pane.Chrome()).Render(strings.Join(rows, "\n"))
}

// checkHead is the workflow and its tally, the tally pushed to the far edge.
func (m Model) checkHead(g checkGroup, width int) string {
	icon, c := comp.CheckStateIcon(m.theme, g.state)
	lead := lipgloss.NewStyle().Foreground(c).Render(icon) + " " +
		lipgloss.NewStyle().Foreground(m.theme.Text).Bold(true).Render(g.name)

	line := m.checkLine(lead, m.checkTally(g), width, lipgloss.NewStyle())
	return clipTo(line, width, m.faint())
}

// checkTally counts the jobs by what they came to, in the words the rest of the
// screen uses for them. Only the states with something in them, worst first: a
// run of four counts where three are zero buries the one that matters.
func (m Model) checkTally(g checkGroup) string {
	order := []gh.CheckState{
		gh.CheckStateFailure,
		gh.CheckStateError,
		gh.CheckStatePending,
		gh.CheckStateExpected,
		gh.CheckStateSuccess,
		gh.CheckStateSkipped,
	}

	var parts []string
	for _, s := range order {
		n := 0
		for _, c := range g.checks {
			if c.State == s {
				n++
			}
		}
		if n == 0 {
			continue
		}
		label, c := comp.CheckStateLabel(m.theme, s)
		parts = append(parts, lipgloss.NewStyle().Foreground(c).Render(strconv.Itoa(n)+" "+label))
	}
	return strings.Join(parts, m.faint().Render(" · "))
}

// jobRow is one job: its state and name on the left, what it came to on the
// right.
func (m Model) jobRow(c gh.Check, width int) string {
	icon, ic := comp.CheckStateIcon(m.theme, c.State)
	lead := " " + lipgloss.NewStyle().Foreground(ic).Render(icon) + " " +
		lipgloss.NewStyle().Foreground(m.theme.Text).Render(c.Name)

	label, lc := comp.CheckStateLabel(m.theme, c.State)
	word := lipgloss.NewStyle().Foreground(lc).Render(label)

	return clipTo(m.checkLine(lead, word, width, lipgloss.NewStyle()), width, m.faint())
}
