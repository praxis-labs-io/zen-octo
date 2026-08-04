// Package app holds the root Bubble Tea model. It owns the screens, divides the
// frame between them and the status bar, and handles the keys that answer
// whatever has focus. All model mutation happens in Update; commands do the
// asynchronous work and deliver typed messages back.
package app

import (
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
	"github.com/zen-octo/zen-octo/internal/tui/comp"
	"github.com/zen-octo/zen-octo/internal/tui/keys"
	"github.com/zen-octo/zen-octo/internal/tui/list"
	"github.com/zen-octo/zen-octo/internal/tui/prview"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// PRSearcher is the slice of the GitHub client this model needs. Declaring it
// here rather than in the gh package is what lets tests drive the UI without a
// network.
type PRSearcher interface {
	SearchPullRequests(ctx context.Context, query string, limit int) (gh.SearchResult, error)
}

type prsFetchedMsg struct {
	prs   []gh.PullRequest
	rate  gh.RateLimit
	query string

	// announce marks a fetch the user asked for. A refresh that returns the
	// same rows moves nothing on screen, so the toast is the only way to tell
	// it happened. A first load and a section switch are both self-evident.
	announce bool
}

type prsFailedMsg struct {
	err   error
	query string
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
	client PRSearcher
	theme  theme.Theme
	limit  int

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
	rate     gh.RateLimit
	query    string

	width  int
	height int
}

// New builds the root model over the configured PR sections.
func New(cfg *config.Config, client PRSearcher) Model {
	th, ok := theme.Get(cfg.Theme)

	h := help.New()
	h.Styles = helpStyles(th)

	m := Model{
		client: client,
		theme:  th,
		limit:  cfg.Defaults.PRsLimit,
		list:   list.New(th, cfg.PRSections),
		status: comp.NewStatusBar(th),
		help:   h,
		query:  cfg.PRSections[0].Filters,
	}
	if !ok {
		m.notice = fmt.Sprintf("Unknown theme %q, using %s. Known: %s",
			cfg.Theme, th.Name, strings.Join(theme.Names(), ", "))
	}
	return m
}

// Init starts the list and the first fetch.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.list.Init(), m.fetchPRs(m.query, false))
}

func (m Model) fetchPRs(query string, announce bool) tea.Cmd {
	client, limit := m.client, m.limit
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		res, err := client.SearchPullRequests(ctx, query, limit)
		if err != nil {
			return prsFailedMsg{err: err, query: query}
		}
		return prsFetchedMsg{prs: res.PullRequests, rate: res.RateLimit, query: query, announce: announce}
	}
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

	case prsFetchedMsg:
		// A section switch can land after a fetch it did not ask for. Dropping
		// the stale one keeps the wrong rows off the new tab.
		if msg.query != m.query {
			return m, nil
		}
		m.rate = msg.rate
		m.list.SetPullRequests(msg.prs)
		if !msg.announce {
			return m, nil
		}
		return m, m.toasts.Show(comp.ToastSuccess, loadedSummary(len(msg.prs)))

	case prsFailedMsg:
		// Same guard as the success path. A timeout from a section the user
		// already left would otherwise replace rows that loaded fine.
		if msg.query != m.query {
			return m, nil
		}
		m.list.SetError(msg.err)
		return m, nil

	case spinner.TickMsg:
		// The list owns the only spinner, and the chain re-arms from its own
		// Update. Delegating by focus kills it the moment the detail screen
		// opens over a fetch in flight.
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd

	case comp.ToastExpiredMsg:
		m.toasts.Expire(msg)
		return m, nil

	case list.OpenMsg:
		m.detail = prview.New(m.theme, msg.PR, m.detail.Rail())
		m.screen = screenDetail
		m.resize()
		return m, nil

	case list.SectionChangedMsg:
		m.query = msg.Section.Filters
		return m, m.fetchPRs(m.query, false)

	case list.RefreshMsg:
		return m, m.fetchPRs(m.query, true)

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
	if m.rate.Limit > 0 {
		right = append(right, m.status.Budget(m.rate.Remaining))
	}
	right = append(right, m.status.Context(m.contextLabel()))
	return strings.Join(right, m.status.Context(" · "))
}

func (m Model) contextLabel() string {
	if m.screen == screenDetail {
		return "#" + strconv.Itoa(m.detail.PullRequest().Number)
	}
	return m.list.Section().Title
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

func loadedSummary(n int) string {
	if n == 1 {
		return "Loaded 1 pull request"
	}
	return "Loaded " + strconv.Itoa(n) + " pull requests"
}
