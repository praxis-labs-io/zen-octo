package comp

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

// Over composites over on top of base, centered in a width by height frame.
func Over(base, over string, width, height int) string {
	if width <= 0 || height <= 0 {
		return base
	}
	x, y := OverOrigin(over, width, height)
	return At(base, clip(over, width, height), x, y, width, height)
}

func OverOrigin(over string, width, height int) (x, y int) {
	if width <= 0 || height <= 0 {
		return 0, 0
	}
	over = clip(over, width, height)
	x = (width - lipgloss.Width(over)) / 2
	y = (height - lipgloss.Height(over)) / 2
	return max(0, min(x, width-lipgloss.Width(over))), max(0, min(y, height-lipgloss.Height(over)))
}

// At composites over on top of base with its top-left at (x, y), clipped and clamped inside a width by height frame.
func At(base, over string, x, y, width, height int) string {
	if width <= 0 || height <= 0 {
		return base
	}
	over = clip(over, width, height)

	x = max(0, min(x, width-lipgloss.Width(over)))
	y = max(0, min(y, height-lipgloss.Height(over)))

	out := lipgloss.NewCompositor(
		lipgloss.NewLayer(base),
		lipgloss.NewLayer(over).X(x).Y(y).Z(1),
	).Render()

	lines := strings.Split(out, "\n")
	for i, line := range lines {
		lines[i] = line + strings.Repeat(" ", max(0, width-lipgloss.Width(line)))
	}
	return strings.Join(lines, "\n")
}

func clip(over string, width, height int) string {
	return lipgloss.NewStyle().MaxWidth(width).MaxHeight(height).Render(over)
}

// ModalLead is the columns a modal's border and padding take before its content.
const ModalLead = 2

func Modal(th theme.Theme, title, content string) string {
	padded := lipgloss.NewStyle().Padding(0, 1).Render(content)
	w, h := lipgloss.Size(padded)
	return NewPane(th).Title(title).Focus(true).Size(w+2, h+2).Render(padded)
}
