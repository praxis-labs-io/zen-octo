// Package list is the pull request list screen: one pane, sections as tabs in
// its top border, and the row count in its bottom.
package list

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/config"
	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
	"github.com/zen-octo/zen-octo/internal/tui/keys"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// OpenMsg asks the root to open a pull request. The list does not know a detail
// screen exists.
type OpenMsg struct{ PR gh.PullRequest }

// SectionChangedMsg asks the root to load a different section.
type SectionChangedMsg struct{ Section config.Section }

// RefreshMsg asks the root to refetch the current section.
type RefreshMsg struct{}

// Model is the list screen.
type Model struct {
	theme    theme.Theme
	pane     comp.Pane
	view     viewport.Model
	spinner  spinner.Model
	sections []config.Section
	active   int

	prs     []gh.PullRequest
	cursor  int
	loading bool
	err     error
	focused bool
}

// New builds the list over the configured sections.
func New(th theme.Theme, sections []config.Section) Model {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	sp.Style = lipgloss.NewStyle().Foreground(th.Secondary)

	vp := viewport.New()
	vp.SoftWrap = false
	vp.FillHeight = true

	return Model{
		theme:    th,
		pane:     comp.NewPane(th),
		view:     vp,
		spinner:  sp,
		sections: sections,
		loading:  true,
		focused:  true,
	}
}

// Init starts the spinner.
func (m Model) Init() tea.Cmd { return m.spinner.Tick }

// Update handles the keys that belong to this screen and the spinner. Anything
// the root has to act on leaves as a command.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	k := keys.List

	switch {
	case key.Matches(msg, k.Down):
		m.moveCursor(1)
	case key.Matches(msg, k.Up):
		m.moveCursor(-1)
	case key.Matches(msg, k.Top):
		m.setCursor(0)
	case key.Matches(msg, k.Bottom):
		m.setCursor(len(m.prs) - 1)
	case key.Matches(msg, k.PageDown):
		m.moveCursor(m.view.Height())
	case key.Matches(msg, k.PageUp):
		m.moveCursor(-m.view.Height())
	case key.Matches(msg, k.HalfPageDown):
		m.moveCursor(m.view.Height() / 2)
	case key.Matches(msg, k.HalfPageUp):
		m.moveCursor(-m.view.Height() / 2)

	case key.Matches(msg, k.NextSection):
		return m.changeSection(1)
	case key.Matches(msg, k.PrevSection):
		return m.changeSection(-1)

	case key.Matches(msg, k.Open):
		pr, ok := m.Selected()
		if !ok {
			return m, nil
		}
		return m, func() tea.Msg { return OpenMsg{PR: pr} }

	case key.Matches(msg, k.Refresh):
		if m.loading {
			return m, nil
		}
		m.loading = true
		// Clearing the error matters: the body checks it before loading, so a
		// retry would otherwise redraw the old failure with no spinner.
		m.err = nil
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg { return RefreshMsg{} })
	}

	return m, nil
}

func (m Model) changeSection(delta int) (Model, tea.Cmd) {
	if len(m.sections) < 2 {
		return m, nil
	}

	m.active = (m.active + delta + len(m.sections)) % len(m.sections)
	m.cursor, m.prs, m.err, m.loading = 0, nil, nil, true
	m.syncContent()

	section := m.sections[m.active]
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg { return SectionChangedMsg{Section: section} })
}

func (m *Model) moveCursor(delta int) { m.setCursor(m.cursor + delta) }

func (m *Model) setCursor(i int) {
	if len(m.prs) == 0 {
		m.cursor = 0
		return
	}

	m.cursor = max(0, min(i, len(m.prs)-1))
	// Selection is painted into the rows themselves, so a move means a redraw
	// before the viewport can scroll to it.
	m.syncContent()
	m.scrollToCursor()
}

// scrollToCursor brings the selected row into view, moving the window by the
// least it can.
//
// viewport.EnsureVisible is the obvious call and it is wrong here: it acts only
// once the line is already outside the window and then puts it on the top row,
// so one press down scrolls a whole page and the next nine move nothing.
func (m *Model) scrollToCursor() {
	height := m.view.Height()
	if height <= 0 {
		return
	}

	switch offset := m.view.YOffset(); {
	case m.cursor < offset:
		m.view.SetYOffset(m.cursor)
	case m.cursor >= offset+height:
		m.view.SetYOffset(m.cursor - height + 1)
	}
}

// SetSize tells the pane its outer size and the viewport what is left inside.
// Nothing here derives a height from a count of chrome lines.
func (m *Model) SetSize(width, height int) {
	m.pane = m.pane.Size(width, height)
	m.view.SetWidth(m.pane.InnerWidth())
	m.view.SetHeight(m.pane.InnerHeight())
	m.syncContent()
	// A shrink can leave the selection below the fold, where the next enter
	// opens a pull request the user cannot see.
	m.scrollToCursor()
}

// Focus colors the pane border and is set by the root.
func (m *Model) Focus(v bool) { m.focused = v }

// SetPullRequests replaces the rows, holding the selection on the same pull
// request where it survived the refresh.
func (m *Model) SetPullRequests(prs []gh.PullRequest) {
	m.restoreCursor(prs)
	m.prs = prs
	m.loading = false
	m.err = nil
	m.syncContent()
	m.scrollToCursor()
}

// SetError puts the screen into its failed state.
func (m *Model) SetError(err error) {
	m.err = err
	m.loading = false
}

// Selected reports the pull request under the cursor.
func (m Model) Selected() (gh.PullRequest, bool) {
	if m.cursor < 0 || m.cursor >= len(m.prs) {
		return gh.PullRequest{}, false
	}
	return m.prs[m.cursor], true
}

// Section is the section currently on screen.
func (m Model) Section() config.Section {
	if m.active >= len(m.sections) {
		return config.Section{}
	}
	return m.sections[m.active]
}

// restoreCursor keeps the selection on the same pull request across a refresh,
// falling back to the nearest valid row when it has gone.
func (m *Model) restoreCursor(next []gh.PullRequest) {
	if len(next) == 0 {
		m.cursor = 0
		return
	}
	if m.cursor < len(m.prs) {
		want := m.prs[m.cursor].ID
		for i, pr := range next {
			if pr.ID == want {
				m.cursor = i
				return
			}
		}
	}
	m.cursor = min(m.cursor, len(next)-1)
}

// syncContent re-renders the rows into the viewport. Row appearance depends on
// the cursor, so this runs after any move as well as after new data.
func (m *Model) syncContent() {
	width := m.pane.InnerWidth()
	if width <= 0 {
		return
	}

	rows := make([]string, len(m.prs))
	for i, pr := range m.prs {
		rows[i] = renderRow(m.theme, pr, width, i == m.cursor)
	}
	m.view.SetContent(strings.Join(rows, "\n"))
}

// View renders the screen.
func (m Model) View() string {
	tabs := make([]comp.Tab, len(m.sections))
	for i, s := range m.sections {
		tabs[i] = comp.Tab{Label: s.Title}
	}
	// Only the section on screen has been fetched, so it is the only one whose
	// count is known. A blank badge says that; a zero would lie.
	if m.active < len(tabs) && !m.loading && m.err == nil {
		tabs[m.active].Badge = strconv.Itoa(len(m.prs))
	}

	return m.pane.
		Tabs(tabs, m.active).
		Footer(m.footer()).
		Focus(m.focused).
		Render(m.body())
}

func (m Model) body() string {
	faint := lipgloss.NewStyle().Foreground(m.theme.Faint)

	switch {
	case m.err != nil:
		label := lipgloss.NewStyle().Foreground(m.theme.Error).Bold(true).Render("Failed to load")
		// Scope errors carry a multi-line fix; keep the newlines the error wrote.
		return label + "\n" + faint.Render(m.err.Error())
	case m.loading:
		return m.spinner.View() + " " + faint.Render("Loading pull requests")
	case len(m.prs) == 0:
		return faint.Render("Nothing matches this section.")
	default:
		return m.view.View()
	}
}

func (m Model) footer() string {
	if m.loading || m.err != nil || len(m.prs) == 0 {
		return ""
	}
	return strconv.Itoa(m.cursor+1) + " of " + strconv.Itoa(len(m.prs))
}

// Keys is the keymap live while this screen has focus.
func (m Model) Keys() keys.ListMap { return keys.List }
