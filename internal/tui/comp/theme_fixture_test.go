package comp_test

import (
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

// testTheme is the shipped theme over a fixed background, so a test asserts the
// colors a reader is actually given rather than a palette nothing runs. The
// background is named rather than detected: a test that queried the terminal
// would answer one way here and another in CI.
var testTheme = theme.Terminal(lipgloss.Color("#232136"), false)
