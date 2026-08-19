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

// The Checks timer has no startup chain of its own. Entering the tab is the one
// edge that starts it, or every open session would carry an idle second timer.
func TestTheChecksBeatStartsOnTheChecksTab(t *testing.T) {
	m := onADetail(t)
	model, _ := m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	_, cmd := model.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	if !carries[checksTickMsg](cmd, checksBeat+time.Second) {
		t.Error("entering Checks did not arm its beat")
	}
}

// A Tick cannot be cancelled, so leaving lets the one already armed land. It
// must end there rather than carrying the chain onto another tab.
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

	// The old chain ended while Files was up. Coming round to Checks again
	// starts a fresh one rather than leaving the tab permanently stopped.
	for range 2 {
		model, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
		m = model.(Model)
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	if !carries[checksTickMsg](cmd, checksBeat+time.Second) {
		t.Error("returning to Checks after its old beat ended did not arm a new one")
	}
}

// A pending Tick is the chain even while its tab is away. Re-entering before it
// lands must reuse that wait rather than leave a timer and goroutine per visit.
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

// If two chains ever arrive, only the first due tick may survive. This is the
// backstop for leaving and returning before the old tab timer lands.
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

// The five-second beat may answer just before the Checks beat lands. The recent
// detail stamp suppresses a second request after the in-flight guard is gone.
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

func onTheChecksTab(t *testing.T) Model {
	t.Helper()

	model, _ := onTheCommitsTab(t).Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	return model.(Model)
}

func pollConfig() *config.Config {
	return &config.Config{Defaults: config.Defaults{PRsLimit: 20}, Theme: "rose-pine-moon"}
}
