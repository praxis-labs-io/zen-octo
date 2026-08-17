package comp_test

import (
	"testing"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/zen-octo/zen-octo/internal/tui/comp"
)

func TestClipFillsTheWidthItWasGiven(t *testing.T) {
	tests := []struct {
		name    string
		content string
		width   int
		want    int
	}{
		{name: "no room at all", content: "hello", width: 0, want: 0},
		{name: "room for the mark alone", content: "hello", width: 1, want: 1},
		// Clip always marks, so content that fits still ends in one.
		{name: "content that already fits", content: "hi", width: 8, want: 3},
		{name: "an ascii cut", content: "hello world", width: 6, want: 6},
		// 世 and 界 are two cells each, so a cut leaves an odd column for a rune
		// needing two and comes back one short of the edge.
		{name: "a cut landing on a two-cell rune", content: "世界です", width: 4, want: 4},
		{name: "a wider cut landing on a two-cell rune", content: "世界です", width: 6, want: 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := comp.Clip(tt.content, tt.width, lipgloss.NewStyle())
			if w := lipgloss.Width(got); w != tt.want {
				t.Errorf("Clip(%q, %d) is %d columns wide, want %d", tt.content, tt.width, w, tt.want)
			}
		})
	}
}

// The gap goes in front of the mark rather than after the content, so the mark
// stays against the edge where a reader looks for it.
func TestClipPutsTheGapBeforeTheMark(t *testing.T) {
	got := xansi.Strip(comp.Clip("世界です", 4, lipgloss.NewStyle()))

	if want := "世 …"; got != want {
		t.Errorf("Clip is %q, want %q", got, want)
	}
}
