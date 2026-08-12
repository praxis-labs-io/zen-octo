package gh_test

import (
	"context"
	"fmt"
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

	assertValidated(t, err)
}

// assertValidated reads a rejection for which step it failed at.
func assertValidated(t *testing.T, err error) {
	t.Helper()

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

// TestLiveTheAddReplyDocumentMatchesTheSchema is the check above, for the reply
// mutation. Same reasoning, same bad node id, and nothing written either way.
//
//	ZEN_OCTO_LIVE=1 go test ./internal/gh/ -run TestLive -v
func TestLiveTheAddReplyDocumentMatchesTheSchema(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := gh.New()
	if err != nil {
		t.Fatalf("gh.New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = client.AddReply(ctx, "NOT_A_NODE", "zen-octo schema check, never posted")
	if err == nil {
		t.Fatal("replying to a thread that does not exist came back as a success")
	}

	assertValidated(t, err)
}

// TestLiveTheThreadResolveDocumentsMatchTheSchema is the same check for both
// halves of the resolve toggle. It is the only thing that catches the input
// field being named wrong: canned JSON decodes whatever it is sent.
//
//	ZEN_OCTO_LIVE=1 go test ./internal/gh/ -run TestLive -v
func TestLiveTheThreadResolveDocumentsMatchTheSchema(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := gh.New()
	if err != nil {
		t.Fatalf("gh.New() error = %v", err)
	}

	for _, resolved := range []bool{true, false} {
		t.Run(fmt.Sprintf("resolved=%v", resolved), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			_, err := client.SetThreadResolved(ctx, "NOT_A_NODE", resolved)
			if err == nil {
				t.Fatal("resolving a thread that does not exist came back as a success")
			}

			assertValidated(t, err)
		})
	}
}

// TestLiveTheSetLabelsDocumentMatchesTheSchema is the same check for the label
// write. Nothing is written: the id belongs to no pull request, so the document
// validates and then fails to resolve.
//
//	ZEN_OCTO_LIVE=1 go test ./internal/gh/ -run TestLive -v
func TestLiveTheSetLabelsDocumentMatchesTheSchema(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := gh.New()
	if err != nil {
		t.Fatalf("gh.New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = client.SetLabels(ctx, "NOT_A_NODE", []string{"NOT_A_LABEL"})
	if err == nil {
		t.Fatal("labelling a pull request that does not exist came back as a success")
	}

	assertValidated(t, err)
}

// TestLiveTheSetAssigneesDocumentMatchesTheSchema is the SetLabels test's twin,
// and exists for the same reason: both ride updatePullRequest, so a wrong field
// name in either is caught by the schema rather than by a reader pressing the
// key. Nothing is written.
//
//	ZEN_OCTO_LIVE=1 go test ./internal/gh/ -run TestLive -v
func TestLiveTheSetAssigneesDocumentMatchesTheSchema(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := gh.New()
	if err != nil {
		t.Fatalf("gh.New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = client.SetAssignees(ctx, "NOT_A_NODE", []string{"NOT_A_USER"})
	if err == nil {
		t.Fatal("assigning a pull request that does not exist came back as a success")
	}

	assertValidated(t, err)
}

// TestLiveTheRepoMetaQueryMatchesTheSchema reads rather than writes, so it runs
// against a real repository and proves every field resolves. It asserts the
// shape rather than the contents: labels and branches change under it.
//
//	ZEN_OCTO_LIVE=1 go test ./internal/gh/ -run TestLive -v
func TestLiveTheRepoMetaQueryMatchesTheSchema(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := gh.New()
	if err != nil {
		t.Fatalf("gh.New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := client.RepoMeta(ctx, "zen-octo/zen-octo")
	if err != nil {
		t.Fatalf("RepoMeta: %v", err)
	}

	// A repository can genuinely have no labels, so the count proves nothing.
	// The rate limit is what says the query resolved rather than came back an
	// empty shell.
	if res.RateLimit.Limit == 0 {
		t.Error("no rate limit came back, so the query is not selecting it")
	}
	for _, l := range res.Meta.Labels {
		if l.ID == "" {
			t.Errorf("label %q came back with no node id, which the write path needs", l.Name)
		}
	}

	// The assignee write sets people by node id and has no spelling that takes
	// a login, so a user without one is a row the picker can show and never
	// apply. Every repository has at least the viewer here, which is why this
	// one can assert the list is not empty where the labels cannot.
	if len(res.Meta.Users) == 0 {
		t.Error("no assignable users came back, so the query is not selecting them")
	}
	for _, u := range res.Meta.Users {
		if u.ID == "" {
			t.Errorf("user %q came back with no node id, which the write path needs", u.Login)
		}
	}
}

// TestLiveTheStateDocumentsMatchTheSchema checks all four transitions the same
// way, against a node id that belongs to nothing. Four documents rather than
// one, and each aliases its own payload, so a typo in any of them would
// otherwise only surface the first time somebody pressed that menu item.
//
//	ZEN_OCTO_LIVE=1 go test ./internal/gh/ -run TestLive -v
func TestLiveTheStateDocumentsMatchTheSchema(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := gh.New()
	if err != nil {
		t.Fatalf("gh.New() error = %v", err)
	}

	for _, to := range []gh.PRTransition{
		gh.TransitionReady, gh.TransitionDraft, gh.TransitionClose, gh.TransitionReopen,
	} {
		t.Run(string(to), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			_, err := client.SetState(ctx, "NOT_A_NODE", to)
			if err == nil {
				t.Fatal("changing the state of a pull request that does not exist came back as a success")
			}

			assertValidated(t, err)
		})
	}
}

// TestLiveTheSetBaseDocumentMatchesTheSchema is the third of the
// updatePullRequest checks, and the one that needed it most: baseRefName is the
// only field on that input this client sends as a name rather than a node id,
// so a document that reached for an id would validate nowhere but here.
// Nothing is written.
//
//	ZEN_OCTO_LIVE=1 go test ./internal/gh/ -run TestLive -v
func TestLiveTheSetBaseDocumentMatchesTheSchema(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := gh.New()
	if err != nil {
		t.Fatalf("gh.New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = client.SetBase(ctx, "NOT_A_NODE", "main")
	if err == nil {
		t.Fatal("retargeting a pull request that does not exist came back as a success")
	}

	assertValidated(t, err)
}

// TestLiveTheBranchSearchMatchesTheSchema runs the real search, because unlike
// the mutations there is nothing to write and no id to get wrong.
//
// It holds two things the schema cannot. GitHub takes an orderBy on refs/heads
// and ignores it, so the order has to be built here, and the query argument has
// to match a substring of the name rather than a prefix. Both are load-bearing
// and both are only observable against a repository with real branches.
//
//	ZEN_OCTO_LIVE=1 go test ./internal/gh/ -run TestLive -v
func TestLiveTheBranchSearchMatchesTheSchema(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := gh.New()
	if err != nil {
		t.Fatalf("gh.New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// A repository with thousands of branches, which is the case the search
	// exists for: its first page alphabetically is names nobody would look for.
	res, err := client.Branches(ctx, "microsoft/vscode", "notebook")
	if err != nil {
		t.Fatalf("Branches: %v", err)
	}

	if len(res.Branches) == 0 {
		t.Fatal("the search matched nothing, which this repository cannot be true of")
	}
	if res.Default == "" {
		t.Error("the default branch came back empty")
	}
	if res.Query != "notebook" {
		t.Errorf("Query = %q, want the search it answers", res.Query)
	}

	// Mid-name, not a prefix. The picker's own filter matches the same way, and
	// a prefix search would disagree with it on every branch under a handle.
	var midName bool
	for _, b := range res.Branches {
		if !strings.HasPrefix(strings.ToLower(b), "notebook") {
			midName = true
		}
		if !strings.Contains(strings.ToLower(b), "notebook") {
			t.Errorf("the search returned %q, which does not carry the query", b)
		}
	}
	if !midName {
		t.Log("every match was a prefix, so this run did not prove the substring rule")
	}

	if res.More <= 0 {
		t.Errorf("More = %d, want the overflow reported on a repository this size", res.More)
	}
}
