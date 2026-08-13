package comp_test

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/tui/comp"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
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

// The chain has to end itself. Nothing else stops it, and a spinner turning
// over a screen that already has its answer is a lie about what is happening.
func TestTheChainRunsWhileLoadingAndStopsWhenItIsNot(t *testing.T) {
	s := comp.NewSpinner(theme.RosePineMoon)

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

// Two screens can be waiting at once, and the root hands every tick to both.
// Without the tag check each would advance the other and the pair would run at
// double speed.
func TestASpinnerIgnoresAnotherSpinnersTick(t *testing.T) {
	mine := comp.NewSpinner(theme.RosePineMoon)
	theirs := comp.NewSpinner(theme.RosePineMoon)

	before := mine.Render("")
	if cmd := mine.Advance(tick(t, theirs.Tick()), true); cmd != nil {
		t.Error("a tick from another spinner re-armed this one's chain")
	}
	if mine.Render("") != before {
		t.Error("a tick from another spinner moved this one's glyph")
	}
}

func TestALabelSitsBesideTheGlyph(t *testing.T) {
	s := comp.NewSpinner(theme.RosePineMoon)

	bare := s.Render("")
	labelled := s.Render("Loading pull requests")

	if !strings.Contains(labelled, "Loading pull requests") {
		t.Error("the label is missing")
	}
	if !strings.HasPrefix(labelled, bare) {
		t.Error("the glyph does not lead the line")
	}
}

// The status bar's spinner sits beside the key hints, and a label in the grey
// those are rendered in reads as one more hint rather than as something
// happening.
func TestTheAccentLabelIsNotTheMutedOne(t *testing.T) {
	s := comp.NewSpinner(theme.RosePineMoon)

	accent := s.RenderAccent("Refreshing")
	if !strings.Contains(accent, fgSeq(theme.RosePineMoon.Accent)) {
		t.Errorf("RenderAccent() = %q, want the label in the accent", accent)
	}
	if strings.Contains(accent, fgSeq(theme.RosePineMoon.Subtle)) {
		t.Errorf("RenderAccent() = %q, want nothing left in the muted grey", accent)
	}
	if got := s.Render("Refreshing"); !strings.Contains(got, fgSeq(theme.RosePineMoon.Subtle)) {
		t.Errorf("Render() = %q, want the label still receding", got)
	}
}
