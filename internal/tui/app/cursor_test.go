package app_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/app"
	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

// filteringPicker opens a label picker long enough to earn a filter row. The
// shared fixture carries two labels, and a picker shorter than pickerFilterFrom
// takes no text at all, so there would be nothing to put a cursor in.
func filteringPicker(t *testing.T, client *fakeSearcher) tea.Model {
	t.Helper()

	many := make([]gh.Label, 0, 9)
	for _, name := range []string{"bug", "build", "chore", "docs", "design", "infra", "perf", "test", "ux"} {
		many = append(many, gh.Label{ID: "LA_" + name, Name: name})
	}

	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveLabels("PR_412", many[:1])
	client.serveRepoMeta(gh.RepoMeta{Labels: many})

	m := press(loaded(t, client, 160, 40), "enter", "1", "j", "j", "j", "enter")
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Type to filter") {
		t.Fatalf("the picker opened without a filter row\n%s", out)
	}
	return m
}

// noticing is the app with the config notice up, which is the one thing that
// puts a row above the screen. An unknown theme name is what raises it.
func noticing(t *testing.T, client *fakeSearcher, width, height int) tea.Model {
	t.Helper()

	cfg := testConfig()
	cfg.Theme = "not-a-theme"
	m := drive(t, app.New(cfg, client), tea.WindowSizeMsg{Width: width, Height: height})
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Unknown theme") {
		t.Fatalf("no notice on the frame\n%s", out)
	}
	return m
}

// cursorOf is the terminal cursor the root asks for. Nothing else in this suite
// reads a tea.View field other than Content, so this is the whole of the access.
func cursorOf(t *testing.T, m tea.Model) *tea.Cursor {
	t.Helper()
	return m.View().Cursor
}

// afterOnScreen is where a run of text ends in the rendered frame: the row it is
// drawn on and the column of the cell just past it.
//
// The cursor is asserted against the frame rather than against a number written
// down here. A layout that moves takes the expectation with it, where a literal
// would go on passing while pointing at the wrong cell.
func afterOnScreen(t *testing.T, frame, want string) (x, y int) {
	t.Helper()

	for row, line := range strings.Split(stripANSI(frame), "\n") {
		at := strings.Index(line, want)
		if at < 0 {
			continue
		}
		return lipgloss.Width(line[:at] + want), row
	}
	t.Fatalf("no line in the frame holds %q\n%s", want, stripANSI(frame))
	return 0, 0
}

// wantCursorAfter holds the one property every input on this screen shares: the
// terminal's cursor sits in the cell after what has been typed, on the row that
// shows it.
func wantCursorAfter(t *testing.T, m tea.Model, typed string) {
	t.Helper()

	c := cursorOf(t, m)
	if c == nil {
		t.Fatalf("no cursor while %q is being typed\n%s", typed, stripANSI(render(t, m)))
	}

	x, y := afterOnScreen(t, render(t, m), typed)
	if c.X != x || c.Y != y {
		t.Errorf("cursor at (%d,%d), want (%d,%d), after %q\n%s",
			c.X, c.Y, x, y, typed, stripANSI(render(t, m)))
	}
}

func TestTheCursorSitsAfterWhatHasBeenTypedInEveryBox(t *testing.T) {
	t.Run("list search", func(t *testing.T) {
		m := write(press(loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40), "/"), "auth")
		wantCursorAfter(t, m, "/ auth")
	})

	t.Run("compose box", func(t *testing.T) {
		wantCursorAfter(t, composed(t, &fakeSearcher{prs: samplePRs()}, "looks good"), "looks good")
	})

	// zq rather than a word: the query has to be a run that appears nowhere
	// else on the frame, or the row it is found on is not the one it was typed
	// into.
	t.Run("picker filter", func(t *testing.T) {
		wantCursorAfter(t, write(filteringPicker(t, &fakeSearcher{prs: samplePRs()}), "zq"), "zq")
	})

	t.Run("merge headline", func(t *testing.T) {
		// The form opens on the method chooser, which takes no text. One tab
		// hands the keyboard to the headline, whose caret opens at the start of
		// the message GitHub prefilled.
		m := write(press(openMergeForm(t, &fakeSearcher{prs: samplePRs()}), "tab"), "zq")
		wantCursorAfter(t, m, "zq")
	})

	t.Run("merge message", func(t *testing.T) {
		m := write(press(openMergeForm(t, &fakeSearcher{prs: samplePRs()}), "tab", "tab"), "zq")
		wantCursorAfter(t, m, "zq")
	})
}

// A box that is not taking text has no cursor to place. A drawn caret could be
// left behind; the terminal's cannot, because there is only the one.
func TestNothingTakingTextMeansNoCursor(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	tests := []struct {
		name string
		m    func() tea.Model
	}{
		{name: "the list at rest", m: func() tea.Model {
			return loaded(t, client, 120, 40)
		}},
		{name: "a filter settled with enter", m: func() tea.Model {
			return press(write(press(loaded(t, client, 120, 40), "/"), "auth"), "enter")
		}},
		{name: "the search closed with esc", m: func() tea.Model {
			return press(write(press(loaded(t, client, 120, 40), "/"), "auth"), "esc")
		}},
		{name: "the help overlay", m: func() tea.Model {
			return press(loaded(t, client, 120, 40), "?")
		}},
		{name: "below the floor", m: func() tea.Model {
			return settle(press(loaded(t, client, 120, 40), "/"),
				tea.WindowSizeMsg{Width: 30, Height: 8})
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if c := cursorOf(t, tt.m()); c != nil {
				t.Errorf("cursor at (%d,%d), want none", c.X, c.Y)
			}
		})
	}
}

// The notice is a row above the screen, and a screen reports against its own
// frame. Nothing else separates the two, which is why this is the one thing the
// local-to-absolute step can get wrong, and why it is invisible without one up.
func TestANoticeMovesTheCursorDownExactlyOneRow(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	plain := write(press(loaded(t, client, 120, 40), "/"), "auth")
	before := cursorOf(t, plain)
	if before == nil {
		t.Fatal("no cursor to move")
	}

	noticed := write(press(noticing(t, client, 120, 40), "/"), "auth")
	after := cursorOf(t, noticed)
	if after == nil {
		t.Fatalf("no cursor under a notice\n%s", stripANSI(render(t, noticed)))
	}

	if after.Y != before.Y+1 || after.X != before.X {
		t.Errorf("cursor moved from (%d,%d) to (%d,%d), want one row down and no more",
			before.X, before.Y, after.X, after.Y)
	}
}

// It is muted rather than accent, and it blinks. The blink is the terminal's,
// so it costs no message and no repaint; a drawn caret could only blink by
// re-rendering the frame it sits in.
func TestTheCursorIsMutedAndBlinks(t *testing.T) {
	m := write(press(loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40), "/"), "auth")

	c := cursorOf(t, m)
	if c == nil {
		t.Fatal("no cursor to read")
	}
	if !c.Blink {
		t.Error("the cursor does not blink")
	}
	if c.Color != theme.RosePineMoon.MutedOrSubtle() {
		t.Errorf("cursor colour is %v, want the theme's muted", c.Color)
	}
}
