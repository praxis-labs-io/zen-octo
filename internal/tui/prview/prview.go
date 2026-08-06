// Package prview is the pull request detail screen: a conversation pane with
// its own tab strip, and a details rail beside it that collapses when the frame
// gets narrow.
package prview

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/store"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
	"github.com/zen-octo/zen-octo/internal/tui/keys"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// BackMsg asks the root to return to the list.
type BackMsg struct{}

// NeedFilesMsg asks the root for a diff this screen does not hold. The screen
// cannot fetch, so opening the Files tab reaches the root as a message and the
// request starts there.
type NeedFilesMsg struct{ ID string }

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

// railGutter is the space between the rail's borders and what it holds, on both
// sides. Text against a border reads as a rendering fault rather than as a
// column.
const railGutter = 1

// contentMeasure caps the conversation and centres it. Text set the full width
// of a wide terminal is a paragraph the eye loses its place in on every line.
// The diff is exempt: code wants every column it can have.
const contentMeasure = 90

// treeWidth is the file column at its full width: enough for a name after two
// levels of nesting, which is where most of a repository sits. It is the one
// column that shrinks rather than holding its size, because it is the only
// navigation the tab has and hiding it costs more than narrowing it.
const treeWidth = 40

// treeMin is as narrow as the column goes before it hides instead.
const treeMin = 24

// treeMinFrame is the width below which the tree hides. Under it the diff is
// down to a gutter and a fragment, and the file headings in the diff itself
// still carry navigation.
const treeMinFrame = 70

// diffMeasure is the width the diff keeps before the tree gives any up. Below
// it a hunk stops reading as code.
const diffMeasure = 80

// contentLead is the blank line every pane opens with. The diff records where
// each file starts in its own body, and scrolling to one has to clear this.
const contentLead = 1

type pane int

const (
	paneTree pane = iota
	paneMain
	paneRail
)

// tabFiles is the diff. The tabs are a slice rather than an enum, so the one
// with a body of its own is named.
const tabFiles = 3

// tabs on the detail screen. Commits and Checks land with their own tickets.
var tabs = []comp.Tab{
	{Label: "Conversation"},
	{Label: "Commits"},
	{Label: "Checks"},
	{Label: "Files"},
}

// Model is the detail screen.
type Model struct {
	theme theme.Theme
	tree  comp.Pane
	main  comp.Pane
	rail  comp.Pane

	// Each pane scrolls on its own. The rail overflows a short frame as readily
	// as the conversation does, and its branch names are the only place some of
	// them appear.
	treeView viewport.Model
	view     viewport.Model
	railView viewport.Model

	md      comp.Markdown
	syntax  comp.Syntax
	spinner comp.Spinner

	pr     gh.PullRequest
	detail store.Detail
	files  store.Files
	tab    int
	focus  pane

	// rows is what the file tree has on screen, and cursor points into it. The
	// diff beside it is keyed by the same rows, so folding one folds both.
	rows      []row
	cursor    int
	collapsed map[string]bool

	// spans is where each file's block sits inside the diff body, in the order
	// the tree has them. The tree scrolls the diff by it and follows the diff
	// back by it.
	spans []fileSpan

	// expanded unfolds every <details> block on the screen at once. It is one
	// bool because the conversation has no cursor to hang a per-block state on;
	// that lands with the ticket that gives it one.
	expanded bool

	// offsets parks the scroll position of each tab. One viewport serves all
	// four, and without this switching to a short tab clamps the offset to zero
	// and switching back lands at the top of a conversation you were halfway
	// down.
	offsets []int

	// railOn is what the user last asked for, and railUserSet whether they have
	// asked at all. Until they do, width decides.
	railOn      bool
	railUserSet bool

	width  int
	height int
}

// New builds the screen over one pull request row, carrying forward whatever
// the user last asked of the rail. The row is what the list already had, so the
// header and the rail paint before the detail query answers.
func New(th theme.Theme, pr gh.PullRequest, rail RailPreference, syntax comp.Syntax) Model {
	return Model{
		theme:       th,
		tree:        comp.NewPane(th),
		main:        comp.NewPane(th),
		rail:        comp.NewPane(th),
		treeView:    newViewport(),
		view:        newViewport(),
		railView:    newViewport(),
		md:          comp.NewMarkdown(th),
		syntax:      syntax,
		spinner:     comp.NewSpinner(th),
		pr:          pr,
		focus:       paneMain,
		collapsed:   make(map[string]bool),
		offsets:     make([]int, len(tabs)),
		railOn:      rail.On,
		railUserSet: rail.Set,
	}
}

// SetFiles takes what the store holds for this pull request's diff. A cursor
// still at the top goes to the first file rather than the directory above it,
// which is what the diff beside it is already showing.
func (m *Model) SetFiles(f store.Files) {
	m.files = f
	m.syncRows()

	m.cursor = min(m.cursor, max(0, len(m.rows)-1))
	if m.cursor == 0 {
		m.cursor = m.firstFile()
	}
	m.syncContent()
}

func (m Model) firstFile() int {
	for i, r := range m.rows {
		if r.file != nil {
			return i
		}
	}
	return 0
}

// SetDetail takes what the store holds for this pull request. A detail that has
// loaded replaces the row, so the header and the rail stop showing the thinner
// version search returned.
func (m *Model) SetDetail(d store.Detail) {
	m.detail = d
	if d.Loaded {
		m.pr = d.Detail.PullRequest
	}
	m.syncContent()
}

func newViewport() viewport.Model {
	vp := viewport.New()
	vp.SoftWrap = true
	vp.FillHeight = true
	return vp
}

// Init starts the spinner, which runs until the conversation lands.
func (m Model) Init() tea.Cmd { return m.spinner.Tick() }

// Update handles the keys that belong to this screen, and the spinner.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		cmd := m.spinner.Advance(msg, m.waiting())
		// The body is built into the viewport rather than in View, so a new
		// frame of the glyph only reaches the screen through a resync.
		m.syncContent()
		return m, cmd
	}
	return m, nil
}

// waiting is the one state the spinner belongs in: nothing to read yet. A
// refetch behind content already on screen spins over nothing, and each tab
// waits on its own request.
func (m Model) waiting() bool {
	if m.tab == tabFiles {
		return !m.files.Loaded && m.files.Status == store.StatusLoading
	}
	return !m.detail.Loaded && m.detail.Status == store.StatusLoading
}

func (m Model) handleKey(keyMsg tea.KeyPressMsg) (Model, tea.Cmd) {
	k := keys.Detail
	before := m.view.YOffset()

	switch {
	case key.Matches(keyMsg, k.Back):
		return m, func() tea.Msg { return BackMsg{} }

	case key.Matches(keyMsg, k.NextTab):
		return m, m.changeTab(1)
	case key.Matches(keyMsg, k.PrevTab):
		return m, m.changeTab(-1)

	case key.Matches(keyMsg, k.PaneLeft):
		m.focusPane(m.focus - 1)
	case key.Matches(keyMsg, k.PaneRight):
		m.focusPane(m.focus + 1)
	case key.Matches(keyMsg, k.FocusPane):
		m.focusIndex(keyMsg.String())

	case key.Matches(keyMsg, k.Expand):
		if m.focus == paneTree {
			m.toggleFold()
			break
		}
		m.expanded = !m.expanded
		m.syncContent()

	case key.Matches(keyMsg, k.ToggleRail):
		m.railOn, m.railUserSet = !m.railVisible(), true
		// Focus cannot stay on a rail that just went away.
		if !m.railVisible() && m.focus == paneRail {
			m.focus = paneMain
		}
		m.layout()

	case key.Matches(keyMsg, k.Down):
		m.move(1)
	case key.Matches(keyMsg, k.Up):
		m.move(-1)
	case key.Matches(keyMsg, k.Top):
		if !m.jumped(-len(m.rows)) {
			m.scroll().GotoTop()
		}
	case key.Matches(keyMsg, k.Bottom):
		if !m.jumped(len(m.rows)) {
			m.scroll().GotoBottom()
		}
	case key.Matches(keyMsg, k.PageDown):
		if !m.jumped(m.treeView.Height()) {
			m.scroll().PageDown()
		}
	case key.Matches(keyMsg, k.PageUp):
		if !m.jumped(-m.treeView.Height()) {
			m.scroll().PageUp()
		}
	case key.Matches(keyMsg, k.HalfPageDown):
		if !m.jumped(m.treeView.Height() / 2) {
			m.scroll().HalfPageDown()
		}
	case key.Matches(keyMsg, k.HalfPageUp):
		if !m.jumped(-m.treeView.Height() / 2) {
			m.scroll().HalfPageUp()
		}
	}

	// Whatever the key did to the diff, the file column follows it. Gated on
	// the diff having actually moved: a key that only changes focus must not
	// pull the cursor off the row the reader left it on.
	if m.view.YOffset() != before {
		m.trackDiff()
	}
	return m, nil
}

// move is a row on the tree and a line everywhere else. The tree is the only
// pane with something to point at.
func (m *Model) move(delta int) {
	if m.focus == paneTree {
		m.moveCursor(delta)
		return
	}
	if delta > 0 {
		m.scroll().ScrollDown(delta)
		return
	}
	m.scroll().ScrollUp(-delta)
}

// jumped is the same split for the keys that move further than a line, and
// reports whether it took the key. On the tree they move the cursor: a column
// scrolled away from its own cursor answers nothing.
func (m *Model) jumped(rows int) bool {
	if m.focus != paneTree {
		return false
	}
	m.moveCursor(rows)
	return true
}

// changeTab moves the strip and takes the scroll position with it. The offset
// is restored after the content, because SetYOffset clamps to what is there.
//
// Opening Files for the first time asks the root for the diff: the fetch is the
// root's to make, and the tab is the only thing that says it is wanted.
func (m *Model) changeTab(delta int) tea.Cmd {
	m.offsets[m.tab] = m.view.YOffset()
	m.tab = (m.tab + delta + len(tabs)) % len(tabs)

	// The tree comes and goes with the tab, and focus cannot sit on a pane that
	// is no longer on screen.
	m.layout()
	m.view.SetYOffset(m.offsets[m.tab])

	if m.tab != tabFiles || m.files.Loaded || m.files.Status == store.StatusLoading {
		return nil
	}
	id := m.pr.ID
	return func() tea.Msg { return NeedFilesMsg{ID: id} }
}

// focusPane moves focus to a pane, skipping whatever is not on screen. Focus
// walks left to right, which is the order the panes are numbered in.
func (m *Model) focusPane(want pane) {
	for _, p := range []pane{paneTree, paneMain, paneRail} {
		if p == want && m.paneVisible(p) {
			m.focus = p
			m.syncContent()
			return
		}
	}
}

// focusIndex answers a digit with the pane sitting in that position. The panes
// are numbered by where they are rather than by what they hold, so 2 is the
// diff on the Files tab and the rail everywhere else.
func (m *Model) focusIndex(digit string) {
	n, err := strconv.Atoi(digit)
	if err != nil {
		return
	}
	at := 0
	for _, p := range []pane{paneTree, paneMain, paneRail} {
		if !m.paneVisible(p) {
			continue
		}
		if at++; at == n {
			m.focus = p
			m.syncContent()
			return
		}
	}
}

func (m Model) paneVisible(p pane) bool {
	switch p {
	case paneTree:
		return m.treeVisible()
	case paneRail:
		return m.railVisible()
	}
	return true
}

// scroll is the viewport the movement keys drive. Focus decides, which is what
// the help text promises and what the pane borders show.
func (m *Model) scroll() *viewport.Model {
	switch m.focus {
	case paneTree:
		return &m.treeView
	case paneRail:
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

// layout sizes the panes for the current frame, tab, and rail state.
func (m *Model) layout() {
	// Focus follows the panes: a tab switch or a resize can take the one that
	// had it off the screen.
	if !m.paneVisible(m.focus) {
		m.focus = paneMain
	}

	mainWidth := m.width
	if m.treeVisible() {
		column := m.treeColumn()
		mainWidth -= column
		m.tree = m.tree.Size(column, m.height)
		m.treeView.SetWidth(m.tree.InnerWidth())
		m.treeView.SetHeight(m.tree.InnerHeight())
	}
	if m.railVisible() {
		mainWidth -= railWidth
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
//
// The Files tab does without it. Everything the rail carries is about the pull
// request rather than the change, the tree already wants a column, and a diff
// between the two is a gutter and a fragment.
func (m Model) railVisible() bool {
	if m.tab == tabFiles {
		return false
	}
	if m.railUserSet {
		return m.railOn && m.width >= railMinForced
	}
	return m.width >= railMinFrame
}

// treeVisible decides whether the file column is on screen. It belongs to the
// Files tab alone, and a frame too narrow for it falls back to the file
// headings inside the diff.
func (m Model) treeVisible() bool {
	return m.tab == tabFiles && m.width >= treeMinFrame
}

// treeColumn is what the file column gets. It takes its full width where the
// diff can still be read at its measure and gives the rest back below that,
// down to a floor.
func (m Model) treeColumn() int {
	return min(treeWidth, max(treeMin, m.width-diffMeasure))
}

// PullRequest is what the screen is showing.
func (m Model) PullRequest() gh.PullRequest { return m.pr }

// Keys is the keymap live while this screen is up.
func (m Model) Keys() keys.DetailMap { return keys.Detail }

func (m *Model) syncContent() {
	// Code is clipped to the pane rather than wrapped: a line of source folded
	// onto a second row puts its tail under the gutter and every line below it
	// out of step with its own number.
	m.view.SoftWrap = m.tab != tabFiles

	if m.main.InnerWidth() > 0 {
		// The blank line above the first block is the same one the list opens
		// with. Content flush against the top border reads as clipped.
		m.view.SetContent("\n" + indent(m.tabBody(), m.bodyGutter()))
	}
	if m.treeVisible() {
		// No gutter and no opening blank line. The column is a list of rows,
		// each led by its own fold marker, and every cell it gives back to
		// padding comes off a file name that was already clipping.
		if inner := m.tree.InnerWidth(); inner > 0 {
			m.treeView.SetContent(m.treeBody(inner))
		}
	}
	if inner := m.rail.InnerWidth(); inner > railGutter*2 {
		// The rail opens with a blank line and sits in from both borders, the
		// same as the conversation beside it.
		body := indent(railBody(m.theme, m.railDetail(), inner-railGutter*2), railGutter)
		m.railView.SetContent("\n" + body)
	}
}

// bodyWidth is the measure the conversation is set to. Prose stops being
// readable somewhere past this, and a wide terminal would otherwise run a
// comment the whole way across the screen. Code is not prose, so the diff takes
// the pane.
func (m Model) bodyWidth() int {
	if m.tab == tabFiles {
		return m.main.InnerWidth()
	}
	return min(m.main.InnerWidth(), contentMeasure)
}

// bodyGutter centres the measure in the pane.
func (m Model) bodyGutter() int { return max(0, (m.main.InnerWidth()-m.bodyWidth())/2) }

// railDetail is what the rail has to work with. Before the query answers that
// is the list row alone, and the rail drops every section behind it.
func (m Model) railDetail() gh.PullRequestDetail {
	if m.detail.Loaded {
		return m.detail.Detail
	}
	return gh.PullRequestDetail{PullRequest: m.pr}
}

// View renders the screen. The panes carry a bracketed index only when there is
// more than one of them, because a lone pane numbered [1] is just noise, and
// they are numbered left to right rather than by what they hold.
func (m Model) View() string {
	index, at := make(map[pane]int), 0
	for _, p := range []pane{paneTree, paneMain, paneRail} {
		if m.paneVisible(p) {
			at++
			index[p] = at
		}
	}
	if at < 2 {
		index = map[pane]int{}
	}

	// The tab strip goes on the diff rather than on the tree beside it: the
	// strip is wider than the file column and would clip to a fragment there.
	panes := []string{m.main.
		Index(index[paneMain]).
		Tabs(tabs, m.tab).
		Footer(scrollFooter(m.view)).
		Focus(m.focus == paneMain).
		Render(m.view.View())}

	if m.treeVisible() {
		tree := m.tree.
			Index(index[paneTree]).
			Title(m.treeTitle()).
			Footer(scrollFooter(m.treeView)).
			Focus(m.focus == paneTree).
			Render(m.treeView.View())
		panes = append([]string{tree}, panes...)
	}

	if m.railVisible() {
		panes = append(panes, m.rail.
			Index(index[paneRail]).
			Title("Details").
			Footer(scrollFooter(m.railView)).
			Focus(m.focus == paneRail).
			Render(m.railView.View()))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, panes...)
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
func (m *Model) tabBody() string {
	switch m.tab {
	case 0:
		return m.conversation() + "\n" + m.conversationBody()
	case tabFiles:
		return m.filesBody()
	}
	return m.faint().Render(tabs[m.tab].Label + " land with their own ticket.")
}

// conversation is the header block every GitHub PR page leads with. It comes
// off the list row, so it is on screen before the detail query answers.
func (m Model) conversation() string {
	rule := lipgloss.NewStyle().Foreground(m.theme.BorderFaintOrSecondary()).
		Render(strings.Repeat("─", max(0, m.bodyWidth())))

	// The blank sets the status apart from the two lines naming the pull
	// request. Three stacked lines read as one block and the eye skips the last.
	lines := []string{m.titleLine(), m.branchLine(), "", m.statusLine()}
	if status := m.collapsedStatus(); status != "" {
		lines = append(lines, wrap(status, m.bodyWidth()))
	}
	return strings.Join(append(lines, rule), "\n")
}

// titleLine is the number, the title, and the churn pushed to the far edge.
// The churn is a fixed few cells and the title is not, so the title is the one
// that gives way: it clips rather than pushing the numbers off the line.
func (m Model) titleLine() string {
	// The number leads, in the accent the list numbers rows with, so the same
	// pull request reads the same on both screens.
	lead := lipgloss.NewStyle().Foreground(m.theme.Secondary).Bold(true).
		Render("#"+strconv.Itoa(m.pr.Number)) + " " +
		lipgloss.NewStyle().Foreground(m.theme.Primary).Bold(true).Render(m.pr.Title)

	churn := m.churn()
	room := max(0, m.bodyWidth()-lipgloss.Width(churn)-1)
	if lipgloss.Width(lead) > room {
		lead = comp.Clip(lead, room, lipgloss.NewStyle().Foreground(m.theme.Faint))
	}

	gap := max(1, m.bodyWidth()-lipgloss.Width(lead)-lipgloss.Width(churn))
	return lead + strings.Repeat(" ", gap) + churn
}

// opened is when the pull request was raised and who raised it, as one clause.
// Either half can be missing: a deleted account has no login, and the row the
// list opens with carries no timestamp until the detail query answers.
func (m Model) opened() string {
	age, login := comp.LongAgo(m.pr.CreatedAt), m.pr.Author.Login

	switch {
	case age != "" && login != "":
		return "Opened " + age + " by " + comp.Handle(login)
	case age != "":
		return "Opened " + age
	case login != "":
		return "Opened by " + comp.Handle(login)
	}
	return ""
}

// branchLine is where the work is going and where it came from. It stays on one
// line: the head branch is the long one and the one carrying a ticket key at
// the front, so it is what gives way rather than the line wrapping.
func (m Model) branchLine() string {
	faint := lipgloss.NewStyle().Foreground(m.theme.Faint)
	target := faint.Render(m.pr.BaseRefName + " ← ")

	room := max(0, m.bodyWidth()-lipgloss.Width(target))
	if lipgloss.Width(m.pr.HeadRefName) > room {
		return target + comp.Clip(faint.Render(m.pr.HeadRefName), room, faint)
	}
	return target + faint.Render(m.pr.HeadRefName)
}

// statusLine is where the pull request stands, then who raised it and when. The
// state always has something to say, so this line is never empty even when the
// clause after it is.
func (m Model) statusLine() string {
	faint := lipgloss.NewStyle().Foreground(m.theme.Faint)

	label, c := comp.PRStateLabel(m.theme, m.pr)
	icon, _ := comp.PRStateIcon(m.theme, m.pr)

	line := lipgloss.NewStyle().Foreground(c).Render(icon + " " + label)
	if opened := m.opened(); opened != "" {
		line += faint.Render(" · " + opened)
	}
	return wrap(line, m.bodyWidth())
}

// churn is the diff stat in the colors the list gives its own columns.
func (m Model) churn() string {
	return lipgloss.NewStyle().Foreground(m.theme.Success).Render("+"+strconv.Itoa(m.pr.Additions)) +
		" " + lipgloss.NewStyle().Foreground(m.theme.Error).Render("−"+strconv.Itoa(m.pr.Deletions))
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
