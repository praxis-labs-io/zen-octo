package store_test

import (
	"testing"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
)

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

func reactionOn(rs []gh.Reaction, c gh.ReactionContent) (gh.Reaction, bool) {
	for _, r := range rs {
		if r.Content == c {
			return r, true
		}
	}
	return gh.Reaction{}, false
}

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

func TestANewReactionLandsInGitHubsOrder(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", reactedDetail())

	s.PendingReaction("PR_1", "IC_1", "", gh.ReactionLaugh, true)

	got := s.Detail("PR_1").Detail.Timeline[0].Said().Reactions
	if len(got) != 2 || got[0].Content != gh.ReactionThumbsUp || got[1].Content != gh.ReactionLaugh {
		t.Errorf("reactions = %+v, want thumbs up then laugh", got)
	}
}

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

func TestTwoReactionsSettlingOutOfOrderKeepBoth(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", reactedDetail())

	up := s.PendingReaction("PR_1", "IC_1", "", gh.ReactionThumbsUp, true)
	rocket := s.PendingReaction("PR_1", "IC_1", "", gh.ReactionRocket, true)

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

	s.DetailApplied("PR_2", reactedDetail())
	last := s.PendingReaction("PR_2", "RC_1", "RT_1", gh.ReactionEyes, false)
	s.ReactionApplied("PR_2", last, gh.ReactionResult{})

	if got := s.Detail("PR_2").Detail.Threads[0].Comments[0].Reactions; len(got) != 0 {
		t.Errorf("reactions = %+v, want the pill off the card", got)
	}
}

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

func TestAReactionOnAVanishedCommentIsSkipped(t *testing.T) {
	s := store.New(configured())
	s.DetailApplied("PR_1", reactedDetail())
	s.PendingReaction("PR_1", "IC_gone", "", gh.ReactionThumbsUp, true)

	got := s.Detail("PR_1").Detail.Timeline[0].Said().Reactions
	if len(got) != 1 || got[0].Count != 2 {
		t.Errorf("reactions = %+v, want the fetched card untouched", got)
	}
}

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
