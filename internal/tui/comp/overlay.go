package comp

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
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
	over = clip(over, width, height)

	x := (width - lipgloss.Width(over)) / 2
	y := (height - lipgloss.Height(over)) / 2
	return At(base, over, x, y, width, height)
}

// At composites over on top of base with its top-left corner at (x, y), held
// inside a frame of the given size. Over is this with the corner worked out
// rather than handed in.
func At(base, over string, x, y, width, height int) string {
	if width <= 0 || height <= 0 {
		return base
	}
	over = clip(over, width, height)

	// Clamped against the clipped size, never the size asked for: an overlay
	// wider than the frame is cut down first, and a corner measured before that
	// puts what is left of it off the right edge.
	x = max(0, min(x, width-lipgloss.Width(over)))
	y = max(0, min(y, height-lipgloss.Height(over)))

	out := lipgloss.NewCompositor(
		lipgloss.NewLayer(base),
		lipgloss.NewLayer(over).X(x).Y(y).Z(1),
	).Render()

	// The compositor trims each line's trailing spaces, so a base line that ends
	// in padding rather than in a border rune comes back short and the frame no
	// longer fills the width it was given. Every pane line ends in a border; the
	// header pinned above them does not.
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		lines[i] = line + strings.Repeat(" ", max(0, width-lipgloss.Width(line)))
	}
	return strings.Join(lines, "\n")
}

// clip holds an overlay inside the frame. One larger than it would otherwise
// grow the composite past the terminal, which is the one thing the frame must
// never do.
func clip(over string, width, height int) string {
	return lipgloss.NewStyle().MaxWidth(width).MaxHeight(height).Render(over)
}

// Modal frames content as a dialog for Over to place. It is a focused pane, so
// modals, pickers, and confirms all inherit the same chrome.
func Modal(th theme.Theme, title, content string) string {
	padded := lipgloss.NewStyle().Padding(0, 1).Render(content)
	w, h := lipgloss.Size(padded)
	return NewPane(th).Title(title).Focus(true).Size(w+2, h+2).Render(padded)
}
