package prview_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

func checkRollup() gh.CheckRollup {
	ago := time.Now().Add(-2 * time.Minute)
	return gh.CheckRollup{
		State: gh.CheckStateFailure,
		Checks: []gh.Check{
			{Name: "unit", Workflow: "CI", State: gh.CheckStateSuccess, JobID: 101, StartedAt: ago, CompletedAt: ago.Add(20 * time.Second), Duration: 20 * time.Second},
			{Name: "lint", Workflow: "Build", State: gh.CheckStateSuccess, JobID: 102},
			{Name: "test", Workflow: "Build", State: gh.CheckStateFailure, JobID: 103},
			{Name: "codecov", State: gh.CheckStateSkipped},
		},
	}
}

func onChecks(width, height int) prview.Model {
	return overRollup(checkRollup(), width, height)
}

func overRollup(r gh.CheckRollup, width, height int) prview.Model {
	d := sampleDetail()
	d.Rollup = r
	return press(detailed(held(d), width, height), "]", "]")
}

func loadedJob(id int64, failed bool) store.Job {
	at := time.Date(2026, 8, 19, 14, 0, 0, 0, time.UTC)
	state := gh.CheckStateSuccess
	line := "ok"
	if failed {
		state, line = gh.CheckStateFailure, "##[error]tests failed"
	}
	job := gh.Job{
		ID: id, Name: "test", State: state, StartedAt: at, CompletedAt: at.Add(8 * time.Second), Duration: 8 * time.Second,
		Steps: []gh.JobStep{
			{Number: 1, Name: "Set up job", State: gh.CheckStateSuccess, StartedAt: at, CompletedAt: at.Add(2 * time.Second), Duration: 2 * time.Second},
			{Number: 2, Name: "Run tests", State: state, StartedAt: at.Add(2 * time.Second), CompletedAt: at.Add(8 * time.Second), Duration: 6 * time.Second},
		},
	}
	log := "2026-08-19T14:00:00Z ##[group]Set up job\n" +
		"2026-08-19T14:00:01Z runner ready\n" +
		"2026-08-19T14:00:02Z ##[endgroup]\n" +
		"2026-08-19T14:00:02Z ##[group]Run tests\n" +
		"2026-08-19T14:00:03Z " + line + "\n" +
		"2026-08-19T14:00:08Z ##[endgroup]\n"
	return store.Job{Job: job, Log: log, Status: store.StatusReady, Loaded: true}
}

func filledCheckRows(m prview.Model) []string {
	var out []string
	for _, row := range columnLines(m.View()) {
		if strings.TrimSpace(row) != "" {
			out = append(out, row)
		}
	}
	return out
}

func TestTheChecksTreeFlattensSingleJobsAndNestsMultiJobWorkflows(t *testing.T) {
	rows := filledCheckRows(onChecks(160, 24))
	if len(rows) != 5 {
		t.Fatalf("rows = %d, want one single job, one parent, two children and one status: %q", len(rows), rows)
	}
	for i, want := range []string{"CI / unit", "Build", "lint", "test", "codecov"} {
		if !strings.Contains(rows[i], want) {
			t.Errorf("row %d = %q, want %q", i, rows[i], want)
		}
	}
	if !strings.Contains(rows[1], "▾") || !strings.Contains(rows[1], "2") {
		t.Errorf("multi-job parent = %q, want an open fold and count", rows[1])
	}
	if !strings.HasPrefix(strings.TrimLeft(rows[2], " "), "●") || !strings.HasPrefix(rows[2], "  ") {
		t.Errorf("child = %q, want an indented state-bearing row", rows[2])
	}
}

func TestTheRightPaneShowsOnlyTheSelectedJob(t *testing.T) {
	out := stripANSI(onChecks(160, 24).View())
	if !strings.Contains(out, "CI / unit") || !strings.Contains(out, "Loading the job log") {
		t.Errorf("selected job is not in the pane:\n%s", out)
	}
	if strings.Count(out, "Build / lint") != 0 || strings.Contains(out, "1 failing") {
		t.Error("the right pane still contains the other workflow cards")
	}
}

func TestMovingAcrossAParentKeepsTheSelectedJobThenAChildReplacesIt(t *testing.T) {
	m := onChecks(160, 24)
	m = press(m, "j")
	if out := stripANSI(m.View()); !strings.Contains(out, "CI / unit") {
		t.Error("a workflow parent replaced the selected job")
	}
	m = press(m, "j")
	out := stripANSI(m.View())
	if !strings.Contains(out, "✓ Build / lint") {
		t.Errorf("the child did not replace the pane:\n%s", out)
	}
}

func TestSpaceFoldsAndExpandsAMultiJobParent(t *testing.T) {
	m := press(onChecks(160, 24), "j", "space")
	rows := filledCheckRows(m)
	if len(rows) != 3 || !strings.Contains(rows[1], "▸") {
		t.Fatalf("folded rows = %q, want the two children hidden", rows)
	}
	m = press(m, "space")
	if rows := filledCheckRows(m); len(rows) != 5 || !strings.Contains(rows[1], "▾") {
		t.Errorf("expanded rows = %q, want the children restored", rows)
	}
}

func TestFoldAndSelectionSurviveAPoll(t *testing.T) {
	m := press(onChecks(160, 24), "j", "space", "j") // folded Build, then codecov
	next := checkRollup()
	next.Checks = append([]gh.Check{{Name: "docs", Workflow: "Docs", State: gh.CheckStateSuccess, JobID: 99}}, next.Checks...)
	m.SetDetail(held(func() gh.PullRequestDetail {
		d := sampleDetail()
		d.Rollup = next
		return d
	}()))

	out := stripANSI(m.View())
	if !strings.Contains(out, "No job log is available for this status check") {
		t.Error("the selected status context did not survive the poll")
	}
	rows := filledCheckRows(m)
	for _, row := range rows {
		if strings.Contains(row, "lint") || strings.Contains(row, "test") {
			t.Errorf("the poll reopened the folded workflow: %q", rows)
		}
	}
}

func TestARerunKeepsLogicalSelectionButLoadsTheNewAttempt(t *testing.T) {
	m := press(onChecks(160, 24), "j", "j", "j") // Build / test
	m.SetJob(103, loadedJob(103, true))

	next := checkRollup()
	next.Checks[2].JobID = 203
	next.Checks[2].State = gh.CheckStatePending
	d := sampleDetail()
	d.Rollup = next
	cmd := m.SetDetail(held(d))
	if cmd == nil {
		t.Fatal("the rerun did not ask for its new concrete job")
	}
	out := stripANSI(m.View())
	if !strings.Contains(out, "Build / test") || strings.Contains(out, "tests failed") {
		t.Error("the old attempt's log remained under the rerun")
	}
}

func TestAFinishedJobReplacesItsRunningMetadataAndAsksForTheLog(t *testing.T) {
	r := checkRollup()
	r.Checks[0].State = gh.CheckStatePending
	m := overRollup(r, 160, 24)
	m.SetJob(101, store.Job{
		Job: gh.Job{ID: 101, Name: "unit", State: gh.CheckStatePending,
			Steps: []gh.JobStep{{Number: 1, Name: "Run tests", State: gh.CheckStatePending}}},
		Status: store.StatusReady, Loaded: true,
	})

	r.Checks[0].State = gh.CheckStateSuccess
	d := sampleDetail()
	d.Rollup = r
	if cmd := m.SetDetail(held(d)); cmd == nil {
		t.Fatal("the completed job did not ask for its now-available log")
	}
	if out := stripANSI(m.View()); strings.Contains(out, "Log output will be available") {
		t.Error("the running-job note remained after completion")
	}
}

func TestAStatusContextHasAnExplicitNoLogState(t *testing.T) {
	m := press(onChecks(160, 24), "G")
	out := stripANSI(m.View())
	if !strings.Contains(out, "codecov") || !strings.Contains(out, "No job log is available for this status check") {
		t.Errorf("status context pane:\n%s", out)
	}
}

func TestJobSummaryCarriesStateTimingAndDuration(t *testing.T) {
	m := onChecks(160, 24)
	m.SetJob(101, loadedJob(101, false))
	out := stripANSI(m.View())
	for _, want := range []string{"CI / unit", "passing", "8s", "Set up job", "Run tests"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary is missing %q:\n%s", want, out)
		}
	}
}

func TestFailedStepsStartOpenAndPassingStepsStartClosed(t *testing.T) {
	m := press(onChecks(160, 24), "j", "j", "j")
	m.SetJob(103, loadedJob(103, true))
	out := stripANSI(m.View())
	if !strings.Contains(out, "▸ ✓ Set up job") || !strings.Contains(out, "▾ ✗ Run tests") {
		t.Errorf("step folds are not pass-closed and failure-open:\n%s", out)
	}
	if !strings.Contains(out, "tests failed") || strings.Contains(out, "runner ready") {
		t.Error("the wrong step output is expanded")
	}
}

func TestGitHubLogAnnotationsUseTheThemeWhenTheToolSentNoColor(t *testing.T) {
	m := press(onChecks(160, 24), "j", "j", "j")
	m.SetJob(103, loadedJob(103, true))
	for _, line := range strings.Split(m.View(), "\n") {
		if strings.Contains(line, "tests failed") {
			if !strings.Contains(line, fgSeq(theme.RosePineMoon.Error)) {
				t.Errorf("error line has no error color: %q", line)
			}
			return
		}
	}
	t.Fatal("the error annotation is not on screen")
}

func TestSpaceTogglesTheStepUnderTheMainPaneCursor(t *testing.T) {
	m := press(onChecks(160, 24), "j", "j", "j")
	m.SetJob(103, loadedJob(103, true))
	m = press(m, "2", "j", "space")
	if out := stripANSI(m.View()); strings.Contains(out, "tests failed") {
		t.Error("space did not close the failed step")
	}
	m = press(m, "space")
	if out := stripANSI(m.View()); !strings.Contains(out, "tests failed") {
		t.Error("space did not reopen the failed step")
	}
}

func TestDownloadedLogControlSequencesCannotReachTheTerminal(t *testing.T) {
	m := onChecks(160, 24)
	job := loadedJob(101, false)
	job.Job.Steps[0].State = gh.CheckStateFailure
	job.Log = "2026-08-19T14:00:00Z ##[group]Set up job\n2026-08-19T14:00:01Z \x1b[2Jdanger\r\n"
	m.SetJob(101, job)
	out := m.View()
	if strings.Contains(out, "\x1b[2J") || !strings.Contains(stripANSI(out), "danger") {
		t.Error("the log either kept its control sequence or lost its text")
	}
}

func TestSlashSearchHighlightsInPlaceAndOpensAMatchingStep(t *testing.T) {
	m := onChecks(160, 24)
	m.SetJob(101, loadedJob(101, false))
	m = press(m, "/", "r", "u", "n", "n", "e", "r")
	out := stripANSI(m.View())
	if !strings.Contains(out, "Search: runner") || !strings.Contains(out, "runner ready") {
		t.Errorf("search did not expose its matching line:\n%s", out)
	}
	if !strings.Contains(out, "▾ ✓ Set up job") {
		t.Error("search did not temporarily open the matching step")
	}
}

func TestSearchOwnsPrintableKeysUntilItCloses(t *testing.T) {
	m := onChecks(160, 24)
	m.SetJob(101, loadedJob(101, false))
	m = press(m, "/", "j")
	if out := stripANSI(m.View()); !strings.Contains(out, "Search: j") || !strings.Contains(out, "Set up job") {
		t.Error("j moved the step cursor instead of entering the query")
	}
	m = press(m, "enter")
	if out := stripANSI(m.View()); strings.Contains(out, "Search: j▏") {
		t.Error("enter left the query in editing mode")
	}
}

func TestFirstFailureJumpsToItsStep(t *testing.T) {
	m := press(onChecks(160, 14), "j", "j", "j")
	m.SetJob(103, loadedJob(103, true))
	m = press(m, "2", "f")
	out := stripANSI(m.View())
	if !strings.Contains(out, "Run tests") || !strings.Contains(out, "tests failed") {
		t.Errorf("failure jump did not show the failed step:\n%s", out)
	}
	m = press(m, "space")
	if out := stripANSI(m.View()); strings.Contains(out, "tests failed") {
		t.Error("the failure jump did not move the step cursor")
	}
}

func TestAJobFailureRendersItsReason(t *testing.T) {
	m := onChecks(160, 24)
	m.SetJob(101, store.Job{Status: store.StatusFailed, Err: errors.New("no such host")})
	if out := stripANSI(m.View()); !strings.Contains(out, "Could not load the job log: no such host") {
		t.Errorf("failure pane:\n%s", out)
	}
}

func TestTheChecksTreeStaysAtEveryDrawableWidth(t *testing.T) {
	for _, width := range []int{69, 60, 56} {
		m := onChecks(width, 24)
		column := strings.Join(filledCheckRows(m), "\n")
		if !strings.Contains(column, "CI / unit") || !strings.Contains(column, "Build") {
			t.Errorf("at %d columns the job tree disappeared:\n%s", width, stripANSI(m.View()))
		}
	}
}

func TestChecksTreeRowsClipWithoutWrapping(t *testing.T) {
	r := checkRollup()
	r.Checks[0].Workflow = strings.Repeat("integration-suite-", 10)
	rows := filledCheckRows(overRollup(r, 56, 24))
	if len(rows) != 5 {
		t.Errorf("long row wrapped the tree to %d rows: %q", len(rows), rows)
	}
}

func TestTheFrameFillsItsSizeExactlyOnTheChecksTab(t *testing.T) {
	for _, size := range []struct{ width, height int }{{200, 40}, {160, 24}, {70, 23}, {56, 23}} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			lines := strings.Split(onChecks(size.width, size.height).View(), "\n")
			if len(lines) != size.height {
				t.Fatalf("height = %d, want %d", len(lines), size.height)
			}
			for i, line := range lines {
				if got := lipgloss.Width(line); got != size.width {
					t.Errorf("line %d width = %d, want %d", i, got, size.width)
				}
			}
		})
	}
}
