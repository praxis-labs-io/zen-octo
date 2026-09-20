// Package prview is the pull request detail screen.
package prview

import (
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
	"github.com/praxis-labs-io/zen-octo/internal/tui/paint"
	"github.com/praxis-labs-io/zen-octo/internal/tui/syntax"
	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

type BackMsg struct{}

type NeedFilesMsg struct{ ID string }

// NeedJobMsg asks the root for one job attempt's metadata and log. A rerun is a new JobID.
type NeedJobMsg struct {
	JobID   int64
	Refresh bool
}

type JobSettleMsg struct {
	Key     string
	JobID   int64
	Refresh bool
}

type ToggleFileViewedMsg struct {
	ID     string
	Path   string
	Viewed bool
}

// RefreshMsg asks the root to refetch pull request ID, plus its diff when Files
// is set and commit SHA's diff when SHA is non-empty.
type RefreshMsg struct {
	ID    string
	Files bool
	SHA   string
}

type PostCommentMsg struct {
	ID   string
	Body string
}

type PostReplyMsg struct {
	ID       string
	ThreadID string
	Body     string
}

type ResolveThreadMsg struct {
	ID       string
	ThreadID string
	Resolved bool
}

// SplitTooNarrowMsg reports a refused side-by-side toggle, the pane being Short columns too narrow.
type SplitTooNarrowMsg struct{ Short int }

type ThreadNotInDiffMsg struct{ Path string }

// EditorFailedMsg reports an external editor that failed. The draft and its box are untouched.
type EditorFailedMsg struct{ Err error }

type CopyLinkMsg struct{ PR gh.PullRequest }

type BrowseMsg struct{ PR gh.PullRequest }

// RailPreference is the reader's rail choice, carried to the next screen.
// Set is false until they make one, and width decides meanwhile.
type RailPreference struct {
	On  bool
	Set bool
}

const columnWidth = 37

const railMinFrame = 120

const railColumnFrom = columnWidth + 40

const railGutter = 2

const branchMeasure = 96

const contentMeasure = 90

const sideMin = 24

const treeMinFrame = 70

const diffMeasure = 80

const contentLead = 1

const titleMin = 24

const headGutter = 1

const headRoom = 3

type pane int

const (
	// Leads so the per-tab slice of parked panes starts as tabs nobody has opened.
	paneNone pane = iota
	paneSide
	paneMain
	paneRail
)

const (
	tabCommits = 1
	tabChecks  = 2
	tabFiles   = 3
)

var tabs = []comp.Tab{
	{Label: "Conversation"},
	{Label: "Commits"},
	{Label: "Checks"},
	{Label: "Files"},
}

type Model struct {
	theme theme.Theme
	side  comp.Pane
	main  comp.Pane
	rail  comp.Pane

	sideView viewport.Model
	view     viewport.Model
	railView viewport.Model

	md      comp.Markdown
	syntax  syntax.Syntax
	painter paint.Painter
	spinner comp.Spinner

	pr     gh.PullRequest
	detail store.Detail
	files  store.Files
	tab    int
	focus  pane

	rows      []row
	cursor    int
	collapsed map[string]bool

	shownPath string

	diff diffBody

	diffCursor int
	diffOn     focusKey

	selection span

	// What the reader asked for; splitting() is what a narrow pane actually draws.
	split  bool
	column gh.DiffSide

	commit commits

	check checks

	shown   string
	shownAt int

	filesAsked bool

	// Latched on the first sized layout even without a lead, so widening later never moves the keys.
	led bool

	jump string

	pageRing ring

	railRing ring

	open map[focusKey]bool

	offsets []int

	panes []pane

	compose composer
	who     gh.Actor

	inline inline

	conv convCache

	boxLine int
	boxCol  int

	mention mention

	// Once per screen, so a failed fetch is not retried on every keystroke inside an @word.
	mentionsAsked bool

	railOn      bool
	railUserSet bool

	repo store.Repo

	branches store.Branches

	picking picking

	merging merging

	width  int
	height int
}

// New builds the screen over a list row, so the header and rail paint before the detail arrives.
func New(th theme.Theme, pr gh.PullRequest, rail RailPreference, syntax syntax.Syntax) Model {
	return Model{
		theme:       th,
		side:        comp.NewPane(th),
		main:        comp.NewPane(th),
		rail:        comp.NewPane(th),
		sideView:    newViewport(),
		view:        newViewport(),
		railView:    newViewport(),
		md:          comp.NewMarkdown(th),
		syntax:      syntax,
		painter:     paint.Painter{Theme: th},
		spinner:     comp.NewSpinner(th),
		pr:          pr,
		focus:       paneMain,
		diff:        diffBody{threads: true},
		column:      gh.SideRight,
		commit:      commits{diff: diffBody{headings: true}},
		compose:     newComposer(th),
		inline:      newInline(th),
		mention:     mention{dismissed: -1},
		collapsed:   make(map[string]bool),
		open:        make(map[focusKey]bool),
		offsets:     make([]int, len(tabs)),
		panes:       make([]pane, len(tabs)),
		railOn:      rail.On,
		railUserSet: rail.Set,
	}
}

// SetFiles takes the store's diff for this pull request. The command reports a pending jump that could not land.
func (m *Model) SetFiles(f store.Files) tea.Cmd {
	m.files = f
	m.diff.blocks = nil
	m.syncRows()

	m.cursor = min(m.cursor, max(0, len(m.rows)-1))
	if m.cursor == 0 {
		m.cursor = m.firstFile()
	}

	m.nameShownFile()
	m.layout()
	return m.finishJump()
}

func (m Model) firstFile() int {
	for i, r := range m.rows {
		if r.file != nil && len(r.file.Hunks) > 0 {
			return i
		}
	}
	for i, r := range m.rows {
		if r.file != nil {
			return i
		}
	}
	return 0
}

// SetDetail takes the store's detail for this pull request. The command arms the commit and job fetch waits.
func (m *Model) SetDetail(d store.Detail) tea.Cmd {
	headMoved := m.detail.Loaded && d.Loaded && m.detail.Detail.HeadRefOid != d.Detail.HeadRefOid
	m.detail = d
	m.diff.blocks = nil
	m.conv.ok = false
	if d.Loaded {
		m.pr = d.Detail.PullRequest
	}
	if headMoved {
		for key := range m.open {
			if key.kind == focusHunk {
				delete(m.open, key)
			}
		}
	}

	for _, t := range d.Detail.Threads {
		if !t.IsResolved {
			delete(m.open, threadKey(t))
		}
	}
	m.syncCommits()
	m.syncChecks()

	held := m.focusWhole()

	m.layout()
	m.keepFocusWhole(held)
	return tea.Batch(m.armCommit(), m.armJob(), m.armJobRender())
}

func (m Model) focusWhole() bool {
	top := bodyTop(&m.view)
	return m.mainRing().show(top, m.view.Height()) == top
}

func (m *Model) keepFocusWhole(was bool) {
	if !was || m.focusWhole() {
		return
	}
	m.showFocus(&m.pageRing, &m.view, bodyTop(&m.view))
}

func newViewport() viewport.Model {
	vp := viewport.New()
	vp.SoftWrap = true
	vp.FillHeight = true
	return vp
}

func (m Model) Init() tea.Cmd { return tea.Batch(m.spinner.Tick(), m.armJobRender()) }

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case CommitSettleMsg:
		return m, m.settleCommit(msg)
	case JobSettleMsg:
		return m, m.settleJob(msg)
	case jobParsedMsg:
		return m, m.jobParsed(msg)
	case jobRenderMsg:
		return m, m.jobRendered(msg)
	case SearchSettleMsg:
		return m, m.settleCheckSearch(msg)

	case BranchSettleMsg:
		return m, m.settleBranches(msg)

	case editorDoneMsg:
		return m.editorReturned(msg)

	case spinner.TickMsg:
		cmd := m.spinner.Advance(msg, m.waiting())
		m.syncContent()
		return m, cmd

	default:
		if m.merging.open {
			return m, m.merging.update(msg)
		}

		box := m.writing()
		if box == nil {
			return m, nil
		}

		var cmd tea.Cmd
		box.area, cmd = box.area.Update(msg)

		ask := m.syncMention()
		m.showBox()
		return m, tea.Batch(cmd, ask)
	}
}

func (m *Model) writing() *composer {
	switch {
	case m.compose.typing:
		return &m.compose
	case m.inline.typing:
		return &m.inline.composer
	}
	return nil
}

func (m *Model) showBox() {
	if m.inline.typing {
		m.showInline()
		return
	}
	m.showCompose()
}

// Answers for every request rather than the current tab's, because one tick chain drives the spinner on all of them.
func (m Model) waiting() bool {
	return (!m.detail.Loaded && m.detail.Status == store.StatusLoading) ||
		waitingFor(m.files) || waitingFor(m.commit.files) || waitingForJob(m.check.job) || m.commitBlank() ||
		m.rerunBlank() || m.mentionWaiting()
}

func (m Model) rerunBlank() bool {
	return m.tab == tabChecks && m.checkRerunning(m.check.selected)
}

func (m Model) commitBlank() bool {
	if m.tab != tabCommits || m.commit.sha != "" {
		return false
	}
	_, ok := m.underCursor()
	return ok
}

func waitingFor(f store.Files) bool {
	return !f.Loaded && f.Status == store.StatusLoading
}

func waitingForJob(j store.Job) bool {
	return !j.Loaded && j.Status == store.StatusLoading
}

func (m Model) handleKey(keyMsg tea.KeyPressMsg) (Model, tea.Cmd) {
	k := keys.Detail

	switch {
	case m.merging.open:
		return m.mergeKey(keyMsg)
	case m.picking.open():
		return m.pickerKey(keyMsg)
	case m.compose.typing:
		return m.composeKey(keyMsg)
	case m.inline.typing:
		return m.inlineKey(keyMsg)
	case m.check.searching:
		return m.checkSearchKey(keyMsg)
	}

	switch {
	case key.Matches(keyMsg, k.Back):
		if m.tab == tabChecks && !m.check.search.Empty() {
			m.clearCheckSearch()
			return m, nil
		}
		if m.selecting() {
			m.clearSelection()
			return m, nil
		}
		return m, func() tea.Msg { return BackMsg{} }

	case key.Matches(keyMsg, k.Sync):
		return m, m.refresh()

	case key.Matches(keyMsg, k.ToggleViewed) && m.tab == tabFiles:
		return m.toggleFileViewed()

	case key.Matches(keyMsg, k.Search) && m.tab == tabChecks:
		m.startCheckSearch()
		return m, nil
	case key.Matches(keyMsg, k.NextMatch) && m.tab == tabChecks:
		m.moveCheckMatch(1)
		return m, nil
	case key.Matches(keyMsg, k.PrevMatch) && m.tab == tabChecks:
		m.moveCheckMatch(-1)
		return m, nil
	case key.Matches(keyMsg, k.FirstFailure) && m.tab == tabChecks:
		m.jumpFirstCheckFailure()
		return m, nil

	case key.Matches(keyMsg, k.CopyLink):
		return m, func() tea.Msg { return CopyLinkMsg{PR: m.pr} }

	case key.Matches(keyMsg, k.Browse):
		return m, func() tea.Msg { return BrowseMsg{PR: m.pr} }

	case key.Matches(keyMsg, k.Comment) && m.canCompose():
		return m.writeComment()

	case key.Matches(keyMsg, k.Activate) && m.focus == paneRail:
		return m.openRailPicker()

	case key.Matches(keyMsg, k.Activate) && m.canCompose() && m.pageRing.on.kind == focusCompose:
		return m.writeComment()

	case key.Matches(keyMsg, k.Reply) && m.canRerunCheck():
		return m, m.rerunCheck()

	case key.Matches(keyMsg, k.Reply) && m.canRerunRun(false):
		return m, m.rerunRun(false)
	case key.Matches(keyMsg, k.QuoteReply) && m.canRerunRun(true):
		return m, m.rerunRun(true)

	case key.Matches(keyMsg, k.Reply):
		return m.startReply(false)
	case key.Matches(keyMsg, k.QuoteReply):
		return m.startReply(true)

	case key.Matches(keyMsg, k.Edit):
		return m.startEdit()
	case key.Matches(keyMsg, k.Delete):
		return m.startDelete()

	case key.Matches(keyMsg, k.React):
		return m.startReact()

	case key.Matches(keyMsg, k.Resolve):
		return m.toggleResolved()
	case key.Matches(keyMsg, k.Select):
		return m.toggleSelection()
	case key.Matches(keyMsg, k.Activate):
		return m.showInDiff()

	case key.Matches(keyMsg, k.NextTab):
		return m, m.changeTab(1)
	case key.Matches(keyMsg, k.PrevTab):
		return m, m.changeTab(-1)

	case key.Matches(keyMsg, k.NextInColumn) && m.columnNoun() != "":
		m.stepColumnItem(1)
	case key.Matches(keyMsg, k.PrevInColumn) && m.columnNoun() != "":
		m.stepColumnItem(-1)

	case key.Matches(keyMsg, k.NextBlock) && m.tab == tabFiles:
		m.walkDiff(1)
	case key.Matches(keyMsg, k.PrevBlock) && m.tab == tabFiles:
		m.walkDiff(-1)

	case key.Matches(keyMsg, k.NextBlock) && m.tab == tabCommits:
		m.jumpCommitFile(1)
	case key.Matches(keyMsg, k.PrevBlock) && m.tab == tabCommits:
		m.jumpCommitFile(-1)
	case key.Matches(keyMsg, k.NextBlock) && m.tab == tabChecks:
		m.moveCheckStep(1)
	case key.Matches(keyMsg, k.PrevBlock) && m.tab == tabChecks:
		m.moveCheckStep(-1)

	case key.Matches(keyMsg, k.NextBlock) && !m.railDriving():
		m.stepFocus(1)
	case key.Matches(keyMsg, k.PrevBlock) && !m.railDriving():
		m.stepFocus(-1)

	case key.Matches(keyMsg, k.PaneLeft):
		if !m.stepColumn(gh.SideLeft) {
			m.stepPane(-1)
		}
	case key.Matches(keyMsg, k.PaneRight):
		if !m.stepColumn(gh.SideRight) {
			m.stepPane(1)
		}
	case key.Matches(keyMsg, k.SplitView) && m.tab == tabFiles:
		return m, m.toggleSplit()
	case key.Matches(keyMsg, k.FocusPane):
		m.focusIndex(keyMsg.String())

	case key.Matches(keyMsg, k.Expand) && m.checkFoldable():
		m.toggleCheckFold()
	case key.Matches(keyMsg, k.Expand) && m.checkStepFoldable():
		m.toggleCheckStep()
	case key.Matches(keyMsg, k.Expand) && m.tab == tabFiles && m.focus != paneMain:
		m.toggleFold()
	case key.Matches(keyMsg, k.Expand):
		m.toggleBlockFold()

	case key.Matches(keyMsg, k.ToggleRail) && !m.railTab():

	case key.Matches(keyMsg, k.ToggleRail):
		m.railOn, m.railUserSet = !m.railVisible(), true
		switch {
		case m.railVisible():
			m.focus = paneRail
		case m.focus == paneRail:
			m.focus = paneMain
		}
		m.layout()
		m.landCursor()

	case key.Matches(keyMsg, k.Down):
		m.move(1)
	case key.Matches(keyMsg, k.Up):
		m.move(-1)
	case key.Matches(keyMsg, k.Top):
		if !m.gotoCheckLine(false) && !m.jumped(-m.sideRows()) {
			m.scroll().GotoTop()
		}
	case key.Matches(keyMsg, k.Bottom):
		if !m.gotoCheckLine(true) && !m.jumped(m.sideRows()) {
			m.scroll().GotoBottom()
		}
	case key.Matches(keyMsg, k.PageDown):
		if !m.pageCheckLine(m.view.Height(), true) && !m.jumped(m.sidePage()) {
			m.scroll().PageDown()
		}
	case key.Matches(keyMsg, k.PageUp):
		if !m.pageCheckLine(-m.view.Height(), true) && !m.jumped(-m.sidePage()) {
			m.scroll().PageUp()
		}
	case key.Matches(keyMsg, k.HalfPageDown):
		if m.tab == tabChecks && m.focus == paneSide {
			break
		}
		if !m.pageCheckLine(max(1, m.view.Height()/2), false) && !m.jumped(m.sidePage()/2) {
			m.scroll().HalfPageDown()
		}
	case key.Matches(keyMsg, k.HalfPageUp):
		if m.tab == tabChecks && m.focus == paneSide {
			break
		}
		if !m.pageCheckLine(-max(1, m.view.Height()/2), false) && !m.jumped(-m.sidePage()/2) {
			m.scroll().HalfPageUp()
		}
	}

	return m, tea.Batch(m.armCommit(), m.armJob(), m.armJobRender())
}

func (m Model) refresh() tea.Cmd {
	msg := RefreshMsg{ID: m.pr.ID}
	switch m.tab {
	case tabFiles:
		msg.Files = true
	case tabCommits:
		msg.SHA = m.commit.sha
	}
	return func() tea.Msg { return msg }
}

func (m *Model) move(delta int) {
	switch {
	case m.moveCheckLine(delta):
		return
	case m.moveDiffCursor(delta):
		return
	case m.sideDriving():
		m.moveSide(delta)
		return
	case m.railDriving() && m.stepFocus(delta):
		return
	}
	if delta > 0 {
		m.scroll().ScrollDown(delta)
		return
	}
	m.scroll().ScrollUp(-delta)
}

func (m *Model) jumped(rows int) bool {
	if !m.sideDriving() {
		return false
	}
	m.moveSide(rows)
	return true
}

func (m Model) railDriving() bool {
	return m.focus == paneRail && m.railVisible() && m.railRing.stops() > 0
}

func (m Model) sideDriving() bool { return m.focus == paneSide && m.sideRows() > 0 }

func (m *Model) moveSide(delta int) {
	switch m.tab {
	case tabCommits:
		m.moveCommit(delta)
	case tabChecks:
		m.moveCheck(delta)
	default:
		m.moveCursor(delta)
	}
}

func (m *Model) stepColumnItem(delta int) {
	if m.tab == tabFiles {
		m.jumpFile(delta)
		return
	}
	m.moveSide(delta)
}

func (m Model) columnNoun() string {
	switch m.tab {
	case tabCommits:
		return "commit"
	case tabChecks:
		return "check"
	case tabFiles:
		return "file"
	}
	return ""
}

func (m Model) sideRows() int {
	switch m.tab {
	case tabCommits:
		return len(m.detail.Detail.Commits)
	case tabChecks:
		return len(m.check.rows)
	}
	return len(m.rows)
}

func (m Model) sidePage() int {
	if m.tab == tabCommits {
		return max(1, m.sideView.Height()/commitRowHeight)
	}
	return m.sideView.Height()
}

func showRow(v *viewport.Model, cursor int) {
	height := max(1, v.Height())

	switch offset := v.YOffset(); {
	case cursor < offset:
		v.SetYOffset(cursor)
	case cursor >= offset+height:
		v.SetYOffset(cursor - height + 1)
	}
}

func (m *Model) showSideCursor() {
	if m.sideRows() == 0 {
		m.sideView.SetYOffset(0)
		return
	}
	switch m.tab {
	case tabCommits:
		m.moveCommit(0)
	case tabChecks:
		showRow(&m.sideView, m.check.cursor)
	default:
		m.showCursorRow()
	}
}

func (m *Model) changeTab(delta int) tea.Cmd {
	return m.goToTab((m.tab + delta + len(tabs)) % len(tabs))
}

func (m *Model) goToTab(at int) tea.Cmd {
	m.offsets[m.tab] = m.view.YOffset()
	m.panes[m.tab] = m.focus
	m.tab = at

	m.clearMention()

	m.layout()
	m.view.SetYOffset(m.offsets[m.tab])
	m.showSideCursor()

	m.focusPane(m.panes[m.tab])

	if m.panes[m.tab] == paneNone && (m.tab == tabCommits || m.tab == tabChecks) && m.sideVisible() {
		m.focus = paneSide
		m.syncContent()
	}

	if m.tab != tabFiles || m.filesAsked || m.files.Status == store.StatusLoading {
		return tea.Batch(m.armCommit(), m.armJob(), m.armJobRender())
	}
	m.filesAsked = true

	id := m.pr.ID
	return func() tea.Msg { return NeedFilesMsg{ID: id} }
}

func (m *Model) leadPane() {
	if panes := m.visiblePanes(); len(panes) > 1 {
		m.focusPane(panes[0])
	}
}

func (m *Model) focusRing() (*ring, *viewport.Model) {
	switch {
	case m.focus == paneRail && m.railVisible():
		return &m.railRing, &m.railView
	case m.focus == paneMain && m.ringTab():
		return &m.pageRing, &m.view
	}
	return nil, nil
}

func (m *Model) stepFocus(delta int) bool {
	r, vp := m.focusRing()
	if r == nil {
		return false
	}

	top := bodyTop(vp)
	if !r.step(delta, top, vp.Height()) {
		return false
	}

	m.syncContent()
	m.showFocus(r, vp, top)
	return true
}

func (m *Model) walkDiff(delta int) {
	if m.focus != paneMain && m.focusPane(paneMain) {
		return
	}

	m.unpoint()
	if !m.stepFocus(delta) {
		m.crossFile(delta)
	}
}

func (m *Model) showFocus(r *ring, vp *viewport.Model, top int) {
	at := r.index()
	if m.tab == tabFiles && at >= 0 && r.items[at].kind != focusHunk && !r.items[at].whole(top, vp.Height()) {
		vp.SetYOffset(contentLead + m.jumpTop(m.shownPath, r.items[at].start))
		return
	}
	vp.SetYOffset(contentLead + r.show(top, vp.Height()))
}

// Unclamped at zero: clamping here while contentLead is added back moves the page a line.
func bodyTop(vp *viewport.Model) int { return vp.YOffset() - contentLead }

func (m *Model) toggleBlockFold() bool {
	r, vp := m.focusRing()
	if r == nil || (!r.on.kind.prose() && r.on.kind != focusHunk) {
		return false
	}

	top := bodyTop(vp)
	if !r.live(top, vp.Height()) {
		return false
	}

	key := m.foldTarget()
	if key.kind == focusHunk {
		m.open[key] = !m.hunkOpen(key)
	} else {
		m.open[key] = !m.open[key]
		m.conv.ok = false
	}

	m.syncContent()
	m.showFocus(r, vp, top)
	return true
}

func (m Model) foldTarget() focusKey {
	if m.mainRing().on.kind == focusHunk {
		return m.mainRing().on
	}
	t, ok := m.threadOnRing()
	if !ok {
		return m.mainRing().on
	}
	if t.IsResolved {
		return threadKey(t)
	}
	if within := m.within(t); within != "" {
		return focusKey{kind: focusThreadComment, id: within}
	}
	return m.mainRing().on
}

func (m Model) visiblePanes() []pane {
	out := make([]pane, 0, 2)
	for _, p := range []pane{paneRail, paneSide, paneMain} {
		if m.paneVisible(p) {
			out = append(out, p)
		}
	}
	return out
}

func (m *Model) focusPane(want pane) bool {
	if !m.paneVisible(want) {
		return false
	}
	m.focus = want
	m.syncContent()
	return m.landCursor()
}

func (m *Model) stepPane(delta int) {
	panes := m.visiblePanes()
	for i, p := range panes {
		if p != m.focus {
			continue
		}
		if next := i + delta; next >= 0 && next < len(panes) {
			m.focusPane(panes[next])
		}
		return
	}
}

func (m *Model) landCursor() bool {
	r, _ := m.focusRing()
	if r == nil || r.index() >= 0 {
		return false
	}
	return m.stepFocus(1)
}

func (m *Model) focusIndex(digit string) {
	n, err := strconv.Atoi(digit)
	if err != nil {
		return
	}
	if panes := m.visiblePanes(); n >= 1 && n <= len(panes) {
		m.focusPane(panes[n-1])
	}
}

func (m Model) paneVisible(p pane) bool {
	switch p {
	case paneSide:
		return m.sideVisible()
	case paneRail:
		return m.railVisible()
	case paneMain:
		return true
	}
	return false
}

func (m *Model) scroll() *viewport.Model {
	switch {
	case m.sideDriving():
		return &m.sideView
	case m.focus == paneRail:
		return &m.railView
	}
	return &m.view
}

func (m Model) Rail() RailPreference {
	return RailPreference{On: m.railOn, Set: m.railUserSet}
}

func (m *Model) SetSize(width, height int) {
	split := m.splitting()
	m.width, m.height = width, height
	m.layout()
	m.dropSelectionAcrossSplit(split)

	m.merging.resize(width, height)
}

func (m *Model) layout() {
	m.conv.ok = false

	if !m.paneVisible(m.focus) {
		m.focus = m.visiblePanes()[0]
	}

	paneHeight := m.height
	if head := m.head(); head != "" {
		paneHeight = max(0, m.height-strings.Count(head, "\n")-1)
	}

	mainWidth := m.width
	if m.sideVisible() {
		column := m.sideColumn()
		mainWidth -= column
		m.side = m.side.Size(column, paneHeight)
		m.sideView.SetWidth(m.side.InnerWidth())
		m.sideView.SetHeight(m.sideHeight())
	}
	if m.railVisible() {
		if m.railColumn() {
			mainWidth -= columnWidth
		}
		m.rail = m.rail.Size(columnWidth, paneHeight)
		m.railView.SetWidth(m.rail.InnerWidth())
		m.railView.SetHeight(m.rail.InnerHeight())
	}

	m.main = m.main.Size(mainWidth, paneHeight).Header(m.mainHeading())
	m.view.SetWidth(m.main.InnerWidth())
	m.view.SetHeight(max(0, m.main.InnerHeight()-(m.main.Above()-1)))
	m.syncContent()

	if !m.led && m.width > 0 {
		m.led = true
		m.leadPane()
	}

	m.landCursor()
}

func (m Model) railVisible() bool {
	if !m.railTab() {
		return false
	}
	if !m.railColumn() && m.Composing() {
		return false
	}
	if m.railUserSet {
		return m.railOn
	}
	return m.width >= railMinFrame
}

func (m Model) railColumn() bool { return m.width >= railColumnFrom }

func (m Model) railTab() bool { return !m.sideVisibleTab() }

func (m Model) ringTab() bool { return m.tab == tabFiles || m.railTab() }

func (m Model) mainRing() ring {
	if !m.ringTab() {
		return ring{}
	}
	return m.pageRing
}

func (m Model) sideVisibleTab() bool {
	switch m.tab {
	case tabCommits, tabChecks, tabFiles:
		return true
	}
	return false
}

// Checks keeps its column at every width, since hiding it leaves every other job unreachable.
func (m Model) sideVisible() bool {
	return m.sideVisibleTab() && (m.width >= treeMinFrame || m.tab == tabChecks)
}

func (m Model) sideColumn() int {
	return min(columnWidth, max(sideMin, m.width-diffMeasure))
}

func (m Model) sideHeight() int {
	h := m.side.InnerHeight()
	if m.tab == tabCommits && h >= commitRowHeight {
		return h / commitRowHeight * commitRowHeight
	}
	return h
}

func (m Model) PullRequest() gh.PullRequest { return m.pr }

// ShowsTimeline is whether the Conversation tab is up.
func (m Model) ShowsTimeline() bool { return m.railTab() }

func (m Model) ShowsChecks() bool { return m.tab == tabChecks }

func (m Model) Keys() keys.DetailMap { return keys.Detail }

// ShortHelp is the status bar's key hints, limited to keys that act on the
// current tab and focus. Nil while a modal or box has the keyboard.
func (m Model) ShortHelp() []key.Binding {
	if m.Composing() || m.picking.open() || m.merging.open {
		return nil
	}
	if m.check.searching {
		return keys.Detail.SearchHelp()
	}
	file := m.fileViewTarget()
	rail := m.railDriving()

	job := m.check.job.Loaded || m.checkHasJob()
	return keys.Detail.ShortHelp(keys.DetailContext{
		Blocks:      !rail && (m.tab != tabChecks || job),
		Expand:      !rail && (m.tab == tabFiles || m.railTab() || m.checkFoldable() || m.checkStepFoldable()),
		Activate:    rail,
		Panes:       rail,
		Rail:        m.railTab(),
		Column:      m.columnNoun(),
		Split:       m.tab == tabFiles && m.files.Loaded,
		Select:      m.diffDriving() && m.walkedInto(m.pageRing.on),
		Selecting:   m.selecting(),
		FileView:    file != nil && !file.Viewing,
		FileViewed:  file != nil && file.Viewed == gh.FileViewed,
		JobLog:      m.tab == tabChecks && job,
		JobFailure:  m.tab == tabChecks && job && m.checkFailed(),
		JobMatches:  m.tab == tabChecks && len(m.check.matchLines) > 0,
		JobRerun:    m.canRerunCheck(),
		RunRerun:    m.canRerunRun(false),
		RunRerunAll: m.canRerunRun(true),

		SearchStanding: m.tab == tabChecks && !m.check.search.Empty(),
	})
}

func (m Model) fileViewTarget() *gh.ChangedFile {
	if m.tab != tabFiles {
		return nil
	}
	if m.sideDriving() {
		if m.cursor >= 0 && m.cursor < len(m.rows) {
			return m.rows[m.cursor].file
		}
		return nil
	}
	return m.shownFile()
}

func (m Model) toggleFileViewed() (Model, tea.Cmd) {
	file := m.fileViewTarget()
	if file == nil || file.Viewing {
		return m, nil
	}
	msg := ToggleFileViewedMsg{
		ID:     m.pr.ID,
		Path:   file.Path,
		Viewed: file.Viewed != gh.FileViewed,
	}
	if msg.Viewed {
		if !m.sideDriving() {
			m.pointFileCursor(file.Path)
		}
		m.jumpFile(1)
	}
	return m, func() tea.Msg { return msg }
}

func (m *Model) pointFileCursor(path string) {
	for i, row := range m.rows {
		if row.file != nil && row.file.Path == path {
			m.cursor = i
			return
		}
	}
}

func (m *Model) syncContent() {
	m.view.SoftWrap = false

	if inner := m.main.InnerWidth(); inner > 0 {
		body := "\n" + indent(m.tabBody(), m.bodyGutter()) + "\n"
		if body != m.shown || inner != m.shownAt {
			m.view.SetContent(body)
			m.shown, m.shownAt = body, inner
		}
	}
	if m.sideVisible() {
		if inner := m.side.InnerWidth(); inner > 0 {
			m.sideView.SetContent(m.sideBody(inner))
		}
	}
	if inner := m.rail.InnerWidth(); m.railVisible() && inner > railGutter*2 {
		m.railView.SetContent("\n" + m.railBody(inner))
	} else {
		m.railRing = ring{}
	}
}

func (m Model) bodyWidth() int {
	if m.sideVisibleTab() {
		return m.main.InnerWidth()
	}
	return min(m.main.InnerWidth(), contentMeasure)
}

func (m Model) bodyGutter() int { return max(0, (m.main.InnerWidth()-m.bodyWidth())/2) }

func (m Model) railDetail() gh.PullRequestDetail {
	if m.detail.Loaded {
		return m.detail.Detail
	}
	return gh.PullRequestDetail{PullRequest: m.pr}
}

func (m Model) View() string {
	index := map[pane]int{}
	if panes := m.visiblePanes(); len(panes) > 1 {
		for i, p := range panes {
			index[p] = i + 1
		}
	}

	mainView := m.view.View()
	if m.tab == tabChecks {
		mainView = m.paintCheckCursor(mainView)
	}
	panes := []string{m.main.
		Index(index[paneMain]).
		Title(m.mainTitle()).
		Header(m.mainHeading()).
		Footer(scrollFooter(m.view)).
		Focus(m.focus == paneMain).
		Render(mainView)}

	if m.sideVisible() {
		column := m.side.
			Index(index[paneSide]).
			Title(m.sideTitle()).
			Footer(scrollFooter(m.sideView)).
			Focus(m.focus == paneSide).
			Render(m.sideView.View())
		panes = append([]string{column}, panes...)
	}

	rail := ""
	if m.railVisible() {
		rail = m.rail.
			Index(index[paneRail]).
			Title("Details").
			Footer(scrollFooter(m.railView)).
			Focus(m.focus == paneRail).
			Render(m.railView.View())
	}
	if m.railColumn() && rail != "" {
		panes, rail = append([]string{rail}, panes...), ""
	}

	frame := lipgloss.JoinHorizontal(lipgloss.Top, panes...)

	head := m.head()
	lead := headRows(head)
	if head != "" {
		frame = lipgloss.JoinVertical(lipgloss.Left, head, frame)
	}

	if rail != "" {
		frame = comp.At(frame, rail, 0, lead, m.width, m.height)
	}

	return m.mergeOverlay(m.pickerOverlay(m.mentionOverlay(frame, lead)))
}

func (m Model) headLead() int { return headRows(m.head()) }

func headRows(head string) int {
	if head == "" {
		return 0
	}
	return strings.Count(head, "\n") + 1
}

// Cursor is where the terminal draws its cursor, relative to this screen's frame, or nil when nothing is taking text.
func (m Model) Cursor() *tea.Cursor {
	switch {
	case m.merging.open:
		return m.merging.cursor(m.theme, m.width, m.height)
	case m.picking.open():
		return m.picking.p.Cursor(m.theme, m.width, m.height)
	case m.Composing():
		return m.composeCursor(m.headLead())
	case m.check.searching:
		return m.checkCursor(m.headLead())
	}
	return nil
}

func scrollFooter(v viewport.Model) string {
	total := v.TotalLineCount()
	if total <= v.Height() {
		return ""
	}
	return strconv.Itoa(min(v.YOffset()+v.Height(), total)) + "/" + strconv.Itoa(total)
}

func (m *Model) tabBody() string {
	m.pageRing.reset()

	switch m.tab {
	case tabCommits:
		return m.commitBody()
	case tabChecks:
		return m.checkBody()
	case tabFiles:
		return m.filesBody()
	}

	if body, ok := m.conversationNote(); ok {
		return comp.Centered(body, m.bodyWidth(), m.view.Height()-contentLead)
	}
	return m.conversationBody()
}

func (m *Model) sideBody(width int) string {
	switch m.tab {
	case tabCommits:
		return m.commitColumn(width)
	case tabChecks:
		return m.checkColumn(width)
	}
	return m.treeBody(width)
}

func (m Model) sideTitle() string {
	switch m.tab {
	case tabCommits:
		return "Commits"
	case tabChecks:
		return "Checks"
	}
	return "Files"
}

func (m Model) mainTitle() string {
	switch m.tab {
	case tabCommits, tabFiles:
		return "Diff"
	case tabChecks:
		return "Log"
	}
	return "Feed"
}

func (m Model) frameHead() string {
	width := m.headWidth()

	lines := []string{m.titleLine(width), m.branchRow(width), "", m.tabStrip(width), ""}
	return indent(strings.Join(lines, "\n"), headGutter)
}

func (m Model) tabStrip(width int) string {
	strip := m.renderTabs(m.tabCounts())
	if lipgloss.Width(strip) > width {
		strip = m.renderTabs(tabs)
	}
	if lipgloss.Width(strip) > width {
		return paint.Clip(strip, width, lipgloss.NewStyle().Foreground(m.theme.Subtle))
	}
	return strip
}

func (m Model) renderTabs(list []comp.Tab) string {
	active := lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true).Underline(true)
	idle := lipgloss.NewStyle().Foreground(m.theme.Subtle)
	count := lipgloss.NewStyle().Foreground(m.theme.MutedOrSubtle())

	parts := make([]string, 0, len(list))
	for i, tab := range list {
		style := idle
		if i == m.tab {
			style = active
		}
		part := style.Render(tab.Label)
		if tab.Badge != "" {
			part += count.Render(" " + tab.Badge)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "  ")
}

// Commits and Checks render bare until the detail lands, because a zero would claim they are empty.
func (m Model) tabCounts() []comp.Tab {
	counted := make([]comp.Tab, len(tabs))
	copy(counted, tabs)

	counted[0].Badge = tabCount(m.pr.Comments)
	counted[tabFiles].Badge = tabCount(m.pr.ChangedFiles)
	if m.detail.Loaded {
		counted[tabCommits].Badge = tabCount(len(m.detail.Detail.Commits))
		counted[tabChecks].Badge = tabCount(len(m.detail.Detail.Rollup.Checks))
	}
	return counted
}

func tabCount(n int) string {
	if n == 0 {
		return ""
	}
	return "(" + strconv.Itoa(n) + ")"
}

func (m Model) head() string {
	lines := strings.Split(m.frameHead(), "\n")

	room := max(0, m.height-headRoom)
	if len(lines) <= room {
		return strings.Join(lines, "\n")
	}

	lines = lines[:room]
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func (m Model) headWidth() int { return max(1, m.width-headGutter*2) }

func (m Model) titleLine(width int) string {
	lead := lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true).
		Render("#"+strconv.Itoa(m.pr.Number)) + " " +
		lipgloss.NewStyle().Foreground(m.theme.Text).Bold(true).Render(m.pr.Title)

	return m.spread(lead, m.titleRight(max(0, width-titleMin-1)), width)
}

func (m Model) titleRight(room int) string {
	state := m.stateBadge()

	for _, half := range []string{
		m.statusHalf() + m.churn(),
		m.statusHalf(),
		m.joinStatus(state, m.reviewBadge()),
		state,
	} {
		if lipgloss.Width(half) <= room {
			return half
		}
	}
	return state
}

func (m Model) churn() string {
	if changes := m.changes(); changes != "" {
		return "  " + changes
	}
	return ""
}

func (m Model) spread(left, right string, width int) string {
	faint := lipgloss.NewStyle().Foreground(m.theme.Subtle)

	room := width
	if right != "" {
		room = width - lipgloss.Width(right) - 1
	}
	if room < 1 {
		if lipgloss.Width(right) > width {
			return paint.Clip(right, width, faint)
		}
		return right
	}
	if lipgloss.Width(left) > room {
		left = paint.Clip(left, room, faint)
	}

	gap := max(0, width-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", gap) + right
}

// Readout is the author and age of the pull request for the status bar. Either half may be missing.
func (m Model) Readout() string {
	age, login := comp.RelativeTime(m.pr.CreatedAt), comp.Handle(m.pr.Author.Login)

	switch {
	case age != "" && login != "":
		return login + " · " + age
	case age != "":
		return age
	}
	return login
}

func (m Model) branchLine(width int) string {
	faint := lipgloss.NewStyle().Foreground(m.theme.Subtle)
	arrow := " ← "

	base, head := m.pr.BaseRefName, m.pr.HeadRefName
	baseRoom, headRoom := shareBranchRoom(lipgloss.Width(base), lipgloss.Width(head),
		min(width, branchMeasure)-lipgloss.Width(arrow))

	return clipTo(faint.Render(base), baseRoom, faint) +
		faint.Render(arrow) +
		clipTo(faint.Render(head), headRoom, faint)
}

func shareBranchRoom(base, head, room int) (int, int) {
	if room <= 0 {
		return 0, 0
	}
	if base+head <= room {
		return base, head
	}

	half := room / 2
	switch {
	case base <= half:
		return base, room - base
	case head <= half:
		return room - head, head
	}
	return half, room - half
}

func (m Model) branchRow(width int) string {
	return m.branchLine(width)
}

func (m Model) statusHalf() string {
	return m.joinStatus(m.stateBadge(), m.checksBadge(), m.reviewBadge())
}

func (m Model) joinStatus(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, lipgloss.NewStyle().Foreground(m.theme.Subtle).Render(" · "))
}

func (m Model) stateBadge() string {
	label, c := comp.PRStateLabel(m.theme, m.pr)
	icon, _ := comp.PRStateIcon(m.theme, m.pr)
	return lipgloss.NewStyle().Foreground(c).Render(icon + " " + label)
}

func (m Model) checksBadge() string {
	label, c := comp.CheckStateLabel(m.theme, m.pr.Checks)
	if label == "" {
		return ""
	}
	glyph, _ := comp.CheckStateIcon(m.theme, m.pr.Checks)
	return lipgloss.NewStyle().Foreground(c).Render(glyph + " " + label)
}

func (m Model) reviewBadge() string {
	label, c := comp.ReviewLabel(m.theme, m.pr.ReviewDecision)
	if label == "" {
		return ""
	}
	return lipgloss.NewStyle().Foreground(c).Render(label)
}

func (m Model) changes() string {
	return lipgloss.NewStyle().Foreground(m.theme.Success).Render("+"+strconv.Itoa(m.pr.Additions)) +
		" " + lipgloss.NewStyle().Foreground(m.theme.Error).Render("−"+strconv.Itoa(m.pr.Deletions))
}
