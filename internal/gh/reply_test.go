package gh

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

const repliedBody = `{
  "addPullRequestReviewThreadReply": {
    "comment": {
      "id": "PRRC_NEW",
      "createdAt": "2026-08-09T12:00:00Z",
      "body": "Capped it at 30s.",
      "author": {"login": "drucial"},
      "viewerDidAuthor": true,
      "viewerCanUpdate": true,
      "viewerCanDelete": true,
      "viewerCanReact": true,
      "reactionGroups": [
        {"content": "HEART", "viewerHasReacted": true, "reactors": {"totalCount": 2}}
      ]
    }
  }
}`

// A reply is a thread comment, not an issue comment. The kind is what picks the
// mutation that edits it later, so getting it wrong here is a delete that fails
// three tickets from now.
func TestARepliedCommentComesBackAsAThreadComment(t *testing.T) {
	doer := &fakeDoer{body: repliedBody}

	res, err := newWithDoer(doer, nil).AddReply(context.Background(), "PRRT_1", "Capped it at 30s.")
	if err != nil {
		t.Fatalf("AddReply: %v", err)
	}

	want := Comment{
		Kind:            CommentThread,
		ID:              "PRRC_NEW",
		Author:          Actor{Login: "drucial"},
		CreatedAt:       time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC),
		Body:            "Capped it at 30s.",
		ViewerDidAuthor: true,
		CanEdit:         true,
		CanDelete:       true,
		CanReact:        true,
		Reactions:       []Reaction{{Content: ReactionHeart, Count: 2, Viewer: true}},
	}
	if !reflect.DeepEqual(res.Comment, want) {
		t.Errorf("Comment = %+v, want %+v", res.Comment, want)
	}

	if res.Comment.Pending {
		t.Error("a reply GitHub confirmed came back marked pending")
	}
}

func TestTheReplySendsTheThreadAndBodyAsVariables(t *testing.T) {
	doer := &fakeDoer{body: repliedBody}

	if _, err := newWithDoer(doer, nil).AddReply(context.Background(), "PRRT_1", "``` fenced ```"); err != nil {
		t.Fatalf("AddReply: %v", err)
	}

	if got := doer.gotVars["threadId"]; got != "PRRT_1" {
		t.Errorf("threadId = %v, want PRRT_1", got)
	}
	if got := doer.gotVars["body"]; got != "``` fenced ```" {
		t.Errorf("body = %v, want it passed through untouched", got)
	}
	if strings.Contains(doer.gotQuery, "fenced") {
		t.Error("the body was written into the document instead of sent as a variable")
	}
}

// The same trap as the comment mutation: a field the document never asks for
// decodes to a zero value from canned JSON, so every other test here stays green
// while the field is dead.
func TestTheReplyMutationAsksForWhatTheViewerMayDoNext(t *testing.T) {
	for _, want := range []string{
		"id", "createdAt", "body", "author { login }",
		"viewerDidAuthor", "viewerCanUpdate", "viewerCanDelete", "viewerCanReact",
	} {
		if !strings.Contains(addReplyMutation, want) {
			t.Errorf("the mutation does not ask for %q", want)
		}
	}

	if strings.Contains(addReplyMutation, "rateLimit") {
		t.Error("the mutation asks for rateLimit, which does not exist on Mutation")
	}
}

// The reply goes to the thread, not to the pull request. addComment takes a
// Commentable and a review thread is not one, so a mutation that drifted back to
// the wrong input would post the reply as a loose comment.
func TestTheReplyIsAddressedToTheThread(t *testing.T) {
	if !strings.Contains(addReplyMutation, "pullRequestReviewThreadId: $threadId") {
		t.Error("the mutation does not address the thread")
	}
}

func TestAReplyThatReturnsNoCommentIsAnError(t *testing.T) {
	_, err := newWithDoer(&fakeDoer{body: `{"addPullRequestReviewThreadReply": {"comment": {}}}`}, nil).
		AddReply(context.Background(), "PRRT_1", "hi")
	if err == nil {
		t.Fatal("an empty node came back as a posted reply")
	}
	if !strings.Contains(err.Error(), "returned no comment") {
		t.Errorf("error = %q, want it to say nothing came back", err)
	}
}

func TestAFailedReplySaysWhatItWasDoing(t *testing.T) {
	sunk := errors.New("network is down")

	_, err := newWithDoer(&fakeDoer{err: sunk}, nil).AddReply(context.Background(), "PRRT_1", "hi")
	if !errors.Is(err, sunk) {
		t.Fatalf("error = %v, want it to wrap %v", err, sunk)
	}
	if !strings.Contains(err.Error(), "posting a reply") {
		t.Errorf("error = %q, want it to name the call", err)
	}
}
