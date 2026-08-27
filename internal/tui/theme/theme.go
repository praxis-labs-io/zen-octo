// Package theme holds the colors the UI styles from. Nothing in the TUI
// hardcodes one: a color that isn't here means this struct needs a field.
//
// There is one theme and it is derived rather than written down. The hues are
// ANSI slots, so they are whatever the reader's terminal maps them to; the
// surfaces are blended from the background the terminal reports at launch, so
// they sit just above it whatever it is.
package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Theme is one palette. Optional fields are nil-able and have accessors that
// fall back, so adding a field doesn't force every derivation to be rewritten.
type Theme struct {
	// Syntax names the Chroma style code is highlighted with. Chroma ships its
	// own palettes and a diff needs far more token colors than the chrome has
	// fields, so a theme points at the one that matches rather than restating
	// it. Empty falls back to Chroma's own default.
	Syntax string

	// Text, brightest first. Subtle is text a reader still reads: a row's
	// repository line, a count beside a heading. Muted is chrome they glance
	// at: a key hint, a pane's footer, the label naming what is on screen.
	Text     color.Color
	Accent   color.Color
	Subtle   color.Color
	Muted    color.Color
	Inverted color.Color

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

	// Borders, brightest first, on the same ladder the text colors use.
	Border       color.Color
	BorderSubtle color.Color
	BorderMuted  color.Color
}

// InvertedOrText is the text color to use on top of a filled surface.
func (t Theme) InvertedOrText() color.Color {
	if t.Inverted != nil {
		return t.Inverted
	}
	return t.Text
}

// MutedOrSubtle falls back for themes that define one grey.
func (t Theme) MutedOrSubtle() color.Color {
	if t.Muted != nil {
		return t.Muted
	}
	return t.Subtle
}

// BorderSubtleOrBorder falls back for themes that define one border color.
func (t Theme) BorderSubtleOrBorder() color.Color {
	if t.BorderSubtle != nil {
		return t.BorderSubtle
	}
	return t.Border
}

// BorderMutedOrSubtle falls back through the border ladder.
func (t Theme) BorderMutedOrSubtle() color.Color {
	if t.BorderMuted != nil {
		return t.BorderMuted
	}
	return t.BorderSubtleOrBorder()
}

// The hues, as ANSI slots. Painted, a slot is whatever the terminal maps it to,
// which is the whole point: a reader's palette reaches the chrome without being
// configured. Only the low eight are taken. A terminal is free to leave 8 to 15
// undeclared or collapsed onto 0 to 7, and nothing here would be able to tell.
const (
	slotBlack = lipgloss.Black
	slotRed   = lipgloss.Red
	slotGreen = lipgloss.Green
	slotGold  = lipgloss.Yellow
	slotIris  = lipgloss.Magenta
	slotFoam  = lipgloss.Cyan
	slotWhite = lipgloss.White
	slotGrey  = lipgloss.BrightBlack
)

// SyntaxDark and SyntaxLight are the Chroma styles code is highlighted with.
// The chrome follows the terminal and code cannot: Chroma styles are truecolor
// and there is no ANSI one to reach for. Pairing them against the background is
// what stops a light terminal rendering #e6edf3 source on white.
const (
	SyntaxDark  = "github-dark"
	SyntaxLight = "github"
)

// Terminal derives the theme from the background the terminal reported, which
// is nil when nothing answered the query. transparent asks for the painted
// surfaces to be dropped, for a terminal running translucent.
func Terminal(bg color.Color, transparent bool) Theme {
	t := Theme{
		Syntax: SyntaxDark,

		// Text is the terminal's own foreground rather than a color of ours.
		// Nothing matches a reader's palette as exactly as the palette.
		Text:   lipgloss.NoColor{},
		Accent: slotIris,

		Success: slotGreen,
		Warning: slotGold,
		Error:   slotRed,
		Actor:   slotFoam,

		// Never painted. The terminal's own shows through, which is what a
		// translucent one needs and what every other one is happy with.
		Background: nil,
	}

	if bg == nil {
		// Slots are the best available guess at a grey and a border, and the
		// caveat above applies: a palette that collapsed 8 onto 0 puts muted
		// chrome on the background and there is no way to see it coming. It is
		// the fallback because there is nothing better, not because it is good.
		t.Subtle, t.Muted = slotWhite, slotGrey
		t.Border, t.BorderSubtle, t.BorderMuted = slotGrey, slotGrey, slotGrey
		t.Inverted = slotBlack
		return t
	}

	// The greys are blends where the hues are slots, and the split is
	// deliberate. A palette's identity lives in its hues. Greys are structural
	// and only have to stay legible, which a slot cannot promise and a blend off
	// the known background is by construction.
	away := contrast(bg)
	t.Subtle = mix(bg, away, 0.65)
	t.Muted = mix(bg, away, 0.45)
	t.Border = mix(bg, away, 0.30)
	t.BorderSubtle = mix(bg, away, 0.20)
	t.BorderMuted = mix(bg, away, 0.12)

	// Text drawn on top of a filled Accent, so it wants to read as the page
	// does: the background the fill was placed over.
	t.Inverted = bg

	if !isDark(bg) {
		t.Syntax = SyntaxLight
	}

	if transparent {
		return t
	}

	t.SelectedBackground = mix(bg, away, 0.10)

	// A slot's RGBA() is the canonical value, never what the terminal mapped it
	// to, so these two are a standard-green and standard-red wash over the real
	// background rather than a wash in the reader's own green and red. Painting
	// a slot follows the palette; blending one cannot. Reading the true palette
	// would take an OSC 4 query per slot, which is not worth it for a tint.
	t.AddedBackground = mix(bg, slotGreen, 0.18)
	t.RemovedBackground = mix(bg, slotRed, 0.18)

	return t
}

// mix blends ratio of b into a, per channel. Both are read at 8 bits, which is
// what a terminal takes and what keeps the arithmetic legible.
func mix(a, b color.Color, ratio float64) color.Color {
	ar, ag, ab := rgb8(a)
	br, bg, bb := rgb8(b)

	blend := func(x, y uint8) uint8 {
		return uint8(float64(x)*(1-ratio) + float64(y)*ratio)
	}
	return lipgloss.RGBColor{R: blend(ar, br), G: blend(ag, bg), B: blend(ab, bb)}
}

// contrast is the direction a shade moves in to stay visible against c: toward
// white on a dark background, toward black on a light one. It is what lets one
// set of ratios serve both without a second table.
func contrast(c color.Color) color.Color {
	if isDark(c) {
		return lipgloss.RGBColor{R: 0xff, G: 0xff, B: 0xff}
	}
	return lipgloss.RGBColor{R: 0x00, G: 0x00, B: 0x00}
}

// isDark reports whether c is dark, by perceived luminance. lipgloss has this
// and does not export it.
func isDark(c color.Color) bool {
	r, g, b := rgb8(c)
	return 0.299*float64(r)+0.587*float64(g)+0.114*float64(b) < 128
}

func rgb8(c color.Color) (uint8, uint8, uint8) {
	r, g, b, _ := c.RGBA()
	return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)
}
