package app

import "charm.land/lipgloss/v2"

// testBG stands in for what a terminal would report, so New has a background to
// derive from without one of these tests reaching for the real one.
var testBG = lipgloss.Color("#232136")
