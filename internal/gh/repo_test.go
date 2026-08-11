package gh

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const repoMetaBody = `{
  "rateLimit": {"limit": 5000, "cost": 1, "remaining": 4900, "resetAt": "2026-08-04T18:00:00Z"},
  "repository": {
    "mergeCommitAllowed": true,
    "squashMergeAllowed": true,
    "rebaseMergeAllowed": false,
    "deleteBranchOnMerge": true,
    "assignableUsers": {"nodes": [{"login": "drucial"}, {"login": "octocat"}]},
    "labels": {"nodes": [
      {"id": "LA_1", "name": "bug", "color": "d73a4a"},
      {"id": "LA_2", "name": "enhancement", "color": "a2eeef"}
    ]},
    "refs": {"nodes": [{"name": "main"}, {"name": "release/v1"}]}
  }
}`

func TestRepoMetaSendsOwnerAndName(t *testing.T) {
	f := &fakeDoer{body: repoMetaBody}

	if _, err := newWithDoer(f, nil).RepoMeta(context.Background(), "zen-octo/zen-octo"); err != nil {
		t.Fatalf("RepoMeta: %v", err)
	}

	if got, want := f.gotVars["owner"], "zen-octo"; got != want {
		t.Errorf("owner = %v, want %v", got, want)
	}
	if got, want := f.gotVars["name"], "zen-octo"; got != want {
		t.Errorf("name = %v, want %v", got, want)
	}
	if !strings.Contains(f.gotQuery, "assignableUsers") {
		t.Error("query does not ask for assignableUsers")
	}
}

func TestRepoMetaMapsEveryList(t *testing.T) {
	f := &fakeDoer{body: repoMetaBody}

	res, err := newWithDoer(f, nil).RepoMeta(context.Background(), "zen-octo/zen-octo")
	if err != nil {
		t.Fatalf("RepoMeta: %v", err)
	}
	meta := res.Meta

	if got, want := len(meta.Assignable), 2; got != want {
		t.Fatalf("assignable = %d, want %d", got, want)
	}
	if got, want := meta.Assignable[0].Login, "drucial"; got != want {
		t.Errorf("assignable[0] = %q, want %q", got, want)
	}

	if got, want := len(meta.Labels), 2; got != want {
		t.Fatalf("labels = %d, want %d", got, want)
	}
	// The id is what the write path needs and the one field nothing renders,
	// so it is the one worth asserting.
	if got, want := meta.Labels[0].ID, "LA_1"; got != want {
		t.Errorf("labels[0].ID = %q, want %q", got, want)
	}
	if got, want := meta.Labels[0].Name, "bug"; got != want {
		t.Errorf("labels[0].Name = %q, want %q", got, want)
	}

	if got, want := len(meta.Branches), 2; got != want {
		t.Fatalf("branches = %d, want %d", got, want)
	}
	if got, want := meta.Branches[1], "release/v1"; got != want {
		t.Errorf("branches[1] = %q, want %q", got, want)
	}

	want := MergeMethods{Merge: true, Squash: true, Rebase: false, DeleteBranch: true}
	if meta.Merge != want {
		t.Errorf("merge = %+v, want %+v", meta.Merge, want)
	}

	if got, want := res.RateLimit.Remaining, 4900; got != want {
		t.Errorf("remaining = %d, want %d", got, want)
	}
}

func TestRepoMetaRejectsMalformedName(t *testing.T) {
	for _, repo := range []string{"", "zen-octo", "/zen-octo", "zen-octo/"} {
		f := &fakeDoer{body: repoMetaBody}
		if _, err := newWithDoer(f, nil).RepoMeta(context.Background(), repo); err == nil {
			t.Errorf("RepoMeta(%q) = nil error, want one", repo)
		}
		if f.gotQuery != "" {
			t.Errorf("RepoMeta(%q) called GitHub anyway", repo)
		}
	}
}

// A repository the token cannot see comes back as a null node with no error.
// Reading that as an empty set would render a repository with no labels.
func TestRepoMetaNullRepositoryIsAnError(t *testing.T) {
	f := &fakeDoer{body: `{"repository": null}`}

	_, err := newWithDoer(f, nil).RepoMeta(context.Background(), "zen-octo/nope")
	if err == nil {
		t.Fatal("RepoMeta = nil error, want one")
	}
	if !strings.Contains(err.Error(), "no repository") {
		t.Errorf("error = %q, want it to name the missing repository", err)
	}
}

func TestRepoMetaWrapsTransportError(t *testing.T) {
	boom := errors.New("boom")
	f := &fakeDoer{err: boom}

	_, err := newWithDoer(f, nil).RepoMeta(context.Background(), "zen-octo/zen-octo")
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want it to wrap %v", err, boom)
	}
	if !strings.Contains(err.Error(), "repository metadata") {
		t.Errorf("error = %q, want it to name what failed", err)
	}
}
