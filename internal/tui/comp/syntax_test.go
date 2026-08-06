package comp_test

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/tui/comp"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

func syntax(t *testing.T) comp.Syntax {
	t.Helper()
	s, ok := comp.NewSyntax(theme.RosePineMoon.Syntax)
	if !ok {
		t.Fatalf("Chroma does not know %q", theme.RosePineMoon.Syntax)
	}
	return s
}

func TestTheDefaultThemeNamesAStyleChromaShips(t *testing.T) {
	syntax(t)
}

func TestAnUnknownStyleStillColorizes(t *testing.T) {
	s, ok := comp.NewSyntax("not-a-chroma-style")
	if ok {
		t.Error("an unknown style reported itself as known")
	}
	if got := s.Lines("a.go", "package a"); len(got) != 1 {
		t.Errorf("lines = %d, want the code back anyway", len(got))
	}
}

func TestCodeIsSplitIntoLinesOfColoredTokens(t *testing.T) {
	s := syntax(t)
	lines := s.Lines("a.go", "package main\n\nconst n = 4")

	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3", len(lines))
	}
	if text(lines[0]) != "package main" {
		t.Errorf("first line = %q", text(lines[0]))
	}
	if text(lines[1]) != "" {
		t.Errorf("blank line = %q, want empty", text(lines[1]))
	}
	if text(lines[2]) != "const n = 4" {
		t.Errorf("third line = %q", text(lines[2]))
	}
}

// A keyword and a number are different colors in every style worth using. If
// the tokens come back all one color the lexer never ran.
func TestTokensCarryDifferentColorsWithinALine(t *testing.T) {
	s := syntax(t)
	seen := make(map[string]bool)
	for _, tok := range s.Lines("a.go", "const n = 4")[0] {
		if tok.Color != nil {
			seen[hex(tok.Color)] = true
		}
	}
	if len(seen) < 2 {
		t.Errorf("the line came back in %d colors, want the keyword and the number apart", len(seen))
	}
}

// The lexer is chosen from the path, so the same text has to come back
// differently coloured under a different extension.
func TestTheLexerFollowsTheFileName(t *testing.T) {
	s := syntax(t)
	goLine := colors(s.Lines("a.go", "package main")[0])
	txtLine := colors(s.Lines("a.txt", "package main")[0])

	if goLine == txtLine {
		t.Errorf("a.go and a.txt colored identically: %s", goLine)
	}
}

// Chroma's terminal formatter writes its own SGR, resets included, and a reset
// mid-line clears whatever background the caller put behind the row. Tokens are
// returned raw so the caller keeps that control.
func TestTokensCarryNoEscapeSequencesOfTheirOwn(t *testing.T) {
	s := syntax(t)
	for _, tok := range s.Lines("a.go", "const n = 4")[0] {
		if strings.Contains(tok.Text, "\x1b") {
			t.Errorf("token %q carries its own escape sequence", tok.Text)
		}
	}
}

// A style's own background would paint over the terminal's, which is what
// keeps a transparent one transparent.
func TestABackgroundStaysWithTheCaller(t *testing.T) {
	base := lipgloss.NewStyle().Background(theme.RosePineMoon.SelectedBackground)

	s := syntax(t)
	var b strings.Builder
	for _, tok := range s.Lines("a.go", "const n = 4")[0] {
		style := base
		if tok.Color != nil {
			style = base.Foreground(tok.Color)
		}
		b.WriteString(style.Render(tok.Text))
	}

	r, g, bl, _ := theme.RosePineMoon.SelectedBackground.RGBA()
	want := fmt.Sprintf("48;2;%d;%d;%d", r>>8, g>>8, bl>>8)
	if got := strings.Count(b.String(), want); got < 3 {
		t.Errorf("the caller's background survives %d runs, want it on every token", got)
	}
}

// Highlighting is done on an Update path and a diff is re-rendered on every
// resize and every scroll.
func TestTheSameFileIsOnlyTokenisedOnce(t *testing.T) {
	s := syntax(t)
	first := s.Lines("a.go", "const n = 4")
	second := s.Lines("a.go", "const n = 4")

	if &first[0] != &second[0] {
		t.Error("the second call re-tokenised instead of answering from the cache")
	}
}

func text(tokens []comp.Token) string {
	var b strings.Builder
	for _, tok := range tokens {
		b.WriteString(tok.Text)
	}
	return b.String()
}

func colors(tokens []comp.Token) string {
	out := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		out = append(out, hex(tok.Color))
	}
	return strings.Join(out, ",")
}

func hex(c color.Color) string {
	if c == nil {
		return "none"
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}
