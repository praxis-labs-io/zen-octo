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

func TestSetBodySendsTheTextAndReturnsWhatGitHubRecorded(t *testing.T) {
	f := &fakeDoer{body: `{"updatePullRequest": {"pullRequest":
	  {"id": "PR_1", "body": "Rewritten."}}}`}

	res, err := newWithDoer(f, nil).SetBody(context.Background(), "PR_1", "Rewritten.")
	if err != nil {
		t.Fatalf("SetBody: %v", err)
	}

	if got, want := f.gotVars["pullRequestId"], "PR_1"; got != want {
		t.Errorf("pullRequestId = %v, want %v", got, want)
	}
	if got, want := f.gotVars["body"], "Rewritten."; got != want {
		t.Errorf("body = %v, want it passed through untouched", got)
	}
	if strings.Contains(f.gotQuery, "rateLimit") {
		t.Error("mutation selects rateLimit, which GitHub rejects")
	}

	if got, want := res.Body, "Rewritten."; got != want {
		t.Errorf("Body = %q, want %q", got, want)
	}
}

func TestSetBodyTakesAnEmptyDescription(t *testing.T) {
	f := &fakeDoer{body: `{"updatePullRequest": {"pullRequest": {"id": "PR_1", "body": ""}}}`}

	res, err := newWithDoer(f, nil).SetBody(context.Background(), "PR_1", "")
	if err != nil {
		t.Fatalf("SetBody: %v", err)
	}
	if res.Body != "" {
		t.Errorf("Body = %q, want it empty", res.Body)
	}
}

func TestSetBodyRefusesANullPullRequest(t *testing.T) {
	f := &fakeDoer{body: `{"updatePullRequest": {"pullRequest": null}}`}

	if _, err := newWithDoer(f, nil).SetBody(context.Background(), "PR_1", "hi"); err == nil {
		t.Fatal("SetBody: want an error for a null pull request")
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

func TestSetBaseRefusesANullPullRequest(t *testing.T) {
	f := &fakeDoer{body: `{"updatePullRequest": {"pullRequest": null}}`}

	if _, err := newWithDoer(f, nil).SetBase(context.Background(), "PR_1", "develop"); err == nil {
		t.Fatal("SetBase: want an error for a null pull request")
	}
}
