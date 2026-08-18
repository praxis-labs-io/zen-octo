package theme_test

import (
	"slices"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
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
		Text:   lipgloss.Color("#ffffff"),
		Subtle: lipgloss.Color("#888888"),
		Border: lipgloss.Color("#333333"),
	}

	if got := bare.InvertedOrText(); got != bare.Text {
		t.Errorf("InvertedOrText() = %v, want Text when Inverted is unset", got)
	}
	if got := bare.MutedOrSubtle(); got != bare.Subtle {
		t.Errorf("MutedOrSubtle() = %v, want Subtle when Muted is unset", got)
	}
	if got := bare.BorderSubtleOrBorder(); got != bare.Border {
		t.Errorf("BorderSubtleOrBorder() = %v, want Border when unset", got)
	}
	if got := bare.BorderMutedOrSubtle(); got != bare.Border {
		t.Errorf("BorderMutedOrSubtle() = %v, want it to fall through to Border", got)
	}
}

func TestSetOptionalFieldsWin(t *testing.T) {
	full := theme.RosePineMoon

	if got := full.InvertedOrText(); got != full.Inverted {
		t.Errorf("InvertedOrText() = %v, want Inverted when it is set", got)
	}
	if got := full.MutedOrSubtle(); got != full.Muted {
		t.Errorf("MutedOrSubtle() = %v, want Muted when it is set", got)
	}
	if got := full.BorderMutedOrSubtle(); got != full.BorderMuted {
		t.Errorf("BorderMutedOrSubtle() = %v, want BorderMuted when it is set", got)
	}
}
