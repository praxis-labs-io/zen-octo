package comp

import (
	"image/color"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

// Spinner is the themed loading indicator. Each carries its own tag and ignores other spinners' ticks.
type Spinner struct {
	theme theme.Theme
	model spinner.Model
}

// NewSpinner returns a stopped spinner. Tick starts it.
func NewSpinner(th theme.Theme) Spinner {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	sp.Style = lipgloss.NewStyle().Foreground(th.Accent)
	return Spinner{theme: th, model: sp}
}

func (s Spinner) Tick() tea.Cmd { return s.model.Tick }

// Advance moves the frame on and re-arms only while loading; a tick from another spinner is ignored.
func (s *Spinner) Advance(msg spinner.TickMsg, loading bool) tea.Cmd {
	if !loading {
		return nil
	}
	var cmd tea.Cmd
	s.model, cmd = s.model.Update(msg)
	return cmd
}

// Render is the glyph followed by label, or the glyph alone for an empty label.
func (s Spinner) Render(label string) string { return s.render(label, s.theme.Subtle) }

// RenderAccent is Render with the label in the accent color, for the status bar.
func (s Spinner) RenderAccent(label string) string { return s.render(label, s.theme.Accent) }

func (s Spinner) render(label string, c color.Color) string {
	if label == "" {
		return s.model.View()
	}
	return s.model.View() + " " + lipgloss.NewStyle().Foreground(c).Render(label)
}
