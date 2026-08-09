package gh

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const resolvedBody = `{
  "resolveReviewThread": {
    "thread": {
      "id": "PRRT_1",
      "isResolved": true,
      "viewerCanResolve": false,
      "viewerCanUnresolve": true
    }
  }
}`

const unresolvedBody = `{
  "unresolveReviewThread": {
    "thread": {
      "id": "PRRT_1",
      "isResolved": false,
      "viewerCanResolve": true,
      "viewerCanUnresolve": false
    }
  }
}`

// The permissions come back flipped, and that is the point of asking for them.
// A viewer who has just resolved a thread cannot resolve it again, and the key
// on the card has to say so.
func TestAResolvedThreadComesBackWithItsPermissionsFlipped(t *testing.T) {
	doer := &fakeDoer{body: resolvedBody}

	res, err := newWithDoer(doer, nil).SetThreadResolved(context.Background(), "PRRT_1", true)
	if err != nil {
		t.Fatalf("SetThreadResolved: %v", err)
	}

	want := ThreadResult{ID: "PRRT_1", IsResolved: true, CanUnresolve: true}
	if res != want {
		t.Errorf("ThreadResult = %+v, want %+v", res, want)
	}
}

func TestAnUnresolvedThreadComesBackOpen(t *testing.T) {
	doer := &fakeDoer{body: unresolvedBody}

	res, err := newWithDoer(doer, nil).SetThreadResolved(context.Background(), "PRRT_1", false)
	if err != nil {
		t.Fatalf("SetThreadResolved: %v", err)
	}

	want := ThreadResult{ID: "PRRT_1", CanResolve: true}
	if res != want {
		t.Errorf("ThreadResult = %+v, want %+v", res, want)
	}
}

// The operation name is what the assertion anchors on. resolveReviewThread is a
// substring of unresolveReviewThread, so a test looking for the field name
// passes on both documents and says nothing.
func TestTheDirectionPicksTheMutation(t *testing.T) {
	tests := []struct {
		name     string
		resolved bool
		body     string
		want     string
	}{
		{"resolve", true, resolvedBody, "mutation ResolveThread("},
		{"unresolve", false, unresolvedBody, "mutation UnresolveThread("},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doer := &fakeDoer{body: tt.body}

			if _, err := newWithDoer(doer, nil).
				SetThreadResolved(context.Background(), "PRRT_1", tt.resolved); err != nil {
				t.Fatalf("SetThreadResolved: %v", err)
			}
			if !strings.Contains(doer.gotQuery, tt.want) {
				t.Errorf("the call sent the wrong document, wanted one starting %q", tt.want)
			}
		})
	}
}

// One response struct decodes both payloads, so a method reading whichever
// field came back filled would answer with the wrong half the first time
// GitHub sent both.
func TestAnUnresolveDoesNotReadTheResolvePayload(t *testing.T) {
	both := `{
	  "resolveReviewThread": {"thread": {"id": "PRRT_1", "isResolved": true}},
	  "unresolveReviewThread": {"thread": {"id": "PRRT_1", "isResolved": false}}
	}`

	res, err := newWithDoer(&fakeDoer{body: both}, nil).
		SetThreadResolved(context.Background(), "PRRT_1", false)
	if err != nil {
		t.Fatalf("SetThreadResolved: %v", err)
	}
	if res.IsResolved {
		t.Error("the unresolve read the resolve half of the response")
	}
}

func TestTheThreadSendsAsAVariable(t *testing.T) {
	doer := &fakeDoer{body: resolvedBody}

	if _, err := newWithDoer(doer, nil).
		SetThreadResolved(context.Background(), "PRRT_ODD", true); err != nil {
		t.Fatalf("SetThreadResolved: %v", err)
	}

	if got := doer.gotVars["threadId"]; got != "PRRT_ODD" {
		t.Errorf("threadId = %v, want PRRT_ODD", got)
	}
	if strings.Contains(doer.gotQuery, "PRRT_ODD") {
		t.Error("the thread was written into the document instead of sent as a variable")
	}
}

// The same trap the reply mutation has: a field the document never asks for
// decodes to a zero value from canned JSON, so every other test here stays
// green while the field is dead.
func TestTheThreadMutationsAskForWhatTheViewerMayDoNext(t *testing.T) {
	tests := []struct {
		name string
		doc  string
	}{
		{"resolve", resolveThreadMutation},
		{"unresolve", unresolveThreadMutation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, want := range []string{"id", "isResolved", "viewerCanResolve", "viewerCanUnresolve"} {
				if !strings.Contains(tt.doc, want) {
					t.Errorf("the mutation does not ask for %q", want)
				}
			}
			if !strings.Contains(tt.doc, "input: {threadId: $threadId}") {
				t.Error("the mutation does not address the thread")
			}
			if strings.Contains(tt.doc, "rateLimit") {
				t.Error("the mutation asks for rateLimit, which does not exist on Mutation")
			}
		})
	}
}

func TestAResolveThatReturnsNoThreadIsAnError(t *testing.T) {
	_, err := newWithDoer(&fakeDoer{body: `{"resolveReviewThread": {"thread": {}}}`}, nil).
		SetThreadResolved(context.Background(), "PRRT_1", true)
	if err == nil {
		t.Fatal("an empty node came back as a resolved thread")
	}
	if !strings.Contains(err.Error(), "returned no thread") {
		t.Errorf("error = %q, want it to say nothing came back", err)
	}
}

func TestAFailedResolveSaysWhatItWasDoing(t *testing.T) {
	tests := []struct {
		name     string
		resolved bool
		want     string
	}{
		{"resolve", true, "resolving a review thread"},
		{"unresolve", false, "unresolving a review thread"},
	}

	sunk := errors.New("network is down")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newWithDoer(&fakeDoer{err: sunk}, nil).
				SetThreadResolved(context.Background(), "PRRT_1", tt.resolved)
			if !errors.Is(err, sunk) {
				t.Fatalf("error = %v, want it to wrap %v", err, sunk)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to say %q", err, tt.want)
			}
		})
	}
}
