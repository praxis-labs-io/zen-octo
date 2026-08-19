package prview

import (
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
		m.check.step >= 0 && m.check.step < len(m.check.job.Job.Steps)
}

func (m *Model) moveCheckStep(delta int) bool {
	if !m.checkStepFoldable() {
		return false
	}
	m.check.step = min(max(m.check.step+delta, 0), len(m.check.job.Job.Steps)-1)
	m.syncContent()
	m.showCheckStep()
	return true
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
	m.syncContent()
	m.showCheckStep()
}

func (m *Model) showCheckStep() {
	if m.check.step < len(m.check.stepStarts) {
		m.view.SetYOffset(contentLead + m.jobStepLead() + m.check.stepStarts[m.check.step])
	}
}

func (m Model) jobStepLead() int {
	lead := 6 // five summary-card rows and the blank below it
	if m.check.searching || !m.check.search.Empty() {
		lead += 2 // query and the blank below it
	}
	return lead
}

func (m *Model) startCheckSearch() {
	if m.tab != tabChecks || !m.check.job.Loaded {
		return
	}
	m.check.searching = true
	m.focus = paneMain
	m.syncContent()
}

func (m *Model) checkSearchKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter":
		m.check.searching = false
		m.syncContent()
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
	if at >= 0 && at < len(m.check.matchLines) {
		m.view.SetYOffset(contentLead + m.jobStepLead() + m.check.matchLines[at])
	}
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
	if m.check.searching || !m.check.search.Empty() {
		query := m.check.search.Query()
		caret := ""
		if m.check.searching {
			caret = "▏"
		}
		body = m.faint().Render("Search: ") + query + caret + "\n\n" + body
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
	sections := splitJobLog(m.check.job.Job, m.check.job.Log)
	if len(sections) == 0 {
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

	rows := make([]string, 0, len(sections))
	m.check.stepStarts = make([]int, len(sections))
	m.check.matchLines = nil
	at := 0
	for i, section := range sections {
		m.check.stepStarts[i] = at
		open := m.check.stepOpen[section.step.Number]
		if !m.check.stepSeen[section.step.Number] && rank(section.step.State) >= rank(gh.CheckStateFailure) {
			open = true
		}
		if sectionMatches(m.check.search, section) {
			open = true
		}
		row, matches := m.jobStepRow(section, width, i == m.check.step, open)
		for _, line := range matches {
			m.check.matchLines = append(m.check.matchLines, at+line)
		}
		rows = append(rows, row)
		at += strings.Count(row, "\n") + 1
	}
	return strings.Join(rows, "\n")
}

func sectionMatches(search comp.Search, section jobSection) bool {
	for _, line := range section.lines {
		if search.Matches(xansi.Strip(line)) {
			return true
		}
	}
	return false
}

func (m Model) jobStepRow(section jobSection, width int, selected, open bool) (string, []int) {
	base := lipgloss.NewStyle()
	if selected {
		base = base.Background(m.theme.SelectedBackground)
	}
	fold := "▸"
	if open {
		fold = "▾"
	}
	icon, c := comp.CheckStateIcon(m.theme, section.step.State)
	lead := base.Foreground(m.theme.Subtle).Render(fold) + base.Render(" ") +
		base.Foreground(c).Render(icon) + base.Render(" ") +
		base.Foreground(m.theme.Text).Render(section.step.Name)
	right := base.Foreground(m.theme.Subtle).Render(shortDuration(section.step.Duration))
	head := m.padTo(m.checkLine(lead, right, width, base), width, base)
	if !open {
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
		line = "    " + line
		lines = append(lines, clipTo(line, width, m.faint()))
	}
	if len(section.lines) == 0 {
		lines = append(lines, m.faint().Render("    No log output."))
	}
	return strings.Join(lines, "\n"), matches
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
