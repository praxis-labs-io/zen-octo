package paint

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// gutterMin is the narrowest a line-number column gets. A file under ten lines
// still reads better with the two columns lined up against its neighbours.
const gutterMin = 2

// Gutter is the line-number column width for a file whose highest line number is
// widest. Callers hand the result to both Line and HunkHeader so the two agree.
func Gutter(widest int) int {
	return max(gutterMin, len(strconv.Itoa(widest)))
}

// Clip truncates to width, marking the cut. It always marks: a caller that
// wants the content left alone when it fits checks the width first.
//
// A single column has room for the mark and nothing else, and MaxWidth(0) means
// no limit rather than no room.
//
// The mark carries its own style, because the content is already rendered by the
// time it is cut: a caller that restyles the result afterwards passes a plain
// one, and a caller clipping a finished row passes the row's, or its background
// stops one cell short of the edge.
func Clip(content string, width int, mark lipgloss.Style) string {
	switch {
	case width <= 0:
		return ""
	case width == 1:
		return mark.Render("…")
	}
	cut := lipgloss.NewStyle().MaxWidth(width - 1).Render(content)

	// A two-cell rune cannot half-fill the last column, so a cut landing on one
	// comes back a column short and a tinted row stops before the pane edge. The
	// gap goes in front of the mark, keeping the mark at the edge.
	if lipgloss.Width(content) > width-1 {
		if gap := width - 1 - lipgloss.Width(cut); gap > 0 {
			cut += mark.Render(strings.Repeat(" ", gap))
		}
	}
	return cut + mark.Render("…")
}

// CodeColumn is where the source starts in a painted row, past both number
// columns and the marker. A caller hanging its own block under a row indents to it.
func CodeColumn(gutter int) int {
	return gutter*2 + 5
}

// HalfColumn is where the source starts in a Half, past its one number column
// and the marker. It is CodeColumn for a side-by-side row.
func HalfColumn(gutter int) int {
	return gutter + 4
}

// markerSlot is the marker and the space after it. A two-cell marker eats that
// space rather than pushing a heading's text past the code column.
const markerSlot = 2

// number right-aligns a line number, or holds the column open on the side a line
// does not belong to.
func number(n, width int) string {
	if n == 0 {
		return strings.Repeat(" ", width)
	}
	s := strconv.Itoa(n)
	return strings.Repeat(" ", max(0, width-len(s))) + s
}
