package gh

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
)

// fakeDoer answers a GraphQL call with canned JSON or a canned error.
type fakeDoer struct {
	body string
	err  error

	gotQuery string
	gotVars  map[string]any
}

func (f *fakeDoer) DoWithContext(_ context.Context, query string, vars map[string]any, response any) error {
	f.gotQuery = query
	f.gotVars = vars
	if f.err != nil {
		return f.err
	}
	return json.Unmarshal([]byte(f.body), response)
}

const twoPRsBody = `{
  "search": {
    "nodes": [
      {
        "id": "PR_1", "number": 412, "title": "Fix auth retry",
        "url": "https://github.com/zen-octo/zen-octo/pull/412",
        "isDraft": false, "state": "OPEN",
        "createdAt": "2026-08-01T10:00:00Z", "updatedAt": "2026-08-02T11:30:00Z",
        "additions": 42, "deletions": 7, "changedFiles": 3,
        "headRefName": "fix-auth", "baseRefName": "main",
        "reviewDecision": "APPROVED",
        "author": {"login": "drucial"},
        "repository": {"nameWithOwner": "zen-octo/zen-octo"},
        "statusCheckRollup": {"nodes": [{"commit": {"statusCheckRollup": {"state": "SUCCESS"}}}]}
      },
      {
        "id": "PR_2", "number": 408, "title": "Bump deps",
        "url": "https://github.com/zen-octo/zen-octo/pull/408",
        "isDraft": true, "state": "OPEN",
        "createdAt": "2026-07-30T09:00:00Z", "updatedAt": "2026-07-31T09:00:00Z",
        "additions": 1, "deletions": 1, "changedFiles": 1,
        "headRefName": "bump", "baseRefName": "main",
        "reviewDecision": "",
        "author": null,
        "repository": {"nameWithOwner": "zen-octo/zen-octo"},
        "statusCheckRollup": {"nodes": []}
      }
    ]
  }
}`

func TestSearchPullRequestsMapsResponseToDomainTypes(t *testing.T) {
	doer := &fakeDoer{body: twoPRsBody}

	got, err := newWithDoer(doer).SearchPullRequests(context.Background(), "is:open author:@me", 20)
	if err != nil {
		t.Fatalf("SearchPullRequests() error = %v, want nil", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d pull requests, want 2", len(got))
	}

	first := got[0]
	if first.Number != 412 || first.Title != "Fix auth retry" {
		t.Errorf("first PR = #%d %q, want #412 \"Fix auth retry\"", first.Number, first.Title)
	}
	if first.Author.Login != "drucial" {
		t.Errorf("Author.Login = %q, want drucial", first.Author.Login)
	}
	if first.Repository != "zen-octo/zen-octo" {
		t.Errorf("Repository = %q, want zen-octo/zen-octo", first.Repository)
	}
	if first.Checks != CheckStateSuccess {
		t.Errorf("Checks = %q, want %q", first.Checks, CheckStateSuccess)
	}
	if first.ReviewDecision != ReviewDecisionApproved {
		t.Errorf("ReviewDecision = %q, want %q", first.ReviewDecision, ReviewDecisionApproved)
	}
	if first.State != PRStateOpen {
		t.Errorf("State = %q, want %q", first.State, PRStateOpen)
	}
	if first.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero, want the parsed timestamp")
	}

	// A deleted author and a commit with no checks are both normal, not errors.
	second := got[1]
	if second.Author.Login != "" {
		t.Errorf("Author.Login = %q, want empty for a null author", second.Author.Login)
	}
	if second.Checks != CheckStateNone {
		t.Errorf("Checks = %q, want empty when nothing reported", second.Checks)
	}
	if !second.IsDraft {
		t.Error("IsDraft = false, want true")
	}
}

func TestSearchPullRequestsSkipsNonPullRequestNodes(t *testing.T) {
	// Search returns issues in the same connection; the inline fragment leaves
	// them as empty nodes.
	doer := &fakeDoer{body: `{"search": {"nodes": [{}, {"id": "PR_1", "number": 7}, {}]}}`}

	got, err := newWithDoer(doer).SearchPullRequests(context.Background(), "is:open", 20)
	if err != nil {
		t.Fatalf("SearchPullRequests() error = %v, want nil", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d pull requests, want 1", len(got))
	}
	if got[0].Number != 7 {
		t.Errorf("Number = %d, want 7", got[0].Number)
	}
}

func TestSearchPullRequestsPassesQueryAndLimit(t *testing.T) {
	doer := &fakeDoer{body: `{"search": {"nodes": []}}`}

	if _, err := newWithDoer(doer).SearchPullRequests(context.Background(), "is:open author:@me", 5); err != nil {
		t.Fatalf("SearchPullRequests() error = %v, want nil", err)
	}

	if doer.gotVars["q"] != "is:open author:@me" {
		t.Errorf("q = %v, want the raw query unmodified", doer.gotVars["q"])
	}
	if doer.gotVars["limit"] != 5 {
		t.Errorf("limit = %v, want 5", doer.gotVars["limit"])
	}
}

func TestSearchPullRequestsNamesTheMissingScope(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Oauth-Scopes", "gist, read:org, repo")
	headers.Set("X-Accepted-Oauth-Scopes", "repo, workflow")

	doer := &fakeDoer{err: &api.HTTPError{
		StatusCode: 403,
		Message:    "Resource not accessible by personal access token",
		Headers:    headers,
	}}

	_, err := newWithDoer(doer).SearchPullRequests(context.Background(), "is:open", 20)
	if err == nil {
		t.Fatal("SearchPullRequests() error = nil, want an error")
	}

	var scopeErr *ScopeError
	if !errors.As(err, &scopeErr) {
		t.Fatalf("error = %v, want it to unwrap to a *ScopeError", err)
	}
	if len(scopeErr.Missing) != 1 || scopeErr.Missing[0] != "workflow" {
		t.Errorf("Missing = %v, want [workflow]", scopeErr.Missing)
	}
	if !strings.Contains(err.Error(), "gh auth refresh -s workflow") {
		t.Errorf("error = %q, want it to name the fix command", err)
	}
}

func TestSearchPullRequestsLeavesOtherErrorsAlone(t *testing.T) {
	doer := &fakeDoer{err: &api.HTTPError{StatusCode: 500, Message: "server blew up"}}

	_, err := newWithDoer(doer).SearchPullRequests(context.Background(), "is:open", 20)
	if err == nil {
		t.Fatal("SearchPullRequests() error = nil, want an error")
	}

	var scopeErr *ScopeError
	if errors.As(err, &scopeErr) {
		t.Errorf("error = %v, want a plain error rather than a *ScopeError", err)
	}
	if !strings.Contains(err.Error(), "server blew up") {
		t.Errorf("error = %q, want it to carry the original message", err)
	}
}
