package config

import (
	"fmt"

	"github.com/goccy/go-yaml"
)

// Theme is the config file's `theme` map of token names to colors, every one optional.
type Theme struct {
	Colors map[string]string

	// Named is a theme name found where colors were expected, tolerated from older configs.
	Named string
}

// UnmarshalYAML accepts the color map, or a bare theme name into Named.
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
