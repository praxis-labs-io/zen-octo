package prview_test

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/tui/prview"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// The conversation in sampleDetail, in the order tab walks it. The resolved
// thread is last and has no card, so it is not here; it is asserted on its own.
const (
	cardDescription = "drucial · opened this"
	cardComment     = "octobot · commented"
	cardReview      = "nkr · requested changes"
	cardThread      = "internal/gh/client.go:42"
)

// A screen opens with nothing focused. The reader came to read it.
func TestNothingIsFocusedUntilTabIsPressed(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 60)

	if got := focusedCard(t, m.View()); got != "" {
		t.Errorf("card %q is focused on open, want none", got)
	}
	if got := focusedCard(t, press(m, "tab").View()); !strings.HasPrefix(got, cardDescription) {
		t.Errorf("tab focused %q, want the description", got)
	}
}

// Tab walks the cards in the order they were written, and comes back round.
func TestTabWalksTheCardsInOrderAndWraps(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 60)

	want := []string{cardDescription, cardComment, cardReview, cardThread}
	for i, card := range want {
		presses := strings.Repeat("tab ", i+1)
		got := focusedCard(t, press(m, strings.Fields(presses)...).View())
		if !strings.HasPrefix(got, card) {
			t.Errorf("tab %d focused %q, want %q", i+1, got, card)
		}
	}

	// The fifth is the resolved thread, which is one line with no border to
	// take the accent, so the text carries it instead.
	fifth := press(m, "tab", "tab", "tab", "tab", "tab").View()
	if !strings.Contains(fifth, fgSeq(theme.RosePineMoon.Secondary)+"m✓ internal/store/store.go:88") {
		t.Error("the resolved thread is not marked when focus reaches it")
	}

	sixth := focusedCard(t, press(m, "tab", "tab", "tab", "tab", "tab", "tab").View())
	if !strings.HasPrefix(sixth, cardDescription) {
		t.Errorf("tab past the last card focused %q, want it back at the description", sixth)
	}
}

// Shift+tab walks the other way, and from nothing it takes the last card on
// screen rather than the first.
func TestShiftTabWalksBack(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 60)

	back := focusedCard(t, press(m, "tab", "tab", "shift+tab").View())
	if !strings.HasPrefix(back, cardDescription) {
		t.Errorf("shift+tab focused %q, want the description", back)
	}
}

// Focus does not survive being scrolled out of the window. A reader who
// scrolled away has moved on, and hauling them back to the card they left is
// the one thing the ring must not do.
func TestTabReanchorsToWhatIsOnScreen(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 200, 16), "tab", "tab", "tab", "tab")
	if got := focusedCard(t, m.View()); !strings.HasPrefix(got, cardThread) {
		t.Fatalf("focus started on %q, want the thread card", got)
	}

	// Back to the top, which the thread is a long way below.
	top := press(m, "g")
	if strings.Contains(stripANSI(top.View()), cardThread) {
		t.Fatal("the thread is still on screen, so nothing was re-anchored")
	}

	got := focusedCard(t, press(top, "tab").View())
	if !strings.HasPrefix(got, cardDescription) {
		t.Errorf("tab focused %q, want the first card in the window", got)
	}
}

// A card scrolled to goes to the top of the window. Landed at the foot of it,
// the replies the card is worth reading for are all below the fold.
func TestTabScrollsACardToTheTopOfTheWindow(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 16)

	if strings.Contains(stripANSI(m.View()), cardThread) {
		t.Fatal("the thread is already on screen, so this proves nothing")
	}

	// Four presses reach the thread card, which is well below the fold.
	got, at := focusedCardAt(t, press(m, "tab", "tab", "tab", "tab").View())
	if !strings.HasPrefix(got, cardThread) {
		t.Fatalf("focus landed on %q, want the thread card whole", got)
	}
	// Line zero is the pane's own border, so line one is the top of the window.
	if at != 1 {
		t.Errorf("the card's border landed on frame line %d, want line 1", at)
	}
}

// A card already on screen whole leaves the page alone. The highlight says
// where focus is, and scrolling under a reader who can see it is worse.
func TestTabDoesNotScrollACardAlreadyOnScreen(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 60)

	before := stripANSI(m.View())
	after := stripANSI(press(m, "tab", "tab").View())
	if before != after {
		t.Error("the page moved to focus a card that was already on screen whole")
	}
}

// A card taller than the window pins to its top. Bottom-aligning it opens on
// the end of a comment with the line saying whose it is above the window.
func TestTabPinsACardTallerThanTheWindowToItsTop(t *testing.T) {
	d := sampleDetail()
	d.Body = strings.Repeat("The retry path backs off forever.\n\n", 20)

	m := press(detailed(held(d), 200, 20), "tab")
	if got := focusedCard(t, m.View()); !strings.HasPrefix(got, cardDescription) {
		t.Errorf("focus landed on %q, want the description with its heading on screen", got)
	}
}

// The strip kept the brackets when the ring took tab.
func TestTabNoLongerSwitchesTabs(t *testing.T) {
	m := detailed(held(sampleDetail()), 160, 24)
	active := fgSeq(theme.RosePineMoon.Primary)

	if !strings.Contains(firstLine(press(m, "tab").View()), active+"mConversation") {
		t.Error("tab moved off the Conversation tab")
	}
	if !strings.Contains(firstLine(press(m, "shift+tab").View()), active+"mConversation") {
		t.Error("shift+tab moved off the Conversation tab")
	}
	if !strings.Contains(firstLine(press(m, "]").View()), active+"mCommits") {
		t.Error("] no longer switches tabs")
	}
}

// Letting go of a card and leaving the screen are two intentions on one key.
func TestEscLetsGoBeforeItBacksOut(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 200, 60), "tab")

	m, cmd := m.Update(escape())
	if cmd != nil {
		t.Error("esc backed out of the screen while a card was focused")
	}
	if got := focusedCard(t, m.View()); got != "" {
		t.Errorf("card %q is still focused after esc", got)
	}

	if _, cmd = m.Update(escape()); cmd == nil {
		t.Fatal("esc with nothing focused did not back out")
	}
	if _, ok := cmd().(prview.BackMsg); !ok {
		t.Errorf("esc sent %T, want a BackMsg", cmd())
	}
}

// A tab with a column shows no ring. Focus held over from the conversation is
// invisible there, and swallowing esc for it strands the reader on the screen.
func TestEscBacksOutFromATabWithNoRing(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 200, 60), "tab", "]")

	_, cmd := m.Update(escape())
	if cmd == nil {
		t.Fatal("esc on the Commits tab did not back out")
	}
	if _, ok := cmd().(prview.BackMsg); !ok {
		t.Errorf("esc sent %T, want a BackMsg", cmd())
	}
}

// The rail is walked by the same key. Its rows have no border to take the
// accent, so the row itself is painted, the way the file column paints its own.
func TestTabWalksTheRailRows(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 200, 44), "l")

	// The state row leads: it is the first thing in the column and the first
	// thing there is anything to do to.
	if got := markedRailRow(t, press(m, "tab").View()); !strings.Contains(got, "Open") {
		t.Errorf("tab marked %q, want the state row", got)
	}
	if got := markedRailRow(t, press(m, "tab", "tab").View()); got != "@nkr" {
		t.Errorf("a second tab marked %q, want the first reviewer", got)
	}

	// Nothing in the conversation is lit while the focus is in the rail.
	if got := focusedCard(t, press(m, "tab").View()); got != "" {
		t.Errorf("card %q is lit while the rail holds the focus", got)
	}
}

// The add row closes the Reviewers section rather than sitting under the
// section heading or at the end of the column.
func TestTheAddReviewerRowFollowsTheReviewers(t *testing.T) {
	rows := railRows(t, detailed(held(sampleDetail()), 200, 44).View())

	at := -1
	for i, row := range rows {
		if row == "Reviewers" {
			at = i
			break
		}
	}
	if at < 0 {
		t.Fatalf("no Reviewers section in the rail: %q", rows)
	}

	// Three reviewers in the fixture, then the row that adds a fourth.
	if got := rows[at+4]; got != "+ Add reviewer" {
		t.Errorf("the row after the reviewers is %q, want the add row", got)
	}

	// Five tabs: the state, three reviewers, then the add row.
	m := press(detailed(held(sampleDetail()), 200, 44), "l")
	if got := markedRailRow(t, press(m, "tab", "tab", "tab", "tab", "tab").View()); got != "+ Add reviewer" {
		t.Errorf("the fifth tab marked %q, want the add row", got)
	}
}

// The rail sections the reader can act on are walkable and the rest are not,
// so tab does not stop on the merge state on its way to the checks.
func TestTabSkipsTheRailRowsThereIsNothingToDoTo(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 200, 44), "l")

	seen := map[string]bool{}
	for i := range 16 {
		seen[markedRailRow(t, press(m, strings.Fields(strings.Repeat("tab ", i+1))...).View())] = true
	}

	// The state row leads with a glyph, so it is read by what it says.
	reached := func(text string) bool {
		for row := range seen {
			if strings.Contains(row, text) {
				return true
			}
		}
		return false
	}

	for _, want := range []string{
		"Open", "@nkr", "@drucial", "bug", "Rails Unit Tests / test", "Blocked",
		"+ Add reviewer", "+ Add assignee", "+ Add label",
	} {
		if !reached(want) {
			t.Errorf("tab never reached the %q row", want)
		}
	}
	for _, skip := range []string{"behind main", "+42"} {
		if reached(skip) {
			t.Errorf("tab stopped on %q, which there is nothing to do to", skip)
		}
	}
}

// A frame with a card focused is still exactly the size it was given.
func TestAFocusedFrameFillsItsSizeExactly(t *testing.T) {
	sizes := []struct{ width, height int }{
		{width: 200, height: 40},
		{width: 160, height: 24},
		{width: 100, height: 20},
		{width: 60, height: 10},
	}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			m := press(detailed(held(sampleDetail()), size.width, size.height), "tab", "tab", "tab")
			lines := strings.Split(m.View(), "\n")

			if len(lines) != size.height {
				t.Errorf("frame is %d lines, want %d", len(lines), size.height)
			}
			for i, line := range lines {
				if w := lipgloss.Width(line); w != size.width {
					t.Errorf("line %d is %d cells wide, want %d", i, w, size.width)
				}
			}
		})
	}
}

func escape() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyEscape, Text: "esc"}
}

// focusedCard is the heading of the card whose border is in the accent, or ""
// when no card holds the focus. The border is the only signal there is, and
// reading the heading under it proves the card is on the screen whole.
func focusedCard(t *testing.T, frame string) string {
	t.Helper()

	head, _ := focusedCardAt(t, frame)
	return head
}

// focusedCardAt is the same, with the frame line the card's top border landed
// on. Line zero is the pane's own top border, so a card at the top of the
// window sits on line one.
func focusedCardAt(t *testing.T, frame string) (string, int) {
	t.Helper()

	accent := fgSeq(theme.RosePineMoon.Secondary)
	lines := strings.Split(frame, "\n")

	for i, line := range lines {
		// A line opening with a corner is a pane's own border, not a card's.
		at := strings.Index(line, "╭")
		if at < 0 || i+1 >= len(lines) || strings.HasPrefix(stripANSI(line), "╭") {
			continue
		}
		start := strings.LastIndex(line[:at], "\x1b[")
		if start < 0 || !strings.HasPrefix(line[start+2:], accent) {
			continue
		}
		return cardHeading(stripANSI(lines[i+1]), stripANSI(line)), i
	}
	return "", -1
}

// cardHeading is the heading row read out of the card whose top border is on
// the line above it. The card sits at some column inside the pane, with the
// thread rail to its left and the details pane to its right, so it is cut out
// by the column its own border is in.
func cardHeading(row, border string) string {
	col := utf8.RuneCountInString(border[:strings.Index(border, "╭")])

	runes := []rune(row)
	if col+1 >= len(runes) {
		return ""
	}

	inner := string(runes[col+1:])
	if end := strings.Index(inner, "│"); end >= 0 {
		inner = inner[:end]
	}
	return strings.TrimSpace(inner)
}

// markedRailRow is the text of the rail row painted as the cursor line.
func markedRailRow(t *testing.T, frame string) string {
	t.Helper()

	for _, raw := range railRaw(t, frame) {
		if !strings.Contains(raw, bgSeq(theme.RosePineMoon.SelectedBackground)) {
			continue
		}
		return strings.TrimSpace(strings.Trim(stripANSI(raw), "│● "))
	}
	return ""
}
