package comp

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// These reach into the cache map, which has no interface of its own: the whole
// point of a cache is that the caller cannot tell it is there.

func TestTheSameBodyIsOnlyRenderedOnce(t *testing.T) {
	m := NewMarkdown(theme.RosePineMoon)

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

// Glamour wraps at a width, so an entry from another width is wrong rather than
// stale. Dropping the map beats keeping one dead entry per column a drag-resize
// passed through.
func TestAWidthChangeDropsWhatWasCached(t *testing.T) {
	m := NewMarkdown(theme.RosePineMoon)

	m.Render("# One", 60)
	m.Render("# Two", 60)
	m.Render("# One", 40)

	if len(m.cache) != 1 {
		t.Errorf("cache holds %d entries after a width change, want 1", len(m.cache))
	}
}

func TestOutputWrapsAtTheWidthItWasGiven(t *testing.T) {
	m := NewMarkdown(theme.RosePineMoon)
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

// Glamour ships its own palette. A heading in a color nobody configured is the
// whole reason this file builds a style config rather than patching one.
func TestHeadingsTakeTheThemeAndNotGlamoursOwn(t *testing.T) {
	th := theme.RosePineMoon
	m := NewMarkdown(th)

	out := m.Render("# Heading\n\nA paragraph.", 60)

	r, g, b, _ := th.Secondary.RGBA()
	want := fmt.Sprintf("38;2;%d;%d;%d", r>>8, g>>8, b>>8)
	if !strings.Contains(out, want) {
		t.Errorf("no heading in the theme's accent: %q", out)
	}
}

func TestNothingToRenderComesBackEmpty(t *testing.T) {
	m := NewMarkdown(theme.RosePineMoon)

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
