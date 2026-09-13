package prview

import (
	"image/color"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/paint"
	"github.com/praxis-labs-io/zen-octo/internal/tui/syntax"
)

const splitCodeMin = 28

const splitRule = "│"

func (m Model) splitting() bool { return m.split && m.splitShort() == 0 }

func (m Model) splitShort() int {
	f := m.shownFile()
	if f == nil {
		return 0
	}

	gutter := paint.Gutter(widest(*f))
	want := 2*(paint.HalfColumn(gutter)+splitCodeMin) + lipgloss.Width(splitRule)
	return max(0, want-m.bodyWidth())
}

// An odd cell goes to the head, the side being read.
func (m Model) columns(width int) (int, int) {
	w := max(0, width-lipgloss.Width(splitRule))
	return w / 2, w - w/2
}

// A diff still in flight holds the mode rather than refusing it, for splitting to apply on landing.
func (m *Model) toggleSplit() tea.Cmd {
	if !m.split {
		if short := m.splitShort(); short > 0 {
			return func() tea.Msg { return SplitTooNarrowMsg{Short: short} }
		}
	}

	m.split = !m.split
	m.unpoint()
	m.syncContent()
	m.showFocus(&m.pageRing, &m.view, bodyTop(&m.view))
	return nil
}

// -1 is a column with no line, which only side-by-side has.
type pair struct{ left, right int }

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

func sides(p pair) []int {
	switch {
	case p.left < 0:
		return []int{p.right}
	case p.right < 0 || p.right == p.left:
		return []int{p.left}
	}
	return []int{p.left, p.right}
}

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

func (m Model) halves(r diffRow, column gh.DiffSide, gutter, width int, fill, bar color.Color) string {
	left, right := m.columns(width)

	l, rt := r.line, r.right
	if column == gh.SideLeft {
		l.Fill, l.Bar = fill, bar
	} else {
		rt.Fill, rt.Bar = fill, bar
	}

	rule := lipgloss.NewStyle().Foreground(m.theme.MutedOrSubtle()).Render(splitRule)
	return m.painter.Half(l, gutter, left) + rule + m.painter.Half(rt, gutter, right)
}

// A block with no rows in the focused column is walked in the other, so an added file stays walkable.
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

// Asks the render whether the step took: the column asked for and the column walked can differ.
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

	if m.walkedColumn() == was {
		m.column = from
		m.syncContent()
		return false
	}

	if at := min(m.diffCursor, m.diffRows(m.pageRing.on)); at != m.diffCursor {
		m.point(at)
		m.syncContent()
	}

	m.showDiffCursor()
	return true
}
