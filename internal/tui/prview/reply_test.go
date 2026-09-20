package prview_test

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

const (
	tabThread = 4
	tabReply  = 5
	tabLocked = 7
	tabOther  = 8
)

func onThread(t *testing.T, n int) prview.Model {
	t.Helper()
	return walked(detailed(held(sampleDetail()), 200, 60), n)
}

func replying(t *testing.T, n int, key string) prview.Model {
	t.Helper()
	return press(onThread(t, n), key)
}

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

func TestAReplyIsSetInFromTheThreadItAnswers(t *testing.T) {
	lines := strings.Split(stripANSI(onThread(t, tabThread).View()), "\n")

	thread := cardEdgeColumn(t, lines, "internal/gh/client.go:42")
	reply := cardEdgeColumn(t, lines, "octobot · said · 1h")
	if got := reply - thread; got != treeGutterCols {
		t.Errorf("the reply card starts %d cells in from its thread, want %d", got, treeGutterCols)
	}
}

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
		if !strings.Contains(lines[i-1], "╭") {
			t.Errorf("%s takes the rail on a row that is not its byline: %q", at.what, lines[i-1])
		}
	}
}

const treeGutterCols = 2

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

func footerRow(t *testing.T, lines []string, at int) string {
	t.Helper()

	for _, line := range lines[at:] {
		if strings.Contains(line, "╯") {
			return line
		}
	}
	t.Fatalf("the card headed on row %d never closes:\n%s", at, strings.Join(lines[at:], "\n"))
	return ""
}

// Counted in cells: the rail glyph in the gutter is three bytes.
func cardEdgeColumn(t *testing.T, lines []string, heading string) int {
	t.Helper()

	border := lines[headingRow(t, lines, heading)-1]
	at := strings.Index(border, "╭")
	if at < 0 {
		t.Fatalf("the card headed %q opens on no border: %q", heading, border)
	}
	return utf8.RuneCountInString(border[:at])
}

func TestReplyIsInertOnAThreadThatTakesNoReply(t *testing.T) {
	locked := onThread(t, tabLocked)

	before := locked.View()
	if got := focusedCard(t, before); !strings.HasPrefix(got, cardLocked) {
		t.Fatalf("the ring landed on %q, want the locked thread", got)
	}

	if after := press(locked, "r").View(); after != before {
		t.Errorf("r opened something on a thread that takes no reply:\n%s", stripANSI(after))
	}
	if after := press(locked, "R").View(); after != before {
		t.Errorf("R opened something on a thread that takes no reply:\n%s", stripANSI(after))
	}
}

func TestReplyNeedsSomethingFocused(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 60)

	if after := press(m, "r").View(); after != m.View() {
		t.Error("r opened a box with nothing focused")
	}
}

func TestReplyingToSomethingWithNoThreadUsesTheCommentBox(t *testing.T) {
	out := stripANSI(press(onThread(t, 2), "R").View())

	if !strings.Contains(out, "> Coverage held at 84.2%.") {
		t.Errorf("R did not quote the comment into the box:\n%s", out)
	}
	if strings.Contains(out, "write a reply") {
		t.Error("R opened a thread box for a comment with no thread")
	}

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

func TestQuoteReplyTakesTheCommentOfTheCardItIsOn(t *testing.T) {
	out := stripANSI(replying(t, tabThread, "R").View())

	if !strings.Contains(out, "> This backs off forever.") {
		t.Errorf("R quoted the wrong comment:\n%s", out)
	}
	if strings.Contains(out, "> Seconded, the cap is the fix.") {
		t.Error("R quoted a comment the sub-cursor was not on")
	}
}

func TestQuotingAReplyTakesTheReply(t *testing.T) {
	out := stripANSI(press(onThread(t, tabReply), "R").View())

	if !strings.Contains(out, "> Seconded, the cap is the fix.") {
		t.Errorf("R on the reply quoted something else:\n%s", out)
	}
	if strings.Contains(out, "> This backs off forever.") {
		t.Error("R quoted the comment the reply answers rather than the reply")
	}
}

func litCards(t *testing.T, frame string) []string {
	t.Helper()

	accent := fgSeq(testTheme.Accent)
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

func subCursor(t *testing.T, frame string) string {
	t.Helper()

	lit := litCards(t, frame)
	if len(lit) != 1 {
		t.Errorf("%d cards are lit, want exactly one: %q", len(lit), lit)
		return ""
	}
	return lit[0]
}

func TestOnlyTheCardTheRingIsOnIsLit(t *testing.T) {
	if got := subCursor(t, onThread(t, tabThread).View()); !strings.HasPrefix(got, cardThread) {
		t.Errorf("the thread step lit %q, want the thread card", got)
	}
	if got := subCursor(t, onThread(t, tabReply).View()); !strings.HasPrefix(got, "octobot · said") {
		t.Errorf("the reply step lit %q, want the reply", got)
	}

	if got := litCards(t, onThread(t, tabLocked).View()); slices.Contains(got, "octobot · said · 1h") {
		t.Errorf("a reply is lit on a thread that does not hold the focus: %q", got)
	}
}

func TestAReplyNamesOnlyTheKeysItAnswersTo(t *testing.T) {
	lines := strings.Split(stripANSI(onThread(t, tabReply).View()), "\n")
	reply := footerRow(t, lines, headingRow(t, lines, "octobot · said · 1h"))

	if !strings.Contains(reply, "r reply") {
		t.Errorf("the reply holding the keys names none of them: %q", reply)
	}
	for _, key := range []string{"x resolve", "v in diff", "x unresolve"} {
		if strings.Contains(reply, key) {
			t.Errorf("the reply names %q, which acts on the thread and not on it: %q", key, reply)
		}
	}

	thread := footerRow(t, lines, headingRow(t, lines, "internal/gh/client.go:42"))
	if strings.Contains(thread, "r reply") {
		t.Errorf("the thread names keys while a reply holds them: %q", thread)
	}
}

func TestAnOpenedResolvedThreadNamesTheFoldKeyOnce(t *testing.T) {
	d := sampleDetail()
	d.Threads[1].Comments[0].Body = "<details><summary>The trace</summary>\n\nA line of it.\n</details>"

	m := press(walked(detailed(held(d), 200, 60), tabResolved), "space")
	lines := strings.Split(stripANSI(m.View()), "\n")
	footer := footerRow(t, lines, headingRow(t, lines, "internal/store/store.go:88"))

	if strings.Contains(footer, "space expand") {
		t.Errorf("the resolved thread names a fold o does not do: %q", footer)
	}
	if !strings.Contains(footer, "space close") {
		t.Errorf("the resolved thread does not name what o does: %q", footer)
	}
}

func TestACardWithTheBoxOverItNamesNoKeys(t *testing.T) {
	for _, at := range []struct {
		what    string
		tab     int
		heading string
	}{
		{"a reply", tabReply, "octobot · said · 1h"},
		{"the comment that opened the thread", tabThread, "internal/gh/client.go:42"},
	} {
		t.Run(at.what, func(t *testing.T) {
			lines := strings.Split(stripANSI(press(onWritable(at.tab), "e").View()), "\n")
			footer := footerRow(t, lines, headingRow(t, lines, "edit this comment"))

			for _, key := range []string{"r reply", "R quote", "e edit", "D delete"} {
				if strings.Contains(footer, key) {
					t.Errorf("%s names %q with the box over it: %q", at.what, key, footer)
				}
			}
		})
	}
}

func TestTheCardHoldingTheBoxIsLit(t *testing.T) {
	for _, at := range []struct {
		what, heading string
		tab           int
	}{
		{"a reply", "drucial · edit this comment", tabReply},
		{"the comment that opened the thread", "internal/gh/client.go:42", tabThread},
	} {
		t.Run(at.what, func(t *testing.T) {
			if got := litCards(t, press(onWritable(at.tab), "e").View()); len(got) != 1 ||
				!strings.HasPrefix(got[0], at.heading) {
				t.Errorf("editing %s lit %q, want the card holding the box", at.what, got)
			}
		})
	}
}

func TestTheThreadKeysAreInertOnAReply(t *testing.T) {
	m := onThread(t, tabReply)

	for _, key := range []string{"x", "enter"} {
		if after := press(m, key).View(); after != m.View() {
			t.Errorf("%s did something from a reply:\n%s", key, stripANSI(after))
		}
	}
}

func TestTheRingWalksOffAReplyToItsEndAndStops(t *testing.T) {
	away := walked(onThread(t, tabReply), 9)
	if got := subCursor(t, away.View()); !strings.HasPrefix(got, cardCompose) {
		t.Errorf("walking past the last card landed on %q, want the comment box", got)
	}
}

func TestASingleCommentThreadLightsOnlyItself(t *testing.T) {
	on := onThread(t, tabOther).View()

	if got := focusedCard(t, on); !strings.HasPrefix(got, "internal/tui/keys/keys.go:7") {
		t.Fatalf("the seventh tab focused %q, want the second answerable thread", got)
	}
	if got := litCards(t, on); len(got) != 1 {
		t.Errorf("a one-comment thread lit %q, want the card alone", got)
	}
}

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

func TestOpeningTheBoxLandsTheControlThatSendsIt(t *testing.T) {
	for _, at := range []struct {
		what, key, sends string
	}{
		{"a reply box", "r", "post"},
		{"an edit box", "e", "save"},
	} {
		t.Run(at.what, func(t *testing.T) {
			d := writable()
			d.Threads[0].Comments[1].Body = strings.Repeat("A line of the answer.\n\n", 20)

			out := stripANSI(press(walked(viewing(d, 160, 24), tabReply), at.key).View())
			if !strings.Contains(out, at.sends) {
				t.Errorf("%s opened with its %s control below the fold:\n%s", at.what, at.sends, out)
			}
		})
	}
}

func TestOpeningTheBoxKeepsTheThreadInView(t *testing.T) {
	m := walked(detailed(held(sampleDetail()), 160, 24), tabReply)

	before := stripANSI(m.View())
	if !strings.Contains(before, "Seconded, the cap is the fix.") {
		t.Fatal("the reply is not on screen to begin with, so this proves nothing")
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

func TestOpeningTheBoxTakesTheSubCursorOffTheReply(t *testing.T) {
	if before := subCursor(t, onThread(t, tabThread).View()); before == "" {
		t.Fatal("no sub-cursor on the focused thread to begin with")
	}

	after := litCards(t, replying(t, tabThread, "r").View())
	if len(after) != 1 || !strings.Contains(after[0], "write a reply") {
		t.Errorf("with the box open the lit cards are %q, want the box alone", after)
	}
}

func TestTheCommentThatOpenedAThreadSitsInsideItsCard(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 60)
	m.SetFiles(loadedFiles(sampleFiles(), 0))

	frame := press(m, "]", "]", "]").View()
	line := strings.Split(stripANSI(frame), "\n")[lineOf(t, frame, "This backs off forever.")]

	runes := []rune(line)
	text := sliceIndex(runes, []rune("This backs off forever."))
	card := lastRune(runes[:text], '│')
	if gap := text - card - 1; gap != cardGutterCols {
		t.Errorf("the comment sits %d columns in from its border, want %d: %q", gap, cardGutterCols, line)
	}
}

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

func TestAResolvedThreadIsACardThatOpens(t *testing.T) {
	closed := onThread(t, tabResolved)

	if got := focusedCard(t, closed.View()); !strings.HasPrefix(got, "✓ internal/store/store.go:88") {
		t.Fatalf("the ring landed on %q, want the resolved thread's card", got)
	}
	out := stripANSI(closed.View())
	if !strings.Contains(out, "▸ 2 comments") {
		t.Errorf("the closed card does not say what is behind it:\n%s", out)
	}
	if strings.Contains(out, "Typo.") {
		t.Error("the resolved thread is showing its comments while closed")
	}

	open := press(closed, "space")
	if out := stripANSI(open.View()); !strings.Contains(out, "Typo.") {
		t.Errorf("o did not open the resolved thread:\n%s", out)
	}

	if out := stripANSI(press(open, "r").View()); !strings.Contains(out, "write a reply") {
		t.Errorf("r did not open a box on an opened resolved thread:\n%s", out)
	}

	if out := stripANSI(press(open, "space").View()); strings.Contains(out, "Typo.") {
		t.Error("o did not close it again")
	}
}

func TestTheExpandHintFollowsTheFolds(t *testing.T) {
	d := sampleDetail()
	d.Body = "The problem.\n\n<details>\n<summary>What it does</summary>\n\nIt retries forever.\n\n</details>\n"
	m := detailed(held(d), 200, 60)

	folded := stripANSI(walked(m, 1).View())
	if !strings.Contains(folded, "R quote · space expand") {
		t.Errorf("the description has a fold and does not offer o:\n%s", folded)
	}
	if out := stripANSI(press(walked(m, 1), "space").View()); !strings.Contains(out, "It retries forever") {
		t.Error("o is named on the description and does nothing")
	}

	plain := stripANSI(walked(m, 2).View())
	if strings.Contains(plain, "space expand") {
		t.Errorf("a body with nothing to unfold still offers o:\n%s", plain)
	}
	if !strings.Contains(plain, "R quote") {
		t.Errorf("the comment lost its quote hint:\n%s", plain)
	}
}

func TestAFoldedBlockIsALineNotABox(t *testing.T) {
	d := sampleDetail()
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

func TestAFocusedCardNamesItsKeysInTheBorder(t *testing.T) {
	if out := stripANSI(onThread(t, tabThread).View()); !strings.Contains(out, "r reply · R quote") {
		t.Errorf("the thread card names none of its keys:\n%s", out)
	}

	single := stripANSI(onThread(t, tabOther).View())
	if !strings.Contains(single, "r reply · R quote") {
		t.Errorf("a one-comment thread does not name reply:\n%s", single)
	}

	if locked := stripANSI(onThread(t, tabLocked).View()); strings.Contains(locked, "r reply") {
		t.Error("a thread that takes no reply still offers r")
	}

	if closed := stripANSI(onThread(t, tabResolved).View()); !strings.Contains(closed, "space open") {
		t.Errorf("the closed thread does not name the key that opens it:\n%s", closed)
	}

	comment := stripANSI(onThread(t, 2).View())
	if !strings.Contains(comment, "R quote") || strings.Contains(comment, "r reply") {
		t.Errorf("a loose comment names the wrong keys:\n%s", comment)
	}
}

func TestOnlyTheFocusedCardNamesItsKeys(t *testing.T) {
	resting := stripANSI(detailed(held(sampleDetail()), 200, 60).View())

	for _, hint := range []string{"R quote", "r reply", "space open"} {
		if n := strings.Count(resting, hint); n > 1 {
			t.Errorf("%q is on %d cards, want the focused one alone", hint, n)
		}
	}

	if strings.Contains(resting, "x resolve") {
		t.Error("a thread key is named while the cursor is on the description")
	}
}

func TestQuotingAnEmptyCommentSeedsNothing(t *testing.T) {
	d := sampleDetail()
	d.Threads[0].Comments[0].Body = ""

	m := walked(detailed(held(d), 200, 60), tabThread)
	box := boxLines(t, press(m, "R").View())

	for _, line := range box {
		if strings.HasPrefix(line, ">") {
			t.Errorf("R on an empty comment seeded a blockquote: %q", box)
			break
		}
	}

	if len(box) == 0 {
		t.Errorf("R on an empty comment opened no box:\n%s", stripANSI(press(m, "R").View()))
	}
}

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

func TestReplyDoesNotQuote(t *testing.T) {
	if out := stripANSI(replying(t, tabThread, "r").View()); strings.Contains(out, "> This backs off") {
		t.Error("r quoted the comment without being asked")
	}
}

func TestTheReplyBoxTakesTheKeyboard(t *testing.T) {
	out := stripANSI(typed(replying(t, tabThread, "r"), "capped it").View())

	if !strings.Contains(out, "capped it") {
		t.Errorf("the box did not take the letters:\n%s", out)
	}
	if !strings.Contains(out, "write a reply") {
		t.Error("typing closed the box")
	}
}

func TestEveryKeyIsALetterInTheReplyBox(t *testing.T) {
	out := stripANSI(typed(replying(t, tabThread, "r"), "cdoqr").View())

	if !strings.Contains(out, "cdoqr") {
		t.Errorf("a bound key was swallowed instead of typed:\n%s", out)
	}
	if strings.Contains(out, "Leave a comment") && strings.Count(out, "Leave a") > 1 {
		t.Error("c opened the compose card on top of the reply box")
	}
}

func TestOnlyOneBoxTakesTheKeysAtOnce(t *testing.T) {
	out := stripANSI(typed(composing(200, 60), "reply r").View())

	if strings.Contains(out, "write a reply") {
		t.Errorf("r opened a reply box from inside the compose card:\n%s", out)
	}
}

func TestEscClosesTheBoxAndKeepsTheWords(t *testing.T) {
	closed := press(typed(replying(t, tabThread, "r"), "capped it"), "esc")

	if out := stripANSI(closed.View()); strings.Contains(out, "write a reply") {
		t.Errorf("esc left the box on the page:\n%s", out)
	}

	if out := stripANSI(press(closed, "r").View()); !strings.Contains(out, "capped it") {
		t.Errorf("the words did not come back with the box:\n%s", out)
	}
}

func TestADraftStaysOnItsOwnThread(t *testing.T) {
	held := press(typed(replying(t, tabThread, "r"), "capped it"), "esc")

	other := press(held, "}", "}", "}", "}", "r")

	out := stripANSI(other.View())
	if !strings.Contains(out, "write a reply") {
		t.Fatalf("the second thread did not open a box:\n%s", out)
	}
	if strings.Contains(out, "capped it") {
		t.Errorf("a draft leaked onto another thread:\n%s", out)
	}
}

func TestEscGivesFocusBackToTheComment(t *testing.T) {
	closed := press(replying(t, tabThread, "r"), "esc")

	if got := focusedCard(t, closed.View()); !strings.HasPrefix(got, cardThread) {
		t.Errorf("esc focused %q, want the thread it was opened from", got)
	}
}

func TestTheReplyBoxFollowsTheWriterOntoTheFilesTab(t *testing.T) {
	closed := press(typed(replying(t, tabThread, "r"), "capped it"), "esc")
	closed.SetFiles(loadedFiles(sampleFiles(), 0))
	onFiles := press(closed, "]", "]", "]")

	out := stripANSI(onFiles.View())
	if !strings.Contains(out, "This backs off forever.") {
		t.Fatal("setup: the thread is not on the Files tab at all")
	}
	if strings.Contains(out, "write a reply") {
		t.Errorf("a reply box rendered in the diff:\n%s", out)
	}
	if strings.Contains(out, "capped it") {
		t.Errorf("a held draft leaked into the diff:\n%s", out)
	}

	onFiles.RestoreReply("RT_1", "and again")

	out = stripANSI(onFiles.View())
	if !strings.Contains(out, "and again") {
		t.Errorf("a failed reply gave the words back nowhere on the Files tab:\n%s", out)
	}
	if !onFiles.Composing() {
		t.Error("the box reopened in the diff without taking the keyboard")
	}
}

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

func TestTypingInTheBoxRendersWhatARebuildWould(t *testing.T) {
	joined := typed(replying(t, tabThread, "r"), "capped it")

	rebuilt := joined
	rebuilt.SetSize(200, 60)

	if joined.View() != rebuilt.View() {
		t.Errorf("the cached page and a rebuilt one differ:\n%s\nwant\n%s",
			stripANSI(joined.View()), stripANSI(rebuilt.View()))
	}
}

func TestTypingInTheCommentBoxRendersWhatARebuildWould(t *testing.T) {
	joined := typed(composing(200, 60), "ship it")

	rebuilt := joined
	rebuilt.SetSize(200, 60)

	if joined.View() != rebuilt.View() {
		t.Errorf("the cached page and a rebuilt one differ:\n%s\nwant\n%s",
			stripANSI(joined.View()), stripANSI(rebuilt.View()))
	}
}

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

func TestAnEmptyReplyPostsNothing(t *testing.T) {
	if _, cmd := chord(replying(t, tabThread, "r")); cmd != nil {
		t.Error("an empty box asked the root to post")
	}
}

func TestPostingClearsTheDraft(t *testing.T) {
	m, _ := chord(typed(replying(t, tabThread, "r"), "capped it"))

	if out := stripANSI(press(m, "r").View()); strings.Contains(out, "capped it") {
		t.Errorf("the posted words came back as a draft:\n%s", out)
	}
}

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

	if head := strings.Index(out, "internal/gh/client.go:42"); head < 0 ||
		strings.Index(out, "write a reply") < head {
		t.Error("the words came back somewhere other than the thread")
	}
}

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

	back := press(walked(fromTop(press(m, "esc")), tabThread), "r")
	if out := stripANSI(back.View()); !strings.Contains(out, "capped it") {
		t.Errorf("the words are not waiting on their thread:\n%s", out)
	}
}

func TestARestoredReplyToAThreadThatIsGoneOpensNothing(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 60)
	m.RestoreReply("RT_GONE", "capped it")

	if out := stripANSI(m.View()); strings.Contains(out, "write a reply") {
		t.Errorf("a box opened for a thread the page does not carry:\n%s", out)
	}
}

func TestTheOpenBoxTakesTheAccent(t *testing.T) {
	frame := replying(t, tabThread, "r").View()

	if got := focusedCard(t, frame); !strings.HasPrefix(got, "write a reply") {
		t.Errorf("the lit card is %q, want the box", got)
	}

	if !strings.Contains(stripANSI(frame), cardThread) {
		t.Error("the thread went off the screen when the box opened")
	}
}
