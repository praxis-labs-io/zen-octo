package comp

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

func TestTheSameBodyIsOnlyRenderedOnce(t *testing.T) {
	m := NewMarkdown(testTheme)

	first := m.Render("# One", 60)
	if m.Render("# One", 60) != first {
		t.Error("the same body at the same width came back different")
	}
	if len(m.cache) != 1 {
		t.Errorf("cache holds %d entries, want 1", len(m.cache))
	}

	m.Render("# Two", 60)
	if len(m.cache) != 2 {
		t.Errorf("cache holds %d entries after a second body, want 2", len(m.cache))
	}
}

func TestAWidthChangeDropsWhatWasCached(t *testing.T) {
	m := NewMarkdown(testTheme)

	m.Render("# One", 60)
	m.Render("# Two", 60)
	m.Render("# One", 40)

	if len(m.cache) != 1 {
		t.Errorf("cache holds %d entries after a width change, want 1", len(m.cache))
	}
}

func TestOutputWrapsAtTheWidthItWasGiven(t *testing.T) {
	m := NewMarkdown(testTheme)
	body := strings.Repeat("some words that have to go somewhere ", 10)

	for _, width := range []int{40, 60, 100} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			for i, line := range strings.Split(m.Render(body, width), "\n") {
				if w := lipgloss.Width(line); w > width {
					t.Errorf("line %d is %d cells wide, want no more than %d", i, w, width)
				}
			}
		})
	}
}

func TestHeadingsTakeTheThemeAndNotGlamoursOwn(t *testing.T) {
	th := testTheme
	m := NewMarkdown(th)

	out := m.Render("# Heading\n\nA paragraph.", 60)

	styled := lipgloss.NewStyle().Foreground(th.Accent).Render("x")
	want := styled[:strings.Index(styled, "m")+1]
	if !strings.Contains(out, strings.TrimSuffix(want, "m")) {
		t.Errorf("no heading in the theme's accent: %q", out)
	}
}

func TestNothingToRenderComesBackEmpty(t *testing.T) {
	m := NewMarkdown(testTheme)

	tests := []struct {
		name  string
		body  string
		width int
	}{
		{name: "no body", body: "", width: 60},
		{name: "whitespace only", body: "   \n\n  ", width: 60},
		{name: "no width", body: "# Heading", width: 0},
		{name: "negative width", body: "# Heading", width: -4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.Render(tt.body, tt.width); got != "" {
				t.Errorf("Render() = %q, want empty", got)
			}
		})
	}
}

func TestASingleNewlineIsALineBreak(t *testing.T) {
	m := NewMarkdown(testTheme)

	lines := body(m.Render("this is a sentence\nand this is another", 60))
	if len(lines) != 2 {
		t.Fatalf("rendered %d lines, want two:\n%q", len(lines), lines)
	}
	if !strings.HasPrefix(lines[0], "this is a sentence") {
		t.Errorf("first line = %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "and this is another") {
		t.Errorf("second line = %q", lines[1])
	}
}

func TestABlankLineStillSeparatesParagraphs(t *testing.T) {
	m := NewMarkdown(testTheme)

	if got := body(m.Render("one\n\ntwo", 60)); len(got) != 3 {
		t.Errorf("rendered %d lines, want two paragraphs with a blank between:\n%q", len(got), got)
	}
}

func body(out string) []string {
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		lines = append(lines, strings.TrimRight(xansi.Strip(line), " "))
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
