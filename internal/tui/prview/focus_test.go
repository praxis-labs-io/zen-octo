package prview_test

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/paint"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

const (
	cardDescription = "drucial · opened this"
	cardComment     = "octobot · commented"
	cardReview      = "nkr · requested changes"
	cardThread      = "internal/gh/client.go:42"
	cardLocked      = "internal/tui/app/app.go:12"
	cardCompose     = "write a comment"
)

func TestAScreenOpensWithItsCursorOnTheDescription(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 60)

	if got := focusedCard(t, m.View()); !strings.HasPrefix(got, cardDescription) {
		t.Errorf("card %q is focused on open, want the description", got)
	}
	if got := focusedCard(t, press(m, "}").View()); !strings.HasPrefix(got, cardComment) {
		t.Errorf("the first brace focused %q, want it to have moved on", got)
	}
}

func TestTheLeadingPaneHoldsTheKeysOnArrival(t *testing.T) {
	m := opened(held(sampleDetail()), 200, 44)

	if got := markedRailRow(t, m.View()); got == "" {
		t.Error("the rail leads the row and holds no cursor on open")
	}
	if got := focusedCard(t, m.View()); got != "" {
		t.Errorf("card %q is lit while the rail holds the keys", got)
	}
}

func TestTheStripLeavesTheKeysWhereTheReaderPutThem(t *testing.T) {
	m := press(opened(held(sampleDetail()), 200, 44), "l")
	if got := focusedCard(t, m.View()); !strings.HasPrefix(got, cardDescription) {
		t.Fatalf("setup: the page holds %q, want the description", got)
	}

	back := press(m, "]", "]", "]", "]")
	if got := markedRailRow(t, back.View()); got != "" {
		t.Errorf("the strip handed the keys back to the rail, which lit %q", got)
	}
	if got := focusedCard(t, back.View()); !strings.HasPrefix(got, cardDescription) {
		t.Errorf("the page came back holding %q, want the card the reader left on", got)
	}
}

func TestWideningAFrameDoesNotTakeTheKeys(t *testing.T) {
	m := opened(held(sampleDetail()), 100, 40)
	if got := markedRailRow(t, m.View()); got != "" {
		t.Fatalf("setup: a frame under railMinFrame drew a rail cursor at %q", got)
	}
	if got := focusedCard(t, m.View()); !strings.HasPrefix(got, cardDescription) {
		t.Fatalf("setup: the only pane holds %q, want the description", got)
	}

	m.SetSize(200, 40)
	if got := markedRailRow(t, m.View()); got != "" {
		t.Errorf("widening handed the keys to the rail, which lit %q", got)
	}
	if got := focusedCard(t, m.View()); !strings.HasPrefix(got, cardDescription) {
		t.Errorf("the page holds %q after the widen, want the card the reader was on", got)
	}
}

func TestTheRailCursorCarriesTheBar(t *testing.T) {
	m := opened(held(sampleDetail()), 200, 44)

	on := markedRailRow(t, m.View())
	lit := ""
	for _, raw := range railRaw(t, m.View()) {
		if strings.Contains(stripANSI(raw), on) {
			lit = raw
			break
		}
	}
	if lit == "" {
		t.Fatalf("no rail row reading %q", on)
	}

	if !strings.Contains(stripANSI(lit), paint.BarGlyph) {
		t.Errorf("the rail's cursor line carries no bar: %q", stripANSI(lit))
	}
	if !strings.Contains(lit, fgSeq(testTheme.Accent)) {
		t.Error("the bar is not in the accent the diff draws its own in")
	}

	if !strings.Contains(stripANSI(lit), paint.BarGlyph+" ") {
		t.Errorf("the content sits against the bar: %q", stripANSI(lit))
	}

	bars := func(frame string) int {
		n := 0
		for _, row := range railRaw(t, frame) {
			row = stripANSI(row)
			if c := strings.Count(row, paint.BarGlyph); c > 1 {
				t.Errorf("row %q carries more than one bar", row)
			} else {
				n += c
			}
		}
		return n
	}
	if got := bars(m.View()); got != 1 {
		t.Errorf("the rail carries %d bars, want the one under the cursor", got)
	}

	if got := bars(press(m, "l").View()); got != 0 {
		t.Error("the bar is still on the rail once the page took the keys")
	}
}

func TestTheRailGivesUpItsCursorWhenItGivesUpTheKeys(t *testing.T) {
	m := opened(held(sampleDetail()), 200, 44)
	if markedRailRow(t, m.View()) == "" {
		t.Fatal("setup: the rail has no cursor to give up")
	}

	page := press(m, "l")
	if got := markedRailRow(t, page.View()); got != "" {
		t.Errorf("rail row %q is still lit once the page took the keys", got)
	}
	if got := focusedCard(t, page.View()); !strings.HasPrefix(got, cardDescription) {
		t.Errorf("the page took the keys and lit %q, want the description", got)
	}

	if got := markedRailRow(t, press(page, "h").View()); got == "" {
		t.Error("the rail took the keys back and lit nothing")
	}
}

func walked(m prview.Model, n int) prview.Model {
	m = press(m, "2")
	return press(m, strings.Fields(strings.Repeat("} ", max(0, n-1)))...)
}

func fromTop(m prview.Model) prview.Model {
	return press(press(m, "g"), strings.Fields(strings.Repeat("{ ", 40))...)
}

func TestTheRingWalksTheCardsInOrder(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 60)

	want := []string{cardDescription, cardComment, cardReview, cardThread}
	for i, card := range want {
		got := focusedCard(t, walked(m, i+1).View())
		if !strings.HasPrefix(got, card) {
			t.Errorf("step %d focused %q, want %q", i+1, got, card)
		}
	}

	if got := focusedCard(t, walked(m, 5).View()); !strings.HasPrefix(got, "octobot · said") {
		t.Errorf("the fifth step focused %q, want the reply on the thread", got)
	}

	if got := focusedCard(t, walked(m, 6).View()); !strings.HasPrefix(got, "✓ internal/store/store.go:88") {
		t.Errorf("the sixth step focused %q, want the resolved thread", got)
	}

	if got := focusedCard(t, walked(m, 7).View()); !strings.HasPrefix(got, cardLocked) {
		t.Errorf("the seventh step focused %q, want the unowned thread", got)
	}

	if got := focusedCard(t, walked(m, 9).View()); !strings.HasPrefix(got, cardCompose) {
		t.Errorf("the ninth step focused %q, want the comment box", got)
	}

	if got := focusedCard(t, walked(m, 10).View()); !strings.HasPrefix(got, cardCompose) {
		t.Errorf("a step past the last card focused %q, want it to stay on the comment box", got)
	}
}

func TestTheRingStopsAtTheFirstCard(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 200, 60), "}", "{")

	if got := focusedCard(t, m.View()); !strings.HasPrefix(got, cardDescription) {
		t.Fatalf("setup: focus landed on %q, want the first card", got)
	}
	if got := focusedCard(t, press(m, "{").View()); !strings.HasPrefix(got, cardDescription) {
		t.Errorf("a step before the first card focused %q, want it to stay put", got)
	}
}

func TestTheRingWalksBack(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 60)

	back := focusedCard(t, press(m, "}", "{").View())
	if !strings.HasPrefix(back, cardDescription) {
		t.Errorf("the ring back focused %q, want the description", back)
	}
}

func TestTheRingReanchorsToWhatIsOnScreen(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 200, 16), "}", "}", "}")
	if got := focusedCard(t, m.View()); !strings.HasPrefix(got, cardThread) {
		t.Fatalf("focus started on %q, want the thread card", got)
	}

	top := press(m, "g")
	if strings.Contains(stripANSI(top.View()), cardThread) {
		t.Fatal("the thread is still on screen, so nothing was re-anchored")
	}

	got := focusedCard(t, press(top, "}").View())
	if !strings.HasPrefix(got, cardDescription) {
		t.Errorf("the ring focused %q, want the first card in the window", got)
	}
}

func TestTheRingScrollsACardToTheTopOfTheWindow(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 16)

	if strings.Contains(stripANSI(m.View()), cardThread) {
		t.Fatal("the thread is already on screen, so this proves nothing")
	}

	got, at := focusedCardAt(t, press(m, "}", "}", "}").View())
	if !strings.HasPrefix(got, cardThread) {
		t.Fatalf("focus landed on %q, want the thread card whole", got)
	}
	if at != 1 {
		t.Errorf("the card's border landed on pane row %d, want row 1", at)
	}
}

func TestTheRingDoesNotScrollACardAlreadyOnScreen(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 60)

	before := lineOf(t, m.View(), cardDescription)
	after := lineOf(t, press(m, "}", "}").View(), cardDescription)
	if before != after {
		t.Errorf("the description moved from line %d to %d to focus a card already on screen whole", before, after)
	}
}

func lineOf(t *testing.T, frame, want string) int {
	t.Helper()

	for i, line := range strings.Split(stripANSI(frame), "\n") {
		if strings.Contains(line, want) {
			return i
		}
	}
	t.Fatalf("%q is not on the frame", want)
	return -1
}

func TestTheRingPinsACardTallerThanTheWindowToItsTop(t *testing.T) {
	d := sampleDetail()
	d.Body = strings.Repeat("The retry path backs off forever.\n\n", 20)

	m := detailed(held(d), 200, 20)
	if got := focusedCard(t, m.View()); !strings.HasPrefix(got, cardDescription) {
		t.Errorf("focus landed on %q, want the description with its heading on screen", got)
	}
}

func TestUnfoldingAThreadReachesTheDiff(t *testing.T) {
	d := sampleDetail()
	d.Threads[0].Comments[0].Body = "Look.\n\n<details>\n<summary>What it does</summary>\n\nIt retries forever.\n\n</details>\n"

	m := detailed(held(d), 200, 60)
	m.SetFiles(loadedFiles(sampleFiles(), 0))

	onFiles := press(m, "]", "]", "]")
	if !strings.Contains(stripANSI(onFiles.View()), "▸ What it does") {
		t.Fatal("the diff is not showing the thread's fold")
	}

	m = press(m, "}", "}", "}", "K", "space")
	if !strings.Contains(stripANSI(m.View()), "It retries forever") {
		t.Fatal("o did not unfold the thread in the conversation")
	}

	if !strings.Contains(stripANSI(press(m, "]", "]", "]").View()), "It retries forever") {
		t.Error("the diff still shows the thread folded")
	}
}

func TestTheEmptyChecksNoteIsNotWalkable(t *testing.T) {
	d := sampleDetail()
	d.Rollup = gh.CheckRollup{}

	m := press(detailed(held(d), 200, 44), "h")
	if got := markedRailRow(t, m.View()); strings.Contains(got, "None yet") {
		t.Error("the cursor landed on the empty checks note")
	}

	seen := map[string]bool{}
	for i := range 14 {
		seen[markedRailRow(t, press(m, strings.Fields(strings.Repeat("j ", i))...).View())] = true
	}
	if seen["None yet"] {
		t.Error("the cursor walks the empty checks note")
	}
}

func TestARowWithNoMarkKeepsTheCellsTheMarkWouldHaveTaken(t *testing.T) {
	d := sampleDetail()
	d.Assignees = []gh.Actor{{Login: strings.Repeat("a", 31)}}

	rows := railRows(t, detailed(held(d), 200, 44).View())
	for i, row := range rows {
		if row != "Assignees" {
			continue
		}
		if got := rows[i+1]; strings.HasSuffix(got, "…") {
			t.Errorf("assignee row = %q, want the name whole", got)
		}
		return
	}
	t.Fatalf("no Assignees section in the rail: %q", rows)
}

func tall() prview.Model {
	d := sampleDetail()
	d.Body = strings.Repeat("The retry path backs off forever.\n\n", 20)

	return detailed(held(d), 200, 20)
}

func scrolledIn(t *testing.T, m prview.Model) prview.Model {
	t.Helper()

	m = press(m, strings.Fields(strings.Repeat("j ", 12))...)
	if strings.Contains(stripANSI(m.View()), cardDescription) {
		t.Fatal("setup: the byline is still on screen, so this proves nothing")
	}
	return m
}

func TestTheBraceForwardFromInsideACardLeavesIt(t *testing.T) {
	m := press(scrolledIn(t, tall()), "}")

	if got := focusedCard(t, m.View()); !strings.HasPrefix(got, cardComment) {
		t.Errorf("the ring focused %q, want the card after the one the window is full of", got)
	}
	if strings.Contains(stripANSI(m.View()), cardDescription) {
		t.Error("} scrolled back up to the description's byline")
	}
}

func TestTheBraceBackFromInsideACardOpensOnItsByline(t *testing.T) {
	got, at := focusedCardAt(t, press(scrolledIn(t, tall()), "{").View())
	if !strings.HasPrefix(got, cardDescription) {
		t.Fatalf("the ring focused %q, want the card the window is full of", got)
	}
	if at != 1 {
		t.Errorf("the card's border landed on pane row %d, want the top of the window", at)
	}
}

func TestTheBraceBackReentersOnTheLastCardWholeOnScreen(t *testing.T) {
	d := sampleDetail()
	d.Body = strings.Repeat("The retry path backs off forever.\n\n", 12)

	m := press(detailed(held(d), 200, 40), strings.Fields(strings.Repeat("j ", 12))...)
	if strings.Contains(stripANSI(m.View()), cardDescription) {
		t.Fatal("setup: the description's byline is still on screen")
	}
	if strings.Contains(stripANSI(m.View()), "octobot · said") {
		t.Fatal("setup: the thread is whole on screen, so it is the card { should take")
	}

	before := lineOf(t, m.View(), cardReview)
	after := press(m, "{")

	if got := focusedCard(t, after.View()); !strings.HasPrefix(got, cardReview) {
		t.Errorf("{ landed on %q, want the last card whole on the screen", got)
	}
	if now := lineOf(t, after.View(), cardReview); now != before {
		t.Errorf("the page moved from line %d to %d to light a card already on screen whole", before, now)
	}
}

func TestAFocusScrolledOffItsBylineStopsBeingTheStep(t *testing.T) {
	m := tall()
	if got := focusedCard(t, m.View()); !strings.HasPrefix(got, cardDescription) {
		t.Fatalf("setup: focus landed on %q, want the description", got)
	}

	if got := focusedCard(t, press(scrolledIn(t, m), "{").View()); !strings.HasPrefix(got, cardDescription) {
		t.Errorf("{ landed on %q, want the byline of the card the reader is in", got)
	}
}

func TestTheRingAtTheTopOfAScrollablePaneDoesNotMoveThePage(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 24)

	row := func(frame string) string {
		lines := strings.Split(stripANSI(frame), "\n")
		return strings.TrimSpace(strings.Trim(lines[paneTopAt(frame)+1], "│ "))
	}
	if got := row(m.View()); got != "" {
		t.Fatalf("the pane opens on %q, want its blank line", got)
	}
	if got := row(press(m, "}").View()); got != "" {
		t.Errorf("after a step the pane opens on %q, want it not to have moved", got)
	}
}

func TestEscBacksOutWhenTheFocusIsOffScreen(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 200, 16), "}", "G")
	if strings.Contains(stripANSI(m.View()), cardDescription) {
		t.Fatal("the focused card is still on screen, so this proves nothing")
	}

	_, cmd := m.Update(escape())
	if cmd == nil {
		t.Fatal("esc was swallowed by a focus off the screen")
	}
	if _, ok := cmd().(prview.BackMsg); !ok {
		t.Errorf("esc sent %T, want a BackMsg", cmd())
	}
}

func TestOLeavesThePageAloneWhenTheFocusIsOffScreen(t *testing.T) {
	d := sampleDetail()
	d.Body = "Look.\n\n<details>\n<summary>Hidden</summary>\n\nThe secret.\n\n</details>\n"

	m := press(detailed(held(d), 200, 16), "}", "G")

	before := stripANSI(m.View())
	if before != stripANSI(press(m, "space").View()) {
		t.Error("o acted on a card off the screen")
	}
}

func TestOnlyThePaneHoldingTheKeysPaintsItsFocus(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 44)
	if got := focusedCard(t, m.View()); !strings.HasPrefix(got, cardDescription) {
		t.Fatalf("focus started on %q, want the description", got)
	}

	rail := press(m, "h", "}")
	if got := focusedCard(t, rail.View()); got != "" {
		t.Errorf("card %q is lit while the rail holds the keys", got)
	}
	if markedRailRow(t, rail.View()) == "" {
		t.Fatal("the rail row is not painted at all")
	}

	if got := markedRailRow(t, press(rail, "l").View()); got != "" {
		t.Errorf("rail row %q is lit while the conversation holds the keys", got)
	}
}

func TestTabStepsTheColumnThatDrivesThePane(t *testing.T) {
	d := sampleDetail()
	d.Commits = sampleCommits()

	m := press(detailed(held(d), 200, 40), "]")
	if got := conversationBorder(t, press(m, "2").View()); got != fgSeq(testTheme.Accent) {
		t.Fatal("setup: 2 did not put the keys on the page")
	}
	m = press(m, "2")

	before := stripANSI(selectedRow(m.View()))
	after := stripANSI(selectedRow(press(m, "tab").View()))
	if before == "" || after == "" {
		t.Fatalf("no row lit in the column: %q then %q", before, after)
	}
	if before == after {
		t.Errorf("tab left the cursor on %q, want the next commit", before)
	}
	if back := stripANSI(selectedRow(press(m, "tab", "shift+tab").View())); back != before {
		t.Errorf("shift+tab landed on %q, want back on %q", back, before)
	}

	if got := conversationBorder(t, press(m, "tab").View()); got != fgSeq(testTheme.Accent) {
		t.Error("tab took the keys to the column it stepped")
	}
}

func TestTabIsInertOnTheConversation(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 200, 40), "2")

	before := m.View()
	for _, k := range []string{"tab", "shift+tab"} {
		if got := press(m, k).View(); got != before {
			t.Errorf("%q moved something on the conversation", k)
		}
	}
}

func TestATabGivesBackThePaneItWasLeftOn(t *testing.T) {
	var (
		lit  = fgSeq(testTheme.Accent)
		idle = fgSeq(testTheme.BorderSubtle)
	)

	m := opened(held(sampleDetail()), 200, 40)
	if got := conversationBorder(t, m.View()); got != idle {
		t.Fatalf("setup: the page holds the keys on arrival, want the rail")
	}
	if got := conversationBorder(t, press(m, "]", "[").View()); got != idle {
		t.Error("the round trip took the keys off the rail")
	}

	page := press(m, "2")
	if got := conversationBorder(t, page.View()); got != lit {
		t.Fatalf("setup: 2 did not put the keys on the page")
	}
	if got := conversationBorder(t, press(page, "]", "[").View()); got != lit {
		t.Error("the round trip took the keys off the page")
	}
}

func TestACommitsColumnIsTakenOnArrivalAndNotAgain(t *testing.T) {
	idle := fgSeq(testTheme.BorderSubtle)

	m := press(opened(held(sampleDetail()), 200, 40), "]")
	if got := conversationBorder(t, m.View()); got != idle {
		t.Fatal("setup: Commits did not take its column on arrival")
	}

	m = press(m, "2")
	if got := conversationBorder(t, press(m, "[", "]").View()); got == idle {
		t.Error("Commits took its column again on a tab the reader had left")
	}
}

func TestOnlyTheBracketsMoveTheTabStrip(t *testing.T) {
	m := detailed(held(sampleDetail()), 160, 24)

	if got := currentTab(t, press(m, "]").View()); got != "Commits" {
		t.Errorf("] moved to %q, want Commits", got)
	}
	if got := currentTab(t, press(m, "[").View()); got != "Files" {
		t.Errorf("[ wrapped to %q, want Files", got)
	}
	for _, k := range []string{"}", "{", "tab", "shift+tab"} {
		if got := currentTab(t, press(m, k).View()); got != "Conversation" {
			t.Errorf("%q moved off the Conversation tab, to %q", k, got)
		}
	}
}

func TestEscBacksOutWithACardFocused(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 200, 60), "}")
	if got := focusedCard(t, m.View()); got == "" {
		t.Fatal("setup: no card is focused")
	}

	_, cmd := m.Update(escape())
	if cmd == nil {
		t.Fatal("esc did not back out while a card was focused")
	}
	if _, ok := cmd().(prview.BackMsg); !ok {
		t.Errorf("esc sent %T, want a BackMsg", cmd())
	}
}

func TestEscBacksOutFromATabWithNoRing(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 200, 60), "}", "]")

	_, cmd := m.Update(escape())
	if cmd == nil {
		t.Fatal("esc on the Commits tab did not back out")
	}
	if _, ok := cmd().(prview.BackMsg); !ok {
		t.Errorf("esc sent %T, want a BackMsg", cmd())
	}
}

func TestTheRailCursorWalksItsRowsOnTheMovementKeys(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 200, 44), "h")

	if got := markedRailRow(t, m.View()); !strings.Contains(got, "Open") {
		t.Errorf("taking the rail marked %q, want the state row", got)
	}
	for _, k := range []string{"j", "down"} {
		if got := markedRailRow(t, press(m, k).View()); got != "@nkr" {
			t.Errorf("%q marked %q, want the first reviewer", k, got)
		}
	}

	if got := focusedCard(t, m.View()); got != "" {
		t.Errorf("card %q is lit while the rail holds the focus", got)
	}
}

func TestTheRailCursorStopsAtItsEnds(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 200, 20), "h")

	first := markedRailRow(t, m.View())
	if !strings.Contains(first, "Open") {
		t.Fatalf("setup: landed on %q, want the state row", first)
	}
	if got := markedRailRow(t, press(m, "k").View()); got != first {
		t.Errorf("k on the first control marked %q, want it held on %q", got, first)
	}

	walkDown := press(m, strings.Fields(strings.Repeat("j ", 30))...)
	last := markedRailRow(t, walkDown.View())
	if got := markedRailRow(t, press(walkDown, "j").View()); got != last {
		t.Errorf("past the last control the cursor marked %q, want it held on %q", got, last)
	}

	if !strings.Contains(stripANSI(walkDown.View()), "Blocked") {
		t.Errorf("the rail's last row is off screen with the cursor at its end:\n%s", stripANSI(walkDown.View()))
	}
}

func TestTheBracesDoNothingOnTheRail(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 200, 44), "h")

	for _, k := range []string{"}", "{"} {
		if got := press(m, k).View(); got != m.View() {
			t.Errorf("%q moved something on the rail", k)
		}
	}
}

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

	if got := rows[at+4]; got != "+ Add reviewer" {
		t.Errorf("the row after the reviewers is %q, want the add row", got)
	}

	m := press(detailed(held(sampleDetail()), 200, 44), "h")
	if got := markedRailRow(t, press(m, "j", "j", "j", "j").View()); got != "+ Add reviewer" {
		t.Errorf("the fourth step marked %q, want the add row", got)
	}
}

func TestTheRingSkipsTheRailRowsThereIsNothingToDoTo(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 200, 44), "h")

	seen := map[string]bool{}
	for i := range 16 {
		seen[markedRailRow(t, press(m, strings.Fields(strings.Repeat("j ", i))...).View())] = true
	}

	reached := func(text string) bool {
		for row := range seen {
			if strings.Contains(row, text) {
				return true
			}
		}
		return false
	}

	for _, want := range []string{
		"Open", "@nkr", "@drucial", "bug", "Rails Unit Tests / test",
		"+ Add reviewer", "+ Add assignee", "+ Add label", "behind main",
	} {
		if !reached(want) {
			t.Errorf("the ring never reached the %q row", want)
		}
	}

	for _, skip := range []string{"+42", "Blocked"} {
		if reached(skip) {
			t.Errorf("the ring stopped on %q, which there is nothing to do to", skip)
		}
	}
}

func TestFocusHoldsThroughAReorderedTimeline(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 200, 60), "}")
	if got := focusedCard(t, m.View()); !strings.HasPrefix(got, cardComment) {
		t.Fatalf("one step focused %q, want %q", got, cardComment)
	}

	m.SetDetail(held(reordered(sampleDetail())))

	if got := focusedCard(t, m.View()); !strings.HasPrefix(got, cardComment) {
		t.Errorf("the reorder moved focus to %q, want %q still", got, cardComment)
	}
}

func TestAnUnfoldHoldsThroughAReorderedTimeline(t *testing.T) {
	folded := func() gh.PullRequestDetail {
		d := sampleDetail()
		d.Timeline[0].Comment.Body = "Look.\n\n<details>\n<summary>Hidden</summary>\n\nThe secret.\n\n</details>\n"
		return d
	}

	m := press(detailed(held(folded()), 200, 60), "}", "space")
	if !strings.Contains(stripANSI(m.View()), "The secret.") {
		t.Fatal("o did not unfold the comment")
	}

	m.SetDetail(held(reordered(folded())))

	out := stripANSI(m.View())
	if strings.Contains(out, "▸ Hidden") {
		t.Error("the comment renders folded after the reorder")
	}
	if !strings.Contains(out, "The secret.") {
		t.Error("the reorder folded the comment back up")
	}
}

func reordered(d gh.PullRequestDetail) gh.PullRequestDetail {
	d.Timeline[0], d.Timeline[1] = d.Timeline[1], d.Timeline[0]
	return d
}

func TestRailFocusHoldsThroughAnInsertedLabel(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 200, 44), "h")

	m = press(m, strings.Fields(strings.Repeat("j ", 7))...)
	if got := markedRailRow(t, m.View()); got != "bug" {
		t.Fatalf("seven steps marked %q, want the label", got)
	}

	d := sampleDetail()
	d.Labels = append([]gh.Label{{Name: "docs"}}, d.Labels...)
	m.SetDetail(held(d))

	if got := markedRailRow(t, m.View()); got != "bug" {
		t.Errorf("the new label moved the cursor to %q, want %q still", got, "bug")
	}
}

func TestAFocusedFrameFillsItsSizeExactly(t *testing.T) {
	sizes := []struct{ width, height int }{
		{width: 200, height: 40},
		{width: 160, height: 24},
		{width: 100, height: 20},
		{width: 60, height: 10},
	}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			m := press(detailed(held(sampleDetail()), size.width, size.height), "}", "}", "}")
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

func focusedCard(t *testing.T, frame string) string {
	t.Helper()

	head, _ := focusedCardAt(t, frame)
	return head
}

func focusedCardAt(t *testing.T, frame string) (string, int) {
	t.Helper()

	accent := fgSeq(testTheme.Accent)
	lines := strings.Split(frame, "\n")
	top := paneTopAt(frame)

	for i, line := range lines {
		at := strings.Index(line, "╭")
		if at < 0 || i+1 >= len(lines) || strings.HasPrefix(stripANSI(line), "╭") {
			continue
		}
		start := strings.LastIndex(line[:at], "\x1b[")
		if start < 0 || !strings.HasPrefix(line[start+2:], accent) {
			continue
		}
		return cardHeading(stripANSI(lines[i+1]), stripANSI(line)), i - top
	}
	return "", -1
}

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

func markedRailRow(t *testing.T, frame string) string {
	t.Helper()

	for _, raw := range railRaw(t, frame) {
		if !strings.Contains(raw, bgSeq(testTheme.SelectedBackground)) {
			continue
		}
		return strings.TrimSpace(strings.Trim(stripANSI(raw), "│●○✓✗ "+paint.BarGlyph))
	}
	return ""
}
