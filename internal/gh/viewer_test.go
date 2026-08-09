package gh

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const viewerBody = `{
  "rateLimit": {"limit": 5000, "cost": 1, "remaining": 4999, "resetAt": "2026-08-05T18:00:00Z"},
  "viewer": {"login": "drucial"}
}`

func TestTheViewerIsWhoTheTokenBelongsTo(t *testing.T) {
	res, err := newWithDoer(&fakeDoer{body: viewerBody}, nil).Viewer(context.Background())
	if err != nil {
		t.Fatalf("Viewer: %v", err)
	}

	if res.Viewer.Login != "drucial" {
		t.Errorf("Viewer.Login = %q, want drucial", res.Viewer.Login)
	}

	// The budget is shared with every other query, so the one point this costs
	// has to arrive with it or the status bar is a point ahead all session.
	want := RateLimit{
		Limit:     5000,
		Cost:      1,
		Remaining: 4999,
		ResetAt:   time.Date(2026, 8, 5, 18, 0, 0, 0, time.UTC),
	}
	if res.RateLimit != want {
		t.Errorf("RateLimit = %+v, want %+v", res.RateLimit, want)
	}
}

// An empty login is not an account with no name. It is a response that did not
// answer the question, and returning it would have the UI quietly deciding
// nothing is the viewer's own writing.
func TestAViewerWithNoLoginIsAnError(t *testing.T) {
	_, err := newWithDoer(&fakeDoer{body: `{"viewer": {"login": ""}}`}, nil).Viewer(context.Background())
	if err == nil {
		t.Fatal("an empty login came back as a viewer")
	}
	if !strings.Contains(err.Error(), "no account behind this token") {
		t.Errorf("error = %q, want it to say the token has no account", err)
	}
}

func TestAFailedViewerCallSaysWhatItWasDoing(t *testing.T) {
	sunk := errors.New("network is down")

	_, err := newWithDoer(&fakeDoer{err: sunk}, nil).Viewer(context.Background())
	if !errors.Is(err, sunk) {
		t.Fatalf("error = %v, want it to wrap %v", err, sunk)
	}
	if !strings.Contains(err.Error(), "fetching the viewer") {
		t.Errorf("error = %q, want it to name the call", err)
	}
}
