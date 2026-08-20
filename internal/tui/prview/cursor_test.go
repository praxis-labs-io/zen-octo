package prview_test

import (
	"strings"
	"testing"
)

// barredRow is the row the cursor is on, read off the bar it paints in the
// leading cell. The fill is shared with a lit card and the bar is not.
func barredRow(frame string) string {
	for _, line := range strings.Split(frame, "\n") {
		if strings.Contains(stripANSI(line), "▌") {
			return strings.TrimSpace(stripANSI(line))
		}
	}
	return ""
}

// The braces land on a block and j walks the code under it. Without the second
// half a reader can point at a hunk and at nothing inside it.
func TestJWalksTheRowsUnderTheLitHunk(t *testing.T) {
	m := onFiles(200, 50)
	if got := barredRow(m.View()); got != "" {
		t.Fatalf("the tab opened with %q barred, want nothing", got)
	}

	m = press(m, "}")
	if got := barredRow(m.View()); !strings.Contains(got, "@@ -40,4 +40,5 @@") {
		t.Fatalf("the brace barred %q, want the hunk's own heading", got)
	}

	m = press(m, "j")
	first := barredRow(m.View())
	if first == "" || strings.Contains(first, "@@") {
		t.Fatalf("j barred %q, want the first row of code under the heading", first)
	}
	if got := litHunk(m.View()); got != "" {
		t.Errorf("the heading is still filled with the cursor on %q under it", first)
	}

	if got := barredRow(press(m, "j").View()); got == first {
		t.Errorf("a second j stayed on %q", got)
	}
	if got := barredRow(press(m, "j", "k").View()); got != first {
		t.Errorf("j then k landed on %q, want %q", got, first)
	}
}

// A block runs out and the cursor steps to the next one, forward onto its head
// and back onto its last row.
func TestTheCursorCrossesBetweenBlocks(t *testing.T) {
	m := press(onFiles(200, 50), "}")
	head := barredRow(m.View())

	// Four rows of code under the hunk, then the thread written against it.
	last := press(m, "j", "j", "j", "j")
	if got := barredRow(last.View()); !strings.Contains(got, "time.Sleep(delay)") {
		t.Fatalf("setup: the fourth row is %q, want the hunk's last line", got)
	}

	card := press(last, "j")
	if got := focusedCard(t, card.View()); !strings.Contains(got, "internal/gh/client.go:42") {
		t.Errorf("stepping off the hunk lit %q, want the thread under it", got)
	}
	if got := barredRow(card.View()); got != "" {
		t.Errorf("the card took a bar as well as its border: %q", got)
	}
	if litHunk(card.View()) != "" {
		t.Error("stepping off the hunk left its heading filled")
	}

	if got := barredRow(press(card, "k").View()); !strings.Contains(got, "time.Sleep(delay)") {
		t.Errorf("k off the card landed on %q, want the hunk's last row", got)
	}
	if got := barredRow(press(m, "k").View()); got != head {
		t.Errorf("k at the head of the first block moved to %q", got)
	}
}

// The braces name a block, so one pressed from inside another lands on its own
// heading rather than carrying the row the cursor had walked to.
func TestABraceZeroesTheRowCursor(t *testing.T) {
	m := press(onFiles(200, 50), "}", "j", "j")
	if got := barredRow(m.View()); strings.Contains(got, "@@") {
		t.Fatalf("setup: the cursor is on %q, want a row of code", got)
	}

	if got := barredRow(press(m, "{", "}").View()); !strings.Contains(got, "@@ -40,4 +40,5 @@") {
		t.Errorf("the brace landed on %q, want the hunk's heading", got)
	}
}

// The cursor walking down a pane shorter than the file has to bring its own row
// with it, or the reader is moving something they cannot see.
func TestTheCursorScrollsThePaneToStayOnIt(t *testing.T) {
	m := press(onFiles(120, 14), "}")
	for range 12 {
		m = press(m, "j")
		if barredRow(m.View()) == "" && litHunk(m.View()) == "" && focusedCard(t, m.View()) == "" {
			t.Fatalf("the cursor walked off the frame:\n%s", m.View())
		}
	}
}
