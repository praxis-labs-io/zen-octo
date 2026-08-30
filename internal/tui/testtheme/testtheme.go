// Package testtheme is the surface every render test paints against, named
// rather than detected: a test that queried the terminal would answer one way
// here and another in CI.
package testtheme

import (
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

var Background = lipgloss.Color("#232136")

var Surface = theme.Surface{Background: Background, Foreground: lipgloss.Color("#e0def4")}

var Theme = theme.Terminal(Surface, false)
