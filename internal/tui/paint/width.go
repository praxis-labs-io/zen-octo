package paint

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

const gutterMin = 2

// Gutter is the line-number column width for a file whose highest line number is widest.
func Gutter(widest int) int {
	return max(gutterMin, len(strconv.Itoa(widest)))
}

// Clip truncates content to width, always ending in an ellipsis styled by mark.
// Callers wanting fitting content left alone check the width first.
func Clip(content string, width int, mark lipgloss.Style) string {
	switch {
	case width <= 0:
		return ""
	case width == 1:
		return mark.Render("…")
	}
	cut := lipgloss.NewStyle().MaxWidth(width - 1).Render(content)

	if lipgloss.Width(content) > width-1 {
		if gap := width - 1 - lipgloss.Width(cut); gap > 0 {
			cut += mark.Render(strings.Repeat(" ", gap))
		}
	}
	return cut + mark.Render("…")
}

// CodeColumn is the cell where source starts in a row painted by Line.
func CodeColumn(gutter int) int {
	return gutter*2 + 5
}

// HalfColumn is the cell where source starts in a row painted by Half.
func HalfColumn(gutter int) int {
	return gutter + 4
}

const markerSlot = 2

func number(n, width int) string {
	if n == 0 {
		return strings.Repeat(" ", width)
	}
	s := strconv.Itoa(n)
	return strings.Repeat(" ", max(0, width-len(s))) + s
}
