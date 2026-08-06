package prview

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/store"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
)

// gutterMin is the narrowest a line-number column gets. A file under ten lines
// still reads better with the two columns lined up against its neighbours.
const gutterMin = 2

// tabWidth is what a tab expands to. A raw tab is a variable number of cells,
// and one anywhere in a line puts every column after it out of step with the
// line above.
const tabWidth = 4

// threadIndent sets a review thread in from the code it hangs off, so a card
// full of prose does not read as another hunk.
const threadIndent = 3

// anchor is where a review thread hangs: a line on one side of the diff.
type anchor struct {
	side gh.DiffSide
	line int
}

// filesBody is the diff. A diff that has loaded once keeps rendering through a
// failed refetch; the root raises a toast for that.
func (m *Model) filesBody() string {
	switch {
	case m.files.Loaded:
		return m.diff()
	case m.files.Status == store.StatusFailed:
		return m.faint().Render("Could not load the diff: " + m.files.Err.Error())
	}
	return m.spinner.Render("Loading the diff")
}

// diff renders every file the tree has on screen, and records where each one
// starts so selecting it in the tree can scroll to it.
func (m *Model) diff() string {
	if len(m.files.Files) == 0 {
		return m.faint().Render("No files changed.")
	}

	width := m.bodyWidth()
	m.diffAt = make(map[string]int, len(m.rows))

	blocks := make([]string, 0, len(m.rows))
	at := 0
	for _, r := range m.rows {
		if r.file == nil {
			continue
		}
		m.diffAt[r.key] = at

		block := m.fileBlock(*r.file, r.folded, width)
		blocks = append(blocks, block)
		// The join puts a blank line after every block but the last, and the
		// next block starts on the line after that.
		at += strings.Count(block, "\n") + 2
	}

	if n := m.files.MoreFiles; n > 0 {
		blocks = append(blocks, wrap(m.faint().Render(comp.Plural(n, "more file")+" on GitHub"), width))
	}
	return strings.Join(blocks, "\n\n")
}

// fileBlock is one file: its heading, then its hunks and the review threads
// anchored inside them.
func (m *Model) fileBlock(f gh.ChangedFile, folded bool, width int) string {
	head := m.fileHead(f, folded, width)
	if folded {
		return head
	}
	if f.Omitted != "" {
		return head + "\n" + indent(m.faint().Render(f.Omitted), threadIndent)
	}

	threads := m.threadsIn(f.Path)
	placed := make(map[int]bool, len(threads))

	tokens := m.lineTokens(f)
	gutter := max(gutterMin, len(strconv.Itoa(widest(f))))

	lines := []string{head}
	seen := 0
	for _, h := range f.Hunks {
		lines = append(lines, m.hunkHead(h, gutter, width))
		for _, l := range h.Lines {
			lines = append(lines, m.diffLine(l, tokens[seen], gutter, width))
			seen++
			lines = append(lines, m.threadsAt(threads, placed, l, width)...)
		}
	}

	lines = append(lines, m.strayThreads(threads, placed, width)...)
	return strings.Join(lines, "\n")
}

// fileHead is the path and the churn, with a marker saying whether the diff
// under it is folded away.
func (m Model) fileHead(f gh.ChangedFile, folded bool, width int) string {
	marker := "▾ "
	if folded {
		marker = "▸ "
	}

	path := f.Path
	if f.PreviousPath != "" {
		path = f.PreviousPath + " → " + f.Path
	}

	lead := m.faint().Render(marker) +
		lipgloss.NewStyle().Foreground(m.theme.Primary).Bold(true).Render(path)

	churn := m.fileChurn(f)
	room := max(0, width-lipgloss.Width(churn)-1)
	if lipgloss.Width(lead) > room {
		lead = comp.Clip(lead, room, m.faint())
	}

	gap := max(1, width-lipgloss.Width(lead)-lipgloss.Width(churn))
	return lead + strings.Repeat(" ", gap) + churn
}

func (m Model) fileChurn(f gh.ChangedFile) string {
	return lipgloss.NewStyle().Foreground(m.theme.Success).Render("+"+strconv.Itoa(f.Additions)) +
		" " + lipgloss.NewStyle().Foreground(m.theme.Error).Render("−"+strconv.Itoa(f.Deletions))
}

// hunkHead is the @@ line, set in over the gutter the numbers below it use.
func (m Model) hunkHead(h gh.Hunk, gutter, width int) string {
	line := strings.Repeat(" ", gutter*2+3) +
		lipgloss.NewStyle().Foreground(m.theme.Secondary).Render(h.Header)
	return clipTo(line, width, m.faint())
}

// diffLine is one line of code: the two line numbers, the marker, and the
// highlighted source. The numbers are faint except on the side the line
// belongs to, which is the same signal the marker carries.
func (m Model) diffLine(l gh.DiffLine, tokens []comp.Token, gutter, width int) string {
	marker, c := " ", m.theme.Faint
	switch l.Kind {
	case gh.DiffAdded:
		marker, c = "+", m.theme.Success
	case gh.DiffRemoved:
		marker, c = "−", m.theme.Error
	}

	kind := lipgloss.NewStyle().Foreground(c)
	oldNum, newNum := m.faint(), m.faint()
	switch l.Kind {
	case gh.DiffAdded:
		newNum = kind
	case gh.DiffRemoved:
		oldNum = kind
	}

	line := oldNum.Render(number(l.Old, gutter)) + " " +
		newNum.Render(number(l.New, gutter)) + " " +
		kind.Render(marker) + " " + code(tokens)

	return clipTo(line, width, m.faint())
}

// code renders one line's tokens. Every token carries its own color and nothing
// else, so a caller that wants a background can put one behind the whole line.
func code(tokens []comp.Token) string {
	var b strings.Builder
	for _, t := range tokens {
		text := strings.ReplaceAll(t.Text, "\t", strings.Repeat(" ", tabWidth))
		if t.Color == nil {
			b.WriteString(text)
			continue
		}
		b.WriteString(lipgloss.NewStyle().Foreground(t.Color).Render(text))
	}
	return b.String()
}

// lineTokens colors a whole file at once, one pass per side. A lexer carries
// state across lines, so highlighting line by line comes apart on the first
// multi-line string; and running the two sides together would feed it a file
// holding both halves of every change.
//
// A context line goes into both sides so neither reads as source with its
// unchanged lines missing, and takes its color from the new one.
func (m *Model) lineTokens(f gh.ChangedFile) [][]comp.Token {
	type at struct {
		left bool
		i    int
	}

	var oldSrc, newSrc []string
	var index []at

	for _, h := range f.Hunks {
		for _, l := range h.Lines {
			switch l.Kind {
			case gh.DiffRemoved:
				index = append(index, at{left: true, i: len(oldSrc)})
				oldSrc = append(oldSrc, l.Content)
			case gh.DiffAdded:
				index = append(index, at{i: len(newSrc)})
				newSrc = append(newSrc, l.Content)
			default:
				index = append(index, at{i: len(newSrc)})
				oldSrc = append(oldSrc, l.Content)
				newSrc = append(newSrc, l.Content)
			}
		}
	}

	oldTok := m.syntax.Lines(f.Path, strings.Join(oldSrc, "\n"))
	newTok := m.syntax.Lines(f.Path, strings.Join(newSrc, "\n"))

	out := make([][]comp.Token, len(index))
	for i, a := range index {
		src := newTok
		if a.left {
			src = oldTok
		}
		if a.i < len(src) {
			out[i] = src[a.i]
		}
	}
	return out
}

// threadsIn is every review thread written against a file, keyed by where it
// hangs.
func (m Model) threadsIn(path string) map[anchor][]gh.ReviewThread {
	out := make(map[anchor][]gh.ReviewThread)
	for _, t := range m.detail.Detail.Threads {
		if t.Path != path || t.Line == 0 {
			continue
		}
		key := anchor{side: t.Side, line: t.Line}
		out[key] = append(out[key], t)
	}
	return out
}

// threadsAt renders whatever hangs off a line. A context line sits on both
// sides of the diff, so it answers to a comment written against either.
func (m *Model) threadsAt(threads map[anchor][]gh.ReviewThread, placed map[int]bool, l gh.DiffLine, width int) []string {
	var out []string
	for _, key := range anchorsOf(l) {
		for _, t := range threads[key] {
			placed[thumbprint(t)] = true
			out = append(out, indent(m.thread(t, width-threadIndent), threadIndent))
		}
	}
	return out
}

func anchorsOf(l gh.DiffLine) []anchor {
	switch l.Kind {
	case gh.DiffAdded:
		return []anchor{{side: gh.SideRight, line: l.New}}
	case gh.DiffRemoved:
		return []anchor{{side: gh.SideLeft, line: l.Old}}
	}
	return []anchor{{side: gh.SideRight, line: l.New}, {side: gh.SideLeft, line: l.Old}}
}

// strayThreads is what the diff had no line for. An outdated thread anchors to
// a line the pull request has since moved past, and dropping it loses the only
// record of what was asked.
func (m *Model) strayThreads(threads map[anchor][]gh.ReviewThread, placed map[int]bool, width int) []string {
	var out []string
	for _, held := range threads {
		for _, t := range held {
			if placed[thumbprint(t)] {
				continue
			}
			out = append(out, indent(m.thread(t, width-threadIndent), threadIndent))
		}
	}
	return out
}

// thumbprint identifies a thread across the two passes over the same map. The
// domain type carries no id, and the anchor plus the first comment is what no
// two threads on one file share.
func thumbprint(t gh.ReviewThread) int {
	h := 17
	for _, s := range []string{string(t.Side), t.Path, t.ReviewID} {
		for _, r := range s {
			h = h*31 + int(r)
		}
	}
	if len(t.Comments) > 0 {
		for _, r := range t.Comments[0].Body {
			h = h*31 + int(r)
		}
	}
	return h*31 + t.Line
}

// widest is the longest line number the file has to print, which is what the
// gutter is sized to.
func widest(f gh.ChangedFile) int {
	n := 0
	for _, h := range f.Hunks {
		for _, l := range h.Lines {
			n = max(n, l.Old, l.New)
		}
	}
	return n
}

// number right-aligns a line number, or leaves the column blank on the side a
// line does not belong to.
func number(n, width int) string {
	if n == 0 {
		return strings.Repeat(" ", width)
	}
	s := strconv.Itoa(n)
	return strings.Repeat(" ", max(0, width-len(s))) + s
}

// clipTo cuts a line to the pane rather than letting it wrap. A wrapped line of
// code puts its tail under the gutter and every line below it out of step.
func clipTo(line string, width int, mark lipgloss.Style) string {
	if lipgloss.Width(line) <= width {
		return line
	}
	return comp.Clip(line, width, mark)
}

// treeBody is the file column. It paints its own selection, so it hands the
// viewport lines already the full inner width.
func (m *Model) treeBody(width int) string {
	if !m.files.Loaded {
		return ""
	}
	if len(m.rows) == 0 {
		return m.faint().Render("No files changed.")
	}

	lines := make([]string, len(m.rows))
	for i, r := range m.rows {
		lines[i] = renderRow(m.theme, r, width, i == m.cursor && m.focus == paneTree)
	}
	return strings.Join(lines, "\n")
}

// treeTitle names the column by what it holds, since the tab strip beside it
// already says which tab this is.
func (m Model) treeTitle() string {
	if !m.files.Loaded {
		return "Files"
	}
	return comp.Plural(len(m.files.Files), "file")
}

// moveCursor walks the tree and takes the diff with it. The rows are one line
// each, so keeping the cursor on screen is a clamp rather than the boundary
// arithmetic a two-line row needs.
func (m *Model) moveCursor(delta int) {
	if len(m.rows) == 0 {
		return
	}
	m.cursor = min(max(m.cursor+delta, 0), len(m.rows)-1)

	height := m.treeView.Height()
	switch offset := m.treeView.YOffset(); {
	case m.cursor < offset:
		m.treeView.SetYOffset(m.cursor)
	case m.cursor >= offset+height:
		m.treeView.SetYOffset(m.cursor - height + 1)
	}

	m.syncContent()
	m.showCursorFile()
}

// showCursorFile scrolls the diff to whatever the tree is pointing at. A
// directory has no block of its own, so it scrolls to the first file under it.
func (m *Model) showCursorFile() {
	if m.cursor >= len(m.rows) {
		return
	}
	for _, r := range m.rows[m.cursor:] {
		if at, ok := m.diffAt[r.key]; ok {
			m.view.SetYOffset(contentLead + at)
			return
		}
	}
}

// toggleFold folds the row under the cursor: a directory out of the tree, a
// file's diff out of the pane beside it.
func (m *Model) toggleFold() {
	if m.cursor >= len(m.rows) {
		return
	}
	key := m.rows[m.cursor].key
	m.collapsed[key] = !m.collapsed[key]

	m.syncRows()
	m.cursor = min(m.cursor, max(0, len(m.rows)-1))
	m.syncContent()
}

// syncRows rebuilds what the tree has on screen. Folding changes it, and so
// does a diff arriving.
func (m *Model) syncRows() {
	m.rows = flatten(buildTree(m.files.Files), m.collapsed, 0, nil)
}
