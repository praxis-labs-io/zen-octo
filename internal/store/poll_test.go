package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/store"
)

// settled is a pull request with its checks in, which is what a second recheck
// answering the same thing looks like.
func settled(at time.Time) gh.Pulse {
	return gh.Pulse{
		State: gh.PRStateOpen, Merge: gh.MergeClean, UpdatedAt: at,
		Rollup: gh.CheckRollup{
			State:  gh.CheckStateSuccess,
			Checks: []gh.Check{{Name: "test", Workflow: "ci", State: gh.CheckStateSuccess}},
			Passed: 1,
		},
	}
}

// applyPulse begins one and lands it, returning whether the store saw a change.
func applyPulse(t *testing.T, s *store.Store, p gh.Pulse) bool {
	t.Helper()

	if !s.BeginPulse("pr1") {
		t.Fatal("setup: the store refused a pulse on a loaded detail")
	}
	return s.PulseApplied("pr1", pulsed(p))
}

// heldAt is the page under it, fetched as of an instant. The bare fixture
// carries none, which reads as every first recheck owing a page.
func heldAt(t *testing.T, at time.Time) store.Store {
	t.Helper()

	res := reviewed()
	res.Detail.UpdatedAt = at

	s := store.New(configured())
	s.BeginDetail("pr1")
	s.DetailApplied("pr1", res)
	return s
}

// The whole reason the answer is worth reporting: SetDetail relayouts the page,
// and on a timer an unconditional push is a hitch a beat for a page sitting still.
func TestARecheckThatChangesNothingSaysSo(t *testing.T) {
	s := held(t)
	at := time.Now()

	if !applyPulse(t, &s, settled(at)) {
		t.Fatal("setup: the first recheck moved nothing, so there is no baseline")
	}
	if applyPulse(t, &s, settled(at)) {
		t.Error("a recheck answering the same thing reported a change")
	}
}

// One check turning green is the change the reader is sitting there waiting for.
func TestACheckTurningGreenIsAChange(t *testing.T) {
	s := held(t)
	at := time.Now()

	running := settled(at)
	running.Rollup = gh.CheckRollup{
		State:   gh.CheckStatePending,
		Checks:  []gh.Check{{Name: "test", Workflow: "ci", State: gh.CheckStatePending}},
		Pending: 1,
	}
	applyPulse(t, &s, running)

	if !applyPulse(t, &s, settled(at)) {
		t.Error("the check went from pending to passing and the store called it unchanged")
	}
}

// A rollup can gain a job without the summary moving: five pending become six.
func TestAnAddedCheckIsAChangeWhileTheSummaryHoldsStill(t *testing.T) {
	s := held(t)
	at := time.Now()

	one := settled(at)
	one.Rollup.State = gh.CheckStatePending
	one.Rollup.Checks = []gh.Check{{Name: "test", Workflow: "ci", State: gh.CheckStatePending}}
	applyPulse(t, &s, one)

	two := one
	two.Rollup.Checks = append(append([]gh.Check(nil), one.Rollup.Checks...),
		gh.Check{Name: "lint", Workflow: "ci", State: gh.CheckStatePending})

	if !applyPulse(t, &s, two) {
		t.Error("a second job appeared behind an unchanged summary and read as no change")
	}
}

// GitHub's instant is the only thing on this wire that reports a comment, a
// review or a label, none of which the pulse itself can carry.
func TestAMovedInstantOwesTheWholePage(t *testing.T) {
	at := time.Now()
	s := heldAt(t, at)

	// The same instant the page was fetched at: nothing has happened since.
	applyPulse(t, &s, settled(at))
	if s.StaleTimeline("pr1") {
		t.Fatal("a recheck answering the fetched instant owes a page anyway")
	}

	applyPulse(t, &s, settled(at.Add(time.Minute)))
	if !s.StaleTimeline("pr1") {
		t.Error("somebody commented, the instant moved, and nothing owes the page it is on")
	}
}

// The debt is what a full fetch is for, so landing one pays it.
func TestTheFetchThatArrivesPaysTheDebt(t *testing.T) {
	s := held(t)
	at := time.Now()

	applyPulse(t, &s, settled(at.Add(time.Minute)))
	if !s.StaleTimeline("pr1") {
		t.Fatal("setup: nothing owed a page, so there is no debt to pay")
	}

	s.BeginDetail("pr1")
	s.DetailApplied("pr1", reviewed())
	if s.StaleTimeline("pr1") {
		t.Error("the page arrived and the debt for it is still standing")
	}
}

// A response the store threw away wrote nothing, so nothing moved. Reporting a
// change would relayout the page for an answer that was never taken.
func TestADroppedRecheckReportsNoChange(t *testing.T) {
	s := held(t)

	if !s.BeginPulse("pr1") {
		t.Fatal("setup: the store refused a pulse on a loaded detail")
	}
	// A write settling underneath it is what marks the answer overtaken: it
	// carries the state from before the write and would put it back undone.
	key := s.PendingState("pr1", gh.TransitionClose)
	s.StateApplied("pr1", key, gh.PRStateResult{State: gh.PRStateClosed})

	if s.PulseApplied("pr1", pulsed(settled(time.Now()))) {
		t.Error("a recheck the store dropped reported a change anyway")
	}
	if got := s.Detail("pr1").Detail.State; got != gh.PRStateClosed {
		t.Errorf("the pull request reads %v, want the write the recheck was dropped for", got)
	}
}

// The map has to be the one New built. Built on first write instead, it is built
// on whichever copy wrote first and every read after that sees a nil one.
func TestTheDebtSurvivesTheCopyItIsMarkedOn(t *testing.T) {
	s := held(t)
	at := time.Now()

	applyPulse(t, &s, settled(at))
	markOnCopy(s, settled(at.Add(time.Minute)))

	if !s.StaleTimeline("pr1") {
		t.Error("the debt was marked on a copy and went with it")
	}
}

// markOnCopy takes the Store by value, the way the app's own value receivers do.
func markOnCopy(s store.Store, p gh.Pulse) {
	s.BeginPulse("pr1")
	s.PulseApplied("pr1", pulsed(p))
}

// loadedSection is one section with rows in it, which is the only kind polled.
func loadedSection(t *testing.T) store.Store {
	t.Helper()

	s := store.New(configured())
	if !s.Begin(0) {
		t.Fatal("setup: the store refused the first fetch")
	}
	s.Applied(0, result("pr1", "pr2"))
	return s
}

// The list renders the error state instead of the rows, so a failed poll nobody
// asked for would empty a tab the reader is reading fine.
func TestAFailedPollKeepsTheRows(t *testing.T) {
	s := loadedSection(t)

	if !s.Begin(0) {
		t.Fatal("setup: the store refused the poll")
	}
	s.PollFailed(0)

	got := s.Sections()[0]
	if got.Status != store.StatusReady {
		t.Errorf("the section sits at status %v, want it ready the way it was", got.Status)
	}
	if got.Err != nil {
		t.Errorf("the section carries %v, want a poll nobody asked for to keep quiet", got.Err)
	}
	if len(got.PRs) != 2 {
		t.Errorf("the section holds %d rows, want the two it had", len(got.PRs))
	}
}

// The contrast that makes the point: the key the reader pressed does report it.
func TestAFailedSyncStillSaysSo(t *testing.T) {
	s := loadedSection(t)

	s.Begin(0)
	s.Failed(0, errors.New("502 Bad Gateway"))

	if got := s.Sections()[0]; got.Status != store.StatusFailed || got.Err == nil {
		t.Errorf("a sync failed at status %v with error %v, want the failure shown", got.Status, got.Err)
	}
}

// Ending the flight is the other half of its job: leaving the section loading
// would have store.Begin refuse every poll after it for the rest of the session.
func TestAFailedPollLetsTheNextOneStart(t *testing.T) {
	s := loadedSection(t)

	s.Begin(0)
	s.PollFailed(0)

	if !s.Begin(0) {
		t.Error("the section never came out of flight, so nothing can ask again")
	}
}
