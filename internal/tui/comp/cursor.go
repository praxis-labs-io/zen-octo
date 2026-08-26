package comp

import (
	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

// Cursor is the terminal's own cursor, placed. Every text input in this app
// reports a position and none of them draws a caret: the terminal has exactly
// one cursor, so there is nothing to keep in step. Five carets styled to match
// is the arrangement that let one of them drift onto paint.BarGlyph, which
// already means something else.
//
// It blinks, and that is free. A drawn caret blinks by re-rendering, which is a
// message every half second and a relayout behind it; this one blinks in the
// terminal emulator, where nothing in this process can see it and nothing
// crosses an ssh connection to do it.
//
// Muted rather than accent: it marks where the next character lands, which is
// chrome, and every focused box already says it has the keyboard with its
// border.
func Cursor(th theme.Theme, x, y int) *tea.Cursor {
	c := tea.NewCursor(x, y)
	c.Shape = tea.CursorBlock
	c.Blink = true
	c.Color = th.MutedOrSubtle()
	return c
}

// Offset moves a cursor by the origin of whatever drew it. A widget reports a
// position inside itself and cannot know where it was placed; only the caller
// stacking it into a frame knows that.
//
// Nil in, nil out: a widget with no cursor to give is the common case, and
// making every caller guard would spread the same check over seven sites.
func Offset(c *tea.Cursor, dx, dy int) *tea.Cursor {
	if c == nil {
		return nil
	}
	moved := *c
	moved.X += dx
	moved.Y += dy
	return &moved
}
