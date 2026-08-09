package gh_test

import (
	"context"
	"os"
	"strings"
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

// TestLiveDetailAndFiles covers the three calls the detail screen makes. The
// detail query is GraphQL and dies whole on one unknown field; the two diff
// calls are REST and die on a path, a media type, or a token scope instead.
// None of those failures is reachable from canned JSON.
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

	// Every comment and thread the account can reach carries an id. Nothing on
	// the real schema answers a comment without one, so an empty id here is a
	// field the query asked for under a name GitHub does not use.
	for _, item := range detail.Detail.Timeline {
		if item.Comment != nil && item.Said().ID == "" {
			t.Errorf("a %s carries a comment with no id", item.Kind)
		}
	}
	for _, thread := range detail.Detail.Threads {
		if thread.ID == "" {
			t.Errorf("a thread on %s came back with no id", thread.Path)
		}
		for _, c := range thread.Comments {
			if c.ID == "" {
				t.Errorf("a comment on %s came back with no id", thread.Path)
			}
		}
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

	commits := detail.Detail.Commits
	if len(commits) == 0 {
		t.Fatalf("#%d has no commits, which no open pull request can be", pr.Number)
	}
	for _, c := range commits {
		if c.SHA == "" || c.Short == "" {
			t.Errorf("a commit came back with no sha: %+v", c)
		}
	}

	commitFiles, err := client.CommitFiles(ctx, pr.Repository, commits[0].SHA)
	if err != nil {
		t.Fatalf("CommitFiles() error = %v", err)
	}
	if len(commitFiles.Files) == 0 {
		t.Errorf("%s changed no files, which no commit does", commits[0].Short)
	}
}

// TestLiveViewer is the same schema check over the one query that has no
// variables. A token that authenticates always has an account behind it, so an
// empty login here is the query being wrong rather than the account being odd.
//
//	ZEN_OCTO_LIVE=1 go test ./internal/gh/ -run TestLive -v
func TestLiveViewer(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := gh.New()
	if err != nil {
		t.Fatalf("gh.New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := client.Viewer(ctx)
	if err != nil {
		t.Fatalf("Viewer() error = %v", err)
	}
	if res.Viewer.Login == "" {
		t.Error("Viewer.Login is empty, want the account behind the token")
	}
	if res.RateLimit.Remaining == 0 {
		t.Error("RateLimit.Remaining is 0, want the live budget")
	}
}

// TestLiveTheAddCommentDocumentMatchesTheSchema validates the mutation without
// writing anything, by asking it to comment on a node that does not exist.
//
// The other live tests read, so they can run against the real thing freely. A
// write cannot: there is no delete beside it to clean up after one, and a test
// that leaves a comment on somebody's pull request every time it runs is worse
// than no test. So this one stops at the step that matters.
//
// GraphQL validates a document before it resolves anything. A misspelled field
// fails at that step, whatever the variables say, which is exactly how
// `rateLimit` shipped on a mutation that never worked: it is a field on Query,
// and every unit test decodes canned JSON that never notices. A well-formed
// document gets past validation and dies resolving the id instead.
//
// So an error is expected. Which error is the assertion.
//
//	ZEN_OCTO_LIVE=1 go test ./internal/gh/ -run TestLive -v
func TestLiveTheAddCommentDocumentMatchesTheSchema(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := gh.New()
	if err != nil {
		t.Fatalf("gh.New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// A node id that belongs to no repository, so nothing is written wherever
	// this runs and there is nothing to tidy up after it.
	_, err = client.AddComment(ctx, "NOT_A_NODE", "zen-octo schema check, never posted")
	if err == nil {
		t.Fatal("commenting on a node that does not exist came back as a success")
	}

	// The shapes a rejected document comes back as. Any of them means the
	// mutation is wrong, not the id.
	for _, broken := range []string{
		"doesn't exist on type",
		"Unknown argument",
		"Field must have selections",
		"Parse error",
	} {
		if strings.Contains(err.Error(), broken) {
			t.Fatalf("the document does not match the schema: %v", err)
		}
	}

	// And the shape that means it validated and then could not find the node,
	// which is the whole document proved good.
	if !strings.Contains(err.Error(), "Could not resolve to") {
		t.Logf("unexpected error shape, read it before trusting this test: %v", err)
	}
}
