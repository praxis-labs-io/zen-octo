// Package app holds the root Bubble Tea model. It owns view routing, sizing,
// and the global keymap. All model mutation happens in Update; commands do the
// asynchronous work and deliver typed messages back.
package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/config"
	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// PRSearcher is the slice of the GitHub client this model needs. Declaring it
// here rather than in the gh package is what lets tests drive the UI without a
// network.
type PRSearcher interface {
	SearchPullRequests(ctx context.Context, query string, limit int) ([]gh.PullRequest, error)
}

type prsFetchedMsg struct{ prs []gh.PullRequest }

type prsFailedMsg struct{ err error }

// fetchTimeout bounds a single request. Without it a half-open socket leaves
// the UI spinning with no error and no way out but quitting.
const fetchTimeout = 30 * time.Second

// Model is the root of the UI.
type Model struct {
	client  PRSearcher
	theme   theme.Theme
	section config.Section
	limit   int

	// notice reports a recoverable config problem, like a theme name that
	// isn't registered. Silently falling back reads as "my config is ignored".
	notice string

	prs     []gh.PullRequest
	err     error
	loading bool
	cursor  int
	offset  int
	spinner spinner.Model

	width  int
	height int
}

// New builds the root model. M0 renders the first configured PR section; the
// rest arrive with the section tabs in M1.
func New(cfg *config.Config, client PRSearcher) Model {
	th, ok := theme.Get(cfg.Theme)

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	sp.Style = lipgloss.NewStyle().Foreground(th.Secondary)

	m := Model{
		client:  client,
		theme:   th,
		section: cfg.PRSections[0],
		limit:   cfg.Defaults.PRsLimit,
		loading: true,
		spinner: sp,
	}
	if !ok {
		m.notice = fmt.Sprintf("Unknown theme %q, using %s. Known: %s",
			cfg.Theme, th.Name, strings.Join(theme.Names(), ", "))
	}
	return m
}

// Init starts the spinner and the first fetch.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchPRs())
}

func (m Model) fetchPRs() tea.Cmd {
	client, query, limit := m.client, m.section.Filters, m.limit
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		prs, err := client.SearchPullRequests(ctx, query, limit)
		if err != nil {
			return prsFailedMsg{err: err}
		}
		return prsFetchedMsg{prs: prs}
	}
}

// Update applies every message. Nothing else mutates the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampScroll()
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case prsFetchedMsg:
		m.loading = false
		m.err = nil
		m.restoreCursor(msg.prs)
		m.prs = msg.prs
		m.clampScroll()
		return m, nil

	case prsFailedMsg:
		m.loading = false
		m.err = msg.err
		return m, nil

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

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "j", "down":
		if m.cursor < len(m.prs)-1 {
			m.cursor++
		}
		m.clampScroll()
		return m, nil

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
		m.clampScroll()
		return m, nil

	case "g", "home":
		m.cursor = 0
		m.clampScroll()
		return m, nil

	case "G", "end":
		if len(m.prs) > 0 {
			m.cursor = len(m.prs) - 1
		}
		m.clampScroll()
		return m, nil

	case "r":
		if m.loading {
			return m, nil
		}
		m.loading = true
		// Clearing the error matters: render checks it before loading, so a
		// retry would otherwise redraw the old failure with no spinner.
		m.err = nil
		return m, tea.Batch(m.spinner.Tick, m.fetchPRs())
	}

	return m, nil
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

// clampScroll keeps the cursor inside the visible window. It is the minimal
// stand-in for a real viewport; ZNO-10 replaces it.
func (m *Model) clampScroll() {
	rows := m.visibleRows()
	if rows <= 0 || len(m.prs) == 0 {
		m.offset = 0
		return
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+rows {
		m.offset = m.cursor - rows + 1
	}
	m.offset = max(0, min(m.offset, max(0, len(m.prs)-rows)))
}

// View renders the model. It reads state and returns a frame; it never fetches
// or mutates. In v2 the view declares its own screen mode, so alt screen is set
// here rather than as a program option.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}
