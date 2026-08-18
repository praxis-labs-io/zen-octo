package prview

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
)

// showInDiff takes the thread the ring is on to its place in the Files tab.
//
// A diff already here that does not carry the file is answered where the reader
// is standing. Switching to a tab that cannot show them what they asked for and
// saying so from there is two moves to deliver one piece of bad news.
func (m Model) showInDiff() (Model, tea.Cmd) {
	// The tab rather than jumpable, which is false for a file the diff does not
	// carry: that one is answered below with a toast rather than with silence.
	t, ok := m.threadOnRing()
	if !ok || t.Line == 0 || m.tab == tabFiles {
		return m, nil
	}

	if m.files.Loaded && !m.hasPath(t.Path) {
		path := t.Path
		return m, func() tea.Msg { return ThreadNotInDiffMsg{Path: path} }
	}

	m.jump = t.ID

	// A diff that failed is asked for again rather than landed on. Pressing v is
	// asking to see the code, the pane carries no retry of its own, and dropping
	// the jump here would leave the reader on an error with nothing said about
	// what they pressed. Clearing filesAsked is what lets the tab ask twice.
	if retry := !m.files.Loaded && m.files.Status == store.StatusFailed; retry {
		m.filesAsked = false
		return m, m.goToTab(tabFiles)
	}

	// Both taken before the return. finishJump writes the offset onto this
	// model, and a return statement is free to read its own operand before the
	// calls beside it.
	tab := m.goToTab(tabFiles)
	landed := m.finishJump()
	return m, tea.Batch(tab, landed)
}

// jumpable is whether v has somewhere to take a thread. A diff still out is
// counted in: the file is probably in it, and hiding the key until the tab has
// been opened once teaches the reader it is not there.
func (m Model) jumpable(t gh.ReviewThread) bool {
	// Inside the diff there is nowhere left to go, so the key is inert and the
	// footer that reads this stops naming it.
	if t.Line == 0 || m.tab == tabFiles {
		return false
	}
	return !m.files.Loaded || m.hasPath(t.Path)
}

// finishJump lands a jump on a diff that is here, and reports what it could
// not do. It runs on the key and again on every diff arriving, so a jump made
// before the first request answered lands the moment it does.
func (m *Model) finishJump() tea.Cmd {
	if m.jump == "" {
		return nil
	}

	if !m.files.Loaded {
		// A fetch that failed has nothing to land in, and the pane already says
		// why. Anything else is still on its way.
		if m.files.Status == store.StatusFailed {
			m.jump = ""
		}
		return nil
	}

	// The reader tabbed away while the diff was out. Hauling the page back to
	// where they no longer are is the one thing every key on this screen
	// refuses to do.
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

	// Rendered before either pane is scrolled: both were only measured now, and
	// SetYOffset clamps to the content the viewport is holding. Scrolling the
	// column first against the tree as it was folded clamps the cursor to the
	// top and leaves it off screen once the rows come back.
	m.syncContent()
	m.showCursorRow()

	if line, ok := m.threadLine(t.ID); ok {
		m.view.SetYOffset(contentLead + m.jumpTop(t.Path, line))
		return nil
	}

	// The file is here and the thread is not drawn in it, which is a file whose
	// body GitHub omitted. Naming it was the whole of the move.
	return nil
}

// threadLine is where a thread's card landed in the rendered diff. The stops
// are a handful per file, so they are walked rather than indexed.
func (m Model) threadLine(id string) (int, bool) {
	want := focusKey{kind: focusThread, id: id}
	for _, s := range m.diff.stops {
		if s.focusKey == want {
			return s.start, true
		}
	}
	return 0, false
}

// jumpLead is the code kept above a thread when a jump lands: the line it
// answers, and enough of the hunk around it to read that line in context.
const jumpLead = 4

// jumpTop is where the diff opens for a thread. Not the card's own line: the
// card hangs under the line it was written against, so putting it on the top
// row scrolls away the one thing the reader pressed the key to see. The same
// rule the reply box follows, one tab over.
//
// It never opens above the file's own heading. A thread near the top of a file
// would otherwise show the tail of the file before it, which reads as the wrong
// file until the eye finds the border.
func (m Model) jumpTop(path string, line int) int {
	top := line - jumpLead
	if at := slices.IndexFunc(m.diff.spans, func(s fileSpan) bool { return s.key == path }); at >= 0 {
		top = max(top, m.diff.spans[at].start)
	}
	return max(0, top)
}

// reveal takes a file out from under every fold hiding it. A file inside a
// collapsed directory is in no row and no span, so there is no cursor to point
// at it and no block to scroll to. A chain of directories collapses under the
// deepest key in the chain, so every prefix goes rather than the one key the
// tree happens to be using.
func (m *Model) reveal(path string) {
	segments := strings.Split(path, "/")
	for i := range segments {
		delete(m.collapsed, strings.Join(segments[:i+1], "/"))
	}
	m.syncRows()
}

// pointAt moves the tree cursor to a file, so the column agrees with the pane
// beside it. Bringing the cursor into the column's own window is the caller's,
// because the column has to be rendered first.
func (m *Model) pointAt(path string) {
	at := slices.IndexFunc(m.rows, func(r row) bool { return r.file != nil && r.key == path })
	if at < 0 {
		return
	}
	m.cursor = at
	m.nameShownFile()
}

// hasPath is whether the diff carries a file. The tree and the thread key by
// the same path, so a rename needs no second check.
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
