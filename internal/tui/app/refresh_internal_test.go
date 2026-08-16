package app

import (
	"errors"
	"testing"
	"time"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/prview"
)

// The diff was the odd leg out. Its first fetch is out for as long as a diff
// takes, which is exactly where r lands, and the summary named half the ask.
func TestARefreshWaitsOnADiffAlreadyOnItsWay(t *testing.T) {
	m := onADetail(t)
	if !m.store.BeginFiles("PR_1") || !m.store.BeginCommitFiles("9f1c2b7") {
		t.Fatal("setup: nothing is in flight, so there is nothing for r to adopt")
	}

	model, _ := m.refreshDetail(prview.RefreshMsg{ID: "PR_1", Files: true, SHA: "9f1c2b7"})
	got := model.(Model).detailRefreshing

	if !got.files.running() {
		t.Error("r dropped the pull request's diff, which is already on its way")
	}
	if !got.commit.running() {
		t.Error("r dropped the commit's diff, which is already on its way")
	}
}

// r adopts a page fetch a beat already sent, and pageFailedMsg was the one
// answer reaching the store and never the leg: the bar then spins for good.
func TestAFailedPageEndsTheRefreshThatAdoptedIt(t *testing.T) {
	m := adoptingAPage(t)
	if !m.detailRefreshing.running() {
		t.Fatal("setup: r adopted nothing, so there is no leg to strand")
	}

	next, _ := m.Update(pageFailedMsg{id: "PR_1", err: errors.New("502 Bad Gateway")})
	if next.(Model).detailRefreshing.running() {
		t.Error("the refresh never ends: the status bar spins for the rest of the session")
	}
}

// adoptingAPage is a detail screen with a background page fetch out, and r
// pressed over it. The debt is what a comment posted elsewhere leaves behind.
func adoptingAPage(t *testing.T) Model {
	t.Helper()

	m := onADetail(t)
	m.store.PulseApplied("PR_1", gh.PulseResult{Pulse: gh.Pulse{
		State: gh.PRStateOpen, UpdatedAt: time.Now(),
	}})
	if m.correctTimeline("PR_1", time.Now()) == nil {
		t.Fatal("setup: no page fetch went out, so there is nothing for r to adopt")
	}

	model, _ := m.refreshDetail(prview.RefreshMsg{ID: "PR_1"})
	return model.(Model)
}

// onADetail is the detail screen over a landed pull request carrying one commit,
// with nothing in flight: open fetches over what it drew, and that is answered.
func onADetail(t *testing.T) Model {
	t.Helper()

	pr := gh.PullRequest{ID: "PR_1", Number: 1, State: gh.PRStateOpen, Repository: "zen-octo/zen-octo"}
	landed := gh.DetailResult{Detail: gh.PullRequestDetail{
		PullRequest: pr,
		Commits:     []gh.Commit{{SHA: "9f1c2b7", Short: "9f1c2b7", Headline: "Cap the backoff"}},
	}}

	m := New(pollConfig(), Mock{})
	m.width, m.height = 160, 44
	m.store.DetailApplied("PR_1", landed)

	model, _ := m.open(pr)
	m = model.(Model)
	m.store.DetailApplied("PR_1", landed)
	return m
}
