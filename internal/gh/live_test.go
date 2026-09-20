package gh

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLiveSearchPullRequests(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := client.SearchPullRequests(ctx, "is:pr author:@me", 3)
	if err != nil {
		t.Fatalf("SearchPullRequests() error = %v", err)
	}

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

func TestLiveDetailAndFiles(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

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

	files, err := client.PullRequestFiles(ctx, pr.ID, pr.Repository, pr.Number, pr.ChangedFiles)
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

func TestLiveThePulseDocumentMatchesTheSchema(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	found, err := client.SearchPullRequests(ctx, "is:pr is:merged author:@me", 1)
	if err != nil {
		t.Fatalf("SearchPullRequests() error = %v", err)
	}
	if len(found.PullRequests) == 0 {
		t.Skip("the authenticated account has no pull requests to check against")
	}
	pr := found.PullRequests[0]

	res, err := client.Pulse(ctx, pr.ID)
	if err != nil {
		t.Fatalf("Pulse() error = %v", err)
	}

	if res.RateLimit.Limit == 0 {
		t.Error("no rate limit came back, so the query is not selecting it")
	}
	if res.Pulse.State != pr.State {
		t.Errorf("pulse says %q, search said %q, for the same pull request", res.Pulse.State, pr.State)
	}
	if res.Pulse.HeadRefOid == "" {
		t.Error("no head commit came back, which is what says a held diff went stale")
	}
	if res.Pulse.UpdatedAt.IsZero() {
		t.Error("no updatedAt came back, which is what says the page moved")
	}
}

func TestLiveViewer(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
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

// Targets a missing node: GraphQL validates the document before resolving the id, so nothing is written.
func TestLiveTheAddCommentDocumentMatchesTheSchema(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = client.AddComment(ctx, "NOT_A_NODE", "zen-octo schema check, never posted")
	if err == nil {
		t.Fatal("commenting on a node that does not exist came back as a success")
	}

	assertValidated(t, err)
}

func assertValidated(t *testing.T, err error) {
	t.Helper()

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

	if !strings.Contains(err.Error(), "Could not resolve to") {
		t.Logf("unexpected error shape, read it before trusting this test: %v", err)
	}
}

func TestLiveTheAddReplyDocumentMatchesTheSchema(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = client.AddReply(ctx, "NOT_A_NODE", "zen-octo schema check, never posted")
	if err == nil {
		t.Fatal("replying to a thread that does not exist came back as a success")
	}

	assertValidated(t, err)
}

func TestLiveTheThreadResolveDocumentsMatchTheSchema(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
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

func TestLiveTheSetLabelsDocumentMatchesTheSchema(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = client.SetLabels(ctx, "NOT_A_NODE", []string{"NOT_A_LABEL"})
	if err == nil {
		t.Fatal("labelling a pull request that does not exist came back as a success")
	}

	assertValidated(t, err)
}

func TestLiveTheSetAssigneesDocumentMatchesTheSchema(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = client.SetAssignees(ctx, "NOT_A_NODE", []string{"NOT_A_USER"})
	if err == nil {
		t.Fatal("assigning a pull request that does not exist came back as a success")
	}

	assertValidated(t, err)
}

func TestLiveTheRepoMetaQueryMatchesTheSchema(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := client.RepoMeta(ctx, "praxis-labs-io/zen-octo")
	if err != nil {
		t.Fatalf("RepoMeta: %v", err)
	}

	if res.RateLimit.Limit == 0 {
		t.Error("no rate limit came back, so the query is not selecting it")
	}
	for _, l := range res.Meta.Labels {
		if l.ID == "" {
			t.Errorf("label %q came back with no node id, which the write path needs", l.Name)
		}
	}

	if len(res.Meta.Users) == 0 {
		t.Error("no assignable users came back, so the query is not selecting them")
	}
	for _, u := range res.Meta.Users {
		if u.ID == "" {
			t.Errorf("user %q came back with no node id, which the write path needs", u.Login)
		}
	}
}

func TestLiveTheStateDocumentsMatchTheSchema(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	for _, to := range []PRTransition{
		TransitionReady, TransitionDraft, TransitionClose, TransitionReopen,
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

func TestLiveTheSetBaseDocumentMatchesTheSchema(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = client.SetBase(ctx, "NOT_A_NODE", "main")
	if err == nil {
		t.Fatal("retargeting a pull request that does not exist came back as a success")
	}

	assertValidated(t, err)
}

func TestLiveTheBranchSearchMatchesTheSchema(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

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

func TestLiveTheMergeDocumentsMatchTheSchema(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	for _, method := range []MergeMethod{
		MergeMethodMerge, MergeMethodSquash, MergeMethodRebase,
	} {
		t.Run(string(method), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			_, err := client.Merge(ctx, "NOT_A_NODE", MergeOptions{
				Method:          method,
				Headline:        "headline",
				Body:            "body",
				ExpectedHeadOid: "0000000000000000000000000000000000000000",
			})
			if err == nil {
				t.Fatal("merging a pull request that does not exist came back as a success")
			}

			assertValidated(t, err)
		})
	}

	t.Run("deleteRef", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := client.DeleteRef(ctx, "NOT_A_NODE"); err == nil {
			t.Fatal("deleting a branch that does not exist came back as a success")
		} else {
			assertValidated(t, err)
		}
	})
}

func TestLiveTheReviewDocumentsMatchTheSchema(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	calls := []struct {
		name string
		call func(context.Context) error
	}{
		{"start", func(ctx context.Context) error {
			_, err := client.StartReview(ctx, "NOT_A_NODE", "zen-octo schema check, never posted")
			return err
		}},
		{"submit", func(ctx context.Context) error {
			_, err := client.SubmitReview(ctx, "NOT_A_NODE", ReviewEventComment, "zen-octo schema check, never posted")
			return err
		}},
		{"discard", func(ctx context.Context) error {
			return client.DiscardReview(ctx, "NOT_A_NODE")
		}},
	}

	for _, tt := range calls {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := tt.call(ctx); err == nil {
				t.Fatal("a review on a node that does not exist came back as a success")
			} else {
				assertValidated(t, err)
			}
		})
	}
}

func TestLiveTheAddReviewThreadDocumentMatchesTheSchema(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	anchors := []struct {
		name string
		in   ReviewThreadInput
	}{
		{"one line", ReviewThreadInput{
			ReviewID: "NOT_A_NODE", Path: "README.md", Subject: SubjectLine,
			Line: 3, Side: SideRight,
		}},
		{"a range", ReviewThreadInput{
			ReviewID: "NOT_A_NODE", Path: "README.md", Subject: SubjectLine,
			Line: 9, Side: SideRight, StartLine: 3, StartSide: SideRight,
		}},
		{"a whole file", ReviewThreadInput{
			ReviewID: "NOT_A_NODE", Path: "README.md", Subject: SubjectFile,
		}},
		{"no review of its own", ReviewThreadInput{
			PullRequestID: "NOT_A_NODE", Path: "README.md", Line: 3, Side: SideRight,
		}},
	}

	for _, tt := range anchors {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			in := tt.in
			in.Body = "zen-octo schema check, never posted"

			if _, err := client.AddReviewThread(ctx, in); err == nil {
				t.Fatal("a thread on a node that does not exist came back as a success")
			} else {
				assertValidated(t, err)
			}
		})
	}
}
