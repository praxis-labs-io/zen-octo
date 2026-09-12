package gh

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

const postedBody = `{
  "addComment": {
    "commentEdge": {
      "node": {
        "id": "IC_NEW",
        "createdAt": "2026-08-09T12:00:00Z",
        "body": "Looks right to me.",
        "author": {"login": "drucial"},
        "viewerDidAuthor": true,
        "viewerCanUpdate": true,
        "viewerCanDelete": true,
        "viewerCanReact": true,
        "reactionGroups": [
          {"content": "THUMBS_UP", "viewerHasReacted": false, "reactors": {"totalCount": 0}}
        ]
      }
    }
  }
}`

func TestAPostedCommentComesBackAsTheReadPathWouldHaveIt(t *testing.T) {
	doer := &fakeDoer{body: postedBody}

	res, err := newWithDoer(doer, nil).AddComment(context.Background(), "PR_1", "Looks right to me.")
	if err != nil {
		t.Fatalf("AddComment: %v", err)
	}

	want := Comment{
		Kind:            CommentIssue,
		ID:              "IC_NEW",
		Author:          Actor{Login: "drucial"},
		CreatedAt:       time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC),
		Body:            "Looks right to me.",
		ViewerDidAuthor: true,
		CanEdit:         true,
		CanDelete:       true,
		CanReact:        true,
	}
	if !reflect.DeepEqual(res.Comment, want) {
		t.Errorf("Comment = %+v, want %+v", res.Comment, want)
	}

	if res.Comment.Pending {
		t.Error("a comment GitHub confirmed came back marked pending")
	}
}

func TestThePostSendsTheSubjectAndBodyAsVariables(t *testing.T) {
	doer := &fakeDoer{body: postedBody}

	if _, err := newWithDoer(doer, nil).AddComment(context.Background(), "PR_1", "``` fenced ```"); err != nil {
		t.Fatalf("AddComment: %v", err)
	}

	if got := doer.gotVars["subjectId"]; got != "PR_1" {
		t.Errorf("subjectId = %v, want PR_1", got)
	}
	if got := doer.gotVars["body"]; got != "``` fenced ```" {
		t.Errorf("body = %v, want it passed through untouched", got)
	}
	if strings.Contains(doer.gotQuery, "fenced") {
		t.Error("the body was written into the document instead of sent as a variable")
	}
}

func TestTheMutationAsksForWhatTheViewerMayDoNext(t *testing.T) {
	for _, want := range []string{
		"id", "createdAt", "body", "author { login }",
		"viewerDidAuthor", "viewerCanUpdate", "viewerCanDelete", "viewerCanReact",
	} {
		if !strings.Contains(addCommentMutation, want) {
			t.Errorf("the mutation does not ask for %q", want)
		}
	}

	if strings.Contains(addCommentMutation, "rateLimit") {
		t.Error("the mutation asks for rateLimit, which does not exist on Mutation")
	}
}

func TestAPostThatReturnsNoCommentIsAnError(t *testing.T) {
	_, err := newWithDoer(&fakeDoer{body: `{"addComment": {"commentEdge": {"node": {}}}}`}, nil).
		AddComment(context.Background(), "PR_1", "hi")
	if err == nil {
		t.Fatal("an empty node came back as a posted comment")
	}
	if !strings.Contains(err.Error(), "returned no comment") {
		t.Errorf("error = %q, want it to say nothing came back", err)
	}
}

func TestAFailedPostSaysWhatItWasDoing(t *testing.T) {
	sunk := errors.New("network is down")

	_, err := newWithDoer(&fakeDoer{err: sunk}, nil).AddComment(context.Background(), "PR_1", "hi")
	if !errors.Is(err, sunk) {
		t.Fatalf("error = %v, want it to wrap %v", err, sunk)
	}
	if !strings.Contains(err.Error(), "posting a comment") {
		t.Errorf("error = %q, want it to name the call", err)
	}
}
