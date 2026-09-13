package prview

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/paint"
	"github.com/praxis-labs-io/zen-octo/internal/tui/syntax"
)

const threadIndent = 3

type anchor struct {
	side gh.DiffSide
	line int
}

type fileSpan struct {
	key   string
	start int
	end   int
}

type blockKey struct {
	key     string
	heading bool
}

// Exactly one index is set; the other is stopNone.
type blockStop struct {
	hunk   int
	thread int
}

const stopNone = -1

type diffRow struct {
	line  paint.Line
	right paint.Line
	text  string
}

func (r diffRow) code(column gh.DiffSide) bool {
	if column == gh.SideRight {
		return r.right.Old != 0 || r.right.New != 0
	}
	return r.line.Old != 0 || r.line.New != 0
}

// Kept as rows so one can be repainted lit without re-tokenising the file.
type run struct {
	rows []diffRow
	text string
}

func newRun(rows []diffRow) run {
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = row.text
	}
	return run{rows: rows, text: strings.Join(out, "\n")}
}

func (r run) codeRows(column gh.DiffSide) int {
	n := 0
	for _, row := range r.rows {
		if row.code(column) {
			n++
		}
	}
	return n
}

func (r run) rowAt(n int, column gh.DiffSide) int {
	if n <= 0 {
		return -1
	}
	for i, row := range r.rows {
		if !row.code(column) {
			continue
		}
		if n--; n == 0 {
			return i
		}
	}
	return -1
}

type drawnFile struct {
	text   string
	stops  []focusItem
	boxAt  int
	boxCol int

	cursorAt int

	// Read by the next key, not derived again: only the render knows which card code hangs under.
	rows map[focusKey]int

	// Not always the column asked for: a block empty in the focused column is walked in the other.
	columns map[focusKey]gh.DiffSide
}

type block struct {
	at blockState

	// Runs and stops interleave, runs first: always one more run than stop.
	runs  []run
	stops []blockStop
}

type blockState struct {
	width int
	folds string

	// A mode rather than an identity, so it retires the block instead of keeping one painted per mode.
	split bool
}

// Blocks are cached because rendering one tokenises the whole file, and a cursor step repaints the diff.
type diffBody struct {
	spans  []fileSpan
	blocks map[blockKey]block

	stops []focusItem

	headings bool

	// A commit's diff carries none: threads are written against the head, not older commits.
	threads bool

	// The body's own mode, never read off the model: the Commits tab never splits.
	split bool

	lead int

	cursorLine int

	rows map[focusKey]int

	columns map[focusKey]gh.DiffSide
}

func (m *Model) filesBody() string {
	switch {
	case m.files.Loaded:
		m.diff.split = m.splitting()
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

func (m *Model) renderDiff(rows []row, res store.Files, d *diffBody) string {
	width := m.bodyWidth()
	d.spans, d.stops = d.spans[:0], d.stops[:0]
	d.cursorLine, d.rows = -1, make(map[focusKey]int)
	d.columns = make(map[focusKey]gh.DiffSide)

	if len(res.Files) == 0 {
		return m.faint().Render("No files changed.")
	}

	if d.blocks == nil {
		d.blocks = make(map[blockKey]block, len(rows))
	}

	blocks := make([]string, 0, len(rows))
	at := d.lead
	for _, r := range rows {
		if r.file == nil {
			continue
		}

		bk := blockKey{key: r.key, heading: d.headings}
		state := blockState{width: width, folds: m.hunkFoldState(*r.file), split: d.split}
		b, ok := d.blocks[bk]
		if !ok || b.at != state {
			b = m.fileBlock(*r.file, width, d.threads, d.headings, d.split)
			b.at = state
			d.blocks[bk] = b
		}

		drawn := m.fileText(*r.file, b, width, d.split)
		blocks = append(blocks, drawn.text)

		for _, p := range drawn.stops {
			p.start += at
			d.stops = append(d.stops, p)
		}
		if drawn.boxAt > 0 {
			m.boxLine, m.boxCol = at+drawn.boxAt, drawn.boxCol
		}
		if drawn.cursorAt >= 0 {
			d.cursorLine = at + drawn.cursorAt
		}
		for k, n := range drawn.rows {
			d.rows[k], d.columns[k] = n, drawn.columns[k]
		}

		lines := strings.Count(drawn.text, "\n") + 1
		d.spans = append(d.spans, fileSpan{key: r.key, start: at, end: at + lines})
		at += lines + 1
	}

	if note := overflow(res); note != "" {
		blocks = append(blocks, wrap(m.faint().Render(note), width))
	}
	return strings.Join(blocks, "\n\n")
}

func overflow(res store.Files) string {
	switch {
	case res.MoreFiles > 0:
		return comp.Plural(res.MoreFiles, "more file") + " on GitHub"
	case res.Truncated:
		return "More files on GitHub"
	}
	return ""
}

func (m *Model) fileBlock(f gh.ChangedFile, width int, threads, heading, split bool) block {
	b := m.fileBody(f, width, threads, split)
	if !heading {
		return b
	}

	head := diffRow{text: m.fileHead(f, "▾ ", width)}
	b.runs[0] = newRun(append([]diffRow{head}, b.runs[0].rows...))
	return b
}

// Pads to the full inner width itself: the pane's plain-space padding breaks a changed line's tint.
func (m *Model) fileBody(f gh.ChangedFile, width int, threads, split bool) block {
	if f.Omitted != "" {
		text := " " + clipTo(m.faint().Render(f.Omitted), width-1, m.faint())
		return block{runs: []run{newRun([]diffRow{{text: text}})}}
	}

	var anchored map[anchor][]int
	if threads {
		anchored = m.threadsIn(f.Path)
	}
	placed := make(map[int]bool, len(anchored))

	tokens := m.lineTokens(f)
	gutter := paint.Gutter(widest(f))

	var b block
	var open []diffRow

	closeRun := func() {
		b.runs = append(b.runs, newRun(open))
		open = nil
	}
	stop := func(s blockStop) {
		closeRun()
		b.stops = append(b.stops, s)
	}

	seen := 0
	for i, h := range f.Hunks {
		hunkOpen := m.hunkOpen(hunkKey(f.Path, h))

		if i > 0 {
			open = append(open, diffRow{})
		}
		stop(blockStop{hunk: i, thread: stopNone})

		own := tokens[seen : seen+len(h.Lines)]
		for _, p := range pairs(h.Lines, split) {
			if hunkOpen {
				open = append(open, m.paintRow(h.Lines, p, own, gutter, width, split))
			}
			for _, j := range sides(p) {
				for _, at := range threadsAt(anchored, placed, h.Lines[j]) {
					if hunkOpen {
						stop(blockStop{hunk: stopNone, thread: at})
					}
				}
			}
		}
		seen += len(h.Lines)
	}

	if threads {
		for _, at := range m.strayThreads(f.Path, placed) {
			stop(blockStop{hunk: stopNone, thread: at})
		}
	}
	closeRun()
	return b
}

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
	return clipTo(lead+strings.Repeat(" ", gap)+churn, width, m.faint())
}

func (m Model) fileChurn(f gh.ChangedFile) string {
	return lipgloss.NewStyle().Foreground(m.theme.Success).Render("+"+strconv.Itoa(f.Additions)) +
		" " + lipgloss.NewStyle().Foreground(m.theme.Error).Render("−"+strconv.Itoa(f.Deletions))
}

func (m Model) cursorOn(key focusKey) bool { return m.lit(key) && !m.walkedInto(key) }

func (m Model) litRun(r run, at, gutter, width int, column gh.DiffSide) run {
	rows := make([]diffRow, len(r.rows))
	copy(rows, r.rows)

	fill, bar := m.theme.SelectedBackground, m.theme.Accent
	if column != "" {
		rows[at].text = m.halves(rows[at], column, gutter, width, fill, bar)
		return newRun(rows)
	}

	l := rows[at].line
	l.Fill, l.Bar = fill, bar
	rows[at].text = m.painter.Line(l, gutter, width)
	return newRun(rows)
}

func (m Model) hunkHead(h gh.Hunk, gutter, width int, key focusKey, open, split bool) string {
	marker := ""
	if open {
		marker = ""
	}

	head := paint.Header{Text: h.Header, Marker: marker}
	if m.cursorOn(key) {
		head.Fill, head.Bar = m.theme.SelectedBackground, m.theme.Accent
	}

	if split {
		return m.painter.HalfHeader(head, gutter, width)
	}
	return m.painter.HunkHeader(head, gutter, width)
}

func (m Model) paintRow(lines []gh.DiffLine, p pair, tokens [][]syntax.Token, gutter, width int, split bool) diffRow {
	if split {
		return m.splitRow(lines, p, tokens, gutter, width)
	}
	return m.diffRow(lines[p.left], tokens[p.left], gutter, width)
}

func (m Model) diffRow(l gh.DiffLine, tokens []syntax.Token, gutter, width int) diffRow {
	line := paint.Line{Kind: kindOf(l.Kind), Old: l.Old, New: l.New, Tokens: tokens}
	return diffRow{line: line, text: m.painter.Line(line, gutter, width)}
}

func kindOf(k gh.DiffKind) paint.Kind {
	switch k {
	case gh.DiffAdded:
		return paint.Added
	case gh.DiffRemoved:
		return paint.Removed
	}
	return paint.Context
}

// Tokenises each side as a whole file, because a lexer carries state across lines.
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

func (m *Model) diffThread(i, width int) rendered {
	t := m.detail.Detail.Threads[i]
	v := m.thread(t, width-threadIndent, false)
	v.block = indent(v.block, threadIndent)
	if v.boxAt > 0 {
		v.boxCol += threadIndent
	}
	return v
}

func (m *Model) fileText(f gh.ChangedFile, b block, width int, split bool) drawnFile {
	gutter := paint.Gutter(widest(f))

	var sb strings.Builder
	out := drawnFile{
		stops:    make([]focusItem, 0, len(b.stops)),
		rows:     make(map[focusKey]int, len(b.stops)),
		columns:  make(map[focusKey]gh.DiffSide, len(b.stops)),
		cursorAt: -1,
	}
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

	var owner focusKey
	for i, r := range b.runs {
		if owner != (focusKey{}) {
			column, rows := m.walkColumn(r, split)
			out.rows[owner], out.columns[owner] = rows, column
			if m.lit(owner) && m.walkedInto(owner) {
				if lit := r.rowAt(min(m.diffCursor, rows), column); lit >= 0 {
					out.cursorAt = at + lit
					r = m.litRun(r, lit, gutter, width, column)
				}
			}
		}
		write(r.text, len(r.rows))
		if i == len(b.stops) {
			break
		}

		s := b.stops[i]
		var text string
		if s.hunk != stopNone {
			h := f.Hunks[s.hunk]
			key := hunkKey(f.Path, h)
			text = m.hunkHead(h, gutter, width, key, m.hunkOpen(key), split)
			if m.cursorOn(key) {
				out.cursorAt = at
			}
			out.stops = append(out.stops, focusItem{focusKey: key, start: at, lines: 1})
			owner = key
		} else {
			v := m.diffThread(s.thread, width)
			text = v.block
			if v.boxAt > 0 {
				out.boxAt, out.boxCol = at+v.boxAt, v.boxCol
			}
			for _, st := range v.stops {
				if m.cursorOn(st.focusKey) {
					out.cursorAt = at + st.start
				}
				out.stops = append(out.stops, focusItem{focusKey: st.focusKey, start: at + st.start, lines: st.lines})
				out.rows[st.focusKey] = 0
				owner = st.focusKey
			}
		}

		write(text, strings.Count(text, "\n")+1)
	}

	out.text = sb.String()
	return out
}

// Reads the default without writing it: View runs on a copy.
func (m Model) hunkOpen(key focusKey) bool {
	open, set := m.open[key]
	return !set || open
}

func (m Model) hunkFoldState(f gh.ChangedFile) string {
	state := make([]byte, len(f.Hunks))
	for i, h := range f.Hunks {
		if !m.hunkOpen(hunkKey(f.Path, h)) {
			state[i] = 1
		}
	}
	return string(state)
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

// Walks the query's threads, not the map, whose random order reshuffles the comments each render.
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

func widest(f gh.ChangedFile) int {
	n := 0
	for _, h := range f.Hunks {
		for _, l := range h.Lines {
			n = max(n, l.Old, l.New)
		}
	}
	return n
}

func clipTo(line string, width int, mark lipgloss.Style) string {
	if lipgloss.Width(line) <= width {
		return line
	}
	return paint.Clip(line, width, mark)
}

func (m *Model) treeBody(width int) string {
	if !m.files.Loaded {
		return ""
	}
	if len(m.rows) == 0 {
		return m.faint().Render("No files changed.")
	}

	lines := make([]string, len(m.rows))
	for i, r := range m.rows {
		lines[i] = renderRow(m.theme, r, width, i == m.cursor)
	}
	return strings.Join(lines, "\n")
}

func (m Model) fileHeading() string {
	f := m.shownFile()
	if m.tab != tabFiles || f == nil || !m.files.Loaded {
		return ""
	}
	return " " + m.fileHead(*f, "", max(0, m.main.InnerWidth()-1))
}

func (m *Model) moveCursor(delta int) {
	if len(m.rows) == 0 {
		return
	}
	m.cursor = min(max(m.cursor+delta, 0), len(m.rows)-1)

	m.nameShownFile()
	m.showCursorRow()
	m.syncContent()
}

func (m *Model) showCursorRow() { showRow(&m.sideView, m.cursor) }

func (m Model) shownRows() []row {
	for _, r := range m.rows {
		if r.file != nil && r.file.Path == m.shownPath {
			return []row{r}
		}
	}
	return nil
}

func (m Model) shownFile() *gh.ChangedFile {
	if rows := m.shownRows(); len(rows) == 1 {
		return rows[0].file
	}
	return nil
}

func (m *Model) nameShownFile() {
	if m.cursor >= len(m.rows) {
		return
	}
	f := m.rows[m.cursor].file
	if f == nil || f.Path == m.shownPath {
		return
	}
	m.shownPath = f.Path

	if m.tab == tabFiles {
		m.view.SetYOffset(0)
	}
}

func (m *Model) jumpFile(delta int) bool {
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
		return true
	}
	return false
}

func (m *Model) crossFile(delta int) {
	was := m.shownPath
	for m.shownPath == was {
		if !m.jumpFile(delta) {
			return
		}
	}

	if m.pageRing.stops() == 0 {
		return
	}
	at := 0
	if delta < 0 {
		at = m.pageRing.stops() - 1
	}
	m.pageRing.on = m.pageRing.items[at].focusKey

	m.syncContent()
	m.showFocus(&m.pageRing, &m.view, bodyTop(&m.view))
}

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

func (m *Model) syncRows() {
	m.rows = flatten(buildTree(m.files.Files), m.collapsed, 0, nil)

	if m.shownFile() == nil {
		m.cursor = m.firstFile()
		m.nameShownFile()
	}
}
