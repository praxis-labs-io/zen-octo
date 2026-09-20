package prview

import (
	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

// The row a range runs from, in the hunk and column it was dropped in.
type span struct {
	on     focusKey
	anchor int
}

func (m Model) selecting() bool {
	return m.selection.on.kind == focusHunk &&
		m.pageRing.on == m.selection.on &&
		m.walkedInto(m.selection.on)
}

func (m Model) toggleSelection() (Model, tea.Cmd) {
	if m.selecting() {
		m.clearSelection()
		return m, nil
	}
	if !m.diffDriving() || !m.walkedInto(m.pageRing.on) {
		return m, nil
	}

	m.selection = span{on: m.pageRing.on, anchor: m.diffAt()}
	m.syncContent()
	return m, nil
}

// A resize crosses the split on its own, and a row ordinal names a different line either side of it.
func (m *Model) dropSelectionAcrossSplit(was bool) {
	if m.splitting() != was {
		m.clearSelection()
	}
}

func (m *Model) clearSelection() {
	m.selection = span{}
	m.syncContent()
}

// Measured against the row count this render found, never diffAt's, which is the render before it.
func (m Model) litSpan(r run, key focusKey, rows int, column gh.DiffSide) (int, int) {
	if key != m.selection.on || !m.selecting() {
		return 1, 0
	}

	at, anchor := min(m.diffCursor, rows), min(m.selection.anchor, rows)
	return r.rowAt(min(at, anchor), column), r.rowAt(max(at, anchor), column)
}
