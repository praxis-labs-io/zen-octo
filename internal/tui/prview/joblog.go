package prview

import (
	"fmt"
	"image/color"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
)

type jobSection struct {
	step  gh.JobStep
	lines []string
}

func (m Model) checkHasFailure() bool {
	for _, step := range m.check.job.Job.Steps {
		if step.State == gh.CheckStateFailure || step.State == gh.CheckStateError {
			return true
		}
	}
	return false
}

func (m Model) checkStepFoldable() bool {
	return m.tab == tabChecks && m.focus == paneMain && m.check.job.Loaded &&
		m.check.step >= 0 && m.check.step < len(m.check.job.Job.Steps) &&
		m.check.step < len(m.check.sections) && len(m.check.sections[m.check.step].lines) > 0 &&
		m.check.step < len(m.check.stepStarts)
}

// moveCheckStep is block motion. Line motion is j and k; braces move between
// headings and leave every expanded output line to those keys.
func (m *Model) moveCheckStep(delta int) bool {
	if !m.check.job.Loaded || len(m.check.job.Job.Steps) == 0 {
		return false
	}
	m.check.step = min(max(m.check.step+delta, 0), len(m.check.job.Job.Steps)-1)
	if m.check.step < len(m.check.stepStarts) {
		m.check.line = m.check.stepStarts[m.check.step]
	}
	m.syncContent()
	m.showCheckStep()
	return true
}

func (m *Model) moveCheckLine(delta int) bool {
	if m.tab != tabChecks || m.focus != paneMain || !m.check.job.Loaded || m.check.stepLines == 0 {
		return false
	}
	m.check.line = min(max(m.check.line+delta, 0), m.check.stepLines-1)
	m.check.step = m.stepAtCheckLine(m.check.line)
	m.syncContent()
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
	m.syncContent()
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
	m.check.rendered = nil
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
	return 6 // five summary-card rows and the blank below it
}

func (m Model) mainHeading() string {
	if m.tab != tabChecks || (!m.check.searching && m.check.search.Empty()) {
		return m.fileHeading()
	}

	caret := ""
	if m.check.searching {
		caret = lipgloss.NewStyle().Foreground(m.theme.Accent).Render("▏")
	}
	left := " " + m.faint().Render("Search: ") + m.check.search.Query() + caret
	right := ""
	if !m.check.search.Empty() {
		at, total := 0, len(m.check.matchLines)
		if total > 0 {
			at = m.check.search.Cursor() + 1
		}
		right = m.faint().Render(fmt.Sprintf("%d/%d", at, total))
	}
	return m.checkLine(left, right, max(1, m.main.InnerWidth()-1), lipgloss.NewStyle())
}

func (m *Model) startCheckSearch() {
	if m.tab != tabChecks || !m.check.job.Loaded {
		return
	}
	m.check.searching = true
	m.focus = paneMain
	m.layout()
}

func (m *Model) clearCheckSearch() {
	m.check.searching = false
	m.check.search = comp.Search{}
	m.layout()
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
		}
		return *m, nil
	}
	if m.check.search.Insert(msg) {
		m.syncContent()
		m.showCheckMatch()
	}
	return *m, nil
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
	m.syncContent()
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
		m.syncContent()
		m.showCheckStep()
		return
	}
}

func (m *Model) jobBody(check gh.Check, width int) string {
	summary := m.jobSummary(check, width)
	if check.JobID == 0 {
		return summary + "\n\n" + m.faint().Render("No job log is available for this status check.")
	}

	var body string
	switch {
	case m.check.job.Loaded:
		body = m.jobSteps(width)
	case m.check.job.Status == store.StatusFailed:
		body = m.faint().Render("Could not load the job log: " + m.check.job.Err.Error())
	default:
		body = m.spinner.Render("Loading the job log")
	}
	return summary + "\n\n" + body
}

func (m Model) jobSummary(check gh.Check, width int) string {
	name := check.Name
	if check.Workflow != "" {
		name = check.Workflow + " / " + check.Name
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

func (m *Model) jobSteps(width int) string {
	sections := m.check.sections
	if len(sections) == 0 {
		m.check.stepLines = 0
		return m.faint().Render("No steps were reported for this job.")
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
		m.renderJobSteps(width)
	}
	m.check.line = min(max(m.check.line, 0), m.check.stepLines-1)
	m.check.step = m.stepAtCheckLine(m.check.line)

	var out strings.Builder
	for i, line := range m.check.rendered {
		if i > 0 {
			out.WriteByte('\n')
		}
		if i == m.check.line {
			line = selectedJobLogLine(line, width, m.theme.SelectedBackground, m.faint())
		}
		out.WriteString(line)
	}
	return out.String()
}

// renderJobSteps pays the highlighting and clipping cost only when the log's
// shape changes. Cursor and match motion then repaint one row and join the
// already-rendered lines instead of parsing the whole log again.
func (m *Model) renderJobSteps(width int) {
	sections := m.check.sections
	opens := make([]bool, len(sections))
	m.check.stepStarts = make([]int, len(sections))
	m.check.matchLines = nil
	m.check.stepLines = 0
	for i, section := range sections {
		m.check.stepStarts[i] = m.check.stepLines
		open := m.check.stepOpen[section.step.Number]
		if !m.check.stepSeen[section.step.Number] && rank(section.step.State) >= rank(gh.CheckStateFailure) {
			open = true
		}
		if sectionMatches(m.check.search, section) {
			open = true
		}
		open = open && len(section.lines) > 0
		opens[i] = open
		m.check.stepLines++
		if open {
			m.check.stepLines += len(section.lines)
		}
	}

	m.check.rendered = make([]string, 0, m.check.stepLines)
	for i, section := range sections {
		start := len(m.check.rendered)
		row, matches := m.jobStepRow(section, width, opens[i])
		m.check.rendered = append(m.check.rendered, strings.Split(row, "\n")...)
		for _, line := range matches {
			m.check.matchLines = append(m.check.matchLines, start+line)
		}
	}
	m.check.renderWidth = width
	m.check.renderQuery = m.check.search.Query()
}

func sectionMatches(search comp.Search, section jobSection) bool {
	for _, line := range section.lines {
		if search.Matches(xansi.Strip(line)) {
			return true
		}
	}
	return false
}

func (m Model) jobStepRow(section jobSection, width int, open bool) (string, []int) {
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
		base.Foreground(m.theme.Text).Render(section.step.Name)
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
	for _, line := range section.lines {
		if m.check.search.Matches(xansi.Strip(line)) {
			matches = append(matches, len(lines))
			line = m.check.search.Highlight(line, mark)
		}
		line = m.styleJobLogLine(line)
		lines = append(lines, clipTo("    "+line, width, m.faint()))
	}
	return strings.Join(lines, "\n"), matches
}

// selectedJobLogLine reapplies the cursor background after every SGR run. A
// log line may reset or set its own colours, so wrapping the finished string in
// a background style would paint only as far as its first reset.
func selectedJobLogLine(line string, width int, fill color.Color, faint lipgloss.Style) string {
	line = clipTo(line, width, faint)
	if pad := width - lipgloss.Width(line); pad > 0 {
		line += strings.Repeat(" ", pad)
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

// splitJobLog uses the timestamps GitHub prefixes to every downloaded line to
// assign it to the step whose interval contains it. Step metadata remains the
// authority, so a skipped step still gets a row even when it has no lines.
func splitJobLog(job gh.Job, raw string) []jobSection {
	if len(job.Steps) == 0 {
		return nil
	}
	out := make([]jobSection, len(job.Steps))
	for i, step := range job.Steps {
		out[i].step = step
	}

	at, next := 0, 1
	for _, line := range strings.Split(strings.TrimSuffix(raw, "\n"), "\n") {
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

// Logs are untrusted terminal text. SGR changes how their own text looks and
// is safe to keep; every other escape and control could move the cursor,
// rewrite chrome, or open a terminal command, so it is dropped.
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
	for _, r := range seq {
		if unicode.IsControl(r) {
			return false
		}
	}
	return seq != ""
}

// styleJobLogLine is the fallback for GitHub's own annotations, which carry a
// semantic marker even when the command that wrote them emitted no ANSI.
func (m Model) styleJobLogLine(line string) string {
	plain := strings.TrimSpace(xansi.Strip(line))
	// A tool that chose its own colors keeps them. Wrapping ANSI in another
	// style loses the outer color at the first inner reset.
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
