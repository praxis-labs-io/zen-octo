package prview

// diffDriving is whether the diff's row cursor has the movement keys, which is
// the diff pane holding them over a diff that has loaded.
func (m Model) diffDriving() bool {
	return m.tab == tabFiles && m.focus == paneMain && m.files.Loaded && m.pageRing.stops() > 0
}

// diffRows is how far the cursor can walk into a block, which is the code drawn
// under it. A folded hunk and a comment card have none.
func (m Model) diffRows(key focusKey) int {
	rows := m.shownRows()
	if len(rows) != 1 || rows[0].file == nil {
		return 0
	}

	b, ok := m.diff.blocks[blockKey{key: rows[0].key, heading: m.diff.headings}]
	if !ok {
		return 0
	}
	for i, s := range b.stops {
		if m.stopKey(*rows[0].file, s) == key {
			return b.runs[i+1].codeRows()
		}
	}
	return 0
}

// diffAt is how far into the focused block the cursor has walked. A block it was
// not counted against is unwalked, and a fold takes the rows it counted.
func (m Model) diffAt() int {
	if m.pageRing.on != m.diffOn {
		return 0
	}
	return min(m.diffCursor, m.diffRows(m.pageRing.on))
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
		// are. The file column is what crosses to the next one.
		if !m.advanceFocus(1) {
			return true
		}
		m.unpoint()
	case at > 0:
		m.point(at - 1)
	default:
		if !m.advanceFocus(-1) {
			return true
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
