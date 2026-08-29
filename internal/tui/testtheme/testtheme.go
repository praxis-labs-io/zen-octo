// Package testtheme is the surface every render test paints against. It is one
// declaration rather than a copy per package, so a test cannot drift onto
// colors the tests one package over never see.
package testtheme

import (
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

// Background stands in for what a terminal would report. It is named rather
// than detected: a test that queried the terminal would answer one way here and
// another in CI.
var Background = lipgloss.Color("#232136")

// Surface is that background with the foreground a dark terminal would report
// beside it, since the shades travel toward the foreground rather than toward
// pure white.
var Surface = theme.Surface{Background: Background, Foreground: lipgloss.Color("#e0def4")}

// Theme is the shipped theme over that surface, so a test asserts the colors a
// reader is actually given rather than a palette nothing runs.
var Theme = theme.Terminal(Surface, false)

// Transparent is the same surface asked to paint nothing, which is what
// transparent: true and a terminal that answered no background both get. A test
// that only ever runs Theme cannot see a cursor go missing there.
var Transparent = theme.Terminal(Surface, true)
