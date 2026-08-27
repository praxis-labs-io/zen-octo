package prview_test

import (
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

// testTheme is the shipped theme over a fixed background, so a test asserts the
// colors a reader is actually given rather than a palette nothing runs. The
// background is named rather than detected: a test that queried the terminal
// would answer one way here and another in CI.
// testSurface is a background with the foreground a dark terminal would report
// beside it, since the shades travel toward it.
var testSurface = theme.Surface{
	Background: lipgloss.Color("#232136"),
	Foreground: lipgloss.Color("#e0def4"),
}

var testTheme = theme.Terminal(testSurface, false)
