package comp

import (
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// Spinner is the loading indicator, dressed in the theme and wired the one way
// every screen wants it.
//
// Each one carries its own tag, so a root that hands every tick to every screen
// is safe: a screen advances on its own ticks and drops the rest. That is what
// lets two screens spin at once without either running at double speed.
type Spinner struct {
	theme theme.Theme
	model spinner.Model
}

// NewSpinner returns a stopped spinner. Tick starts it.
func NewSpinner(th theme.Theme) Spinner {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	sp.Style = lipgloss.NewStyle().Foreground(th.Secondary)
	return Spinner{theme: th, model: sp}
}

// Tick starts the chain.
func (s Spinner) Tick() tea.Cmd { return s.model.Tick }

// Advance moves the frame on and re-arms while there is still something in
// flight. A tick that arrives with nothing loading ends the chain rather than
// spinning over a screen that already has its answer.
func (s *Spinner) Advance(msg spinner.TickMsg, loading bool) tea.Cmd {
	if !loading {
		return nil
	}
	var cmd tea.Cmd
	s.model, cmd = s.model.Update(msg)
	return cmd
}

// Render is the glyph and what it is waiting on. An empty label is the glyph
// alone.
func (s Spinner) Render(label string) string {
	if label == "" {
		return s.model.View()
	}
	return s.model.View() + " " + lipgloss.NewStyle().Foreground(s.theme.Faint).Render(label)
}
