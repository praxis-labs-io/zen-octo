// Package app holds the root Bubble Tea model. It owns view routing, sizing,
// and the global keymap. All model mutation happens in Update; commands do the
// asynchronous work and deliver typed messages back.
package app

import (
	"context"

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

// Model is the root of the UI.
type Model struct {
	client  PRSearcher
	theme   theme.Theme
	section config.Section
	limit   int

	prs     []gh.PullRequest
	err     error
	loading bool
	cursor  int
	spinner spinner.Model

	width  int
	height int
}

// New builds the root model. M0 renders the first configured PR section; the
// rest arrive with the section tabs in M1.
func New(cfg *config.Config, client PRSearcher) Model {
	th, _ := theme.Get(cfg.Theme)

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	sp.Style = lipgloss.NewStyle().Foreground(th.Secondary)

	return Model{
		client:  client,
		theme:   th,
		section: cfg.PRSections[0],
		limit:   cfg.Defaults.PRsLimit,
		loading: true,
		spinner: sp,
	}
}

// Init starts the spinner and the first fetch.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchPRs())
}

func (m Model) fetchPRs() tea.Cmd {
	client, query, limit := m.client, m.section.Filters, m.limit
	return func() tea.Msg {
		prs, err := client.SearchPullRequests(context.Background(), query, limit)
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
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case prsFetchedMsg:
		m.loading = false
		m.err = nil
		m.prs = msg.prs
		m.cursor = 0
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
		return m, nil

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case "g", "home":
		m.cursor = 0
		return m, nil

	case "G", "end":
		if len(m.prs) > 0 {
			m.cursor = len(m.prs) - 1
		}
		return m, nil

	case "r":
		if m.loading {
			return m, nil
		}
		m.loading = true
		return m, tea.Batch(m.spinner.Tick, m.fetchPRs())
	}

	return m, nil
}

// View renders the model. It reads state and returns a frame; it never fetches
// or mutates. In v2 the view declares its own screen mode, so alt screen is set
// here rather than as a program option.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}
