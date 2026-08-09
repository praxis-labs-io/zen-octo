package prview_test

import (
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/tui/prview"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// Where tab stops on the fixture: the thread GitHub will take a reply to, the
// one it will not, and a second answerable one further down.
const (
	tabThread = 4
	tabLocked = 6
	tabOther  = 7
)

// onThread puts the ring on a card by tab count.
func onThread(t *testing.T, n int) prview.Model {
	t.Helper()
	return tabbed(detailed(held(sampleDetail()), 200, 60), n)
}

// replying opens a box from that card.
func replying(t *testing.T, n int, key string) prview.Model {
	t.Helper()
	return press(onThread(t, n), key)
}

// The box goes under the thread it answers, as a card of its own. The box at
// the foot of the page files a comment against the pull request, which is a
// different thing from an answer to a line of code.
func TestTheBoxOpensUnderTheThreadItAnswers(t *testing.T) {
	out := stripANSI(replying(t, tabThread, "r").View())

	head := strings.Index(out, "internal/gh/client.go:42")
	last := strings.Index(out, "Seconded, the cap is the fix.")
	box := strings.Index(out, "write a reply")
	foot := strings.Index(out, "write a comment")

	switch {
	case box < 0:
		t.Fatalf("r opened no box:\n%s", out)
	case head < 0 || box < head:
		t.Error("the box is above the thread it answers")
	case last < 0 || box < last:
		t.Error("the box is above the comments it follows on from")
	case foot >= 0 && box > foot:
		t.Error("the box is below the compose card rather than under its thread")
	}

	if !strings.Contains(out, "Leave a reply") {
		t.Error("the box does not say what it is for")
	}
}

// The thread that renders at the foot of the page is the one nobody may answer.
// A key that opens a box GitHub will reject is worse than one that does nothing.
func TestReplyIsInertOnAThreadThatTakesNoReply(t *testing.T) {
	locked := onThread(t, tabLocked)

	before := locked.View()
	if got := focusedCard(t, before); !strings.HasPrefix(got, cardLocked) {
		t.Fatalf("the sixth tab focused %q, want the locked thread", got)
	}

	if after := press(locked, "r").View(); after != before {
		t.Errorf("r opened something on a thread that takes no reply:\n%s", stripANSI(after))
	}
	if after := press(locked, "R").View(); after != before {
		t.Errorf("R opened something on a thread that takes no reply:\n%s", stripANSI(after))
	}
}

// Both keys read the ring, so neither does anything with nothing focused.
func TestReplyNeedsSomethingFocused(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 60)

	if after := press(m, "r").View(); after != m.View() {
		t.Error("r opened a box with nothing focused")
	}
}

// GitHub does not thread the conversation, so there is nothing to hang a reply
// off a top-level comment. Answering one is a comment at the foot of the page,
// which is what the browser's quote reply does too. Without this r is a key that
// does nothing on most of the page.
func TestReplyingToSomethingWithNoThreadUsesTheCommentBox(t *testing.T) {
	// The second card is the top-level comment.
	out := stripANSI(press(onThread(t, 2), "R").View())

	if !strings.Contains(out, "> Coverage held at 84.2%.") {
		t.Errorf("R did not quote the comment into the box:\n%s", out)
	}
	if strings.Contains(out, "write a reply") {
		t.Error("R opened a thread box for a comment with no thread")
	}

	// The description answers the same way, and so does a review's own words.
	if out := stripANSI(press(onThread(t, 1), "R").View()); !strings.Contains(out, "> Caps the backoff at 30s") {
		t.Errorf("R did not quote the description:\n%s", out)
	}
	if out := stripANSI(press(onThread(t, 3), "R").View()); !strings.Contains(out, "> Two things on the retry path.") {
		t.Errorf("R did not quote the review:\n%s", out)
	}
}

func TestQuoteReplyPutsTheCommentInTheBox(t *testing.T) {
	out := stripANSI(press(replying(t, tabThread, "R"), "n", "o").View())

	if !strings.Contains(out, "> Seconded, the cap is the fix.") {
		t.Errorf("the quote is not in the box:\n%s", out)
	}
	if !strings.Contains(out, "no") {
		t.Error("the cursor is not under the quote")
	}
}

// Tab lands on a thread, not on a comment inside it, so a quote has to pick one.
// The last is the answer until J or K says otherwise: it is the newest, the one
// at the bottom of the card, and the one an answer follows on from.
func TestQuoteReplyTakesTheLastCommentByDefault(t *testing.T) {
	out := stripANSI(replying(t, tabThread, "R").View())

	if !strings.Contains(out, "> Seconded, the cap is the fix.") {
		t.Errorf("R quoted the wrong comment:\n%s", out)
	}
	if strings.Contains(out, "> This backs off forever.") {
		t.Error("R quoted a comment the sub-cursor was not on")
	}
}

// K steps the sub-cursor back up the thread, which is what makes answering the
// first comment of three possible.
func TestSteppingWithinTheThreadMovesWhatAQuoteTakes(t *testing.T) {
	out := stripANSI(press(onThread(t, tabThread), "K", "R").View())

	if !strings.Contains(out, "> This backs off forever.") {
		t.Errorf("K did not move what R quotes:\n%s", out)
	}
	if strings.Contains(out, "> Seconded, the cap is the fix.") {
		t.Error("R quoted the comment the sub-cursor left")
	}
}

// barred is the text of every line carrying the sub-cursor's bar. The bar runs
// the height of one comment, so this is that comment, line by line.
func barred(frame string) []string {
	var out []string
	for _, line := range strings.Split(stripANSI(frame), "\n") {
		if at := strings.Index(line, "▍"); at >= 0 {
			out = append(out, strings.TrimSpace(strings.Trim(line[at+len("▍"):], "│ ")))
		}
	}
	return out
}

// The bar is the only thing saying which comment the keys have, since the card
// is lit for the whole thread either way.
func TestTheSubCursorIsMarkedWithABar(t *testing.T) {
	// It opens on the last comment, which is the one an answer follows on from.
	last := barred(onThread(t, tabThread).View())
	if len(last) == 0 {
		t.Fatalf("no bar on the focused thread:\n%s", stripANSI(onThread(t, tabThread).View()))
	}
	if !strings.HasPrefix(last[0], "octobot · said") {
		t.Errorf("the bar opens on %q, want the last comment", last[0])
	}
	if !slices.Contains(last, "Seconded, the cap is the fix.") {
		t.Errorf("the bar does not run the height of the comment: %q", last)
	}

	// K moves it up rather than adding a second one.
	up := barred(press(onThread(t, tabThread), "K").View())
	if !strings.HasPrefix(up[0], "nkr · said") {
		t.Errorf("after K the bar is on %q, want the first comment", up[0])
	}
	if slices.Contains(up, "Seconded, the cap is the fix.") {
		t.Error("the bar is on both comments at once")
	}

	// And it is gone once the thread is not the focus.
	if away := barred(onThread(t, tabLocked).View()); len(away) > 0 {
		t.Errorf("a bar is drawn on a thread that does not hold the focus: %q", away)
	}
}

// It clamps at both ends. Running off a thread is a reader asking for the block
// above or below it, and tab is the key for that.
func TestTheSubCursorStopsAtTheEndsOfTheThread(t *testing.T) {
	m := onThread(t, tabThread)

	top := press(m, "K", "K", "K")
	if got := barred(top.View()); !strings.HasPrefix(got[0], "nkr · said") {
		t.Errorf("K past the top landed on %q, want the first comment", got[0])
	}

	bottom := press(top, "J", "J", "J")
	if got := barred(bottom.View()); !strings.HasPrefix(got[0], "octobot · said") {
		t.Errorf("J past the end landed on %q, want the last comment", got[0])
	}
}

// Where the reader left the sub-cursor is where they find it. Tab past a thread
// and back is not them changing their mind about which comment they meant.
func TestTheSubCursorIsRememberedPerThread(t *testing.T) {
	stepped := press(onThread(t, tabThread), "K")

	// A full lap of the ring, which is eight stops on this fixture.
	away := tabbed(stepped, 8)
	if got := focusedCard(t, away.View()); !strings.HasPrefix(got, cardThread) {
		t.Fatalf("a lap of the ring landed on %q, want back on the thread", got)
	}
	if got := barred(away.View()); !strings.HasPrefix(got[0], "nkr · said") {
		t.Errorf("coming back, the bar is on %q, want where it was left", got[0])
	}
}

// A single-comment thread has nothing to disambiguate, and a bar there is a
// second mark for what the card's own border already says.
func TestASingleCommentThreadTakesNoBar(t *testing.T) {
	on := onThread(t, tabOther).View()

	if got := focusedCard(t, on); !strings.HasPrefix(got, "internal/tui/keys/keys.go:7") {
		t.Fatalf("the seventh tab focused %q, want the second answerable thread", got)
	}
	if strings.Contains(on, fgSeq(theme.RosePineMoon.Secondary)+"m▍") {
		t.Error("a one-comment thread drew a bar")
	}
}

// Reserved whether it is drawn or not, so the words do not reflow as the bar
// moves down the card.
func TestTheBarCostsTheSameSpaceWhenItIsNotDrawn(t *testing.T) {
	resting := stripANSI(detailed(held(sampleDetail()), 200, 60).View())
	focused := stripANSI(onThread(t, tabThread).View())

	if strings.Count(resting, "This backs off forever.") != 1 {
		t.Fatal("the fixture comment is not on the resting frame once")
	}
	if strings.Count(focused, "This backs off forever.") != 1 {
		t.Error("the comment reflowed when the thread took focus")
	}
}

// Opening a box must not take the thread off the screen with it. The comments
// sit above the box, so a scroll that puts the box on the top row leaves the
// reader answering something they can no longer see.
func TestOpeningTheBoxKeepsTheThreadInView(t *testing.T) {
	// Short enough that the card and its box cannot both fit whole, which is
	// the case the scroll has to get right.
	m := tabbed(detailed(held(sampleDetail()), 160, 24), tabThread)

	before := stripANSI(m.View())
	if !strings.Contains(before, "Seconded, the cap is the fix.") {
		t.Fatal("the thread is not on screen to begin with, so this proves nothing")
	}

	out := stripANSI(press(m, "r").View())
	box := strings.Index(out, "write a reply")
	answered := strings.Index(out, "Seconded, the cap is the fix.")

	switch {
	case box < 0:
		t.Fatalf("no box opened:\n%s", out)
	case answered < 0:
		t.Errorf("opening the box scrolled the thread off the screen:\n%s", out)
	case answered > box:
		t.Errorf("the box opened above the comment it answers:\n%s", out)
	}
}

// GitHub has no reply for a loose comment, so r says so by doing nothing rather
// than opening the box c already opens.
func TestReplyIsInertOnSomethingWithNoThread(t *testing.T) {
	for _, at := range []struct {
		name string
		tabs int
	}{
		{"the description", 1},
		{"a top-level comment", 2},
		{"a review's own words", 3},
	} {
		t.Run(at.name, func(t *testing.T) {
			m := onThread(t, at.tabs)
			if after := press(m, "r").View(); after != m.View() {
				t.Errorf("r opened something on %s:\n%s", at.name, stripANSI(after))
			}
		})
	}
}

// The bar says which comment the keys have. Once a box is open the keys are all
// going into it, and the box has a border of its own to say so, so a bar left
// on a comment would claim they act somewhere they do not.
func TestOpeningTheBoxTakesTheBarOffTheComment(t *testing.T) {
	if before := barred(onThread(t, tabThread).View()); len(before) == 0 {
		t.Fatal("no bar on the focused thread to begin with")
	}

	if after := barred(replying(t, tabThread, "r").View()); len(after) > 0 {
		t.Errorf("the thread still carries a bar with the box open: %q", after)
	}
}

// The Files tab has no ring, so no bar can ever draw there and the two columns
// it would need are nothing but a wider indent.
func TestTheFilesTabReservesNoRoomForABarItCannotDraw(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 60)
	m.SetFiles(loadedFiles(sampleFiles(), 0))

	frame := press(m, "]", "]", "]").View()
	line := strings.Split(stripANSI(frame), "\n")[lineOf(t, frame, "This backs off forever.")]

	// The comment sits one gutter in from its own card's border, the way it does
	// in the conversation when nothing is focused. Three would mean the bar's two
	// columns are still being held for a bar that cannot draw here.
	//
	// Counted in runes. A box-drawing character is three bytes, so byte offsets
	// report a gutter three times the width the reader sees.
	runes := []rune(line)
	text := sliceIndex(runes, []rune("This backs off forever."))
	card := lastRune(runes[:text], '│')
	if gap := text - card - 1; gap != cardGutterCols {
		t.Errorf("the comment sits %d columns in from its border, want %d: %q", gap, cardGutterCols, line)
	}
}

// cardGutterCols is the space a card puts between its border and its content.
const cardGutterCols = 1

func sliceIndex(hay, needle []rune) int {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if string(hay[i:i+len(needle)]) == string(needle) {
			return i
		}
	}
	return -1
}

func lastRune(hay []rune, want rune) int {
	for i := len(hay) - 1; i >= 0; i-- {
		if hay[i] == want {
			return i
		}
	}
	return -1
}

// Resolved threads are closed, not absent. GitHub hides them for the same
// reason, and a bare line was the one block on the page with no border to take
// the accent.
func TestAResolvedThreadIsACardThatOpens(t *testing.T) {
	// The fifth tab is the resolved thread.
	closed := onThread(t, 5)

	if got := focusedCard(t, closed.View()); !strings.HasPrefix(got, "✓ internal/store/store.go:88") {
		t.Fatalf("the fifth tab focused %q, want the resolved thread's card", got)
	}
	out := stripANSI(closed.View())
	if !strings.Contains(out, "▸ 2 comments") {
		t.Errorf("the closed card does not say what is behind it:\n%s", out)
	}
	if strings.Contains(out, "Typo.") {
		t.Error("the resolved thread is showing its comments while closed")
	}

	open := press(closed, "o")
	if out := stripANSI(open.View()); !strings.Contains(out, "Typo.") {
		t.Errorf("o did not open the resolved thread:\n%s", out)
	}

	// And once open it answers like any other thread.
	if out := stripANSI(press(open, "r").View()); !strings.Contains(out, "write a reply") {
		t.Errorf("r did not open a box on an opened resolved thread:\n%s", out)
	}

	if out := stripANSI(press(open, "o").View()); strings.Contains(out, "Typo.") {
		t.Error("o did not close it again")
	}
}

// o works on any body carrying a <details> block, so the hint has to name it
// there and nowhere else. A key missing from the line is the same lie as a key
// that does not work, told the other way round.
func TestTheExpandHintFollowsTheFolds(t *testing.T) {
	d := sampleDetail()
	d.Body = "The problem.\n\n<details>\n<summary>What it does</summary>\n\nIt retries forever.\n\n</details>\n"
	m := detailed(held(d), 200, 60)

	// The description has a fold, so both keys are named and both work.
	folded := stripANSI(tabbed(m, 1).View())
	if !strings.Contains(folded, "R quote · o expand") {
		t.Errorf("the description has a fold and does not offer o:\n%s", folded)
	}
	if out := stripANSI(press(tabbed(m, 1), "o").View()); !strings.Contains(out, "It retries forever") {
		t.Error("o is named on the description and does nothing")
	}

	// The comment below it has none, so o is not offered.
	plain := stripANSI(tabbed(m, 2).View())
	if strings.Contains(plain, "o expand") {
		t.Errorf("a body with nothing to unfold still offers o:\n%s", plain)
	}
	if !strings.Contains(plain, "R quote") {
		t.Errorf("the comment lost its quote hint:\n%s", plain)
	}
}

// A fold is inline inside a body, not a block of the page. It reads as prose
// with a marker on it, and a border around one line inside an already-bordered
// card is more chrome than the thing it wraps.
func TestAFoldedBlockIsALineNotABox(t *testing.T) {
	d := sampleDetail()
	// Prose either side of the fold, so the card's own borders are not the
	// lines this looks at.
	d.Body = "The problem.\n\n<details>\n<summary>What it does</summary>\n\nIt retries forever.\n\n</details>\n\nThe fix.\n"

	frame := detailed(held(d), 200, 60).View()
	lines := strings.Split(stripANSI(frame), "\n")
	at := lineOf(t, frame, "▸ What it does")

	for _, edge := range []struct {
		at   int
		what string
	}{{at - 1, "above"}, {at + 1, "below"}} {
		if strings.Contains(lines[edge.at], "╭") || strings.Contains(lines[edge.at], "╰") {
			t.Errorf("the fold has a border %s it:\n%s", edge.what, strings.Join(lines[at-2:at+2], "\n"))
		}
	}
}

// The keys a card answers to ride in its bottom border, and only the ones that
// do anything to it. A key named on a card it is inert on is the lie the line
// exists to prevent.
func TestAFocusedCardNamesItsKeysInTheBorder(t *testing.T) {
	if out := stripANSI(onThread(t, tabThread).View()); !strings.Contains(out, "J/K in thread · r reply · R quote") {
		t.Errorf("the thread card names none of its keys:\n%s", out)
	}

	// One comment, so there is nothing to step between.
	single := stripANSI(onThread(t, tabOther).View())
	if strings.Contains(single, "J/K in thread") {
		t.Error("a one-comment thread offers a key that would do nothing")
	}
	if !strings.Contains(single, "r reply · R quote") {
		t.Errorf("a one-comment thread does not name reply:\n%s", single)
	}

	// No reply permitted, so neither key is named.
	if locked := stripANSI(onThread(t, tabLocked).View()); strings.Contains(locked, "r reply") {
		t.Error("a thread that takes no reply still offers r")
	}

	// A closed thread offers the one key that changes that.
	if closed := stripANSI(onThread(t, 5).View()); !strings.Contains(closed, "o open") {
		t.Errorf("the closed thread does not name the key that opens it:\n%s", closed)
	}

	// A block with no thread gets the quote and not the reply.
	comment := stripANSI(onThread(t, 2).View())
	if !strings.Contains(comment, "R quote") || strings.Contains(comment, "r reply") {
		t.Errorf("a loose comment names the wrong keys:\n%s", comment)
	}
}

// Hints are for the card the keys are going to. On every card at once they are
// wallpaper.
func TestAnUnfocusedCardNamesNothing(t *testing.T) {
	resting := stripANSI(detailed(held(sampleDetail()), 200, 60).View())

	for _, hint := range []string{"R quote", "r reply", "J/K in thread", "o open"} {
		if strings.Contains(resting, hint) {
			t.Errorf("%q is on a page with nothing focused", hint)
		}
	}
}

// Plain r leaves the buffer alone. A reader who wanted the quote has a key for
// it, and one who did not would have to clear five lines before writing.
func TestReplyDoesNotQuote(t *testing.T) {
	if out := stripANSI(replying(t, tabThread, "r").View()); strings.Contains(out, "> This backs off") {
		t.Error("r quoted the comment without being asked")
	}
}

// Typing goes into the box and nowhere else. r is a letter in there, and so is
// every other key this screen binds.
func TestTheReplyBoxTakesTheKeyboard(t *testing.T) {
	out := stripANSI(typed(replying(t, tabThread, "r"), "capped it").View())

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
	out := stripANSI(typed(replying(t, tabThread, "r"), "cdoqr").View())

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
	closed := press(typed(replying(t, tabThread, "r"), "capped it"), "esc")

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
	held := press(typed(replying(t, tabThread, "r"), "capped it"), "esc")

	// esc put the ring back on the thread the box was opened from, so three
	// tabs reach the next thread that takes a reply.
	other := press(tabbed(held, tabOther-tabThread), "r")

	out := stripANSI(other.View())
	if !strings.Contains(out, "write a reply") {
		t.Fatalf("the second thread did not open a box:\n%s", out)
	}
	if strings.Contains(out, "capped it") {
		t.Errorf("a draft leaked onto another thread:\n%s", out)
	}
}

// esc puts the reader back where they were rather than nowhere. The card that
// opened the box is the one they were reading.
func TestEscGivesFocusBackToTheComment(t *testing.T) {
	closed := press(replying(t, tabThread, "r"), "esc")

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
	onFiles := press(press(typed(replying(t, tabThread, "r"), "capped it"), "esc"), "]", "]", "]")

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
		m := press(tabbed(detailed(held(sampleDetail()), size.width, size.height), tabThread), "r")
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
	joined := typed(replying(t, tabThread, "r"), "capped it")

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
	m, cmd := chord(typed(replying(t, tabThread, "r"), "capped it"))

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
	if _, cmd := chord(replying(t, tabThread, "r")); cmd != nil {
		t.Error("an empty box asked the root to post")
	}
}

// A posted reply is not a draft. Reopening the box on that thread must not
// serve the words back as though they never left.
func TestPostingClearsTheDraft(t *testing.T) {
	m, _ := chord(typed(replying(t, tabThread, "r"), "capped it"))

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
	back := press(tabbed(press(m, "esc"), tabThread), "r")
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

// The accent lands on the box rather than on the thread above it, because the
// box is where the keys are going. Two lit cards would say the keys are in two
// places.
func TestTheOpenBoxTakesTheAccent(t *testing.T) {
	frame := replying(t, tabThread, "r").View()

	if got := focusedCard(t, frame); !strings.HasPrefix(got, "write a reply") {
		t.Errorf("the lit card is %q, want the box", got)
	}

	// focusedCard reads the first lit card down the page, and the thread sits
	// above the box, so finding the box means the thread is not lit.
	if !strings.Contains(stripANSI(frame), cardThread) {
		t.Error("the thread went off the screen when the box opened")
	}
}
