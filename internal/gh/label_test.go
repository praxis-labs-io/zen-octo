package gh

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const setLabelsBody = `{
  "updatePullRequest": {
    "pullRequest": {
      "id": "PR_1",
      "labels": {"nodes": [
        {"id": "LA_1", "name": "bug", "color": "d73a4a"}
      ]}
    }
  }
}`

func TestSetLabelsSendsIDsAndReturnsTheSet(t *testing.T) {
	f := &fakeDoer{body: setLabelsBody}

	res, err := newWithDoer(f, nil).SetLabels(context.Background(), "PR_1", []string{"LA_1"})
	if err != nil {
		t.Fatalf("SetLabels: %v", err)
	}

	if got, want := f.gotVars["pullRequestId"], "PR_1"; got != want {
		t.Errorf("pullRequestId = %v, want %v", got, want)
	}
	ids, ok := f.gotVars["labelIds"].([]string)
	if !ok || len(ids) != 1 || ids[0] != "LA_1" {
		t.Errorf("labelIds = %v, want [LA_1]", f.gotVars["labelIds"])
	}
	if !strings.Contains(f.gotQuery, "updatePullRequest") {
		t.Error("query does not use updatePullRequest")
	}
	// rateLimit is a field on Query alone; a mutation naming it is rejected
	// whole, so the document must not carry it.
	if strings.Contains(f.gotQuery, "rateLimit") {
		t.Error("mutation selects rateLimit, which GitHub rejects")
	}

	if got, want := len(res.Labels), 1; got != want {
		t.Fatalf("labels = %d, want %d", got, want)
	}
	if got, want := res.Labels[0].Name, "bug"; got != want {
		t.Errorf("labels[0].Name = %q, want %q", got, want)
	}
}

// Clearing every label is a real write, not a call to skip. A nil slice would
// marshal to null and the non-null [ID!]! type rejects it.
func TestSetLabelsSendsEmptyArrayNotNull(t *testing.T) {
	f := &fakeDoer{body: `{"updatePullRequest": {"pullRequest": {"id": "PR_1", "labels": {"nodes": []}}}}`}

	res, err := newWithDoer(f, nil).SetLabels(context.Background(), "PR_1", nil)
	if err != nil {
		t.Fatalf("SetLabels: %v", err)
	}

	ids, ok := f.gotVars["labelIds"].([]string)
	if !ok {
		t.Fatalf("labelIds = %#v, want an empty []string", f.gotVars["labelIds"])
	}
	if ids == nil {
		t.Error("labelIds is nil, which marshals to null")
	}
	if len(ids) != 0 {
		t.Errorf("labelIds = %v, want empty", ids)
	}

	if res.Labels == nil {
		t.Error("Labels is nil, want an empty slice")
	}
	if len(res.Labels) != 0 {
		t.Errorf("Labels = %v, want empty", res.Labels)
	}
}

func TestSetLabelsMissingPullRequestIsAnError(t *testing.T) {
	f := &fakeDoer{body: `{"updatePullRequest": {"pullRequest": null}}`}

	_, err := newWithDoer(f, nil).SetLabels(context.Background(), "PR_1", []string{"LA_1"})
	if err == nil {
		t.Fatal("SetLabels = nil error, want one")
	}
	if !strings.Contains(err.Error(), "no pull request") {
		t.Errorf("error = %q, want it to say nothing came back", err)
	}
}

func TestSetLabelsWrapsTransportError(t *testing.T) {
	boom := errors.New("boom")
	f := &fakeDoer{err: boom}

	_, err := newWithDoer(f, nil).SetLabels(context.Background(), "PR_1", []string{"LA_1"})
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want it to wrap %v", err, boom)
	}
	if !strings.Contains(err.Error(), "setting labels") {
		t.Errorf("error = %q, want it to name what failed", err)
	}
}
