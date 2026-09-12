package prview_test

import (
	"strings"
	"testing"
)

func barredRow(frame string) string {
	for _, line := range strings.Split(frame, "\n") {
		bare := stripANSI(line)
		if strings.HasPrefix(bare, "│") && strings.Contains(bare, "▌") {
			return strings.TrimSpace(bare)
		}
	}
	return ""
}

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

func TestTheCursorCrossesBetweenBlocks(t *testing.T) {
	m := press(onFiles(200, 50), "}")
	head := barredRow(m.View())

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

func TestABraceZeroesTheRowCursor(t *testing.T) {
	m := press(onFiles(200, 50), "}", "j", "j")
	if got := barredRow(m.View()); strings.Contains(got, "@@") {
		t.Fatalf("setup: the cursor is on %q, want a row of code", got)
	}

	if got := barredRow(press(m, "{", "}").View()); !strings.Contains(got, "@@ -40,4 +40,5 @@") {
		t.Errorf("the brace landed on %q, want the hunk's heading", got)
	}
}

func TestTheCursorScrollsThePaneToStayOnIt(t *testing.T) {
	m := press(onFiles(120, 14), "}")
	for i, want := range []string{
		"for {",
		"−         time.Sleep(delay)",
		"+         delay = min(delay*2, fetchTimeout)",
		"+         time.Sleep(delay)",
	} {
		m = press(m, "j")
		if got := barredRow(m.View()); !strings.Contains(got, want) {
			t.Fatalf("press %d barred %q, want %q on the frame", i+1, got, want)
		}
	}
}

func TestTheCursorWalksTheRepliesBeforeTheCodeUnderThem(t *testing.T) {
	m := press(onFiles(200, 50), "}", "}")
	if got := focusedCard(t, m.View()); !strings.Contains(got, "internal/gh/client.go:42") {
		t.Fatalf("setup: the second brace lit %q, want the thread card", got)
	}

	reply := press(m, "j")
	if got := focusedCard(t, reply.View()); !strings.Contains(got, "octobot") {
		t.Errorf("j off the thread card lit %q, want the reply hanging off it", got)
	}
	if got := barredRow(reply.View()); got != "" {
		t.Errorf("the reply took a bar as well as its border: %q", got)
	}

	code := press(reply, "j")
	if got := barredRow(code.View()); !strings.Contains(got, "42 43") {
		t.Errorf("j off the reply barred %q, want the code under the card", got)
	}
	if got := focusedCard(t, code.View()); got != "" {
		t.Errorf("the reply is still lit with the cursor on %q under it", got)
	}

	if got := focusedCard(t, press(code, "k").View()); !strings.Contains(got, "octobot") {
		t.Errorf("k off the code lit %q, want the reply above it", got)
	}
}

func TestJAtTheLastStopScrollsRatherThanStalling(t *testing.T) {
	m := press(onFiles(100, 14), "}", "j", "j", "j", "j", "j", "j", "j")
	if got := barredRow(m.View()); !strings.Contains(got, "42 43") {
		t.Fatalf("setup: the cursor is on %q, want the file's last row", got)
	}

	before := m.View()
	m = press(m, "j")
	if m.View() == before {
		t.Error("j at the last stop neither moved the cursor nor scrolled the pane")
	}
	if got := barredRow(m.View()); !strings.Contains(got, "42 43") {
		t.Errorf("the scroll left the cursor on %q, want it still on its own row", got)
	}
}

func TestAFoldedHunkOffersTheCursorNoRows(t *testing.T) {
	m := press(onFiles(200, 50), "}")
	if got := barredRow(m.View()); !strings.Contains(got, "@@ -40,4 +40,5 @@") {
		t.Fatalf("setup: the brace barred %q, want the hunk's heading", got)
	}

	m = press(m, "space", "j", "j")
	got := barredRow(m.View())
	if strings.Contains(got, "time.Sleep(delay)") {
		t.Errorf("j barred %q, which the fold took off the page", got)
	}
	if !strings.Contains(got, "@@ -40,4 +40,5 @@") {
		t.Errorf("j left the folded heading for %q", got)
	}
}
