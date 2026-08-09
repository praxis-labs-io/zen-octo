package prview_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/prview"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// type sends a string one keypress at a time, the way a reader writes it.
func typed(m prview.Model, text string) prview.Model {
	for _, r := range text {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// pressed is press with the command kept, for the keys that ask the root for
// something.
func pressed(m prview.Model, keys ...string) (prview.Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, k := range keys {
		m, cmd = m.Update(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
	}
	return m, cmd
}

// runCmd is what a command produced, or nil for a key the screen answered on
// its own.
func runCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

// chord is a key the plain path cannot build: ctrl+enter carries no text, only
// a code and a modifier, which is exactly why a terminal has to be asked
// whether it can send one.
func chord(m prview.Model) (prview.Model, tea.Cmd) {
	return m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
}

func composing(width, height int) prview.Model {
	return press(detailed(held(sampleDetail()), width, height), "c")
}

// The box is part of the conversation, not something summoned. It is on the
// page before any key is pressed, which is how anyone finds out it is there.
func TestTheCommentBoxIsAlwaysOnThePage(t *testing.T) {
	out := stripANSI(detailed(held(sampleDetail()), 200, 60).View())

	if !strings.Contains(out, "Leave a comment") {
		t.Errorf("no comment box in the conversation:\n%s", out)
	}
	if !strings.Contains(out, "write a comment") {
		t.Error("the box has no heading saying what it is for")
	}
	// c is what works from anywhere on the page. enter only works once the ring
	// is on the box, so naming it here would be a key that does nothing.
	if !strings.Contains(out, "c to write") {
		t.Error("the box does not say how to start writing in it")
	}
}

// It renders through the same card the comments above it render through, so
// one being written sits among the ones already made rather than beside them.
func TestTheCommentBoxRendersAsACard(t *testing.T) {
	lines := strings.Split(stripANSI(detailed(held(sampleDetail()), 200, 60).View()), "\n")

	column := func(want string) int {
		for i, line := range lines {
			if strings.Contains(line, want) && i > 0 {
				return strings.Index(lines[i-1], "╭")
			}
		}
		return -1
	}

	box, comment := column("write a comment"), column("octobot · commented")
	if box < 0 || comment < 0 {
		t.Fatalf("could not find both cards: box %d, comment %d", box, comment)
	}
	if box != comment {
		t.Errorf("the box opens at column %d and a comment card at %d", box, comment)
	}
}

// The box is content inside the pane, so it can no more overflow the frame than
// a comment can. The short sizes are here because it is the tallest block the
// conversation builds.
func TestTheFrameStillFillsItsSizeWithTheComposerOpen(t *testing.T) {
	sizes := []struct{ width, height int }{
		{width: 200, height: 40},
		{width: 160, height: 24},
		{width: 100, height: 20},
		{width: 60, height: 12},
		{width: 100, height: 8},
		{width: 100, height: 6},
		{width: 100, height: 4},
	}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			lines := strings.Split(composing(size.width, size.height).View(), "\n")

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

func TestWhatIsTypedShowsInTheComposer(t *testing.T) {
	m := typed(composing(200, 40), "ship it")

	if got := stripANSI(m.View()); !strings.Contains(got, "ship it") {
		t.Errorf("the composer does not hold what was typed:\n%s", got)
	}
}

// Every letter is a letter while the pane is open. j and k scroll everywhere
// else on this screen, and a composer that ate them would be unusable.
func TestTheComposerTakesTheKeysTheScreenWouldHaveAnswered(t *testing.T) {
	m := typed(composing(200, 40), "jkdorq")

	if got := stripANSI(m.View()); !strings.Contains(got, "jkdorq") {
		t.Errorf("the screen answered keys meant for the text:\n%s", got)
	}
}

// esc hands the keyboard back and keeps every word. The box stays where it is:
// it is part of the conversation, not something that was opened over it.
func TestEscapeKeepsTheWordsAndLeavesTheBox(t *testing.T) {
	m := press(typed(composing(200, 60), "half written"), "esc")

	out := stripANSI(m.View())
	if !strings.Contains(out, "half written") {
		t.Error("esc took the words away")
	}
	// The ring is still standing on the box, so enter is what resumes.
	if !strings.Contains(out, "enter to write") {
		t.Error("the box does not say the keyboard went back to the screen")
	}

	// And a letter is a key again rather than a letter.
	if got := stripANSI(press(m, "j").View()); strings.Contains(got, "half writtenj") {
		t.Error("the box is still taking letters after esc")
	}
}

// Enter in the text is a newline and can be nothing else. A key that sends a
// half-written comment is worse than one more keystroke.
func TestEnterInTheTextIsANewlineNotAPost(t *testing.T) {
	m, cmd := pressed(typed(composing(200, 40), "one"), "enter")
	if msg := runCmd(cmd); msg != nil {
		if _, posted := msg.(prview.PostCommentMsg); posted {
			t.Fatal("enter in the text posted the comment")
		}
	}

	m = typed(m, "two")
	out := stripANSI(m.View())
	if !strings.Contains(out, "one") || !strings.Contains(out, "two") {
		t.Errorf("the newline did not land:\n%s", out)
	}
}

func TestTabReachesThePostButtonAndEnterSendsIt(t *testing.T) {
	m := typed(composing(200, 40), "ship it")

	m, cmd := pressed(m, "tab", "enter")

	msg, ok := runCmd(cmd).(prview.PostCommentMsg)
	if !ok {
		t.Fatalf("enter on the button sent %T, want a PostCommentMsg", runCmd(cmd))
	}
	if msg.Body != "ship it" {
		t.Errorf("Body = %q, want what was typed", msg.Body)
	}
	if msg.ID != samplePR().ID {
		t.Errorf("ID = %q, want the pull request on screen", msg.ID)
	}

	// The pane closes and empties. The words are the root's now, and it puts
	// them back if the write fails.
	if got := stripANSI(m.View()); strings.Contains(got, "ship it") {
		t.Errorf("the composer still holds a comment it sent:\n%s", got)
	}
}

func TestCtrlEnterPostsFromTheText(t *testing.T) {
	_, cmd := chord(typed(composing(200, 40), "ship it"))

	msg, ok := runCmd(cmd).(prview.PostCommentMsg)
	if !ok {
		t.Fatalf("ctrl+enter sent %T, want a PostCommentMsg", runCmd(cmd))
	}
	if msg.Body != "ship it" {
		t.Errorf("Body = %q, want what was typed", msg.Body)
	}
}

// A buffer of whitespace is nothing to post, and the button says so by going
// faint rather than by swallowing the press.
func TestAnEmptyComposerPostsNothing(t *testing.T) {
	m := typed(composing(200, 40), "   \n  ")

	_, cmd := pressed(m, "tab", "enter")
	if msg := runCmd(cmd); msg != nil {
		t.Errorf("an empty composer sent %T, want nothing", msg)
	}

	if lit(m.View()) {
		t.Error("the post button is lit with nothing to post")
	}
}

// The button stays muted until it holds the focus. The writing is what the pane
// is for, and a filled block in the corner would out-shout it.
func TestThePostButtonLightsOnlyWhenItHoldsFocus(t *testing.T) {
	written := typed(composing(200, 40), "ship it")
	if lit(written.View()) {
		t.Error("the button is lit before anything reached it")
	}

	if !lit(press(written, "tab").View()) {
		t.Error("tab did not light the button")
	}
	if lit(press(written, "tab", "tab").View()) {
		t.Error("the button is still lit after focus went back to the text")
	}
}

// lit is whether the post button carries the accent it takes on focus.
func lit(frame string) bool {
	return strings.Contains(frame, bgSeq(theme.RosePineMoon.Secondary)+"mPost")
}

// It is a button at every state, filled surface and all. Muted is the colour it
// wears, not a different shape: a control that turns into a word when it is not
// focused is one the reader has to hunt for.
func TestThePostButtonIsAFilledSurfaceAtEveryState(t *testing.T) {
	empty := composing(200, 40)
	written := typed(empty, "ship it")

	states := map[string]string{
		"with nothing written": empty.View(),
		"ready to send":        written.View(),
		"holding focus":        press(written, "tab").View(),
	}
	for name, frame := range states {
		if !strings.Contains(frame, bgSeq(theme.RosePineMoon.SelectedBackground)+"mPost") &&
			!lit(frame) {
			t.Errorf("the button has no background %s", name)
		}
	}
}

// The button sits against the right edge of the pane, one column in, which is
// the corner every dialog puts its confirm in.
func TestThePostButtonSitsInTheBottomRight(t *testing.T) {
	// Narrow enough that the rail is hidden, so the composer's own border is
	// the last thing on the row.
	m := typed(composing(100, 40), "ship it")

	lines := strings.Split(stripANSI(m.View()), "\n")
	at := -1
	for i, line := range lines {
		if strings.Contains(line, "Post") {
			at = i
		}
	}
	if at < 0 {
		t.Fatalf("no post button on the frame:\n%s", strings.Join(lines, "\n"))
	}

	// The last row inside the pane: the line under it closes the border.
	if !strings.Contains(lines[at+1], "╰") {
		t.Errorf("the button is not on the pane's last row: %q", lines[at+1])
	}

	// Nothing but the button's own padding and the gutter between the label and
	// the card's border.
	tail := strings.TrimLeft(lines[at][strings.Index(lines[at], "Post")+len("Post"):], " ")
	if !strings.HasPrefix(tail, "│") {
		t.Errorf("the button is followed by %q, want the card's border next", tail)
	}
}

// A top-level comment lands in the conversation, so it is written from there.
// The three tabs with a column each anchor a different kind of comment.
func TestCDoesNothingOnTheTabsWithAColumn(t *testing.T) {
	for _, tab := range []struct {
		name    string
		presses []string
	}{
		{name: "commits", presses: []string{"]"}},
		{name: "checks", presses: []string{"]", "]"}},
		{name: "files", presses: []string{"]", "]", "]"}},
	} {
		t.Run(tab.name, func(t *testing.T) {
			m := press(detailed(held(sampleDetail()), 200, 40), append(tab.presses, "c")...)
			if got := stripANSI(m.View()); strings.Contains(got, "Leave a comment") {
				t.Error("c opened the composer on a tab that has no top-level comment")
			}
		})
	}
}

// The footer names the chord only where the terminal can send it. Elsewhere
// ctrl+enter arrives as a plain enter and would add a blank line, and hinting
// it would be promising a key that does the opposite of what it says.
func TestTheFooterNamesTheChordOnlyWhereItWorks(t *testing.T) {
	plain := stripANSI(composing(200, 40).View())
	if strings.Contains(plain, "ctrl+enter") {
		t.Errorf("the footer names ctrl+enter on a terminal that cannot send it:\n%s", plain)
	}
	if !strings.Contains(plain, "tab · enter post") {
		t.Errorf("the footer does not name the button path:\n%s", plain)
	}

	m := detailed(held(sampleDetail()), 200, 40)
	m.SetChords(true)
	if got := stripANSI(press(m, "c").View()); !strings.Contains(got, "ctrl+enter post") {
		t.Errorf("the footer does not name the chord where it works:\n%s", got)
	}
}

// The revert branch, from the screen's side. A post that failed puts the words
// back where they were written.
func TestARestoredDraftReopensTheComposerWithTheWords(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 40)
	m.RestoreDraft("this did not send")

	if got := stripANSI(m.View()); !strings.Contains(got, "this did not send") {
		t.Errorf("the draft did not come back:\n%s", got)
	}
}

// A card that has landed and one that still might not must not read the same.
// Only one of the two can disappear.
func TestACommentStillInFlightSaysSo(t *testing.T) {
	d := sampleDetail()
	pending := gh.Comment{
		Kind: gh.CommentIssue, ID: "pending-1", Author: gh.Actor{Login: "drucial"},
		CreatedAt: time.Now(), Body: "on its way", Pending: true,
	}
	d.Timeline = append(d.Timeline, gh.TimelineItem{
		Kind: gh.TimelineComment, Actor: pending.Author, CreatedAt: pending.CreatedAt,
		Comment: &pending,
	})

	out := stripANSI(detailed(held(d), 200, 60).View())
	if !strings.Contains(out, "drucial · commented · posting") {
		t.Errorf("the pending comment does not say it is still going:\n%s", out)
	}
	if !strings.Contains(out, "on its way") {
		t.Error("the pending comment's body is not on the screen")
	}
}

// ctrl+e hands off rather than answering on the spot. The round trip needs a
// real editor and a real terminal, so this holds the one thing a test can:
// that the key produces a command instead of being swallowed.
func TestCtrlEHandsTheBufferOff(t *testing.T) {
	_, cmd := pressed(typed(composing(200, 40), "draft"), "ctrl+e")
	if cmd == nil {
		t.Error("ctrl+e did nothing")
	}
}

// The box costs the screen no layout at all. It is a block in the conversation,
// so writing in it leaves the rail and the pane borders exactly where they were.
func TestWritingACommentDoesNotMoveTheLayout(t *testing.T) {
	resting := detailed(held(sampleDetail()), 200, 40)
	writing := typed(press(resting, "c"), "ship it")

	if before, after := railRows(t, resting.View()), railRows(t, writing.View()); len(before) != len(after) {
		t.Errorf("the rail is %d rows while a comment is being written and %d otherwise", len(after), len(before))
	}

	// The top border only. The one at the foot carries the scroll counter, and
	// that legitimately moves: c scrolls the page down to the box.
	top := func(m prview.Model) string {
		return strings.Split(stripANSI(m.View()), "\n")[0]
	}
	if top(resting) != top(writing) {
		t.Errorf("the panes moved:\n%s\nwant\n%s", top(writing), top(resting))
	}
}

// The box is the last card, so on a long thread it starts below the fold. c
// brings it onto the screen rather than leaving the reader to scroll for it.
func TestCBringsTheBoxOntoTheScreen(t *testing.T) {
	d := sampleDetail()
	d.Body = strings.Repeat("The retry path backs off forever.\n\n", 25)

	m := detailed(held(d), 100, 30)
	if strings.Contains(stripANSI(m.View()), "Leave a comment") {
		t.Fatal("the box is already on screen, so this proves nothing")
	}

	if !strings.Contains(stripANSI(press(m, "c").View()), "Leave a comment") {
		t.Error("c did not bring the box onto the screen")
	}
}

// Typing rebuilds the page and the box is the last thing on it, so the page has
// to hold at the foot or the box being written in scrolls away under the words.
func TestTheBoxStaysOnScreenWhileItIsWrittenIn(t *testing.T) {
	d := sampleDetail()
	d.Body = strings.Repeat("The retry path backs off forever.\n\n", 25)

	m := typed(press(detailed(held(d), 100, 30), "c"), "still here")

	if got := stripANSI(m.View()); !strings.Contains(got, "still here") {
		t.Errorf("the box scrolled away while it was being written in:\n%s", got)
	}
}

// The page above the box is kept while it is being written in, which is what
// makes a keystroke cheap. A refetch landing mid-sentence has to drop it, or
// the reader carries on typing over a conversation that has moved on.
func TestARefetchWhileTypingStillReachesTheScreen(t *testing.T) {
	m := typed(composing(200, 60), "half written")

	next := sampleDetail()
	next.Timeline = append(next.Timeline, commented("nkr", time.Now(), "something new arrived"))
	m.SetDetail(held(next))

	out := stripANSI(m.View())
	if !strings.Contains(out, "something new arrived") {
		t.Errorf("the refetch never reached the screen:\n%s", out)
	}
	if !strings.Contains(out, "half written") {
		t.Error("the refetch took the words out of the box")
	}
}

// Focus moving onto the box unlights whichever card had it. The kept page holds
// the highlight, so it has to be dropped when the box takes the keyboard.
func TestTakingTheBoxUnlightsTheCardThatHadFocus(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 200, 60), "tab")
	if got := focusedCard(t, m.View()); !strings.HasPrefix(got, cardDescription) {
		t.Fatalf("tab focused %q, want the description", got)
	}

	if got := focusedCard(t, press(m, "c").View()); !strings.HasPrefix(got, cardCompose) {
		t.Errorf("c left the accent on %q, want it on the box", got)
	}
}

// Posting finishes the action, so nothing is left selected. An accent on an
// empty box says the keyboard is somewhere it is not.
func TestPostingLetsGoOfTheBox(t *testing.T) {
	m := typed(composing(200, 60), "ship it")
	if got := focusedCard(t, m.View()); !strings.HasPrefix(got, cardCompose) {
		t.Fatalf("the box is not focused before posting, it is %q", got)
	}

	m, _ = chord(m)

	if got := focusedCard(t, m.View()); got != "" {
		t.Errorf("posting left %q focused, want nothing", got)
	}
	if !strings.Contains(stripANSI(m.View()), "c to write") {
		t.Error("the box does not say the keyboard went back to the screen")
	}
}

// The hint names the key that works from where the reader is standing. enter
// only starts writing once the ring is on the box; anywhere else it is c, and
// naming the wrong one is a key that does nothing to whoever tries it.
func TestTheHintNamesTheKeyThatWorksFromHere(t *testing.T) {
	away := detailed(held(sampleDetail()), 200, 60)
	if got := stripANSI(away.View()); !strings.Contains(got, "c to write") {
		t.Errorf("with focus elsewhere the box names the wrong key:\n%s", got)
	}

	// Six tabs walks the ring onto the box without starting to write in it.
	onIt := press(away, strings.Fields(strings.Repeat("tab ", 6))...)
	out := stripANSI(onIt.View())
	if !strings.Contains(out, "enter to write") {
		t.Errorf("with the ring on the box it does not name enter:\n%s", out)
	}
	if strings.Contains(out, "c to write") {
		t.Error("the box still names c once the ring is on it")
	}
}

// A paste is not a keypress. The terminal sends it whole as its own message,
// and nothing routed it to the box, so pasting a comment in did nothing at all.
func TestAPastedCommentReachesTheBox(t *testing.T) {
	m := typed(composing(200, 60), "before ")

	m, _ = m.Update(tea.PasteMsg{Content: "pasted words"})

	if got := stripANSI(m.View()); !strings.Contains(got, "before pasted words") {
		t.Errorf("the paste never reached the box:\n%s", got)
	}
}

// It only goes there while the box has the keyboard. A paste onto a screen that
// is being read is not text anybody asked for.
func TestAPasteWhileReadingIsIgnored(t *testing.T) {
	m, _ := detailed(held(sampleDetail()), 200, 60).Update(tea.PasteMsg{Content: "pasted words"})

	if got := stripANSI(m.View()); strings.Contains(got, "pasted words") {
		t.Errorf("a paste landed in the box with the keyboard elsewhere:\n%s", got)
	}
}
