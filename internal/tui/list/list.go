// Package list is the pull request list screen: one pane, sections as tabs in
// its top border, and the row count in its bottom.
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

// OpenMsg asks the root to open a pull request. The list does not know a detail
// screen exists.
type OpenMsg struct{ PR gh.PullRequest }

// CopyLinkMsg asks the root to put a pull request's URL on the clipboard.
type CopyLinkMsg struct{ PR gh.PullRequest }

// BrowseMsg asks the root to open a pull request in a browser.
type BrowseMsg struct{ PR gh.PullRequest }

// RefreshMsg asks the root to refetch. The store holds every section, so what a
// refresh covers is the root's call; the list only reports the key.
type RefreshMsg struct{}

// Model is the list screen.
type Model struct {
	theme   theme.Theme
	pane    comp.Pane
	view    viewport.Model
	spinner comp.Spinner

	sections []store.Section
	active   int
	// cursors is the pull request the selection sat on in each section, by id.
	// Every section is fetched now, so a tab switch is a move rather than a
	// reload, and coming back to row zero would throw the user's place away. A
	// row index would not survive: a refresh reorders a section nobody is
	// looking at, and the index then names a different pull request.
	cursors []string

	rows   rows
	cursor int // indexes the selectable rows, so a header is never addressable
}

// New builds the list. It starts with no sections: the store is the one source
// of what they are, and the root pushes a snapshot before the first frame.
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

// Init starts the spinner.
func (m Model) Init() tea.Cmd { return m.spinner.Tick() }

// Update handles the keys that belong to this screen and the spinner. Anything
// the root has to act on leaves as a command.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		// Any section, not just the one on screen: gating on the active one
		// kills the chain, and a switch onto a slower tab finds it frozen.
		cmd := m.spinner.Advance(msg, spinning(m.sections))
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	k := keys.List

	switch {
	case key.Matches(msg, k.NextSection):
		m.changeSection(1)
		return m, nil
	case key.Matches(msg, k.PrevSection):
		m.changeSection(-1)
		return m, nil
	case key.Matches(msg, k.Sync):
		return m, func() tea.Msg { return RefreshMsg{} }
	}

	// A reload keeps its rows up, so taking the keyboard for the length of one
	// would lock the screen the fetch is refreshing.
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
	}

	return m, nil
}

// changeSection moves to another tab. Nothing is fetched: the store already
// holds every section, which is what makes the switch instant.
func (m *Model) changeSection(delta int) {
	if len(m.sections) < 2 {
		return
	}

	m.cursors[m.active] = m.selectedID()
	m.active = (m.active + delta + len(m.sections)) % len(m.sections)
	m.rows = newRows(m.activeSection().PRs)

	// The window belongs to the section being left. Opening the new one at the
	// top and scrolling to the cursor lands somewhere valid whatever the two
	// sections' lengths are.
	m.view.SetYOffset(0)
	m.setCursor(m.rowOf(m.cursors[m.active]))
}

func (m Model) selectedID() string {
	pr, ok := m.rows.pr(m.cursor)
	if !ok {
		return ""
	}
	return pr.ID
}

// rowOf is the row holding a pull request, falling back to the first when it
// has gone: a section can lose the row the cursor was parked on while the user
// is looking at another tab.
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
	// Selection is painted into the rows themselves, so a move means a redraw
	// before the viewport can scroll to it.
	m.syncContent()
	m.scrollToCursor()
}

// page is a screenful measured in rows. A row carries its rule with it, so this
// runs a row short around a group boundary, which beats overshooting: a page
// key that skips rows loses work off the top of the screen.
func (m Model) page() int { return max(1, m.view.Height()/(rowLines+1)) }

// halfPage floors at a row for the same reason page does. A pane short enough
// to make half a page nothing leaves the key looking broken.
func (m Model) halfPage() int { return max(1, m.page()/2) }

// scrollToCursor brings the selected row into view, moving the window by the
// least it can. It works in lines: a row is two of them and a header is one, so
// the cursor is not an offset into the viewport.
//
// viewport.EnsureVisible is the obvious call and it is wrong here: it acts only
// once the line is already outside the window and then puts it on the top row,
// so one press down scrolls a whole page and the next nine move nothing.
func (m *Model) scrollToCursor() {
	height := m.view.Height()
	if height <= 0 {
		return
	}

	first, last := m.rows.span(m.cursor)

	// Scrolling up brings the row's header with it. Anchoring on the row alone
	// left the header stranded above the window with nothing able to scroll back
	// to it.
	top := m.rows.top(m.cursor, height)

	switch offset := m.view.YOffset(); {
	case top < offset:
		m.view.SetYOffset(top)
	case last >= offset+height:
		// A window shorter than a row can only hold one of its lines, and the
		// first is the one carrying the title.
		m.view.SetYOffset(min(m.rows.align(last-height+1), first))
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

// SetSections takes the store's snapshot. It is the only way data reaches this
// screen: tabs, counts, rows, the spinner, and the error state all come from it.
//
// Rows rebuild on every snapshot, including one another section triggered.
// restoreCursor matches on the pull request id, so an arrival elsewhere moves
// nothing under the selection.
func (m *Model) SetSections(sections []store.Section) {
	if len(m.cursors) != len(sections) {
		m.cursors = make([]string, len(sections))
	}
	m.sections = sections

	next := newRows(m.activeSection().PRs)
	m.restoreCursor(next)
	same := m.rows.same(next)
	m.rows = next
	// Grouping is cheap and rendering every row is not, so a snapshot another
	// section triggered pays for the compare rather than the repaint.
	if same {
		return
	}
	m.syncContent()
	m.scrollToCursor()
}

// Selected reports the pull request under the cursor.
func (m Model) Selected() (gh.PullRequest, bool) { return m.rows.pr(m.cursor) }

// Section is the section currently on screen.
func (m Model) Section() store.Section { return m.activeSection() }

// ActiveIndex is where that section sits. The store keys sections by position,
// so refetching the one on screen takes the number rather than the value.
func (m Model) ActiveIndex() int { return m.active }

func (m Model) activeSection() store.Section {
	if m.active < 0 || m.active >= len(m.sections) {
		return store.Section{}
	}
	return m.sections[m.active]
}

// restoreCursor keeps the selection on the same pull request across a refresh,
// falling back to the nearest valid row when it has gone. Grouping means a
// refresh can reorder rows without adding or removing any, so matching on the
// id is the only thing that holds.
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

// syncContent re-renders the list into the viewport. Row appearance depends on
// the cursor, so this runs after any move as well as after new data.
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
	// The blank lines under the last row are what keep the viewport's own clamp
	// on an item boundary. See rows.pad.
	for range m.rows.pad(m.view.Height()) {
		lines = append(lines, strings.Repeat(" ", width))
	}
	m.view.SetContent(strings.Join(lines, "\n"))
}

// View renders the screen.
func (m Model) View() string {
	tabs := make([]comp.Tab, len(m.sections))
	for i, s := range m.sections {
		tabs[i] = comp.Tab{Label: s.Title, Badge: badge(s)}
	}

	// No Focus call: this screen is one pane, so a focused border would be
	// telling the user something they cannot act on.
	return m.pane.
		Tabs(tabs, m.active).
		Footer(m.footer()).
		Render(m.body())
}

// badge is the count a tab carries. A section that has never answered gets
// nothing, because a zero would claim it is empty; one that has keeps its count
// through a reload, rather than blanking the whole strip for the fetch. A
// failure takes the badge, because that is the news on that tab.
//
// A count is bracketed and the failure mark is not: parentheses read as holding
// a quantity, and "(!)" reads like one that came back strange.
func badge(s store.Section) string {
	switch {
	case s.Status == store.StatusFailed:
		return "!"
	case s.Loaded:
		return "(" + strconv.Itoa(len(s.PRs)) + ")"
	}
	return ""
}

// body is the rows once they are there, and a single block saying why they are
// not when they aren't. That block is centred: it is the only thing in the
// pane, and a sentence in the top-left corner of an empty frame reads as the
// first row of a list still filling in.
func (m Model) body() string {
	faint := lipgloss.NewStyle().Foreground(m.theme.Subtle)
	section := m.activeSection()

	var block string
	switch {
	case section.Status == store.StatusFailed:
		label := lipgloss.NewStyle().Foreground(m.theme.Error).Bold(true).Render("Failed to load")
		// Scope errors carry a multi-line fix; keep the newlines the error wrote.
		block = label + "\n" + faint.Render(section.Err.Error())
	case !section.Loaded:
		block = m.spinner.Render("Loading pull requests")
	case m.rows.len() == 0:
		block = faint.Render("Nothing matches this section.")
	default:
		return m.view.View()
	}
	return comp.Centered(block, m.pane.InnerWidth(), m.pane.InnerHeight())
}

// showsRows is whether the pane holds the section's rows rather than a block
// standing in for them. A section that has answered keeps them through a reload.
func showsRows(s store.Section) bool {
	return s.Loaded && s.Status != store.StatusFailed
}

// spinning is whether any section would draw the pane's spinner, which is the
// only thing the chain feeds. A reload keeps its rows and draws none.
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

// Keys is the keymap live while this screen has focus.
func (m Model) Keys() keys.ListMap { return keys.List }
