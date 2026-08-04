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

// railWidth is fixed: the rail holds labels and short values, and a rail that
// grows with the frame just moves the conversation around.
const railWidth = 24

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
	view  viewport.Model

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

// New builds the screen over one pull request.
func New(th theme.Theme, pr gh.PullRequest) Model {
	vp := viewport.New()
	vp.SoftWrap = true
	vp.FillHeight = true

	return Model{
		theme: th,
		main:  comp.NewPane(th),
		rail:  comp.NewPane(th),
		view:  vp,
		pr:    pr,
	}
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

	case key.Matches(keyMsg, k.NextPane), key.Matches(keyMsg, k.PrevPane):
		// Two panes, so next and previous are the same flip.
		if m.railVisible() {
			if m.focus == paneMain {
				m.focus = paneRail
			} else {
				m.focus = paneMain
			}
		}
	case key.Matches(keyMsg, k.FocusMain):
		m.focus = paneMain
	case key.Matches(keyMsg, k.FocusRail):
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
		m.view.ScrollDown(1)
	case key.Matches(keyMsg, k.Up):
		m.view.ScrollUp(1)
	case key.Matches(keyMsg, k.Top):
		m.view.GotoTop()
	case key.Matches(keyMsg, k.Bottom):
		m.view.GotoBottom()
	case key.Matches(keyMsg, k.PageDown):
		m.view.PageDown()
	case key.Matches(keyMsg, k.PageUp):
		m.view.PageUp()
	case key.Matches(keyMsg, k.HalfPageDown):
		m.view.HalfPageDown()
	case key.Matches(keyMsg, k.HalfPageUp):
		m.view.HalfPageUp()
	}

	return m, nil
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
	if m.main.InnerWidth() <= 0 {
		return
	}
	m.view.SetContent(m.tabBody())
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
		Footer(m.scrollFooter()).
		Focus(m.focus == paneMain).
		Render(m.view.View())

	if !m.railVisible() {
		return main
	}

	rail := m.rail.
		Index(railIndex).
		Title("Details").
		Focus(m.focus == paneRail).
		Render(railBody(m.theme, m.pr))

	return lipgloss.JoinHorizontal(lipgloss.Top, main, rail)
}

// scrollFooter reports position only when there is somewhere to scroll to. A
// counter on content that already fits is noise.
func (m Model) scrollFooter() string {
	total := m.view.TotalLineCount()
	if total <= m.view.Height() {
		return ""
	}
	return strconv.Itoa(min(m.view.YOffset()+m.view.Height(), total)) + "/" + strconv.Itoa(total)
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

	meta := lipgloss.NewStyle().Foreground(stateColor).Render(stateIcon+" "+stateLabel) +
		faint.Render(" · "+m.pr.Author.Login+" · "+m.pr.BaseRefName+" ← "+m.pr.HeadRefName) +
		faint.Render(" · +"+strconv.Itoa(m.pr.Additions)+" −"+strconv.Itoa(m.pr.Deletions))

	rule := lipgloss.NewStyle().Foreground(m.theme.BorderFaintOrSecondary()).
		Render(strings.Repeat("─", max(0, m.main.InnerWidth())))

	return strings.Join([]string{title, meta, rule, faint.Render("The description and timeline arrive with the full pull request query.")}, "\n")
}
