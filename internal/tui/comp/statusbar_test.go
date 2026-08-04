package comp_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/tui/comp"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

func bar() comp.StatusBar { return comp.NewStatusBar(theme.RosePineMoon) }

func TestStatusBarFillsItsWidthExactly(t *testing.T) {
	tests := []struct {
		name         string
		width        int
		left, right  string
		wantRendered bool
	}{
		{name: "room for both", width: 60, left: "j/k move", right: "◆ 4821", wantRendered: true},
		{name: "nothing to show", width: 40, left: "", right: "", wantRendered: true},
		{name: "left alone overflows", width: 12, left: strings.Repeat("key ", 20), right: "", wantRendered: true},
		{name: "no width at all", width: 0, left: "j/k", right: "x", wantRendered: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bar().Size(tt.width).Render(tt.left, tt.right)

			if !tt.wantRendered {
				if got != "" {
					t.Errorf("Render() = %q, want empty", got)
				}
				return
			}
			if w := lipgloss.Width(got); w != tt.width {
				t.Errorf("Render() is %d cells wide, want %d", w, tt.width)
			}
			if strings.Contains(got, "\n") {
				t.Error("Render() wrapped, want a single line")
			}
		})
	}
}

func TestStatusBarPushesTheRightSideToTheEnd(t *testing.T) {
	got := bar().Size(40).Render("j/k move", "◆ 4821")

	if !strings.HasSuffix(got, "◆ 4821 ") {
		t.Errorf("Render() = %q, want the right side at the end", got)
	}
	if !strings.HasPrefix(got, " j/k move") {
		t.Errorf("Render() = %q, want the left side at the start", got)
	}
}

// The right side is reference material. The left side tells you how to get out,
// so it is what survives a squeeze.
func TestStatusBarDropsTheRightSideBeforeTheLeft(t *testing.T) {
	got := bar().Size(20).Render("j/k move · q quit", "◆ 4821 · My PRs")

	if strings.Contains(got, "4821") {
		t.Errorf("Render() = %q, want the right side dropped when it cannot fit", got)
	}
	if !strings.Contains(got, "j/k move") {
		t.Errorf("Render() = %q, want the keys kept", got)
	}
}

func TestBudgetWarnsWhenThePoolRunsLow(t *testing.T) {
	s := bar()

	if got := s.Budget(4821); !strings.Contains(got, fgSeq(theme.RosePineMoon.Faint)) {
		t.Error("a healthy budget is not rendered faint")
	}
	if got := s.Budget(120); !strings.Contains(got, fgSeq(theme.RosePineMoon.Warning)) {
		t.Error("a low budget is not rendered as a warning")
	}
	if got := s.Budget(0); got != "" {
		t.Errorf("Budget(0) = %q, want empty rather than a zero we never fetched", got)
	}
}
