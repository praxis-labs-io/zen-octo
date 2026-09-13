package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/config"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

const settleBudget = time.Second

func TestTheBackgroundBeatStartsWithTheSession(t *testing.T) {
	m := New(pollConfig(), Mock{}, testSurface, nil)

	if !carries[pollTickMsg](m.Init(), pollBeat+time.Second) {
		t.Error("nothing at startup arms the background beat, so nothing ever polls")
	}
}

func TestABeatArmsTheNextEvenHavingAskedForNothing(t *testing.T) {
	m := New(pollConfig(), Mock{}, testSurface, nil)

	_, cmd := m.Update(pollTickMsg{at: time.Now()})
	if cmd == nil {
		t.Fatal("a beat produced no command at all, so the chain ends on the first one")
	}
	if !carries[pollTickMsg](cmd, pollBeat+time.Second) {
		t.Error("a beat does not arm the next, so the poll runs once and stops")
	}
}

func TestTheChecksBeatStartsOnTheChecksTab(t *testing.T) {
	m := onADetail(t)
	model, _ := m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	_, cmd := model.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	if !carries[checksTickMsg](cmd, checksBeat+time.Second) {
		t.Error("entering Checks did not arm its beat")
	}
}

func TestTheChecksBeatStopsAfterATabSwitch(t *testing.T) {
	m := onTheChecksTab(t)
	wasDue := m.poller.checksAt

	model, _ := m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	m = model.(Model)
	if m.poller.checksAt != wasDue {
		t.Error("leaving Checks lost track of the tick still pending")
	}

	model, cmd := m.Update(checksTickMsg{at: wasDue})
	m = model.(Model)
	if cmd != nil {
		t.Error("the Checks beat rearmed after the tab was left")
	}
	if !m.poller.checksAt.IsZero() {
		t.Error("the stopped Checks chain still reads as armed")
	}

	for range 2 {
		model, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
		m = model.(Model)
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	if !carries[checksTickMsg](cmd, checksBeat+time.Second) {
		t.Error("returning to Checks after its old beat ended did not arm a new one")
	}
}

func TestReturningToChecksDoesNotArmASecondBeat(t *testing.T) {
	m := onTheChecksTab(t)
	wasDue := m.poller.checksAt

	for range 3 {
		model, _ := m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
		m = model.(Model)
	}
	model, cmd := m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	m = model.(Model)

	if cmd != nil {
		t.Error("returning to Checks armed a second beat over the one pending")
	}
	if m.poller.checksAt != wasDue {
		t.Error("returning to Checks replaced the pending beat")
	}
}

func TestASecondChecksChainDiesInsideTheInterval(t *testing.T) {
	m := onTheChecksTab(t)
	at := time.Now()
	m.poller.checksAt = at.Add(checksBeat)

	model, first := m.Update(checksTickMsg{at: at.Add(checksBeat)})
	m = model.(Model)
	_, second := m.Update(checksTickMsg{at: at.Add(checksBeat + time.Second)})

	if first == nil {
		t.Fatal("the due Checks beat did not continue its chain")
	}
	if second != nil {
		t.Error("a second Checks chain inside the interval survived")
	}
}

func TestTheChecksBeatDefersToARecentlyAnsweredBackgroundBeat(t *testing.T) {
	m := onTheChecksTab(t)
	due := time.Now()
	m.poller.checksAt = due
	m.poller.stampDetail(m.detail.PullRequest().ID, due.Add(-pollBeat))

	_, cmd := m.Update(checksTickMsg{at: due})
	if carries[pulseFetchedMsg](cmd, 50*time.Millisecond) {
		t.Error("the Checks beat rechecked a detail the background beat had just answered")
	}
}

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

func (m Model) pulseSettledCmd(moved bool) tea.Cmd {
	_, cmd := m.pulseSettled(m.detail.PullRequest().ID, moved)
	return cmd
}

func onTheCommitsTab(t *testing.T) Model {
	t.Helper()

	model, _ := onADetail(t).Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	return model.(Model)
}

func onTheChecksTab(t *testing.T) Model {
	t.Helper()

	model, _ := onTheCommitsTab(t).Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	return model.(Model)
}

func pollConfig() *config.Config {
	return &config.Config{Defaults: config.Defaults{PRsLimit: 20}}
}
