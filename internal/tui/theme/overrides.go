package theme

import (
	"fmt"
	"image/color"
	"sort"
	"strconv"

	"charm.land/lipgloss/v2"
)

// A key missing here is refused by name rather than ignored: a dropped color reads as a broken theme.
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

// Derivation inputs rather than tokens, which is why they are absent from setters.
const (
	backgroundKey = "background"
	foregroundKey = "foreground"
)

// Keys lists every override key, sorted.
func Keys() []string {
	keys := make([]string, 0, len(setters)+2)
	for k := range setters {
		keys = append(keys, k)
	}
	keys = append(keys, backgroundKey, foregroundKey)
	sort.Strings(keys)
	return keys
}

// Overrides is a set of optional per-token colors layered over the derived theme.
type Overrides struct {
	raw map[string]string

	// Named is a theme name found where colors were expected, tolerated from older configs.
	Named string
}

func NewOverrides(colors map[string]string, named string) Overrides {
	return Overrides{raw: colors, Named: named}
}

// Surface is the background and foreground config names, each nil where it names none.
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

// Resolve derives the theme from reported, with config's named surface
// outranking it field by field, then applies the remaining overrides.
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

// Validate reports the first unknown key or unusable value, naming it. Colors
// are hex, or an ANSI index from 0 to 255 as a bare number.
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

// Apply layers the overrides onto t, skipping any value that does not parse.
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

// Range-checked because lipgloss.Color packs an integer past 255 as RGB rather than failing.
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
