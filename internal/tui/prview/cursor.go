package prview

import "github.com/praxis-labs-io/zen-octo/internal/gh"

// diffDriving is whether the diff's row cursor has the movement keys, which is
// the diff pane holding them over a diff that has loaded.
func (m Model) diffDriving() bool {
	return m.tab == tabFiles && m.focus == paneMain && m.files.Loaded && m.pageRing.stops() > 0
}

// diffRows is how far the cursor can walk into a block, which is the code the
// last render drew under it, in the column that render drew it for. A folded
// hunk and a comment card have none, and so does a column the block has no line
// in. Every key that changes the column re-renders before the next one lands.
func (m Model) diffRows(key focusKey) int { return m.diff.rows[key] }

// cursorColumn is the column the cursor is in, and empty is a unified row,
// which names both. A comment and a fill are read on it. The mode is the body's
// rather than the model's: the Commits tab draws unified whatever Files is on.
func (m Model) cursorColumn(split bool) gh.DiffSide {
	if !split {
		return ""
	}
	return m.column
}

// diffAt is how far into the focused block the cursor has walked. A block it was
// not counted against is unwalked, and a fold takes the rows it counted.
func (m Model) diffAt() int {
	if m.tab != tabFiles || m.pageRing.on != m.diffOn {
		return 0
	}
	return min(m.diffCursor, m.diffRows(m.pageRing.on))
}

// walkedInto is whether the cursor has stepped off a block into the code under
// it. A render asks this rather than diffAt, because the row counts diffAt reads
// are the ones that render is still measuring.
func (m Model) walkedInto(key focusKey) bool {
	return m.tab == tabFiles && m.diffOn == key && m.diffCursor > 0
}

// point puts the cursor a number of rows into the block the ring is on.
func (m *Model) point(rows int) {
	m.diffCursor, m.diffOn = rows, m.pageRing.on
}

// unpoint gives up the walk, so every block reads as being at its own head.
func (m *Model) unpoint() { m.diffOn = focusKey{} }

// moveDiffCursor walks the cursor a row and reports whether it took the key. Off
// the end of a block it steps the ring and lands on the near end of the next.
func (m *Model) moveDiffCursor(delta int) bool {
	if !m.diffDriving() {
		return false
	}

	// A reader who scrolled has moved, and the row they left is not the one to
	// count from. The ring re-enters from the window and the walk starts over.
	if !m.cursorShown() {
		m.unpoint()
		return m.stepFocus(delta)
	}

	at := m.diffAt()
	switch {
	case delta > 0 && at < m.diffRows(m.pageRing.on):
		m.point(at + 1)
	case delta > 0:
		// The last block of the file is a boundary, the way both ends of the ring
		// are. The file column is what crosses to the next one, and the key goes
		// back untaken so the pane can scroll to whatever sits under the stop.
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
		// Back into the block above lands on its last row, the one the reader
		// was walking towards.
		m.point(m.diffRows(m.pageRing.on))
	}

	m.syncContent()
	m.showDiffCursor()
	return true
}

// advanceFocus steps the ring by index and brings what it landed on into view.
func (m *Model) advanceFocus(delta int) bool {
	top := bodyTop(&m.view)
	if !m.pageRing.advance(delta) {
		return false
	}

	m.syncContent()
	m.showFocus(&m.pageRing, &m.view, top)
	return true
}

// cursorShown is whether the row the cursor is on is one the reader can see.
func (m Model) cursorShown() bool {
	if m.diff.cursorLine < 0 {
		return false
	}

	line, top := contentLead+m.diff.cursorLine, m.view.YOffset()
	return line >= top && line < top+max(1, m.view.Height())
}

// showDiffCursor brings the cursor's row back on screen by the shortest scroll.
// A row at a time is not a journey, and hauling the page for one is worse.
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
