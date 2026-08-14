package store_test

import (
	"testing"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/store"
)

// reactedDetail is a timeline comment, a thread comment and a description, each
// carrying reactions somebody else gave. Three subjects because a reaction
// reaches three different places and the fold takes a different route to each.
func reactedDetail() gh.DetailResult {
	c := gh.Comment{
		Kind: gh.CommentIssue, ID: "IC_1", Body: "first",
		Reactions: []gh.Reaction{{Content: gh.ReactionThumbsUp, Count: 2}},
	}
	return gh.DetailResult{Detail: gh.PullRequestDetail{
		Body:      "the description",
		Reactions: []gh.Reaction{{Content: gh.ReactionHeart, Count: 1}},
		Timeline:  []gh.TimelineItem{{Kind: gh.TimelineComment, Comment: &c}},
		Threads: []gh.ReviewThread{{ID: "RT_1", CanReply: true, Comments: []gh.Comment{{
			Kind: gh.CommentThread, ID: "RC_1", Body: "asked",
			Reactions: []gh.Reaction{{Content: gh.ReactionEyes, Count: 3, Viewer: true}},
		}}}},
	}}
}

// reactionOn is the counts a subject carries, keyed by content, as the store
// currently folds them.
func reactionOn(rs []gh.Reaction, c gh.ReactionContent) (gh.Reaction, bool) {
	for _, r := range rs {
		if r.Content == c {
			return r, true
		}
	}
	return gh.Reaction{}, false
}

// The pill moves before GitHub has seen it. That is the whole of what
// optimistic means here.
func TestAReactionRendersBeforeItLands(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", reactedDetail())

	s.PendingReaction("PR_1", "IC_1", "", gh.ReactionThumbsUp, true)

	got, ok := reactionOn(s.Detail("PR_1").Detail.Timeline[0].Said().Reactions, gh.ReactionThumbsUp)
	if !ok || got.Count != 3 || !got.Viewer {
		t.Errorf("reaction = %+v, want three with the viewer in it", got)
	}
	if !got.Pending {
		t.Error("the reaction is not marked as being written")
	}
}

// The three subjects are three routes through the fold. A reaction on the
// description never reaches a comment at all: it is a field of the pull request.
func TestAReactionReachesEverySubject(t *testing.T) {
	for _, tt := range []struct {
		name      string
		commentID string
		threadID  string
		content   gh.ReactionContent
		read      func(store.Detail) []gh.Reaction
	}{
		{
			name: "the description", content: gh.ReactionHeart,
			read: func(d store.Detail) []gh.Reaction { return d.Detail.Reactions },
		},
		{
			name: "a timeline comment", commentID: "IC_1", content: gh.ReactionThumbsUp,
			read: func(d store.Detail) []gh.Reaction { return d.Detail.Timeline[0].Said().Reactions },
		},
		{
			name: "a thread comment", commentID: "RC_1", threadID: "RT_1", content: gh.ReactionEyes,
			read: func(d store.Detail) []gh.Reaction { return d.Detail.Threads[0].Comments[0].Reactions },
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := store.New(configured())
			s.DetailApplied("PR_1", reactedDetail())

			before, _ := reactionOn(tt.read(s.Detail("PR_1")), tt.content)
			s.PendingReaction("PR_1", tt.commentID, tt.threadID, tt.content, !before.Viewer)

			after, _ := reactionOn(tt.read(s.Detail("PR_1")), tt.content)
			if after.Viewer == before.Viewer {
				t.Errorf("viewer = %v both before and after the toggle", after.Viewer)
			}
			if !after.Pending {
				t.Error("the reaction is not marked as being written")
			}
		})
	}
}

// A reaction given and then taken back leaves a group at zero, and it stays on
// the list while the write is out. It is the only thing a second press can
// read, and the key is what goes inert on it.
func TestAReactionTakenBackStaysWhileTheWriteIsOut(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", reactedDetail())

	s.PendingReaction("PR_1", "RC_1", "RT_1", gh.ReactionEyes, false)

	got, ok := reactionOn(s.Detail("PR_1").Detail.Threads[0].Comments[0].Reactions, gh.ReactionEyes)
	if !ok {
		t.Fatal("the reaction went off the card with the write still out")
	}
	if got.Count != 2 || got.Viewer || !got.Pending {
		t.Errorf("reaction = %+v, want two, the viewer out of it, and pending", got)
	}
}

// A reaction nobody has given yet lands in GitHub's own order, not on the end.
// Appending would move the pill sideways the moment the answer arrived.
func TestANewReactionLandsInGitHubsOrder(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", reactedDetail())

	// THUMBS_UP is already there and LAUGH sorts between it and nothing else.
	s.PendingReaction("PR_1", "IC_1", "", gh.ReactionLaugh, true)

	got := s.Detail("PR_1").Detail.Timeline[0].Said().Reactions
	if len(got) != 2 || got[0].Content != gh.ReactionThumbsUp || got[1].Content != gh.ReactionLaugh {
		t.Errorf("reactions = %+v, want thumbs up then laugh", got)
	}
}

// The reason a reaction is held beside the detail rather than written into it.
// A refetch that answers before the mutation does must not undo the pill.
func TestARefetchDoesNotUndoAReactionStillOut(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", reactedDetail())
	s.PendingReaction("PR_1", "IC_1", "", gh.ReactionThumbsUp, true)

	s.DetailApplied("PR_1", reactedDetail())

	got, _ := reactionOn(s.Detail("PR_1").Detail.Timeline[0].Said().Reactions, gh.ReactionThumbsUp)
	if got.Count != 3 {
		t.Errorf("count = %d, want the pending reaction still folded in", got.Count)
	}
}

// The fold hands out a detail. A timeline item holds a pointer to its comment,
// so writing through it moves the pill inside the held one and the revert then
// has nothing to put back.
//
// The read before the revert is what makes this a test. Reading twice proves
// nothing: the toggle is idempotent, so a second fold over an already-flipped
// comment leaves it exactly where the first one did.
func TestFoldingAReactionLeavesTheHeldDetailAlone(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", reactedDetail())

	key := s.PendingReaction("PR_1", "IC_1", "", gh.ReactionThumbsUp, true)
	_ = s.Detail("PR_1")
	s.ReactionReverted("PR_1", key)

	got, _ := reactionOn(s.Detail("PR_1").Detail.Timeline[0].Said().Reactions, gh.ReactionThumbsUp)
	if got.Count != 2 || got.Viewer {
		t.Errorf("reaction = %+v, want the fetched two: the fold wrote into the held detail", got)
	}
}

// Same question one level down, where the clone has to reach the thread's own
// comment slice as well as the slice of threads.
func TestFoldingAThreadReactionLeavesTheHeldDetailAlone(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", reactedDetail())

	key := s.PendingReaction("PR_1", "RC_1", "RT_1", gh.ReactionEyes, false)
	_ = s.Detail("PR_1")
	s.ReactionReverted("PR_1", key)

	got, _ := reactionOn(s.Detail("PR_1").Detail.Threads[0].Comments[0].Reactions, gh.ReactionEyes)
	if got.Count != 3 || !got.Viewer {
		t.Errorf("reaction = %+v, want the fetched three: the fold wrote into the held detail", got)
	}
}

// A settle takes the count GitHub reported for the reaction the write moved,
// and leaves every other group where it was.
func TestASettledReactionTakesGitHubsCount(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", reactedDetail())
	key := s.PendingReaction("PR_1", "IC_1", "", gh.ReactionThumbsUp, true)

	s.ReactionApplied("PR_1", key, gh.ReactionResult{Reactions: []gh.Reaction{
		{Content: gh.ReactionThumbsUp, Count: 9, Viewer: true},
	}})

	got, ok := reactionOn(s.Detail("PR_1").Detail.Timeline[0].Said().Reactions, gh.ReactionThumbsUp)
	if !ok || got.Count != 9 || !got.Viewer {
		t.Errorf("reaction = %+v, want the nine GitHub reported", got)
	}
	if got.Pending {
		t.Error("a settled reaction is still marked as being written")
	}
}

// Two toggles on one subject answer in whatever order the network gives them,
// and each response is a snapshot of the subject as GitHub had it at the time.
// Taking either one whole lets the older snapshot land last and delete a
// reaction the other one added.
func TestTwoReactionsSettlingOutOfOrderKeepBoth(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", reactedDetail())

	up := s.PendingReaction("PR_1", "IC_1", "", gh.ReactionThumbsUp, true)
	rocket := s.PendingReaction("PR_1", "IC_1", "", gh.ReactionRocket, true)

	// The rocket answers first, from a subject GitHub had already given the
	// thumbs up. The thumbs up answers second, from before the rocket existed.
	s.ReactionApplied("PR_1", rocket, gh.ReactionResult{Reactions: []gh.Reaction{
		{Content: gh.ReactionThumbsUp, Count: 3, Viewer: true},
		{Content: gh.ReactionRocket, Count: 1, Viewer: true},
	}})
	s.ReactionApplied("PR_1", up, gh.ReactionResult{Reactions: []gh.Reaction{
		{Content: gh.ReactionThumbsUp, Count: 3, Viewer: true},
	}})

	got := s.Detail("PR_1").Detail.Timeline[0].Said().Reactions
	if len(got) != 2 {
		t.Fatalf("reactions = %+v, want both of them", got)
	}
	if got[0].Content != gh.ReactionThumbsUp || got[1].Content != gh.ReactionRocket {
		t.Errorf("reactions = %+v, want thumbs up then rocket", got)
	}
}

// Taking the last reaction off leaves a subject with none, and GitHub answers
// with nothing for it. That is the write having worked.
func TestSettlingTheLastReactionTakesThePillOff(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", reactedDetail())
	key := s.PendingReaction("PR_1", "RC_1", "RT_1", gh.ReactionEyes, false)

	s.ReactionApplied("PR_1", key, gh.ReactionResult{Reactions: []gh.Reaction{
		{Content: gh.ReactionEyes, Count: 2},
	}})

	got, ok := reactionOn(s.Detail("PR_1").Detail.Threads[0].Comments[0].Reactions, gh.ReactionEyes)
	if !ok || got.Count != 2 || got.Viewer || got.Pending {
		t.Errorf("reaction = %+v, want the two GitHub kept, without the viewer", got)
	}

	// And the same again where the viewer was the only one in it.
	s.DetailApplied("PR_2", reactedDetail())
	last := s.PendingReaction("PR_2", "RC_1", "RT_1", gh.ReactionEyes, false)
	s.ReactionApplied("PR_2", last, gh.ReactionResult{})

	if got := s.Detail("PR_2").Detail.Threads[0].Comments[0].Reactions; len(got) != 0 {
		t.Errorf("reactions = %+v, want the pill off the card", got)
	}
}

// The revert branch. Nothing was typed and no words are at stake, so the pill
// going back is the whole of it.
func TestARefusedReactionGoesBack(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", reactedDetail())
	key := s.PendingReaction("PR_1", "IC_1", "", gh.ReactionThumbsUp, true)

	s.ReactionReverted("PR_1", key)

	got, _ := reactionOn(s.Detail("PR_1").Detail.Timeline[0].Said().Reactions, gh.ReactionThumbsUp)
	if got.Count != 2 || got.Viewer || got.Pending {
		t.Errorf("reaction = %+v, want the two it was fetched with", got)
	}
}

// A refetch landing while the write is out may no longer carry the comment.
// There is nothing honest to invent, so the write waits out of sight.
func TestAReactionOnAVanishedCommentIsSkipped(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", reactedDetail())
	s.PendingReaction("PR_1", "IC_gone", "", gh.ReactionThumbsUp, true)

	got := s.Detail("PR_1").Detail.Timeline[0].Said().Reactions
	if len(got) != 1 || got[0].Count != 2 {
		t.Errorf("reactions = %+v, want the fetched card untouched", got)
	}
}

// A fetch asked for before the write settled was answered from the reactions
// before it, so taking it would put the pill back where it was.
func TestASettledReactionMarksAFetchInFlightStale(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", reactedDetail())
	key := s.PendingReaction("PR_1", "IC_1", "", gh.ReactionThumbsUp, true)
	s.BeginDetail("PR_1")

	s.ReactionApplied("PR_1", key, gh.ReactionResult{Reactions: []gh.Reaction{
		{Content: gh.ReactionThumbsUp, Count: 3, Viewer: true},
	}})

	if !s.StaleDetail("PR_1") {
		t.Error("the fetch in flight was not marked stale")
	}
}
