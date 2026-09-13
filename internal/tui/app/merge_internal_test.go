package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/config"
	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

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
		{"a first landing that does not know", false, landed(gh.PRStateOpen, gh.MergeUnknown), true},

		{"a refetch that does not know", true, landed(gh.PRStateOpen, gh.MergeUnknown), false},

		{"a first landing that knows", false, landed(gh.PRStateOpen, gh.MergeClean), false},

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

// Waits the delay out rather than dropping it, the only way to watch the tick arrive.
func TestADetailLandingArmsTheProbe(t *testing.T) {
	cfg := &config.Config{Defaults: config.Defaults{PRsLimit: 20}}
	m := New(cfg, nil, testSurface, nil)

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

func carries[T tea.Msg](cmd tea.Cmd, budget time.Duration) bool {
	if cmd == nil {
		return false
	}

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
