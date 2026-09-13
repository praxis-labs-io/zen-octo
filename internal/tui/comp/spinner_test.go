package comp_test

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
)

func tick(t *testing.T, cmd tea.Cmd) spinner.TickMsg {
	t.Helper()

	if cmd == nil {
		t.Fatal("no command, want a tick")
	}
	msg, ok := cmd().(spinner.TickMsg)
	if !ok {
		t.Fatalf("command produced %T, want a spinner tick", cmd())
	}
	return msg
}

func TestTheChainRunsWhileLoadingAndStopsWhenItIsNot(t *testing.T) {
	s := comp.NewSpinner(testTheme)

	first := s.Render("")
	next := s.Advance(tick(t, s.Tick()), true)
	if next == nil {
		t.Fatal("the chain ended while something was still loading")
	}
	if s.Render("") == first {
		t.Error("the glyph did not move")
	}

	if again := s.Advance(tick(t, next), false); again != nil {
		t.Error("the chain re-armed with nothing loading")
	}
}

func TestASpinnerIgnoresAnotherSpinnersTick(t *testing.T) {
	mine := comp.NewSpinner(testTheme)
	theirs := comp.NewSpinner(testTheme)

	before := mine.Render("")
	if cmd := mine.Advance(tick(t, theirs.Tick()), true); cmd != nil {
		t.Error("a tick from another spinner re-armed this one's chain")
	}
	if mine.Render("") != before {
		t.Error("a tick from another spinner moved this one's glyph")
	}
}

func TestALabelSitsBesideTheGlyph(t *testing.T) {
	s := comp.NewSpinner(testTheme)

	bare := s.Render("")
	labelled := s.Render("Loading pull requests")

	if !strings.Contains(labelled, "Loading pull requests") {
		t.Error("the label is missing")
	}
	if !strings.HasPrefix(labelled, bare) {
		t.Error("the glyph does not lead the line")
	}
}

func TestTheAccentLabelIsNotTheMutedOne(t *testing.T) {
	s := comp.NewSpinner(testTheme)

	accent := s.RenderAccent("Refreshing")
	if !strings.Contains(accent, fgSeq(testTheme.Accent)) {
		t.Errorf("RenderAccent() = %q, want the label in the accent", accent)
	}
	if strings.Contains(accent, fgSeq(testTheme.Subtle)) {
		t.Errorf("RenderAccent() = %q, want nothing left in the muted grey", accent)
	}
	if got := s.Render("Refreshing"); !strings.Contains(got, fgSeq(testTheme.Subtle)) {
		t.Errorf("Render() = %q, want the label still receding", got)
	}
}
