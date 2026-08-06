package prview

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/store"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
)

// commitRowHeight is what one commit takes in the column. The sha, the marker
// and the headline fill a line on their own at this width; the author and the
// age go under them.
//
// Every row is the same height on purpose. A headline wrapped to whatever it
// needs makes the scroll arithmetic a walk over the rows rather than a
// multiply, and a column that lands mid-row cuts a sha off above the window.
const commitRowHeight = 2

// commits is the Commits tab: what the column holds, where its cursor is, and
// the diff of whichever commit was last selected.
//
// sha is empty until the first selection. The diff is a request of its own, so
// it waits to be asked for rather than following the cursor.
type commits struct {
	cursor int
	sha    string
	files  store.Files
	rows   []row
	diff   diffBody
}

// NeedCommitMsg asks the root for a commit's diff. Same shape as the pull
// request's own: the screen cannot fetch, so the selection reaches the root as
// a message and the request starts there.
type NeedCommitMsg struct{ SHA string }

// SetCommitFiles takes what the store holds for a commit's diff. It drops
// anything that is not the commit on screen: the cursor can have moved on and
// a second commit been asked for while the first was still out.
func (m *Model) SetCommitFiles(sha string, f store.Files) {
	if sha != m.commit.sha {
		return
	}
	m.commit.files = f
	m.commit.diff.blocks = nil
	m.commit.rows = flatten(buildTree(f.Files), nil, 0, nil)
	m.syncContent()
}

// syncCommits keeps the cursor inside a commit list that has just arrived or
// changed under it.
func (m *Model) syncCommits() {
	m.commit.cursor = min(m.commit.cursor, max(0, len(m.detail.Detail.Commits)-1))
}

// selectCommit asks for the diff of the commit under the cursor. A commit
// already on screen costs nothing: the answer is the one being read.
func (m *Model) selectCommit() tea.Cmd {
	list := m.detail.Detail.Commits
	if m.commit.cursor >= len(list) {
		return nil
	}

	sha := list[m.commit.cursor].SHA
	if sha == "" || sha == m.commit.sha {
		return nil
	}

	m.commit.sha = sha
	m.commit.files = store.Files{}
	m.commit.rows = nil
	m.commit.diff.blocks = nil
	m.view.GotoTop()
	m.syncContent()

	return func() tea.Msg { return NeedCommitMsg{SHA: sha} }
}

// commitTitle counts what the column holds, or names it before the detail query
// answers and there is nothing to count.
func (m Model) commitTitle() string {
	if !m.detail.Loaded {
		return "Commits"
	}
	return comp.Plural(len(m.detail.Detail.Commits), "commit")
}

// commitColumn is the list. It paints its own selection, so it hands the
// viewport lines already the full inner width.
func (m Model) commitColumn(width int) string {
	if !m.detail.Loaded {
		return ""
	}

	list := m.detail.Detail.Commits
	if len(list) == 0 {
		return m.faint().Render("No commits.")
	}

	lines := make([]string, 0, len(list)*commitRowHeight)
	for i, c := range list {
		lines = append(lines, m.commitRow(c, width, i == m.commit.cursor)...)
	}
	return strings.Join(lines, "\n")
}

// commitRow is one commit over two lines: the check marker, the short sha and
// the headline, then who wrote it and when.
//
// Selection is painted cell by cell, the same way the file column paints it.
// Every styled run ends in a reset that clears the background with it, so a
// joined row wrapped in the background style afterwards paints only its first
// cell.
func (m Model) commitRow(c gh.Commit, width int, selected bool) []string {
	base := lipgloss.NewStyle()
	if selected {
		base = base.Background(m.theme.SelectedBackground)
	}

	_, checks := comp.CheckStateIcon(m.theme, c.Checks)
	head := base.Foreground(checks).Render(glyphCheck) + base.Render(" ") +
		base.Foreground(m.theme.Secondary).Render(c.Short) + base.Render(" ") +
		base.Foreground(m.theme.Primary).Render(c.Headline)

	// The second line is set in under the sha rather than under the marker, so
	// the two lines read as one row.
	by := base.Render("  ") + base.Foreground(m.theme.Faint).Render(commitBy(c))

	return []string{m.padTo(head, width, base), m.padTo(by, width, base)}
}

// commitBy names who wrote a commit and when. The account is empty when the
// commit email matches none, and the name git recorded stands in for it: a row
// with a blank where the author goes reads as a rendering fault.
func commitBy(c gh.Commit) string {
	who := comp.Handle(c.Author.Login)
	if who == "" {
		who = c.AuthorName
	}

	at := comp.RelativeTime(c.CommittedAt)
	switch {
	case who != "" && at != "":
		return who + " · " + at
	case who != "":
		return who
	}
	return at
}

// padTo fills a line out to the column, or clips it when it runs past. Both the
// fill and the clip mark carry the row's own style, so a selection background
// runs the full width rather than stopping at the last word.
func (m Model) padTo(line string, width int, base lipgloss.Style) string {
	switch w := lipgloss.Width(line); {
	case w > width:
		return comp.Clip(line, width, base.Foreground(m.theme.Faint))
	case w < width:
		return line + base.Render(strings.Repeat(" ", width-w))
	}
	return line
}

// commitBody is the selected commit's diff, through the same renderer the Files
// tab uses. Nothing selected yet says which key selects one.
func (m *Model) commitBody() string {
	switch {
	case m.commit.sha == "":
		// Named off the binding rather than written out, so a rebind moves the
		// prompt with it.
		return m.faint().Render("Press " + m.Keys().Select.Help().Key + " to show a commit's diff.")
	case m.commit.files.Loaded:
		return m.renderDiff(m.commit.rows, m.commit.files, &m.commit.diff)
	case m.commit.files.Status == store.StatusFailed:
		return m.faint().Render("Could not load the diff: " + m.commit.files.Err.Error())
	}
	return m.spinner.Render("Loading the diff")
}

// moveCommit walks the column and keeps the cursor inside its own window. The
// diff does not follow: it is a request, and it waits to be asked for.
func (m *Model) moveCommit(delta int) {
	list := m.detail.Detail.Commits
	if len(list) == 0 {
		return
	}
	m.commit.cursor = min(max(m.commit.cursor+delta, 0), len(list)-1)

	// The window is counted in rows rather than lines, so the offset always
	// lands on a row boundary. A pane with an odd number of lines would
	// otherwise open the column on a row's second line, with its sha cut off
	// above.
	rows := max(1, m.sideView.Height()/commitRowHeight)
	first := m.sideView.YOffset() / commitRowHeight

	switch {
	case m.commit.cursor < first:
		first = m.commit.cursor
	case m.commit.cursor >= first+rows:
		first = m.commit.cursor - rows + 1
	}

	m.sideView.SetYOffset(first * commitRowHeight)
	m.syncContent()
}

// jumpCommitFile moves the commit's diff a whole file at a time. There is no
// cursor beside it to drive, so the next file is the first one starting below
// the window and the previous the last one starting above it.
func (m *Model) jumpCommitFile(delta int) {
	spans := m.commit.diff.spans
	if len(spans) == 0 {
		return
	}

	at := m.view.YOffset()
	next := contentLead + spans[0].start

	if delta > 0 {
		next = contentLead + spans[len(spans)-1].start
		for _, s := range spans {
			if start := contentLead + s.start; start > at {
				next = start
				break
			}
		}
	} else {
		for _, s := range spans {
			if start := contentLead + s.start; start < at {
				next = start
			}
		}
	}
	m.view.SetYOffset(next)
}
