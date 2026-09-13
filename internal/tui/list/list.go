// Package list is the pull request list screen.
package list

import (
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/keys"
	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

type OpenMsg struct{ PR gh.PullRequest }

type CopyLinkMsg struct{ PR gh.PullRequest }

type BrowseMsg struct{ PR gh.PullRequest }

// RefreshMsg asks the root to refetch; which sections it covers is the root's call.
type RefreshMsg struct{}

type Model struct {
	theme   theme.Theme
	pane    comp.Pane
	view    viewport.Model
	spinner comp.Spinner

	sections []store.Section
	active   int
	// By pull request id, since a refresh can reorder a section nobody is looking at.
	cursors []string

	rows   rows
	cursor int

	search    comp.Search
	searching bool
}

// New builds a list with no sections; the root pushes them with SetSections.
func New(th theme.Theme) Model {
	vp := viewport.New()
	vp.SoftWrap = false
	vp.FillHeight = true

	return Model{
		theme:   th,
		pane:    comp.NewPane(th),
		view:    vp,
		spinner: comp.NewSpinner(th),
	}
}

func (m Model) Init() tea.Cmd { return m.spinner.Tick() }

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		cmd := m.spinner.Advance(msg, spinning(m.sections))
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	k := keys.List

	if m.searching {
		return m.searchKey(msg)
	}

	switch {
	case key.Matches(msg, k.NextSection):
		m.changeSection(1)
		return m, nil
	case key.Matches(msg, k.PrevSection):
		m.changeSection(-1)
		return m, nil
	case key.Matches(msg, k.Sync):
		return m, func() tea.Msg { return RefreshMsg{} }

	case m.searchOpen() && key.Matches(msg, k.ClearSearch):
		m.clearSearch()
		return m, nil
	}

	if !showsRows(m.activeSection()) {
		return m, nil
	}

	switch {
	case key.Matches(msg, k.Down):
		m.moveCursor(1)
	case key.Matches(msg, k.Up):
		m.moveCursor(-1)
	case key.Matches(msg, k.Top):
		m.setCursor(0)
	case key.Matches(msg, k.Bottom):
		m.setCursor(m.rows.len() - 1)
	case key.Matches(msg, k.PageDown):
		m.moveCursor(m.page())
	case key.Matches(msg, k.PageUp):
		m.moveCursor(-m.page())
	case key.Matches(msg, k.HalfPageDown):
		m.moveCursor(m.halfPage())
	case key.Matches(msg, k.HalfPageUp):
		m.moveCursor(-m.halfPage())

	case key.Matches(msg, k.Open):
		pr, ok := m.Selected()
		if !ok {
			return m, nil
		}
		return m, func() tea.Msg { return OpenMsg{PR: pr} }

	case key.Matches(msg, k.CopyLink):
		pr, ok := m.Selected()
		if !ok {
			return m, nil
		}
		return m, func() tea.Msg { return CopyLinkMsg{PR: pr} }

	case key.Matches(msg, k.Browse):
		pr, ok := m.Selected()
		if !ok {
			return m, nil
		}
		return m, func() tea.Msg { return BrowseMsg{PR: pr} }

	case key.Matches(msg, k.Search):
		m.startSearch()
	}

	return m, nil
}

func (m *Model) changeSection(delta int) {
	if len(m.sections) < 2 {
		return
	}

	m.cursors[m.active] = m.selectedID()
	m.active = (m.active + delta + len(m.sections)) % len(m.sections)
	m.rows = newRows(m.visible())

	m.view.SetYOffset(0)
	m.cursor = min(m.rowOf(m.cursors[m.active]), max(0, m.rows.len()-1))
	m.relayout()
}

func (m Model) selectedID() string {
	pr, ok := m.rows.pr(m.cursor)
	if !ok {
		return ""
	}
	return pr.ID
}

func (m Model) rowOf(id string) int {
	for n := range m.rows.len() {
		if pr, _ := m.rows.pr(n); pr.ID == id {
			return n
		}
	}
	return 0
}

func (m *Model) moveCursor(delta int) { m.setCursor(m.cursor + delta) }

func (m *Model) setCursor(i int) {
	if m.rows.len() == 0 {
		m.cursor = 0
		return
	}

	m.cursor = max(0, min(i, m.rows.len()-1))
	m.syncContent()
	m.scrollToCursor()
}

// Runs a row short across a group boundary rather than overshoot and skip rows.
func (m Model) page() int { return max(1, m.view.Height()/(rowLines+1)) }

func (m Model) halfPage() int { return max(1, m.page()/2) }

// Moves the offset by hand: viewport.EnsureVisible jumps a page, then stalls.
func (m *Model) scrollToCursor() {
	height := m.view.Height()
	if height <= 0 {
		return
	}

	first, last := m.rows.span(m.cursor)

	top := m.rows.top(m.cursor, height)

	switch offset := m.view.YOffset(); {
	case top < offset:
		m.view.SetYOffset(top)
	case last >= offset+height:
		m.view.SetYOffset(min(m.rows.align(last-height+1), first))
	}
}

func (m *Model) SetSize(width, height int) {
	m.pane = m.pane.Size(width, height)
	m.relayout()
}

func (m *Model) relayout() {
	m.view.SetWidth(m.pane.InnerWidth())
	m.view.SetHeight(m.bodyHeight())
	m.syncContent()
	m.scrollToCursor()
}

func (m Model) bodyHeight() int {
	return max(0, m.pane.InnerHeight()-m.searchLines())
}

// SetSections takes the store's snapshot of every section, keeping the cursor on the same pull request.
func (m *Model) SetSections(sections []store.Section) {
	if len(m.cursors) != len(sections) {
		m.cursors = make([]string, len(sections))
	}
	m.sections = sections

	next := newRows(m.visible())
	m.restoreCursor(next)
	same := m.rows.same(next)
	m.rows = next
	if same && !m.searchOpen() {
		return
	}
	m.relayout()
}

func (m Model) Selected() (gh.PullRequest, bool) { return m.rows.pr(m.cursor) }

func (m Model) Section() store.Section { return m.activeSection() }

// ActiveIndex is the position of the section on screen, which is how the store keys sections.
func (m Model) ActiveIndex() int { return m.active }

func (m Model) activeSection() store.Section {
	if m.active < 0 || m.active >= len(m.sections) {
		return store.Section{}
	}
	return m.sections[m.active]
}

func (m *Model) restoreCursor(next rows) {
	if next.len() == 0 {
		m.cursor = 0
		return
	}
	if want, ok := m.rows.pr(m.cursor); ok {
		for n := range next.len() {
			if pr, _ := next.pr(n); pr.ID == want.ID {
				m.cursor = n
				return
			}
		}
	}
	m.cursor = min(m.cursor, next.len()-1)
}

func (m *Model) syncContent() {
	width := m.pane.InnerWidth()
	if width <= 0 {
		return
	}

	selected := m.rows.item(m.cursor)
	lines := make([]string, 0, m.rows.total)
	for i, it := range m.rows.items {
		if !it.isPR() {
			lines = append(lines, renderHeader(m.theme, it, width)...)
			continue
		}
		lines = append(lines, renderRow(m.theme, it, width, i == selected)...)
	}
	for range m.rows.pad(m.view.Height()) {
		lines = append(lines, strings.Repeat(" ", width))
	}
	m.view.SetContent(strings.Join(lines, "\n"))
}

func (m Model) View() string {
	tabs := make([]comp.Tab, len(m.sections))
	for i, s := range m.sections {
		tabs[i] = comp.Tab{Label: s.Title, Badge: badge(s)}
	}

	return m.pane.
		Tabs(tabs, m.active).
		Footer(m.footer()).
		Render(m.body())
}

// A section that never answered gets no count, since a zero would claim it is empty.
func badge(s store.Section) string {
	switch {
	case s.Status == store.StatusFailed:
		return "!"
	case s.Loaded:
		return "(" + strconv.Itoa(len(s.PRs)) + ")"
	}
	return ""
}

func (m Model) body() string {
	if !m.searchOpen() {
		return m.rowsBody()
	}
	return m.searchBox(m.pane.InnerWidth()) + "\n" + m.rowsBody()
}

func (m Model) rowsBody() string {
	faint := lipgloss.NewStyle().Foreground(m.theme.Subtle)
	section := m.activeSection()

	var block string
	switch {
	case section.Status == store.StatusFailed:
		label := lipgloss.NewStyle().Foreground(m.theme.Error).Bold(true).Render("Failed to load")
		block = label + "\n" + faint.Render(section.Err.Error())
	case !section.Loaded:
		block = m.spinner.Render("Loading pull requests")
	case m.rows.len() == 0 && !m.search.Empty():
		block = faint.Render("Nothing in this section matches that search.")
	case m.rows.len() == 0:
		block = faint.Render("Nothing matches this section.")
	default:
		return m.view.View()
	}
	return comp.Centered(block, m.pane.InnerWidth(), m.bodyHeight())
}

func showsRows(s store.Section) bool {
	return s.Loaded && s.Status != store.StatusFailed
}

func spinning(sections []store.Section) bool {
	return slices.ContainsFunc(sections, func(s store.Section) bool {
		return !s.Loaded && s.Status != store.StatusFailed
	})
}

func (m Model) footer() string {
	if !showsRows(m.activeSection()) || m.rows.len() == 0 {
		return ""
	}
	return strconv.Itoa(m.cursor+1) + " of " + strconv.Itoa(m.rows.len())
}

func (m Model) Keys() keys.ListMap { return keys.List }

// Capturing reports whether the search bar has the keyboard, so the root lets letters through.
func (m Model) Capturing() bool { return m.searching }

func (m Model) ShortHelp() []key.Binding {
	if m.searching {
		return keys.List.SearchHelp()
	}
	return keys.List.ShortHelp(keys.ListContext{
		Rows:   showsRows(m.activeSection()),
		Search: m.searchOpen(),
	})
}
