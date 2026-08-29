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

	// The pair a terminal reports. The shades travel toward the foreground, so
	// most of these assertions need both halves.
	dark  = theme.Surface{Background: darkBG, Foreground: lipgloss.Color("#e0def4")}
	light = theme.Surface{Background: lightBG, Foreground: lipgloss.Color("#575279")}

	// Real palettes: a background bluer than its own green, olive hues, a light
	// one, and one with almost no room between its ends.
	mocha = theme.Surface{Background: lipgloss.Color("#1e1e2e"), Foreground: lipgloss.Color("#cdd6f4"),
		Red: lipgloss.Color("#f38ba8"), Green: lipgloss.Color("#a6e3a1")}
	gruvbox = theme.Surface{Background: lipgloss.Color("#282828"), Foreground: lipgloss.Color("#ebdbb2"),
		Red: lipgloss.Color("#cc241d"), Green: lipgloss.Color("#98971a")}
	solarizedLight = theme.Surface{Background: lipgloss.Color("#fdf6e3"), Foreground: lipgloss.Color("#657b83"),
		Red: lipgloss.Color("#dc322f"), Green: lipgloss.Color("#859900")}
	lowContrast = theme.Surface{Background: lipgloss.Color("#2b2b2b"), Foreground: lipgloss.Color("#8a8a8a"),
		Red: lipgloss.Color("#5c3030"), Green: lipgloss.Color("#305c30")}
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
	// The hues are the reader's, so a light terminal and a dark one get the
	// same slots. Only the shades move.
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

// A theme carries the background its every shade was derived against, so the
// two cannot disagree. It is also what a config naming a different background
// is asking to have painted.
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

// Nothing established a background, so there is none to carry and none to
// paint. The terminal's own goes on showing through.
func TestNoBackgroundCarriesNone(t *testing.T) {
	if got := theme.Terminal(theme.Surface{}, false).Background; got != nil {
		t.Errorf("Background = %v, want nil when nothing answered", got)
	}
}

// transparent is one rule: paint nothing. The background goes with the surfaces,
// or a translucent terminal is filled in solid by the thing meant to spare it.
func TestTransparentCarriesNoBackground(t *testing.T) {
	if got := theme.Terminal(dark, true).Background; got != nil {
		t.Errorf("Background = %v, want nil under transparent", got)
	}
}

func TestShadesLightenADarkBackground(t *testing.T) {
	th := theme.Terminal(dark, false)
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
	// A tint groups a run of changed lines. At full strength it buries the
	// source sitting on it, so it has to stay nearer the background than the
	// color it leans toward.
	th := theme.Terminal(dark, false)
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

	// Borders are drawn runes rather than fills, so they still have a color to
	// be. A slot is the only thing left to reach for.
	if _, ok := th.Border.(xansi.BasicColor); !ok {
		t.Errorf("Border = %T, want a slot when nothing was detected", th.Border)
	}
}

func TestTransparentDropsTheSurfacesAndKeepsTheRest(t *testing.T) {
	th := theme.Terminal(dark, true)

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
	opaque := theme.Terminal(dark, false)
	if th.Subtle != opaque.Subtle || th.Border != opaque.Border {
		t.Error("transparent changed the derived shades, want only the painted surfaces dropped")
	}
}

func TestSyntaxIsPairedAgainstTheBackground(t *testing.T) {
	// Chroma has no ANSI style, so code is the one thing that cannot follow the
	// palette. Pairing is what stops a light terminal drawing dark-theme source.
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

// overrides builds a set the way config hands one over, from plain strings.
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
	// A user pinning a color may well want another of their own slots rather
	// than a hex, and lipgloss already spells one as a bare number.
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

		// lipgloss reads any integer: past 255 it packs the value as RGB, so
		// "256" is a near-black rather than an error, and a negative is
		// silently made positive. Both are ordinary off-by-ones against the
		// range this documents, and on the background they paint the whole app.
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

// An out-of-range index must not reach a field either, or refusing it in
// Validate is a promise the layering does not keep.
func TestAnOutOfRangeIndexIsNotApplied(t *testing.T) {
	derived := theme.Terminal(dark, false)
	if got := overrides("accent", "256").Apply(derived); got.Accent != derived.Accent {
		t.Errorf("Accent = %v, want the derived %v rather than a packed near-black", got.Accent, derived.Accent)
	}
}

func TestAThemeNameIsToleratedRatherThanFatal(t *testing.T) {
	// `theme: rose-pine-moon` is on disk for anyone running the last release. A
	// scalar into a map is a parse error, and refusing to start over a color
	// scheme is the wrong trade.
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

// A terminal that answers nothing, or answers wrong, leaves the reader with no
// way to correct it. Naming the background is that way, and it has to reach the
// derivation rather than one field: the shades, the surfaces and the syntax
// pairing all hang off it.
func TestANamedBackgroundDrivesTheDerivation(t *testing.T) {
	o := overrides("background", "#faf4ed")

	// Nothing answered, which is the case this exists for.
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
	// The reported one is what was wrong, so it has to lose.
	o := overrides("background", "#faf4ed")
	got := o.Resolve(dark, false)

	if got.Syntax != theme.SyntaxLight {
		t.Errorf("Syntax = %q, want the named background to win over the reported one", got.Syntax)
	}

	// Only the background was named, so the reported foreground stands beside
	// it. That pair has no separation left — a light page under light text — and
	// the shades fall back to contrast, which is what keeps them readable.
	want := theme.Terminal(theme.Surface{Background: lightBG, Foreground: dark.Foreground}, false)
	if got.Subtle != want.Subtle {
		t.Errorf("Subtle = %v, want %v, derived from the named background", got.Subtle, want.Subtle)
	}
	if luma(got.Subtle) >= luma(lightBG) {
		t.Error("Subtle is not darker than the named background, want it readable against it")
	}
}

// Naming one is how a reader asks for a chrome that disagrees with their
// terminal, so it has to be carried and painted rather than only derived from.
func TestANamedBackgroundIsPainted(t *testing.T) {
	got := overrides("background", "#faf4ed").Resolve(dark, false)
	if r, g, b := rgb(got.Background); r != 0xfa || g != 0xf4 || b != 0xed {
		t.Errorf("Background = %d,%d,%d, want the named fa,f4,ed painted", r, g, b)
	}
}

// Naming a background corrects the page and says nothing about the palette, so
// the reported slots have to survive the correction.
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

// The three surfaces are derived now rather than mixed at a fixed ratio, and a
// reader who wrote one down has to go on outranking whatever replaced it.
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

// Named and translucent together is the reader who wrote it down only because
// their terminal could not answer. They get the derivation and no fill.
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

// Resolve with nothing named is Terminal, so the ordinary path gains no
// behaviour from the escape hatch existing.
func TestResolveWithoutANamedBackgroundIsTheReportedOne(t *testing.T) {
	var o theme.Overrides
	got, want := o.Resolve(dark, false), theme.Terminal(dark, false)
	if got.Subtle != want.Subtle || got.Syntax != want.Syntax || got.SelectedBackground != want.SelectedBackground {
		t.Error("Resolve changed the derivation when config named no background")
	}
}

// The shades travel toward the terminal's own text rather than toward pure
// white or black, so they sit on the axis between the page and the words on it.
func TestShadesTravelTowardTheReportedForeground(t *testing.T) {
	warm := lipgloss.Color("#e0c0a0") // a foreground well off neutral
	got := theme.Terminal(theme.Surface{Background: darkBG, Foreground: warm}, false)
	flat := theme.Terminal(theme.Surface{Background: darkBG}, false)

	if got.Subtle == flat.Subtle {
		t.Error("Subtle ignored the reported foreground, want it derived toward it")
	}

	// Toward a warm foreground the grey has to come out warm: red above blue,
	// where the pure-white fallback keeps the background's own balance.
	r, _, b := rgb(got.Subtle)
	if r <= b {
		t.Errorf("Subtle = %d,_,%d, want the warm foreground to lead red over blue", r, b)
	}
}

// A terminal reporting two colors close together would give a ladder nobody can
// read. Pure white or black is the worse fit and the safer one.
func TestAForegroundTooCloseToTheBackgroundIsRefused(t *testing.T) {
	murky := theme.Surface{Background: darkBG, Foreground: lipgloss.Color("#2b2940")}
	got := theme.Terminal(murky, false)
	flat := theme.Terminal(theme.Surface{Background: darkBG}, false)

	if got.Subtle != flat.Subtle {
		t.Error("a foreground with no separation was used, want the contrast fallback")
	}
}

// It is a derivation input like the background, for the same reader: a terminal
// that cannot answer, or answered wrongly.
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

// The foreground is a direction to travel, never a color to paint: Text stays
// the terminal's own, which follows a change this query only saw once.
func TestAReportedForegroundIsNotPaintedAsText(t *testing.T) {
	got := theme.Terminal(theme.Surface{Background: darkBG, Foreground: lipgloss.Color("#e0c0a0")}, false)
	if _, ok := got.Text.(lipgloss.NoColor); !ok {
		t.Errorf("Text = %v, want NoColor even when a foreground was reported", got.Text)
	}
}

// Naming a background that flips the terminal light-to-dark leaves the reported
// foreground on the wrong side of it. The separation guard is what stops that
// pair producing a ladder drawn in the page's own color.
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

// Which channel leads is the palette's business: over a background as blue as
// Catppuccin's, a wash of a real green is still bluer than green.
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

// between holds a tint to the run from the background to its hue, at the nearer
// end of it: past the middle the code sitting on the tint goes under.
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

// Blending a slot takes its canonical value, which is a color nobody is looking
// at: xterm's dark system palette rather than the reader's own.
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

// The same fraction that clears one palette's green leaves the row flat against
// another's, which is why the lift solves for a distance.
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

// A hue at the background's own weight cannot lift the row but moves a long way
// in color, so the floor every surface meets is a channel distance.
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

// A reader scanning a hunk reads the block before the marker in it, so the two
// tints have to stay apart even where neither can lift.
func TestTheTwoTintsNeverCollapseTogether(t *testing.T) {
	for _, tc := range []struct {
		name string
		s    theme.Surface
	}{
		{"dark", dark},
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

// Along the shade axis a selection took the foreground's tint, which is a color
// the reader never chose.
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
