package list

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/paint"
)

// One string in the row's own punctuation, so a query can span repository, #number, and @handle.
func haystack(pr gh.PullRequest) string {
	parts := []string{"#" + strconv.Itoa(pr.Number), pr.Repository, pr.Title, pr.HeadRefName}
	if pr.Author.Login != "" {
		parts = append(parts, "@"+pr.Author.Login)
	}
	return strings.Join(parts, " ")
}

func filter(prs []gh.PullRequest, s comp.Search) []gh.PullRequest {
	if s.Empty() {
		return prs
	}
	out := make([]gh.PullRequest, 0, len(prs))
	for _, pr := range prs {
		if s.Matches(haystack(pr)) {
			out = append(out, pr)
		}
	}
	return out
}

func (m Model) visible() []gh.PullRequest {
	return filter(m.activeSection().PRs, m.search)
}

func (m Model) searchOpen() bool { return m.searching || !m.search.Empty() }

const (
	searchBoxLines = 3

	searchBoxIndent = 1
	searchBoxLead   = 1 + searchBoxIndent + 1 + 1

	searchPrompt = "/ "
)

func (m Model) searchLines() int {
	if !m.searchOpen() {
		return 0
	}
	return searchBoxLines
}

func (m Model) searchRow(inner int) (lead, right string, room int) {
	prompt := m.theme.Subtle
	if m.searching {
		prompt = m.theme.Accent
	}
	lead = lipgloss.NewStyle().Foreground(prompt).Render(searchPrompt)

	faint := lipgloss.NewStyle().Foreground(m.theme.Subtle)
	switch {
	case !m.search.Empty():
		lead += m.search.Query()
	case m.searching:
		lead += faint.Render("Search")
	}

	if !m.search.Empty() && showsRows(m.activeSection()) {
		right = faint.Render(strconv.Itoa(m.rows.len()) +
			" of " + strconv.Itoa(len(m.activeSection().PRs)))
	}
	return lead, right, max(0, inner-lipgloss.Width(right)-1)
}

func (m Model) searchInner() int {
	box := comp.NewPane(m.theme).Size(max(0, m.pane.InnerWidth()-2), searchBoxLines)
	return max(0, box.InnerWidth()-2)
}

func (m Model) searchBox(width int) string {
	box := comp.NewPane(m.theme).Focus(m.searching).Size(max(0, width-2), searchBoxLines)
	if box.InnerWidth() == 0 {
		return ""
	}

	inner := max(0, box.InnerWidth()-2)
	lead, right, room := m.searchRow(inner)

	faint := lipgloss.NewStyle().Foreground(m.theme.Subtle)
	if lipgloss.Width(lead) > room {
		lead = paint.Clip(lead, room, faint)
	}
	gap := max(0, inner-lipgloss.Width(lead)-lipgloss.Width(right))
	content := " " + lead + strings.Repeat(" ", gap) + right + " "

	indent := strings.Repeat(" ", searchBoxIndent)
	lines := strings.Split(box.Render(content), "\n")
	for i, line := range lines {
		lines[i] = indent + line + indent
	}
	return strings.Join(lines, "\n")
}

// Cursor is the terminal cursor in the search box, relative to the screen's frame, or nil unless the box has the keyboard.
func (m Model) Cursor() *tea.Cursor {
	if !m.searching {
		return nil
	}

	inner := m.searchInner()
	if inner <= 0 {
		return nil
	}
	_, _, room := m.searchRow(inner)

	typed := lipgloss.Width(searchPrompt) + lipgloss.Width(m.search.Query())
	return comp.Cursor(m.theme, searchBoxLead+min(typed, room), m.pane.Above()+1)
}

// Runs ahead of every binding, since ] and s are characters in a search.
func (m Model) searchKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.clearSearch()
		return m, nil
	case "enter":
		m.searching = false
		m.relayout()
		return m, nil
	}
	if !m.search.Insert(msg) {
		return m, nil
	}
	m.refilter()
	return m, nil
}

func (m *Model) startSearch() {
	m.searching = true
	m.relayout()
}

func (m *Model) clearSearch() {
	m.searching = false
	m.search = comp.Search{}
	m.refilter()
}

func (m *Model) refilter() {
	next := newRows(m.visible())
	m.restoreCursor(next)
	m.rows = next
	m.view.SetYOffset(0)
	m.relayout()
}
