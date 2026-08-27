package app

import (
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

// testSurface stands in for what a terminal would report, so New has something
// to derive from without one of these tests reaching for the real one.
var testSurface = theme.Surface{
	Background: lipgloss.Color("#232136"),
	Foreground: lipgloss.Color("#e0def4"),
}
