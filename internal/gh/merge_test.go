package gh

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestMergeSendsTheMethodTheOidAndTheMessage(t *testing.T) {
	f := &fakeDoer{body: `{"mergePullRequest": {"pullRequest":
	  {"id": "PR_1", "state": "MERGED"}}}`}

	res, err := newWithDoer(f, nil).Merge(context.Background(), "PR_1", MergeOptions{
		Method:          MergeMethodSquash,
		Headline:        "ZNO-48: merge from the rail (#24)",
		Body:            "* the form\n* the write",
		ExpectedHeadOid: "abc123",
	})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}

	for _, want := range []struct{ name, value string }{
		{"pullRequestId", "PR_1"},
		{"mergeMethod", "SQUASH"},
		{"commitHeadline", "ZNO-48: merge from the rail (#24)"},
		{"commitBody", "* the form\n* the write"},
		// The commit the reader was looking at. Without it a push that landed
		// while they read the diff is merged unseen.
		{"expectedHeadOid", "abc123"},
	} {
		if got := f.gotVars[want.name]; got != want.value {
			t.Errorf("%s = %v, want %v", want.name, got, want.value)
		}
	}

	if strings.Contains(f.gotQuery, "rateLimit") {
		t.Error("mutation selects rateLimit, which GitHub rejects")
	}
	if got, want := res.State, PRStateMerged; got != want {
		t.Errorf("State = %q, want %q", got, want)
	}
}

// A rebase writes no commit of its own, so both fields go over as null rather
// than as empty strings GitHub has to decide what to do with.
func TestMergeSendsNoMessageOnARebase(t *testing.T) {
	f := &fakeDoer{body: `{"mergePullRequest": {"pullRequest":
	  {"id": "PR_1", "state": "MERGED"}}}`}

	_, err := newWithDoer(f, nil).Merge(context.Background(), "PR_1", MergeOptions{
		Method:          MergeMethodRebase,
		ExpectedHeadOid: "abc123",
	})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}

	for _, name := range []string{"commitHeadline", "commitBody"} {
		if got := f.gotVars[name]; got != nil {
			t.Errorf("%s = %#v, want nil", name, got)
		}
	}
}

// A body somebody cleared is a body they meant to clear. Sent as null it comes
// back as GitHub's own default, which is the text they just deleted.
func TestMergeSendsAnEmptyBodyAsEmpty(t *testing.T) {
	f := &fakeDoer{body: `{"mergePullRequest": {"pullRequest":
	  {"id": "PR_1", "state": "MERGED"}}}`}

	_, err := newWithDoer(f, nil).Merge(context.Background(), "PR_1", MergeOptions{
		Method:   MergeMethodSquash,
		Headline: "Fix auth retry (#412)",
	})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}

	if got := f.gotVars["commitBody"]; got != "" {
		t.Errorf("commitBody = %#v, want an empty string rather than null", got)
	}
}

func TestMergeWrapsAFailure(t *testing.T) {
	boom := errors.New("Head branch was modified. Review and try the merge again.")
	f := &fakeDoer{err: boom}

	_, err := newWithDoer(f, nil).Merge(context.Background(), "PR_1", MergeOptions{Method: MergeMethodMerge})
	if err == nil {
		t.Fatal("Merge: want an error")
	}
	if !strings.Contains(err.Error(), "merging") {
		t.Errorf("error = %q, want it to name what failed", err)
	}
	// GitHub's own sentence is what tells the reader to sync, so it has to
	// survive being wrapped rather than be replaced with a house message.
	if !strings.Contains(err.Error(), boom.Error()) {
		t.Errorf("error = %q, want it to carry what GitHub said", err)
	}
}

// A refusal can come back as a 200 with a null pull request. Reading that as a
// success would leave the rail claiming a merge that never happened.
func TestMergeRefusesANullPullRequest(t *testing.T) {
	f := &fakeDoer{body: `{"mergePullRequest": {"pullRequest": null}}`}

	if _, err := newWithDoer(f, nil).Merge(context.Background(), "PR_1", MergeOptions{Method: MergeMethodMerge}); err == nil {
		t.Fatal("Merge: want an error for a null pull request")
	}
}

func TestDeleteRefSendsTheNodeID(t *testing.T) {
	f := &fakeDoer{body: `{"deleteRef": {"clientMutationId": null}}`}

	if err := newWithDoer(f, nil).DeleteRef(context.Background(), "REF_1"); err != nil {
		t.Fatalf("DeleteRef: %v", err)
	}
	// A node id, not a branch name. deleteRef takes no name at all.
	if got, want := f.gotVars["refId"], "REF_1"; got != want {
		t.Errorf("refId = %v, want %v", got, want)
	}
}

func TestDeleteRefWrapsAFailure(t *testing.T) {
	boom := errors.New("Reference does not exist")
	f := &fakeDoer{err: boom}

	err := newWithDoer(f, nil).DeleteRef(context.Background(), "REF_1")
	if err == nil {
		t.Fatal("DeleteRef: want an error")
	}
	if !strings.Contains(err.Error(), "deleting the branch") {
		t.Errorf("error = %q, want it to name what failed", err)
	}
	if !strings.Contains(err.Error(), boom.Error()) {
		t.Errorf("error = %q, want it to carry what GitHub said", err)
	}
}
