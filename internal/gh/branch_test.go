package gh

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

// branchesBody is five branches in the alphabetical order GitHub returns them
// in, with dates that are deliberately not in that order.
const branchesBody = `{
  "rateLimit": {"limit": 5000, "cost": 1, "remaining": 4999,
                "resetAt": "2026-08-12T15:00:00Z"},
  "repository": {
    "defaultBranchRef": {"name": "main"},
    "refs": {
      "totalCount": 41,
      "nodes": [
        {"name": "alpha",   "target": {"committedDate": "2024-07-22T20:31:05Z"}},
        {"name": "beta",    "target": {"committedDate": "2026-08-09T00:12:27Z"}},
        {"name": "gamma",   "target": {"committedDate": "2025-01-04T11:00:00Z"}},
        {"name": "orphan",  "target": null},
        {"name": "main",    "target": {"committedDate": "2026-08-11T09:00:00Z"}}
      ]
    }
  }
}`

func TestBranchesComeBackNewestFirst(t *testing.T) {
	f := &fakeDoer{body: branchesBody}

	res, err := newWithDoer(f, nil).Branches(context.Background(), "zen-octo/zen-octo", "")
	if err != nil {
		t.Fatalf("Branches: %v", err)
	}

	// GitHub sorts refs/heads alphabetically whatever orderBy it is handed, so
	// this order exists only because the client built it.
	want := []string{"main", "beta", "gamma", "alpha", "orphan"}
	if !slices.Equal(res.Branches, want) {
		t.Errorf("branches = %v, want %v", res.Branches, want)
	}
}

// A branch GitHub sent no commit date for is still a branch somebody can
// retarget onto. Dropping it makes it unreachable through the only control
// there is.
func TestABranchWithNoDateIsKeptAndSortsLast(t *testing.T) {
	f := &fakeDoer{body: branchesBody}

	res, err := newWithDoer(f, nil).Branches(context.Background(), "zen-octo/zen-octo", "")
	if err != nil {
		t.Fatalf("Branches: %v", err)
	}

	if !slices.Contains(res.Branches, "orphan") {
		t.Fatalf("branches = %v, want it to carry orphan", res.Branches)
	}
	if got := res.Branches[len(res.Branches)-1]; got != "orphan" {
		t.Errorf("last branch = %q, want orphan", got)
	}
}

func TestBranchesSendsTheSearchAndReportsTheOverflow(t *testing.T) {
	f := &fakeDoer{body: branchesBody}

	res, err := newWithDoer(f, nil).Branches(context.Background(), "zen-octo/zen-octo", "release")
	if err != nil {
		t.Fatalf("Branches: %v", err)
	}

	if got, want := f.gotVars["query"], "release"; got != want {
		t.Errorf("query = %v, want %v", got, want)
	}
	if got, want := f.gotVars["first"], branchPage; got != want {
		t.Errorf("first = %v, want %v", got, want)
	}
	if !strings.Contains(f.gotQuery, `refPrefix: "refs/heads/"`) {
		t.Error("query does not scope itself to branches")
	}

	// Query rides back so a caller can tell an answer to the search being run
	// from one to a search two keystrokes ago.
	if got, want := res.Query, "release"; got != want {
		t.Errorf("Query = %q, want %q", got, want)
	}
	// 41 matched, 5 came back.
	if got, want := res.More, 36; got != want {
		t.Errorf("More = %d, want %d", got, want)
	}
	if got, want := res.Default, "main"; got != want {
		t.Errorf("Default = %q, want %q", got, want)
	}
	if got, want := res.RateLimit.Remaining, 4999; got != want {
		t.Errorf("RateLimit.Remaining = %d, want %d", got, want)
	}
}

// A page that reached everything has no overflow. A negative count would render
// as "-4 more" beside the title.
func TestBranchesReportsNoOverflowWhenThePageReachedThemAll(t *testing.T) {
	f := &fakeDoer{body: `{"repository": {"defaultBranchRef": {"name": "main"},
	  "refs": {"totalCount": 1, "nodes": [{"name": "main", "target": null}]}}}`}

	res, err := newWithDoer(f, nil).Branches(context.Background(), "zen-octo/zen-octo", "")
	if err != nil {
		t.Fatalf("Branches: %v", err)
	}
	if got := res.More; got != 0 {
		t.Errorf("More = %d, want 0", got)
	}
}

// A repository with no default branch is one that has never been pushed to. It
// is not a failure, and the picker offers whatever branches came back.
func TestBranchesSurvivesAMissingDefaultBranch(t *testing.T) {
	f := &fakeDoer{body: `{"repository": {"defaultBranchRef": null,
	  "refs": {"totalCount": 0, "nodes": []}}}`}

	res, err := newWithDoer(f, nil).Branches(context.Background(), "zen-octo/zen-octo", "")
	if err != nil {
		t.Fatalf("Branches: %v", err)
	}
	if res.Default != "" {
		t.Errorf("Default = %q, want empty", res.Default)
	}
}

// A repository the token cannot see answers 200 with a null node. An empty
// picker would read as a repository with no branches, which is a different
// thing entirely.
func TestBranchesRefusesANullRepository(t *testing.T) {
	f := &fakeDoer{body: `{"repository": null}`}

	if _, err := newWithDoer(f, nil).Branches(context.Background(), "zen-octo/zen-octo", ""); err == nil {
		t.Fatal("Branches: want an error for a null repository")
	}
}

func TestBranchesRejectsAMalformedRepo(t *testing.T) {
	f := &fakeDoer{body: branchesBody}

	_, err := newWithDoer(f, nil).Branches(context.Background(), "zen-octo", "")
	if err == nil {
		t.Fatal("Branches: want an error for a name that is not owner/name")
	}
	if f.gotQuery != "" {
		t.Error("Branches sent a request for a repository it could not address")
	}
}

func TestBranchesWrapsAFailure(t *testing.T) {
	boom := errors.New("bad credentials")
	f := &fakeDoer{err: boom}

	_, err := newWithDoer(f, nil).Branches(context.Background(), "zen-octo/zen-octo", "")
	if err == nil {
		t.Fatal("Branches: want an error")
	}
	if !strings.Contains(err.Error(), "fetching branches") {
		t.Errorf("error = %q, want it to name what failed", err)
	}
}
