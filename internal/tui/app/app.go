// Package app holds the root Bubble Tea model. It owns the screens, divides the
// frame between them and the status bar, and handles the keys that answer
// whatever has focus. All model mutation happens in Update; commands do the
// asynchronous work and deliver typed messages back.
package app

import (
	"cmp"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/config"
	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/store"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
	"github.com/zen-octo/zen-octo/internal/tui/keys"
	"github.com/zen-octo/zen-octo/internal/tui/list"
	"github.com/zen-octo/zen-octo/internal/tui/prview"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// GitHub is the slice of the client this model needs. Declaring it here rather
// than in the gh package is what lets tests drive the UI without a network.
type GitHub interface {
	SearchPullRequests(ctx context.Context, query string, limit int) (gh.SearchResult, error)
	PullRequest(ctx context.Context, id, headRef string) (gh.DetailResult, error)
	PullRequestFiles(ctx context.Context, repo string, number, changedFiles int) (gh.FilesResult, error)
}

type sectionFetchedMsg struct {
	index int
	res   gh.SearchResult
}

type sectionFailedMsg struct {
	index int
	err   error
}

// The detail messages name a pull request rather than a screen. Open one,
// escape, open another, and the first response still arrives; the id is what
// keeps it off the screen that replaced it.
type detailFetchedMsg struct {
	id  string
	res gh.DetailResult
}

type detailFailedMsg struct {
	id  string
	err error
}

// The diff is a second request, made the first time someone opens the Files
// tab. It names its pull request for the same reason the detail messages do.
type filesFetchedMsg struct {
	id  string
	res gh.FilesResult
}

type filesFailedMsg struct {
	id  string
	err error
}

// fetchTimeout bounds a single request. Without it a half-open socket leaves
// the UI spinning with no error and no way out but quitting.
const fetchTimeout = 30 * time.Second

// statusBarHeight is the one line the status bar occupies. It is subtracted
// once, here, and every region below is told what it got.
const statusBarHeight = 1

type screen int

const (
	screenList screen = iota
	screenDetail
)

// Model is the root of the UI.
type Model struct {
	client GitHub
	theme  theme.Theme
	syntax comp.Syntax
	limit  int

	store store.Store

	screen screen
	list   list.Model
	detail prview.Model
	status comp.StatusBar
	toasts comp.Toasts
	help   help.Model

	// notice reports a recoverable config problem, like a theme name that isn't
	// registered. Silently falling back reads as "my config is ignored".
	notice   string
	showHelp bool

	// refreshing is the sections the last r press actually started. A refresh
	// returns the same rows more often than not, so the toast is the only sign
	// it happened, and it has to report the sections it fetched rather than
	// every section configured: store.Begin refuses one already in flight.
	refreshing []int

	width  int
	height int
}

// New builds the root model over the configured PR sections.
func New(cfg *config.Config, client GitHub) Model {
	th, ok := theme.Get(cfg.Theme)

	// The syntax palette is a separate question from the chrome's. A theme
	// names the Chroma style that matches it, and config overrides that for a
	// theme with no counterpart.
	syntaxName := cmp.Or(cfg.SyntaxTheme, th.Syntax)
	syntax, syntaxOK := comp.NewSyntax(syntaxName)

	h := help.New()
	h.Styles = helpStyles(th)

	m := Model{
		client: client,
		theme:  th,
		syntax: syntax,
		limit:  cfg.Defaults.PRsLimit,
		store:  store.New(cfg.PRSections),
		list:   list.New(th),
		status: comp.NewStatusBar(th),
		help:   h,
	}
	// Init fetches every section, and a command runs off the update loop where
	// it cannot mark anything, so the store is put in that state here.
	m.store.BeginAll()
	m.list.SetSections(m.store.Sections())

	switch {
	case !ok:
		m.notice = fmt.Sprintf("Unknown theme %q, using %s. Known: %s",
			cfg.Theme, th.Name, strings.Join(theme.Names(), ", "))
	case !syntaxOK:
		m.notice = fmt.Sprintf("Unknown syntax theme %q, using Chroma's default. Known: %s",
			syntaxName, strings.Join(comp.SyntaxNames(), ", "))
	}
	return m
}

// Init starts the list and fetches every section. tea.Batch runs its commands
// concurrently, which is the whole of the concurrency here: no goroutine of
// ours touches the model.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.list.Init()}
	for i, section := range m.store.Sections() {
		cmds = append(cmds, m.fetchSection(i, section.Filters))
	}
	return tea.Batch(cmds...)
}

func (m Model) fetchSection(index int, query string) tea.Cmd {
	client, limit := m.client, m.limit
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		res, err := client.SearchPullRequests(ctx, query, limit)
		if err != nil {
			return sectionFailedMsg{index: index, err: err}
		}
		return sectionFetchedMsg{index: index, res: res}
	}
}

// refresh refetches every section that is not already on its way. The tab
// counts are on screen too, so a refresh that left them as they were would be
// making only part of the frame true.
func (m Model) refresh() (tea.Model, tea.Cmd) {
	sections := m.store.Sections()

	var cmds []tea.Cmd
	started := make([]int, 0, len(sections))
	for i, section := range sections {
		if !m.store.Begin(i) {
			continue
		}
		started = append(started, i)
		cmds = append(cmds, m.fetchSection(i, section.Filters))
	}
	// Every section is already on its way; the refresh is the one running.
	if len(cmds) == 0 {
		return m, nil
	}

	m.refreshing = started
	m.list.SetSections(m.store.Sections())
	return m, tea.Batch(append(cmds, m.list.Init())...)
}

// sectionSettled pushes the new snapshot down and, once the sections a refresh
// started have all landed, reports it. Waiting on the whole store instead would
// hold the toast behind a section the refresh never touched.
func (m Model) sectionSettled() (tea.Model, tea.Cmd) {
	sections := m.store.Sections()
	m.list.SetSections(sections)

	if len(m.refreshing) == 0 || stillLoading(sections, m.refreshing) {
		return m, nil
	}

	kind, text := refreshSummary(sections, m.refreshing)
	m.refreshing = nil
	return m, m.toasts.Show(kind, text)
}

func stillLoading(sections []store.Section, indices []int) bool {
	for _, i := range indices {
		if sections[i].Status == store.StatusLoading {
			return true
		}
	}
	return false
}

// open puts the detail screen up over whatever the store already holds for this
// pull request, then fetches anyway. A pull request opened before paints on the
// first frame; the refetch swaps in behind it, and SetContent keeps the scroll
// position, so nothing moves under the reader.
func (m Model) open(pr gh.PullRequest) (tea.Model, tea.Cmd) {
	m.detail = prview.New(m.theme, pr, m.detail.Rail(), m.syntax)
	m.screen = screenDetail

	var cmds []tea.Cmd
	if m.store.BeginDetail(pr.ID) {
		cmds = append(cmds, m.fetchDetail(pr.ID, pr.HeadRefName))
	}
	// Init arms this screen's own spinner chain, and the screen is new on every
	// open. Arming it with the fetch instead would leave it frozen on a reopen
	// while the first request is still out: BeginDetail refuses that one, and
	// the old chain's ticks carry a tag the new spinner drops. It costs a tick
	// where there is nothing to wait for, which is what ends the chain anyway.
	cmds = append(cmds, m.detail.Init())

	m.detail.SetDetail(m.store.Detail(pr.ID))
	m.detail.SetFiles(m.store.Files(pr.ID))
	m.resize()
	return m, tea.Batch(cmds...)
}

// needFiles answers the screen asking for a diff it does not have. The screen
// cannot fetch, so entering the Files tab reaches the root as a message and the
// request starts here.
func (m Model) needFiles(id string) (tea.Model, tea.Cmd) {
	if m.detail.PullRequest().ID != id || !m.store.BeginFiles(id) {
		return m, nil
	}
	pr := m.detail.PullRequest()
	m.detail.SetFiles(m.store.Files(id))
	return m, tea.Batch(m.fetchFiles(id, pr.Repository, pr.Number, pr.ChangedFiles), m.detail.Init())
}

// fetchFiles carries the repository and number because the diff comes over
// REST, which addresses a pull request by path rather than by node id. The
// count is what the response is measured against to report its overflow.
func (m Model) fetchFiles(id, repo string, number, changedFiles int) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		res, err := client.PullRequestFiles(ctx, repo, number, changedFiles)
		if err != nil {
			return filesFailedMsg{id: id, err: err}
		}
		return filesFetchedMsg{id: id, res: res}
	}
}

// filesSettled pushes a diff into the screen, but only while the screen is
// still showing the pull request it was fetched for.
func (m Model) filesSettled(id string, err error) (tea.Model, tea.Cmd) {
	held := m.store.Files(id)
	if m.screen != screenDetail || m.detail.PullRequest().ID != id {
		return m, nil
	}

	m.detail.SetFiles(held)
	if err != nil && held.Loaded {
		return m, m.toasts.Show(comp.ToastError, "Could not refresh the diff for #"+strconv.Itoa(m.detail.PullRequest().Number))
	}
	return m, nil
}

// fetchDetail carries the head branch as well as the id: the query asks how far
// behind the base the branch has fallen, and it needs the name to do it.
func (m Model) fetchDetail(id, headRef string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		res, err := client.PullRequest(ctx, id, headRef)
		if err != nil {
			return detailFailedMsg{id: id, err: err}
		}
		return detailFetchedMsg{id: id, res: res}
	}
}

// detailSettled pushes a response into the screen, but only when the screen is
// still showing the pull request it was fetched for.
//
// A failure over a detail already on screen only gets a toast: the screen keeps
// what it had, so without one nothing would say the refetch happened at all.
func (m Model) detailSettled(id string, err error) (tea.Model, tea.Cmd) {
	held := m.store.Detail(id)
	if m.screen != screenDetail || m.detail.PullRequest().ID != id {
		return m, nil
	}

	m.detail.SetDetail(held)
	if err != nil && held.Loaded {
		return m, m.toasts.Show(comp.ToastError, "Could not refresh #"+strconv.Itoa(m.detail.PullRequest().Number))
	}
	return m, nil
}

// Update applies every message. Nothing else mutates the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case sectionFetchedMsg:
		// No staleness guard: store.Begin refuses a section that already has a
		// request out, so a response always belongs in the slot it names.
		m.store.Applied(msg.index, msg.res)
		return m.sectionSettled()

	case sectionFailedMsg:
		m.store.Failed(msg.index, msg.err)
		return m.sectionSettled()

	case spinner.TickMsg:
		// Both screens get every tick, and each drops the ones that are not its
		// own: comp.Spinner tags them. Delegating by focus instead would kill
		// the list's chain the moment the detail screen opened over a fetch.
		var listCmd, detailCmd tea.Cmd
		m.list, listCmd = m.list.Update(msg)
		m.detail, detailCmd = m.detail.Update(msg)
		return m, tea.Batch(listCmd, detailCmd)

	case comp.ToastExpiredMsg:
		m.toasts.Expire(msg)
		return m, nil

	case detailFetchedMsg:
		m.store.DetailApplied(msg.id, msg.res)
		return m.detailSettled(msg.id, nil)

	case detailFailedMsg:
		m.store.DetailFailed(msg.id, msg.err)
		return m.detailSettled(msg.id, msg.err)

	case filesFetchedMsg:
		m.store.FilesApplied(msg.id, msg.res)
		return m.filesSettled(msg.id, nil)

	case filesFailedMsg:
		m.store.FilesFailed(msg.id, msg.err)
		return m.filesSettled(msg.id, msg.err)

	case prview.NeedFilesMsg:
		return m.needFiles(msg.ID)

	case list.OpenMsg:
		return m.open(msg.PR)

	case list.RefreshMsg:
		return m.refresh()

	case prview.BackMsg:
		m.screen = screenList
		m.resize()
		return m, nil
	}

	return m.delegate(msg)
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Global.Quit):
		return m, tea.Quit

	case key.Matches(msg, keys.Global.Help):
		m.showHelp = !m.showHelp
		return m, nil
	}

	// While help is up it owns the keyboard, so a movement key scrolls the
	// screen underneath instead of dismissing what is covering it.
	if m.showHelp {
		if msg.String() == "esc" {
			m.showHelp = false
		}
		return m, nil
	}

	return m.delegate(msg)
}

// delegate hands a message to the screen that has focus.
func (m Model) delegate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.screen {
	case screenList:
		m.list, cmd = m.list.Update(msg)
	case screenDetail:
		m.detail, cmd = m.detail.Update(msg)
	}
	return m, cmd
}

// resize divides the frame. The status bar is fixed, the notice takes a line
// when there is one, and the active screen gets the rest.
func (m *Model) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}

	body := max(0, m.height-statusBarHeight-m.noticeHeight())
	m.status = m.status.Size(m.width)
	m.help.SetWidth(m.width)

	switch m.screen {
	case screenList:
		m.list.SetSize(m.width, body)
	case screenDetail:
		m.detail.SetSize(m.width, body)
	}
}

// noticeHeight is the line the notice takes. On a frame with room for one line
// the status bar wins it: the keys that quit matter more than a config warning.
func (m Model) noticeHeight() int {
	if m.notice == "" || m.height < 2 {
		return 0
	}
	return 1
}

func (m Model) screenView() string {
	if m.screen == screenDetail {
		return m.detail.View()
	}
	return m.list.View()
}

// View renders the model. It reads state and returns a frame; it never fetches
// or mutates. In v2 the view declares its own screen mode, so alt screen is set
// here rather than as a program option.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

func (m Model) render() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	parts := make([]string, 0, 3)
	if m.noticeHeight() > 0 {
		parts = append(parts, m.status.Render(m.noticeLine(), ""))
	}
	// A pane with no room for content renders nothing at all, and appending
	// that empty string would still cost the line it was denied.
	if body := m.screenView(); body != "" {
		parts = append(parts, body)
	}
	parts = append(parts, m.status.Render(m.statusLeft(), m.statusRight()))

	frame := strings.Join(parts, "\n")
	if !m.showHelp {
		return frame
	}
	return comp.Over(frame, comp.Modal(m.theme, "Keys", m.helpBody()), m.width, m.height)
}

// noticeLine renders the config warning in the warning color. In the chrome
// grey it reads as decoration, and a notice nobody acts on is one that failed.
func (m Model) noticeLine() string {
	return lipgloss.NewStyle().Foreground(m.theme.Warning).Render(m.notice)
}

// statusLeft carries a toast while one is showing and the keymap hints the rest
// of the time.
func (m Model) statusLeft() string {
	if !m.toasts.Empty() {
		return m.toasts.Render(m.theme)
	}
	switch m.screen {
	case screenDetail:
		return m.help.ShortHelpView(m.detail.Keys().ShortHelp())
	default:
		return m.help.ShortHelpView(m.list.Keys().ShortHelp())
	}
}

func (m Model) statusRight() string {
	right := make([]string, 0, 2)
	// Limit is zero until a response lands. Gating on it rather than on
	// Remaining is what lets an exhausted budget still read as zero.
	if rate := m.store.Rate(); rate.Limit > 0 {
		right = append(right, m.status.Budget(rate.Remaining))
	}
	right = append(right, m.status.Context(m.contextLabel()))
	return strings.Join(right, m.status.Context(" · "))
}

// contextLabel names what is on screen. The detail screen gets the repository
// as well as the number: the number alone says which pull request only if you
// already know which repository you opened it from, and the tabs past the
// conversation carry nothing else that answers it.
func (m Model) contextLabel() string {
	if m.screen != screenDetail {
		return m.list.Section().Title
	}

	pr := m.detail.PullRequest()
	label := "#" + strconv.Itoa(pr.Number)
	if pr.Repository != "" {
		label += " " + pr.Repository
	}
	return label
}

func (m Model) helpBody() string {
	groups := m.list.Keys().FullHelp()
	if m.screen == screenDetail {
		groups = m.detail.Keys().FullHelp()
	}
	return m.help.FullHelpView(refitHelp(groups, m.width-modalChrome))
}

// modalChrome is what the overlay spends on itself: two border runes and a
// space of padding either side.
const modalChrome = 4

// refitHelp re-columns the bindings to whatever the frame can carry. The help
// bubble sizes its columns from their contents and never wraps, so a set that
// is one column too wide gets sheared by the overlay rather than reflowed, and
// the modal loses its right border.
func refitHelp(groups [][]key.Binding, width int) [][]key.Binding {
	var flat []key.Binding
	widest := 0
	for _, group := range groups {
		for _, b := range group {
			flat = append(flat, b)
			if w := len(b.Help().Key) + len(b.Help().Desc) + 1; w > widest {
				widest = w
			}
		}
	}
	if len(flat) == 0 {
		return groups
	}

	// The help bubble puts a gap between columns; budget for it so the last
	// column is not the one that overflows.
	const columnGap = 4
	columns := max(1, width/(widest+columnGap))
	if columns >= len(groups) {
		return groups
	}

	rows := (len(flat) + columns - 1) / columns
	out := make([][]key.Binding, 0, columns)
	for i := 0; i < len(flat); i += rows {
		out = append(out, flat[i:min(i+rows, len(flat))])
	}
	return out
}

// helpStyles dresses the help bubble in the active theme. Its own defaults are
// fixed greys that ignore whatever palette is loaded.
func helpStyles(th theme.Theme) help.Styles {
	key := lipgloss.NewStyle().Foreground(th.Secondary)
	desc := lipgloss.NewStyle().Foreground(th.Faint)
	sep := lipgloss.NewStyle().Foreground(th.BorderFaintOrSecondary())

	return help.Styles{
		Ellipsis:       sep,
		ShortKey:       key,
		ShortDesc:      desc,
		ShortSeparator: sep,
		FullKey:        key,
		FullDesc:       desc,
		FullSeparator:  sep,
	}
}

// refreshSummary names what came back. It counts sections rather than rows,
// because sections are what a refresh covers, and only the ones it started: a
// section left in flight is not a section this refresh can claim.
func refreshSummary(sections []store.Section, started []int) (comp.ToastKind, string) {
	failed := 0
	for _, i := range started {
		if sections[i].Status == store.StatusFailed {
			failed++
		}
	}

	switch {
	case failed == 0:
		return comp.ToastSuccess, "Refreshed " + comp.Plural(len(started), "section")
	case failed == len(started):
		return comp.ToastError, "Refresh failed"
	default:
		return comp.ToastError, "Refreshed " + comp.Plural(len(started)-failed, "section") +
			", " + strconv.Itoa(failed) + " failed"
	}
}
