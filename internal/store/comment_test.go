package store_test

import (
	"testing"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
)

// An edit shows before GitHub has seen it, which is what optimistic means, and
// says it has not landed.
func TestAnEditedCommentRendersBeforeItLands(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", detailWith("typo"))

	s.PendingCommentEdit("PR_1", "IC_typo", "", "fixed")

	if got := bodies(s.Detail("PR_1")); len(got) != 1 || got[0] != "fixed" {
		t.Errorf("timeline = %q, want the new body", got)
	}
	if !s.Detail("PR_1").Detail.Timeline[0].Said().Editing {
		t.Error("the edited comment is not marked as being written")
	}
}

func TestAnEditedThreadCommentRendersBeforeItLands(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "typo", "second"))

	s.PendingCommentEdit("PR_1", "RC_typo", "RT_1", "fixed")

	if got := replies(s.Detail("PR_1"), "RT_1"); len(got) != 2 || got[0] != "fixed" {
		t.Errorf("thread = %q, want the first comment rewritten", got)
	}
	if !s.Detail("PR_1").Detail.Threads[0].Comments[0].Editing {
		t.Error("the edited comment is not marked as being written")
	}
}

// The card comes off the page at once. There is no placeholder for a delete:
// the gap is the acknowledgement.
func TestADeletedCommentComesOffThePageBeforeItLands(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", detailWith("first", "second"))

	s.PendingCommentDelete("PR_1", "IC_first", "")

	if got := bodies(s.Detail("PR_1")); len(got) != 1 || got[0] != "second" {
		t.Errorf("timeline = %q, want the first comment gone", got)
	}
}

// GitHub drops a thread whose last comment goes, and an empty card would say a
// discussion is still there with nothing in it.
func TestDeletingTheLastCommentTakesTheThreadWithIt(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "only"))

	s.PendingCommentDelete("PR_1", "RC_only", "RT_1")

	if got := s.Detail("PR_1").Detail.Threads; len(got) != 0 {
		t.Errorf("threads = %+v, want the thread gone with its last comment", got)
	}
}

func TestDeletingOneCommentLeavesTheThreadStanding(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "asked", "answered"))

	s.PendingCommentDelete("PR_1", "RC_asked", "RT_1")

	if got := replies(s.Detail("PR_1"), "RT_1"); len(got) != 1 || got[0] != "answered" {
		t.Errorf("thread = %q, want the other comment still there", got)
	}
}

// The aliasing rule the clones are there for. The timeline item holds a pointer
// to its comment, so an edit written through it reaches the detail already
// handed out and rendered.
func TestFoldingAnEditDoesNotWriteIntoADetailAlreadyHandedOut(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", detailWith("typo"))
	held := s.Detail("PR_1")

	s.PendingCommentEdit("PR_1", "IC_typo", "", "fixed")
	_ = s.Detail("PR_1")

	if got := bodies(held); got[0] != "typo" {
		t.Errorf("the detail already handed out now reads %q, want it unchanged", got)
	}
}

func TestFoldingAThreadEditDoesNotWriteIntoADetailAlreadyHandedOut(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "typo", "second"))
	held := s.Detail("PR_1")

	s.PendingCommentEdit("PR_1", "RC_typo", "RT_1", "fixed")
	_ = s.Detail("PR_1")

	if got := replies(held, "RT_1"); got[0] != "typo" {
		t.Errorf("the detail already handed out now reads %q, want it unchanged", got)
	}
}

// Reading twice gives the same answer. A delete folded into the held slice
// would take another comment off on every call.
func TestReadingADetailTwiceFoldsTheSameDelete(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", detailWith("first", "second", "third"))
	s.PendingCommentDelete("PR_1", "IC_second", "")

	_ = s.Detail("PR_1")
	if got := bodies(s.Detail("PR_1")); len(got) != 2 {
		t.Errorf("timeline = %q on the second read, want two comments", got)
	}
}

// A refetch that landed while the write was out may not carry the comment at
// all. There is nothing to rewrite and nothing honest to invent.
func TestAnEditOfACommentTheRefetchDroppedIsSkipped(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", detailWith("typo"))
	s.PendingCommentEdit("PR_1", "IC_typo", "", "fixed")

	s.DetailApplied("PR_1", detailWith("something else"))

	if got := bodies(s.Detail("PR_1")); len(got) != 1 || got[0] != "something else" {
		t.Errorf("timeline = %q, want the refetch left alone", got)
	}
}

func TestAnEditedCommentTakesGitHubsAnswer(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", detailWith("typo"))
	key := s.PendingCommentEdit("PR_1", "IC_typo", "", "fixed")

	s.CommentEditApplied("PR_1", key, gh.CommentResult{
		Comment: gh.Comment{Kind: gh.CommentIssue, ID: "IC_typo", Body: "fixed by GitHub"},
	})

	d := s.Detail("PR_1")
	if got := bodies(d); len(got) != 1 || got[0] != "fixed by GitHub" {
		t.Fatalf("timeline = %q, want what GitHub recorded", got)
	}
	// The write is gone, so nothing is still claiming to be in flight.
	if d.Detail.Timeline[0].Said().Editing {
		t.Error("the settled comment still reads as being written")
	}
}

func TestAnEditedThreadCommentTakesGitHubsAnswer(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "typo", "second"))
	key := s.PendingCommentEdit("PR_1", "RC_typo", "RT_1", "fixed")

	s.CommentEditApplied("PR_1", key, gh.CommentResult{
		Comment: gh.Comment{Kind: gh.CommentThread, ID: "RC_typo", Body: "fixed by GitHub"},
	})

	d := s.Detail("PR_1")
	if got := replies(d, "RT_1"); len(got) != 2 || got[0] != "fixed by GitHub" {
		t.Fatalf("thread = %q, want what GitHub recorded", got)
	}
	if d.Detail.Threads[0].Comments[0].Editing {
		t.Error("the settled comment still reads as being written")
	}
}

// The settle writes the removal into the held detail. Dropping the write alone
// would put the card back until something refetched.
func TestASettledDeleteKeepsTheCommentOff(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", detailWith("first", "second"))
	key := s.PendingCommentDelete("PR_1", "IC_first", "")

	s.CommentDeleteApplied("PR_1", key)

	if got := bodies(s.Detail("PR_1")); len(got) != 1 || got[0] != "second" {
		t.Errorf("timeline = %q, want the comment still gone", got)
	}
}

func TestASettledThreadDeleteKeepsTheCommentOff(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", threadWith("RT_1", "asked", "answered"))
	key := s.PendingCommentDelete("PR_1", "RC_asked", "RT_1")

	s.CommentDeleteApplied("PR_1", key)

	if got := replies(s.Detail("PR_1"), "RT_1"); len(got) != 1 || got[0] != "answered" {
		t.Errorf("thread = %q, want the comment still gone", got)
	}
}

func TestAFailedEditPutsTheOldBodyBack(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", detailWith("typo"))
	key := s.PendingCommentEdit("PR_1", "IC_typo", "", "fixed")

	s.CommentWriteReverted("PR_1", key)

	if got := bodies(s.Detail("PR_1")); len(got) != 1 || got[0] != "typo" {
		t.Errorf("timeline = %q, want the fetched body back", got)
	}
}

func TestAFailedDeletePutsTheCommentBack(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", detailWith("first", "second"))
	key := s.PendingCommentDelete("PR_1", "IC_first", "")

	s.CommentWriteReverted("PR_1", key)

	if got := bodies(s.Detail("PR_1")); len(got) != 2 || got[0] != "first" {
		t.Errorf("timeline = %q, want the comment back where it was", got)
	}
}

// A response for a key already gone is one that settled already. Applying it
// again would take a second comment off the page.
func TestAResponseForACommentWriteAlreadySettledIsIgnored(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", detailWith("first", "second"))
	key := s.PendingCommentDelete("PR_1", "IC_first", "")

	s.CommentDeleteApplied("PR_1", key)
	s.CommentDeleteApplied("PR_1", key)

	if got := bodies(s.Detail("PR_1")); len(got) != 1 {
		t.Errorf("timeline = %q, want one comment left", got)
	}
}

// An edit and a comment being posted are two writes with one counter behind
// them, and each settles on its own key.
func TestAnEditAndAPostInFlightSettleSeparately(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", detailWith("typo"))

	edit := s.PendingCommentEdit("PR_1", "IC_typo", "", "fixed")
	post := s.PendingComment("PR_1", gh.Comment{Kind: gh.CommentIssue, Body: "new one"})
	if edit == post {
		t.Fatal("two writes in flight took the same key")
	}

	s.CommentWriteReverted("PR_1", edit)

	if got := bodies(s.Detail("PR_1")); len(got) != 2 || got[0] != "typo" || got[1] != "new one" {
		t.Errorf("timeline = %q, want the edit reverted and the post standing", got)
	}
}

// The description is a field of the pull request rather than a comment, so it
// goes through the edit queue the labels and the base go through.
func TestAnEditedDescriptionRendersBeforeItLands(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", gh.DetailResult{Detail: gh.PullRequestDetail{Body: "old"}})

	key := s.PendingBody("PR_1", "new")
	if got := s.Detail("PR_1").Detail.Body; got != "new" {
		t.Errorf("Body = %q, want the new description", got)
	}

	s.BodyApplied("PR_1", key, gh.BodyResult{Body: "new, as GitHub kept it"})
	if got := s.Detail("PR_1").Detail.Body; got != "new, as GitHub kept it" {
		t.Errorf("Body = %q, want what GitHub recorded", got)
	}
}

func TestAFailedDescriptionEditPutsTheOldTextBack(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", gh.DetailResult{Detail: gh.PullRequestDetail{Body: "old"}})

	key := s.PendingBody("PR_1", "new")
	s.EditReverted("PR_1", key)

	if got := s.Detail("PR_1").Detail.Body; got != "old" {
		t.Errorf("Body = %q, want the fetched description back", got)
	}
}
