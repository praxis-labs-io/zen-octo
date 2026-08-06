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

// TestLiveDetailAndFiles covers the two calls the detail screen makes. The
// detail query is GraphQL and dies whole on one unknown field; the files call
// is REST and dies on a path, a media type, or a token scope instead. Neither
// failure is reachable from canned JSON.
//
//	ZEN_OCTO_LIVE=1 go test ./internal/gh/ -run TestLive -v
func TestLiveDetailAndFiles(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := gh.New()
	if err != nil {
		t.Fatalf("gh.New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Open, because the detail query compares against the head branch and a
	// merged pull request has usually had its branch deleted.
	found, err := client.SearchPullRequests(ctx, "is:pr is:open author:@me", 1)
	if err != nil {
		t.Fatalf("SearchPullRequests() error = %v", err)
	}
	if len(found.PullRequests) == 0 {
		t.Skip("the authenticated account has no pull requests to check against")
	}
	pr := found.PullRequests[0]

	detail, err := client.PullRequest(ctx, pr.ID, pr.HeadRefName)
	if err != nil {
		t.Fatalf("PullRequest() error = %v", err)
	}
	if detail.Detail.ID != pr.ID {
		t.Errorf("detail is for %q, want %q", detail.Detail.ID, pr.ID)
	}

	files, err := client.PullRequestFiles(ctx, pr.Repository, pr.Number, pr.ChangedFiles)
	if err != nil {
		t.Fatalf("PullRequestFiles() error = %v", err)
	}
	if len(files.Files) == 0 {
		t.Fatalf("#%d touched %d files, got none back", pr.Number, pr.ChangedFiles)
	}

	for _, f := range files.Files {
		if f.Path == "" {
			t.Error("a file came back with no path")
		}
		if len(f.Hunks) == 0 && f.Omitted == "" {
			t.Errorf("%s has no hunks and no reason why", f.Path)
		}
	}
}
