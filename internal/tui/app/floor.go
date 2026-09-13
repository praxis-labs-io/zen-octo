package app

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/paint"
)

const (
	// Clears the rail's column and the merge form's key hints. A judgment: nothing breaks narrower, it degrades.
	minWidth = 56

	// The merge form's tallest 21 rows, which an overlay clips rather than scrolls, plus the status bar and notice.
	minHeight = 23
)

func (m Model) tooSmall() string {
	style := lipgloss.NewStyle().Foreground(m.theme.Subtle)
	text := fmt.Sprintf("the terminal is %dx%d, and this needs %dx%d",
		m.width, m.height, minWidth, minHeight)

	if lipgloss.Width(text) > m.width {
		text = fmt.Sprintf("needs %dx%d", minWidth, minHeight)
	}

	lines := []string{fit(style.Render(text), m.width, style)}
	blank := strings.Repeat(" ", m.width)
	for len(lines) < m.height {
		lines = append(lines, blank)
	}
	return strings.Join(lines[:m.height], "\n")
}

func fit(text string, width int, mark lipgloss.Style) string {
	if w := lipgloss.Width(text); w <= width {
		return text + strings.Repeat(" ", width-w)
	}
	return paint.Clip(text, width, mark)
}
