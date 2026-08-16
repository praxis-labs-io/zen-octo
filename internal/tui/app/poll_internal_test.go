package app

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/config"
	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/prview"
)

// settleBudget is long enough for the commit debounce to answer inside.
const settleBudget = time.Second

// The chain is invisible from outside this package, which drops a tea.Tick: from
// there an armed beat and one never armed look exactly the same.
func TestTheBackgroundBeatStartsWithTheSession(t *testing.T) {
	m := New(pollConfig(), Mock{})

	if !carries[pollTickMsg](m.Init(), pollBeat+time.Second) {
		t.Error("nothing at startup arms the background beat, so nothing ever polls")
	}
}

// Every beat arms the next, and this beat asks for nothing: no section has
// answered yet. A chain that ended where it found no work would never restart.
func TestABeatArmsTheNextEvenHavingAskedForNothing(t *testing.T) {
	m := New(pollConfig(), Mock{})

	_, cmd := m.Update(pollTickMsg{at: time.Now()})
	if cmd == nil {
		t.Fatal("a beat produced no command at all, so the chain ends on the first one")
	}
	if !carries[pollTickMsg](cmd, pollBeat+time.Second) {
		t.Error("a beat does not arm the next, so the poll runs once and stops")
	}
}

// A needless relayout shows nowhere in the frame: with the same detail in hand
// SetDetail draws the same page. Arming the commit debounce is where it shows.
func TestARecheckThatMovedNothingDoesNotRebuildThePage(t *testing.T) {
	quiet := onTheCommitsTab(t).pulseSettledCmd(false)
	if carries[prview.CommitSettleMsg](quiet, settleBudget) {
		t.Error("a recheck that moved nothing rebuilt the page anyway")
	}

	loud := onTheCommitsTab(t).pulseSettledCmd(true)
	if !carries[prview.CommitSettleMsg](loud, settleBudget) {
		t.Fatal("the check is broken: one that moved something rebuilt nothing either")
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

	pr := gh.PullRequest{ID: "PR_1", Number: 1, State: gh.PRStateOpen, Repository: "zen-octo/zen-octo"}
	landed := gh.DetailResult{Detail: gh.PullRequestDetail{PullRequest: pr}}

	m := New(pollConfig(), Mock{})
	m.width, m.height = 160, 44
	m.store.DetailApplied("PR_1", landed)
	model, _ := m.open(pr)
	m = model.(Model)
	// open fetches over what it drew, and the page below has to be the only
	// thing in flight.
	m.store.DetailApplied("PR_1", landed)

	m.store.PulseApplied("PR_1", gh.PulseResult{Pulse: gh.Pulse{
		State: gh.PRStateOpen, UpdatedAt: time.Now(),
	}})
	if m.correctTimeline("PR_1", time.Now()) == nil {
		t.Fatal("setup: no page fetch went out, so there is nothing for r to adopt")
	}

	model, _ = m.refreshDetail(prview.RefreshMsg{ID: "PR_1"})
	return model.(Model)
}

// pulseSettledCmd settles one over the pull request the screen is showing.
func (m Model) pulseSettledCmd(moved bool) tea.Cmd {
	_, cmd := m.pulseSettled(m.detail.PullRequest().ID, moved)
	return cmd
}

// onTheCommitsTab is a detail screen with a commit under the cursor whose diff
// has not landed, which is the state SetDetail arms the debounce from.
func onTheCommitsTab(t *testing.T) Model {
	t.Helper()

	pr := gh.PullRequest{ID: "PR_1", Number: 1, State: gh.PRStateOpen, Repository: "zen-octo/zen-octo"}
	m := New(pollConfig(), Mock{})
	m.width, m.height = 160, 44
	m.store.DetailApplied("PR_1", gh.DetailResult{Detail: gh.PullRequestDetail{
		PullRequest: pr,
		Commits:     []gh.Commit{{SHA: "9f1c2b7", Short: "9f1c2b7", Headline: "Cap the backoff"}},
	}})

	model, _ := m.open(pr)
	m = model.(Model)
	model, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	return model.(Model)
}

func pollConfig() *config.Config {
	return &config.Config{Defaults: config.Defaults{PRsLimit: 20}, Theme: "rose-pine-moon"}
}
