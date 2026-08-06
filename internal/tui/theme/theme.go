// Package theme holds the color palettes the UI styles from. Nothing in the
// TUI hardcodes a color: a color that isn't here means this struct needs a
// field.
package theme

import (
	"image/color"
	"sort"

	"charm.land/lipgloss/v2"
)

// Theme is one palette. Optional fields are nil-able and have accessors that
// fall back, so adding a field doesn't force every theme to be rewritten.
type Theme struct {
	Name string

	// Syntax names the Chroma style code is highlighted with. Chroma ships its
	// own palettes and a diff needs far more token colors than the chrome has
	// fields, so a theme points at the one that matches rather than restating
	// it. Empty falls back to Chroma's own default.
	Syntax string

	// Text
	Primary   color.Color
	Secondary color.Color
	Faint     color.Color
	Inverted  color.Color

	// Semantic
	Success color.Color
	Warning color.Color
	Error   color.Color
	Actor   color.Color

	// Surfaces. A nil Background means "leave the terminal's own background
	// alone", which is what keeps transparency working.
	Background         color.Color
	SelectedBackground color.Color

	// Diff surfaces. A changed line is read as a block, not a character at a
	// time, and a marker column alone does not carry that. They are tints of
	// Success and Error over the base rather than the colors themselves: a
	// filled row at full strength buries the code sitting on it.
	AddedBackground   color.Color
	RemovedBackground color.Color

	// Borders
	Border          color.Color
	BorderSecondary color.Color
	BorderFaint     color.Color
}

// InvertedOrPrimary is the text color to use on top of a filled surface.
func (t Theme) InvertedOrPrimary() color.Color {
	if t.Inverted != nil {
		return t.Inverted
	}
	return t.Primary
}

// BorderSecondaryOrBorder falls back for themes that define one border color.
func (t Theme) BorderSecondaryOrBorder() color.Color {
	if t.BorderSecondary != nil {
		return t.BorderSecondary
	}
	return t.Border
}

// BorderFaintOrSecondary falls back through the border ladder.
func (t Theme) BorderFaintOrSecondary() color.Color {
	if t.BorderFaint != nil {
		return t.BorderFaint
	}
	return t.BorderSecondaryOrBorder()
}

// RosePineMoon is the default. The values match the gh-dash palette Drew
// already runs, so the two look the same side by side.
var RosePineMoon = Theme{
	Name:               "rose-pine-moon",
	Syntax:             "rose-pine-moon",
	Primary:            lipgloss.Color("#e0def4"),
	Secondary:          lipgloss.Color("#c4a7e7"),
	Faint:              lipgloss.Color("#a5a1bc"),
	Inverted:           lipgloss.Color("#232136"),
	Success:            lipgloss.Color("#9ccfd8"),
	Warning:            lipgloss.Color("#f6c177"),
	Error:              lipgloss.Color("#eb6f92"),
	Actor:              lipgloss.Color("#ea9a97"),
	Background:         nil,
	SelectedBackground: lipgloss.Color("#2a283e"),
	AddedBackground:    lipgloss.Color("#26383c"),
	RemovedBackground:  lipgloss.Color("#3c2635"),
	Border:             lipgloss.Color("#56526e"),
	BorderSecondary:    lipgloss.Color("#44415a"),
	BorderFaint:        lipgloss.Color("#393552"),
}

// Default names the theme used when config asks for one that doesn't exist.
const Default = "rose-pine-moon"

var registry = map[string]Theme{
	RosePineMoon.Name: RosePineMoon,
}

// Get returns the named theme. An unknown name yields the default and false,
// so a typo in config degrades to a working UI instead of a crash.
func Get(name string) (Theme, bool) {
	t, ok := registry[name]
	if !ok {
		return registry[Default], false
	}
	return t, true
}

// Names lists the registered themes in a stable order.
func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
