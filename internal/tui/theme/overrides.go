package theme

import (
	"fmt"
	"image/color"
	"sort"
	"strconv"

	"charm.land/lipgloss/v2"
)

// setters maps a config key to the field it writes. It is the whole of the
// override vocabulary: a key that isn't here is refused by name rather than
// ignored, because a silently dropped color reads as a theme that did not work.
var setters = map[string]func(*Theme, color.Color){
	"text":               func(t *Theme, c color.Color) { t.Text = c },
	"accent":             func(t *Theme, c color.Color) { t.Accent = c },
	"subtle":             func(t *Theme, c color.Color) { t.Subtle = c },
	"muted":              func(t *Theme, c color.Color) { t.Muted = c },
	"inverted":           func(t *Theme, c color.Color) { t.Inverted = c },
	"success":            func(t *Theme, c color.Color) { t.Success = c },
	"warning":            func(t *Theme, c color.Color) { t.Warning = c },
	"error":              func(t *Theme, c color.Color) { t.Error = c },
	"actor":              func(t *Theme, c color.Color) { t.Actor = c },
	"selectedBackground": func(t *Theme, c color.Color) { t.SelectedBackground = c },
	"addedBackground":    func(t *Theme, c color.Color) { t.AddedBackground = c },
	"removedBackground":  func(t *Theme, c color.Color) { t.RemovedBackground = c },
	"border":             func(t *Theme, c color.Color) { t.Border = c },
	"borderSubtle":       func(t *Theme, c color.Color) { t.BorderSubtle = c },
	"borderMuted":        func(t *Theme, c color.Color) { t.BorderMuted = c },
}

// backgroundKey and foregroundKey are derivation inputs rather than tokens,
// which is why they are known here and absent from setters. They say what the
// terminal is, where it could not say so itself or said so wrongly, and the
// shades, the surfaces and the syntax pairing are all built from them.
const (
	backgroundKey = "background"
	foregroundKey = "foreground"
)

// Keys lists the override vocabulary in a stable order, for the message that
// follows a key nobody recognised.
func Keys() []string {
	keys := make([]string, 0, len(setters)+2)
	for k := range setters {
		keys = append(keys, k)
	}
	keys = append(keys, backgroundKey, foregroundKey)
	sort.Strings(keys)
	return keys
}

// Overrides is a token name against a color, every one optional, layered over
// the derived theme. Pinning one color and bringing a whole palette are then the
// same feature.
//
// It takes plain strings rather than reading a file: what a config document
// looks like belongs to the package that loads one, and a package that draws
// colors has no business knowing. That is also what keeps this one a leaf.
type Overrides struct {
	raw map[string]string

	// Named is a theme name found where the colors were expected. There is one
	// theme now and it has no name, but `theme: rose-pine-moon` is on disk for
	// anyone running the last release, and refusing to start over a color
	// scheme is the wrong trade.
	Named string
}

// NewOverrides builds the set from whatever config read.
func NewOverrides(colors map[string]string, named string) Overrides {
	return Overrides{raw: colors, Named: named}
}

// Surface is what config says the terminal is, either field nil where it says
// nothing. It outranks what the terminal reported: it is written down because
// that answer was wrong, or because nothing answered at all.
func (o Overrides) Surface() Surface {
	var s Surface
	if c, ok := parse(o.raw[backgroundKey]); ok {
		s.Background = c
	}
	if c, ok := parse(o.raw[foregroundKey]); ok {
		s.Foreground = c
	}
	return s
}

// Resolve builds the theme. What config named outranks what was reported, field
// by field, and the rest of the overrides land on what is derived from the pair
// that won. The order is the whole of the point: a background written down after
// derivation would correct one field, where the shades, the surfaces and the
// syntax pairing all hang off it.
func (o Overrides) Resolve(reported Surface, transparent bool) Theme {
	named := o.Surface()
	if named.Background != nil {
		reported.Background = named.Background
	}
	if named.Foreground != nil {
		reported.Foreground = named.Foreground
	}
	return o.Apply(Terminal(reported, transparent))
}

// Validate reports the first key or value it cannot use, naming it. Colors are
// hex, or an ANSI index as a bare number the way lipgloss spells one.
func (o Overrides) Validate() error {
	for _, key := range sorted(o.raw) {
		if _, ok := setters[key]; !ok && key != backgroundKey && key != foregroundKey {
			return fmt.Errorf("theme: unknown color %q. Known: %v", key, Keys())
		}
		if _, ok := parse(o.raw[key]); !ok {
			return fmt.Errorf("theme: %s: %q is not a color. Want a hex like \"#c4a7e7\" or an ANSI index from 0 to 255",
				key, o.raw[key])
		}
	}
	return nil
}

// Apply layers the overrides onto a derived theme. Validate has already refused
// anything unusable, so anything that does not parse here is skipped rather than
// written as the absence of a color.
func (o Overrides) Apply(t Theme) Theme {
	for _, key := range sorted(o.raw) {
		set, ok := setters[key]
		if !ok {
			continue
		}
		if c, ok := parse(o.raw[key]); ok {
			set(&t, c)
		}
	}
	return t
}

// parse reads one config value: a hex, or an ANSI index as a bare number.
//
// The range check is the whole reason this is not lipgloss.Color alone. That
// one reads any integer: past 255 it packs the value as RGB, so "256" is a
// near-black #000100 rather than an error, and a negative is silently made
// positive. Both are ordinary off-by-ones against the 0-255 this documents, and
// the background is the field they land worst on, since it paints the whole app
// and every shade is derived against it.
//
// lipgloss answers unparseable text with NoColor, which means "the terminal's
// own" everywhere else here, so it is the failure rather than a value to offer.
func parse(s string) (color.Color, bool) {
	if n, err := strconv.Atoi(s); err == nil {
		if n < 0 || n > 255 {
			return nil, false
		}
		return lipgloss.Color(s), true
	}

	c := lipgloss.Color(s)
	if _, blank := c.(lipgloss.NoColor); blank {
		return nil, false
	}
	return c, true
}

func sorted(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
