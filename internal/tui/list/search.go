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

// haystack is everything about a pull request a reader would type looking for
// it, as one string. Matching the fields separately would refuse "acme/api"
// against a repository and a number the eye reads as one line, and the query is
// one substring: what it spans is the whole row rather than a column of it.
//
// The punctuation is the row's own. A number is written "#1204" on screen, so
// that is what a reader types, and a handle carries its "@".
func haystack(pr gh.PullRequest) string {
	parts := []string{"#" + strconv.Itoa(pr.Number), pr.Repository, pr.Title, pr.HeadRefName}
	if pr.Author.Login != "" {
		parts = append(parts, "@"+pr.Author.Login)
	}
	return strings.Join(parts, " ")
}

// filter narrows the section to what the query matches. An empty query is not a
// filter that matches nothing: it is no filter, and the section comes back whole.
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

// visible is the rows the section offers under the query. Both places that
// build rows go through it, so nothing can render the section unfiltered.
func (m Model) visible() []gh.PullRequest {
	return filter(m.activeSection().PRs, m.search)
}

// searchOpen is whether the bar is drawn: while it has the keyboard, and after
// it has given it back with a query still standing. A filter nothing on the
// screen accounts for is a list that looks like it lost rows.
func (m Model) searchOpen() bool { return m.searching || !m.search.Empty() }

// headerRow is the bar, or nothing when there is no filter and none is being
// typed. The pane reads it for its own height, so it is set in Update and never
// while drawing.
func (m Model) headerRow() string {
	if !m.searchOpen() {
		return ""
	}
	return m.searchBar(m.pane.InnerWidth())
}

// searchBar is the query on the left and what it left out on the right. The
// caret is drawn rather than a real cursor: nobody edits the middle of a search,
// and a blinking one costs a command plumbed through two packages.
func (m Model) searchBar(width int) string {
	faint := lipgloss.NewStyle().Foreground(m.theme.Subtle)

	caret := ""
	if m.searching {
		caret = lipgloss.NewStyle().Foreground(m.theme.Accent).Render("▏")
	}
	lead := " " + faint.Render("Search: ") + m.search.Query() + caret

	right := ""
	if !m.search.Empty() {
		right = faint.Render(strconv.Itoa(m.rows.len()) +
			" of " + strconv.Itoa(len(m.activeSection().PRs)) + " ")
	}

	room := max(0, width-lipgloss.Width(right))
	if lipgloss.Width(lead) > room {
		lead = paint.Clip(lead, room, faint)
	}
	gap := max(0, width-lipgloss.Width(lead)-lipgloss.Width(right))
	return lead + strings.Repeat(" ", gap) + right
}

// searchKey is the bar's own keyboard. It runs ahead of every binding on this
// screen: "]" and "s" are characters in a search, and a key that both types and
// changes tab is a key that does the wrong one of the two.
func (m Model) searchKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.clearSearch()
		return m, nil
	case "enter":
		// The filter stands and the bar stays drawn. What is handed back is the
		// keyboard, so j and k walk what the search left.
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

// refilter rebuilds the rows under the query. The window belongs to the list
// the query just replaced, so it opens at the top and scrolls to the cursor,
// which restoreCursor has already kept on the same pull request wherever the
// filter left it there.
func (m *Model) refilter() {
	next := newRows(m.visible())
	m.restoreCursor(next)
	m.rows = next
	m.view.SetYOffset(0)
	m.relayout()
}
