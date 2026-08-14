package gh

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// GitHub answers with all eight groups on every subject it has ever been asked
// about, nearly all at zero. Returning them whole puts a row of eight empty
// pills under every comment on the page.
func TestOnlyTheReactionsSomebodyGaveComeBack(t *testing.T) {
	groups := []reactionGroup{
		{Content: ReactionThumbsUp, ViewerHasReacted: true},
		{Content: ReactionThumbsDown},
		{Content: ReactionLaugh},
		{Content: ReactionHooray},
		{Content: ReactionConfused},
		{Content: ReactionHeart},
		{Content: ReactionRocket},
		{Content: ReactionEyes},
	}
	groups[0].Reactors.TotalCount = 3
	groups[6].Reactors.TotalCount = 1

	want := []Reaction{
		{Content: ReactionThumbsUp, Count: 3, Viewer: true},
		{Content: ReactionRocket, Count: 1},
	}
	if got := reactions(groups); !reflect.DeepEqual(got, want) {
		t.Errorf("reactions = %+v, want %+v", got, want)
	}
}

// A subject nobody has reacted to still answers with eight groups. Nil is what
// says there is nothing to draw; an empty non-nil slice reads the same to the
// card and differently to every test comparing sets.
func TestASubjectWithNoReactionsComesBackWithNone(t *testing.T) {
	groups := make([]reactionGroup, len(ReactionOrder))
	for i, c := range ReactionOrder {
		groups[i].Content = c
	}
	if got := reactions(groups); got != nil {
		t.Errorf("reactions = %+v, want nil", got)
	}
}

const addedReaction = `{
  "addReaction": {
    "reactionGroups": [
      {"content": "THUMBS_UP", "viewerHasReacted": true, "reactors": {"totalCount": 4}},
      {"content": "EYES", "viewerHasReacted": false, "reactors": {"totalCount": 0}}
    ]
  }
}`

const removedReaction = `{
  "removeReaction": {
    "reactionGroups": [
      {"content": "THUMBS_UP", "viewerHasReacted": false, "reactors": {"totalCount": 3}}
    ]
  }
}`

// The direction picks the document. Sending the add where a remove was meant
// puts the reaction back on the card the reader just took it off.
func TestTheReactionDirectionPicksTheMutation(t *testing.T) {
	for _, tt := range []struct {
		name    string
		on      bool
		body    string
		mutates string
		want    []Reaction
	}{
		{
			name:    "adding",
			on:      true,
			body:    addedReaction,
			mutates: "addReaction",
			want:    []Reaction{{Content: ReactionThumbsUp, Count: 4, Viewer: true}},
		},
		{
			name:    "removing",
			body:    removedReaction,
			mutates: "removeReaction",
			want:    []Reaction{{Content: ReactionThumbsUp, Count: 3}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			doer := &fakeDoer{body: tt.body}

			res, err := newWithDoer(doer, nil).SetReaction(
				context.Background(), "IC_1", ReactionThumbsUp, tt.on)
			if err != nil {
				t.Fatalf("SetReaction: %v", err)
			}

			if !strings.Contains(doer.gotQuery, tt.mutates+"(input:") {
				t.Errorf("the document does not call %s", tt.mutates)
			}
			if got := doer.gotVars["subjectId"]; got != "IC_1" {
				t.Errorf("subjectId = %v, want IC_1", got)
			}
			if got := doer.gotVars["content"]; got != "THUMBS_UP" {
				t.Errorf("content = %v, want THUMBS_UP", got)
			}
			if !reflect.DeepEqual(res.Reactions, tt.want) {
				t.Errorf("Reactions = %+v, want %+v", res.Reactions, tt.want)
			}
		})
	}
}

// Both payloads decode into one struct, and reading whichever came back filled
// would let an add answered by a remove pass silently.
func TestEachDirectionReadsItsOwnHalfOfThePayload(t *testing.T) {
	if _, err := newWithDoer(&fakeDoer{body: removedReaction}, nil).SetReaction(
		context.Background(), "IC_1", ReactionThumbsUp, true); err == nil {
		t.Error("an add answered by a remove payload came back as a success")
	}
}

// Taking the last reaction off a subject leaves it with none, and GitHub can
// answer with an empty list for it. That is the write having worked, and
// reporting it as a failure would toast an error and put the pill back on a
// card GitHub had already cleared.
func TestARemovalThatEmptiesASubjectIsNotAFailure(t *testing.T) {
	doer := &fakeDoer{body: `{"removeReaction": {"reactionGroups": []}}`}

	res, err := newWithDoer(doer, nil).SetReaction(
		context.Background(), "IC_1", ReactionThumbsUp, false)
	if err != nil {
		t.Fatalf("SetReaction: %v", err)
	}
	if len(res.Reactions) != 0 {
		t.Errorf("Reactions = %+v, want none", res.Reactions)
	}
}

func TestAFailedReactionSaysWhichDirectionItWas(t *testing.T) {
	doer := &fakeDoer{err: errors.New("boom")}

	_, err := newWithDoer(doer, nil).SetReaction(
		context.Background(), "IC_1", ReactionHeart, false)
	if err == nil || !strings.Contains(err.Error(), "removing a reaction") {
		t.Errorf("err = %v, want it to name removing a reaction", err)
	}
}

// A reaction to the description is addressed to the pull request, so the
// mutation input takes an ID and not a comment id. The variable is the whole of
// what says so.
func TestAReactionTakesWhateverNodeItIsGiven(t *testing.T) {
	doer := &fakeDoer{body: addedReaction}

	if _, err := newWithDoer(doer, nil).SetReaction(
		context.Background(), "PR_1", ReactionThumbsUp, true); err != nil {
		t.Fatalf("SetReaction: %v", err)
	}
	if got := doer.gotVars["subjectId"]; got != "PR_1" {
		t.Errorf("subjectId = %v, want PR_1", got)
	}
}
