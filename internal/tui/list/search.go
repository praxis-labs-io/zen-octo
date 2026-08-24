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

// searchOpen is whether the box is drawn: while it has the keyboard, and after
// it has given it back with a query still standing. A filter nothing on the
// screen accounts for is a list that looks like it lost rows.
func (m Model) searchOpen() bool { return m.searching || !m.search.Empty() }

// searchBoxLines is what the box costs the rows below it: its two borders and
// the one line it holds.
const searchBoxLines = 3

// searchLines is the height the box takes off the pane, which is none at all
// while there is nothing to search by.
func (m Model) searchLines() int {
	if !m.searchOpen() {
		return 0
	}
	return searchBoxLines
}

// searchBox is the query in a box of its own, inside the pane and above the
// rows. It is a comp.Pane rather than a bordered row built here: the modal is
// already a pane sized to its content, and the border a pane draws is the one
// the rest of this app draws.
//
// It sits a column in from each side. Flush against the pane's own border the
// two verticals meet, which reads as a frame that has come apart rather than as
// a box inside one.
//
// The border and the prompt both answer to whether it has the keyboard, which
// is the only thing on this screen that says where the keys are going: the list
// pane itself never takes focus, having nothing to hand it to.
func (m Model) searchBox(width int) string {
	box := comp.NewPane(m.theme).Focus(m.searching).Size(max(0, width-2), searchBoxLines)
	if box.InnerWidth() == 0 {
		return ""
	}

	prompt := m.theme.Subtle
	if m.searching {
		prompt = m.theme.Accent
	}
	lead := lipgloss.NewStyle().Foreground(prompt).Render("/ ")

	faint := lipgloss.NewStyle().Foreground(m.theme.Subtle)
	switch {
	case !m.search.Empty():
		lead += m.search.Query()
	case m.searching:
		// The prompt is a glyph rather than a word, so the box says what it is
		// for in the one state where nothing else on it does.
		lead += faint.Render("Search")
	}
	if m.searching {
		lead += lipgloss.NewStyle().Foreground(m.theme.Accent).Render("▏")
	}

	right := ""
	if !m.search.Empty() {
		right = faint.Render(strconv.Itoa(m.rows.len()) +
			" of " + strconv.Itoa(len(m.activeSection().PRs)))
	}

	// A column of padding inside the box, which is comp.Modal's own: a prompt
	// against the border reads as text that has run into it.
	inner := max(0, box.InnerWidth()-2)

	room := max(0, inner-lipgloss.Width(right)-1)
	if lipgloss.Width(lead) > room {
		lead = paint.Clip(lead, room, faint)
	}
	gap := max(0, inner-lipgloss.Width(lead)-lipgloss.Width(right))
	content := " " + lead + strings.Repeat(" ", gap) + right + " "

	// The box is indented rather than the pane padded: the rows under it have a
	// margin of their own and the pane holds no gutter for anyone.
	lines := strings.Split(box.Render(content), "\n")
	for i, line := range lines {
		lines[i] = " " + line + " "
	}
	return strings.Join(lines, "\n")
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
