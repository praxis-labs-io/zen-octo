package prview_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
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
	m, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
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
	if !strings.HasPrefix(strings.TrimLeft(rows[2], " "), "✓") || !strings.HasPrefix(rows[2], "  ") {
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
	m := press(onChecks(160, 24), "j", "space", "j")
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
	m := press(overRollup(bulkRollup(), 160, 24), "j", "j", "j")

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
	failed := press(overRollup(bulkRollup(), 160, 24), "j", "j", "j")
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
	m := press(onChecks(160, 24), "j", "j", "j")
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
			if !strings.Contains(line, fgSeq(testTheme.Error)) {
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
			!strings.Contains(line, fgSeq(testTheme.MutedOrSubtle())) {
			t.Errorf("line number is not muted: %q", line)
		}
	}
	out := stripANSI(frame)
	for _, want := range []string{"  1 runner ready", "  2 ok"} {
		if !strings.Contains(out, want) {
			t.Errorf("numbered log is missing %q:\n%s", want, out)
		}
	}

	m = press(m, "{", "space")
	out = stripANSI(m.View())
	if strings.Contains(out, "runner ready") || !strings.Contains(out, "  2 ok") {
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

	fill := bgSeq(testTheme.SelectedBackground)
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
	fill := bgSeq(testTheme.SelectedBackground)
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

func TestTheChecksSearchReportsACursorAfterItsQuery(t *testing.T) {
	m := onChecks(160, 24)
	m.SetJob(101, loadedJob(101, false))
	m = press(m, "/", "z", "q")

	c := m.Cursor()
	if c == nil {
		t.Fatalf("no cursor while the search is taking text:\n%s", stripANSI(m.View()))
	}

	for row, line := range strings.Split(stripANSI(m.View()), "\n") {
		at := strings.Index(line, "Search: zq")
		if at < 0 {
			continue
		}
		want := lipgloss.Width(line[:at] + "Search: zq")
		if c.X != want || c.Y != row {
			t.Errorf("cursor at (%d,%d), want (%d,%d)", c.X, c.Y, want, row)
		}
		return
	}
	t.Fatalf("the search bar is not on the frame:\n%s", stripANSI(m.View()))
}

func TestTheChecksTabHasNoCursorUntilTheSearchOpens(t *testing.T) {
	m := onChecks(160, 24)
	m.SetJob(101, loadedJob(101, false))

	if c := m.Cursor(); c != nil {
		t.Errorf("cursor at (%d,%d) with nothing taking text", c.X, c.Y)
	}
	if c := press(m, "/", "z").Cursor(); c == nil {
		t.Error("no cursor once the search has the keyboard")
	}
}

func TestFirstFailureIsOfferedOnlyWhereThereIsALogToJumpInto(t *testing.T) {
	r := checkRollup()
	r.Checks[3].State = gh.CheckStateFailure

	tests := []struct {
		name string
		to   []string
		want bool
	}{
		{name: "a failed job", to: []string{"j", "j", "j"}, want: true},
		{name: "a failed status context", to: []string{"j", "j", "j", "j"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := press(overRollup(r, 160, 24), tt.to...)
			found := false
			for _, binding := range m.ShortHelp() {
				if binding.Help().Desc == "first failure" {
					found = true
				}
			}
			if found != tt.want {
				t.Errorf("first failure offered = %v, want %v", found, tt.want)
			}
		})
	}
}

func bulkRollup() gh.CheckRollup {
	r := checkRollup()
	for i := range r.Checks {
		switch r.Checks[i].Workflow {
		case "Build":
			r.Checks[i].RunID = 555200001
		case "CI":
			r.Checks[i].RunID = 555200002
		}
	}
	return r
}

func TestROnAWorkflowRowRerunsOnlyItsFailedJobs(t *testing.T) {
	m := press(overRollup(bulkRollup(), 160, 24), "j")

	var cmd tea.Cmd
	m, cmd = key(m, "r")
	if cmd == nil {
		t.Fatal("r on the workflow row did not ask for a rerun")
	}
	raw := cmd()
	msg, ok := raw.(prview.RerunRunMsg)
	if !ok {
		t.Fatalf("r sent %T, want a RerunRunMsg", raw)
	}
	if msg.All {
		t.Error("r asked for every job, want the failed ones")
	}
	if msg.RunID != 555200001 || msg.Name != "Build" || msg.Repo == "" {
		t.Errorf("rerun = %+v", msg)
	}
	if !slices.Equal(msg.JobIDs, []int64{103}) {
		t.Errorf("JobIDs = %v, want the failed job alone", msg.JobIDs)
	}

	if _, again := key(m, "r"); again != nil {
		t.Error("a second r started another rerun while the first was in flight")
	}

	m.RunRerunSettled(msg.JobIDs)
	if out := stripANSI(m.View()); strings.Contains(out, "rerunning") {
		t.Error("a refused bulk rerun left its marks on the column")
	}
}

func TestROnAWorkflowRowRerunsEveryJobInIt(t *testing.T) {
	_, cmd := key(press(overRollup(bulkRollup(), 160, 24), "j"), "R")
	if cmd == nil {
		t.Fatal("R on the workflow row did not ask for a rerun")
	}
	msg, ok := cmd().(prview.RerunRunMsg)
	if !ok {
		t.Fatal("R did not send a RerunRunMsg")
	}
	if !msg.All {
		t.Error("R asked for the failed jobs, want every one")
	}
	if !slices.Equal(msg.JobIDs, []int64{102, 103}) {
		t.Errorf("JobIDs = %v, want both jobs in the workflow", msg.JobIDs)
	}
}

func TestRIsQuietOnAWorkflowWithNothingFailed(t *testing.T) {
	r := bulkRollup()
	for i := range r.Checks {
		if r.Checks[i].Workflow == "Build" {
			r.Checks[i].State = gh.CheckStateSuccess
		}
	}
	r.State = gh.CheckStateSuccess

	m := press(overRollup(r, 160, 24), "j")
	if _, cmd := key(m, "r"); cmd != nil {
		t.Error("r asked to rerun failed jobs in a workflow where none failed")
	}
	if _, cmd := key(m, "R"); cmd == nil {
		t.Error("R refused a passing workflow, which is still a run to rerun")
	}
}

func TestTheBulkKeysAreDeadOnAJobRow(t *testing.T) {
	m := press(overRollup(bulkRollup(), 160, 24), "j", "j")

	_, cmd := key(m, "R")
	if cmd == nil {
		return
	}
	if _, ok := cmd().(prview.RerunRunMsg); ok {
		t.Error("R on a job row asked to rerun the whole run")
	}
}

func failFirstRollup() gh.CheckRollup {
	return gh.CheckRollup{
		State: gh.CheckStateFailure,
		Checks: []gh.Check{
			{Name: "unit", Workflow: "CI", State: gh.CheckStateSuccess, JobID: 101},
			{Name: "atest", Workflow: "Build", State: gh.CheckStateFailure, JobID: 103, RunID: 555200001},
			{Name: "zlint", Workflow: "Build", State: gh.CheckStateSuccess, JobID: 102, RunID: 555200001},
		},
	}
}

func TestROnAWorkflowRowMeansTheRunEvenWithAFailedJobSelected(t *testing.T) {
	m := press(overRollup(failFirstRollup(), 160, 24), "j", "j", "k")

	var reruns []string
	for _, binding := range m.ShortHelp() {
		if strings.Contains(binding.Help().Desc, "rerun") {
			reruns = append(reruns, binding.Help().Key)
		}
	}
	if !slices.Equal(reruns, []string{"r", "R"}) {
		t.Errorf("the hint line offers %v, want r for the failed jobs and R for all of them", reruns)
	}

	_, cmd := key(m, "r")
	if cmd == nil {
		t.Fatal("r on the workflow row did nothing")
	}
	raw := cmd()
	msg, ok := raw.(prview.RerunRunMsg)
	if !ok {
		t.Fatalf("r sent %T, want the run rather than the job under the selection", raw)
	}
	if !slices.Equal(msg.JobIDs, []int64{103}) {
		t.Errorf("JobIDs = %v, want the failed job of the run", msg.JobIDs)
	}
}

func TestTheBulkKeysAreDeadWhileTheLogPaneHasTheKeys(t *testing.T) {
	m := press(overRollup(bulkRollup(), 160, 24), "j", "2")

	for _, k := range []string{"r", "R"} {
		_, cmd := key(m, k)
		if cmd == nil {
			continue
		}
		if _, ok := cmd().(prview.RerunRunMsg); ok {
			t.Errorf("%s from the log pane asked to rerun the whole run", k)
		}
	}
}

func TestASupersededAttemptDoesNotGetItsOwnRow(t *testing.T) {
	old := time.Now().Add(-10 * time.Minute)
	r := gh.CheckRollup{
		State: gh.CheckStateFailure,
		Checks: []gh.Check{
			{Name: "unit", Workflow: "CI", State: gh.CheckStateSkipped, JobID: 101, DistinctID: 1, CompletedAt: old},
			{Name: "vet", Workflow: "CI", State: gh.CheckStateSuccess, JobID: 104},
			{Name: "unit", Workflow: "CI", State: gh.CheckStateFailure, JobID: 105, DistinctID: 2, CompletedAt: time.Now()},
		},
	}

	rows := filledCheckRows(overRollup(r, 160, 24))
	if got := strings.Count(strings.Join(rows, "\n"), "unit"); got != 1 {
		t.Errorf("unit takes %d rows, want the newest attempt alone:\n%s", got, strings.Join(rows, "\n"))
	}
	if len(rows) < 3 || !strings.Contains(rows[1], "unit") || !strings.Contains(rows[2], "vet") {
		t.Errorf("rows = %q, want the kept attempt in the slot the first one had", rows)
	}
	if !strings.Contains(rows[1], "✗") {
		t.Errorf("unit = %q, want the newest attempt's state rather than the skipped one's", rows[1])
	}
}

func TestARerunKeepsItsRowWhileGitHubHasDroppedIt(t *testing.T) {
	m := press(overRollup(bulkRollup(), 160, 24), "j")
	if _, cmd := key(m, "r"); cmd == nil {
		t.Fatal("r on the workflow row did not ask for a rerun")
	}
	m, _ = key(m, "r")

	gone := bulkRollup()
	gone.Checks = slices.DeleteFunc(gone.Checks, func(c gh.Check) bool { return c.JobID == 103 })
	d := sampleDetail()
	d.Rollup = gone
	m.SetDetail(held(d))

	rows := strings.Join(filledCheckRows(m), "\n")
	if !strings.Contains(rows, "test") {
		t.Errorf("the rerunning job left the column:\n%s", rows)
	}
	if !strings.Contains(rows, "● test") {
		t.Errorf("the held row does not read as running:\n%s", rows)
	}
	if _, again := key(m, "r"); again != nil {
		t.Error("the mark was dropped with the check, so r started a second rerun")
	}
}

func TestARerunDropsTheLogOfTheAttemptItReplaces(t *testing.T) {
	for _, tt := range []struct {
		name   string
		rollup gh.CheckRollup
		walk   []string
		key    string
	}{
		{name: "one job", rollup: bulkRollup(), walk: []string{"j", "j", "j"}, key: "r"},
		{name: "the whole run", rollup: failFirstRollup(), walk: []string{"j", "j", "k"}, key: "R"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := press(overRollup(tt.rollup, 160, 24), tt.walk...)
			m.SetJob(103, loadedJob(103, true))
			if out := stripANSI(m.View()); !strings.Contains(out, "tests failed") {
				t.Fatalf("the log never landed to begin with:\n%s", out)
			}

			m, cmd := key(m, tt.key)
			if cmd == nil {
				t.Fatalf("%s did not ask for a rerun", tt.key)
			}
			if out := stripANSI(m.View()); strings.Contains(out, "tests failed") {
				t.Errorf("the replaced attempt's log is still on the pane:\n%s", out)
			}
		})
	}
}

func TestRerunningAJobTakesItsWorkflowOffFailing(t *testing.T) {
	m := press(overRollup(bulkRollup(), 160, 24), "j", "j", "j")

	rows := filledCheckRows(m)
	if !strings.Contains(rows[1], "✗ Build") {
		t.Fatalf("the workflow does not start out failing: %q", rows[1])
	}

	m, cmd := key(m, "r")
	if cmd == nil {
		t.Fatal("r did not ask for a rerun")
	}

	rows = filledCheckRows(m)
	if !strings.Contains(rows[1], "● Build") {
		t.Errorf("the workflow = %q, want it running while its only failure is rerunning", rows[1])
	}
	if !strings.Contains(rows[3], "● test") {
		t.Errorf("the job = %q, want it running", rows[3])
	}
}

func TestALineWhereTheStepsWouldBeSitsInsideThePaneAndWraps(t *testing.T) {
	long := "no such host: " + strings.Repeat("a very long resolver name ", 12)
	m := onChecks(160, 24)
	m.SetJob(101, store.Job{Status: store.StatusFailed, Err: errors.New(long)})

	lines := strings.Split(stripANSI(m.View()), "\n")
	head := slices.IndexFunc(lines, func(l string) bool {
		return strings.Contains(l, "Could not load the job log")
	})
	if head < 0 {
		t.Fatalf("the note is not on the pane:\n%s", strings.Join(lines, "\n"))
	}

	body := logPaneText(t, lines[head])
	if got := len(body) - len(strings.TrimLeft(body, " ")); got != 2 {
		t.Errorf("the note is inset %d columns, want %d:\n%q", got, 2, lines[head])
	}
	if !strings.Contains(stripANSI(m.View()), "resolver name") {
		t.Fatal("the fixture never reached the pane")
	}
	if strings.Contains(lines[head], "a very long resolver name a very long resolver name a very long resolver name") {
		t.Errorf("the note did not wrap:\n%q", lines[head])
	}
}

func logPaneText(t *testing.T, line string) string {
	t.Helper()
	at := strings.LastIndex(line, "│")
	first := strings.Index(line, "│")
	if first < 0 || at <= first {
		t.Fatalf("no log pane on the row: %q", line)
	}
	inner := line[:at]
	if cut := strings.LastIndex(inner, "│"); cut >= 0 {
		inner = inner[cut+len("│"):]
	}
	return inner
}

func TestTheLogPaneSaysItIsWaitingOnTheNewAttempt(t *testing.T) {
	m := press(overRollup(bulkRollup(), 160, 24), "j", "j", "j")
	m.SetJob(103, loadedJob(103, true))

	m, cmd := key(m, "r")
	if cmd == nil {
		t.Fatal("r did not ask for a rerun")
	}

	out := stripANSI(m.View())
	if !strings.Contains(out, "Waiting for the new attempt") {
		t.Errorf("the pane does not say what it is waiting on:\n%s", out)
	}
	if strings.Contains(out, "Loading the job log") {
		t.Error("the pane claims a fetch that is not in flight")
	}

	if _, tick := m.Update(spinner.TickMsg{}); tick == nil {
		t.Error("the tick chain died, so the glyph freezes on its first frame")
	}
}

func TestAQueuedAttemptWinsTheCollapseOverTheOneItReplaces(t *testing.T) {
	done := time.Now().Add(-5 * time.Minute)
	r := gh.CheckRollup{State: gh.CheckStateFailure, Checks: []gh.Check{
		{Name: "test", Workflow: "CI", State: gh.CheckStateFailure, JobID: 1, DistinctID: 1, StartedAt: done, CompletedAt: done},
		{Name: "test", Workflow: "CI", State: gh.CheckStatePending, JobID: 2, DistinctID: 2},
	}}

	rows := filledCheckRows(overRollup(r, 160, 24))
	if len(rows) != 1 {
		t.Fatalf("rows = %q, want the two attempts collapsed to one", rows)
	}
	if !strings.Contains(rows[0], "●") {
		t.Errorf("row = %q, want the queued attempt rather than the one it replaces", rows[0])
	}
}

func TestHeldRowsComeBackInTheSameOrderEveryTime(t *testing.T) {
	run := gh.CheckRollup{State: gh.CheckStateFailure, Checks: []gh.Check{
		{Name: "solo", Workflow: "S", State: gh.CheckStateSuccess, JobID: 1},
		{Name: "aaa", Workflow: "W", State: gh.CheckStateFailure, JobID: 11, RunID: 900},
		{Name: "bbb", Workflow: "W", State: gh.CheckStateFailure, JobID: 22, RunID: 900},
		{Name: "ccc", Workflow: "W", State: gh.CheckStateFailure, JobID: 33, RunID: 900},
	}}

	seen := map[string]bool{}
	for range 40 {
		m := press(overRollup(run, 160, 24), "j")
		m, cmd := key(m, "R")
		if cmd == nil {
			t.Fatal("R did not ask for a rerun")
		}
		d := sampleDetail()
		d.Rollup = gh.CheckRollup{}
		m.SetDetail(held(d))
		seen[strings.Join(filledCheckRows(m), "|")] = true
	}
	if len(seen) != 1 {
		t.Errorf("the held rows came back in %d different orders", len(seen))
		for k := range seen {
			t.Logf("  %s", k)
		}
	}
}

func TestAWorkflowRunningTwiceIsTwoRowsAndTwoReruns(t *testing.T) {
	r := gh.CheckRollup{State: gh.CheckStateFailure, Checks: []gh.Check{
		{Name: "solo", Workflow: "S", State: gh.CheckStateSuccess, JobID: 1},
		{Name: "aaa", Workflow: "CI", State: gh.CheckStateFailure, JobID: 11, RunID: 900},
		{Name: "bbb", Workflow: "CI", State: gh.CheckStateFailure, JobID: 22, RunID: 901},
	}}

	rows := filledCheckRows(overRollup(r, 160, 24))
	if len(rows) != 3 {
		t.Fatalf("rows = %q, want one row per run rather than a parent over both", rows)
	}

	m := press(overRollup(r, 160, 24), "j")
	if _, cmd := key(m, "R"); cmd != nil {
		if msg, ok := cmd().(prview.RerunRunMsg); ok && len(msg.JobIDs) > 1 {
			t.Errorf("R marked %v, want the jobs of one run", msg.JobIDs)
		}
	}
}

func TestRerunAllIsQuietWhileTheRunIsStillGoing(t *testing.T) {
	r := bulkRollup()
	for i := range r.Checks {
		if r.Checks[i].Name == "lint" {
			r.Checks[i].State = gh.CheckStatePending
		}
	}
	m := press(overRollup(r, 160, 24), "j")

	if _, cmd := key(m, "R"); cmd != nil {
		if msg, ok := cmd().(prview.RerunRunMsg); ok {
			t.Errorf("R asked to rerun a run still in progress: %+v", msg)
		}
	}
	if _, cmd := key(m, "r"); cmd == nil {
		t.Error("r went quiet too, but a failed job can be rerun while others run")
	}
}

func TestASelectionFollowsItsCheckIntoANewAttempt(t *testing.T) {
	old := time.Now().Add(-5 * time.Minute)
	before := gh.CheckRollup{State: gh.CheckStateFailure, Checks: []gh.Check{
		{Name: "first", Workflow: "A", State: gh.CheckStateSuccess, JobID: 1},
		{Name: "test", Workflow: "B", State: gh.CheckStateFailure, JobID: 2, DistinctID: 1, CompletedAt: old},
	}}
	m := press(overRollup(before, 160, 24), "j")
	if got := logPaneHead(t, m); got != "B / test" {
		t.Fatalf("the walk did not land on the check, pane reads %q", got)
	}

	after := before
	after.Checks = []gh.Check{
		before.Checks[0],
		{Name: "test", Workflow: "B", State: gh.CheckStatePending, JobID: 3, DistinctID: 2},
	}
	d := sampleDetail()
	d.Rollup = after
	m.SetDetail(held(d))

	if got := logPaneHead(t, m); got != "B / test" {
		t.Errorf("the selection landed on %q, want it to follow the check into its new attempt", got)
	}
}

func logPaneHead(t *testing.T, m prview.Model) string {
	t.Helper()
	for _, line := range strings.Split(stripANSI(m.View()), "\n") {
		for _, glyph := range []string{"✓ ", "✗ ", "● ", "○ "} {
			at := strings.Index(line, "│ "+glyph)
			if at < 0 {
				continue
			}
			rest := line[at+len("│ "+glyph):]
			if cut := strings.Index(rest, "│"); cut >= 0 {
				rest = rest[:cut]
			}
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

func TestTheRailListsTheSameAttemptsTheTabDoes(t *testing.T) {
	old := time.Now().Add(-5 * time.Minute)
	d := sampleDetail()
	d.Rollup = gh.CheckRollup{State: gh.CheckStateFailure, Checks: []gh.Check{
		{Name: "test", Workflow: "CI", State: gh.CheckStateFailure, JobID: 1, DistinctID: 1, CompletedAt: old},
		{Name: "test", Workflow: "CI", State: gh.CheckStatePending, JobID: 2, DistinctID: 2},
	}}

	out := stripANSI(detailed(held(d), 200, 60).View())
	if got := strings.Count(out, "CI / test"); got != 1 {
		t.Errorf("the rail lists CI / test %d times, want the one attempt the tab draws:\n%s", got, out)
	}
}
