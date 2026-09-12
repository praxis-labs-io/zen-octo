package app

import (
	"strconv"
	"testing"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

func oneDiff() gh.FilesResult {
	return gh.FilesResult{Files: []gh.ChangedFile{{Path: "main.go"}}}
}

// The store's caps are not reachable from this package to name.
const past = 60

func TestADiffOutlivingItsDetailIsNotRefetched(t *testing.T) {
	m := onADetail(t)

	pr := gh.PullRequest{ID: "PR_gone", Number: 9, Repository: "acme/rocket"}
	m.store.DetailApplied("PR_gone", gh.DetailResult{Detail: gh.PullRequestDetail{PullRequest: pr}})
	m.store.BeginFiles("PR_gone")
	m.store.FilesApplied("PR_gone", oneDiff())

	m.store.BeginPulse("PR_gone")
	m.store.PulseApplied("PR_gone", gh.PulseResult{Pulse: gh.Pulse{HeadRefOid: "deadbee"}})
	if !m.store.StaleFiles("PR_gone") {
		t.Fatal("setup: the push left no debt, so nothing would correct the diff")
	}

	for i := range past {
		id := "PR_" + strconv.Itoa(i)
		m.store.DetailApplied(id, gh.DetailResult{Detail: gh.PullRequestDetail{
			PullRequest: gh.PullRequest{ID: id},
		}})
	}
	if m.store.Detail("PR_gone").Loaded {
		t.Fatal("setup: the detail is still held, so there is nothing missing under the diff")
	}

	if cmd := m.correctFiles("PR_gone"); cmd != nil {
		t.Error("the diff was refetched with no number behind it, which GitHub 404s as repos//pulls/0")
	}
}

func TestReadingACommitDiffAgainKeepsIt(t *testing.T) {
	m := onADetail(t)

	sha := func(i int) string { return "sha_" + strconv.Itoa(i) }
	for i := range past {
		m.store.BeginCommitFiles(sha(i))
		m.store.CommitFilesApplied(sha(i), oneDiff())
	}

	oldest := sha(oldestHeld(t, past, func(i int) bool { return m.store.CommitFiles(sha(i)).Loaded }))
	m.needCommit(oldest)

	m.store.BeginCommitFiles("sha_new")
	m.store.CommitFilesApplied("sha_new", oneDiff())

	if !m.store.CommitFiles(oldest).Loaded {
		t.Errorf("%s was read again and dropped anyway, as though it had only been fetched", oldest)
	}
}

func TestReopeningAPullRequestKeepsItsDiff(t *testing.T) {
	m := onADetail(t)

	id := func(i int) string { return "PR_" + strconv.Itoa(i) }
	for i := range past {
		m.store.BeginFiles(id(i))
		m.store.FilesApplied(id(i), oneDiff())
	}

	oldest := oldestHeld(t, past, func(i int) bool { return m.store.Files(id(i)).Loaded })
	m.open(gh.PullRequest{ID: id(oldest), Number: oldest, Repository: "acme/rocket"})

	m.store.BeginFiles("PR_new")
	m.store.FilesApplied("PR_new", oneDiff())

	if !m.store.Files(id(oldest)).Loaded {
		t.Errorf("%s was reopened and its diff dropped anyway", id(oldest))
	}
}

func oldestHeld(t *testing.T, count int, loaded func(int) bool) int {
	t.Helper()

	for i := range count {
		if loaded(i) {
			return i
		}
	}
	t.Fatal("setup: the cache holds none of what it was given")
	return 0
}
