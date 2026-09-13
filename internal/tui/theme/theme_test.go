package theme_test

import (
	"image/color"
	"slices"
	"testing"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

var (
	darkBG  = lipgloss.Color("#232136")
	lightBG = lipgloss.Color("#faf4ed")

	dark  = theme.Surface{Background: darkBG, Foreground: lipgloss.Color("#e0def4")}
	light = theme.Surface{Background: lightBG, Foreground: lipgloss.Color("#575279")}

	mocha = theme.Surface{Background: lipgloss.Color("#1e1e2e"), Foreground: lipgloss.Color("#cdd6f4"),
		Red: lipgloss.Color("#f38ba8"), Green: lipgloss.Color("#a6e3a1")}
	gruvbox = theme.Surface{Background: lipgloss.Color("#282828"), Foreground: lipgloss.Color("#ebdbb2"),
		Red: lipgloss.Color("#cc241d"), Green: lipgloss.Color("#98971a")}
	solarizedLight = theme.Surface{Background: lipgloss.Color("#fdf6e3"), Foreground: lipgloss.Color("#657b83"),
		Red: lipgloss.Color("#dc322f"), Green: lipgloss.Color("#859900")}
	lowContrast = theme.Surface{Background: lipgloss.Color("#2b2b2b"), Foreground: lipgloss.Color("#8a8a8a"),
		Red: lipgloss.Color("#5c3030"), Green: lipgloss.Color("#305c30")}
)

func rgb(c color.Color) (int, int, int) {
	r, g, b, _ := c.RGBA()
	return int(r >> 8), int(g >> 8), int(b >> 8)
}

func luma(c color.Color) float64 {
	r, g, b := rgb(c)
	return 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
}

func TestHuesStayASlot(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  color.Color
		want xansi.BasicColor
	}{
		{"Accent", theme.Terminal(dark, false).Accent, lipgloss.Blue},
		{"Success", theme.Terminal(dark, false).Success, lipgloss.Green},
		{"Warning", theme.Terminal(dark, false).Warning, lipgloss.Yellow},
		{"Error", theme.Terminal(dark, false).Error, lipgloss.Red},
		{"Actor", theme.Terminal(dark, false).Actor, lipgloss.Magenta},
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
	dark, light := theme.Terminal(dark, false), theme.Terminal(light, false)
	if dark.Accent != light.Accent {
		t.Errorf("Accent = %v on dark and %v on light, want the same slot", dark.Accent, light.Accent)
	}
	if dark.Error != light.Error {
		t.Errorf("Error = %v on dark and %v on light, want the same slot", dark.Error, light.Error)
	}
}

func TestTextIsTheTerminalsOwn(t *testing.T) {
	if _, ok := theme.Terminal(dark, false).Text.(lipgloss.NoColor); !ok {
		t.Errorf("Text = %v, want NoColor so the terminal's own foreground is used", theme.Terminal(dark, false).Text)
	}
}

func TestAThemeCarriesItsBackground(t *testing.T) {
	for _, tc := range []struct {
		name string
		s    theme.Surface
	}{
		{"dark", dark},
		{"light", light},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := theme.Terminal(tc.s, false).Background; got != tc.s.Background {
				t.Errorf("Background = %v, want the %v it was derived from", got, tc.s.Background)
			}
		})
	}
}

func TestNoBackgroundCarriesNone(t *testing.T) {
	if got := theme.Terminal(theme.Surface{}, false).Background; got != nil {
		t.Errorf("Background = %v, want nil when nothing answered", got)
	}
}

func TestTransparentCarriesNoBackground(t *testing.T) {
	if got := theme.Terminal(dark, true).Background; got != nil {
		t.Errorf("Background = %v, want nil under transparent", got)
	}
}

func TestShadesLightenADarkBackground(t *testing.T) {
	th := theme.Terminal(dark, false)
	base := luma(darkBG)

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
	th := theme.Terminal(light, false)
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
	th := theme.Terminal(dark, false)
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
	th := theme.Terminal(dark, false)
	base, added := luma(darkBG), luma(th.AddedBackground)
	if added-base > luma(lipgloss.Green)-base {
		t.Errorf("AddedBackground luma = %.1f, want far nearer the background's %.1f than green's %.1f",
			added, base, luma(lipgloss.Green))
	}
}

func TestNoBackgroundPaintsNoSurface(t *testing.T) {
	th := theme.Terminal(theme.Surface{}, false)
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

	if _, ok := th.Border.(xansi.BasicColor); !ok {
		t.Errorf("Border = %T, want a slot when nothing was detected", th.Border)
	}
}

func TestTransparentWithholdsTheBackgroundAndKeepsTheSurfaces(t *testing.T) {
	th, opaque := theme.Terminal(dark, true), theme.Terminal(dark, false)

	if th.Background != nil {
		t.Errorf("Background = %v under transparent, want nil", th.Background)
	}

	for _, tc := range []struct {
		name      string
		got, want color.Color
	}{
		{"SelectedBackground", th.SelectedBackground, opaque.SelectedBackground},
		{"AddedBackground", th.AddedBackground, opaque.AddedBackground},
		{"RemovedBackground", th.RemovedBackground, opaque.RemovedBackground},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v under transparent, want %v", tc.name, tc.got, tc.want)
		}
	}

	if th.Subtle != opaque.Subtle || th.Border != opaque.Border {
		t.Error("transparent changed the derived shades, want only the background withheld")
	}
}

func TestSyntaxIsPairedAgainstTheBackground(t *testing.T) {
	for _, tc := range []struct {
		name string
		s    theme.Surface
		want string
	}{
		{"dark", dark, theme.SyntaxDark},
		{"light", light, theme.SyntaxLight},
		{"undetected", theme.Surface{}, theme.SyntaxDark},
	} {
		if got := theme.Terminal(tc.s, false).Syntax; got != tc.want {
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
	full := theme.Terminal(dark, false)

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

func overrides(colors ...string) theme.Overrides {
	m := make(map[string]string, len(colors)/2)
	for i := 0; i+1 < len(colors); i += 2 {
		m[colors[i]] = colors[i+1]
	}
	return theme.NewOverrides(m, "")
}

func TestOverridesWinPerTokenAndLeaveTheRestDerived(t *testing.T) {
	o := overrides("accent", "#ff0000")
	derived := theme.Terminal(dark, false)
	got := o.Apply(derived)

	if r, g, b := rgb(got.Accent); r != 0xff || g != 0 || b != 0 {
		t.Errorf("Accent = %d,%d,%d, want the override's ff,00,00", r, g, b)
	}
	if got.Success != derived.Success || got.Subtle != derived.Subtle {
		t.Error("an override for one token changed another, want the rest left derived")
	}
}

func TestOverrideTakesASlotIndex(t *testing.T) {
	got := overrides("accent", "4").Apply(theme.Terminal(dark, false))
	if got.Accent != lipgloss.Blue {
		t.Errorf("Accent = %v, want slot 4", got.Accent)
	}
}

func TestOverridesValidateByName(t *testing.T) {
	for _, tc := range []struct {
		name       string
		key, value string
	}{
		{"unknown key", "chartreuse", "#ff0000"},
		{"unparseable value", "accent", "nonsense"},

		{"index past the range", "accent", "256"},
		{"index far past the range", "background", "16711680"},
		{"negative index", "accent", "-1"},
		{"half a hex", "accent", "#ff00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := overrides(tc.key, tc.value).Validate(); err == nil {
				t.Errorf("Validate() = nil for %s: %q, want an error naming it", tc.key, tc.value)
			}
		})
	}
}

func TestValidOverridesPass(t *testing.T) {
	o := overrides("accent", "#c4a7e7", "selectedBackground", "#2a283e",
		"error", "1", "muted", "255")
	if err := o.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestAnOutOfRangeIndexIsNotApplied(t *testing.T) {
	derived := theme.Terminal(dark, false)
	if got := overrides("accent", "256").Apply(derived); got.Accent != derived.Accent {
		t.Errorf("Accent = %v, want the derived %v rather than a packed near-black", got.Accent, derived.Accent)
	}
}

func TestAThemeNameIsToleratedRatherThanFatal(t *testing.T) {
	o := theme.NewOverrides(nil, "rose-pine-moon")
	if o.Named != "rose-pine-moon" {
		t.Errorf("Named = %q, want the name kept so a notice can report it", o.Named)
	}
	if err := o.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil so the app still starts", err)
	}

	derived := theme.Terminal(dark, false)
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

func TestANamedBackgroundDrivesTheDerivation(t *testing.T) {
	o := overrides("background", "#faf4ed")

	got := o.Resolve(theme.Surface{}, false)

	if got.SelectedBackground == nil {
		t.Error("SelectedBackground is nil, want the named background to restore the surfaces")
	}
	if got.Syntax != theme.SyntaxLight {
		t.Errorf("Syntax = %q, want the pairing to follow the named background", got.Syntax)
	}
	if luma(got.Subtle) >= luma(lightBG) {
		t.Error("Subtle is not darker than the named background, want it derived against it")
	}
}

func TestANamedBackgroundOutranksTheReportedOne(t *testing.T) {
	o := overrides("background", "#faf4ed")
	got := o.Resolve(dark, false)

	if got.Syntax != theme.SyntaxLight {
		t.Errorf("Syntax = %q, want the named background to win over the reported one", got.Syntax)
	}

	want := theme.Terminal(theme.Surface{Background: lightBG, Foreground: dark.Foreground}, false)
	if got.Subtle != want.Subtle {
		t.Errorf("Subtle = %v, want %v, derived from the named background", got.Subtle, want.Subtle)
	}
	if luma(got.Subtle) >= luma(lightBG) {
		t.Error("Subtle is not darker than the named background, want it readable against it")
	}
}

func TestANamedBackgroundIsPainted(t *testing.T) {
	got := overrides("background", "#faf4ed").Resolve(dark, false)
	if r, g, b := rgb(got.Background); r != 0xfa || g != 0xf4 || b != 0xed {
		t.Errorf("Background = %d,%d,%d, want the named fa,f4,ed painted", r, g, b)
	}
}

func TestANamedBackgroundKeepsTheReportedPalette(t *testing.T) {
	got := overrides("background", "#faf4ed").Resolve(mocha, false)
	blind := overrides("background", "#faf4ed").
		Resolve(theme.Surface{Background: mocha.Background, Foreground: mocha.Foreground}, false)

	if got.AddedBackground == blind.AddedBackground {
		t.Error("AddedBackground fell back to the slot, want the reported green carried through Resolve")
	}
	if got.RemovedBackground == blind.RemovedBackground {
		t.Error("RemovedBackground fell back to the slot, want the reported red carried through Resolve")
	}
}

func TestTheSurfaceOverridesWinOverTheDerivedOnes(t *testing.T) {
	o := overrides("addedBackground", "#123456", "removedBackground", "#654321",
		"selectedBackground", "#2a283e")
	got := o.Resolve(mocha, false)

	for _, tc := range []struct {
		name string
		got  color.Color
		want string
	}{
		{"AddedBackground", got.AddedBackground, "#123456"},
		{"RemovedBackground", got.RemovedBackground, "#654321"},
		{"SelectedBackground", got.SelectedBackground, "#2a283e"},
	} {
		wr, wg, wb := rgb(lipgloss.Color(tc.want))
		if r, g, b := rgb(tc.got); r != wr || g != wg || b != wb {
			t.Errorf("%s = %d,%d,%d, want the named %s", tc.name, r, g, b, tc.want)
		}
	}
}

func TestANamedBackgroundIsNotPaintedUnderTransparent(t *testing.T) {
	got := overrides("background", "#faf4ed").Resolve(theme.Surface{}, true)
	if got.Background != nil {
		t.Errorf("Background = %v, want nil so the terminal's own still shows", got.Background)
	}
	want := theme.Terminal(theme.Surface{Background: lightBG}, true)
	if got.Subtle != want.Subtle {
		t.Error("the named background stopped driving the shades under transparent")
	}
}

func TestBackgroundIsAKnownKey(t *testing.T) {
	if err := overrides("background", "#faf4ed").Validate(); err != nil {
		t.Errorf("Validate() = %v, want background accepted", err)
	}
	if err := overrides("background", "nonsense").Validate(); err == nil {
		t.Error("Validate() = nil for an unparseable background, want an error")
	}
	if !slices.Contains(theme.Keys(), "background") {
		t.Errorf("Keys() = %v, want it to name background", theme.Keys())
	}
}

func TestResolveWithoutANamedBackgroundIsTheReportedOne(t *testing.T) {
	var o theme.Overrides
	got, want := o.Resolve(dark, false), theme.Terminal(dark, false)
	if got.Subtle != want.Subtle || got.Syntax != want.Syntax || got.SelectedBackground != want.SelectedBackground {
		t.Error("Resolve changed the derivation when config named no background")
	}
}

func TestShadesTravelTowardTheReportedForeground(t *testing.T) {
	warm := lipgloss.Color("#e0c0a0")
	got := theme.Terminal(theme.Surface{Background: darkBG, Foreground: warm}, false)
	flat := theme.Terminal(theme.Surface{Background: darkBG}, false)

	if got.Subtle == flat.Subtle {
		t.Error("Subtle ignored the reported foreground, want it derived toward it")
	}

	r, _, b := rgb(got.Subtle)
	if r <= b {
		t.Errorf("Subtle = %d,_,%d, want the warm foreground to lead red over blue", r, b)
	}
}

func TestAForegroundTooCloseToTheBackgroundIsRefused(t *testing.T) {
	murky := theme.Surface{Background: darkBG, Foreground: lipgloss.Color("#2b2940")}
	got := theme.Terminal(murky, false)
	flat := theme.Terminal(theme.Surface{Background: darkBG}, false)

	if got.Subtle != flat.Subtle {
		t.Error("a foreground with no separation was used, want the contrast fallback")
	}
}

func TestANamedForegroundDrivesTheShades(t *testing.T) {
	o := overrides("foreground", "#e0c0a0")
	got := o.Resolve(theme.Surface{Background: darkBG}, false)

	if want := theme.Terminal(theme.Surface{Background: darkBG, Foreground: lipgloss.Color("#e0c0a0")}, false); got.Subtle != want.Subtle {
		t.Errorf("Subtle = %v, want %v, derived toward the named foreground", got.Subtle, want.Subtle)
	}
	if err := o.Validate(); err != nil {
		t.Errorf("Validate() = %v, want foreground accepted", err)
	}
	if !slices.Contains(theme.Keys(), "foreground") {
		t.Errorf("Keys() = %v, want it to name foreground", theme.Keys())
	}
}

func TestAReportedForegroundIsNotPaintedAsText(t *testing.T) {
	got := theme.Terminal(theme.Surface{Background: darkBG, Foreground: lipgloss.Color("#e0c0a0")}, false)
	if _, ok := got.Text.(lipgloss.NoColor); !ok {
		t.Errorf("Text = %v, want NoColor even when a foreground was reported", got.Text)
	}
}

func TestANamedBackgroundThatStrandsTheForegroundStaysReadable(t *testing.T) {
	for _, tc := range []struct {
		name    string
		named   string
		against theme.Surface
	}{
		{"light named over a dark terminal", "#faf4ed", dark},
		{"dark named over a light terminal", "#232136", light},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := overrides("background", tc.named).Resolve(tc.against, false)

			for _, shade := range []struct {
				name string
				c    color.Color
			}{{"Subtle", got.Subtle}, {"Muted", got.Muted}, {"Border", got.Border}} {
				if separationOf(shade.c, got.Background) < 24 {
					t.Errorf("%s is indistinguishable from the background it sits on", shade.name)
				}
			}
		})
	}
}

func separationOf(a, b color.Color) float64 {
	if d := luma(a) - luma(b); d < 0 {
		return -d
	} else {
		return d
	}
}

func TestATintLiesBetweenTheBackgroundAndItsHue(t *testing.T) {
	for _, tc := range []struct {
		name string
		s    theme.Surface
	}{
		{"dark", withPalette(dark, "#eb6f92", "#3e8fb0")},
		{"blue-heavy background", mocha},
		{"olive hues", gruvbox},
		{"light", solarizedLight},
	} {
		t.Run(tc.name, func(t *testing.T) {
			th := theme.Terminal(tc.s, false)
			between(t, "AddedBackground", tc.s.Background, th.AddedBackground, tc.s.Green)
			between(t, "RemovedBackground", tc.s.Background, th.RemovedBackground, tc.s.Red)
		})
	}
}

func between(t *testing.T, name string, bg, tint, hue color.Color) {
	t.Helper()

	br, bgr, bb := rgb(bg)
	tr, tg, tb := rgb(tint)
	hr, hg, hb := rgb(hue)

	for _, c := range []struct {
		channel        string
		base, got, end int
	}{
		{"red", br, tr, hr},
		{"green", bgr, tg, hg},
		{"blue", bb, tb, hb},
	} {
		if c.got < min(c.base, c.end) || c.got > max(c.base, c.end) {
			t.Errorf("%s %s = %d, want it between the background's %d and the hue's %d",
				name, c.channel, c.got, c.base, c.end)
		}
		if abs(c.got-c.base) > abs(c.got-c.end) {
			t.Errorf("%s %s = %d, nearer the hue's %d than the background's %d: the code on it goes under",
				name, c.channel, c.got, c.end, c.base)
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func withPalette(s theme.Surface, red, green string) theme.Surface {
	s.Red, s.Green = lipgloss.Color(red), lipgloss.Color(green)
	return s
}

func TestATintTakesTheReportedHueOverTheSlot(t *testing.T) {
	reported := theme.Terminal(withPalette(dark, "#f38ba8", "#a6e3a1"), false)
	canonical := theme.Terminal(dark, false)

	if reported.AddedBackground == canonical.AddedBackground {
		t.Error("AddedBackground ignored the reported green, want it derived from the palette")
	}
	if reported.RemovedBackground == canonical.RemovedBackground {
		t.Error("RemovedBackground ignored the reported red, want it derived from the palette")
	}
}

func TestATintClearsTheBackgroundOnEverySurface(t *testing.T) {
	const least = 8

	for _, tc := range []struct {
		name string
		s    theme.Surface
	}{
		{"blue-heavy background", mocha},
		{"olive hues", gruvbox},
		{"light palette", solarizedLight},
		{"low contrast", lowContrast},
	} {
		t.Run(tc.name, func(t *testing.T) {
			th := theme.Terminal(tc.s, false)
			base := luma(tc.s.Background)

			for _, tint := range []struct {
				name string
				c    color.Color
			}{
				{"AddedBackground", th.AddedBackground},
				{"RemovedBackground", th.RemovedBackground},
				{"SelectedBackground", th.SelectedBackground},
			} {
				if got := luma(tint.c) - base; got < least && got > -least {
					t.Errorf("%s luma = %.1f against a background of %.1f, want it clear by %d",
						tint.name, luma(tint.c), base, least)
				}
			}
		})
	}
}

func TestATintIsPerceptibleOnEverySurface(t *testing.T) {
	const least = 10

	for _, tc := range []struct {
		name string
		s    theme.Surface
	}{
		{"dark", dark},
		{"light", light},
		{"blue-heavy background", mocha},
		{"olive hues", gruvbox},
		{"light palette", solarizedLight},
		{"low contrast", lowContrast},
	} {
		t.Run(tc.name, func(t *testing.T) {
			th := theme.Terminal(tc.s, false)
			br, bg, bb := rgb(tc.s.Background)

			for _, tint := range []struct {
				name string
				c    color.Color
			}{
				{"AddedBackground", th.AddedBackground},
				{"RemovedBackground", th.RemovedBackground},
				{"SelectedBackground", th.SelectedBackground},
			} {
				r, g, b := rgb(tint.c)
				if got := max(abs(r-br), abs(g-bg), abs(b-bb)); got < least {
					t.Errorf("%s is %d,%d,%d over a %d,%d,%d background, no channel moving more than %d",
						tint.name, r, g, b, br, bg, bb, got)
				}
			}
		})
	}
}

// Pinned rather than asserted away: this is the fallback the OSC 4 request exists to avoid.
func TestTheCanonicalFallbackLeansWithoutLifting(t *testing.T) {
	for _, bg := range []string{"#232136", "#282828", "#2b2b2b"} {
		t.Run(bg, func(t *testing.T) {
			base := lipgloss.Color(bg)
			th := theme.Terminal(theme.Surface{Background: base, Foreground: lipgloss.Color("#e0def4")}, false)

			br, bgr, bb := rgb(base)
			rr, rg, rb := rgb(th.RemovedBackground)
			if moved := max(abs(rr-br), abs(rg-bgr), abs(rb-bb)); moved < 10 {
				t.Errorf("RemovedBackground is %d,%d,%d over %d,%d,%d, moving %d: the row does not read as red",
					rr, rg, rb, br, bgr, bb, moved)
			}
			if rr <= rg || rr <= rb {
				t.Errorf("RemovedBackground = %d,%d,%d, want red to lead so the lean survives the flat luma", rr, rg, rb)
			}
			if got := luma(th.AddedBackground) - luma(base); got < 8 {
				t.Errorf("AddedBackground lifted %.1f, want the fallback green to still clear the page", got)
			}
		})
	}
}

func TestTheTwoTintsNeverCollapseTogether(t *testing.T) {
	for _, tc := range []struct {
		name string
		s    theme.Surface
	}{
		{"dark", withPalette(dark, "#eb6f92", "#3e8fb0")},
		{"light", light},
		{"blue-heavy background", mocha},
		{"olive hues", gruvbox},
		{"low contrast", lowContrast},
		{"no palette reported", dark},
	} {
		t.Run(tc.name, func(t *testing.T) {
			th := theme.Terminal(tc.s, false)
			ar, ag, ab := rgb(th.AddedBackground)
			rr, rg, rb := rgb(th.RemovedBackground)

			if ar == rr && ag == rg && ab == rb {
				t.Errorf("both tints are %d,%d,%d, so a changed block cannot say which way it went", ar, ag, ab)
			}
		})
	}
}

func TestTheSelectionIsANeutralLift(t *testing.T) {
	warm := theme.Surface{Background: darkBG, Foreground: lipgloss.Color("#e0c0a0")}
	th := theme.Terminal(warm, false)

	br, bg, bb := rgb(darkBG)
	sr, sg, sb := rgb(th.SelectedBackground)

	dr, dg, db := sr-br, sg-bg, sb-bb
	if spread := max(dr, dg, db) - min(dr, dg, db); spread > 2 {
		t.Errorf("SelectedBackground moves %d,%d,%d off the background, want an even lift", dr, dg, db)
	}
}
