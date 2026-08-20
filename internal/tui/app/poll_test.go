package app_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/app"
)

// beat fires one tick of the background poll, from an instant the test names.
// The harness drops a tea.Tick, so every beat here is delivered by hand.
func beat(m tea.Model, after time.Duration) tea.Model {
	return settle(m, app.PollTick(time.Now().Add(after)))
}

// opened stages a pull request and puts the detail screen on it. What comes back
// carries MergeUnknown, which is a pull request still settling.
func opened(t *testing.T, client *fakeSearcher) tea.Model {
	t.Helper()

	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	return press(loaded(t, client, 160, 44), "enter")
}

// bumpUpdated moves GitHub's own instant, which is what a comment posted in the
// browser looks like from here: the pulse reports it and carries none of it.
func (f *fakeSearcher) bumpUpdated(id string, at time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()

	held := f.details[id]
	held.UpdatedAt = at
	f.details[id] = held
}

// The ten-second beat belongs to one tab. A timer left behind by Checks must not
// refresh the conversation, either diff, or the list after the reader moves on.
func TestTheChecksBeatFiresOnlyOnTheChecksTab(t *testing.T) {
	tests := []struct {
		name   string
		keys   []string
		pulses int
	}{
		{"conversation", nil, 0},
		{"commits", []string{"]"}, 0},
		{"checks", []string{"]", "]"}, 1},
		{"files", []string{"]", "]", "]"}, 0},
		{"list", []string{"]", "]", "esc"}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeSearcher{prs: samplePRs()}
			m := press(opened(t, client), tt.keys...)

			_ = settle(m, app.ChecksTick(time.Now().Add(app.ChecksBeat+time.Second)))
			if got := len(client.pulsed()); got != tt.pulses {
				t.Errorf("the Checks beat made %d rechecks, want %d", got, tt.pulses)
			}
		})
	}
}

// Both timers can land together at the ten-second boundary. BeginPulse is the
// final guard: the Checks chain must join the five-second chain already out.
func TestTheChecksBeatRefreshesSelectedRunningJobMetadata(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "body")
	client.mu.Lock()
	d := client.details["PR_412"]
	d.Rollup = gh.CheckRollup{Checks: []gh.Check{{
		Name: "test", Workflow: "CI", State: gh.CheckStatePending, JobID: 9001,
	}}}
	client.details["PR_412"] = d
	client.mu.Unlock()
	client.servedJob(9001, gh.Job{ID: 9001, State: gh.CheckStatePending,
		Steps: []gh.JobStep{{Number: 1, Name: "Build", State: gh.CheckStatePending}}}, "")

	m := press(loaded(t, client, 160, 44), "enter", "]", "]")
	m = settleJob(m, d.Rollup.Checks[0], false)
	client.servedJob(9001, gh.Job{ID: 9001, State: gh.CheckStatePending,
		Steps: []gh.JobStep{{Number: 1, Name: "Build", State: gh.CheckStateSuccess},
			{Number: 2, Name: "Test", State: gh.CheckStatePending}}}, "")
	m = settle(m, app.ChecksTick(time.Now().Add(app.ChecksBeat+time.Second)))

	if got := client.askedJobs(); len(got) != 2 {
		t.Errorf("job asks = %v, want metadata refreshed on the beat", got)
	}
	if out := render(t, m); !strings.Contains(out, "Test") {
		t.Errorf("refreshed running steps did not reach the pane:\n%s", out)
	}
}

func TestTheChecksBeatDoesNotDoubleTheBackgroundBeat(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := press(opened(t, client), "]", "]")
	at := time.Now()

	m, background := m.Update(app.PollTick(at.Add(6 * time.Second)))
	m, checks := m.Update(app.ChecksTick(at.Add(app.ChecksBeat + time.Second)))
	_ = settle(m, append(responses(background), responses(checks)...)...)

	if got := len(client.pulsed()); got != 1 {
		t.Errorf("the two beats made %d rechecks, want one", got)
	}
}

// CI is what a reader sits and watches, and watching it is the whole complaint.
func TestABeatRechecksAPullRequestStillMoving(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := opened(t, client)

	beat(m, 6*time.Second)

	if got := client.pulsed(); len(got) != 1 {
		t.Errorf("the beat made %d rechecks, want the one the screen was owed", len(got))
	}
}

// The beat is faster than the interval on purpose: it is one clock for two
// screens, and each decides for itself. A beat inside the interval asks nothing.
func TestABeatInsideTheIntervalAsksNothing(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := opened(t, client)

	beat(m, time.Second)

	if got := client.pulsed(); len(got) != 0 {
		t.Errorf("the beat rechecked %v a second after the page landed", got)
	}
}

// A pull request with its checks in and its mergeability known has nothing
// moving, and asking every beat would spend a request on an unchanging answer.
func TestASettledPullRequestIsAskedLessOften(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveMergeable("PR_412")

	m := press(loaded(t, client, 160, 44), "enter")

	m = beat(m, 6*time.Second)
	if got := client.pulsed(); len(got) != 0 {
		t.Fatalf("a settled pull request was rechecked %v after six seconds", got)
	}

	beat(m, app.PollIdle+time.Second)
	if got := client.pulsed(); len(got) != 1 {
		t.Errorf("made %d rechecks past the idle interval, want one", len(got))
	}
}

// A picker or a form has the keyboard, and an answer landing under one relayouts
// the page it is drawn over.
func TestABeatUnderAFormAsksNothing(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := openMergeForm(t, client)

	before := len(client.pulsed())
	beat(m, app.PollIdle+time.Second)

	if got := len(client.pulsed()); got != before {
		t.Errorf("the beat rechecked %d times under an open form, want none", got-before)
	}
}

// A question nobody asked owes no account of itself. A spinner or a toast for
// one would report a fetch the reader never made.
func TestABeatSaysNothingOnTheStatusBar(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := opened(t, client)

	m = beat(m, 6*time.Second)
	if got := client.pulsed(); len(got) != 1 {
		t.Fatalf("setup: the beat made %d rechecks, want one to have happened at all", len(got))
	}

	bar := lastLine(render(t, m))
	for _, word := range []string{"Refreshing", "Refreshed"} {
		if strings.Contains(bar, word) {
			t.Errorf("status bar = %q, want a beat to pass without saying so", strings.TrimSpace(bar))
		}
	}
}

// Only the section on screen. The others are behind a tab, and their counts
// follow when the reader arrives at them.
func TestABeatOnTheListRefetchesTheSectionOnScreen(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := loaded(t, client, 160, 44)

	before := client.calls()
	beat(m, app.PollIdle+time.Second)

	asked := client.asked()
	if got := len(asked) - before; got != 1 {
		t.Fatalf("the beat sent %d searches, want the one section on screen", got)
	}
	if want := "is:open is:pr author:@me"; asked[len(asked)-1] != want {
		t.Errorf("asked %q, want the active section's own filter %q", asked[len(asked)-1], want)
	}
}

// Which section that is follows the tab strip, or the beat spends every request
// on the one the reader opened with and the tab they moved to never moves.
func TestABeatFollowsTheTabStrip(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := press(loaded(t, client, 160, 44), "]")

	if out := stripANSI(render(t, m)); !strings.Contains(out, "Needs My Review") {
		t.Fatalf("setup: the tab strip never moved:\n%s", out)
	}

	before := client.calls()
	beat(m, app.PollIdle+time.Second)

	asked := client.asked()
	if got := len(asked) - before; got != 1 {
		t.Fatalf("the beat sent %d searches, want the one section on screen", got)
	}
	if want := "is:open is:pr review-requested:@me"; asked[len(asked)-1] != want {
		t.Errorf("asked %q, want the section moved to %q", asked[len(asked)-1], want)
	}
}

// The list renders the error state instead of the rows, so a poll nobody asked
// for would empty a tab the reader is reading fine.
func TestAFailedBeatKeepsTheRowsUp(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := loaded(t, client, 160, 44)

	client.err = errors.New("502 Bad Gateway")
	before := client.calls()
	m = beat(m, app.PollIdle+time.Second)
	if client.calls() == before {
		t.Fatal("setup: the beat sent no search, so nothing failed")
	}

	out := stripANSI(render(t, m))
	if !strings.Contains(out, "Fix auth retry") {
		t.Errorf("a failed beat took the rows with it:\n%s", out)
	}
	if strings.Contains(out, "Failed to load") || strings.Contains(out, "502") {
		t.Errorf("a beat nobody asked for reported its own failure:\n%s", out)
	}
	if bar := lastLine(render(t, m)); strings.Contains(bar, "502") {
		t.Errorf("status bar = %q, want the failure kept quiet", strings.TrimSpace(bar))
	}
}

// A beat holds the section on screen every half minute, so s pressed during one
// used to refresh every tab except the one being read and call it a success.
func TestASyncWaitsOnTheSectionABeatIsHolding(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := loaded(t, client, 160, 44)

	m, held := polling(t, m)
	m = press(m, "s")
	if out := stripANSI(render(t, m)); strings.Contains(out, "Refreshed") {
		t.Error("the sync reported itself while the section on screen was still out")
	}

	if out := stripANSI(render(t, settle(m, held...))); !strings.Contains(out, "Refreshed 2 sections") {
		t.Errorf("view = %q, want the sync to count the section it adopted", out)
	}
}

// PollFailed keeps the rows and the ready status, which is right for a beat and
// wrong for one somebody is waiting on: the summary would call a failure a pass.
func TestASyncReportsTheBeatItAdoptedFailing(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := loaded(t, client, 160, 44)

	client.err = errors.New("502 Bad Gateway")
	m, held := polling(t, m)
	client.err = nil

	out := stripANSI(render(t, settle(press(m, "s"), held...)))
	if !strings.Contains(out, "1 failed") {
		t.Errorf("view = %q, want the sync to report the flight it adopted failing", out)
	}
	if !strings.Contains(out, "Failed to load") {
		t.Errorf("the tab kept its rows over a failure the reader asked for:\n%s", out)
	}
}

// polling sends one beat and holds its answer, which is the section on screen
// left in flight: the state a sync pressed a moment later has to reckon with.
func polling(t *testing.T, m tea.Model) (tea.Model, []tea.Msg) {
	t.Helper()

	m, cmd := m.Update(app.PollTick(time.Now().Add(app.PollIdle + time.Second)))
	held := responses(cmd)
	if len(held) != 1 {
		t.Fatalf("setup: the beat produced %d responses, want the one section on screen", len(held))
	}
	return m, held
}

// The contrast that makes the point: the key the reader pressed does report it.
func TestAFailedSyncStillSaysSoOnTheList(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := loaded(t, client, 160, 44)

	client.err = errors.New("502 Bad Gateway")
	m = press(m, "s")

	if out := stripANSI(render(t, m)); !strings.Contains(out, "Failed to load") {
		t.Errorf("a sync the reader pressed failed quietly:\n%s", out)
	}
}

// An answer resets the clock, or the beat after it asks for what just landed.
func TestALandedBeatStartsTheIntervalAgain(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := loaded(t, client, 160, 44)

	before := client.calls()
	m = beat(m, app.PollIdle+time.Second)
	if got := client.calls() - before; got != 1 {
		t.Fatalf("setup: the beat sent %d searches, want the one that was due", got)
	}

	// Two seconds past the answer, which is well inside the interval.
	beat(m, 2*time.Second)
	if got := client.calls() - before; got != 1 {
		t.Errorf("the section was asked for again two seconds after it answered, %d in all", got)
	}
}

// The stamp is written whether the answer was good or not, so a failure costs
// one interval rather than being retried on every beat after it.
func TestAFailedBeatCostsAnIntervalAndNotEveryBeat(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := loaded(t, client, 160, 44)
	client.err = errors.New("502 Bad Gateway")

	before := client.calls()
	m = beat(m, app.PollIdle+time.Second)
	if got := client.calls() - before; got != 1 {
		t.Fatalf("setup: the beat sent %d searches, want the one that fails", got)
	}

	beat(m, 2*time.Second)
	if got := client.calls() - before; got != 1 {
		t.Errorf("the failure was retried on the next beat, %d searches in all", got)
	}
}

// The second half of the complaint. A comment posted elsewhere moves GitHub's
// instant and nothing else here, so the page it is on has to be asked for.
func TestACommentArrivingBringsTheWholePage(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := opened(t, client)

	before := len(client.opened())
	client.bumpUpdated("PR_412", time.Now())
	beat(m, 6*time.Second)

	if got := len(client.opened()) - before; got != 1 {
		t.Errorf("the page was fetched %d more times, want the one the recheck said it owed", got)
	}
}

// The page is megabytes and the conversation is the only tab any of it reaches,
// so the debt keeps until the reader is somewhere it would show.
func TestTheWholePageWaitsForTheTabItShowsOn(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := press(opened(t, client), "]")

	before := len(client.opened())
	client.bumpUpdated("PR_412", time.Now())
	m = beat(m, 6*time.Second)

	if got := len(client.opened()); got != before {
		t.Fatalf("the page was fetched %d more times away from the conversation", got-before)
	}

	// Back round to it, where every word of what changed would be on screen.
	m = press(m, "]", "]", "]")
	beat(m, 12*time.Second)

	if got := len(client.opened()) - before; got != 1 {
		t.Errorf("the reader came back to the conversation and the page was fetched %d times", got)
	}
}

// The page is megabytes and DetailFailed leaves the debt standing, so nothing
// else would keep a beat from re-sending it every five seconds forever.
func TestAFailedPageIsNotAskedForOnEveryBeat(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := opened(t, client)

	client.bumpUpdated("PR_412", time.Now())
	client.detailErr = errors.New("502 Bad Gateway")

	before := len(client.opened())
	m = beat(m, 6*time.Second)
	if got := len(client.opened()) - before; got != 1 {
		t.Fatalf("setup: the beat asked for the page %d times, want the one that fails", got)
	}

	beat(beat(m, 12*time.Second), 18*time.Second)
	if got := len(client.opened()) - before; got != 1 {
		t.Errorf("the failed page was asked for %d times over three beats, want one", got)
	}
}

// Still a beat nobody asked for, so its failure says nothing either: the page
// on screen is unchanged and a toast is the only thing that would deny it.
func TestAFailedPageSaysNothing(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := opened(t, client)

	client.bumpUpdated("PR_412", time.Now())
	client.detailErr = errors.New("502 Bad Gateway")

	before := len(client.opened())
	m = beat(m, 6*time.Second)
	if len(client.opened()) == before {
		t.Fatal("setup: the beat asked for no page, so nothing failed")
	}

	out := stripANSI(render(t, m))
	if !strings.Contains(out, "Caps the backoff") {
		t.Errorf("the failed page took the conversation with it:\n%s", out)
	}
	if bar := lastLine(render(t, m)); strings.Contains(bar, "Could not refresh") || strings.Contains(bar, "502") {
		t.Errorf("status bar = %q, want a beat's failure kept quiet", strings.TrimSpace(bar))
	}
}

// A recheck that changed nothing must cost no rendering, and the frame is what
// the reader sees of that. The store's own tests are what prove the answer.
func TestARecheckThatChangesNothingLeavesTheFrameAlone(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := opened(t, client)

	was := render(t, m)
	m = beat(m, 6*time.Second)

	if got := client.pulsed(); len(got) != 1 {
		t.Fatalf("setup: the beat made %d rechecks, want one to compare across", len(got))
	}
	if now := render(t, m); now != was {
		t.Errorf("a recheck answering the same thing moved the frame:\n%s", now)
	}
}

// The pulse carries the lifecycle, so a pull request merged elsewhere reaches
// the row behind the screen on a beat, with no page fetched for it.
func TestABeatCorrectsTheRowBehindTheScreen(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := opened(t, client)

	client.setDetailState("PR_412", gh.PRStateMerged)
	m = beat(m, 6*time.Second)

	out := stripANSI(render(t, press(m, "esc")))
	group, ok := groupOf(t, out, "Fix auth retry")
	if !ok {
		t.Fatalf("the pull request left the list entirely:\n%s", out)
	}
	if group != "Merged" {
		t.Errorf("the row sits under %q, want it under Merged once the beat landed:\n%s", group, out)
	}
}
