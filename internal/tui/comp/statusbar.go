package comp

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/tui/paint"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// budgetLow is where the remaining GraphQL pool starts being news. Below it a
// session is close enough to the wall that the reader wants warning before a
// fetch comes back refused.
const budgetLow = 500

// StatusBar is the line pinned to the bottom of the frame. It lays out and
// clips; what goes in it is the caller's business.
type StatusBar struct {
	theme theme.Theme
	width int
}

// NewStatusBar returns an unsized bar.
func NewStatusBar(th theme.Theme) StatusBar {
	return StatusBar{theme: th}
}

// Size sets the bar's width.
func (s StatusBar) Size(width int) StatusBar {
	s.width = width
	return s
}

// Render puts left at the start of the line and right at its end. The left side
// never gives up a cell for the right: it carries the keys that get you out,
// and the right carries a readout, which is a thing to glance at rather than
// something to act on.
//
// The right takes what is left over and is clipped to it rather than dropped.
// A readout is written shortest part first, so the cells it keeps are the ones
// worth keeping.
func (s StatusBar) Render(left, right string) string { return s.render(left, right, false) }

// RenderMessage is Render with the priority flipped, for a right side saying
// something happened rather than reporting what is on screen. A toast may be
// the only account of a write that failed, and the hints beside it are a
// reminder of keys that go on working whether or not they are on the line.
func (s StatusBar) RenderMessage(left, right string) string { return s.render(left, right, true) }

func (s StatusBar) render(left, right string, rightWins bool) string {
	if s.width <= 2 {
		return ""
	}
	inner := s.width - 2
	clip := lipgloss.NewStyle().Foreground(s.theme.MutedOrSubtle())

	lw, rw := lipgloss.Width(left), lipgloss.Width(right)
	if rightWins {
		if room := inner - rw - 2; lw > room {
			left = paint.Clip(left, max(0, room), clip)
			lw = lipgloss.Width(left)
		}
	} else if room := inner - lw - 2; rw > room {
		right = paint.Clip(right, max(0, room), clip)
		rw = lipgloss.Width(right)
	}

	// Whichever side lost the first pass can still be too wide on its own, at a
	// width that holds neither. The winner is the one left standing.
	if lw+rw > inner {
		if rightWins {
			right = lipgloss.NewStyle().MaxWidth(inner).Render(right)
			rw = lipgloss.Width(right)
			left = lipgloss.NewStyle().MaxWidth(max(0, inner-rw)).Render(left)
			lw = lipgloss.Width(left)
		} else {
			left = lipgloss.NewStyle().MaxWidth(max(0, inner-rw)).Render(left)
			lw = lipgloss.Width(left)
		}
	}

	gap := max(0, inner-lw-rw)
	return " " + left + strings.Repeat(" ", gap) + right + " "
}

// Budget renders the remaining GraphQL points, and only once the pool has run
// low enough to be worth a reader's attention. A number that is fine is one
// nobody reads, and it sat on the bar taking the eye off the line beside it.
//
// Zero is a reading, not a missing one. Whether there is a budget to read at
// all is the caller's call.
func (s StatusBar) Budget(remaining int) string {
	if remaining >= budgetLow {
		return ""
	}
	return lipgloss.NewStyle().Foreground(s.theme.Warning).Render("◆ " + strconv.Itoa(remaining))
}
