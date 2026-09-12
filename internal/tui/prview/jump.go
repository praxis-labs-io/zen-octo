package prview

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
)

func (m Model) showCheckFromRail() (Model, tea.Cmd) {
	key := m.railRing.on.id
	if key == "" {
		return m, nil
	}

	for _, g := range m.check.groups {
		if slices.ContainsFunc(g.checks, func(c gh.Check) bool { return c.Key() == key }) {
			delete(m.check.folded, checkParentKey(g.name, g.runID))
		}
	}

	m.check.selected = key
	tab := m.goToTab(tabChecks)
	m.syncChecks()

	m.focusPane(paneSide)
	for i, row := range m.check.rows {
		if row.checkKey == key {
			m.check.cursor = i
			break
		}
	}
	m.showSideCursor()

	return m, tab
}

func (m Model) showInDiff() (Model, tea.Cmd) {
	t, ok := m.threadOnRing()
	if !ok || t.Line == 0 || m.tab == tabFiles {
		return m, nil
	}

	if m.files.Loaded && !m.hasPath(t.Path) {
		path := t.Path
		return m, func() tea.Msg { return ThreadNotInDiffMsg{Path: path} }
	}

	m.jump = t.ID

	if retry := !m.files.Loaded && m.files.Status == store.StatusFailed; retry {
		m.filesAsked = false
		return m, m.goToTab(tabFiles)
	}

	tab := m.goToTab(tabFiles)
	landed := m.finishJump()
	return m, tea.Batch(tab, landed)
}

// A diff still in flight counts, or the key hides until the tab has been opened once.
func (m Model) jumpable(t gh.ReviewThread) bool {
	if t.Line == 0 || m.tab == tabFiles {
		return false
	}
	return !m.files.Loaded || m.hasPath(t.Path)
}

// Runs on the key and on every diff arriving, so a jump made early lands with the diff.
func (m *Model) finishJump() tea.Cmd {
	if m.jump == "" {
		return nil
	}

	if !m.files.Loaded {
		if m.files.Status == store.StatusFailed {
			m.jump = ""
		}
		return nil
	}

	if m.tab != tabFiles {
		m.jump = ""
		return nil
	}

	t, ok := m.threadByID(m.jump)
	m.jump = ""
	if !ok {
		return nil
	}
	if !m.hasPath(t.Path) {
		return func() tea.Msg { return ThreadNotInDiffMsg{Path: t.Path} }
	}

	m.reveal(t.Path)
	m.pointAt(t.Path)

	m.unpoint()

	m.syncContent()
	m.showCursorRow()

	if line, ok := m.threadLine(t.ID); ok {
		m.view.SetYOffset(contentLead + m.jumpTop(t.Path, line))
		return nil
	}

	return nil
}

func (m Model) threadLine(id string) (int, bool) {
	want := focusKey{kind: focusThread, id: id}
	for _, s := range m.diff.stops {
		if s.focusKey == want {
			return s.start, true
		}
	}
	return 0, false
}

const jumpLead = 4

// Opens above the card so the line it answers stays in view, and never above the file's heading.
func (m Model) jumpTop(path string, line int) int {
	top := line - jumpLead
	if at := slices.IndexFunc(m.diff.spans, func(s fileSpan) bool { return s.key == path }); at >= 0 {
		top = max(top, m.diff.spans[at].start)
	}
	return max(0, top)
}

// Unfolds every prefix, because a chain of directories collapses under its deepest key.
func (m *Model) reveal(path string) {
	segments := strings.Split(path, "/")
	for i := range segments {
		delete(m.collapsed, strings.Join(segments[:i+1], "/"))
	}
	m.syncRows()
}

func (m *Model) pointAt(path string) {
	at := slices.IndexFunc(m.rows, func(r row) bool { return r.file != nil && r.key == path })
	if at < 0 {
		return
	}
	m.cursor = at
	m.nameShownFile()
}

func (m Model) hasPath(path string) bool {
	return slices.ContainsFunc(m.files.Files, func(f gh.ChangedFile) bool { return f.Path == path })
}

func (m Model) threadByID(id string) (gh.ReviewThread, bool) {
	at := slices.IndexFunc(m.detail.Detail.Threads, func(t gh.ReviewThread) bool { return t.ID == id })
	if at < 0 {
		return gh.ReviewThread{}, false
	}
	return m.detail.Detail.Threads[at], true
}
