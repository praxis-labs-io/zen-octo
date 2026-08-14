package gh

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestUpdateSendsTheIdAndBodyAndMapsTheAnswerBack(t *testing.T) {
	tests := []struct {
		name    string
		kind    CommentKind
		payload string
		mutates string
	}{
		{
			name: "issue comment",
			kind: CommentIssue,
			payload: `{"updateIssueComment": {"issueComment": {
			  "id": "IC_1", "createdAt": "2026-08-09T12:00:00Z", "body": "fixed",
			  "author": {"login": "drucial"},
			  "viewerDidAuthor": true, "viewerCanUpdate": true,
			  "viewerCanDelete": true, "viewerCanReact": true,
			  "reactionGroups": [
			    {"content": "ROCKET", "viewerHasReacted": false, "reactors": {"totalCount": 1}}
			  ]}}}`,
			mutates: "updateIssueComment",
		},
		{
			name: "review comment",
			kind: CommentThread,
			payload: `{"updatePullRequestReviewComment": {"pullRequestReviewComment": {
			  "id": "IC_1", "createdAt": "2026-08-09T12:00:00Z", "body": "fixed",
			  "author": {"login": "drucial"},
			  "viewerDidAuthor": true, "viewerCanUpdate": true,
			  "viewerCanDelete": true, "viewerCanReact": true,
			  "reactionGroups": [
			    {"content": "ROCKET", "viewerHasReacted": false, "reactors": {"totalCount": 1}}
			  ]}}}`,
			mutates: "updatePullRequestReviewComment",
		},
		{
			name: "review body",
			kind: CommentReview,
			payload: `{"updatePullRequestReview": {"pullRequestReview": {
			  "id": "IC_1", "createdAt": "2026-08-09T12:00:00Z", "body": "fixed",
			  "author": {"login": "drucial"},
			  "viewerDidAuthor": true, "viewerCanUpdate": true,
			  "viewerCanDelete": true, "viewerCanReact": true,
			  "reactionGroups": [
			    {"content": "ROCKET", "viewerHasReacted": false, "reactors": {"totalCount": 1}}
			  ]}}}`,
			mutates: "updatePullRequestReview",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeDoer{body: tt.payload}

			res, err := newWithDoer(f, nil).UpdateComment(context.Background(), tt.kind, "IC_1", "fixed")
			if err != nil {
				t.Fatalf("UpdateComment: %v", err)
			}

			if got := f.gotVars["id"]; got != "IC_1" {
				t.Errorf("id = %v, want IC_1", got)
			}
			if got := f.gotVars["body"]; got != "fixed" {
				t.Errorf("body = %v, want it passed through untouched", got)
			}
			if !strings.Contains(f.gotQuery, tt.mutates) {
				t.Errorf("query does not use %s", tt.mutates)
			}
			if strings.Contains(f.gotQuery, "rateLimit") {
				t.Error("mutation selects rateLimit, which GitHub rejects")
			}

			want := Comment{
				Kind:            tt.kind,
				ID:              "IC_1",
				Author:          Actor{Login: "drucial"},
				CreatedAt:       time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC),
				Body:            "fixed",
				ViewerDidAuthor: true,
				CanEdit:         true,
				CanDelete:       true,
				CanReact:        true,
				Reactions:       []Reaction{{Content: ReactionRocket, Count: 1}},
			}
			if !reflect.DeepEqual(res.Comment, want) {
				t.Errorf("Comment = %+v, want %+v", res.Comment, want)
			}
		})
	}
}

// The three inputs disagree on what the id is called. Sending the right
// document with the wrong spelling is a 422 about an unknown field.
func TestEachUpdateNamesTheIdItsOwnInputTakes(t *testing.T) {
	for _, tt := range []struct {
		doc   string
		takes string
	}{
		{updateIssueCommentMutation, "input: {id: $id"},
		{updateReviewCommentMutation, "input: {pullRequestReviewCommentId: $id"},
		{updateReviewMutation, "input: {pullRequestReviewId: $id"},
	} {
		if !strings.Contains(tt.doc, tt.takes) {
			t.Errorf("a mutation does not take %q", tt.takes)
		}
	}
}

// A field missing from a mutation decodes to a zero value from canned JSON, so
// the mapping test above passes while the field is dead.
func TestEveryUpdateAsksForWhatTheViewerMayDoNext(t *testing.T) {
	for _, doc := range []string{
		updateIssueCommentMutation, updateReviewCommentMutation, updateReviewMutation,
	} {
		for _, want := range []string{
			"id", "createdAt", "body", "author { login }",
			"viewerDidAuthor", "viewerCanUpdate", "viewerCanDelete", "viewerCanReact",
		} {
			if !strings.Contains(doc, want) {
				t.Errorf("a mutation does not ask for %q", want)
			}
		}
	}
}

// An answer landing in another payload's field is a failure, never a comment.
// This is what a response read by whichever half came back filled would miss.
func TestAnUpdateReadsTheHalfItsOwnDocumentProduced(t *testing.T) {
	f := &fakeDoer{body: `{"updatePullRequestReview": {"pullRequestReview":
	  {"id": "PRR_1", "body": "fixed"}}}`}

	_, err := newWithDoer(f, nil).UpdateComment(context.Background(), CommentIssue, "IC_1", "fixed")
	if err == nil {
		t.Fatal("a review payload came back as an edited issue comment")
	}
	if !strings.Contains(err.Error(), "returned no comment") {
		t.Errorf("error = %q, want it to say nothing came back", err)
	}
}

func TestAnUpdateOfAnUnknownKindSendsNothing(t *testing.T) {
	f := &fakeDoer{body: `{}`}

	_, err := newWithDoer(f, nil).UpdateComment(context.Background(), CommentKind("PR"), "X_1", "fixed")
	if err == nil {
		t.Fatal("an unknown kind picked a mutation")
	}
	if f.gotQuery != "" {
		t.Error("an unknown kind reached the network")
	}
}

func TestAFailedUpdateSaysWhatItWasDoing(t *testing.T) {
	sunk := errors.New("network is down")

	_, err := newWithDoer(&fakeDoer{err: sunk}, nil).
		UpdateComment(context.Background(), CommentThread, "PRRC_1", "fixed")
	if !errors.Is(err, sunk) {
		t.Fatalf("error = %v, want it to wrap %v", err, sunk)
	}
	if !strings.Contains(err.Error(), "editing a review comment") {
		t.Errorf("error = %q, want it to name the call", err)
	}
}

func TestDeleteSendsTheIdThroughTheMutationForItsKind(t *testing.T) {
	for _, tt := range []struct {
		name    string
		kind    CommentKind
		mutates string
	}{
		{"issue comment", CommentIssue, "deleteIssueComment"},
		{"review comment", CommentThread, "deletePullRequestReviewComment"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeDoer{body: `{}`}

			if err := newWithDoer(f, nil).DeleteComment(context.Background(), tt.kind, "IC_1"); err != nil {
				t.Fatalf("DeleteComment: %v", err)
			}
			if got := f.gotVars["id"]; got != "IC_1" {
				t.Errorf("id = %v, want IC_1", got)
			}
			if !strings.Contains(f.gotQuery, tt.mutates) {
				t.Errorf("query does not use %s", tt.mutates)
			}
			if strings.Contains(f.gotQuery, "rateLimit") {
				t.Error("mutation selects rateLimit, which GitHub rejects")
			}
		})
	}
}

// viewerCanDelete is true on a submitted review and there is no call that
// deletes one. Nothing above offers the key, and this is the backstop: a
// request sent here would come back refused in GitHub's words about a call this
// side chose.
func TestAReviewBodyCannotBeDeleted(t *testing.T) {
	f := &fakeDoer{body: `{}`}

	err := newWithDoer(f, nil).DeleteComment(context.Background(), CommentReview, "PRR_1")
	if err == nil {
		t.Fatal("a review body was sent to a delete mutation")
	}
	if f.gotQuery != "" {
		t.Error("a review body reached the network")
	}
}

func TestAFailedDeleteSaysWhatItWasDoing(t *testing.T) {
	sunk := errors.New("network is down")

	err := newWithDoer(&fakeDoer{err: sunk}, nil).
		DeleteComment(context.Background(), CommentIssue, "IC_1")
	if !errors.Is(err, sunk) {
		t.Fatalf("error = %v, want it to wrap %v", err, sunk)
	}
	if !strings.Contains(err.Error(), "deleting a comment") {
		t.Errorf("error = %q, want it to name the call", err)
	}
}
