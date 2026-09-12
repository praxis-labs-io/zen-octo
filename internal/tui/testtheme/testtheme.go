// Package testtheme is the fixed surface render tests paint against, so no test depends on the terminal.
package testtheme

import (
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

var Background = lipgloss.Color("#232136")

var Surface = theme.Surface{Background: Background, Foreground: lipgloss.Color("#e0def4")}

var Theme = theme.Terminal(Surface, false)
