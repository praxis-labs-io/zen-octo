package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/config"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
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

// pulseSettledCmd settles one over the pull request the screen is showing.
func (m Model) pulseSettledCmd(moved bool) tea.Cmd {
	_, cmd := m.pulseSettled(m.detail.PullRequest().ID, moved)
	return cmd
}

// onTheCommitsTab is a detail screen with a commit under the cursor whose diff
// has not landed, which is the state SetDetail arms the debounce from.
func onTheCommitsTab(t *testing.T) Model {
	t.Helper()

	model, _ := onADetail(t).Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	return model.(Model)
}

func pollConfig() *config.Config {
	return &config.Config{Defaults: config.Defaults{PRsLimit: 20}, Theme: "rose-pine-moon"}
}
