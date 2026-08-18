package app

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/paint"
)

const (
	// minWidth clears the rail's column whole and the merge form's own key
	// hints. A judgment: nothing fails at a width here, it degrades.
	minWidth = 56

	// minHeight is the merge form's, which is what an overlay clipped rather
	// than scrolled costs: its worst 21 rows, the status bar, and the notice.
	minHeight = 23
)

// tooSmall is the whole frame below the floor. It names the size the terminal
// is and the size it needs, and says nothing else there is room to say.
func (m Model) tooSmall() string {
	style := lipgloss.NewStyle().Foreground(m.theme.Subtle)
	text := fmt.Sprintf("the terminal is %dx%d, and this needs %dx%d",
		m.width, m.height, minWidth, minHeight)

	// The size it needs is the half worth keeping. Clipping the sentence would
	// cut that off and leave the reader the size they can already see.
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

// fit pads text out to width, clipping what will not fit. The message is the
// frame rather than something drawn in one, so it is exactly as wide as one.
func fit(text string, width int, mark lipgloss.Style) string {
	if w := lipgloss.Width(text); w <= width {
		return text + strings.Repeat(" ", width-w)
	}
	return paint.Clip(text, width, mark)
}
