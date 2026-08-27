package app_test

import (
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

// testBG stands in for what a terminal would report. It is named rather than
// detected: a test that queried the terminal would answer one way here and
// another in CI.
var testBG = lipgloss.Color("#232136")

// testTheme is the shipped theme over that background, so a test asserts the
// colors a reader is actually given rather than a palette nothing runs.
var testTheme = theme.Terminal(testBG, false)
