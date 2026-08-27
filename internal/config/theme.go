package config

import (
	"fmt"

	"github.com/goccy/go-yaml"
)

// Theme is what the config file carries under `theme`: a color name against a
// value, every one optional.
//
// It stays strings here and is turned into a palette by the package that draws
// one. Reading a document is what this package is for, and the shape of a
// document is the one thing the rendering side must not have to know: a theme
// loaded from a file later is this type's problem and not that one's.
type Theme struct {
	Colors map[string]string

	// Named is a theme name found where the colors were expected. There is one
	// theme now and it has no name, but `theme: rose-pine-moon` is on disk for
	// anyone running the last release, and a scalar unmarshalled into a map is
	// a hard parse error. Refusing to start over a color scheme is the wrong
	// trade, so it is kept and reported instead.
	Named string
}

// UnmarshalYAML takes the map, and tolerates the scalar that used to be there.
func (t *Theme) UnmarshalYAML(b []byte) error {
	var colors map[string]string
	if err := yaml.Unmarshal(b, &colors); err == nil {
		t.Colors = colors
		return nil
	}

	var name string
	if err := yaml.Unmarshal(b, &name); err != nil {
		return fmt.Errorf("theme: want a map of color overrides, got %s", b)
	}
	t.Named = name
	return nil
}
