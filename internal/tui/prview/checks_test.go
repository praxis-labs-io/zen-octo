package prview_test

import (
	"errors"
	"fmt"
	"image/color"
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/store"
	"github.com/zen-octo/zen-octo/internal/tui/prview"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// onChecks is the screen with a detail loaded, sitting on the Checks tab.
func onChecks(width, height int) prview.Model {
	return press(detailed(held(sampleDetail()), width, height), "]", "]")
}

// overRollup is the Checks tab over a rollup of its own.
func overRollup(r gh.CheckRollup, width, height int) prview.Model {
	d := sampleDetail()
	d.Rollup = r
	return press(detailed(held(d), width, height), "]", "]")
}

// oneWorkflow is a rollup of a single workflow whose jobs came to the given
// states, which is what the column has to fold into one marker.
func oneWorkflow(states ...gh.CheckState) gh.CheckRollup {
	out := gh.CheckRollup{State: gh.CheckStateFailure}
	for i, s := range states {
		out.Checks = append(out.Checks, gh.Check{
			Name: "job" + strconv.Itoa(i), Workflow: "CI", State: s,
		})
	}
	return out
}

// filled is the column's rows with the padding under them dropped.
func filled(lines []string) []string {
	var out []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

func TestTheChecksColumnGroupsByWorkflow(t *testing.T) {
	m := onChecks(160, 24)
	column := strings.Join(columnLines(m.View()), "\n")

	for _, want := range []string{"Rails Unit Tests", "Rails Lint", "E2E Tests"} {
		if !strings.Contains(column, want) {
			t.Errorf("the column is missing the workflow %q", want)
		}
	}

	// Two of the workflows have a job called test. The column names workflows,
	// so a job name in it would be one of two rows that read the same.
	if strings.Contains(column, "e2e") || strings.Contains(column, "codecov") {
		t.Errorf("the column names jobs as well as workflows:\n%s", column)
	}

	if !strings.Contains(stripANSI(m.View()), "4 checks") {
		t.Error("the column is not titled with its job count")
	}
}

// The status contexts are what something outside the repository posted. They
// read as a footnote to the workflows this repository runs, so they go last
// however they arrive.
func TestChecksWithNoWorkflowGroupTogetherAtTheEnd(t *testing.T) {
	r := sampleDetail().Rollup
	r.Checks = append([]gh.Check{{Name: "codecov", State: gh.CheckStateSkipped}},
		r.Checks[:3]...)

	rows := filled(columnLines(overRollup(r, 160, 24).View()))
	if len(rows) != 4 {
		t.Fatalf("the column has %d rows, want one per workflow and one for the rest", len(rows))
	}
	if !strings.Contains(rows[3], "Status checks") {
		t.Errorf("the last row is %q, want the workflow-less checks under it", rows[3])
	}
	for i, row := range rows[:3] {
		if strings.Contains(row, "Status checks") {
			t.Errorf("row %d is %q, want the bucket at the end", i, row)
		}
	}
}

// The marker is the one cell the column spends on where a workflow got to, so
// it takes the state of whichever job in it most wants looking at.
func TestTheWorkflowMarkerTakesTheWorstStateInIt(t *testing.T) {
	th := theme.RosePineMoon
	states := map[string]color.Color{
		"passing": th.Success,
		"failing": th.Error,
		"running": th.Warning,
		"skipped": th.Subtle,
	}

	tests := []struct {
		name string
		jobs []gh.CheckState
		want string
	}{
		{"all passing", []gh.CheckState{gh.CheckStateSuccess, gh.CheckStateSuccess}, "passing"},
		{"one failing", []gh.CheckState{gh.CheckStateSuccess, gh.CheckStateFailure}, "failing"},
		{"one still running", []gh.CheckState{gh.CheckStateSuccess, gh.CheckStatePending}, "running"},
		{"all skipped", []gh.CheckState{gh.CheckStateSkipped, gh.CheckStateSkipped}, "skipped"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := overRollup(oneWorkflow(tt.jobs...), 160, 24).View()

			if !marked(out, fgSeq(states[tt.want])) {
				t.Errorf("no %s marker on screen", tt.want)
			}
			for name, c := range states {
				if name == tt.want {
					continue
				}
				if marked(out, fgSeq(c)) {
					t.Errorf("the workflow reads as %s, want %s", name, tt.want)
				}
			}
		})
	}
}

// The pane holds every workflow and opens on the one under the cursor. The
// tally lines are the tell: they belong to the pane, where the column carries
// nothing but names.
func TestThePaneOpensOnTheSelectedWorkflow(t *testing.T) {
	out := stripANSI(onChecks(160, 14).View())

	for _, want := range []string{"Rails Unit Tests", "1 passing", "test"} {
		if !strings.Contains(out, want) {
			t.Errorf("the pane is missing %q", want)
		}
	}
	if strings.Contains(out, "1 running") {
		t.Error("the pane opened below the workflow the cursor is on")
	}
}

// The jobs came with the detail, so the cursor scrolls the pane to them rather
// than a keypress asking for them.
func TestMovingTheChecksCursorScrollsThePaneToThatWorkflow(t *testing.T) {
	m := onChecks(160, 14)

	before := stripANSI(m.View())
	if !strings.Contains(before, "1 passing") || strings.Contains(before, "1 running") {
		t.Fatal("setup: the pane did not open on the first workflow")
	}

	after := stripANSI(press(m, "j").View())
	if !strings.Contains(after, "1 failing") || !strings.Contains(after, "1 running") {
		t.Error("j did not scroll the pane on to the next workflow")
	}
	if strings.Contains(after, "1 passing") {
		t.Error("the pane opened above the workflow the cursor moved to")
	}
}

// Picking a workflow opens the pane on its heading. The pane keeps whatever
// offset it was left at, and a workflow selected from deep inside the card
// above it would otherwise open partway down its own jobs.
func TestSelectingAWorkflowOpensOnItsHeading(t *testing.T) {
	var r gh.CheckRollup
	for w := range 3 {
		for j := range 40 {
			r.Checks = append(r.Checks, gh.Check{
				Name:     fmt.Sprintf("wf%d-job%d", w, j),
				Workflow: "Workflow " + strconv.Itoa(w),
				State:    gh.CheckStateSuccess,
			})
		}
	}

	// Into the pane, deep into the first card, back to the column, on one.
	m := press(overRollup(r, 160, 24), "2")
	for range 30 {
		m = press(m, "j")
	}
	m = press(m, "1", "j")

	if out := stripANSI(m.View()); !strings.Contains(out, "wf1-job0") {
		t.Error("the pane opened partway down the workflow it was pointed at")
	}
}

// Every key a reader presses on this tab is picking a workflow, so the column
// takes focus rather than making them ask for it first.
//
// The tab is reached backwards through Files, where focus sits on the pane. The
// forward route comes through the commit column, which hands its own focus over
// and would answer for this one.
func TestTheChecksTabOpensWithTheColumnFocused(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 160, 24), "[", "[")

	// j moves the cursor when the column has focus, and scrolls the pane beside
	// it when it does not.
	if out := stripANSI(press(m, "j").View()); !strings.Contains(out, "1 failing") {
		t.Error("j never reached the column: the tab opened with focus elsewhere")
	}
}

// G and g go to the ends of the column, which is a count of its workflows
// rather than of the lines they take.
func TestTheEndsOfTheChecksColumnAreItsFirstAndLastWorkflow(t *testing.T) {
	m := overRollup(manyWorkflows(40), 160, 24)

	if out := stripANSI(press(m, "G").View()); !strings.Contains(out, "Workflow number 39") {
		t.Error("G did not reach the last workflow")
	}
	if out := stripANSI(press(m, "G", "g").View()); !strings.Contains(out, "Workflow number 0 ") {
		t.Error("g did not come back to the first workflow")
	}
}

func TestTheSelectedWorkflowIsPaintedCellByCell(t *testing.T) {
	seq := bgSeq(theme.RosePineMoon.SelectedBackground)

	var painted []string
	for _, line := range strings.Split(onChecks(160, 24).View(), "\n") {
		if strings.Contains(line, seq) {
			painted = append(painted, line)
		}
	}

	if len(painted) != 1 {
		t.Fatalf("%d lines carry the selection, want the one of a row", len(painted))
	}
	if count := strings.Count(painted[0], seq); count < 2 {
		t.Errorf("the row paints the selection %d times, want it cell by cell", count)
	}
}

func TestTheChecksStatesReadAsThemselves(t *testing.T) {
	empty := sampleDetail()
	empty.Rollup = gh.CheckRollup{}

	tests := []struct {
		name string
		held store.Detail
		want []string
	}{
		{
			name: "nothing yet",
			held: store.Detail{Status: store.StatusLoading},
			want: []string{"Loading the checks"},
		},
		{
			name: "never loaded and failed",
			held: store.Detail{Status: store.StatusFailed, Err: errors.New("no such host")},
			want: []string{"Could not load the checks: no such host"},
		},
		{
			name: "loaded with nothing reported",
			held: held(empty),
			want: []string{"No checks.", "No checks have reported."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := stripANSI(press(detailed(tt.held, 200, 40), "]", "]").View())
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Errorf("the tab does not carry %q", want)
				}
			}
		})
	}
}

// One viewport serves all three columns, and the offset left in it belongs to
// another column and another column's cursor. Coming back to a scrolled checks
// column has to put its own cursor back inside the window.
func TestSwitchingBackToChecksOpensOnTheWorkflowUnderTheCursor(t *testing.T) {
	for _, height := range []int{12, 13, 14, 15} {
		t.Run(strconv.Itoa(height), func(t *testing.T) {
			d := sampleDetail()
			d.Rollup = manyWorkflows(40)
			d.Commits = manyCommits(40)

			m := press(detailed(held(d), 160, height), "]", "]")
			for range 30 {
				m = press(m, "j")
			}

			// Away to the commit column, down that, then round to Checks.
			m = press(m, "]", "]", "]")
			for range 9 {
				m = press(m, "j")
			}
			m = press(m, "]")

			column := strings.Join(filled(columnLines(m.View())), "\n")
			if !strings.Contains(column, "Workflow number 30") {
				t.Errorf("the column came back without its cursor on screen:\n%s", column)
			}
		})
	}
}

// A page moves the window by what it holds. Anything more and the workflows in
// between never appear on screen at all.
func TestPagingTheChecksColumnMovesAWholeWindow(t *testing.T) {
	m := overRollup(manyWorkflows(40), 160, 24)

	before := filled(columnLines(m.View()))
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	after := filled(columnLines(m.View()))

	if len(before) == 0 || len(after) == 0 {
		t.Fatalf("the column showed %d rows then %d", len(before), len(after))
	}
	if strings.TrimSpace(before[0]) == strings.TrimSpace(after[0]) {
		t.Fatal("page down did not move the column")
	}
	if indexOf(before, after[0]) < 0 {
		t.Errorf("the window jumped from %q to %q, skipping every workflow between",
			before[len(before)-1], after[0])
	}
}

// manyWorkflows is a rollup long enough to scroll, each row telling itself
// apart from the rest.
func manyWorkflows(n int) gh.CheckRollup {
	out := gh.CheckRollup{State: gh.CheckStateSuccess}
	for i := range n {
		out.Checks = append(out.Checks, gh.Check{
			Name:     "build",
			Workflow: "Workflow number " + strconv.Itoa(i),
			State:    gh.CheckStateSuccess,
		})
	}
	return out
}

// A column is a column: a name wrapped to whatever it needs turns one row into
// two that read as two workflows. The name is what gives way rather than the
// count beside it, since a clipped count reads as a real one.
func TestALongWorkflowNameClipsRatherThanWrapping(t *testing.T) {
	r := oneWorkflow(slices.Repeat([]gh.CheckState{gh.CheckStateSuccess}, 7)...)
	for i := range r.Checks {
		r.Checks[i].Workflow = strings.Repeat("Deploy to production ", 8)
	}

	m := overRollup(r, 160, 24)

	rows := filled(columnLines(m.View()))
	if len(rows) != 1 {
		t.Fatalf("the column has %d rows for one workflow, want it clipped to one", len(rows))
	}
	if !strings.Contains(rows[0], "…") {
		t.Errorf("the row is %q, want a mark saying the name was cut", rows[0])
	}
	if !strings.HasSuffix(rows[0], "7") {
		t.Errorf("the row is %q, want its job count kept on the end", rows[0])
	}

	// The card heading beside it gives way the same way.
	if !strings.Contains(stripANSI(m.View()), "7 passing") {
		t.Error("the card heading lost its tally to the name beside it")
	}
}

// A job name that runs past the card clips rather than pushing the state word
// off the end of the row. The word is what the row is for.
func TestALongJobNameClipsInsideItsCard(t *testing.T) {
	r := oneWorkflow(gh.CheckStateSuccess)
	r.Checks[0].Name = strings.Repeat("integration-suite-", 12)

	var rows []string
	for _, line := range strings.Split(stripANSI(overRollup(r, 160, 24).View()), "\n") {
		if strings.Contains(line, "integration-suite-") {
			rows = append(rows, line)
		}
	}

	if len(rows) != 1 {
		t.Fatalf("the job name is on %d lines, want the one row it belongs to", len(rows))
	}
	if !strings.Contains(rows[0], "…") {
		t.Errorf("the row is %q, want a mark saying the name was cut", rows[0])
	}
	if !strings.Contains(rows[0], "passing") {
		t.Errorf("the row is %q, want its state word kept on the end", rows[0])
	}
}

// The rail belongs to the conversation. This tab already spends that side of
// the frame on a column, and the rail's own checks section would say what the
// pane beside it is saying at length.
func TestTheRailIsOffOnTheChecksTab(t *testing.T) {
	for _, width := range []int{200, 160, 120} {
		if strings.Contains(stripANSI(onChecks(width, 24).View()), "Reviewers") {
			t.Errorf("the rail is on screen at %d columns", width)
		}
		if strings.Contains(stripANSI(press(onChecks(width, 24), "d").View()), "Reviewers") {
			t.Errorf("d brought the rail back at %d columns", width)
		}
	}
}

// Below the width the column needs there is nothing left to move it with, so
// every workflow's jobs render in the pane instead.
func TestTheChecksColumnHidesOnANarrowFrame(t *testing.T) {
	for _, width := range []int{160, 100, 70} {
		column := strings.Join(filled(columnLines(onChecks(width, 24).View())), "\n")
		if !strings.Contains(column, "E2E Tests") {
			t.Errorf("the column is gone at %d columns", width)
		}
	}

	for _, width := range []int{69, 60, 40} {
		m := onChecks(width, 40)
		if column := strings.Join(columnLines(m.View()), "\n"); strings.Contains(column, "E2E Tests") {
			t.Errorf("the column is still on screen at %d columns", width)
		}

		out := stripANSI(m.View())
		if !strings.Contains(out, "e2e") || !strings.Contains(out, "codecov") {
			t.Errorf("at %d columns the pane does not carry every workflow's jobs", width)
		}
	}
}

// A refetch reorders nothing the reader asked for. The cursor holds its
// workflow by name, so a run that starts while the tab is open appends below it
// rather than walking it onto something else.
func TestTheChecksCursorHoldsItsWorkflowAcrossARefresh(t *testing.T) {
	m := press(onChecks(160, 24), "j")
	if !strings.Contains(stripANSI(m.View()), "failing") {
		t.Fatal("setup: the cursor never reached the failing workflow")
	}

	d := sampleDetail()
	d.Rollup.Checks = append(
		[]gh.Check{{Name: "setup", Workflow: "Nightly", State: gh.CheckStatePending}},
		d.Rollup.Checks...)
	m.SetDetail(held(d))

	out := stripANSI(m.View())
	if !strings.Contains(out, "Rails Lint") || !strings.Contains(out, "failing") {
		t.Errorf("the refetch moved the cursor off the workflow it was on:\n%s", out)
	}
}

// Holding the workflow moves the cursor's index, and a window left where it was
// is then a column with nothing marked in it, disagreeing with the pane.
func TestARefreshThatAddsWorkflowsAboveTheCursorKeepsItInTheWindow(t *testing.T) {
	d := sampleDetail()
	d.Rollup = manyWorkflows(40)

	m := press(detailed(held(d), 160, 24), "]", "]")
	for range 30 {
		m = press(m, "j")
	}

	next := sampleDetail()
	next.Rollup.Checks = append([]gh.Check{
		{Name: "build", Workflow: "Nightly A", State: gh.CheckStatePending},
		{Name: "build", Workflow: "Nightly B", State: gh.CheckStatePending},
		{Name: "build", Workflow: "Nightly C", State: gh.CheckStatePending},
		{Name: "build", Workflow: "Nightly D", State: gh.CheckStatePending},
		{Name: "build", Workflow: "Nightly E", State: gh.CheckStatePending},
	}, d.Rollup.Checks...)
	m.SetDetail(held(next))

	column := strings.Join(filled(columnLines(m.View())), "\n")
	if !strings.Contains(column, "Workflow number 30") {
		t.Errorf("the refresh scrolled the selection out of the column:\n%s", column)
	}
	if !strings.Contains(m.View(), bgSeq(theme.RosePineMoon.SelectedBackground)) {
		t.Error("no row in the column is marked as selected")
	}
}

// A refresh can land while the frame is too short for the column to hold a row
// at all. Without a floor under the window height the cursor is pushed one row
// past itself, and that offset survives the resize that makes it readable.
func TestARefreshOnAFrameTooShortForTheColumnSurvivesAResize(t *testing.T) {
	d := sampleDetail()
	d.Rollup = manyWorkflows(40)
	m := press(detailed(held(d), 160, 24), "]", "]")

	m.SetSize(160, 2)
	m.SetDetail(held(d))
	m.SetSize(160, 24)

	rows := filled(columnLines(m.View()))
	if len(rows) == 0 || !strings.Contains(rows[0], "Workflow number 0") {
		t.Errorf("the column came back opened past its cursor: %q", rows)
	}
}

// A column with nothing in it has no cursor to walk, and taking the movement
// keys anyway leaves the pane beside it unscrollable. The tab opens on the
// column, and a detail that failed puts its error in the pane.
func TestAnEmptyChecksColumnLeavesTheKeysToThePane(t *testing.T) {
	failed := store.Detail{
		Status: store.StatusFailed,
		Err:    errors.New(strings.Repeat("dial tcp 140.82.113.6:443: i/o timeout. ", 80)),
	}
	m := press(detailed(failed, 160, 10), "]", "]")

	before := stripANSI(m.View())
	if after := stripANSI(press(m, "j", "j", "j").View()); before == after {
		t.Error("the empty column swallowed the keys meant for the pane")
	}
}

// The pane holds every workflow at both widths, so crossing the column's floor
// only takes the column away. Rendering one card beside the column and all of
// them without it moved the reader mid-resize with no keypress.
func TestWideningPastTheColumnFloorKeepsThePaneWhereItWas(t *testing.T) {
	m := press(overRollup(manyWorkflows(12), 60, 20), "G")
	if !strings.Contains(stripANSI(m.View()), "Workflow number 11") {
		t.Fatal("setup: G did not reach the last workflow")
	}

	m.SetSize(160, 20)
	if !strings.Contains(stripANSI(m.View()), "Workflow number 11") {
		t.Error("widening the frame moved the pane off what it was showing")
	}
}

// A job name clips only when the pane has no room for it. The prose measure the
// conversation is set to clipped one while a hundred columns sat empty beside.
func TestALongJobNameFitsWhenThePaneHasRoom(t *testing.T) {
	r := oneWorkflow(gh.CheckStateSuccess)
	r.Checks[0].Name = strings.Repeat("integration-", 8)

	if out := stripANSI(overRollup(r, 200, 24).View()); !strings.Contains(out, r.Checks[0].Name) {
		t.Error("the job name clipped with room to spare beside it")
	}
}

// The bucket is keyed on the workflow behind a check rather than on its own
// label, so a repository that runs a workflow by that name keeps its group.
func TestAWorkflowNamedForTheBucketKeepsItsOwnGroup(t *testing.T) {
	r := gh.CheckRollup{Checks: []gh.Check{
		{Name: "unit", Workflow: "Status checks", State: gh.CheckStateFailure},
		{Name: "lint", Workflow: "CI", State: gh.CheckStateSuccess},
		{Name: "codecov", State: gh.CheckStateSkipped},
	}}

	rows := filled(columnLines(overRollup(r, 160, 24).View()))
	if len(rows) != 3 {
		t.Fatalf("the column has %d rows, want the two workflows and the bucket", len(rows))
	}
	if !strings.Contains(rows[0], "Status checks") || !strings.HasSuffix(rows[0], "1") {
		t.Errorf("the first row is %q, want the workflow that reported first and its one job", rows[0])
	}
	if !strings.Contains(rows[2], "Status checks") || !strings.HasSuffix(rows[2], "1") {
		t.Errorf("the last row is %q, want the status context on its own", rows[2])
	}
}

func TestTheFrameFillsItsSizeExactlyOnTheChecksTab(t *testing.T) {
	sizes := []struct{ width, height int }{
		{width: 200, height: 40},
		{width: 160, height: 24},
		{width: 120, height: 20},
		{width: 70, height: 12},
		{width: 60, height: 10},
		{width: 30, height: 8},
	}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			lines := strings.Split(onChecks(size.width, size.height).View(), "\n")

			if len(lines) != size.height {
				t.Errorf("frame is %d lines, want %d", len(lines), size.height)
			}
			for i, line := range lines {
				if w := lipgloss.Width(line); w != size.width {
					t.Errorf("line %d is %d cells wide, want %d", i, w, size.width)
				}
			}
		})
	}
}
