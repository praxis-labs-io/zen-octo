package app_test

import (
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

// testBG stands in for what a terminal would report. It is named rather than
// detected: a test that queried the terminal would answer one way here and
// another in CI.
var testBG = lipgloss.Color("#232136")

// testSurface is that background with the foreground beside it, since the
// shades travel toward the foreground rather than toward pure white.
var testSurface = theme.Surface{Background: testBG, Foreground: lipgloss.Color("#e0def4")}

// testTheme is the shipped theme over that surface, so a test asserts the
// colors a reader is actually given rather than a palette nothing runs.
var testTheme = theme.Terminal(testSurface, false)
