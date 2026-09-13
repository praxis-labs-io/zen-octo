package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
)

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

func applyPulse(t *testing.T, s *store.Store, p gh.Pulse) bool {
	t.Helper()

	if !s.BeginPulse("pr1") {
		t.Fatal("setup: the store refused a pulse on a loaded detail")
	}
	return s.PulseApplied("pr1", pulsed(p))
}

func heldAt(t *testing.T, at time.Time) store.Store {
	t.Helper()

	res := reviewed()
	res.Detail.UpdatedAt = at

	s := store.New(configured())
	s.BeginDetail("pr1")
	s.DetailApplied("pr1", res)
	return s
}

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

func TestAMovedInstantOwesTheWholePage(t *testing.T) {
	at := time.Now()
	s := heldAt(t, at)

	applyPulse(t, &s, settled(at))
	if s.StaleTimeline("pr1") {
		t.Fatal("a recheck answering the fetched instant owes a page anyway")
	}

	applyPulse(t, &s, settled(at.Add(time.Minute)))
	if !s.StaleTimeline("pr1") {
		t.Error("somebody commented, the instant moved, and nothing owes the page it is on")
	}
}

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

func TestADroppedRecheckReportsNoChange(t *testing.T) {
	s := held(t)

	if !s.BeginPulse("pr1") {
		t.Fatal("setup: the store refused a pulse on a loaded detail")
	}
	key := s.PendingState("pr1", gh.TransitionClose)
	s.StateApplied("pr1", key, gh.PRStateResult{State: gh.PRStateClosed})

	if s.PulseApplied("pr1", pulsed(settled(time.Now()))) {
		t.Error("a recheck the store dropped reported a change anyway")
	}
	if got := s.Detail("pr1").Detail.State; got != gh.PRStateClosed {
		t.Errorf("the pull request reads %v, want the write the recheck was dropped for", got)
	}
}

func TestTheDebtSurvivesTheCopyItIsMarkedOn(t *testing.T) {
	s := held(t)
	at := time.Now()

	applyPulse(t, &s, settled(at))
	markOnCopy(s, settled(at.Add(time.Minute)))

	if !s.StaleTimeline("pr1") {
		t.Error("the debt was marked on a copy and went with it")
	}
}

func markOnCopy(s store.Store, p gh.Pulse) {
	s.BeginPulse("pr1")
	s.PulseApplied("pr1", pulsed(p))
}

func loadedSection(t *testing.T) store.Store {
	t.Helper()

	s := store.New(configured())
	if !s.Begin(0) {
		t.Fatal("setup: the store refused the first fetch")
	}
	s.Applied(0, result("pr1", "pr2"))
	return s
}

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

func TestAFailedSyncStillSaysSo(t *testing.T) {
	s := loadedSection(t)

	s.Begin(0)
	s.Failed(0, errors.New("502 Bad Gateway"))

	if got := s.Sections()[0]; got.Status != store.StatusFailed || got.Err == nil {
		t.Errorf("a sync failed at status %v with error %v, want the failure shown", got.Status, got.Err)
	}
}

func TestAFailedPollLeavesAReportedFailureStanding(t *testing.T) {
	s := loadedSection(t)

	s.Begin(0)
	s.Failed(0, errors.New("502 Bad Gateway"))

	if !s.Begin(0) {
		t.Fatal("setup: the store refused the poll")
	}
	s.PollFailed(0)

	if got := s.Sections()[0]; got.Status != store.StatusFailed {
		t.Errorf("the section reads %v beside a %v, want the failure still shown", got.Status, got.Err)
	}
}

func TestAFailedPollLetsTheNextOneStart(t *testing.T) {
	s := loadedSection(t)

	s.Begin(0)
	s.PollFailed(0)

	if !s.Begin(0) {
		t.Error("the section never came out of flight, so nothing can ask again")
	}
}
