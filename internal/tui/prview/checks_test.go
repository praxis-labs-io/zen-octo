package prview_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
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

func longLoadedJob(id int64) store.Job {
	job := loadedJob(id, true)
	var log strings.Builder
	log.WriteString("2026-08-19T14:00:00Z ##[group]Set up job\n")
	log.WriteString("2026-08-19T14:00:02Z ##[endgroup]\n")
	log.WriteString("2026-08-19T14:00:02Z ##[group]Run tests\n")
	for i := range 60 {
		fmt.Fprintf(&log, "2026-08-19T14:00:03Z line %02d\n", i)
	}
	log.WriteString("2026-08-19T14:00:08Z ##[endgroup]\n")
	job.Log = log.String()
	return job
}

func settleSearch(m prview.Model, query string) prview.Model {
	m, _ = m.Update(prview.SearchSettleMsg{Query: query})
	return m
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

func TestWalkingChecksFetchesOnlyTheJobWhereTheCursorSettles(t *testing.T) {
	m := onChecks(160, 24)
	m, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"}) // workflow parent
	m, lintWait := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m, testWait := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})

	stale, ok := armed(t, lintWait).(prview.JobSettleMsg)
	if !ok {
		t.Fatalf("first wait = %T", armed(t, lintWait))
	}
	if _, cmd := m.Update(stale); cmd != nil {
		t.Error("a job passed over reached the network")
	}
	settled, ok := armed(t, testWait).(prview.JobSettleMsg)
	if !ok {
		t.Fatalf("last wait = %T", armed(t, testWait))
	}
	_, cmd := m.Update(settled)
	if msg, ok := armed(t, cmd).(prview.NeedJobMsg); !ok || msg.JobID != 103 {
		t.Errorf("settled request = %#v, want job 103", msg)
	}
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

func TestRAsksToRerunTheSelectedFailedJob(t *testing.T) {
	r := checkRollup()
	r.Checks[2].RunID = 555200001
	m := press(overRollup(r, 160, 24), "j", "j", "j")

	var cmd tea.Cmd
	m, cmd = key(m, "r")
	if cmd == nil {
		t.Fatal("r did not ask to rerun the failed job")
	}
	raw := cmd()
	msg, ok := raw.(prview.RerunCheckMsg)
	if !ok {
		t.Fatalf("r sent %T, want a RerunCheckMsg", raw)
	}
	if msg.JobID != 103 || msg.Name != "Build / test" || msg.Repo == "" {
		t.Errorf("rerun = %+v", msg)
	}
	if out := stripANSI(m.View()); !strings.Contains(out, "rerunning") {
		t.Errorf("the selected job did not show the write in flight:\n%s", out)
	}
	if _, again := key(m, "r"); again != nil {
		t.Error("a second r started another rerun while the first was in flight")
	}
	m = press(m, "j", "k")
	if _, again := key(m, "r"); again != nil {
		t.Error("navigating away released the pending rerun")
	}

	m.RerunSettled(103)
	if out := stripANSI(m.View()); strings.Contains(out, "rerunning") {
		t.Error("the rerun stayed in flight after it settled")
	}
}

func TestRerunningSurvivesAnOlderAttemptUntilTheNewOneAppears(t *testing.T) {
	r := checkRollup()
	m := press(overRollup(r, 160, 24), "j", "j", "j")
	m, _ = key(m, "r")
	m.RerunAccepted(103, time.Now())

	stale := r
	stale.Checks = slices.Clone(r.Checks)
	stale.Checks[2].JobID = 93
	stale.Checks[2].State = gh.CheckStateSuccess
	stale.Checks[2].StartedAt = time.Now().Add(-time.Minute)
	d := sampleDetail()
	d.Rollup = stale
	m.SetDetail(held(d))
	if out := stripANSI(m.View()); !strings.Contains(out, "rerunning") {
		t.Errorf("an older passing attempt replaced the optimistic rerun:\n%s", out)
	}

	landed := r
	landed.Checks = slices.Clone(r.Checks)
	landed.Checks[2].JobID = 104
	landed.Checks[2].State = gh.CheckStatePending
	d.Rollup = landed
	m.SetDetail(held(d))
	if out := stripANSI(m.View()); strings.Contains(out, "rerunning") || !strings.Contains(out, "Loading the job log") {
		t.Errorf("the new pending attempt did not take over:\n%s", out)
	}
}

func TestTerminalRerunWithoutTimestampsReleasesTheOptimisticState(t *testing.T) {
	r := checkRollup()
	m := press(overRollup(r, 160, 24), "j", "j", "j")
	m, _ = key(m, "r")
	m.RerunAccepted(103, time.Now())

	landed := r
	landed.Checks = slices.Clone(r.Checks)
	landed.Checks[2].JobID = 104
	landed.Checks[2].State = gh.CheckStateSuccess
	d := sampleDetail()
	d.Rollup = landed
	m.SetDetail(held(d))
	if out := stripANSI(m.View()); strings.Contains(out, "rerunning") {
		t.Errorf("timestamp-less replacement stayed optimistic:\n%s", out)
	}
}

func TestRerunIsOfferedOnlyOnARerunnableJob(t *testing.T) {
	r := checkRollup()
	r.Checks[2].RunID = 555200001
	failed := press(overRollup(r, 160, 24), "j", "j", "j")
	found := false
	for _, binding := range failed.ShortHelp() {
		if binding.Help().Desc == "rerun" {
			found = true
		}
	}
	if !found {
		t.Error("the failed job did not offer rerun")
	}
	if msg := asked(t, onChecks(160, 24), "r"); msg != nil {
		t.Errorf("a successful job sent %T on r", msg)
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

func TestCompletedStepsWithoutOutputArePlainRows(t *testing.T) {
	m := onChecks(160, 24)
	job := loadedJob(101, false)
	job.Log = ""
	m.SetJob(101, job)
	m = press(m, "2")
	before := m.View()
	out := stripANSI(before)
	if !strings.Contains(out, "✓ Set up job") || strings.Contains(out, "▸ ✓ Set up job") ||
		strings.Contains(out, "No log output") || strings.Contains(out, "Log output is not available") {
		t.Errorf("logless completed steps were rendered as folds:\n%s", out)
	}
	m = press(m, "space")
	if after := m.View(); after != before {
		t.Error("a completed step without output expanded")
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

func TestSpaceTogglesTheStepHoldingTheMainPaneCursor(t *testing.T) {
	m := press(onChecks(160, 24), "j", "j", "j")
	m.SetJob(103, loadedJob(103, true))
	m = press(m, "2", "}", "j", "space")
	if out := stripANSI(m.View()); strings.Contains(out, "tests failed") {
		t.Error("space did not close the failed step from inside its output")
	}
	m = press(m, "space")
	if out := stripANSI(m.View()); !strings.Contains(out, "tests failed") {
		t.Error("space did not reopen the failed step")
	}
}

func TestExpandedLogLinesCarryStableMutedNumbers(t *testing.T) {
	m := onChecks(160, 24)
	m.SetJob(101, loadedJob(101, false))
	m = press(m, "2", "space", "}", "space")
	frame := m.View()
	for _, line := range strings.Split(frame, "\n") {
		if strings.Contains(stripANSI(line), "runner ready") &&
			!strings.Contains(line, fgSeq(theme.RosePineMoon.MutedOrSubtle())) {
			t.Errorf("line number is not muted: %q", line)
		}
	}
	out := stripANSI(frame)
	for _, want := range []string{"1 │ runner ready", "2 │ ok"} {
		if !strings.Contains(out, want) {
			t.Errorf("numbered log is missing %q:\n%s", want, out)
		}
	}

	m = press(m, "{", "space")
	out = stripANSI(m.View())
	if strings.Contains(out, "runner ready") || !strings.Contains(out, "2 │ ok") {
		t.Errorf("fold changed the remaining line number:\n%s", out)
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

func TestCompletedJobParsingRunsOutsideTheUpdateThatLandsIt(t *testing.T) {
	m := onChecks(160, 24)
	cmd := m.SetJobAsync(101, loadedJob(101, false))
	if out := stripANSI(m.View()); !strings.Contains(out, "Processing the job log") {
		t.Errorf("job did not expose its processing state:\n%s", out)
	}
	m, _ = m.Update(armed(t, cmd))
	if out := stripANSI(m.View()); !strings.Contains(out, "Set up job") || strings.Contains(out, "Processing the job log") {
		t.Errorf("parsed job did not replace the processing state:\n%s", out)
	}
}

func largeLoadedJob(id int64) store.Job {
	job := loadedJob(id, true)
	var log strings.Builder
	log.WriteString("2026-08-19T14:00:00Z ##[group]Set up job\n")
	for range 12000 {
		log.WriteString("2026-08-19T14:00:01Z a deliberately wide retained log line\n")
	}
	log.WriteString("2026-08-19T14:00:02Z ##[endgroup]\n")
	job.Log = log.String()
	return job
}

func TestLargeJobRenderingRunsOutsideTheParsedMessageUpdate(t *testing.T) {
	m := onChecks(160, 24)
	m, render := m.Update(armed(t, m.SetJobAsync(101, largeLoadedJob(101))))
	if out := stripANSI(m.View()); !strings.Contains(out, "Processing the job log") {
		t.Errorf("large log rendered synchronously:\n%s", out)
	}
	if render == nil {
		t.Fatal("large log armed no asynchronous render")
	}
	m, _ = m.Update(armed(t, render))
	if out := stripANSI(m.View()); !strings.Contains(out, "Set up job") || strings.Contains(out, "Processing the job log") {
		t.Errorf("rendered log did not replace processing state:\n%s", out)
	}
}

func TestLargeLogFoldKeepsTheLastCompleteFrameUntilRenderingSettles(t *testing.T) {
	m := onChecks(160, 24)
	m, render := m.Update(armed(t, m.SetJobAsync(101, largeLoadedJob(101))))
	m, _ = m.Update(armed(t, render))
	m = press(m, "2")
	before := m.View()

	m, render = key(m, "space")
	if during := m.View(); during != before || strings.Contains(stripANSI(during), "Processing the job log") {
		t.Errorf("fold replaced the complete frame while rendering:\n%s", stripANSI(during))
	}
	if render == nil {
		t.Fatal("large fold armed no asynchronous render")
	}
	m, _ = m.Update(armed(t, render))
	if after := stripANSI(m.View()); !strings.Contains(after, "a deliberately wide retained log line") {
		t.Error("settled fold did not reveal the log")
	}
}

func TestTheLogCursorBackgroundSurvivesLogColorResets(t *testing.T) {
	m := onChecks(160, 24)
	job := loadedJob(101, true)
	job.Job.Steps = job.Job.Steps[:1]
	job.Job.Steps[0].State = gh.CheckStateFailure
	job.Log = "2026-08-19T14:00:00Z ##[group]Set up job\n" +
		"2026-08-19T14:00:01Z \x1b[31mred\x1b[0m plain\n" +
		"2026-08-19T14:00:02Z ##[endgroup]\n"
	m.SetJob(101, job)
	m = press(m, "2", "j")

	fill := bgSeq(theme.RosePineMoon.SelectedBackground)
	for _, line := range strings.Split(m.View(), "\n") {
		if strings.Contains(stripANSI(line), "red plain") {
			if got := strings.Count(line, fill); got < 3 {
				t.Errorf("cursor background was not restored across SGR resets: %q", line)
			}
			return
		}
	}
	t.Fatal("selected colored log line is not on screen")
}

func TestSlashSearchHighlightsInPlaceAndOpensAMatchingStep(t *testing.T) {
	m := onChecks(160, 24)
	m.SetJob(101, loadedJob(101, false))
	m = press(m, "/", "r", "u", "n", "n", "e", "r")
	m = settleSearch(m, "runner")
	out := stripANSI(m.View())
	if !strings.Contains(out, "Search: runner") || !strings.Contains(out, "runner ready") {
		t.Errorf("search did not expose its matching line:\n%s", out)
	}
	if !strings.Contains(out, "▾ ✓ Set up job") {
		t.Error("search did not temporarily open the matching step")
	}
}

func TestCancelingSearchRestoresTheStepItOpened(t *testing.T) {
	m := onChecks(160, 24)
	m.SetJob(101, loadedJob(101, false))
	m = press(m, "/", "r", "u", "n", "n", "e", "r")
	m = settleSearch(m, "runner")
	m = press(m, "esc", "space")
	if out := stripANSI(m.View()); !strings.Contains(out, "runner ready") {
		t.Errorf("escape moved the cursor off the step search opened:\n%s", out)
	}
}

func TestSearchNeverSlicesAnANSISequence(t *testing.T) {
	m := onChecks(160, 24)
	job := loadedJob(101, false)
	job.Log = "2026-08-19T14:00:00Z ##[group]Set up job\n" +
		"2026-08-19T14:00:01Z \x1b[31m31 errors tail\x1b[0m\n" +
		"2026-08-19T14:00:02Z ##[endgroup]\n"
	m.SetJob(101, job)
	m = press(m, "/", "3", "1")
	m = settleSearch(m, "31")
	if out := stripANSI(m.View()); !strings.Contains(out, "31 errors tail") {
		t.Errorf("search corrupted the ANSI-bearing line:\n%s", out)
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

func TestEscapeCancelsTheLogSearch(t *testing.T) {
	m := onChecks(160, 24)
	m.SetJob(101, loadedJob(101, false))
	m = press(m, "/", "r", "u", "n", "n", "e", "r", "esc")
	if out := stripANSI(m.View()); strings.Contains(out, "Search:") {
		t.Errorf("escape left the search header open:\n%s", out)
	}
}

func TestNextAndPreviousWalkMultiplePinnedSearchResults(t *testing.T) {
	m := onChecks(160, 24)
	m.SetJob(101, longLoadedJob(101))
	m = press(m, "/", "l", "i", "n", "e", "enter")
	if out := stripANSI(m.View()); !strings.Contains(out, "Search: line") || !strings.Contains(out, "1/60") {
		t.Fatalf("search heading did not report its results:\n%s", out)
	}
	if got := logCursorLine(m.View()); !strings.Contains(got, "line 00") {
		t.Fatalf("initial result cursor = %q, want line 00", got)
	}
	m = press(m, "n")
	if got := logCursorLine(m.View()); !strings.Contains(got, "line 01") {
		t.Fatalf("next result cursor = %q, want line 01", got)
	}
	m = press(m, "N")
	if got := logCursorLine(m.View()); !strings.Contains(got, "line 00") {
		t.Fatalf("previous result cursor = %q, want line 00", got)
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

func TestTerminalRollupRejectsAnOvertakenPendingJobResponse(t *testing.T) {
	r := checkRollup()
	r.Checks[0].State = gh.CheckStatePending
	m := overRollup(r, 160, 24)
	m.SetJob(101, store.Job{Status: store.StatusLoading})

	r.Checks[0].State = gh.CheckStateSuccess
	d := sampleDetail()
	d.Rollup = r
	m.SetDetail(held(d))
	pending := loadedJob(101, false)
	pending.Job.State = gh.CheckStatePending
	m.SetJob(101, pending)
	if out := stripANSI(m.View()); strings.Contains(out, "pending") {
		t.Errorf("stale pending response replaced terminal state:\n%s", out)
	}
	msg, ok := armed(t, m.PollJob()).(prview.NeedJobMsg)
	if !ok || !msg.Refresh || msg.JobID != 101 {
		t.Errorf("poll retry = %#v", msg)
	}
}

func TestPendingJobMetadataRefreshesOnTheChecksBeat(t *testing.T) {
	r := checkRollup()
	r.Checks[0].State = gh.CheckStatePending
	m := overRollup(r, 160, 24)
	job := loadedJob(101, false)
	job.Job.State = gh.CheckStatePending
	m.SetJob(101, job)
	msg, ok := armed(t, m.PollJob()).(prview.NeedJobMsg)
	if !ok || !msg.Refresh || msg.JobID != 101 {
		t.Errorf("metadata refresh = %#v", msg)
	}
}

func TestFailedJobRetriesOnPollRatherThanAnUnrelatedKey(t *testing.T) {
	m := onChecks(160, 24)
	m.SetJob(101, store.Job{Status: store.StatusFailed, Err: errors.New("offline")})
	if _, cmd := key(m, "g"); cmd != nil {
		t.Error("an unrelated key armed a failed-job retry")
	}
	msg, ok := armed(t, m.PollJob()).(prview.NeedJobMsg)
	if !ok || msg.JobID != 101 || !msg.Refresh {
		t.Errorf("poll retry = %#v", msg)
	}
}

func TestAJobFailureRendersItsReason(t *testing.T) {
	m := onChecks(160, 24)
	m.SetJob(101, store.Job{Status: store.StatusFailed, Err: errors.New("no such host")})
	if out := stripANSI(m.View()); !strings.Contains(out, "Could not load the job log: no such host") {
		t.Errorf("failure pane:\n%s", out)
	}
}

func logCursorLine(frame string) string {
	fill := bgSeq(theme.RosePineMoon.SelectedBackground)
	for _, line := range strings.Split(frame, "\n") {
		selected := textOnBackground(line, fill)
		for at := 0; at+7 <= len(selected); at++ {
			if strings.HasPrefix(selected[at:], "line ") && selected[at+5] >= '0' && selected[at+5] <= '9' &&
				selected[at+6] >= '0' && selected[at+6] <= '9' {
				return strings.TrimSpace(selected[at:])
			}
		}
	}
	return ""
}

func textOnBackground(line, fill string) string {
	var out strings.Builder
	on := false
	separated := false
	for len(line) > 0 {
		if strings.HasPrefix(line, "\x1b[") {
			end := strings.IndexByte(line, 'm')
			if end < 0 {
				break
			}
			seq := line[:end+1]
			switch {
			case strings.Contains(seq, fill):
				on, separated = true, false
			case seq == "\x1b[m" || seq == "\x1b[0m" || strings.Contains(seq, "[49m"):
				on = false
			}
			line = line[end+1:]
			continue
		}
		if on {
			out.WriteByte(line[0])
			separated = false
		} else if out.Len() > 0 && !separated {
			out.WriteByte(0)
			separated = true
		}
		line = line[1:]
	}
	return out.String()
}

func TestLineAndPageMotionCarryTheLogCursor(t *testing.T) {
	m := onChecks(160, 24)
	m.SetJob(101, longLoadedJob(101))
	m = press(m, "2", "}", "j")
	if got := logCursorLine(m.View()); !strings.Contains(got, "line 00") {
		t.Fatalf("first j cursor = %q, want line 00", got)
	}
	m = press(m, "j")
	before := logCursorLine(m.View())
	if !strings.Contains(before, "line 01") {
		t.Fatalf("second j cursor = %q, want line 01", before)
	}

	m = press(m, "ctrl+d")
	if after := logCursorLine(m.View()); after == before || !strings.Contains(after, "line") {
		t.Fatalf("ctrl+d cursor = %q, want a later log line than %q", after, before)
	}
	m = press(m, "ctrl+u")
	if got := logCursorLine(m.View()); got != before {
		t.Errorf("ctrl+u cursor = %q, want %q", got, before)
	}

	m = press(m, "pgdown")
	if after := logCursorLine(m.View()); after == before || !strings.Contains(after, "line") {
		t.Fatalf("page down cursor = %q, want a later log line than %q", after, before)
	}
	m = press(m, "pgup")
	if got := logCursorLine(m.View()); got != before {
		t.Errorf("page up cursor = %q, want %q", got, before)
	}
}

func TestHalfPageKeysAreInertWhileTheChecksColumnHasFocus(t *testing.T) {
	m := onChecks(160, 24)
	m.SetJob(101, longLoadedJob(101))
	before := m.View()
	m = press(m, "ctrl+d", "ctrl+u")
	if after := m.View(); after != before {
		t.Error("half-page keys moved something while the Checks column had focus")
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
