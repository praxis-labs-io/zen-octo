package prview_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/tui/prview"
)

// onComment puts the ring on a comment inside the thread GitHub will take a
// reply to. Four tabs is the first of its two comments, five the second.
func onComment(t *testing.T, n int) prview.Model {
	t.Helper()
	return tabbed(detailed(held(sampleDetail()), 200, 60), n)
}

// replying opens the box on that comment.
func replying(t *testing.T, n int, key string) prview.Model {
	t.Helper()
	return press(onComment(t, n), key)
}

// The reply goes where the reply goes. The box at the foot of the page files a
// comment against the pull request, which is a different thing from an answer
// to a line of code.
func TestReplyOpensABoxInsideTheThread(t *testing.T) {
	out := stripANSI(replying(t, 4, "r").View())

	head := strings.Index(out, "internal/gh/client.go:42")
	box := strings.Index(out, "write a reply")
	foot := strings.Index(out, "write a comment")

	switch {
	case box < 0:
		t.Fatalf("r opened no box:\n%s", out)
	case head < 0 || box < head:
		t.Error("the box is not inside the thread it answers")
	case foot >= 0 && box > foot:
		t.Error("the box is below the compose card, not in the thread")
	}

	if !strings.Contains(out, "Leave a reply") {
		t.Error("the box does not say what it is for")
	}
}

// The thread that renders at the foot of the page is the one nobody may answer.
// A key that opens a box GitHub will reject is worse than one that does nothing.
func TestReplyIsInertOnAThreadThatTakesNoReply(t *testing.T) {
	locked := onComment(t, 7)

	before := locked.View()
	if got := focusedCard(t, before); !strings.HasPrefix(got, cardLocked) {
		t.Fatalf("the seventh tab focused %q, want the locked thread", got)
	}

	if after := press(locked, "r").View(); after != before {
		t.Errorf("r opened something on a thread that takes no reply:\n%s", stripANSI(after))
	}
	if after := press(locked, "R").View(); after != before {
		t.Errorf("R opened something on a thread that takes no reply:\n%s", stripANSI(after))
	}
}

// Both keys read the ring, so neither does anything with nothing focused.
func TestReplyNeedsAFocusedComment(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 60)

	if after := press(m, "r").View(); after != m.View() {
		t.Error("r opened a box with nothing focused")
	}

	// The description is focusable and is not a review comment.
	onDescription := tabbed(m, 1)
	if after := press(onDescription, "r").View(); after != onDescription.View() {
		t.Error("r opened a box on the description")
	}
}

func TestQuoteReplyPutsTheCommentInTheBox(t *testing.T) {
	out := stripANSI(press(replying(t, 4, "R"), "n", "o").View())

	if !strings.Contains(out, "> This backs off forever.") {
		t.Errorf("the quote is not in the box:\n%s", out)
	}
	if !strings.Contains(out, "no") {
		t.Error("the cursor is not under the quote")
	}
}

// R quotes the comment the ring is on, not the first one in the thread.
func TestQuoteReplyQuotesTheFocusedComment(t *testing.T) {
	out := stripANSI(replying(t, 5, "R").View())

	if !strings.Contains(out, "> Seconded, the cap is the fix.") {
		t.Errorf("R quoted the wrong comment:\n%s", out)
	}
	if strings.Contains(out, "> This backs off forever.") {
		t.Error("R quoted a comment the ring was not on")
	}
}

// Plain r leaves the buffer alone. A reader who wanted the quote has a key for
// it, and one who did not would have to clear five lines before writing.
func TestReplyDoesNotQuote(t *testing.T) {
	if out := stripANSI(replying(t, 4, "r").View()); strings.Contains(out, "> This backs off") {
		t.Error("r quoted the comment without being asked")
	}
}

// Typing goes into the box and nowhere else. r is a letter in there, and so is
// every other key this screen binds.
func TestTheReplyBoxTakesTheKeyboard(t *testing.T) {
	out := stripANSI(typed(replying(t, 4, "r"), "capped it").View())

	if !strings.Contains(out, "capped it") {
		t.Errorf("the box did not take the letters:\n%s", out)
	}
	if !strings.Contains(out, "write a reply") {
		t.Error("typing closed the box")
	}
}

// While the box has the keyboard every key this screen binds is a letter, which
// is the only way a box in a keyboard-driven program can be written in. c is the
// one that would otherwise open a second box.
func TestEveryKeyIsALetterInTheReplyBox(t *testing.T) {
	out := stripANSI(typed(replying(t, 4, "r"), "cdoqr").View())

	if !strings.Contains(out, "cdoqr") {
		t.Errorf("a bound key was swallowed instead of typed:\n%s", out)
	}
	if strings.Contains(out, "Leave a comment") && strings.Count(out, "Leave a") > 1 {
		t.Error("c opened the compose card on top of the reply box")
	}
}

// The compose card is the other way round: r there is a letter too, so the two
// boxes cannot both be open.
func TestOnlyOneBoxTakesTheKeysAtOnce(t *testing.T) {
	out := stripANSI(typed(composing(200, 60), "reply r").View())

	if strings.Contains(out, "write a reply") {
		t.Errorf("r opened a reply box from inside the compose card:\n%s", out)
	}
}

// esc closes the box and keeps the words against the thread they were written
// for, so looking at the code above an answer does not throw the answer away.
func TestEscClosesTheBoxAndKeepsTheWords(t *testing.T) {
	closed := press(typed(replying(t, 4, "r"), "capped it"), "esc")

	if out := stripANSI(closed.View()); strings.Contains(out, "write a reply") {
		t.Errorf("esc left the box on the page:\n%s", out)
	}

	if out := stripANSI(press(closed, "r").View()); !strings.Contains(out, "capped it") {
		t.Errorf("the words did not come back with the box:\n%s", out)
	}
}

// A draft belongs to its thread. Reopening a different one must not serve it
// somebody else's answer.
func TestADraftStaysOnItsOwnThread(t *testing.T) {
	held := press(typed(replying(t, 4, "r"), "capped it"), "esc")

	// The ring went back to the comment it was opened from, so one tab reaches
	// the resolved thread and two the thread nobody may answer.
	other := press(tabbed(held, 3), "r")
	if out := stripANSI(other.View()); strings.Contains(out, "capped it") {
		t.Errorf("a draft leaked onto another thread:\n%s", out)
	}
}

// esc puts the reader back where they were rather than nowhere. The card that
// opened the box is the one they were reading.
func TestEscGivesFocusBackToTheComment(t *testing.T) {
	closed := press(replying(t, 4, "r"), "esc")

	if got := focusedCard(t, closed.View()); !strings.HasPrefix(got, cardThread) {
		t.Errorf("esc focused %q, want the thread it was opened from", got)
	}
}

// The Files tab renders the same threads and has no ring to open a box from, so
// no box belongs in one. It also caches its blocks, which is what makes a stray
// box there permanent: it would stand in the code long after it closed.
func TestTheReplyBoxNeverRendersOnTheFilesTab(t *testing.T) {
	// esc first, because every key is a letter while the box has the keyboard.
	// The words stay held against the thread, which is the state that could put
	// a box in the diff if anything read it there.
	onFiles := press(press(typed(replying(t, 4, "r"), "capped it"), "esc"), "]", "]", "]")

	out := stripANSI(onFiles.View())
	if strings.Contains(out, "write a reply") {
		t.Errorf("a reply box rendered in the diff:\n%s", out)
	}
	if strings.Contains(out, "capped it") {
		t.Errorf("a held draft leaked into the diff:\n%s", out)
	}

	// A reply that fails while the reader is on the Files tab is the one path
	// that asks for a box where there is none to open.
	onFiles.RestoreReply("RT_1", "and again")
	if out := stripANSI(onFiles.View()); strings.Contains(out, "write a reply") {
		t.Errorf("a failed reply opened a box on the Files tab:\n%s", out)
	}
	if onFiles.Composing() {
		t.Error("the Files tab took the keyboard for a box it does not draw")
	}
}

// The frame is the size it was given, whatever is open inside it.
func TestTheReplyBoxDoesNotMoveTheLayout(t *testing.T) {
	for _, size := range []struct{ width, height int }{
		{200, 60}, {160, 40}, {120, 30}, {100, 20},
	} {
		m := press(tabbed(detailed(held(sampleDetail()), size.width, size.height), 4), "r")
		lines := strings.Split(m.View(), "\n")

		if len(lines) != size.height {
			t.Errorf("%dx%d rendered %d lines", size.width, size.height, len(lines))
		}
		for i, line := range lines {
			if w := lipgloss.Width(line); w != size.width {
				t.Errorf("%dx%d line %d is %d columns wide", size.width, size.height, i, w)
				break
			}
		}
	}
}

// Typing joins a freshly rendered box between two halves of the page built once
// and kept, instead of rebuilding the page around it. The saving is the point
// and the join is where it can go wrong: a keystroke has to leave the reader
// looking at the same frame either way.
//
// A resize is what forces the rebuild, since everything on the page is measured
// against a width and none of it survives one.
func TestTypingInTheBoxRendersWhatARebuildWould(t *testing.T) {
	// The thread this opens on hangs off a review, so the halves are cut either
	// side of a block holding a card, a branch gutter and the thread under it.
	// Cutting inside that block is what the split is built to avoid.
	joined := typed(replying(t, 4, "r"), "capped it")

	rebuilt := joined
	rebuilt.SetSize(200, 60)

	if joined.View() != rebuilt.View() {
		t.Errorf("the cached page and a rebuilt one differ:\n%s\nwant\n%s",
			stripANSI(joined.View()), stripANSI(rebuilt.View()))
	}
}

// The same, for the compose card, which is the degenerate split: the whole page
// is the head and the tail is empty.
func TestTypingInTheCommentBoxRendersWhatARebuildWould(t *testing.T) {
	joined := typed(composing(200, 60), "ship it")

	rebuilt := joined
	rebuilt.SetSize(200, 60)

	if joined.View() != rebuilt.View() {
		t.Errorf("the cached page and a rebuilt one differ:\n%s\nwant\n%s",
			stripANSI(joined.View()), stripANSI(rebuilt.View()))
	}
}

// Posting hands the words to the root, addressed to the thread rather than to
// the pull request.
func TestPostingAReplyAsksTheRootForTheThread(t *testing.T) {
	m, cmd := chord(typed(replying(t, 4, "r"), "capped it"))

	msg, ok := runCmd(cmd).(prview.PostReplyMsg)
	if !ok {
		t.Fatalf("posting produced %T, want a PostReplyMsg", runCmd(cmd))
	}
	if msg.ThreadID != "RT_1" {
		t.Errorf("ThreadID = %q, want RT_1", msg.ThreadID)
	}
	if msg.ID != "PR_412" {
		t.Errorf("ID = %q, want the pull request", msg.ID)
	}
	if msg.Body != "capped it" {
		t.Errorf("Body = %q, want what was written", msg.Body)
	}

	if out := stripANSI(m.View()); strings.Contains(out, "write a reply") {
		t.Error("the box is still open after posting")
	}
}

// An empty box posts nothing. The button says so by going faint rather than by
// swallowing the keypress.
func TestAnEmptyReplyPostsNothing(t *testing.T) {
	if _, cmd := chord(replying(t, 4, "r")); cmd != nil {
		t.Error("an empty box asked the root to post")
	}
}

// A posted reply is not a draft. Reopening the box on that thread must not
// serve the words back as though they never left.
func TestPostingClearsTheDraft(t *testing.T) {
	m, _ := chord(typed(replying(t, 4, "r"), "capped it"))

	if out := stripANSI(press(m, "r").View()); strings.Contains(out, "capped it") {
		t.Errorf("the posted words came back as a draft:\n%s", out)
	}
}

// The words are the one thing here that cannot be fetched again, so a failed
// post puts them back where they were written.
func TestARestoredReplyGoesBackToItsThread(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 60)
	m.RestoreReply("RT_1", "capped it")

	out := stripANSI(m.View())
	if !strings.Contains(out, "write a reply") {
		t.Fatalf("the box did not reopen:\n%s", out)
	}
	if !strings.Contains(out, "capped it") {
		t.Error("the words did not come back")
	}

	// Inside the thread, not at the foot of the page.
	if head := strings.Index(out, "internal/gh/client.go:42"); head < 0 ||
		strings.Index(out, "write a reply") < head {
		t.Error("the words came back somewhere other than the thread")
	}
}

// A reply answered for long after it left can arrive while the reader is
// writing something else. Stealing the caret to report old news is worse than
// the toast that reports it.
func TestARestoredReplyDoesNotStealTheKeyboard(t *testing.T) {
	m := typed(composing(200, 60), "a different comment")
	m.RestoreReply("RT_1", "capped it")

	if !m.Composing() {
		t.Fatal("the restore took the keyboard off the box being written in")
	}
	if out := stripANSI(m.View()); !strings.Contains(out, "a different comment") {
		t.Errorf("the comment being written was disturbed:\n%s", out)
	}
	if out := stripANSI(m.View()); strings.Contains(out, "capped it") {
		t.Error("the reply landed in the comment box at the foot of the page")
	}

	// The words are the thread's draft, so the box opens on them once the
	// keyboard is free again.
	back := press(tabbed(press(m, "esc"), 4), "r")
	if out := stripANSI(back.View()); !strings.Contains(out, "capped it") {
		t.Errorf("the words are not waiting on their thread:\n%s", out)
	}
}

// A refetch may not carry the thread any more: resolved and hidden, or off the
// first page. There is nowhere to put the words and nowhere honest to invent.
func TestARestoredReplyToAThreadThatIsGoneOpensNothing(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 60)
	m.RestoreReply("RT_GONE", "capped it")

	if out := stripANSI(m.View()); strings.Contains(out, "write a reply") {
		t.Errorf("a box opened for a thread the page does not carry:\n%s", out)
	}
}

// The mark on the byline is the accent, the same signal every other focused
// thing on this screen uses.
func TestTheOpenBoxLightsItsThreadCard(t *testing.T) {
	if got := focusedCard(t, replying(t, 4, "r").View()); !strings.HasPrefix(got, cardThread) {
		t.Errorf("the card holding the open box is not lit, focused %q", got)
	}
}
