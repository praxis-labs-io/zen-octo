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

func TestCOpensTheComposer(t *testing.T) {
	before := stripANSI(detailed(held(sampleDetail()), 200, 40).View())
	if strings.Contains(before, "Leave a comment") {
		t.Fatal("the composer is open before c is pressed")
	}

	out := stripANSI(composing(200, 40).View())
	if !strings.Contains(out, "Leave a comment") {
		t.Error("c did not open the composer")
	}
	if !strings.Contains(out, "Comment on #412") {
		t.Errorf("the composer does not say what it is writing on:\n%s", out)
	}
}

// The composer takes its height off the panes rather than adding it to the
// frame. A screen that grows past the height it was handed writes over the
// status bar.
func TestTheFrameStillFillsItsSizeWithTheComposerOpen(t *testing.T) {
	sizes := []struct{ width, height int }{
		{width: 200, height: 40},
		{width: 160, height: 24},
		{width: 100, height: 20},
		{width: 60, height: 12},

		// Shorter than the composer wants. It gives way rather than pushing the
		// panes past the bottom of the frame.
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

// esc is the same reflex here as everywhere else on this screen. Losing three
// paragraphs to it once is enough to stop anyone using the pane.
func TestEscapeKeepsTheDraftAndCBringsItBack(t *testing.T) {
	m := typed(composing(200, 40), "half written")
	m = press(m, "esc")

	if got := stripANSI(m.View()); strings.Contains(got, "half written") {
		t.Fatal("esc left the composer open")
	}

	if got := stripANSI(press(m, "c").View()); !strings.Contains(got, "half written") {
		t.Errorf("c did not bring the draft back:\n%s", got)
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
	return strings.Contains(frame, bgSeq(theme.RosePineMoon.Secondary)+"m"+"[ Post ]")
}

// The button sits against the right edge of the pane, one column in, which is
// the corner every dialog puts its confirm in.
func TestThePostButtonSitsInTheBottomRight(t *testing.T) {
	m := typed(composing(200, 40), "ship it")

	lines := strings.Split(stripANSI(m.View()), "\n")
	at := -1
	for i, line := range lines {
		if strings.Contains(line, "[ Post ]") {
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

	// One column of gutter between the label and the border it sits against.
	tail := lines[at][strings.Index(lines[at], "[ Post ]")+len("[ Post ]"):]
	if want := " │"; !strings.HasPrefix(tail, want) {
		t.Errorf("the button is followed by %q, want it against the right border", tail)
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

// The composer belongs to the conversation, not to the screen. It takes its
// height off the conversation pane alone: a comment being written has nothing
// to do with the rail, and shortening the rail for one is the screen
// rearranging itself around a box in the column beside it.
func TestTheComposerTakesItsHeightFromTheConversationAlone(t *testing.T) {
	closed := detailed(held(sampleDetail()), 200, 40)
	open := press(closed, "c")

	before, after := railRows(t, closed.View()), railRows(t, open.View())
	if len(before) != len(after) {
		t.Errorf("the rail is %d rows with the composer open and %d without", len(after), len(before))
	}

	// Both columns still reach the foot of the frame.
	last := strings.Split(stripANSI(open.View()), "\n")
	if got := last[len(last)-1]; strings.Count(got, "╯") != 2 {
		t.Errorf("the last line closes %d panes, want the composer and the rail: %q",
			strings.Count(got, "╯"), got)
	}
}

// It is also no wider than the conversation. Spanning the frame would put it
// under the rail, which is not where the comment is going to appear.
func TestTheComposerIsAsWideAsTheConversation(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 200, 40), "c")

	lines := strings.Split(stripANSI(m.View()), "\n")
	title, foot := -1, -1
	for i, line := range lines {
		if strings.Contains(line, "Comment on #") {
			title = i
		}
		if strings.Contains(line, "Conversation - Commits") {
			foot = i
		}
	}
	if title < 0 || foot < 0 {
		t.Fatalf("could not find both panes:\n%s", strings.Join(lines, "\n"))
	}

	// The conversation's top border and the composer's each close at the same
	// column, which is where the rail begins. Measured in cells: a box rune is
	// three bytes and a tab strip is one per character, so the byte offsets
	// differ on lines that close at the same place.
	closesAt := func(line string) int {
		return lipgloss.Width(line[:strings.Index(line, "╮")])
	}
	if a, b := closesAt(lines[foot]), closesAt(lines[title]); a != b {
		t.Errorf("the composer closes at column %d and the conversation at %d", b, a)
	}
}
