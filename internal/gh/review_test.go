package gh

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

const startedReviewBody = `{
  "addPullRequestReview": {
    "pullRequestReview": {"id": "PRR_1", "state": "PENDING", "body": "Two things."}
  }
}`

const openedThreadBody = `{
  "addPullRequestReviewThread": {
    "thread": {
      "id": "PRRT_1", "isResolved": false, "isOutdated": false,
      "viewerCanReply": true, "viewerCanResolve": false, "viewerCanUnresolve": false,
      "path": "internal/gh/client.go", "line": 42, "startLine": 42,
      "diffSide": "RIGHT", "subjectType": "LINE",
      "comments": {"totalCount": 1, "nodes": [
        {"id": "PRRC_1", "author": {"login": "drucial"}, "createdAt": "2026-09-20T12:00:00Z",
         "body": "Needs a ceiling.", "state": "PENDING",
         "viewerDidAuthor": true, "viewerCanUpdate": true,
         "viewerCanDelete": true, "viewerCanReact": true,
         "pullRequestReview": {"id": "PRR_1"}}
      ]}
    }
  }
}`

const submittedReviewBody = `{
  "submitPullRequestReview": {
    "pullRequestReview": {"id": "PRR_1", "state": "CHANGES_REQUESTED", "body": "Two things."}
  }
}`

const discardedReviewBody = `{
  "deletePullRequestReview": {"pullRequestReview": {"id": "PRR_1", "state": "PENDING"}}
}`

func lineThread() ReviewThreadInput {
	return ReviewThreadInput{
		ReviewID: "PRR_1",
		Path:     "internal/gh/client.go",
		Body:     "Needs a ceiling.",
		Subject:  SubjectLine,
		Line:     42,
		Side:     SideRight,
	}
}

func TestAStartedReviewComesBackPending(t *testing.T) {
	res, err := newWithDoer(&fakeDoer{body: startedReviewBody}, nil).
		StartReview(context.Background(), "PR_412", "Two things.")
	if err != nil {
		t.Fatalf("StartReview: %v", err)
	}

	want := Review{ID: "PRR_1", State: ReviewStatePending, Body: "Two things."}
	if res.Review != want {
		t.Errorf("Review = %+v, want %+v", res.Review, want)
	}
}

func TestTheStartedReviewSendsThePullRequestAndBodyAsVariables(t *testing.T) {
	doer := &fakeDoer{body: startedReviewBody}

	if _, err := newWithDoer(doer, nil).StartReview(context.Background(), "PR_412", "``` fenced ```"); err != nil {
		t.Fatalf("StartReview: %v", err)
	}

	if got := doer.gotVars["prId"]; got != "PR_412" {
		t.Errorf("prId = %v, want PR_412", got)
	}
	if got := doer.gotVars["body"]; got != "``` fenced ```" {
		t.Errorf("body = %v, want it passed through untouched", got)
	}
	if strings.Contains(doer.gotQuery, "fenced") {
		t.Error("the body was written into the document instead of sent as a variable")
	}
}

func TestAnOpenedThreadComesBackUnsent(t *testing.T) {
	res, err := newWithDoer(&fakeDoer{body: openedThreadBody}, nil).
		AddReviewThread(context.Background(), lineThread())
	if err != nil {
		t.Fatalf("AddReviewThread: %v", err)
	}

	want := ReviewThread{
		ID:        "PRRT_1",
		ReviewID:  "PRR_1",
		Path:      "internal/gh/client.go",
		Line:      42,
		StartLine: 42,
		Subject:   SubjectLine,
		Side:      SideRight,
		CanReply:  true,
		Draft:     true,
		Comments: []Comment{{
			Kind:            CommentThread,
			ID:              "PRRC_1",
			Author:          Actor{Login: "drucial"},
			CreatedAt:       time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC),
			Body:            "Needs a ceiling.",
			ViewerDidAuthor: true,
			CanEdit:         true,
			CanDelete:       true,
			CanReact:        true,
			Draft:           true,
		}},
	}

	if !reflect.DeepEqual(res.Thread, want) {
		t.Errorf("Thread = %+v, want %+v", res.Thread, want)
	}
}

func TestTheThreadSendsWhereItIsAnchored(t *testing.T) {
	tests := []struct {
		name string
		in   ReviewThreadInput
		want map[string]any
	}{
		{
			name: "one line",
			in:   lineThread(),
			want: map[string]any{
				"reviewId": "PRR_1", "prId": nil, "subjectType": "LINE",
				"line": 42, "side": "RIGHT", "startLine": nil, "startSide": nil,
			},
		},
		{
			name: "a range",
			in: ReviewThreadInput{
				ReviewID: "PRR_1", Path: "a.go", Body: "hi", Subject: SubjectLine,
				Line: 48, Side: SideRight, StartLine: 42, StartSide: SideRight,
			},
			want: map[string]any{
				"line": 48, "side": "RIGHT", "startLine": 42, "startSide": "RIGHT",
			},
		},
		{
			name: "a range with no start side of its own",
			in: ReviewThreadInput{
				ReviewID: "PRR_1", Path: "a.go", Body: "hi",
				Line: 48, Side: SideLeft, StartLine: 42,
			},
			want: map[string]any{"startSide": "LEFT"},
		},
		{
			name: "a whole file",
			in: ReviewThreadInput{
				ReviewID: "PRR_1", Path: "a.go", Body: "hi", Subject: SubjectFile, Line: 1, Side: SideRight,
			},
			want: map[string]any{
				"subjectType": "FILE",
				"line":        nil, "side": nil, "startLine": nil, "startSide": nil,
			},
		},
		{
			name: "no review of its own",
			in: ReviewThreadInput{
				PullRequestID: "PR_412", Path: "a.go", Body: "hi", Line: 3, Side: SideRight,
			},
			want: map[string]any{"prId": "PR_412", "reviewId": nil, "subjectType": "LINE"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doer := &fakeDoer{body: openedThreadBody}

			if _, err := newWithDoer(doer, nil).AddReviewThread(context.Background(), tt.in); err != nil {
				t.Fatalf("AddReviewThread: %v", err)
			}
			for key, want := range tt.want {
				if got := doer.gotVars[key]; got != want {
					t.Errorf("%s = %v, want %v", key, got, want)
				}
			}
		})
	}
}

func TestAThreadOnNothingIsRefusedBeforeItIsSent(t *testing.T) {
	doer := &fakeDoer{body: openedThreadBody}

	_, err := newWithDoer(doer, nil).AddReviewThread(context.Background(), ReviewThreadInput{Path: "a.go", Body: "hi"})
	if err == nil {
		t.Fatal("a thread with no pull request and no review came back as a success")
	}
	if doer.gotQuery != "" {
		t.Error("the call reached GitHub with nothing to open the thread on")
	}
}

func TestASubmittedReviewComesBackInItsNewState(t *testing.T) {
	res, err := newWithDoer(&fakeDoer{body: submittedReviewBody}, nil).
		SubmitReview(context.Background(), "PRR_1", ReviewEventRequestChanges, "Two things.")
	if err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}

	want := Review{ID: "PRR_1", State: ReviewStateChangesRequested, Body: "Two things."}
	if res.Review != want {
		t.Errorf("Review = %+v, want %+v", res.Review, want)
	}
}

func TestTheSubmitSendsTheEventAndWhateverTheFormHolds(t *testing.T) {
	tests := []struct {
		name  string
		event ReviewEvent
		body  string
		want  any
	}{
		{"a comment", ReviewEventComment, "Looks fine.", "Looks fine."},
		{"a summary cleared before submitting", ReviewEventApprove, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doer := &fakeDoer{body: submittedReviewBody}

			if _, err := newWithDoer(doer, nil).
				SubmitReview(context.Background(), "PRR_1", tt.event, tt.body); err != nil {
				t.Fatalf("SubmitReview: %v", err)
			}

			if got := doer.gotVars["event"]; got != string(tt.event) {
				t.Errorf("event = %v, want %v", got, tt.event)
			}
			if got := doer.gotVars["body"]; got != tt.want {
				t.Errorf("body = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestADiscardSendsTheReviewAsAVariable(t *testing.T) {
	doer := &fakeDoer{body: discardedReviewBody}

	if err := newWithDoer(doer, nil).DiscardReview(context.Background(), "PRR_ODD"); err != nil {
		t.Fatalf("DiscardReview: %v", err)
	}
	if got := doer.gotVars["reviewId"]; got != "PRR_ODD" {
		t.Errorf("reviewId = %v, want PRR_ODD", got)
	}
	if strings.Contains(doer.gotQuery, "PRR_ODD") {
		t.Error("the review was written into the document instead of sent as a variable")
	}
}

func TestAReviewWriteThatReturnsNothingIsAnError(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
		call func(*Client) error
	}{
		{
			name: "start", body: `{"addPullRequestReview": {"pullRequestReview": {}}}`,
			want: "returned no review",
			call: func(c *Client) error {
				_, err := c.StartReview(context.Background(), "PR_412", "")
				return err
			},
		},
		{
			name: "thread", body: `{"addPullRequestReviewThread": {"thread": {}}}`,
			want: "returned no thread",
			call: func(c *Client) error {
				_, err := c.AddReviewThread(context.Background(), lineThread())
				return err
			},
		},
		{
			name: "submit", body: `{"submitPullRequestReview": {"pullRequestReview": {}}}`,
			want: "returned no review",
			call: func(c *Client) error {
				_, err := c.SubmitReview(context.Background(), "PRR_1", ReviewEventApprove, "")
				return err
			},
		},
		{
			name: "discard", body: `{"deletePullRequestReview": {"pullRequestReview": {}}}`,
			want: "returned no review",
			call: func(c *Client) error { return c.DiscardReview(context.Background(), "PRR_1") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(newWithDoer(&fakeDoer{body: tt.body}, nil))
			if err == nil {
				t.Fatal("an empty node came back as a success")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to say %q", err, tt.want)
			}
		})
	}
}

func TestAFailedReviewWriteSaysWhatItWasDoing(t *testing.T) {
	tests := []struct {
		name string
		want string
		call func(*Client) error
	}{
		{"start", "starting a review", func(c *Client) error {
			_, err := c.StartReview(context.Background(), "PR_412", "")
			return err
		}},
		{"thread", "opening a review thread", func(c *Client) error {
			_, err := c.AddReviewThread(context.Background(), lineThread())
			return err
		}},
		{"submit", "submitting a review", func(c *Client) error {
			_, err := c.SubmitReview(context.Background(), "PRR_1", ReviewEventApprove, "")
			return err
		}},
		{"discard", "discarding a review", func(c *Client) error {
			return c.DiscardReview(context.Background(), "PRR_1")
		}},
	}

	sunk := errors.New("network is down")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(newWithDoer(&fakeDoer{err: sunk}, nil))
			if !errors.Is(err, sunk) {
				t.Fatalf("error = %v, want it to wrap %v", err, sunk)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to say %q", err, tt.want)
			}
		})
	}
}

func TestTheReviewMutationsAskForWhatTheCallerNeedsBack(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		want []string
	}{
		{"start", startReviewMutation, []string{"id", "state", "body"}},
		{"submit", submitReviewMutation, []string{"id", "state", "body"}},
		{"discard", discardReviewMutation, []string{"id", "state"}},
		{"thread", addReviewThreadMutation, []string{
			"id", "path", "line", "startLine", "diffSide", "subjectType",
			"isResolved", "isOutdated", "viewerCanReply",
			"viewerCanResolve", "viewerCanUnresolve",
			"state", "diffHunk", "pullRequestReview { id }",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, want := range tt.want {
				if !strings.Contains(tt.doc, want) {
					t.Errorf("the mutation does not ask for %q", want)
				}
			}
			if strings.Contains(tt.doc, "rateLimit") {
				t.Error("the mutation asks for rateLimit, which does not exist on Mutation")
			}
		})
	}
}

// The thread read and the thread write share one decoder, so the two documents have to select the same fields.
func TestTheOpenedThreadIsReadTheSameWayTheFetchedOneIs(t *testing.T) {
	for _, want := range []string{
		"originalLine", "originalStartLine", "subjectType", "state", "diffHunk",
	} {
		if !strings.Contains(addReviewThreadMutation, want) {
			t.Errorf("the mutation does not ask for %q, which the fetched thread carries", want)
		}
	}
}
