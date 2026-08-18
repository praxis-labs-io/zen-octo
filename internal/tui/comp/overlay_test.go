package comp_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

// filled builds a base frame of a single repeated rune, so anything of that
// rune left inside the overlay's rectangle is bleed-through.
func filled(r string, width, height int) string {
	rows := make([]string, height)
	for i := range rows {
		rows[i] = strings.Repeat(r, width)
	}
	return strings.Join(rows, "\n")
}

func TestOverKeepsTheFrameSize(t *testing.T) {
	got := comp.Over(filled("X", 60, 20), comp.Modal(theme.RosePineMoon, "Help", "j  down\nk  up"), 60, 20)

	lines := strings.Split(got, "\n")
	if len(lines) != 20 {
		t.Errorf("composited %d lines, want 20", len(lines))
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w != 60 {
			t.Errorf("line %d is %d cells wide, want 60", i, w)
		}
	}
}

func TestOverDoesNotLetTheLayerBeneathShowThrough(t *testing.T) {
	const w, h = 60, 20
	modal := comp.Modal(theme.RosePineMoon, "Help", "j  down\nk  up\nq  quit")
	mw, mh := lipgloss.Size(modal)

	got := comp.Over(filled("X", w, h), modal, w, h)

	x0, y0 := (w-mw)/2, (h-mh)/2
	for i, line := range strings.Split(got, "\n") {
		if i < y0 || i >= y0+mh {
			continue
		}
		plain := []rune(stripANSI(line))
		for x := x0; x < x0+mw && x < len(plain); x++ {
			if plain[x] == 'X' {
				t.Fatalf("base shows through the modal at row %d col %d: %q", i, x, string(plain))
			}
		}
	}
}

func TestOverLeavesTheBaseVisibleAroundTheModal(t *testing.T) {
	got := comp.Over(filled("X", 60, 20), comp.Modal(theme.RosePineMoon, "Help", "j  down"), 60, 20)

	if !strings.Contains(stripANSI(strings.Split(got, "\n")[0]), "XXXX") {
		t.Error("the top row lost the base, want the modal to cover only its own rectangle")
	}
}

func TestOverCentersTheModal(t *testing.T) {
	const w, h = 60, 20
	modal := comp.Modal(theme.RosePineMoon, "Help", "j  down\nk  up")
	mw, mh := lipgloss.Size(modal)

	lines := strings.Split(comp.Over(filled("·", w, h), modal, w, h), "\n")

	top := stripANSI(lines[(h-mh)/2])
	lead := len([]rune(top)) - len([]rune(strings.TrimLeft(top, "·")))
	if want := (w - mw) / 2; lead != want {
		t.Errorf("modal starts at column %d, want %d", lead, want)
	}
}

func TestOverReturnsTheBaseWhenThereIsNoFrame(t *testing.T) {
	base := "unsized"
	if got := comp.Over(base, "modal", 0, 0); got != base {
		t.Errorf("Over() = %q, want the base back when the frame has no size", got)
	}
}
