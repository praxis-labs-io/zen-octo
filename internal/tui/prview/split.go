package prview

import (
	"image/color"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/paint"
	"github.com/praxis-labs-io/zen-octo/internal/tui/syntax"
)

// splitCodeMin is the narrowest column of source side-by-side is offered at.
// Under it the two halves clip away more than they show.
const splitCodeMin = 28

// splitRule divides the two columns. Two untinted context halves have no tint
// between them saying where one ends.
const splitRule = "│"

// splitting is whether the diff draws side-by-side, which takes both the
// reader's answer and the width to honour it.
func (m Model) splitting() bool { return m.split && m.splitShort() == 0 }

// splitShort is how many more columns the pane needs for two halves, and 0 when
// it has them.
func (m Model) splitShort() int {
	f := m.shownFile()
	if f == nil {
		return 0
	}

	gutter := paint.Gutter(widest(*f))
	want := 2*(paint.HalfColumn(gutter)+splitCodeMin) + lipgloss.Width(splitRule)
	return max(0, want-m.bodyWidth())
}

// columns is the width of each side-by-side column, the rule between them taken
// off the pane first. An odd cell goes to the head, that being the side read.
func (m Model) columns(width int) (int, int) {
	w := max(0, width-lipgloss.Width(splitRule))
	return w / 2, w - w/2
}

// toggleSplit turns side-by-side on or off. Turning it off always takes; on it
// is refused where the pane is too narrow, and says how many columns short.
func (m *Model) toggleSplit() tea.Cmd {
	if !m.split {
		if short := m.splitShort(); short > 0 {
			return func() tea.Msg { return SplitTooNarrowMsg{Short: short} }
		}
	}

	// The rows are numbered per mode, and the two modes do not number them the
	// same. The block the cursor is on is the one thing that carries over.
	m.split = !m.split
	m.unpoint()
	m.syncContent()
	m.showFocus(&m.pageRing, &m.view, bodyTop(&m.view))
	return nil
}

// pair is the diff lines one row draws, by index into a hunk's own. -1 is a
// column with no line, which only side-by-side has.
type pair struct{ left, right int }

// pairs is the rows a hunk draws as. Unified is one line per row; split puts a
// run of removals against the additions after it and pads the shorter side.
func pairs(lines []gh.DiffLine, split bool) []pair {
	if !split {
		out := make([]pair, len(lines))
		for i := range lines {
			out[i] = pair{left: i, right: -1}
		}
		return out
	}

	out := make([]pair, 0, len(lines))
	var rem, add []int

	flush := func() {
		for i := range max(len(rem), len(add)) {
			p := pair{left: -1, right: -1}
			if i < len(rem) {
				p.left = rem[i]
			}
			if i < len(add) {
				p.right = add[i]
			}
			out = append(out, p)
		}
		rem, add = nil, nil
	}

	for i, l := range lines {
		switch l.Kind {
		case gh.DiffRemoved:
			// Additions already banked close the run, so a removal after them opens
			// a new one rather than pairing against a block it did not replace.
			if len(add) > 0 {
				flush()
			}
			rem = append(rem, i)
		case gh.DiffAdded:
			add = append(add, i)
		default:
			flush()
			out = append(out, pair{left: i, right: i})
		}
	}
	flush()

	return out
}

// sides is the lines a row draws, the base first, so a comment scoped to either
// hangs under the row holding it. A context line is named once.
func sides(p pair) []int {
	switch {
	case p.left < 0:
		return []int{p.right}
	case p.right < 0 || p.right == p.left:
		return []int{p.left}
	}
	return []int{p.left, p.right}
}

// splitRow is one side-by-side row: the base column, the rule, and the head.
// A pair with no line on a side draws a blank column rather than shifting up.
func (m Model) splitRow(lines []gh.DiffLine, p pair, tokens [][]syntax.Token, gutter, width int) diffRow {
	var r diffRow
	if p.left >= 0 {
		l := lines[p.left]
		r.line = paint.Line{Kind: kindOf(l.Kind), Old: l.Old, Tokens: tokens[p.left]}
	}
	if p.right >= 0 {
		l := lines[p.right]
		r.right = paint.Line{Kind: kindOf(l.Kind), New: l.New, Tokens: tokens[p.right]}
	}

	r.text = m.halves(r, gutter, width, nil, nil)
	return r
}

// halves joins the two columns of a row. Only the column the cursor is in takes
// the fill: lit across both, a rewritten line says nothing about which side.
func (m Model) halves(r diffRow, gutter, width int, fill, bar color.Color) string {
	left, right := m.columns(width)

	l, rt := r.line, r.right
	if m.column == gh.SideLeft {
		l.Fill, l.Bar = fill, bar
	} else {
		rt.Fill, rt.Bar = fill, bar
	}

	// The rule never lights, or the lit block runs a cell past its column.
	rule := lipgloss.NewStyle().Foreground(m.theme.MutedOrSubtle()).Render(splitRule)
	return m.painter.Half(l, gutter, left) + rule + m.painter.Half(rt, gutter, right)
}

// stepColumn lights the named column and brings the cursor onto a row that has
// a line in it. Only a split diff not already there takes the key.
func (m *Model) stepColumn(to gh.DiffSide) bool {
	if !m.splitting() || m.focus != paneMain || m.column == to {
		return false
	}

	m.column = to
	m.syncContent()
	m.showDiffCursor()
	return true
}
