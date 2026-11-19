package store_test

import (
	"testing"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
)

func drafted() gh.ReviewThread {
	return gh.ReviewThread{
		Path: "main.go", Line: 12, Side: gh.SideRight, Subject: gh.SubjectLine,
		Comments: []gh.Comment{{Author: gh.Actor{Login: "drucial"}, Body: "Name this."}},
	}
}

func onlyThread(t *testing.T, d store.Detail) gh.ReviewThread {
	t.Helper()
	if len(d.Detail.Threads) != 1 {
		t.Fatalf("held %d threads, want 1", len(d.Detail.Threads))
	}
	return d.Detail.Threads[0]
}

func TestAPendingThreadDrawsAsAPostingDraft(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", gh.DetailResult{})

	key := s.PendingThread("PR_1", drafted())

	got := onlyThread(t, s.Detail("PR_1"))
	if got.ID != key || !got.Draft || got.CanReply {
		t.Errorf("thread = %+v, want a draft under the write's key that takes no reply", got)
	}
	if c := got.Comments[0]; c.ID != key || !c.Pending || !c.Draft || c.Kind != gh.CommentThread {
		t.Errorf("comment = %+v, want a pending draft thread comment under the write's key", c)
	}
}

func TestARefetchDoesNotDropAThreadStillInFlight(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", gh.DetailResult{})
	s.PendingThread("PR_1", drafted())

	s.DetailApplied("PR_1", gh.DetailResult{})

	onlyThread(t, s.Detail("PR_1"))
}

func TestAThreadThatLandsTakesGitHubsThread(t *testing.T) {
	tests := []struct {
		name string
		held gh.Review
		want string
	}{
		{"it opens the review", gh.Review{}, "PRR_NEW"},
		{"it joins the one there", gh.Review{ID: "PRR_HELD", State: gh.ReviewStatePending}, "PRR_HELD"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := store.New(configured())
			s.DetailApplied("PR_1", gh.DetailResult{Detail: gh.PullRequestDetail{DraftReview: tt.held}})
			key := s.PendingThread("PR_1", drafted())

			landed := drafted()
			landed.ID, landed.ReviewID, landed.Draft = "PRRT_1", "PRR_NEW", true
			s.ThreadApplied("PR_1", key, gh.ReviewThreadResult{Thread: landed})

			d := s.Detail("PR_1")
			if got := onlyThread(t, d); got.ID != "PRRT_1" {
				t.Errorf("thread = %q, want GitHub's in place of the draft", got.ID)
			}
			if d.Detail.DraftReview.ID != tt.want {
				t.Errorf("pending review = %q, want %q", d.Detail.DraftReview.ID, tt.want)
			}
		})
	}
}

func TestAThreadThatFailsLeavesNoTrace(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", gh.DetailResult{})
	key := s.PendingThread("PR_1", drafted())

	s.EditReverted("PR_1", key)

	if got := s.Detail("PR_1").Detail.Threads; len(got) != 0 {
		t.Errorf("threads = %+v, want the draft gone", got)
	}
}
