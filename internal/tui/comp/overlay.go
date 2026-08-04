package comp

import (
	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// Over composites over on top of base, centered in a frame of the given size.
//
// The compositor works on a cell buffer rather than joining strings, which is
// what keeps the layer beneath from showing through the gaps in the one on top.
// Canvas.Compose looks like the same thing and is not: it ignores a layer's
// position and draws every layer at the origin.
func Over(base, over string, width, height int) string {
	if width <= 0 || height <= 0 {
		return base
	}

	// An overlay larger than the frame would otherwise grow the composite past
	// the terminal, which is the one thing the frame must never do.
	over = lipgloss.NewStyle().MaxWidth(width).MaxHeight(height).Render(over)

	x := max(0, (width-lipgloss.Width(over))/2)
	y := max(0, (height-lipgloss.Height(over))/2)

	return lipgloss.NewCompositor(
		lipgloss.NewLayer(base),
		lipgloss.NewLayer(over).X(x).Y(y).Z(1),
	).Render()
}

// Modal frames content as a dialog for Over to place. It is a focused pane, so
// modals, pickers, and confirms all inherit the same chrome.
func Modal(th theme.Theme, title, content string) string {
	padded := lipgloss.NewStyle().Padding(0, 1).Render(content)
	w, h := lipgloss.Size(padded)
	return NewPane(th).Title(title).Focus(true).Size(w+2, h+2).Render(padded)
}
