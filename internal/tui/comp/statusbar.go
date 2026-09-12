package comp

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/paint"
	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

const budgetLow = 500

type StatusBar struct {
	theme theme.Theme
	width int
}

// NewStatusBar returns an unsized bar.
func NewStatusBar(th theme.Theme) StatusBar {
	return StatusBar{theme: th}
}

func (s StatusBar) Size(width int) StatusBar {
	s.width = width
	return s
}

// Render puts left at the start of the line and right at its end, clipping right to the room left wins.
func (s StatusBar) Render(left, right string) string { return s.render(left, right, false) }

// RenderMessage is Render with right winning, for a toast or other message.
func (s StatusBar) RenderMessage(left, right string) string { return s.render(left, right, true) }

const (
	barPad = 1
	barGap = 2
)

// Room is the widest left may be under Render.
func (s StatusBar) Room() int { return max(0, s.width-2*barPad) }

// MessageRoom is the widest left may be under RenderMessage beside message.
func (s StatusBar) MessageRoom(message string) int {
	return max(0, s.Room()-lipgloss.Width(message)-barGap)
}

func (s StatusBar) render(left, right string, rightWins bool) string {
	if s.width <= 2 {
		return ""
	}
	inner := s.width - 2*barPad
	clip := lipgloss.NewStyle().Foreground(s.theme.MutedOrSubtle())

	lw, rw := lipgloss.Width(left), lipgloss.Width(right)
	if rightWins {
		if room := inner - rw - barGap; lw > room {
			left = paint.Clip(left, max(0, room), clip)
			lw = lipgloss.Width(left)
		}
	} else if room := inner - lw - barGap; rw > room {
		right = paint.Clip(right, max(0, room), clip)
		rw = lipgloss.Width(right)
	}

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
	pad := strings.Repeat(" ", barPad)
	return pad + left + strings.Repeat(" ", gap) + right + pad
}

// Budget renders the remaining GraphQL points, or empty until they run low.
func (s StatusBar) Budget(remaining int) string {
	if remaining >= budgetLow {
		return ""
	}
	return lipgloss.NewStyle().Foreground(s.theme.Warning).Render("◆ " + strconv.Itoa(remaining))
}
