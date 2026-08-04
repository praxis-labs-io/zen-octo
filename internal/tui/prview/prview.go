// Package prview is the pull request detail screen: a conversation pane with
// its own tab strip, and a details rail beside it that collapses when the frame
// gets narrow.
package prview

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
	"github.com/zen-octo/zen-octo/internal/tui/keys"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// BackMsg asks the root to return to the list.
type BackMsg struct{}

// RailPreference is what the user last asked of the details rail. The root
// carries it from one pull request to the next, so hiding the rail stays hidden
// instead of having to be redone on every open.
type RailPreference struct {
	On  bool
	Set bool
}

// railWidth is fixed: a rail that grows with the frame just moves the
// conversation around. It is wide enough for a branch name, which is the
// longest thing it carries.
const railWidth = 34

// railMinFrame is the width below which the rail hides itself. Under it the
// conversation drops past the point where a diff inside a review comment reads.
const railMinFrame = 120

// railMinForced is the floor even when the user asks for the rail by hand. A
// conversation narrower than this is not worth the trade.
const railMinForced = railWidth + 40

type pane int

const (
	paneMain pane = iota
	paneRail
)

// tabs on the detail screen. Only the conversation has content until the full
// pull request query lands.
var tabs = []comp.Tab{
	{Label: "Conversation"},
	{Label: "Commits"},
	{Label: "Checks"},
	{Label: "Files"},
}

// Model is the detail screen.
type Model struct {
	theme theme.Theme
	main  comp.Pane
	rail  comp.Pane

	// Each pane scrolls on its own. The rail overflows a short frame as readily
	// as the conversation does, and its branch names are the only place some of
	// them appear.
	view     viewport.Model
	railView viewport.Model

	pr    gh.PullRequest
	tab   int
	focus pane

	// railOn is what the user last asked for, and railUserSet whether they have
	// asked at all. Until they do, width decides.
	railOn      bool
	railUserSet bool

	width  int
	height int
}

// New builds the screen over one pull request, carrying forward whatever the
// user last asked of the rail.
func New(th theme.Theme, pr gh.PullRequest, rail RailPreference) Model {
	return Model{
		theme:       th,
		main:        comp.NewPane(th),
		rail:        comp.NewPane(th),
		view:        newViewport(),
		railView:    newViewport(),
		pr:          pr,
		railOn:      rail.On,
		railUserSet: rail.Set,
	}
}

func newViewport() viewport.Model {
	vp := viewport.New()
	vp.SoftWrap = true
	vp.FillHeight = true
	return vp
}

// Update handles the keys that belong to this screen.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	k := keys.Detail

	switch {
	case key.Matches(keyMsg, k.Back):
		return m, func() tea.Msg { return BackMsg{} }

	case key.Matches(keyMsg, k.NextTab):
		m.tab = (m.tab + 1) % len(tabs)
		m.syncContent()
	case key.Matches(keyMsg, k.PrevTab):
		m.tab = (m.tab - 1 + len(tabs)) % len(tabs)
		m.syncContent()

	case key.Matches(keyMsg, k.PaneLeft), key.Matches(keyMsg, k.FocusMain):
		m.focus = paneMain
	case key.Matches(keyMsg, k.PaneRight), key.Matches(keyMsg, k.FocusRail):
		if m.railVisible() {
			m.focus = paneRail
		}

	case key.Matches(keyMsg, k.ToggleRail):
		m.railOn, m.railUserSet = !m.railVisible(), true
		// Focus cannot stay on a rail that just went away.
		if !m.railVisible() {
			m.focus = paneMain
		}
		m.layout()

	case key.Matches(keyMsg, k.Down):
		m.scroll().ScrollDown(1)
	case key.Matches(keyMsg, k.Up):
		m.scroll().ScrollUp(1)
	case key.Matches(keyMsg, k.Top):
		m.scroll().GotoTop()
	case key.Matches(keyMsg, k.Bottom):
		m.scroll().GotoBottom()
	case key.Matches(keyMsg, k.PageDown):
		m.scroll().PageDown()
	case key.Matches(keyMsg, k.PageUp):
		m.scroll().PageUp()
	case key.Matches(keyMsg, k.HalfPageDown):
		m.scroll().HalfPageDown()
	case key.Matches(keyMsg, k.HalfPageUp):
		m.scroll().HalfPageUp()
	}

	return m, nil
}

// scroll is the viewport the movement keys drive. Focus decides, which is what
// the help text promises and what the pane borders show.
func (m *Model) scroll() *viewport.Model {
	if m.focus == paneRail {
		return &m.railView
	}
	return &m.view
}

// Rail is the preference to hand the next screen.
func (m Model) Rail() RailPreference {
	return RailPreference{On: m.railOn, Set: m.railUserSet}
}

// SetSize takes the frame and divides it between the panes.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = width, height
	m.layout()
}

// layout sizes the panes for the current frame and rail state.
func (m *Model) layout() {
	mainWidth := m.width
	if m.railVisible() {
		mainWidth = m.width - railWidth
		m.rail = m.rail.Size(railWidth, m.height)
		m.railView.SetWidth(m.rail.InnerWidth())
		m.railView.SetHeight(m.rail.InnerHeight())
	}

	m.main = m.main.Size(mainWidth, m.height)
	m.view.SetWidth(m.main.InnerWidth())
	m.view.SetHeight(m.main.InnerHeight())
	m.syncContent()
}

// railVisible decides whether the rail is on screen. Width decides until the
// user overrides it, and even then the conversation keeps a floor.
func (m Model) railVisible() bool {
	if m.railUserSet {
		return m.railOn && m.width >= railMinForced
	}
	return m.width >= railMinFrame
}

// PullRequest is what the screen is showing.
func (m Model) PullRequest() gh.PullRequest { return m.pr }

// Keys is the keymap live while this screen is up.
func (m Model) Keys() keys.DetailMap { return keys.Detail }

func (m *Model) syncContent() {
	if m.main.InnerWidth() > 0 {
		m.view.SetContent(m.tabBody())
	}
	if m.rail.InnerWidth() > 0 {
		m.railView.SetContent(railBody(m.theme, m.pr))
	}
}

// View renders the screen. The panes carry a bracketed index only when there
// are two of them, because a lone pane numbered [1] is just noise.
func (m Model) View() string {
	mainIndex, railIndex := 0, 0
	if m.railVisible() {
		mainIndex, railIndex = 1, 2
	}

	main := m.main.
		Index(mainIndex).
		Tabs(tabs, m.tab).
		Footer(scrollFooter(m.view)).
		Focus(m.focus == paneMain).
		Render(m.view.View())

	if !m.railVisible() {
		return main
	}

	rail := m.rail.
		Index(railIndex).
		Title("Details").
		Footer(scrollFooter(m.railView)).
		Focus(m.focus == paneRail).
		Render(m.railView.View())

	return lipgloss.JoinHorizontal(lipgloss.Top, main, rail)
}

// scrollFooter reports position only when there is somewhere to scroll to. A
// counter on content that already fits is noise.
func scrollFooter(v viewport.Model) string {
	total := v.TotalLineCount()
	if total <= v.Height() {
		return ""
	}
	return strconv.Itoa(min(v.YOffset()+v.Height(), total)) + "/" + strconv.Itoa(total)
}

// tabBody renders whichever tab is current.
func (m Model) tabBody() string {
	if m.tab != 0 {
		return lipgloss.NewStyle().Foreground(m.theme.Faint).
			Render(tabs[m.tab].Label + " arrive with the full pull request query.")
	}
	return m.conversation()
}

// conversation is the header block every GitHub PR page leads with, followed by
// the body. The description and timeline land with the full query.
func (m Model) conversation() string {
	title := lipgloss.NewStyle().Foreground(m.theme.Primary).Bold(true).
		Render(m.pr.Title + " #" + strconv.Itoa(m.pr.Number))

	stateLabel, stateColor := comp.PRStateLabel(m.theme, m.pr)
	stateIcon, _ := comp.PRStateIcon(m.theme, m.pr)
	faint := lipgloss.NewStyle().Foreground(m.theme.Faint)

	// A deleted account has no login, and joining round it beats printing two
	// separators with nothing between them.
	parts := []string{lipgloss.NewStyle().Foreground(stateColor).Render(stateIcon + " " + stateLabel)}
	if m.pr.Author.Login != "" {
		parts = append(parts, faint.Render(m.pr.Author.Login))
	}
	parts = append(parts,
		faint.Render(m.pr.BaseRefName+" ← "+m.pr.HeadRefName),
		faint.Render("+"+strconv.Itoa(m.pr.Additions)+" −"+strconv.Itoa(m.pr.Deletions)),
	)
	meta := strings.Join(parts, faint.Render(" · "))

	rule := lipgloss.NewStyle().Foreground(m.theme.BorderFaintOrSecondary()).
		Render(strings.Repeat("─", max(0, m.main.InnerWidth())))

	lines := []string{title, meta}
	if status := m.collapsedStatus(); status != "" {
		lines = append(lines, status)
	}
	lines = append(lines, rule, faint.Render("The description and timeline arrive with the full pull request query."))
	return strings.Join(lines, "\n")
}

// collapsedStatus carries the two things the rail holds that the meta line does
// not, so hiding the rail loses nothing. Everything else in the rail is already
// on the line above it.
func (m Model) collapsedStatus() string {
	if m.railVisible() {
		return ""
	}

	var parts []string
	if label, c := comp.CheckStateLabel(m.theme, m.pr.Checks); label != "" {
		glyph, _ := comp.CheckStateIcon(m.theme, m.pr.Checks)
		parts = append(parts, lipgloss.NewStyle().Foreground(c).Render(glyph+" "+label))
	}
	if label, c := comp.ReviewLabel(m.theme, m.pr.ReviewDecision); label != "" {
		parts = append(parts, lipgloss.NewStyle().Foreground(c).Render(label))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, lipgloss.NewStyle().Foreground(m.theme.Faint).Render(" · "))
}
