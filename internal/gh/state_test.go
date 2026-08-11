package gh

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Every payload is aliased to `result`, so one body serves all four documents.
func stateBody(state string, draft bool) string {
	d := "false"
	if draft {
		d = "true"
	}
	return `{"result": {"pullRequest": {"id": "PR_1", "state": "` + state + `", "isDraft": ` + d + `}}}`
}

func TestSetStateSendsTheMutationForEachTransition(t *testing.T) {
	tests := []struct {
		name     string
		to       PRTransition
		mutation string
		state    string
		draft    bool
	}{
		{"ready", TransitionReady, "markPullRequestReadyForReview", "OPEN", false},
		{"draft", TransitionDraft, "convertPullRequestToDraft", "OPEN", true},
		{"close", TransitionClose, "closePullRequest", "CLOSED", false},
		{"reopen", TransitionReopen, "reopenPullRequest", "OPEN", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeDoer{body: stateBody(tt.state, tt.draft)}

			res, err := newWithDoer(f, nil).SetState(context.Background(), "PR_1", tt.to)
			if err != nil {
				t.Fatalf("SetState: %v", err)
			}

			if !strings.Contains(f.gotQuery, tt.mutation) {
				t.Errorf("query does not use %s", tt.mutation)
			}
			if got, want := f.gotVars["pullRequestId"], "PR_1"; got != want {
				t.Errorf("pullRequestId = %v, want %v", got, want)
			}
			// rateLimit is a field on Query alone; a mutation naming it is
			// rejected whole, so no document here may carry it.
			if strings.Contains(f.gotQuery, "rateLimit") {
				t.Error("mutation selects rateLimit, which GitHub rejects")
			}

			if got, want := res.State, PRState(tt.state); got != want {
				t.Errorf("State = %q, want %q", got, want)
			}
			if got, want := res.IsDraft, tt.draft; got != want {
				t.Errorf("IsDraft = %v, want %v", got, want)
			}
		})
	}
}

// Closing a draft leaves it a draft, which is why the result carries both
// fields rather than the one the transition names.
func TestSetStateReturnsBothFields(t *testing.T) {
	f := &fakeDoer{body: stateBody("CLOSED", true)}

	res, err := newWithDoer(f, nil).SetState(context.Background(), "PR_1", TransitionClose)
	if err != nil {
		t.Fatalf("SetState: %v", err)
	}

	if res.State != PRStateClosed {
		t.Errorf("State = %q, want CLOSED", res.State)
	}
	if !res.IsDraft {
		t.Error("IsDraft = false, want the draft flag kept through a close")
	}
}

// A transition with no document behind it is refused here rather than sent, so
// a request that could only come back rejected costs no round trip.
func TestSetStateRefusesAnUnknownTransitionWithoutCalling(t *testing.T) {
	f := &fakeDoer{body: stateBody("OPEN", false)}

	_, err := newWithDoer(f, nil).SetState(context.Background(), "PR_1", PRTransition("SHRED"))
	if err == nil {
		t.Fatal("SetState = nil error, want one")
	}
	if !strings.Contains(err.Error(), "no such transition") {
		t.Errorf("error = %q, want it to name the refusal", err)
	}
	if f.gotQuery != "" {
		t.Errorf("sent a query anyway: %q", f.gotQuery)
	}
}

func TestSetStateMissingPullRequestIsAnError(t *testing.T) {
	f := &fakeDoer{body: `{"result": {"pullRequest": null}}`}

	_, err := newWithDoer(f, nil).SetState(context.Background(), "PR_1", TransitionClose)
	if err == nil {
		t.Fatal("SetState = nil error, want one")
	}
	if !strings.Contains(err.Error(), "no pull request") {
		t.Errorf("error = %q, want it to say nothing came back", err)
	}
}

func TestSetStateWrapsTransportError(t *testing.T) {
	boom := errors.New("boom")
	f := &fakeDoer{err: boom}

	_, err := newWithDoer(f, nil).SetState(context.Background(), "PR_1", TransitionClose)
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want it to wrap %v", err, boom)
	}
	if !strings.Contains(err.Error(), "changing the state") {
		t.Errorf("error = %q, want it to name what failed", err)
	}
}

// The alias is what lets one response struct decode four payloads. Without it
// each document answers under its own mutation name and three of the four
// decode into nothing, which reads as GitHub returning no pull request.
func TestEveryStateMutationAliasesItsPayload(t *testing.T) {
	for _, to := range []PRTransition{TransitionReady, TransitionDraft, TransitionClose, TransitionReopen} {
		doc, ok := stateMutation(to)
		if !ok {
			t.Fatalf("%s has no document", to)
		}
		if !strings.Contains(doc, "result:") {
			t.Errorf("%s does not alias its payload to result", to)
		}
	}
}
