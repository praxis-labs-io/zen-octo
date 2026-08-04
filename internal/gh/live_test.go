package gh_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/zen-octo/zen-octo/internal/gh"
)

// TestLiveSearchPullRequests runs the real query against the real schema.
// GraphQL rejects the whole document for one unknown field, so a unit test
// against canned JSON can pass while the query is dead. This is the check that
// catches that. It needs a working `gh` login.
//
//	ZEN_OCTO_LIVE=1 go test ./internal/gh/ -run TestLive -v
func TestLiveSearchPullRequests(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := gh.New()
	if err != nil {
		t.Fatalf("gh.New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := client.SearchPullRequests(ctx, "is:pr author:@me", 3)
	if err != nil {
		t.Fatalf("SearchPullRequests() error = %v", err)
	}

	// The budget comes back on every response, so it is checkable even when the
	// account has nothing matching.
	if res.RateLimit.Remaining == 0 {
		t.Error("RateLimit.Remaining is 0, want the live budget")
	}
	if res.RateLimit.Cost == 0 {
		t.Error("RateLimit.Cost is 0, want what this query charged")
	}

	if len(res.PullRequests) == 0 {
		t.Skip("the authenticated account has no pull requests to check against")
	}

	for _, pr := range res.PullRequests {
		if pr.ID == "" {
			t.Error("ID is empty, want the node id")
		}
		if pr.Number == 0 {
			t.Error("Number is 0, want the PR number")
		}
		if pr.Repository == "" {
			t.Errorf("#%d Repository is empty, want owner/name", pr.Number)
		}
		if pr.State == "" {
			t.Errorf("#%d State is empty, want OPEN, CLOSED, or MERGED", pr.Number)
		}
		if pr.UpdatedAt.IsZero() {
			t.Errorf("#%d UpdatedAt is zero, want a parsed timestamp", pr.Number)
		}
	}
}
