package prview

import (
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/store"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
	"github.com/zen-octo/zen-octo/internal/tui/paint"
	"github.com/zen-octo/zen-octo/internal/tui/syntax"
)

// threadIndent sets a review thread in from the code it hangs off, so a card
// full of prose does not read as another hunk.
const threadIndent = 3

// anchor is where a review thread hangs: a line on one side of the diff.
type anchor struct {
	side gh.DiffSide
	line int
}

// fileSpan is where one file's block sits in the diff body. It is what lets the
// tree scroll to a file and the cursor follow the diff back.
type fileSpan struct {
	key   string
	start int
	end   int
}

// blockKey identifies a rendered file block. Folding changes what a block is,
// so the same file has one under each state.
type blockKey struct {
	key     string
	folded  bool
	heading bool
}

// blockStop is one segment of a file that something outside the painted code
// can change. Exactly one of the two indexes is set; the other is stopNone.
type blockStop struct {
	hunk   int
	thread int
}

// stopNone is the index a blockStop does not carry.
const stopNone = -1

// run is a stretch of painted code between two stops. The count is kept because
// one empty source line and no lines at all are the same string.
type run struct {
	text  string
	lines int
}

// block is one file rendered. Tokenising is what it costs and that is all in
// the runs, so a stop is drawn again every frame and never held lit.
type block struct {
	// runs and stops interleave, runs first: one more run than stop, always.
	runs  []run
	stops []blockStop
}

// blockState is everything outside a single file that its block is rendered
// against. A change to either retires the whole cache.
//
// folds counts how many times a <details> block has been folded or unfolded.
// Which ones are open is a map, and a count says the same thing to a cache key
// without the cache having to hold the map: a toggle moves it by one either way,
// so no two consecutive states share a number.
type blockState struct {
	width int
	folds int
}

// diffBody is one rendered diff: where each file's block sits inside it, and
// the blocks themselves. Each tab that shows a diff keeps one, because the
// render is the same over different files.
//
// The blocks are cached because moving a cursor one row repaints the diff, and
// rendering a block tokenises the whole file: without this a single keystroke
// costs the diff twice over. at is what the cache was built against; anything
// else invalidates the lot.
type diffBody struct {
	spans  []fileSpan
	blocks map[blockKey]block
	at     blockState

	// stops is every hunk heading and thread card in the rendered body, in the
	// same lines the file spans are counted in. The ring is built from them.
	stops []focusItem

	// headings is whether each file draws its own heading row. The Files tab
	// draws one file and puts its heading in the pane, where it cannot scroll.
	headings bool

	// threads is whether review threads hang off these lines. They are written
	// against the pull request's head, and the same line number in an older
	// commit is different code, so a commit's diff carries none.
	threads bool

	// lead is what sits above the first block. The spans are what a jump lands
	// on, and they have to clear whatever the tab put in front of them.
	lead int
}

// filesBody is the diff. A diff that has loaded once keeps rendering through a
// failed refetch; the root raises a toast for that.
func (m *Model) filesBody() string {
	// Reset on every path, including the two with no diff to walk: a stop left
	// over from before a refetch failed is a card the reader cannot see.
	m.pageRing.reset()

	switch {
	case m.files.Loaded:
		body := m.renderDiff(m.shownRows(), m.files, &m.diff)
		for _, s := range m.diff.stops {
			m.pageRing.add(s.focusKey, s.start, s.lines)
		}
		return body
	case m.files.Status == store.StatusFailed:
		return m.faint().Render("Could not load the diff: " + m.files.Err.Error())
	}
	return m.spinner.Render("Loading the diff")
}

// renderDiff renders every file in a set of rows, and records where each one
// starts so a column beside it can scroll to it.
func (m *Model) renderDiff(rows []row, res store.Files, d *diffBody) string {
	if len(res.Files) == 0 {
		return m.faint().Render("No files changed.")
	}

	width := m.bodyWidth()
	d.spans, d.stops = d.spans[:0], d.stops[:0]

	// A fold only reaches blocks with review threads in them. The Commits tab's
	// diff carries none, so counting folds against it retires the whole cache
	// and re-tokenises a commit for a keypress that changed nothing it renders.
	state := blockState{width: width}
	if d.threads {
		state.folds = m.folds
	}
	if d.blocks == nil || d.at != state {
		d.blocks, d.at = make(map[blockKey]block, len(rows)), state
	}

	blocks := make([]string, 0, len(rows))
	at := d.lead
	for _, r := range rows {
		if r.file == nil {
			continue
		}

		bk := blockKey{key: r.key, folded: r.folded, heading: d.headings}
		b, ok := d.blocks[bk]
		if !ok {
			b = m.fileBlock(*r.file, r.folded, width, d.threads, d.headings)
			d.blocks[bk] = b
		}

		text, placed := m.fileText(*r.file, b, width)
		blocks = append(blocks, text)

		for _, p := range placed {
			p.start += at
			d.stops = append(d.stops, p)
		}

		lines := strings.Count(text, "\n") + 1
		d.spans = append(d.spans, fileSpan{key: r.key, start: at, end: at + lines})
		// The join puts a blank line after every block but the last, and the
		// next one starts on the line after that.
		at += lines + 1
	}

	if note := overflow(res); note != "" {
		blocks = append(blocks, wrap(m.faint().Render(note), width))
	}
	return strings.Join(blocks, "\n\n")
}

// overflow says what the response did not reach. A pull request's diff is
// measured against the file count it carries, and a commit's has no count to
// measure against, so that one says there is more without saying how much.
func overflow(res store.Files) string {
	switch {
	case res.MoreFiles > 0:
		return comp.Plural(res.MoreFiles, "more file") + " on GitHub"
	case res.Truncated:
		return "More files on GitHub"
	}
	return ""
}

// fileBlock is one file: the heading, then its hunks and the review threads
// anchored inside them. No box, or a thread sits three borders deep.
func (m *Model) fileBlock(f gh.ChangedFile, folded bool, width int, threads, heading bool) block {
	if folded {
		// Nothing inside it is on the page, so nothing inside it has a line.
		return block{runs: []run{{text: m.fileHead(f, "▸ ", width), lines: 1}}}
	}

	b := m.fileBody(f, width, threads)
	if !heading {
		return b
	}

	head := m.fileHead(f, "▾ ", width)
	if b.runs[0].lines == 0 {
		b.runs[0] = run{text: head, lines: 1}
		return b
	}
	b.runs[0] = run{text: head + "\n" + b.runs[0].text, lines: b.runs[0].lines + 1}
	return b
}

// fileBody is everything under a file's heading, already the full inner width
// so a changed line's background runs to the border. The pane pads with plain
// spaces, which would leave a hole at the end of every one.
func (m *Model) fileBody(f gh.ChangedFile, width int, threads bool) block {
	if f.Omitted != "" {
		text := " " + clipTo(m.faint().Render(f.Omitted), width-1, m.faint())
		return block{runs: []run{{text: text, lines: 1}}}
	}

	// A nil map answers nothing, so the lines below need no guard of their own.
	var anchored map[anchor][]int
	if threads {
		anchored = m.threadsIn(f.Path)
	}
	placed := make(map[int]bool, len(anchored))

	tokens := m.lineTokens(f)
	gutter := paint.Gutter(widest(f))

	var b block
	var open []string

	// close ends the run being gathered, so the stop after it starts a new one.
	closeRun := func() {
		b.runs = append(b.runs, run{text: strings.Join(open, "\n"), lines: len(open)})
		open = nil
	}
	stop := func(s blockStop) {
		closeRun()
		b.stops = append(b.stops, s)
	}

	seen := 0
	for i, h := range f.Hunks {
		stop(blockStop{hunk: i, thread: stopNone})
		for _, l := range h.Lines {
			open = append(open, m.diffLine(l, tokens[seen], gutter, width, nil))
			seen++
			for _, at := range threadsAt(anchored, placed, l) {
				stop(blockStop{hunk: stopNone, thread: at})
			}
		}
	}

	if threads {
		for _, at := range m.strayThreads(f.Path, placed) {
			stop(blockStop{hunk: stopNone, thread: at})
		}
	}
	closeRun()
	return b
}

// fileHead is the path and the churn, with a marker saying whether the diff
// under it is folded away.
func (m Model) fileHead(f gh.ChangedFile, marker string, width int) string {
	path := f.Path
	if f.PreviousPath != "" {
		path = f.PreviousPath + " → " + f.Path
	}

	lead := m.faint().Render(marker) +
		lipgloss.NewStyle().Foreground(m.theme.Text).Bold(true).Render(path)

	churn := m.fileChurn(f)
	room := max(0, width-lipgloss.Width(churn)-1)
	if lipgloss.Width(lead) > room {
		lead = paint.Clip(lead, room, m.faint())
	}

	gap := max(1, width-lipgloss.Width(lead)-lipgloss.Width(churn))
	// The gap has a floor, so a width with no room for the churn still asks for
	// one more cell than it has. The pane would take that off the end of the
	// count, and a truncated count reads as a real one.
	return clipTo(lead+strings.Repeat(" ", gap)+churn, width, m.faint())
}

func (m Model) fileChurn(f gh.ChangedFile) string {
	return lipgloss.NewStyle().Foreground(m.theme.Success).Render("+"+strconv.Itoa(f.Additions)) +
		" " + lipgloss.NewStyle().Foreground(m.theme.Error).Render("−"+strconv.Itoa(f.Deletions))
}

// hunkFill is the cursor on a hunk heading. It fills the row rather than taking
// the badge slot, which is for a state a heading carries with or without focus.
func (m Model) hunkFill(key focusKey) color.Color {
	if !m.lit(key) {
		return nil
	}
	return m.theme.SelectedBackground
}

// hunkHead is the @@ line, landing at the column the source under it starts in.
// The badge slot stays empty: a hunk here carries no state of its own.
func (m Model) hunkHead(h gh.Hunk, gutter, width int, fill color.Color) string {
	return m.painter.HunkHeader(paint.Header{Text: h.Header, Fill: fill}, gutter, width)
}

// diffLine is one line of code, handed to the painter with whatever fill the
// caller's own state asks for. A nil fill leaves the change's own tint.
func (m Model) diffLine(l gh.DiffLine, tokens []syntax.Token, gutter, width int, fill color.Color) string {
	return m.painter.Line(paint.Line{
		Kind:   kindOf(l.Kind),
		Old:    l.Old,
		New:    l.New,
		Tokens: tokens,
		Fill:   fill,
	}, gutter, width)
}

// kindOf maps a fetched line onto the painter's own three.
func kindOf(k gh.DiffKind) paint.Kind {
	switch k {
	case gh.DiffAdded:
		return paint.Added
	case gh.DiffRemoved:
		return paint.Removed
	}
	return paint.Context
}

// lineTokens colors a whole file at once, one pass per side. A lexer carries
// state across lines, so highlighting line by line comes apart on the first
// multi-line string; and running the two sides together would feed it a file
// holding both halves of every change.
//
// A context line goes into both sides so neither reads as source with its
// unchanged lines missing, and takes its color from the new one.
func (m *Model) lineTokens(f gh.ChangedFile) [][]syntax.Token {
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

	out := make([][]syntax.Token, len(index))
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
// hangs. The buckets hold the thread's place in the detail rather than the
// thread: that index is what the conversation calls the same thread, so the two
// tabs agree on which one has been unfolded.
func (m Model) threadsIn(path string) map[anchor][]int {
	out := make(map[anchor][]int)
	for i, t := range m.detail.Detail.Threads {
		if t.Path != path || t.Line == 0 {
			continue
		}
		key := anchor{side: t.Side, line: t.Line}
		out[key] = append(out[key], i)
	}
	return out
}

// threadsAt is whatever hangs off a line. A context line sits on both sides of
// the diff, so it answers to a comment written against either.
func threadsAt(threads map[anchor][]int, placed map[int]bool, l gh.DiffLine) []int {
	var out []int
	for _, key := range anchorsOf(l) {
		for _, i := range threads[key] {
			placed[i] = true
			out = append(out, i)
		}
	}
	return out
}

// diffThread draws one thread inline in the diff, under the keys the
// conversation gives it, so a fold on one tab is a fold on the other.
func (m *Model) diffThread(i, width int) string {
	t := m.detail.Detail.Threads[i]
	return indent(m.thread(t, width-threadIndent, false).block, threadIndent)
}

// fileText joins a cached block back into a page, drawing every stop fresh, and
// says where each one landed in the lines it wrote.
func (m *Model) fileText(f gh.ChangedFile, b block, width int) (string, []focusItem) {
	gutter := paint.Gutter(widest(f))

	var sb strings.Builder
	placed := make([]focusItem, 0, len(b.stops))
	at, wrote := 0, false

	write := func(text string, lines int) {
		if lines == 0 {
			return
		}
		if wrote {
			sb.WriteByte('\n')
		}
		sb.WriteString(text)
		at, wrote = at+lines, true
	}

	for i, r := range b.runs {
		write(r.text, r.lines)
		if i == len(b.stops) {
			break
		}

		s := b.stops[i]
		var key focusKey
		var text string
		if s.hunk != stopNone {
			h := f.Hunks[s.hunk]
			key = hunkKey(f.Path, h)
			text = m.hunkHead(h, gutter, width, m.hunkFill(key))
		} else {
			key = threadKey(m.detail.Detail.Threads[s.thread])
			text = m.diffThread(s.thread, width)
		}

		lines := strings.Count(text, "\n") + 1
		placed = append(placed, focusItem{focusKey: key, start: at, lines: lines})
		write(text, lines)
	}
	return sb.String(), placed
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
//
// It walks the threads the query returned rather than the map they were
// bucketed into: ranging a map is ordered at random, so the same comments came
// out under the file in a different order every time it was rendered.
func (m Model) strayThreads(path string, placed map[int]bool) []int {
	var out []int
	for i, t := range m.detail.Detail.Threads {
		if t.Path != path || t.Line == 0 || placed[i] {
			continue
		}
		out = append(out, i)
	}
	return out
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

// clipTo cuts a line to the pane rather than letting it wrap. A wrapped line of
// code puts its tail under the gutter and every line below it out of step.
func clipTo(line string, width int, mark lipgloss.Style) string {
	if lipgloss.Width(line) <= width {
		return line
	}
	return paint.Clip(line, width, mark)
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

	// The cursor stays painted with focus elsewhere. Which file the diff is
	// showing is the question the column exists to answer, and the pane borders
	// already say where the keys go.
	lines := make([]string, len(m.rows))
	for i, r := range m.rows {
		lines[i] = renderRow(m.theme, r, width, i == m.cursor)
	}
	return strings.Join(lines, "\n")
}

// fileHeading is the path and the churn of the file in the pane, pinned in the
// pane's own header so it never scrolls off the code it names.
func (m Model) fileHeading() string {
	f := m.shownFile()
	if m.tab != tabFiles || f == nil || !m.files.Loaded {
		return ""
	}
	return " " + m.fileHead(*f, "", max(0, m.main.InnerWidth()-1))
}

// treeTitle names the file column by what it holds.
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

	m.nameShownFile()
	m.showCursorRow()
	m.syncContent()
}

// showCursorRow keeps the cursor inside the tree's own window. The column opens
// with no blank line, so a row is its own offset.
func (m *Model) showCursorRow() { showRow(&m.sideView, m.cursor) }

// shownRows is the one row the diff draws: the file the cursor last named.
// Empty until a file has been named, which SetFiles does as the diff lands.
func (m Model) shownRows() []row {
	for _, r := range m.rows {
		if r.file != nil && r.file.Path == m.shownPath {
			return []row{r}
		}
	}
	return nil
}

// shownFile is the file in the pane, or nil before one has been named.
func (m Model) shownFile() *gh.ChangedFile {
	if rows := m.shownRows(); len(rows) == 1 {
		return rows[0].file
	}
	return nil
}

// nameShownFile points the pane at the row under the cursor. A directory names
// no file and leaves the pane on whatever it was reading.
func (m *Model) nameShownFile() {
	if m.cursor >= len(m.rows) {
		return
	}
	f := m.rows[m.cursor].file
	if f == nil || f.Path == m.shownPath {
		return
	}
	m.shownPath = f.Path

	// The pane is shared with the other tabs, so a file named while one of them
	// is up must not scroll the page the reader is actually on.
	if m.tab == tabFiles {
		m.view.SetYOffset(0)
	}
}

// jumpFile moves the cursor a whole file at a time, skipping the directory rows
// between them. Reading a diff is reading one file after another.
func (m *Model) jumpFile(delta int) {
	step := 1
	if delta < 0 {
		step = -1
	}

	for at := m.cursor + step; at >= 0 && at < len(m.rows); at += step {
		if m.rows[at].file == nil {
			continue
		}
		m.cursor = at
		m.nameShownFile()
		m.showCursorRow()
		m.syncContent()
		return
	}
}

// toggleFold folds the directory under the cursor out of the tree. A file has
// no fold of its own: the pane draws one file, and folding it leaves nothing.
func (m *Model) toggleFold() {
	if m.cursor >= len(m.rows) || m.rows[m.cursor].file != nil {
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

	// A fold can take the file being drawn off the tree, and the first render
	// has none named at all. Either way the cursor is what says which.
	if m.shownFile() == nil {
		m.cursor = m.firstFile()
		m.nameShownFile()
	}
}
