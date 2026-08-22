package paint_test

import (
	"strings"
	"testing"

	"github.com/praxis-labs-io/zen-octo/internal/golden"
	"github.com/praxis-labs-io/zen-octo/internal/tui/paint"
	"github.com/praxis-labs-io/zen-octo/internal/tui/syntax"
	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

// These goldens keep their escapes, where the frame ones are stripped: what a
// painted row gets wrong is the colour and the fill. `cat` one to read it.
func compare(t *testing.T, name, got string) {
	t.Helper()
	golden.Compare(t, name, []byte(got))
}

// Tokens are hand-built rather than taken from Chroma, so a golden file records
// what the painter did and not what a lexer version thought of a line.
func tokens() []syntax.Token {
	return []syntax.Token{
		{Text: "const ", Color: theme.RosePineMoon.Accent},
		{Text: "n", Color: theme.RosePineMoon.Text},
		{Text: " = "},
		{Text: "4", Color: theme.RosePineMoon.Warning},
	}
}

func painter() paint.Painter {
	return paint.Painter{Theme: theme.RosePineMoon}
}

func TestGoldenLines(t *testing.T) {
	tests := []struct {
		name  string
		line  paint.Line
		width int
	}{
		{"line_added", paint.Line{Kind: paint.Added, New: 12, Tokens: tokens()}, 40},
		{"line_removed", paint.Line{Kind: paint.Removed, Old: 11, Tokens: tokens()}, 40},
		{"line_context", paint.Line{Kind: paint.Context, Old: 11, New: 12, Tokens: tokens()}, 40},
		{
			"tabs",
			paint.Line{Kind: paint.Context, Old: 11, New: 12, Tokens: []syntax.Token{
				{Text: "\t"},
				{Text: "return", Color: theme.RosePineMoon.Accent},
				{Text: "\tnil"},
			}},
			40,
		},
		{
			"clipped",
			paint.Line{Kind: paint.Added, New: 12, Tokens: []syntax.Token{
				{Text: "if err != nil { return fmt.Errorf(\"painting: %w\", err) }", Color: theme.RosePineMoon.Text},
			}},
			24,
		},
		// An odd remainder against two-cell runes is where the cut used to come
		// back a column short of the pane.
		{
			"clipped_wide",
			paint.Line{Kind: paint.Added, New: 12, Tokens: []syntax.Token{
				{Text: "// 日本語のコメント", Color: theme.RosePineMoon.Subtle},
			}},
			21,
		},
		{
			"fill_override",
			paint.Line{Kind: paint.Added, New: 12, Tokens: tokens(), Fill: theme.RosePineMoon.SelectedBackground},
			40,
		},
		{"wide_gutter", paint.Line{Kind: paint.Context, Old: 1234, New: 1235, Tokens: tokens()}, 40},
		// The bar takes the leading cell rather than a column of its own, so a
		// barred row is exactly as wide as the one above it.
		{
			"line_barred",
			paint.Line{
				Kind:   paint.Added,
				New:    12,
				Tokens: tokens(),
				Fill:   theme.RosePineMoon.SelectedBackground,
				Bar:    theme.RosePineMoon.Accent,
			},
			40,
		},
	}

	p := painter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gutter := paint.Gutter(max(tt.line.Old, tt.line.New))
			compare(t, tt.name, p.Line(tt.line, gutter, tt.width))
		})
	}
}

// A held-open column has to be exactly as wide as a filled one, or the marker
// moves between a line that has both numbers and a line that has one.
func TestGoldenOneSided(t *testing.T) {
	p := painter()
	gutter := paint.Gutter(120)

	rows := []string{
		p.Line(paint.Line{Kind: paint.Added, New: 120, Tokens: tokens()}, gutter, 40),
		p.Line(paint.Line{Kind: paint.Removed, Old: 119, Tokens: tokens()}, gutter, 40),
		p.Line(paint.Line{Kind: paint.Context, Old: 119, New: 120, Tokens: tokens()}, gutter, 40),
	}
	compare(t, "one_sided", strings.Join(rows, "\n"))
}

// Half is the one renderer whose output is always exactly its width: a short
// column puts the one beside it out of step for the rest of the file.
func TestGoldenHalves(t *testing.T) {
	tests := []struct {
		name  string
		line  paint.Line
		width int
	}{
		{"half_added", paint.Line{Kind: paint.Added, New: 120, Tokens: tokens()}, 26},
		{"half_removed", paint.Line{Kind: paint.Removed, Old: 119, Tokens: tokens()}, 26},
		{"half_context", paint.Line{Kind: paint.Context, New: 120, Tokens: tokens()}, 26},

		// The column facing an unpaired change: no number, no marker, no tint,
		// and still exactly as wide as the one beside it.
		{"half_blank", paint.Line{}, 26},
		{
			"half_clipped",
			paint.Line{Kind: paint.Added, New: 120, Tokens: []syntax.Token{
				{Text: "if err != nil { return err }", Color: theme.RosePineMoon.Text},
			}},
			14,
		},
		{
			"half_filled",
			paint.Line{Kind: paint.Context, New: 120, Tokens: tokens(), Fill: theme.RosePineMoon.SelectedBackground},
			26,
		},
		{
			"half_barred",
			paint.Line{
				Kind:   paint.Added,
				New:    120,
				Tokens: tokens(),
				Fill:   theme.RosePineMoon.SelectedBackground,
				Bar:    theme.RosePineMoon.Accent,
			},
			26,
		},
	}

	p := painter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compare(t, tt.name, p.Half(tt.line, paint.Gutter(120), tt.width))
		})
	}
}

// The code column moves with the gutter, the way it does in a unified row.
func TestGoldenHalfWideGutter(t *testing.T) {
	compare(t, "half_wide_gutter", painter().Half(
		paint.Line{Kind: paint.Added, New: 42100, Tokens: tokens()}, paint.Gutter(42100), 30))
}

// A heading over side-by-side indents to the left column's own code, and its
// bar sits at the pane edge rather than in either column.
func TestGoldenHalfHeader(t *testing.T) {
	compare(t, "half_header", painter().HalfHeader(paint.Header{
		Text:  "@@ -11,4 +12,6 @@ func Paint()",
		Badge: "○",
		Fill:  theme.RosePineMoon.SelectedBackground,
		Bar:   theme.RosePineMoon.Accent,
	}, paint.Gutter(120), 40))
}

func TestGoldenHunkHeader(t *testing.T) {
	compare(t, "hunk_header", painter().HunkHeader(paint.Header{Text: "@@ -11,4 +12,6 @@ func Paint()"}, paint.Gutter(1235), 40))
}

// The heading a cursor is on: filled to the edge, with the mark in the column
// the change marks under it use.
func TestGoldenHunkHeaderMarked(t *testing.T) {
	compare(t, "hunk_header_marked", painter().HunkHeader(paint.Header{
		Text:   "@@ -11,4 +12,6 @@ func Paint()",
		Marker: "▸",
		Fill:   theme.RosePineMoon.SelectedBackground,
	}, paint.Gutter(1235), 40))
}

// A heading the cursor is on takes the bar at the pane edge, where the badge and
// the marker keep the columns they line up with the source in.
func TestGoldenHunkHeaderBarred(t *testing.T) {
	compare(t, "hunk_header_barred", painter().HunkHeader(paint.Header{
		Text:  "@@ -11,4 +12,6 @@ func Paint()",
		Badge: "●",
		Fill:  theme.RosePineMoon.SelectedBackground,
		Bar:   theme.RosePineMoon.Accent,
	}, paint.Gutter(1235), 40))
}

// Both glyphs at once, which is a cursor on a heading that carries a state.
func TestGoldenHunkHeaderBadged(t *testing.T) {
	compare(t, "hunk_header_badged", painter().HunkHeader(paint.Header{
		Text:   "@@ -11,4 +12,6 @@ func Paint()",
		Marker: "▸",
		Badge:  "●",
		Fill:   theme.RosePineMoon.SelectedBackground,
	}, paint.Gutter(1235), 40))
}
