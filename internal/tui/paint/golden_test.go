package paint_test

import (
	"strings"
	"testing"

	"github.com/praxis-labs-io/zen-octo/internal/golden"
	"github.com/praxis-labs-io/zen-octo/internal/tui/paint"
	"github.com/praxis-labs-io/zen-octo/internal/tui/syntax"
)

func compare(t *testing.T, name, got string) {
	t.Helper()
	golden.Compare(t, name, []byte(got))
}

func tokens() []syntax.Token {
	return []syntax.Token{
		{Text: "const ", Color: testTheme.Accent},
		{Text: "n", Color: testTheme.Text},
		{Text: " = "},
		{Text: "4", Color: testTheme.Warning},
	}
}

func painter() paint.Painter {
	return paint.Painter{Theme: testTheme}
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
				{Text: "return", Color: testTheme.Accent},
				{Text: "\tnil"},
			}},
			40,
		},
		{
			"clipped",
			paint.Line{Kind: paint.Added, New: 12, Tokens: []syntax.Token{
				{Text: "if err != nil { return fmt.Errorf(\"painting: %w\", err) }", Color: testTheme.Text},
			}},
			24,
		},
		{
			"clipped_wide",
			paint.Line{Kind: paint.Added, New: 12, Tokens: []syntax.Token{
				{Text: "// 日本語のコメント", Color: testTheme.Subtle},
			}},
			21,
		},
		{
			"fill_override",
			paint.Line{Kind: paint.Added, New: 12, Tokens: tokens(), Fill: testTheme.SelectedBackground},
			40,
		},
		{"wide_gutter", paint.Line{Kind: paint.Context, Old: 1234, New: 1235, Tokens: tokens()}, 40},
		{
			"line_barred",
			paint.Line{
				Kind:   paint.Added,
				New:    12,
				Tokens: tokens(),
				Fill:   testTheme.SelectedBackground,
				Bar:    testTheme.Accent,
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

func TestGoldenHalves(t *testing.T) {
	tests := []struct {
		name  string
		line  paint.Line
		width int
	}{
		{"half_added", paint.Line{Kind: paint.Added, New: 120, Tokens: tokens()}, 26},
		{"half_removed", paint.Line{Kind: paint.Removed, Old: 119, Tokens: tokens()}, 26},
		{"half_context", paint.Line{Kind: paint.Context, New: 120, Tokens: tokens()}, 26},

		{"half_blank", paint.Line{}, 26},
		{
			"half_clipped",
			paint.Line{Kind: paint.Added, New: 120, Tokens: []syntax.Token{
				{Text: "if err != nil { return err }", Color: testTheme.Text},
			}},
			14,
		},
		{
			"half_filled",
			paint.Line{Kind: paint.Context, New: 120, Tokens: tokens(), Fill: testTheme.SelectedBackground},
			26,
		},
		{
			"half_barred",
			paint.Line{
				Kind:   paint.Added,
				New:    120,
				Tokens: tokens(),
				Fill:   testTheme.SelectedBackground,
				Bar:    testTheme.Accent,
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

func TestGoldenHalfWideGutter(t *testing.T) {
	compare(t, "half_wide_gutter", painter().Half(
		paint.Line{Kind: paint.Added, New: 42100, Tokens: tokens()}, paint.Gutter(42100), 30))
}

func TestGoldenHalfHeader(t *testing.T) {
	compare(t, "half_header", painter().HalfHeader(paint.Header{
		Text:  "@@ -11,4 +12,6 @@ func Paint()",
		Badge: "○",
		Fill:  testTheme.SelectedBackground,
		Bar:   testTheme.Accent,
	}, paint.Gutter(120), 40))
}

func TestGoldenHunkHeader(t *testing.T) {
	compare(t, "hunk_header", painter().HunkHeader(paint.Header{Text: "@@ -11,4 +12,6 @@ func Paint()"}, paint.Gutter(1235), 40))
}

func TestGoldenHunkHeaderMarked(t *testing.T) {
	compare(t, "hunk_header_marked", painter().HunkHeader(paint.Header{
		Text:   "@@ -11,4 +12,6 @@ func Paint()",
		Marker: "▸",
		Fill:   testTheme.SelectedBackground,
	}, paint.Gutter(1235), 40))
}

func TestGoldenHunkHeaderBarred(t *testing.T) {
	compare(t, "hunk_header_barred", painter().HunkHeader(paint.Header{
		Text:  "@@ -11,4 +12,6 @@ func Paint()",
		Badge: "●",
		Fill:  testTheme.SelectedBackground,
		Bar:   testTheme.Accent,
	}, paint.Gutter(1235), 40))
}

func TestGoldenHunkHeaderBadged(t *testing.T) {
	compare(t, "hunk_header_badged", painter().HunkHeader(paint.Header{
		Text:   "@@ -11,4 +12,6 @@ func Paint()",
		Marker: "▸",
		Badge:  "●",
		Fill:   testTheme.SelectedBackground,
	}, paint.Gutter(1235), 40))
}
