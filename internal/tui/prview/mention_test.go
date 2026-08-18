package prview_test

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

// typing is typed with the last command kept, for the keystroke that asks the
// root for the people a mention offers.
func typing(m prview.Model, text string) (prview.Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, r := range text {
		m, cmd = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m, cmd
}

// writing is the detail screen with the compose box open and the repository's
// people already held, which is every test that is not about the fetch.
func writing(t *testing.T) prview.Model {
	t.Helper()

	m := composing(200, 60)
	m.SetRepo(loadedRepo())
	return m
}

// arrow is a key with a name rather than a character, which is what tells a
// cursor move from a letter typed into the box.
func arrow(m prview.Model, code rune) (prview.Model, tea.Cmd) {
	return m.Update(tea.KeyPressMsg{Code: code})
}

// asking is the fetch a keystroke started, or nil. A key inside a box answers
// the textarea as well as the screen, so what comes back is a batch and the
// message this cares about is one of the two.
func asking(cmd tea.Cmd) tea.Msg {
	switch msg := runCmd(cmd).(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		for _, c := range msg {
			if found := asking(c); found != nil {
				return found
			}
		}
		return nil
	case prview.NeedRepoMetaMsg:
		return msg
	}
	return nil
}

// A handle on the rail is not the popup: the Reviewers section renders @nkr
// whatever the box is doing. A real name appears nowhere else on the screen, so
// it is what says the list is up.
const (
	onList  = "Nikita Rushmanov"
	offList = "Sam Reed"
)

func TestTheFirstAtInABoxAsksForTheRepositorysPeople(t *testing.T) {
	_, cmd := typing(composing(200, 60), "@")

	want := prview.NeedRepoMetaMsg{Repo: "zen-octo/zen-octo"}
	if got := asking(cmd); got != want {
		t.Fatalf("typing @ sent %#v, want %#v", got, want)
	}
}

// Once per screen, not once per popup. Every keystroke inside an @word re-enters
// the open path, and a request per character is what the latch is for.
func TestASecondAtAsksForNothing(t *testing.T) {
	m, _ := typing(composing(200, 60), "@")
	if _, cmd := typing(m, "dru"); asking(cmd) != nil {
		t.Errorf("typing into the token sent %#v, want nothing", asking(cmd))
	}

	m, _ = typing(m, " again @")
	if _, cmd := typing(m, "n"); asking(cmd) != nil {
		t.Errorf("a second token sent %#v, want nothing", asking(cmd))
	}
}

// A picker opened earlier has already paid for the list. Asking again would be
// a second request for something the root is holding.
func TestAnAtAsksForNothingOnceThePickersHaveFetched(t *testing.T) {
	m, cmd := typing(writing(t), "@")
	if got := asking(cmd); got != nil {
		t.Errorf("typing @ over a held repository sent %#v, want nothing", got)
	}
	if out := stripANSI(m.View()); !strings.Contains(out, onList) {
		t.Errorf("the popup is not on the frame:\n%s", out)
	}
}

// The answer lands while the box has the keyboard, always: the box is what
// asked. SetRepo refuses to open a picker in that state on purpose, and the
// popup has to be handed the list ahead of that refusal.
func TestTheMentionListLandsWhileTheBoxHasTheKeyboard(t *testing.T) {
	m, _ := typing(composing(200, 60), "@")
	if out := stripANSI(m.View()); strings.Contains(out, onList) {
		t.Fatalf("setup: the popup already has people before the fetch answered:\n%s", out)
	}

	m.SetRepo(loadedRepo())

	if out := stripANSI(m.View()); !strings.Contains(out, onList) {
		t.Errorf("the list that landed mid-word never reached the popup:\n%s", out)
	}
}

// The twin. Handing the popup its list must not weaken the guard that keeps a
// late modal off a box somebody is typing in.
func TestALatePickerStillDoesNotOpenOverTheBox(t *testing.T) {
	m := onRailRow(t, detailed(held(sampleDetail()), 200, 60), "bug")
	if _, cmd := key(m, "enter"); asking(cmd) == nil {
		t.Fatal("setup: enter on the label row asked for nothing")
	}
	m, _ = key(m, "enter")

	m = press(m, "1", "c")
	m.SetRepo(loadedRepo())

	// The picker's own hint line, which is the one thing on the frame only a
	// modal puts there: the rail carries an "Add label" row whether or not one
	// is up.
	out := stripANSI(m.View())
	if strings.Contains(out, "space toggle") {
		t.Errorf("a picker opened over the box:\n%s", out)
	}
	if !strings.Contains(out, "Leave a comment") {
		t.Errorf("the box is not on the frame:\n%s", out)
	}
}

func TestTheMentionListSaysItIsStillComing(t *testing.T) {
	m := composing(200, 60)
	m.SetRepo(store.Repo{Status: store.StatusLoading})
	m, _ = typing(m, "@")

	out := stripANSI(m.View())
	if !strings.Contains(out, "Loading people") {
		t.Errorf("the popup does not say the list is on its way:\n%s", out)
	}
	if strings.Contains(out, "No match") || strings.Contains(out, "Nobody to mention") {
		t.Errorf("the popup reads as an answered fetch:\n%s", out)
	}
}

// A silent empty list looks like a broken key. The toast is over the pane and
// gone in seconds; the popup is under the caret.
func TestTheMentionListSaysWhenItWillNotCome(t *testing.T) {
	m := composing(200, 60)
	m.SetRepo(store.Repo{Status: store.StatusFailed, Err: errors.New("boom")})
	m, _ = typing(m, "@")

	out := stripANSI(m.View())
	if !strings.Contains(out, "Could not read the repository") {
		t.Errorf("the popup does not say the list will not come:\n%s", out)
	}
	if strings.Contains(out, "Loading people") {
		t.Errorf("a failed fetch still reads as one on its way:\n%s", out)
	}
}

func TestAFilterThatMatchesNobodySaysSo(t *testing.T) {
	m, _ := typing(writing(t), "@zzz")

	if out := stripANSI(m.View()); !strings.Contains(out, "No match") {
		t.Errorf("a token nobody matches shows nothing at all:\n%s", out)
	}
}

// The sync key drops the held choices. The popup goes back to saying the list
// is coming, and the latch has to come off or nothing asks again.
func TestClearingTheRepositoryPutsTheMentionListBackOnItsWay(t *testing.T) {
	m, _ := typing(writing(t), "@")
	cmd := m.SetRepo(store.Repo{})

	want := prview.NeedRepoMetaMsg{Repo: "zen-octo/zen-octo"}
	if got := asking(cmd); got != want {
		t.Errorf("clearing the repository sent %#v, want %#v", got, want)
	}
	if out := stripANSI(m.View()); strings.Contains(out, "Nobody to mention") {
		t.Errorf("a dropped list reads as a repository with nobody on it:\n%s", out)
	}
}

func TestTheMentionListFiltersAsYouType(t *testing.T) {
	m, _ := typing(writing(t), "@nk")

	out := stripANSI(m.View())
	if !strings.Contains(out, onList) {
		t.Errorf("the match is not on the frame:\n%s", out)
	}
	if strings.Contains(out, offList) {
		t.Errorf("a login the token does not match is still offered:\n%s", out)
	}
}

func TestTheMentionRowsCarryTheRealName(t *testing.T) {
	m, _ := typing(writing(t), "@nk")

	if out := stripANSI(m.View()); !strings.Contains(out, onList) {
		t.Errorf("the row does not name who the handle belongs to:\n%s", out)
	}
}

// An address is not a mention. The @ has to open a word, or every email in
// every comment drops a list of logins over the line being written.
func TestAnAtInsideAWordOpensNothing(t *testing.T) {
	m, _ := typing(writing(t), "mail me at drew@example.com")

	if out := stripANSI(m.View()); strings.Contains(out, onList) {
		t.Errorf("an email address opened the popup:\n%s", out)
	}
}

func TestEnterWritesTheHandleIntoTheBox(t *testing.T) {
	m, _ := typing(writing(t), "thanks @nk")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	out := stripANSI(m.View())
	if !strings.Contains(out, "thanks @nkr") {
		t.Errorf("enter did not write the handle:\n%s", out)
	}
	if strings.Contains(out, onList) {
		t.Errorf("the popup is still up after the handle landed:\n%s", out)
	}
}

// tab inserts too, which is what the box's own tab does one level out: it moves
// to the thing that finishes what is being written.
func TestTabWritesTheHandleRatherThanSteppingToTheButton(t *testing.T) {
	m, _ := typing(writing(t), "thanks @nk")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	if out := stripANSI(m.View()); !strings.Contains(out, "thanks @nkr") {
		t.Errorf("tab did not write the handle:\n%s", out)
	}
}

// A handle run together with the next word is a mention GitHub does not
// resolve, so the space is part of what the key writes.
func TestTheHandleLandsWithASpaceAfterIt(t *testing.T) {
	m, _ := typing(writing(t), "@nk")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = typing(m, "please")

	if out := stripANSI(m.View()); !strings.Contains(out, "@nkr please") {
		t.Errorf("the handle and the next word ran together:\n%s", out)
	}
}

// The caret has to come back to where the handle ends. SetValue leaves it at the
// end of whatever it inserted, and a buffer rebuilt front to back would put
// every further keystroke at the end of the comment.
func TestTheCaretStaysWhereTheHandleEnds(t *testing.T) {
	m, _ := typing(writing(t), "@nk")
	m, _ = typing(m, " and thanks")

	// Back over " and thanks" so the caret sits mid-buffer, then complete.
	m, _ = typing(m, " @dru")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = typing(m, "!")

	if out := stripANSI(m.View()); !strings.Contains(out, "@drucial !") {
		t.Errorf("the caret did not come back to the handle:\n%s", out)
	}
}

func TestTheArrowsWalkTheListAndTheLettersDoNot(t *testing.T) {
	m, _ := typing(writing(t), "@")
	m, _ = arrow(m, tea.KeyDown)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	out := stripANSI(m.View())
	if !strings.Contains(out, "@nkr") {
		t.Errorf("down did not move the cursor onto the second row:\n%s", out)
	}

	// j is a letter in a box. Pressed with the list up it types rather than
	// walking, which is what leaves the token as @j.
	n, _ := typing(writing(t), "@")
	n, _ = typing(n, "j")
	if out := stripANSI(n.View()); !strings.Contains(out, "@j") {
		t.Errorf("j walked the list instead of being typed:\n%s", out)
	}
}

// esc closes the popup and nothing else. Leaked through it closes the box, and
// on an edit it throws the draft away.
func TestEscapeClosesTheListAndLeavesTheBoxOpen(t *testing.T) {
	m, _ := typing(writing(t), "@nk")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

	out := stripANSI(m.View())
	if strings.Contains(out, onList) {
		t.Errorf("esc left the popup up:\n%s", out)
	}
	if !strings.Contains(out, "@nk") {
		t.Errorf("esc took the words with it:\n%s", out)
	}
	// And the next keystroke must not reopen it over the same token, or there is
	// no way to finish a word that begins with an at sign.
	m, _ = typing(m, "r")
	if out := stripANSI(m.View()); strings.Contains(out, onList) {
		t.Errorf("the next keystroke reopened a dismissed popup:\n%s", out)
	}
}

// A space ends the word, so it ends the list. Nothing after it is a handle.
func TestASpaceClosesTheList(t *testing.T) {
	m, _ := typing(writing(t), "@nk ")

	if out := stripANSI(m.View()); strings.Contains(out, onList) {
		t.Errorf("the popup outlived the word it was answering:\n%s", out)
	}
}

// The popup is an overlay over a frame the pane already filled. It must not
// grow it.
func TestTheFrameStillFillsItsSizeWithTheListUp(t *testing.T) {
	for _, size := range []struct{ w, h int }{{200, 60}, {120, 40}, {80, 24}} {
		m := composing(size.w, size.h)
		m.SetRepo(loadedRepo())
		m, _ = typing(m, "@")

		lines := strings.Split(m.View(), "\n")
		if len(lines) != size.h {
			t.Errorf("%dx%d: frame is %d lines, want %d", size.w, size.h, len(lines), size.h)
		}
		for i, line := range lines {
			if w := lipgloss.Width(line); w != size.w {
				t.Errorf("%dx%d: line %d is %d cells, want %d", size.w, size.h, i, w, size.w)
			}
		}
	}
}

// The list goes above the line being typed when there is no room under it. A
// caret on the last row of a full box has the pane's foot directly beneath it,
// and a list drawn there would be off the screen entirely.
func TestTheListGoesAboveTheCaretWhenThereIsNoRoomBelow(t *testing.T) {
	m := composing(120, 24)
	m.SetRepo(loadedRepo())

	// Enter is a newline while no list is up, which is how the caret reaches the
	// foot of a box that has grown to fill the pane.
	for range 12 {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	}
	m, _ = typing(m, "@")

	lines := strings.Split(stripANSI(m.View()), "\n")
	list, foot := -1, -1
	for i, l := range lines {
		if strings.Contains(l, onList) {
			list = i
		}
		if strings.Contains(l, "esc done") {
			foot = i
		}
	}

	if list < 0 || foot < 0 {
		t.Fatalf("list at %d and the box foot at %d, want both on the frame:\n%s",
			list, foot, strings.Join(lines, "\n"))
	}
	if list > foot {
		t.Errorf("the list is at line %d and the box foot at %d, want it above:\n%s",
			list, foot, strings.Join(lines, "\n"))
	}
}

// shift+tab leaves the list for the button, so the list has to go with the
// press. Held open over a blurred box, the enter that follows wrote a handle
// instead of posting, and on a terminal that cannot send the chord that button
// is the only way a comment is sent at all.
func TestShiftTabLeavesTheListAndStepsToTheButton(t *testing.T) {
	// The token has to match the row this asserts on. Written against a token
	// that matched somebody else, the absence it checks for was never there to
	// begin with and the test passed over a list that stayed open.
	m, _ := typing(writing(t), "thanks @n")
	if out := stripANSI(m.View()); !strings.Contains(out, onList) {
		t.Fatalf("setup: the list is not up:\n%s", out)
	}

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if out := stripANSI(m.View()); strings.Contains(out, onList) {
		t.Errorf("shift+tab left the list up:\n%s", out)
	}

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	msg, ok := runCmd(cmd).(prview.PostCommentMsg)
	if !ok {
		t.Fatalf("enter on the button sent %#v, want a comment", runCmd(cmd))
	}
	if want := "thanks @n"; msg.Body != want {
		t.Errorf("posted %q, want %q", msg.Body, want)
	}
}

// A caret put back inside a handle means the handle is being corrected, so the
// whole word goes. Replacing only what is in front of the caret turned @nikita
// into "@nkr kita".
func TestCompletingInsideAHandleReplacesTheWholeWord(t *testing.T) {
	m, _ := typing(writing(t), "hi @nikita")
	for range 4 {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	out := stripANSI(m.View())
	if strings.Contains(out, "@nkr kita") {
		t.Errorf("the tail of the old handle survived:\n%s", out)
	}
	if !strings.Contains(out, "hi @nkr") {
		t.Errorf("the handle was not written:\n%s", out)
	}
}

// A popup with nothing to insert must not eat the key. It closes and the press
// carries on, or the reader presses enter for a newline and gets neither.
func TestEnterWithNothingToChooseStillReachesTheBox(t *testing.T) {
	m, _ := typing(writing(t), "@zzz")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	out := stripANSI(m.View())
	if strings.Contains(out, "No match") {
		t.Errorf("the popup outlived the key:\n%s", out)
	}
	// The newline landed, so the next words go on a line of their own.
	m, _ = typing(m, "hello")
	if out := stripANSI(m.View()); strings.Contains(out, "@zzzhello") {
		t.Errorf("enter was swallowed rather than reaching the box:\n%s", out)
	}
}

// The editor replaces the whole buffer, so a list still open over it holds an
// offset into text that is gone.
func TestHandingOffToTheEditorClosesTheList(t *testing.T) {
	m, _ := typing(writing(t), "@nk")
	if out := stripANSI(m.View()); !strings.Contains(out, onList) {
		t.Fatalf("setup: the list is not up:\n%s", out)
	}

	m, _ = m.Update(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	if out := stripANSI(m.View()); strings.Contains(out, onList) {
		t.Errorf("the list survived the handoff to the editor:\n%s", out)
	}
}

// blink stands in for the messages that are not keys: a cursor blink, a paste,
// a clipboard read. They reach the box through Update's default branch, which
// re-reads the token, and that is a path no keystroke test walks.
type blink struct{}

// Stepping to the button took the list down and the very next blink put it back
// up, because the token was still sitting under the caret. Every test here drove
// keys alone and none of them saw it.
func TestTheListStaysDownOnceTheButtonHasFocus(t *testing.T) {
	m, _ := typing(writing(t), "thanks @n")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if out := stripANSI(m.View()); strings.Contains(out, onList) {
		t.Fatalf("setup: shift+tab left the list up:\n%s", out)
	}

	for range 3 {
		m, _ = m.Update(blink{})
	}
	if out := stripANSI(m.View()); strings.Contains(out, onList) {
		t.Errorf("the list came back while the button holds focus:\n%s", out)
	}
}

// Stepping back into the text is the reader returning to the word, so the list
// returns with them.
func TestTheListComesBackWhenTheTextTakesFocusAgain(t *testing.T) {
	m, _ := typing(writing(t), "thanks @n")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m, _ = m.Update(blink{})

	if out := stripANSI(m.View()); !strings.Contains(out, onList) {
		t.Errorf("the list did not come back with the caret:\n%s", out)
	}
}

// A list escaped away must not return by stepping to the button and back. The
// dismissal outlives the focus change.
func TestAnEscapedListDoesNotComeBackWithTheFocus(t *testing.T) {
	m, _ := typing(writing(t), "thanks @n")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m, _ = m.Update(blink{})

	if out := stripANSI(m.View()); strings.Contains(out, onList) {
		t.Errorf("a dismissed list came back through the button:\n%s", out)
	}
}
