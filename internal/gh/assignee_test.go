package gh

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const setAssigneesBody = `{
  "updatePullRequest": {
    "pullRequest": {
      "id": "PR_1",
      "assignees": {"nodes": [
        {"id": "U_1", "login": "drucial"}
      ]}
    }
  }
}`

func TestSetAssigneesSendsIDsAndReturnsTheSet(t *testing.T) {
	f := &fakeDoer{body: setAssigneesBody}

	res, err := newWithDoer(f, nil).SetAssignees(context.Background(), "PR_1", []string{"U_1"})
	if err != nil {
		t.Fatalf("SetAssignees: %v", err)
	}

	if got, want := f.gotVars["pullRequestId"], "PR_1"; got != want {
		t.Errorf("pullRequestId = %v, want %v", got, want)
	}
	ids, ok := f.gotVars["assigneeIds"].([]string)
	if !ok || len(ids) != 1 || ids[0] != "U_1" {
		t.Errorf("assigneeIds = %v, want [U_1]", f.gotVars["assigneeIds"])
	}
	if !strings.Contains(f.gotQuery, "updatePullRequest") {
		t.Error("query does not use updatePullRequest")
	}
	// rateLimit is a field on Query alone; a mutation naming it is rejected
	// whole, so the document must not carry it.
	if strings.Contains(f.gotQuery, "rateLimit") {
		t.Error("mutation selects rateLimit, which GitHub rejects")
	}

	if got, want := len(res.Assignees), 1; got != want {
		t.Fatalf("assignees = %d, want %d", got, want)
	}
	// The id as well as the login. The picker checks by id, so an answer
	// carrying only logins would reopen with nobody selected.
	if got, want := res.Assignees[0], (Actor{ID: "U_1", Login: "drucial"}); got != want {
		t.Errorf("assignees[0] = %+v, want %+v", got, want)
	}
}

// Clearing every assignee is a real write, not a call to skip. A nil slice
// would marshal to null and the non-null [ID!]! type rejects it.
func TestSetAssigneesSendsEmptyArrayNotNull(t *testing.T) {
	f := &fakeDoer{body: `{"updatePullRequest": {"pullRequest": {"id": "PR_1", "assignees": {"nodes": []}}}}`}

	res, err := newWithDoer(f, nil).SetAssignees(context.Background(), "PR_1", nil)
	if err != nil {
		t.Fatalf("SetAssignees: %v", err)
	}

	ids, ok := f.gotVars["assigneeIds"].([]string)
	if !ok {
		t.Fatalf("assigneeIds = %#v, want an empty []string", f.gotVars["assigneeIds"])
	}
	if ids == nil {
		t.Error("assigneeIds is nil, which marshals to null")
	}
	if len(ids) != 0 {
		t.Errorf("assigneeIds = %v, want empty", ids)
	}

	if res.Assignees == nil {
		t.Error("Assignees is nil, want an empty slice")
	}
	if len(res.Assignees) != 0 {
		t.Errorf("Assignees = %v, want empty", res.Assignees)
	}
}

func TestSetAssigneesMissingPullRequestIsAnError(t *testing.T) {
	f := &fakeDoer{body: `{"updatePullRequest": {"pullRequest": null}}`}

	_, err := newWithDoer(f, nil).SetAssignees(context.Background(), "PR_1", []string{"U_1"})
	if err == nil {
		t.Fatal("SetAssignees = nil error, want one")
	}
	if !strings.Contains(err.Error(), "no pull request") {
		t.Errorf("error = %q, want it to say nothing came back", err)
	}
}

func TestSetAssigneesWrapsTransportError(t *testing.T) {
	boom := errors.New("boom")
	f := &fakeDoer{err: boom}

	_, err := newWithDoer(f, nil).SetAssignees(context.Background(), "PR_1", []string{"U_1"})
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want it to wrap %v", err, boom)
	}
	if !strings.Contains(err.Error(), "setting assignees") {
		t.Errorf("error = %q, want it to name what failed", err)
	}
}

// The picker checks by node id, so every document that feeds or answers it has
// to carry one. A login without its id reopens the picker with nobody selected,
// and applying it then clears the assignees it was showing.
func TestEveryAssigneeDocumentAsksForTheID(t *testing.T) {
	for _, doc := range []struct{ name, query, want string }{
		{"SetAssignees", setAssigneesMutation, "assignees(first: 100) { nodes { id login } }"},
		{"detail", pullRequestQuery, "assignees(first: 10) { nodes { id login } }"},
	} {
		if !strings.Contains(doc.query, doc.want) {
			t.Errorf("%s does not select %q", doc.name, doc.want)
		}
	}
}
