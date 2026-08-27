package theme_test

import (
	"image/color"
	"testing"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/goccy/go-yaml"

	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

var (
	darkBG  = lipgloss.Color("#232136")
	lightBG = lipgloss.Color("#faf4ed")
)

// rgb reads a color the way a terminal will, so a test compares what is painted
// rather than how it was spelled.
func rgb(c color.Color) (int, int, int) {
	r, g, b, _ := c.RGBA()
	return int(r >> 8), int(g >> 8), int(b >> 8)
}

func luma(c color.Color) float64 {
	r, g, b := rgb(c)
	return 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
}

func TestHuesStayASlot(t *testing.T) {
	// A slot has to reach the terminal as a slot. Flattened to RGB it stops
	// following the reader's palette, which is the whole of the feature.
	for _, tc := range []struct {
		name string
		got  color.Color
		want xansi.BasicColor
	}{
		{"Accent", theme.Terminal(darkBG, false).Accent, lipgloss.Magenta},
		{"Success", theme.Terminal(darkBG, false).Success, lipgloss.Green},
		{"Warning", theme.Terminal(darkBG, false).Warning, lipgloss.Yellow},
		{"Error", theme.Terminal(darkBG, false).Error, lipgloss.Red},
		{"Actor", theme.Terminal(darkBG, false).Actor, lipgloss.Cyan},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tc.got.(xansi.BasicColor)
			if !ok {
				t.Fatalf("%s = %T, want a xansi.BasicColor", tc.name, tc.got)
			}
			if got != tc.want {
				t.Errorf("%s = slot %d, want slot %d", tc.name, got, tc.want)
			}
		})
	}
}

func TestHuesDoNotFollowTheBackground(t *testing.T) {
	// The hues are the reader's, so a light terminal and a dark one get the
	// same slots. Only the shades move.
	dark, light := theme.Terminal(darkBG, false), theme.Terminal(lightBG, false)
	if dark.Accent != light.Accent {
		t.Errorf("Accent = %v on dark and %v on light, want the same slot", dark.Accent, light.Accent)
	}
	if dark.Error != light.Error {
		t.Errorf("Error = %v on dark and %v on light, want the same slot", dark.Error, light.Error)
	}
}

func TestTextIsTheTerminalsOwn(t *testing.T) {
	if _, ok := theme.Terminal(darkBG, false).Text.(lipgloss.NoColor); !ok {
		t.Errorf("Text = %v, want NoColor so the terminal's own foreground is used", theme.Terminal(darkBG, false).Text)
	}
}

func TestBackgroundIsNeverPainted(t *testing.T) {
	// Transparency is not a mode for this one field. Painting it would fill a
	// translucent terminal in whatever it reported a moment earlier.
	for _, tc := range []struct {
		name string
		th   theme.Theme
	}{
		{"dark", theme.Terminal(darkBG, false)},
		{"light", theme.Terminal(lightBG, false)},
		{"undetected", theme.Terminal(nil, false)},
		{"transparent", theme.Terminal(darkBG, true)},
	} {
		if tc.th.Background != nil {
			t.Errorf("%s: Background = %v, want nil", tc.name, tc.th.Background)
		}
	}
}

func TestShadesLightenADarkBackground(t *testing.T) {
	th := theme.Terminal(darkBG, false)
	base := luma(darkBG)

	// The ladder, dimmest first. Each has to clear the background it sits on
	// and each has to clear the one below it, or the weights collapse.
	for _, step := range []struct {
		name string
		c    color.Color
	}{
		{"BorderMuted", th.BorderMuted},
		{"BorderSubtle", th.BorderSubtle},
		{"Border", th.Border},
		{"Muted", th.Muted},
		{"Subtle", th.Subtle},
	} {
		if got := luma(step.c); got <= base {
			t.Errorf("%s luma = %.1f, want brighter than the background's %.1f", step.name, got, base)
		}
		base = luma(step.c)
	}
}

func TestShadesDarkenALightBackground(t *testing.T) {
	th := theme.Terminal(lightBG, false)
	base := luma(lightBG)

	for _, step := range []struct {
		name string
		c    color.Color
	}{
		{"BorderMuted", th.BorderMuted},
		{"BorderSubtle", th.BorderSubtle},
		{"Border", th.Border},
		{"Muted", th.Muted},
		{"Subtle", th.Subtle},
	} {
		if got := luma(step.c); got >= base {
			t.Errorf("%s luma = %.1f, want darker than the background's %.1f", step.name, got, base)
		}
		base = luma(step.c)
	}
}

func TestDiffTintsLeanTheirOwnWay(t *testing.T) {
	th := theme.Terminal(darkBG, false)
	br, bg, bb := rgb(darkBG)

	ar, ag, ab := rgb(th.AddedBackground)
	if ag <= bg || ag <= ar || ag <= ab {
		t.Errorf("AddedBackground = %d,%d,%d over a %d,%d,%d base, want green to lead", ar, ag, ab, br, bg, bb)
	}

	rr, rg, rb := rgb(th.RemovedBackground)
	if rr <= br || rr <= rg || rr <= rb {
		t.Errorf("RemovedBackground = %d,%d,%d over a %d,%d,%d base, want red to lead", rr, rg, rb, br, bg, bb)
	}
}

func TestTintsStayUnderTheCode(t *testing.T) {
	// A tint groups a run of changed lines. At full strength it buries the
	// source sitting on it, so it has to stay nearer the background than the
	// color it leans toward.
	th := theme.Terminal(darkBG, false)
	base, added := luma(darkBG), luma(th.AddedBackground)
	if added-base > luma(lipgloss.Green)-base {
		t.Errorf("AddedBackground luma = %.1f, want far nearer the background's %.1f than green's %.1f",
			added, base, luma(lipgloss.Green))
	}
}

func TestNoBackgroundPaintsNoSurface(t *testing.T) {
	// A guessed surface is worse than none. Slot 0 is the background on a great
	// many dark palettes, so a selection painted in it is invisible exactly
	// where it was needed; the bar glyph and the markers carry it instead.
	th := theme.Terminal(nil, false)
	for _, tc := range []struct {
		name string
		c    color.Color
	}{
		{"SelectedBackground", th.SelectedBackground},
		{"AddedBackground", th.AddedBackground},
		{"RemovedBackground", th.RemovedBackground},
	} {
		if tc.c != nil {
			t.Errorf("%s = %v with no background detected, want nil", tc.name, tc.c)
		}
	}

	// Borders are drawn runes rather than fills, so they still have a color to
	// be. A slot is the only thing left to reach for.
	if _, ok := th.Border.(xansi.BasicColor); !ok {
		t.Errorf("Border = %T, want a slot when nothing was detected", th.Border)
	}
}

func TestTransparentDropsTheSurfacesAndKeepsTheRest(t *testing.T) {
	th := theme.Terminal(darkBG, true)

	for _, tc := range []struct {
		name string
		c    color.Color
	}{
		{"SelectedBackground", th.SelectedBackground},
		{"AddedBackground", th.AddedBackground},
		{"RemovedBackground", th.RemovedBackground},
	} {
		if tc.c != nil {
			t.Errorf("%s = %v under transparent, want nil", tc.name, tc.c)
		}
	}

	// The shades are not surfaces and go on being derived: a translucent
	// terminal still has a background, it just must not be painted over.
	opaque := theme.Terminal(darkBG, false)
	if th.Subtle != opaque.Subtle || th.Border != opaque.Border {
		t.Error("transparent changed the derived shades, want only the painted surfaces dropped")
	}
}

func TestSyntaxIsPairedAgainstTheBackground(t *testing.T) {
	// Chroma has no ANSI style, so code is the one thing that cannot follow the
	// palette. Pairing is what stops a light terminal drawing dark-theme source.
	for _, tc := range []struct {
		name string
		bg   color.Color
		want string
	}{
		{"dark", darkBG, theme.SyntaxDark},
		{"light", lightBG, theme.SyntaxLight},
		{"undetected", nil, theme.SyntaxDark},
	} {
		if got := theme.Terminal(tc.bg, false).Syntax; got != tc.want {
			t.Errorf("%s: Syntax = %q, want %q", tc.name, got, tc.want)
		}
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
	full := theme.Terminal(darkBG, false)

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

func overrides(t *testing.T, doc string) theme.Overrides {
	t.Helper()
	var o theme.Overrides
	if err := yaml.Unmarshal([]byte(doc), &o); err != nil {
		t.Fatalf("unmarshalling %q: %v", doc, err)
	}
	return o
}

func TestOverridesWinPerTokenAndLeaveTheRestDerived(t *testing.T) {
	o := overrides(t, "accent: \"#ff0000\"\n")
	derived := theme.Terminal(darkBG, false)
	got := o.Apply(derived)

	if r, g, b := rgb(got.Accent); r != 0xff || g != 0 || b != 0 {
		t.Errorf("Accent = %d,%d,%d, want the override's ff,00,00", r, g, b)
	}
	if got.Success != derived.Success || got.Subtle != derived.Subtle {
		t.Error("an override for one token changed another, want the rest left derived")
	}
}

func TestOverrideTakesASlotIndex(t *testing.T) {
	// A user pinning a color may well want another of their own slots rather
	// than a hex, and lipgloss already spells one as a bare number.
	got := overrides(t, "accent: \"4\"\n").Apply(theme.Terminal(darkBG, false))
	if got.Accent != lipgloss.Blue {
		t.Errorf("Accent = %v, want slot 4", got.Accent)
	}
}

func TestOverridesValidateByName(t *testing.T) {
	for _, tc := range []struct {
		name string
		doc  string
	}{
		{"unknown key", "chartreuse: \"#ff0000\"\n"},
		{"unparseable value", "accent: \"nonsense\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := overrides(t, tc.doc).Validate(); err == nil {
				t.Errorf("Validate() = nil for %q, want an error naming it", tc.doc)
			}
		})
	}
}

func TestValidOverridesPass(t *testing.T) {
	doc := "accent: \"#c4a7e7\"\nselectedBackground: \"#2a283e\"\nerror: \"1\"\n"
	if err := overrides(t, doc).Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestAThemeNameIsToleratedRatherThanFatal(t *testing.T) {
	// `theme: rose-pine-moon` is on disk for anyone running the last release. A
	// scalar into a map is a parse error, and refusing to start over a color
	// scheme is the wrong trade.
	o := overrides(t, "rose-pine-moon\n")
	if o.Named != "rose-pine-moon" {
		t.Errorf("Named = %q, want the name kept so a notice can report it", o.Named)
	}
	if err := o.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil so the app still starts", err)
	}

	derived := theme.Terminal(darkBG, false)
	if got := o.Apply(derived); got.Accent != derived.Accent {
		t.Error("a theme name changed a color, want it ignored")
	}
}

func TestKeysAreStable(t *testing.T) {
	keys := theme.Keys()
	if len(keys) == 0 {
		t.Fatal("Keys() is empty, want the override vocabulary")
	}
	for i := 1; i < len(keys); i++ {
		if keys[i-1] >= keys[i] {
			t.Fatalf("Keys() = %v, want a stable sorted order", keys)
		}
	}
}
