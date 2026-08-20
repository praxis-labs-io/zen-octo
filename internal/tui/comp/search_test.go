package comp_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
)

func TestSearchEditsAUnicodeQuery(t *testing.T) {
	var s comp.Search
	for _, text := range []string{"失", "敗"} {
		if !s.Insert(tea.KeyPressMsg{Code: []rune(text)[0], Text: text}) {
			t.Fatalf("%q was not taken", text)
		}
	}
	if got := s.Query(); got != "失敗" {
		t.Errorf("query = %q, want 失敗", got)
	}
	s.Insert(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if got := s.Query(); got != "失" {
		t.Errorf("backspace left %q, want 失", got)
	}
	s.Insert(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	if !s.Empty() {
		t.Errorf("ctrl+u left %q", s.Query())
	}
}

func TestSearchHighlightsMatchesWithoutHidingTheLine(t *testing.T) {
	var s comp.Search
	for _, r := range "error" {
		s.Insert(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	mark := lipgloss.NewStyle().Bold(true)
	got := s.Highlight("Error: another ERROR", mark)
	if plain := xansi.Strip(got); plain != "Error: another ERROR" {
		t.Errorf("highlight changed the line to %q", plain)
	}
	if strings.Count(got, "1m") != 2 {
		t.Errorf("got %d highlighted runs, want two: %q", strings.Count(got, "1m"), got)
	}
}

func TestSearchHighlightsUnicodeWithoutSplittingRunes(t *testing.T) {
	var s comp.Search
	s.Insert(tea.KeyPressMsg{Code: 'σ', Text: "σ"})
	got := s.Highlight("Σ failure", lipgloss.NewStyle().Bold(true))
	if plain := xansi.Strip(got); plain != "Σ failure" {
		t.Errorf("highlight changed the line to %q", plain)
	}
}

func TestSearchMovesThroughMatchesAndWraps(t *testing.T) {
	var s comp.Search
	s.Insert(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if !s.Move(1, 3) || s.Cursor() != 1 {
		t.Errorf("next cursor = %d, want 1", s.Cursor())
	}
	s.Move(2, 3)
	if s.Cursor() != 0 {
		t.Errorf("wrapped cursor = %d, want 0", s.Cursor())
	}
	s.Move(-1, 3)
	if s.Cursor() != 2 {
		t.Errorf("previous cursor = %d, want 2", s.Cursor())
	}
}
