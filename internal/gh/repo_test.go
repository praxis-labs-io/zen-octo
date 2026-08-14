package gh

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

const repoMetaBody = `{
  "rateLimit": {"limit": 5000, "cost": 1, "remaining": 4900, "resetAt": "2026-08-04T18:00:00Z"},
  "repository": {
    "labels": {"nodes": [
      {"id": "LA_1", "name": "bug", "color": "d73a4a"},
      {"id": "LA_2", "name": "enhancement", "color": "a2eeef"}
    ]},
    "assignableUsers": {"nodes": [
      {"id": "U_1", "login": "drucial"},
      {"id": "U_2", "login": "nkr"}
    ]},
    "mentionableUsers": {"nodes": [
      {"login": "drucial", "name": "Drew White"},
      {"login": "nkr", "name": null},
      {"login": "outsider", "name": "Sam Reed"}
    ]},
    "mergeCommitAllowed": false,
    "squashMergeAllowed": true,
    "rebaseMergeAllowed": true,
    "deleteBranchOnMerge": true
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
	if !strings.Contains(f.gotQuery, "labels(first: 100)") {
		t.Error("query does not ask for the repository's labels")
	}
	if !strings.Contains(f.gotQuery, "assignableUsers(first: 100)") {
		t.Error("query does not ask for the repository's assignable users")
	}
}

// The whole selection in one assertion. A substring test for "name" would pass
// off the labels and one for "id" would pass off them too, so the only thing
// that pins the connection, the page cap, both fields and the absence of an id
// is the selection written out.
func TestRepoMetaAsksForTheMentionableUsersByHandleAndName(t *testing.T) {
	f := &fakeDoer{body: repoMetaBody}

	if _, err := newWithDoer(f, nil).RepoMeta(context.Background(), "zen-octo/zen-octo"); err != nil {
		t.Fatalf("RepoMeta: %v", err)
	}

	want := "mentionableUsers(first: 100) { nodes { login name } }"
	if !strings.Contains(f.gotQuery, want) {
		t.Errorf("query does not ask for %q:\n%s", want, f.gotQuery)
	}
}

func TestRepoMetaMapsTheMentionableUsers(t *testing.T) {
	f := &fakeDoer{body: repoMetaBody}

	res, err := newWithDoer(f, nil).RepoMeta(context.Background(), "zen-octo/zen-octo")
	if err != nil {
		t.Fatalf("RepoMeta: %v", err)
	}

	want := []Mention{
		{Login: "drucial", Name: "Drew White"},
		{Login: "nkr"},
		{Login: "outsider", Name: "Sam Reed"},
	}
	if !slices.Equal(res.Meta.Mentions, want) {
		t.Errorf("mentions = %+v, want %+v", res.Meta.Mentions, want)
	}
}

// An account that has set no name comes back null. Falling back to the login
// would make it read exactly like an account whose name is its handle, and the
// row would say the same thing twice.
func TestRepoMetaLeavesAMissingNameEmpty(t *testing.T) {
	f := &fakeDoer{body: repoMetaBody}

	res, err := newWithDoer(f, nil).RepoMeta(context.Background(), "zen-octo/zen-octo")
	if err != nil {
		t.Fatalf("RepoMeta: %v", err)
	}

	i := slices.IndexFunc(res.Meta.Mentions, func(m Mention) bool { return m.Login == "nkr" })
	if i < 0 {
		t.Fatalf("mentions = %+v, want one for nkr", res.Meta.Mentions)
	}
	if got := res.Meta.Mentions[i].Name; got != "" {
		t.Errorf("nkr's name = %q, want it empty", got)
	}
}

// The two lists are two sets, and the folds are one copy-paste apart. A login in
// one and not the other is what catches a fold written over the wrong nodes.
func TestRepoMetaKeepsTheMentionableAndAssignableListsApart(t *testing.T) {
	f := &fakeDoer{body: repoMetaBody}

	res, err := newWithDoer(f, nil).RepoMeta(context.Background(), "zen-octo/zen-octo")
	if err != nil {
		t.Fatalf("RepoMeta: %v", err)
	}

	if slices.ContainsFunc(res.Meta.Users, func(a Actor) bool { return a.Login == "outsider" }) {
		t.Errorf("users = %+v, want the mentionable-only login left out", res.Meta.Users)
	}
	if !slices.ContainsFunc(res.Meta.Mentions, func(m Mention) bool { return m.Login == "outsider" }) {
		t.Errorf("mentions = %+v, want the mentionable-only login present", res.Meta.Mentions)
	}
}

func TestRepoMetaMapsTheLabels(t *testing.T) {
	f := &fakeDoer{body: repoMetaBody}

	res, err := newWithDoer(f, nil).RepoMeta(context.Background(), "zen-octo/zen-octo")
	if err != nil {
		t.Fatalf("RepoMeta: %v", err)
	}
	meta := res.Meta

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

	if got, want := res.RateLimit.Remaining, 4900; got != want {
		t.Errorf("remaining = %d, want %d", got, want)
	}
}

func TestRepoMetaMapsTheAssignableUsers(t *testing.T) {
	f := &fakeDoer{body: repoMetaBody}

	res, err := newWithDoer(f, nil).RepoMeta(context.Background(), "zen-octo/zen-octo")
	if err != nil {
		t.Fatalf("RepoMeta: %v", err)
	}

	want := []Actor{{ID: "U_1", Login: "drucial"}, {ID: "U_2", Login: "nkr"}}
	if !slices.Equal(res.Meta.Users, want) {
		t.Errorf("users = %+v, want %+v", res.Meta.Users, want)
	}
}

func TestRepoMetaMapsTheMergeMethods(t *testing.T) {
	f := &fakeDoer{body: repoMetaBody}

	res, err := newWithDoer(f, nil).RepoMeta(context.Background(), "zen-octo/zen-octo")
	if err != nil {
		t.Fatalf("RepoMeta: %v", err)
	}

	want := MergeMethods{Merge: false, Squash: true, Rebase: true, DeleteOnMerge: true}
	if res.Meta.Methods != want {
		t.Errorf("methods = %+v, want %+v", res.Meta.Methods, want)
	}
	// A method the repository forbids must read as forbidden rather than as a
	// field nobody asked for: the form offers exactly what Allows answers true.
	if res.Meta.Methods.Allows(MergeMethodMerge) {
		t.Error("Allows(MERGE) is true on a repository that forbids merge commits")
	}
	if !res.Meta.Methods.Allows(MergeMethodSquash) {
		t.Error("Allows(SQUASH) is false on a repository that permits squashing")
	}
}

// The query asks for what the rail renders and nothing else. Every extra
// connection is billed to the reader on the first control they open, so a field
// arrives with the control that reads it. Branches never join it: they are a
// search keyed by what somebody typed, not a set fetched once per repository.
func TestRepoMetaAsksForNothingNobodyReads(t *testing.T) {
	f := &fakeDoer{body: repoMetaBody}

	if _, err := newWithDoer(f, nil).RepoMeta(context.Background(), "zen-octo/zen-octo"); err != nil {
		t.Fatalf("RepoMeta: %v", err)
	}

	if strings.Contains(f.gotQuery, "refs(") {
		t.Error("the query asks for refs, which belong to a search rather than this cache")
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
