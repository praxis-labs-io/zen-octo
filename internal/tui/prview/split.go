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
// is refused where the pane is too narrow, and says how many columns short. A
// diff still on its way is not a refusal: the reader asked for a mode, not for a
// fact about request ordering, so the answer is held and splitting() applies it
// when the files land. Too narrow when they do is the resize fallback's case and
// answers the way that one does, silently and reversibly.
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

	r.text = m.halves(r, m.column, gutter, width, nil, nil)
	return r
}

// halves joins the two columns of a row. Only the column named takes the fill:
// lit across both, a rewritten line says nothing about which side.
func (m Model) halves(r diffRow, column gh.DiffSide, gutter, width int, fill, bar color.Color) string {
	left, right := m.columns(width)

	l, rt := r.line, r.right
	if column == gh.SideLeft {
		l.Fill, l.Bar = fill, bar
	} else {
		rt.Fill, rt.Bar = fill, bar
	}

	// The rule never lights, or the lit block runs a cell past its column.
	rule := lipgloss.NewStyle().Foreground(m.theme.MutedOrSubtle()).Render(splitRule)
	return m.painter.Half(l, gutter, left) + rule + m.painter.Half(rt, gutter, right)
}

// walkColumn is the column a run is walked in and the rows it has there. A block
// the focused column has none of is walked in the other one: the column is a
// side of the diff to read and not a claim about where a file has content, and a
// newly added file has every line of it on one side. Without this a reader whose
// column is the base reaches such a file, presses j at code plainly on the
// screen, and the client answers with nothing.
func (m Model) walkColumn(r run, split bool) (gh.DiffSide, int) {
	column := m.cursorColumn(split)
	if rows := r.codeRows(column); rows > 0 || !split {
		return column, rows
	}

	other := gh.SideRight
	if column == gh.SideRight {
		other = gh.SideLeft
	}
	return other, r.codeRows(other)
}

// stepColumn lights the named column and keeps the cursor on a row that has a
// line in it. Three things have to hold before the key is taken. The tab has to
// be the one with columns, because splitting() reads a remembered file and a
// width and both outlive a tab change. The pane has to be the one the columns
// are in. And the cursor has to be in the code: on a block the two columns draw
// the same frame, so claiming the key there is a press that shows the reader
// nothing and leaves the file column two presses away.
//
// The last of those is asked of the render rather than of m.column, and it is
// the render that answers whether the step took: walkColumn walks a block the
// focused column has no rows in in the other one, so on a file with every line
// on one side the column moves and the bar does not. Comparing the asked-for
// column against the walked one catches the first; drawing and comparing again
// catches the second, which no field on the model can be read for.
func (m *Model) stepColumn(to gh.DiffSide) bool {
	if m.tab != tabFiles || !m.splitting() || m.focus != paneMain {
		return false
	}
	if !m.walkedInto(m.pageRing.on) || m.walkedColumn() == to {
		return false
	}

	from, was := m.column, m.walkedColumn()
	m.column = to
	m.syncContent()

	// Nothing moved, so the key is the pane's. Put the column back, or the next
	// press reads a side the reader never arrived in.
	if m.walkedColumn() == was {
		m.column = from
		m.syncContent()
		return false
	}

	// The two columns do not number their rows the same, and a block with none
	// in this one has no row to stand on. The walk clamps to what that render
	// measured, and 0 puts the cursor back on the block's own head rather than
	// leaving it naming a row nothing draws.
	if at := min(m.diffCursor, m.diffRows(m.pageRing.on)); at != m.diffCursor {
		m.point(at)
		m.syncContent()
	}

	m.showDiffCursor()
	return true
}
