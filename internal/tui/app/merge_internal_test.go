package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/config"
	"github.com/zen-octo/zen-octo/internal/gh"
)

// The probe is a timer, and the harness outside this package drops one rather
// than sleeping on it. So the rule about when one is armed at all has to be
// checked here: everything else about the probe is driven through the real
// interface in merge_test.go.
func TestTheMergeabilityProbeIsArmedOnlyWhereItBuysSomething(t *testing.T) {
	landed := func(state gh.PRState, merge gh.MergeState) gh.DetailResult {
		return gh.DetailResult{Detail: gh.PullRequestDetail{
			PullRequest: gh.PullRequest{ID: "PR_1", State: state},
			Merge:       merge,
		}}
	}

	tests := []struct {
		name   string
		held   bool
		res    gh.DetailResult
		wantOn bool
	}{
		// The first query is what starts GitHub computing, so the answer is
		// usually there a moment later.
		{"a first landing that does not know", false, landed(gh.PRStateOpen, gh.MergeUnknown), true},

		// One extra request, not a loop: a refetch lands over a detail already
		// held, so a pull request GitHub keeps answering UNKNOWN for is asked
		// twice and then left alone.
		{"a refetch that does not know", true, landed(gh.PRStateOpen, gh.MergeUnknown), false},

		{"a first landing that knows", false, landed(gh.PRStateOpen, gh.MergeClean), false},

		// Nothing is going to be merged, so an answer would change nothing on
		// the screen.
		{"merged", false, landed(gh.PRStateMerged, gh.MergeUnknown), false},
		{"closed", false, landed(gh.PRStateClosed, gh.MergeUnknown), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m Model
			if tt.held {
				m.store.DetailApplied("PR_1", landed(gh.PRStateOpen, gh.MergeUnknown))
			}

			if got := m.probeMergeability("PR_1", tt.res) != nil; got != tt.wantOn {
				t.Errorf("armed = %v, want %v", got, tt.wantOn)
			}
		})
	}
}

// And that the arming is actually wired into the response, which the harness
// outside this package cannot see either: it drops a timer rather than sleeping
// on one, so a probe that is never armed and a probe that is armed and dropped
// look the same from there.
//
// This one waits the delay out rather than dropping it, which is the only way
// to watch a tick arrive. One test paying that is worth the call site being
// covered at all.
func TestADetailLandingArmsTheProbe(t *testing.T) {
	cfg := &config.Config{Defaults: config.Defaults{PRsLimit: 20}, Theme: "rose-pine-moon"}
	m := New(cfg, nil)

	landed := gh.DetailResult{Detail: gh.PullRequestDetail{
		PullRequest: gh.PullRequest{ID: "PR_1", State: gh.PRStateOpen},
		Merge:       gh.MergeUnknown,
	}}

	_, cmd := m.Update(detailFetchedMsg{id: "PR_1", res: landed})
	if cmd == nil {
		t.Fatal("a detail that does not know whether it merges produced no command at all")
	}

	if !carries[mergeProbeMsg](cmd, mergeProbeDelay+time.Second) {
		t.Error("nothing in the response arms the mergeability probe")
	}
}

// carries runs a command and everything it batches, and reports whether any of
// them answers with the message type asked for inside the budget.
func carries[T tea.Msg](cmd tea.Cmd, budget time.Duration) bool {
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()

	select {
	case msg := <-done:
		if _, ok := msg.(T); ok {
			return true
		}
		batch, ok := msg.(tea.BatchMsg)
		if !ok {
			return false
		}
		for _, sub := range batch {
			if carries[T](sub, budget) {
				return true
			}
		}
		return false
	case <-time.After(budget):
		return false
	}
}
