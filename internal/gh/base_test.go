package gh

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSetBaseSendsTheBranchNameAndReturnsIt(t *testing.T) {
	f := &fakeDoer{body: `{"updatePullRequest": {"pullRequest":
	  {"id": "PR_1", "baseRefName": "develop"}}}`}

	res, err := newWithDoer(f, nil).SetBase(context.Background(), "PR_1", "develop")
	if err != nil {
		t.Fatalf("SetBase: %v", err)
	}

	if got, want := f.gotVars["pullRequestId"], "PR_1"; got != want {
		t.Errorf("pullRequestId = %v, want %v", got, want)
	}
	// A name, not a node id. This is the one field on updatePullRequest that
	// takes one, and sending an id here is a 422 that reads like a permission
	// failure.
	if got, want := f.gotVars["baseRefName"], "develop"; got != want {
		t.Errorf("baseRefName = %v, want %v", got, want)
	}
	if !strings.Contains(f.gotQuery, "updatePullRequest") {
		t.Error("query does not use updatePullRequest")
	}
	if strings.Contains(f.gotQuery, "rateLimit") {
		t.Error("mutation selects rateLimit, which GitHub rejects")
	}

	if got, want := res.BaseRefName, "develop"; got != want {
		t.Errorf("BaseRefName = %q, want %q", got, want)
	}
}

// GitHub's answer rather than the ask. They part company when somebody
// retargets in the browser first, and the toast names what came back.
func TestSetBaseReturnsWhatGitHubRecordedNotWhatWasAsked(t *testing.T) {
	f := &fakeDoer{body: `{"updatePullRequest": {"pullRequest":
	  {"id": "PR_1", "baseRefName": "release/2.0"}}}`}

	res, err := newWithDoer(f, nil).SetBase(context.Background(), "PR_1", "develop")
	if err != nil {
		t.Fatalf("SetBase: %v", err)
	}
	if got, want := res.BaseRefName, "release/2.0"; got != want {
		t.Errorf("BaseRefName = %q, want %q", got, want)
	}
}

func TestSetBaseWrapsAFailure(t *testing.T) {
	boom := errors.New("Base branch cannot be modified")
	f := &fakeDoer{err: boom}

	_, err := newWithDoer(f, nil).SetBase(context.Background(), "PR_1", "develop")
	if err == nil {
		t.Fatal("SetBase: want an error")
	}
	if !strings.Contains(err.Error(), "setting base branch") {
		t.Errorf("error = %q, want it to name what failed", err)
	}
	if !strings.Contains(err.Error(), boom.Error()) {
		t.Errorf("error = %q, want it to carry what GitHub said", err)
	}
}

// A refusal can come back as a 200 with a null pull request. Reading that as a
// success would settle the optimistic row onto an empty branch name.
func TestSetBaseRefusesANullPullRequest(t *testing.T) {
	f := &fakeDoer{body: `{"updatePullRequest": {"pullRequest": null}}`}

	if _, err := newWithDoer(f, nil).SetBase(context.Background(), "PR_1", "develop"); err == nil {
		t.Fatal("SetBase: want an error for a null pull request")
	}
}
