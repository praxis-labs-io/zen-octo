package comp_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
)

func bar() comp.StatusBar { return comp.NewStatusBar(testTheme) }

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

// A right side one cell too wide used to go entirely, taking the budget with
// it. The leading few cells are the ones that carry anything.
func TestStatusBarClipsTheRightSideRatherThanDroppingIt(t *testing.T) {
	got := bar().Size(45).Render("j/k move · q quit", "◆ 4821 · #412 acme/rocket")

	if !strings.Contains(got, "4821") {
		t.Errorf("Render() = %q, want the budget kept", got)
	}
	if !strings.Contains(got, "#412") {
		t.Errorf("Render() = %q, want the pull request number kept", got)
	}
	if !strings.Contains(got, "j/k move · q quit") {
		t.Errorf("Render() = %q, want the keys untouched", got)
	}
}

// A toast is the only account there is of a write that failed, so it is the one
// thing on this line that must survive a narrow terminal. The hints beside it
// go on working whether or not they are on screen.
func TestRenderMessageKeepsTheMessageAndCutsTheHints(t *testing.T) {
	const message = "Could not request a review from @drucial"

	got := bar().Size(46).RenderMessage("j/k move · ⏎ open · [/] tab · q quit", message)

	if !strings.Contains(got, message) {
		t.Errorf("RenderMessage() = %q, want the whole message kept", got)
	}
	if strings.Contains(got, "q quit") {
		t.Errorf("RenderMessage() = %q, want the hints cut to make room", got)
	}
}

// A message wider than the bar has nowhere to go but the line it is on. Leaving
// the hints their two cells pushed it past the frame.
func TestRenderMessageClipsAMessageWiderThanTheBar(t *testing.T) {
	const width = 30

	got := bar().Size(width).RenderMessage("j/k move · q quit", strings.Repeat("long ", 20))

	if w := lipgloss.Width(got); w > width {
		t.Errorf("RenderMessage() is %d wide, want no more than %d: %q", w, width, got)
	}
}

func TestBudgetWarnsWhenThePoolRunsLow(t *testing.T) {
	s := bar()

	if got := s.Budget(4821); got != "" {
		t.Errorf("Budget(4821) = %q, want nothing while the pool is healthy", got)
	}
	if got := s.Budget(120); !strings.Contains(got, fgSeq(testTheme.Warning)) {
		t.Error("a low budget is not rendered as a warning")
	}
	// An exhausted pool is the one reading the user most needs. Hiding it here
	// made the warning branch unreachable.
	if got := s.Budget(0); !strings.Contains(got, "0") || !strings.Contains(got, fgSeq(testTheme.Warning)) {
		t.Errorf("Budget(0) = %q, want a warning-colored zero", got)
	}
}

// The hints are shed to the room before the bar is handed them, so Room has to
// answer what render will actually leave the left side. Two numbers in two
// packages agreed once and nothing would have failed when they stopped: the
// hints would go back to being cut mid-word, which is the whole of what the
// shed exists to stop.
func TestRoomIsWhatTheLeftSideActuallyGets(t *testing.T) {
	b := bar().Size(40)
	message := "Refreshed 3 sections"

	tests := []struct {
		name  string
		room  int
		right string
		draw  func(left, right string) string
	}{
		{name: "Render, where the left wins", room: b.Room(), draw: b.Render},
		{name: "RenderMessage, where it does not", room: b.MessageRoom(message), right: message, draw: b.RenderMessage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Filled to exactly the room: nothing is cut and the line still
			// fits, which is the two numbers agreeing.
			left := strings.Repeat("x", tt.room)
			got := tt.draw(left, tt.right)

			if w := lipgloss.Width(got); w != 40 {
				t.Errorf("the bar is %d cells wide, want 40", w)
			}
			if !strings.Contains(got, left) {
				t.Errorf("a left side of exactly the room (%d) was cut: %q", tt.room, got)
			}
			if tt.right != "" && !strings.Contains(got, tt.right) {
				t.Errorf("the right side was cut to fit a left the room said would fit: %q", got)
			}
		})
	}
}
