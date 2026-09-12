package prview

import (
	"fmt"
	"image/color"
	"maps"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
)

const (
	searchSettleDelay  = 100 * time.Millisecond
	asyncJobRenderFrom = 256 << 10
)

// SearchSettleMsg applies Query to the job log search once typing pauses.
type SearchSettleMsg struct{ Query string }

type jobSection struct {
	step  gh.JobStep
	lines []string
	plain []string
}

func (m Model) checkStepFoldable() bool {
	return m.tab == tabChecks && m.focus == paneMain && m.check.job.Loaded &&
		m.check.step >= 0 && m.check.step < len(m.check.job.Job.Steps) &&
		m.check.step < len(m.check.sections) && len(m.check.sections[m.check.step].lines) > 0 &&
		m.check.step < len(m.check.stepStarts)
}

func (m *Model) moveCheckStep(delta int) bool {
	if !m.check.job.Loaded || len(m.check.job.Job.Steps) == 0 {
		return false
	}
	m.check.step = min(max(m.check.step+delta, 0), len(m.check.job.Job.Steps)-1)
	if m.check.step < len(m.check.stepStarts) {
		m.check.line = m.check.stepStarts[m.check.step]
	}
	m.showCheckStep()
	return true
}

func (m *Model) moveCheckLine(delta int) bool {
	if m.tab != tabChecks || m.focus != paneMain || !m.check.job.Loaded || m.check.stepLines == 0 {
		return false
	}
	m.check.line = min(max(m.check.line+delta, 0), m.check.stepLines-1)
	m.check.step = m.stepAtCheckLine(m.check.line)
	m.showCheckLine()
	return true
}

func (m *Model) pageCheckLine(delta int, full bool) bool {
	if !m.moveCheckLine(delta) {
		return false
	}
	switch {
	case full && delta > 0:
		m.view.PageDown()
	case full:
		m.view.PageUp()
	case delta > 0:
		m.view.HalfPageDown()
	default:
		m.view.HalfPageUp()
	}
	m.showCheckLine()
	return true
}

func (m *Model) gotoCheckLine(bottom bool) bool {
	if m.tab != tabChecks || m.focus != paneMain || !m.check.job.Loaded || m.check.stepLines == 0 {
		return false
	}
	m.check.line = 0
	if bottom {
		m.check.line = m.check.stepLines - 1
	}
	m.check.step = m.stepAtCheckLine(m.check.line)
	m.showCheckLine()
	return true
}

func (m Model) stepAtCheckLine(line int) int {
	at := 0
	for i, start := range m.check.stepStarts {
		if start > line {
			break
		}
		at = i
	}
	return at
}

func (m *Model) toggleCheckStep() {
	if !m.checkStepFoldable() {
		return
	}
	number := m.check.job.Job.Steps[m.check.step].Number
	open := m.check.stepOpen[number]
	if !m.check.stepSeen[number] && rank(m.check.job.Job.Steps[m.check.step].State) >= rank(gh.CheckStateFailure) {
		open = true
	}
	m.check.stepSeen[number] = true
	m.check.stepOpen[number] = !open
	m.check.line = m.check.stepStarts[m.check.step]
	m.staleJobRender()
	m.syncContent()
	m.showCheckLine()
}

func (m *Model) showCheckStep() {
	if m.check.step < len(m.check.stepStarts) {
		m.view.SetYOffset(contentLead + m.jobStepLead() + m.check.stepStarts[m.check.step])
	}
}

func (m *Model) showCheckLine() {
	line := contentLead + m.jobStepLead() + m.check.line
	top, height := m.view.YOffset(), max(1, m.view.Height())
	switch {
	case line < top:
		m.view.SetYOffset(line)
	case line >= top+height:
		m.view.SetYOffset(line - height + 1)
	}
}

func (m Model) jobStepLead() int {
	return 6
}

const (
	checkSearchLabel = "Search: "

	// The space before the label is measured with the row, in checkSearchRow.
	checkSearchLead = 1
)

// bodyGutter is left out on purpose: it centres the viewport, and this row is the pane's heading.
func (m Model) checkCursor(lead int) *tea.Cursor {
	if m.tab != tabChecks || !m.check.searching {
		return nil
	}

	left, right, width := m.checkSearchRow()
	room := max(0, width-lipgloss.Width(right)-1)

	col := m.mainLeft() + checkSearchLead + min(lipgloss.Width(left), room)
	return comp.Cursor(m.theme, col, lead+1)
}

func (m Model) mainHeading() string {
	if m.tab != tabChecks || (!m.check.searching && m.check.search.Empty()) {
		return m.fileHeading()
	}

	left, right, width := m.checkSearchRow()
	return m.checkLine(left, right, width, lipgloss.NewStyle())
}

func (m Model) checkSearchRow() (left, right string, width int) {
	left = " " + m.faint().Render(checkSearchLabel) + m.check.search.Query()
	if !m.check.search.Empty() && m.check.renderQuery == m.check.search.Query() {
		at, total := 0, len(m.check.matchLines)
		if total > 0 {
			at = m.check.search.Cursor() + 1
		}
		right = m.faint().Render(fmt.Sprintf("%d/%d", at, total))
	}
	return left, right, max(1, m.main.InnerWidth()-1)
}

func (m *Model) startCheckSearch() {
	if m.tab != tabChecks || !m.check.job.Loaded {
		return
	}
	m.check.searchStep = m.stepAtCheckLine(m.check.line)
	if m.check.searchStep < len(m.check.stepStarts) {
		m.check.searchWithin = m.check.line - m.check.stepStarts[m.check.searchStep]
	}
	m.check.searching = true
	m.focus = paneMain
	m.layout()
}

func (m *Model) clearCheckSearch() {
	step, within := m.check.searchStep, m.check.searchWithin
	m.check.searching = false
	m.check.search = comp.Search{}
	m.layout()
	if len(m.check.stepStarts) == 0 {
		return
	}
	step = min(max(step, 0), len(m.check.stepStarts)-1)
	start, end := m.check.stepStarts[step], m.check.stepLines
	if step+1 < len(m.check.stepStarts) {
		end = m.check.stepStarts[step+1]
	}
	m.check.step = step
	m.check.line = min(start+within, end-1)
	m.syncContent()
	m.showCheckLine()
}

func (m *Model) checkSearchKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.clearCheckSearch()
		return *m, nil
	case "enter":
		m.check.searching = false
		if m.check.search.Empty() {
			m.layout()
		} else {
			m.settleCheckSearch(SearchSettleMsg{Query: m.check.search.Query()})
		}
		return *m, nil
	}
	if !m.check.search.Insert(msg) {
		return *m, nil
	}
	query := m.check.search.Query()
	return *m, tea.Tick(searchSettleDelay, func(time.Time) tea.Msg {
		return SearchSettleMsg{Query: query}
	})
}

func (m *Model) settleCheckSearch(msg SearchSettleMsg) tea.Cmd {
	if msg.Query != m.check.search.Query() {
		return nil
	}
	m.syncContent()
	m.showCheckMatch()
	return m.armJobRender()
}

func (m *Model) moveCheckMatch(delta int) {
	if m.check.search.Move(delta, len(m.check.matchLines)) {
		m.showCheckMatch()
	}
}

func (m *Model) showCheckMatch() {
	at := m.check.search.Cursor()
	if at < 0 || at >= len(m.check.matchLines) {
		return
	}
	m.check.line = m.check.matchLines[at]
	m.check.step = m.stepAtCheckLine(m.check.line)
	m.showCheckLine()
}

func (m *Model) jumpFirstCheckFailure() {
	if m.tab != tabChecks || !m.check.job.Loaded {
		return
	}
	for i, step := range m.check.job.Job.Steps {
		if step.State != gh.CheckStateFailure && step.State != gh.CheckStateError {
			continue
		}
		m.check.step = i
		if i < len(m.check.stepStarts) {
			m.check.line = m.check.stepStarts[i]
		}
		m.focus = paneMain
		m.showCheckStep()
		return
	}
}

func (m Model) jobNote(text string, width int) string {
	return indent(wrap(text, max(1, width-jobNoteInset*2)), jobNoteInset)
}

const jobNoteInset = 2

func (m *Model) jobBody(check gh.Check, width int) string {
	summary := m.jobSummary(check, width)
	if check.JobID == 0 {
		return summary + "\n\n" + m.jobNote(m.faint().Render("No job log is available for this status check."), width)
	}

	var body string
	switch {
	case m.check.parsing:
		body = m.jobNote(m.faint().Render("Processing the job log…"), width)
	case m.checkRerunning(m.check.selected):
		body = m.jobNote(m.spinner.Render("Waiting for the new attempt"), width)
	case m.check.job.Loaded:
		body = m.jobSteps(width)
		if m.check.job.Status == store.StatusFailed {
			body += "\n\n" + m.jobNote(m.faint().Render("Log output is unavailable: "+m.check.job.Err.Error()), width)
		}
	case m.check.job.Status == store.StatusFailed:
		body = m.jobNote(m.faint().Render("Could not load the job log: "+m.check.job.Err.Error()), width)
	default:
		body = m.jobNote(m.spinner.Render("Loading the job log"), width)
	}
	return summary + "\n\n" + body
}

func (m Model) jobSummary(check gh.Check, width int) string {
	name := cleanJobLabel(check.Name)
	if check.Workflow != "" {
		name = cleanJobLabel(check.Workflow) + " / " + name
	}
	state, started, completed, duration := check.State, check.StartedAt, check.CompletedAt, check.Duration
	if m.check.job.Loaded && m.check.job.Job.ID == check.JobID {
		state = m.check.job.Job.State
		started = m.check.job.Job.StartedAt
		completed = m.check.job.Job.CompletedAt
		duration = m.check.job.Job.Duration
	}
	icon, c := comp.CheckStateIcon(m.theme, state)
	head := " " + lipgloss.NewStyle().Foreground(c).Render(icon) + " " +
		lipgloss.NewStyle().Foreground(m.theme.Text).Bold(true).Render(name)

	label, lc := comp.CheckStateLabel(m.theme, state)
	if m.checkRerunning(m.check.selected) {
		label, lc = "rerunning", m.theme.Warning
	}
	parts := []string{lipgloss.NewStyle().Foreground(lc).Render(label)}
	if age := comp.LongAgo(completed); age != "" {
		parts = append(parts, age)
	} else if age := comp.LongAgo(started); age != "" {
		parts = append(parts, "started "+age)
	}
	if d := shortDuration(duration); d != "" {
		parts = append(parts, d)
	}
	line := " " + strings.Join(parts, m.faint().Render(" · "))

	pane := comp.NewPane(m.theme).Header(clipTo(head, max(1, width-2), m.faint()))
	return pane.Size(width, pane.Chrome()+1).Render(clipTo(line, max(1, width-2), m.faint()))
}

func shortDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	if d < time.Second {
		return "<1s"
	}
	return d.Round(time.Second).String()
}

type jobRenderMsg struct {
	id           int64
	token        uint64
	rendered     []string
	renderedBody string
	stepStarts   []int
	matchLines   []int
	stepLines    int
	width        int
	query        string
}

func (m *Model) jobSteps(width int) string {
	sections := m.check.sections
	if len(sections) == 0 {
		m.check.stepLines = 0
		return m.jobNote(m.faint().Render("No steps were reported for this job."), width)
	}
	if m.check.step >= len(sections) {
		m.check.step = len(sections) - 1
	}
	if m.check.stepOpen == nil {
		m.check.stepOpen = make(map[int]bool)
	}
	if m.check.stepSeen == nil {
		m.check.stepSeen = make(map[int]bool)
	}

	query := m.check.search.Query()
	if m.check.rendered == nil || m.check.renderWidth != width || m.check.renderQuery != query {
		if !m.largeJobLog() {
			m.renderJobSteps(width)
		} else {
			m.startJobRender(width, query)
			if m.check.renderedBody != "" {
				return m.check.renderedBody
			}
			return m.faint().Render("Processing the job log…")
		}
	}
	m.check.line = min(max(m.check.line, 0), m.check.stepLines-1)
	m.check.step = m.stepAtCheckLine(m.check.line)

	return m.check.renderedBody
}

func (m Model) largeJobLog() bool {
	bytes := 0
	for _, section := range m.check.sections {
		for _, line := range section.lines {
			bytes += len(line)
			if bytes >= asyncJobRenderFrom {
				return true
			}
		}
	}
	return false
}

func (m *Model) startJobRender(width int, query string) {
	if m.check.rendering && m.check.renderWantWidth == width && m.check.renderWantQuery == query {
		return
	}
	m.check.rendering = true
	m.check.renderToken++
	m.check.renderWantWidth = width
	m.check.renderWantQuery = query
}

func (m Model) armJobRender() tea.Cmd {
	if !m.check.rendering || m.check.job.Job.ID == 0 {
		return nil
	}
	clone := m
	clone.check.stepOpen = maps.Clone(m.check.stepOpen)
	clone.check.stepSeen = maps.Clone(m.check.stepSeen)
	clone.check.rendered = nil
	clone.check.renderedBody = ""
	id, token, width, query := m.check.job.Job.ID, m.check.renderToken,
		m.check.renderWantWidth, m.check.renderWantQuery
	return func() tea.Msg {
		clone.renderJobSteps(width)
		return jobRenderMsg{
			id: id, token: token, rendered: clone.check.rendered,
			renderedBody: clone.check.renderedBody, stepStarts: clone.check.stepStarts,
			matchLines: clone.check.matchLines, stepLines: clone.check.stepLines,
			width: width, query: query,
		}
	}
}

func (m *Model) jobRendered(msg jobRenderMsg) tea.Cmd {
	if !m.check.rendering || msg.id != m.check.job.Job.ID || msg.token != m.check.renderToken ||
		msg.width != m.check.renderWantWidth || msg.query != m.check.renderWantQuery {
		return nil
	}
	m.check.rendered = msg.rendered
	m.check.renderedBody = msg.renderedBody
	m.check.stepStarts = msg.stepStarts
	m.check.matchLines = msg.matchLines
	m.check.stepLines = msg.stepLines
	m.check.renderWidth = msg.width
	m.check.renderQuery = msg.query
	m.check.rendering = false
	m.syncContent()
	m.showCheckLine()
	return nil
}

func (m *Model) renderJobSteps(width int) {
	sections := m.check.sections
	totalLogLines := 0
	for _, section := range sections {
		totalLogLines += len(section.lines)
	}
	lineNumberWidth := len(strconv.Itoa(max(1, totalLogLines)))
	opens := make([]bool, len(sections))
	matches := make([][]bool, len(sections))
	m.check.stepStarts = make([]int, len(sections))
	m.check.matchLines = nil
	m.check.stepLines = 0
	for i, section := range sections {
		m.check.stepStarts[i] = m.check.stepLines
		open := m.check.stepOpen[section.step.Number]
		if !m.check.stepSeen[section.step.Number] && rank(section.step.State) >= rank(gh.CheckStateFailure) {
			open = true
		}
		matches[i] = make([]bool, len(section.plain))
		for line, plain := range section.plain {
			matches[i][line] = m.check.search.Matches(plain)
			open = open || matches[i][line]
		}
		open = open && len(section.lines) > 0
		opens[i] = open
		m.check.stepLines++
		if open {
			m.check.stepLines += len(section.lines)
		}
	}

	m.check.rendered = make([]string, 0, m.check.stepLines)
	firstLine := 1
	for i, section := range sections {
		start := len(m.check.rendered)
		row, matchedLines := m.jobStepRow(
			section, width, opens[i], matches[i], firstLine, lineNumberWidth,
		)
		m.check.rendered = append(m.check.rendered, strings.Split(row, "\n")...)
		for _, line := range matchedLines {
			m.check.matchLines = append(m.check.matchLines, start+line)
		}
		firstLine += len(section.lines)
	}
	m.check.renderedBody = strings.Join(m.check.rendered, "\n")
	m.check.renderWidth = width
	m.check.renderQuery = m.check.search.Query()
}

func (m Model) jobStepRow(
	section jobSection,
	width int,
	open bool,
	lineMatches []bool,
	firstLine int,
	lineNumberWidth int,
) (string, []int) {
	base := lipgloss.NewStyle()
	fold := " "
	if len(section.lines) > 0 {
		fold = "▸"
		if open {
			fold = "▾"
		}
	}
	icon, c := comp.CheckStateIcon(m.theme, section.step.State)
	lead := base.Foreground(m.theme.Subtle).Render(fold) + base.Render(" ") +
		base.Foreground(c).Render(icon) + base.Render(" ") +
		base.Foreground(m.theme.Text).Render(cleanJobLabel(section.step.Name))
	rightText := shortDuration(section.step.Duration)
	if len(section.lines) == 0 && !section.step.StartedAt.IsZero() && section.step.CompletedAt.IsZero() {
		rightText = "Log output is not available yet."
	}
	right := base.Foreground(m.theme.Subtle).Render(rightText)
	head := m.padTo(m.checkLine(lead, right, width, base), width, base)
	if !open || len(section.lines) == 0 {
		return head, nil
	}

	lines := make([]string, 0, len(section.lines)+1)
	lines = append(lines, head)
	var matches []int
	mark := lipgloss.NewStyle().Background(m.theme.SelectedBackground).Foreground(m.theme.Warning).Bold(true)
	for i, line := range section.lines {
		plain := section.plain[i]
		if lineMatches[i] {
			matches = append(matches, len(lines))
			line = m.check.search.Highlight(plain, mark)
		} else {
			line = m.styleJobLogLine(line)
		}
		gutter := lipgloss.NewStyle().Foreground(m.theme.MutedOrSubtle()).Render(
			fmt.Sprintf("  %*d ", lineNumberWidth, firstLine+i),
		)
		lines = append(lines, clipTo(gutter+line, width, m.faint()))
	}
	return strings.Join(lines, "\n"), matches
}

func (m Model) paintCheckCursor(view string) string {
	if !m.check.job.Loaded || m.check.parsing || m.check.stepLines == 0 {
		return view
	}
	at := contentLead + m.jobStepLead() + m.check.line - m.view.YOffset()
	rows := strings.Split(view, "\n")
	if at < 0 || at >= len(rows) {
		return view
	}
	gutter := m.bodyGutter()
	line := rows[at]
	if gutter > len(line) {
		return view
	}
	prefix, line := line[:gutter], line[gutter:]
	rows[at] = prefix + selectedJobLogLine(line, m.bodyWidth(), m.theme.SelectedBackground, m.faint())
	return strings.Join(rows, "\n")
}

// Reapplies the fill after every SGR run; a nil fill must never reach RGBA, which panics on one.
func selectedJobLogLine(line string, width int, fill color.Color, faint lipgloss.Style) string {
	line = clipTo(line, width, faint)
	if pad := width - lipgloss.Width(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	if fill == nil {
		return line
	}
	r, g, b, _ := fill.RGBA()
	background := fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r>>8, g>>8, b>>8)

	var out strings.Builder
	out.Grow(len(line) + len(background)*2)
	out.WriteString(background)
	for len(line) > 0 {
		at := strings.Index(line, "\x1b[")
		if at < 0 {
			out.WriteString(line)
			break
		}
		out.WriteString(line[:at])
		line = line[at:]
		end := strings.IndexByte(line, 'm')
		if end < 0 {
			out.WriteString(line)
			break
		}
		out.WriteString(line[:end+1])
		out.WriteString(background)
		line = line[end+1:]
	}
	out.WriteString("\x1b[0m")
	return out.String()
}

func splitJobLog(job gh.Job, raw string) []jobSection {
	if len(job.Steps) == 0 {
		return nil
	}
	out := make([]jobSection, len(job.Steps))
	for i, step := range job.Steps {
		out[i].step = step
	}

	at, next := 0, 1
	raw = strings.TrimSuffix(raw, "\n")
	for raw != "" {
		line, rest, found := strings.Cut(raw, "\n")
		raw = rest
		timestamp, text, ok := jobLogLine(line)
		if ok {
			for next < len(job.Steps) {
				if job.Steps[next].StartedAt.IsZero() {
					next++
					continue
				}
				if timestamp.Before(job.Steps[next].StartedAt) {
					break
				}
				at, next = next, next+1
			}
		}
		text = cleanJobLogLine(text)
		if text == "" || text == "##[endgroup]" || strings.HasPrefix(text, "##[group]") {
			continue
		}
		out[at].lines = append(out[at].lines, text)
		out[at].plain = append(out[at].plain, xansi.Strip(text))
		if !found {
			break
		}
	}
	return out
}

func jobLogLine(line string) (time.Time, string, bool) {
	stamp, rest, ok := strings.Cut(line, " ")
	if !ok {
		return time.Time{}, line, false
	}
	at, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return time.Time{}, line, false
	}
	return at, rest, true
}

// Logs are untrusted: SGR is kept, and every other escape and control is dropped.
func cleanJobLabel(label string) string {
	label = xansi.Strip(label)
	label = strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t':
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, label)
	return strings.TrimSpace(label)
}

func cleanJobLogLine(line string) string {
	line = strings.ReplaceAll(strings.TrimSuffix(line, "\r"), "\t", "    ")

	parser := xansi.NewParser()
	state := byte(xansi.NormalState)
	var out strings.Builder
	styled := false
	for len(line) > 0 {
		seq, width, n, next := xansi.DecodeSequence(line, state, parser)
		if n <= 0 {
			break
		}
		state = next
		switch {
		case sgrSequence(seq, parser):
			out.WriteString(seq)
			styled = true
		case width > 0 || printableSequence(seq):
			out.WriteString(seq)
		}
		line = line[n:]
	}
	if styled {
		out.WriteString(xansi.ResetStyle)
	}
	return out.String()
}

func sgrSequence(seq string, parser *xansi.Parser) bool {
	if len(seq) < 3 || (!strings.HasPrefix(seq, "\x1b[") && seq[0] != xansi.CSI) {
		return false
	}
	cmd := xansi.Cmd(parser.Command())
	return cmd.Final() == 'm' && cmd.Prefix() == 0 && cmd.Intermediate() == 0
}

func printableSequence(seq string) bool {
	if !utf8.ValidString(seq) {
		return false
	}
	for _, r := range seq {
		if unicode.IsControl(r) {
			return false
		}
	}
	return seq != ""
}

func (m Model) styleJobLogLine(line string) string {
	plain := strings.TrimSpace(xansi.Strip(line))
	if plain != strings.TrimSpace(line) {
		return line
	}

	var style lipgloss.Style
	switch {
	case strings.HasPrefix(plain, "##[error]"), strings.HasPrefix(plain, "::error"):
		style = lipgloss.NewStyle().Foreground(m.theme.Error)
	case strings.HasPrefix(plain, "##[warning]"), strings.HasPrefix(plain, "::warning"):
		style = lipgloss.NewStyle().Foreground(m.theme.Warning)
	case strings.HasPrefix(plain, "##[notice]"), strings.HasPrefix(plain, "::notice"):
		style = lipgloss.NewStyle().Foreground(m.theme.Accent)
	case strings.HasPrefix(plain, "##[command]"), strings.HasPrefix(plain, "::debug"):
		style = lipgloss.NewStyle().Foreground(m.theme.Subtle)
	default:
		return line
	}
	return style.Render(line)
}
