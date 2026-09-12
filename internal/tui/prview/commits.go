package prview

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/paint"
)

const commitRowHeight = 2

const commitSettleDelay = 150 * time.Millisecond

// sha is painted, pending is asked for; kept apart so a cached diff flashes no spinner.
type commits struct {
	cursor  int
	sha     string
	pending string
	files   store.Files
	rows    []row
	diff    diffBody
}

type NeedCommitMsg struct{ SHA string }

type CommitSettleMsg struct{ SHA string }

// SetCommitFiles shows f if sha is on the pane or asked for, and drops it otherwise.
func (m *Model) SetCommitFiles(sha string, f store.Files) {
	if sha == m.commit.pending {
		m.commit.pending = ""
	}

	took := sha != m.commit.sha
	if took {
		if under, ok := m.underCursor(); !ok || under != sha {
			return
		}
		m.commit.sha = sha
	}

	m.commit.files = f
	m.commit.diff.blocks = nil
	m.commit.rows = flatten(buildTree(f.Files), nil, 0, nil)
	m.syncContent()

	if took && m.tab == tabCommits {
		m.view.GotoTop()
	}
}

func (m *Model) syncCommits() {
	m.commit.cursor = min(m.commit.cursor, max(0, len(m.detail.Detail.Commits)-1))
}

func (m Model) wanted() (string, bool) {
	sha, ok := m.underCursor()
	if m.tab != tabCommits || !ok {
		return "", false
	}
	if sha == m.commit.sha && m.commit.files.Status != store.StatusFailed {
		return sha, false
	}
	return sha, true
}

func (m Model) armCommit() tea.Cmd {
	sha, want := m.wanted()
	if !want {
		return nil
	}
	return tea.Tick(commitSettleDelay, func(time.Time) tea.Msg {
		return CommitSettleMsg{SHA: sha}
	})
}

func (m *Model) settleCommit(msg CommitSettleMsg) tea.Cmd {
	sha, want := m.wanted()
	if !want || sha != msg.SHA || sha == m.commit.pending {
		return nil
	}

	m.commit.pending = sha
	return func() tea.Msg { return NeedCommitMsg{SHA: sha} }
}

func (m Model) underCursor() (string, bool) {
	list := m.detail.Detail.Commits
	if m.commit.cursor >= len(list) || list[m.commit.cursor].SHA == "" {
		return "", false
	}
	return list[m.commit.cursor].SHA, true
}

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

func (m Model) commitRow(c gh.Commit, width int, selected bool) []string {
	base := lipgloss.NewStyle()
	if selected {
		base = base.Background(m.theme.SelectedBackground)
	}

	_, checks := comp.CheckStateIcon(m.theme, c.Checks)
	head := base.Foreground(checks).Render(glyphCheck) + base.Render(" ") +
		base.Foreground(m.theme.Text).Render(c.Headline)

	by := base.Render("  ") + base.Foreground(m.theme.Accent).Render(c.Short)
	if who := commitBy(c); who != "" {
		by += base.Foreground(m.theme.Subtle).Render(" · " + who)
	}

	return []string{m.padTo(head, width, base), m.padTo(by, width, base)}
}

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

func (m Model) padTo(line string, width int, base lipgloss.Style) string {
	switch w := lipgloss.Width(line); {
	case w > width:
		return paint.Clip(line, width, base.Foreground(m.theme.Subtle))
	case w < width:
		return line + base.Render(strings.Repeat(" ", width-w))
	}
	return line
}

func (m *Model) commitBody() string {
	switch {
	case m.commit.sha == "":
		if _, ok := m.underCursor(); !ok {
			return ""
		}
		return m.spinner.Render("Loading the diff")
	case m.commit.files.Loaded:
		card := m.commitCard(m.bodyWidth())
		if card == "" {
			m.commit.diff.lead = 0
			return m.renderDiff(m.commit.rows, m.commit.files, &m.commit.diff)
		}
		m.commit.diff.lead = strings.Count(card, "\n") + 2
		return card + "\n\n" + m.renderDiff(m.commit.rows, m.commit.files, &m.commit.diff)
	case m.commit.files.Status == store.StatusFailed:
		return m.faint().Render("Could not load the diff: " + m.commit.files.Err.Error())
	}
	return m.spinner.Render("Loading the diff")
}

func (m Model) commitCard(width int) string {
	c, ok := m.selected()
	if !ok {
		return ""
	}

	inner := max(1, width-2)
	_, checks := comp.CheckStateIcon(m.theme, c.Checks)

	lines := []string{wrap(lipgloss.NewStyle().Foreground(checks).Render(glyphCheck)+" "+
		lipgloss.NewStyle().Foreground(m.theme.Text).Bold(true).Render(c.Headline), inner)}

	if body := strings.TrimSpace(c.Body); body != "" {
		lines = append(lines, "", wrap(m.faint().Render(body), inner))
	}

	meta := lipgloss.NewStyle().Foreground(m.theme.Accent).Render(c.SHA)
	if who := commitBy(c); who != "" {
		meta += m.faint().Render(" · " + who)
	}
	lines = append(lines, "", wrap(meta, inner))

	body := strings.Join(lines, "\n")
	pane := comp.NewPane(m.theme)
	return pane.Size(width, strings.Count(body, "\n")+1+pane.Chrome()).Render(body)
}

func (m Model) selected() (gh.Commit, bool) {
	for _, c := range m.detail.Detail.Commits {
		if c.SHA == m.commit.sha {
			return c, true
		}
	}
	return gh.Commit{}, false
}

// Arming the diff is the caller's, because this also runs on a resize.
func (m *Model) moveCommit(delta int) {
	list := m.detail.Detail.Commits
	if len(list) == 0 {
		return
	}
	m.commit.cursor = min(max(m.commit.cursor+delta, 0), len(list)-1)

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
