package prview_test

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

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

// onThread puts the ring on a card by step count.
func onThread(t *testing.T, n int) prview.Model {
	t.Helper()
	return walked(detailed(held(sampleDetail()), 200, 60), n)
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

// A reply hangs off the thread the way a thread hangs off the review that
// opened it: one level in, on a rail of its own.
func TestAReplyIsSetInFromTheThreadItAnswers(t *testing.T) {
	lines := strings.Split(stripANSI(onThread(t, tabThread).View()), "\n")

	thread := cardEdgeColumn(t, lines, "internal/gh/client.go:42")
	reply := cardEdgeColumn(t, lines, "octobot · said · 1h")
	if got := reply - thread; got != treeGutterCols {
		t.Errorf("the reply card starts %d cells in from its thread, want %d", got, treeGutterCols)
	}
}

// The rail joins them: an elbow into each reply's byline, and a corner on the
// last of them so the run stops rather than trailing into whatever comes next.
// The box is the last one while it is open, which is where its answer will land.
func TestTheRailJoinsRepliesToTheThreadTheyAnswer(t *testing.T) {
	lines := strings.Split(stripANSI(replying(t, tabThread, "r").View()), "\n")

	for _, at := range []struct {
		what, heading, join string
	}{
		{"the reply", "octobot · said · 1h", "├─"},
		{"the box", "write a reply", "╰─"},
	} {
		i := headingRow(t, lines, at.heading)
		if !strings.Contains(lines[i], at.join) {
			t.Errorf("%s does not hang off the rail with %q: %q", at.what, at.join, lines[i])
		}
		// The elbow meets the byline rather than the top border, which is where
		// the eye goes and where the card's own name sits.
		if !strings.Contains(lines[i-1], "╭") {
			t.Errorf("%s takes the rail on a row that is not its byline: %q", at.what, lines[i-1])
		}
	}
}

// treeGutterCols is what one level of the rail costs, counted the way a reader
// sees it rather than in bytes.
const treeGutterCols = 2

// headingRow is the row a card's heading landed on.
func headingRow(t *testing.T, lines []string, heading string) int {
	t.Helper()

	for i, line := range lines {
		if strings.Contains(line, heading) {
			return i
		}
	}
	t.Fatalf("no card headed %q:\n%s", heading, strings.Join(lines, "\n"))
	return 0
}

// footerRow is the bottom border of the card whose heading is on row at, which
// is where a card names the keys it answers to.
func footerRow(t *testing.T, lines []string, at int) string {
	t.Helper()

	// Matched on the closing corner rather than the opening one, because the
	// rail's own elbow puts a ╰ on the heading row itself.
	for _, line := range lines[at:] {
		if strings.Contains(line, "╯") {
			return line
		}
	}
	t.Fatalf("the card headed on row %d never closes:\n%s", at, strings.Join(lines[at:], "\n"))
	return ""
}

// cardEdgeColumn is the column a card's top border starts at, counted in cells:
// the rail drawn in its gutter is three bytes to the cell.
func cardEdgeColumn(t *testing.T, lines []string, heading string) int {
	t.Helper()

	border := lines[headingRow(t, lines, heading)-1]
	at := strings.Index(border, "╭")
	if at < 0 {
		t.Fatalf("the card headed %q opens on no border: %q", heading, border)
	}
	return utf8.RuneCountInString(border[:at])
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

	if !strings.Contains(out, "> This backs off forever.") {
		t.Errorf("the quote is not in the box:\n%s", out)
	}
	if !strings.Contains(out, "no") {
		t.Error("the cursor is not under the quote")
	}
}

// Tab lands on a thread, not on a comment inside it, so a quote has to pick one.
// The comment that opened it is the answer until J or K says otherwise: it is
// the one the thread is about, and it sits in the card the anchor and the code
// are in.
func TestQuoteReplyTakesTheOpeningCommentByDefault(t *testing.T) {
	out := stripANSI(replying(t, tabThread, "R").View())

	if !strings.Contains(out, "> This backs off forever.") {
		t.Errorf("R quoted the wrong comment:\n%s", out)
	}
	if strings.Contains(out, "> Seconded, the cap is the fix.") {
		t.Error("R quoted a comment the sub-cursor was not on")
	}
}

// J steps the sub-cursor down the thread, which is what makes answering a reply
// rather than the comment it answers possible.
func TestSteppingWithinTheThreadMovesWhatAQuoteTakes(t *testing.T) {
	out := stripANSI(press(onThread(t, tabThread), "J", "R").View())

	if !strings.Contains(out, "> Seconded, the cap is the fix.") {
		t.Errorf("J did not move what R quotes:\n%s", out)
	}
	if strings.Contains(out, "> This backs off forever.") {
		t.Error("R quoted the comment the sub-cursor left")
	}
}

// litCards is the heading of every card drawn in the accent, top to bottom.
// Exactly one card is lit at a time, so this is mostly a way of proving that.
func litCards(t *testing.T, frame string) []string {
	t.Helper()

	accent := fgSeq(theme.RosePineMoon.Accent)
	lines := strings.Split(frame, "\n")

	var out []string
	for i, line := range lines {
		at := strings.Index(line, "╭")
		if at < 0 || i+1 >= len(lines) || strings.HasPrefix(stripANSI(line), "╭") {
			continue
		}
		start := strings.LastIndex(line[:at], "\x1b[")
		if start < 0 || !strings.HasPrefix(line[start+2:], accent) {
			continue
		}
		out = append(out, cardHeading(stripANSI(lines[i+1]), stripANSI(line)))
	}
	return out
}

// subCursor is the heading of the one lit card, which is the comment the keys
// are on. The comment that opened a thread has no card of its own, so the
// thread's own heading stands for it.
func subCursor(t *testing.T, frame string) string {
	t.Helper()

	lit := litCards(t, frame)
	if len(lit) != 1 {
		t.Errorf("%d cards are lit, want exactly one: %q", len(lit), lit)
		return ""
	}
	return lit[0]
}

// One lit border at a time. A reply is a card, so its own border says the keys
// are on it, and the thread card goes dark when they leave it: two lit borders
// are two claims about where a press lands.
func TestTheSubCursorMovesTheLitBorder(t *testing.T) {
	// It opens on the comment that opened the thread, which is the one the
	// thread is about and the one the anchor and the code sit with.
	if got := subCursor(t, onThread(t, tabThread).View()); !strings.HasPrefix(got, cardThread) {
		t.Errorf("the sub-cursor opens on %q, want the thread card", got)
	}

	// J moves it down onto the reply, and the thread card goes dark.
	if got := subCursor(t, press(onThread(t, tabThread), "J").View()); !strings.HasPrefix(got, "octobot · said") {
		t.Errorf("after J the lit card is %q, want the reply", got)
	}

	// And nothing lights on a thread that is not the focus.
	if got := litCards(t, onThread(t, tabLocked).View()); slices.Contains(got, "octobot · said · 1h") {
		t.Errorf("a reply is lit on a thread that does not hold the focus: %q", got)
	}
}

// The keys ride in the border of the card that is lit, because that is the card
// they act on. Left on the thread while a reply holds them, the footer would
// name e and D over words they no longer reach.
func TestTheHintsFollowTheLitCard(t *testing.T) {
	lines := strings.Split(stripANSI(press(onThread(t, tabThread), "J").View()), "\n")

	reply := footerRow(t, lines, headingRow(t, lines, "octobot · said · 1h"))
	if !strings.Contains(reply, "r reply") {
		t.Errorf("the reply holding the keys names none of them: %q", reply)
	}

	thread := footerRow(t, lines, headingRow(t, lines, "internal/gh/client.go:42"))
	if strings.Contains(thread, "r reply") {
		t.Errorf("the thread names keys while a reply holds them: %q", thread)
	}
}

// It clamps at both ends. Running off a thread is a reader asking for the block
// above or below it, and tab is the key for that.
func TestTheSubCursorStopsAtTheEndsOfTheThread(t *testing.T) {
	m := onThread(t, tabThread)

	top := press(m, "K", "K", "K")
	if got := subCursor(t, top.View()); !strings.HasPrefix(got, cardThread) {
		t.Errorf("K past the top landed on %q, want the thread card", got)
	}

	bottom := press(top, "J", "J", "J")
	if got := subCursor(t, bottom.View()); !strings.HasPrefix(got, "octobot · said") {
		t.Errorf("J past the end landed on %q, want the last comment", got)
	}
}

// Where the reader left the sub-cursor is where they find it. Tab past a thread
// and back is not them changing their mind about which comment they meant.
func TestTheSubCursorIsRememberedPerThread(t *testing.T) {
	stepped := press(onThread(t, tabThread), "J")

	// A full lap of the ring, which is eight stops on this fixture.
	away := walked(stepped, 8)
	if got := subCursor(t, away.View()); !strings.HasPrefix(got, "octobot · said") {
		t.Errorf("coming back, the sub-cursor is on %q, want where it was left", got)
	}
}

// A single-comment thread has nothing to disambiguate, and nothing to hang off
// the rail either: the card's own border is the whole of the answer.
func TestASingleCommentThreadLightsOnlyItself(t *testing.T) {
	on := onThread(t, tabOther).View()

	if got := focusedCard(t, on); !strings.HasPrefix(got, "internal/tui/keys/keys.go:7") {
		t.Fatalf("the seventh tab focused %q, want the second answerable thread", got)
	}
	if got := litCards(t, on); len(got) != 1 {
		t.Errorf("a one-comment thread lit %q, want the card alone", got)
	}
}

// Nothing about the comment that opened the thread moves as the sub-cursor
// arrives: it sits inside the thread card, and the replies that light are cards
// of their own below it.
func TestTakingTheThreadDoesNotReflowTheCommentThatOpenedIt(t *testing.T) {
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
	m := walked(detailed(held(sampleDetail()), 160, 24), tabThread)

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

// The sub-cursor says which comment the keys have. Once a box is open the keys
// are all going into it, and the box has a border of its own to say so, so a
// reply left lit would claim they act somewhere they do not.
func TestOpeningTheBoxTakesTheSubCursorOffTheReply(t *testing.T) {
	if before := subCursor(t, onThread(t, tabThread).View()); before == "" {
		t.Fatal("no sub-cursor on the focused thread to begin with")
	}

	// The box takes the focus off the thread outright, so it is the one lit card
	// on the page rather than a second one under a thread still lit.
	after := litCards(t, replying(t, tabThread, "r").View())
	if len(after) != 1 || !strings.Contains(after[0], "write a reply") {
		t.Errorf("with the box open the lit cards are %q, want the box alone", after)
	}
}

// The comment that opened a thread sits inside the thread's own card, one gutter
// in from its border, on both tabs.
func TestTheCommentThatOpenedAThreadSitsInsideItsCard(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 60)
	m.SetFiles(loadedFiles(sampleFiles(), 0))

	frame := press(m, "]", "]", "]").View()
	line := strings.Split(stripANSI(frame), "\n")[lineOf(t, frame, "This backs off forever.")]

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
	folded := stripANSI(walked(m, 1).View())
	if !strings.Contains(folded, "R quote · o expand") {
		t.Errorf("the description has a fold and does not offer o:\n%s", folded)
	}
	if out := stripANSI(press(walked(m, 1), "o").View()); !strings.Contains(out, "It retries forever") {
		t.Error("o is named on the description and does nothing")
	}

	// The comment below it has none, so o is not offered.
	plain := stripANSI(walked(m, 2).View())
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

// A comment with no body quotes to nothing. Splitting an empty string yields one
// empty line, so a naive quote seeds the box with a bare marker standing over
// nothing.
func TestQuotingAnEmptyCommentSeedsNothing(t *testing.T) {
	d := sampleDetail()
	// The sub-cursor opens on the comment that opened the thread, so that is the
	// one R takes.
	d.Threads[0].Comments[0].Body = ""

	m := walked(detailed(held(d), 200, 60), tabThread)
	box := boxLines(t, press(m, "R").View())

	for _, line := range box {
		if strings.HasPrefix(line, ">") {
			t.Errorf("R on an empty comment seeded a blockquote: %q", box)
			break
		}
	}

	// The box still opens: the reader asked to write, and having nothing to
	// quote is not a reason to refuse.
	if len(box) == 0 {
		t.Errorf("R on an empty comment opened no box:\n%s", stripANSI(press(m, "R").View()))
	}
}

// boxLines is what the open reply box holds, from its heading down to its hint
// row.
func boxLines(t *testing.T, frame string) []string {
	t.Helper()

	lines := strings.Split(stripANSI(frame), "\n")
	at := -1
	for i, line := range lines {
		if strings.Contains(line, "write a reply") {
			at = i
		}
	}
	if at < 0 {
		return nil
	}

	var out []string
	for _, line := range lines[at:] {
		text := strings.TrimSpace(strings.Trim(line, "│╭╮╰╯─ "))
		if strings.Contains(line, "esc done") {
			break
		}
		out = append(out, text)
	}
	return out
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
	other := press(walked(held, tabOther-tabThread), "r")

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
		m := press(walked(detailed(held(sampleDetail()), size.width, size.height), tabThread), "r")
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
	back := press(walked(press(m, "esc"), tabThread), "r")
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
