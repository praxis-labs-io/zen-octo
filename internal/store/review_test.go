package store_test

import (
	"testing"
	"time"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
)

var viewer = gh.Actor{Login: "drucial"}

var submittedAt = time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)

func withPendingReview() gh.DetailResult {
	return gh.DetailResult{Detail: gh.PullRequestDetail{
		DraftReview: gh.Review{ID: "PRR_1", State: gh.ReviewStatePending, Body: "Two things."},
		Reviewers:   []gh.Reviewer{{Actor: viewer, Requested: true}},
		Threads: []gh.ReviewThread{
			{ID: "RT_1", ReviewID: "PRR_1", Draft: true, Comments: []gh.Comment{{ID: "RC_1", Draft: true}}},
			{ID: "RT_2", ReviewID: "PRR_0", Comments: []gh.Comment{{ID: "RC_2"}, {ID: "RC_3", Draft: true}}},
		},
	}}
}

func reviewItem(d store.Detail, id string) (gh.TimelineItem, bool) {
	for _, item := range d.Detail.Timeline {
		if item.Kind == gh.TimelineReview && item.Said().ID == id {
			return item, true
		}
	}
	return gh.TimelineItem{}, false
}

func TestASubmittedReviewReadsAsPublishedBeforeItLands(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", withPendingReview())

	s.PendingReview("PR_1", "PRR_1", gh.ReviewEventApprove, "Two things.", viewer, submittedAt)
	d := s.Detail("PR_1")

	if d.Detail.DraftReview != (gh.Review{}) {
		t.Errorf("DraftReview = %+v, want it gone once submitted", d.Detail.DraftReview)
	}
	item, ok := reviewItem(d, "PRR_1")
	if !ok {
		t.Fatal("the submitted review is not in the timeline")
	}
	if item.Review != gh.ReviewStateApproved || !item.Said().Editing {
		t.Errorf("review = %+v, want an approval still saving", item)
	}
	for _, thread := range d.Detail.Threads {
		for _, c := range thread.Comments {
			if c.Draft {
				t.Errorf("comment %s is still a draft after the submit", c.ID)
			}
		}
	}
	if got := d.Detail.Reviewers[0]; got.State != gh.ReviewStateApproved || got.Requested {
		t.Errorf("viewer's row = %+v, want approved and no longer requested", got)
	}
}

func TestARefetchThatAlreadyHasTheReviewShowsItOnce(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", withPendingReview())
	s.PendingReview("PR_1", "PRR_1", gh.ReviewEventApprove, "Two things.", viewer, submittedAt)

	fetched := withPendingReview()
	fetched.Detail.DraftReview = gh.Review{}
	fetched.Detail.Timeline = []gh.TimelineItem{{
		Kind: gh.TimelineReview, Review: gh.ReviewStateApproved,
		Comment: &gh.Comment{Kind: gh.CommentReview, ID: "PRR_1", Author: viewer},
	}}
	s.DetailApplied("PR_1", fetched)

	n := 0
	for _, item := range s.Detail("PR_1").Detail.Timeline {
		if item.Kind == gh.TimelineReview {
			n++
		}
	}
	if n != 1 {
		t.Errorf("the timeline holds %d reviews, want the submitted one once", n)
	}
}

func TestAReviewWithNothingPendingOpensItsOwnItem(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", gh.DetailResult{})

	key := s.PendingReview("PR_1", "", gh.ReviewEventComment, "Looks fine.", viewer, submittedAt)

	if _, ok := reviewItem(s.Detail("PR_1"), key); !ok {
		t.Fatal("the review is not in the timeline under its key")
	}
	if got := s.Detail("PR_1").Detail.Reviewers; len(got) != 1 || got[0].State != gh.ReviewStateCommented {
		t.Errorf("reviewers = %+v, want the viewer as having commented", got)
	}
}

func TestALandedReviewTakesGitHubsIDAndMarksTheFetchStale(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", gh.DetailResult{})
	if !s.BeginDetail("PR_1") {
		t.Fatal("BeginDetail refused a detail that is not loading")
	}

	key := s.PendingReview("PR_1", "", gh.ReviewEventRequestChanges, "Two things.", viewer, submittedAt)
	s.ReviewApplied("PR_1", key, gh.ReviewResult{Review: gh.Review{
		ID: "PRR_9", State: gh.ReviewStateChangesRequested, Body: "Two things.",
	}})

	item, ok := reviewItem(s.Detail("PR_1"), "PRR_9")
	if !ok {
		t.Fatal("the landed review is not in the timeline under GitHub's id")
	}
	if item.Said().Editing {
		t.Error("the landed review still reads as saving")
	}
	if _, ok := reviewItem(s.Detail("PR_1"), key); ok {
		t.Error("the optimistic item outlived the answer")
	}
	if !s.StaleDetail("PR_1") {
		t.Error("the fetch in flight is not marked stale")
	}
}

func TestARevertedReviewPutsThePendingOneBack(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", withPendingReview())

	key := s.PendingReview("PR_1", "PRR_1", gh.ReviewEventApprove, "", viewer, submittedAt)
	_ = s.Detail("PR_1")
	s.EditRevertedStale("PR_1", key)

	d := s.Detail("PR_1").Detail
	if d.DraftReview.ID != "PRR_1" {
		t.Errorf("DraftReview = %+v, want the pending review back", d.DraftReview)
	}
	if !d.Threads[0].Draft || !d.Threads[0].Comments[0].Draft || !d.Threads[1].Comments[1].Draft {
		t.Error("a draft came back published: the submit wrote into the fetched detail")
	}
}

func TestADiscardTakesTheDraftsWithIt(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", withPendingReview())

	key := s.PendingDiscard("PR_1", "PRR_1")
	d := s.Detail("PR_1").Detail

	if d.DraftReview != (gh.Review{}) {
		t.Errorf("DraftReview = %+v, want it gone", d.DraftReview)
	}
	if len(d.Threads) != 1 || d.Threads[0].ID != "RT_2" {
		t.Fatalf("threads = %+v, want the published one alone", d.Threads)
	}
	if got := d.Threads[0].Comments; len(got) != 1 || got[0].ID != "RC_2" {
		t.Errorf("comments = %+v, want the unsent reply gone", got)
	}

	s.EditReverted("PR_1", key)
	if got := s.Detail("PR_1").Detail; got.DraftReview.ID != "PRR_1" || len(got.Threads) != 2 {
		t.Errorf("a reverted discard left %+v", got)
	}
}
