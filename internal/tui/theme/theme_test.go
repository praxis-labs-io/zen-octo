package theme_test

import (
	"slices"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

func TestGetKnownTheme(t *testing.T) {
	got, ok := theme.Get("rose-pine-moon")
	if !ok {
		t.Fatal("Get(\"rose-pine-moon\") ok = false, want true")
	}
	if got.Name != "rose-pine-moon" {
		t.Errorf("Name = %q, want rose-pine-moon", got.Name)
	}
}

func TestGetUnknownThemeFallsBackToDefault(t *testing.T) {
	got, ok := theme.Get("no-such-theme")
	if ok {
		t.Error("Get(\"no-such-theme\") ok = true, want false")
	}
	if got.Name != theme.Default {
		t.Errorf("Name = %q, want the default %q", got.Name, theme.Default)
	}
}

func TestNamesIncludesRegisteredThemes(t *testing.T) {
	names := theme.Names()
	if !slices.Contains(names, "rose-pine-moon") {
		t.Errorf("Names() = %v, want it to contain rose-pine-moon", names)
	}
	if !slices.IsSorted(names) {
		t.Errorf("Names() = %v, want a stable sorted order", names)
	}
}

func TestOptionalFieldsFallBack(t *testing.T) {
	bare := theme.Theme{
		Primary: lipgloss.Color("#ffffff"),
		Border:  lipgloss.Color("#333333"),
	}

	if got := bare.InvertedOrPrimary(); got != bare.Primary {
		t.Errorf("InvertedOrPrimary() = %v, want Primary when Inverted is unset", got)
	}
	if got := bare.BorderSecondaryOrBorder(); got != bare.Border {
		t.Errorf("BorderSecondaryOrBorder() = %v, want Border when unset", got)
	}
	if got := bare.BorderFaintOrSecondary(); got != bare.Border {
		t.Errorf("BorderFaintOrSecondary() = %v, want it to fall through to Border", got)
	}
}

func TestSetOptionalFieldsWin(t *testing.T) {
	full := theme.RosePineMoon

	if got := full.InvertedOrPrimary(); got != full.Inverted {
		t.Errorf("InvertedOrPrimary() = %v, want Inverted when it is set", got)
	}
	if got := full.BorderFaintOrSecondary(); got != full.BorderFaint {
		t.Errorf("BorderFaintOrSecondary() = %v, want BorderFaint when it is set", got)
	}
}
