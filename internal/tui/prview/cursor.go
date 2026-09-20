package prview

import "github.com/praxis-labs-io/zen-octo/internal/gh"

func (m Model) diffDriving() bool {
	return m.tab == tabFiles && m.focus == paneMain && m.files.Loaded && m.pageRing.stops() > 0
}

func (m Model) diffRows(key focusKey) int { return m.diff.rows[key] }

func (m Model) walkedColumn() gh.DiffSide { return m.diff.columns[m.pageRing.on] }

func (m Model) cursorColumn(split bool) gh.DiffSide {
	if !split {
		return ""
	}
	return m.column
}

func (m Model) diffAt() int {
	if m.tab != tabFiles || m.pageRing.on != m.diffOn {
		return 0
	}
	return min(m.diffCursor, m.diffRows(m.pageRing.on))
}

// A render asks this rather than diffAt, whose row counts that render is still measuring.
func (m Model) walkedInto(key focusKey) bool {
	return m.tab == tabFiles && m.diffOn == key && m.diffCursor > 0
}

func (m *Model) point(rows int) {
	if m.pageRing.on != m.selection.on {
		m.selection = span{}
	}
	m.diffCursor, m.diffOn = rows, m.pageRing.on
}

func (m *Model) unpoint() { m.diffOn, m.selection = focusKey{}, span{} }

func (m *Model) moveDiffCursor(delta int) bool {
	if !m.diffDriving() {
		return false
	}

	if !m.cursorShown() {
		m.unpoint()
		return m.stepFocus(delta)
	}

	at := m.diffAt()
	switch {
	case delta > 0 && at < m.diffRows(m.pageRing.on):
		m.point(at + 1)
	case delta > 0:
		if !m.advanceFocus(1) {
			return false
		}
		m.unpoint()
	case at > 0:
		m.point(at - 1)
	default:
		if !m.advanceFocus(-1) {
			return false
		}
		m.point(m.diffRows(m.pageRing.on))
	}

	m.syncContent()
	m.showDiffCursor()
	return true
}

func (m *Model) advanceFocus(delta int) bool {
	top := bodyTop(&m.view)
	if !m.pageRing.advance(delta) {
		return false
	}

	m.syncContent()
	m.showFocus(&m.pageRing, &m.view, top)
	return true
}

func (m Model) cursorShown() bool {
	if m.diff.cursorLine < 0 {
		return false
	}

	line, top := contentLead+m.diff.cursorLine, m.view.YOffset()
	return line >= top && line < top+max(1, m.view.Height())
}

// Shortest scroll on purpose: a one-row step is not a journey, unlike a ring step.
func (m *Model) showDiffCursor() {
	if m.diff.cursorLine < 0 {
		return
	}

	line := contentLead + m.diff.cursorLine
	top, height := m.view.YOffset(), max(1, m.view.Height())
	switch {
	case line < top:
		m.view.SetYOffset(line)
	case line >= top+height:
		m.view.SetYOffset(line - height + 1)
	}
}
