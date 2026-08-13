package comp

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

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
// and the right carries the rate limit and what is on screen.
//
// The right takes what is left over and is clipped to it rather than dropped.
// Dropping it whole cost the budget and the pull request number together for
// want of one cell, and it is the leading few that carry those.
func (s StatusBar) Render(left, right string) string {
	if s.width <= 2 {
		return ""
	}
	inner := s.width - 2

	lw, rw := lipgloss.Width(left), lipgloss.Width(right)
	if room := inner - lw - 2; rw > room {
		right = Clip(right, max(0, room), lipgloss.NewStyle().Foreground(s.theme.MutedOrSubtle()))
		rw = lipgloss.Width(right)
	}
	if lw+rw > inner {
		left = lipgloss.NewStyle().MaxWidth(max(0, inner-rw)).Render(left)
		lw = lipgloss.Width(left)
	}

	gap := max(0, inner-lw-rw)
	return " " + left + strings.Repeat(" ", gap) + right + " "
}

// Budget renders the remaining GraphQL points. It reads faint until the pool
// runs low, because a number nobody notices is the point right up until it
// isn't.
//
// Zero is a reading, not a missing one, so it renders like any other. Whether
// there is a budget to show at all is the caller's call.
func (s StatusBar) Budget(remaining int) string {
	c := s.theme.MutedOrSubtle()
	if remaining < 500 {
		c = s.theme.Warning
	}
	return lipgloss.NewStyle().Foreground(c).Render("◆ " + strconv.Itoa(remaining))
}

// Context renders the trailing label naming what is on screen.
func (s StatusBar) Context(label string) string {
	return lipgloss.NewStyle().Foreground(s.theme.MutedOrSubtle()).Render(label)
}
