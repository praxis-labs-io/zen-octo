package store_test

import (
	"testing"
	"time"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
)

func pulsed(p gh.Pulse) gh.PulseResult {
	return gh.PulseResult{
		Pulse:     p,
		RateLimit: gh.RateLimit{Limit: 5000, Cost: 2, Remaining: 4200, ResetAt: time.Now().Add(time.Hour)},
	}
}

func held(t *testing.T) store.Store {
	t.Helper()

	s := store.New(configured())
	s.BeginDetail("pr1")
	s.DetailApplied("pr1", reviewed())
	return s
}

func beginOnCopy(s store.Store) bool { return s.BeginPulse("pr1") }

func TestTheFlightSurvivesTheCopyItIsMarkedOn(t *testing.T) {
	s := held(t)

	if !beginOnCopy(s) {
		t.Fatal("setup: the copy could not start a pulse")
	}
	if s.BeginPulse("pr1") {
		t.Error("a second pulse started while the first was still out")
	}
}

func TestAPulseLeavesThePageAlone(t *testing.T) {
	s := held(t)

	if !s.BeginPulse("pr1") {
		t.Fatal("setup: the store refused a pulse on a loaded detail")
	}
	s.PulseApplied("pr1", pulsed(gh.Pulse{State: gh.PRStateOpen, Merge: gh.MergeClean}))

	d := s.Detail("pr1").Detail
	if len(d.Timeline) != 1 {
		t.Errorf("timeline holds %d items, want the one the fetch brought", len(d.Timeline))
	}
	if len(d.Threads) != 2 {
		t.Errorf("threads = %d, want the two the fetch brought", len(d.Threads))
	}
	if len(d.Reviewers) != 1 || d.Reviewers[0].Threads != 2 {
		t.Errorf("reviewers = %+v, want the panel and its counts untouched", d.Reviewers)
	}
}

func TestAPulseMovesTheFieldsItCarries(t *testing.T) {
	s := held(t)

	s.BeginPulse("pr1")
	s.PulseApplied("pr1", pulsed(gh.Pulse{
		State:          gh.PRStateClosed,
		IsDraft:        true,
		ReviewDecision: gh.ReviewDecisionApproved,
		Merge:          gh.MergeBlocked,
		Rollup:         gh.CheckRollup{State: gh.CheckStateFailure, Failed: 1},
	}))

	d := s.Detail("pr1").Detail
	if d.State != gh.PRStateClosed || !d.IsDraft {
		t.Errorf("lifecycle = %q draft=%v, want what the pulse answered", d.State, d.IsDraft)
	}
	if d.ReviewDecision != gh.ReviewDecisionApproved || d.Merge != gh.MergeBlocked {
		t.Errorf("review = %q merge = %q, want what the pulse answered", d.ReviewDecision, d.Merge)
	}
	if d.Rollup.Failed != 1 || d.Checks != gh.CheckStateFailure {
		t.Errorf("checks = %q over %+v, want the rollup and its summary", d.Checks, d.Rollup)
	}
}

func TestAMovedHeadMarksTheDiffStale(t *testing.T) {
	s := held(t)

	s.BeginPulse("pr1")
	s.PulseApplied("pr1", pulsed(gh.Pulse{HeadRefOid: "9f1c2b7"}))

	if !s.StaleFiles("pr1") {
		t.Error("the head moved and the held diff was left reading as current")
	}
}

func TestAHeadThatDidNotMoveLeavesTheDiffAlone(t *testing.T) {
	s := store.New(configured())
	s.BeginDetail("pr1")

	res := reviewed()
	res.Detail.HeadRefOid = "9f1c2b7"
	s.DetailApplied("pr1", res)

	s.BeginPulse("pr1")
	s.PulseApplied("pr1", pulsed(gh.Pulse{HeadRefOid: "9f1c2b7"}))

	if s.StaleFiles("pr1") {
		t.Error("the diff was thrown away for a head that never moved")
	}
}

func TestAPulseCorrectsTheRowBehindIt(t *testing.T) {
	s := store.New(configured())
	s.Applied(0, gh.SearchResult{PullRequests: []gh.PullRequest{{ID: "pr1", State: gh.PRStateOpen}}})
	s.BeginDetail("pr1")
	s.DetailApplied("pr1", reviewed())

	s.BeginPulse("pr1")
	s.PulseApplied("pr1", pulsed(gh.Pulse{State: gh.PRStateMerged}))

	if got := s.Sections()[0].PRs[0].State; got != gh.PRStateMerged {
		t.Errorf("row reads %q, want the pulse's answer written back over it", got)
	}
}

func TestAPulseDoesNotPutTheRowBackUnderAHeldWrite(t *testing.T) {
	s := store.New(configured())
	s.Applied(0, gh.SearchResult{PullRequests: []gh.PullRequest{{ID: "pr1", State: gh.PRStateOpen}}})
	s.BeginDetail("pr1")
	s.DetailApplied("pr1", reviewed())

	s.PendingState("pr1", gh.TransitionClose)

	s.BeginPulse("pr1")
	s.PulseApplied("pr1", pulsed(gh.Pulse{State: gh.PRStateOpen}))

	if got := s.Sections()[0].PRs[0].State; got != gh.PRStateClosed {
		t.Errorf("row reads %q, want the held close kept: GitHub has not seen it yet", got)
	}
}

func TestAPulseAnsweringAfterAWriteIsDropped(t *testing.T) {
	s := held(t)
	s.BeginPulse("pr1")

	key := s.PendingState("pr1", gh.TransitionClose)
	s.StateApplied("pr1", key, gh.PRStateResult{State: gh.PRStateClosed})

	s.PulseApplied("pr1", pulsed(gh.Pulse{State: gh.PRStateOpen}))

	if got := s.Detail("pr1").Detail.State; got != gh.PRStateClosed {
		t.Errorf("state = %q, want it still closed: the pulse predates the write", got)
	}
	if !s.StalePulse("pr1") {
		t.Error("the dropped pulse left no debt, so the row latches on what it had")
	}
}

func TestAPulseOvertakenByADetailIsDropped(t *testing.T) {
	s := held(t)
	s.BeginPulse("pr1")

	if !s.BeginDetail("pr1") {
		t.Fatal("setup: a pulse in flight blocked a real fetch")
	}
	s.PulseApplied("pr1", pulsed(gh.Pulse{State: gh.PRStateClosed}))

	if got := s.Detail("pr1").Detail.State; got == gh.PRStateClosed {
		t.Error("the overtaken pulse landed on top of the fetch that passed it")
	}
}

func TestTheStoreRefusesAPulseItCannotUse(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*store.Store)
	}{
		{"never fetched", func(*store.Store) {}},
		{"a detail already in flight", func(s *store.Store) { s.BeginDetail("pr1") }},
		{"a pulse already in flight", func(s *store.Store) { s.BeginPulse("pr1") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := store.New(configured())
			if tt.name != "never fetched" {
				s.BeginDetail("pr1")
				s.DetailApplied("pr1", reviewed())
			}
			tt.setup(&s)

			if s.BeginPulse("pr1") {
				t.Error("the store took a pulse it had no use for")
			}
		})
	}
}

func TestAPulseDoesNotDisturbAWriteStillInFlight(t *testing.T) {
	s := held(t)
	s.PendingComment("pr1", gh.Comment{Body: "looks right to me", Author: gh.Actor{Login: "drucial"}})

	s.BeginPulse("pr1")
	s.PulseApplied("pr1", pulsed(gh.Pulse{State: gh.PRStateOpen}))

	if got := len(s.Detail("pr1").Detail.Timeline); got != 2 {
		t.Errorf("timeline holds %d, want the fetched item and the held comment", got)
	}
}

func TestAFailedPulseKeepsThePageAndRaisesNoError(t *testing.T) {
	s := held(t)
	s.BeginPulse("pr1")
	s.PulseFailed("pr1")

	d := s.Detail("pr1")
	if d.Err != nil || d.Status != store.StatusReady {
		t.Errorf("detail reads %v at %v, want it untouched", d.Err, d.Status)
	}
	if !s.BeginPulse("pr1") {
		t.Error("the failure left the flight marked, so nothing can recheck again")
	}
}
