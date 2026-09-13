package prview_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

func typing(m prview.Model, text string) (prview.Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, r := range text {
		m, cmd = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m, cmd
}

func writing(t *testing.T) prview.Model {
	t.Helper()

	m := composing(200, 60)
	m.SetRepo(loadedRepo())
	return m
}

func arrow(m prview.Model, code rune) (prview.Model, tea.Cmd) {
	return m.Update(tea.KeyPressMsg{Code: code})
}

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

// Real names, because the rail renders the handles whatever the popup is doing.
const (
	onList  = "Nikita Rushmanov"
	offList = "Sam Reed"
)

func TestTheFirstAtInABoxAsksForTheRepositorysPeople(t *testing.T) {
	_, cmd := typing(composing(200, 60), "@")

	want := prview.NeedRepoMetaMsg{Repo: "acme/rocket"}
	if got := asking(cmd); got != want {
		t.Fatalf("typing @ sent %#v, want %#v", got, want)
	}
}

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

func TestAnAtAsksForNothingOnceThePickersHaveFetched(t *testing.T) {
	m, cmd := typing(writing(t), "@")
	if got := asking(cmd); got != nil {
		t.Errorf("typing @ over a held repository sent %#v, want nothing", got)
	}
	if out := stripANSI(m.View()); !strings.Contains(out, onList) {
		t.Errorf("the popup is not on the frame:\n%s", out)
	}
}

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

func TestTheMentionListStandsUnderTheWordItAnswers(t *testing.T) {
	for _, width := range []int{200, 100} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			anchorsUnderTheWord(t, width)
		})
	}
}

func anchorsUnderTheWord(t *testing.T, width int) {
	t.Helper()

	m, _ := typing(composing(width, 60), "@zq")
	m.SetRepo(loadedRepo())

	lines := strings.Split(stripANSI(m.View()), "\n")

	at, caretRow := -1, -1
	for i, line := range lines {
		if c := strings.Index(line, "@zq"); c >= 0 {
			at, caretRow = c, i
			break
		}
	}
	if at < 0 {
		t.Fatalf("setup: nothing typed on the frame:\n%s", strings.Join(lines, "\n"))
	}

	top, left := -1, -1
	for i := caretRow + 1; i < len(lines); i++ {
		if c := strings.Index(lines[i], "╭"); c >= 0 {
			top, left = i, c
			break
		}
	}
	if top < 0 {
		t.Fatalf("no popup under the caret:\n%s", strings.Join(lines, "\n"))
	}

	if left != at {
		t.Errorf("the popup opens at column %d and the word is at %d", left, at)
	}
	if top != caretRow+1 {
		t.Errorf("the popup opens on row %d and the caret is on %d", top, caretRow)
	}
}

func TestALatePickerStillDoesNotOpenOverTheBox(t *testing.T) {
	m := onRailRow(t, detailed(held(sampleDetail()), 200, 60), "bug")
	if _, cmd := key(m, "enter"); asking(cmd) == nil {
		t.Fatal("setup: enter on the label row asked for nothing")
	}
	m, _ = key(m, "enter")

	m = press(m, "1", "c")
	m.SetRepo(loadedRepo())

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

func TestClearingTheRepositoryPutsTheMentionListBackOnItsWay(t *testing.T) {
	m, _ := typing(writing(t), "@")
	cmd := m.SetRepo(store.Repo{})

	want := prview.NeedRepoMetaMsg{Repo: "acme/rocket"}
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

func TestTabWritesTheHandleRatherThanSteppingToTheButton(t *testing.T) {
	m, _ := typing(writing(t), "thanks @nk")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	if out := stripANSI(m.View()); !strings.Contains(out, "thanks @nkr") {
		t.Errorf("tab did not write the handle:\n%s", out)
	}
}

func TestTheHandleLandsWithASpaceAfterIt(t *testing.T) {
	m, _ := typing(writing(t), "@nk")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = typing(m, "please")

	if out := stripANSI(m.View()); !strings.Contains(out, "@nkr please") {
		t.Errorf("the handle and the next word ran together:\n%s", out)
	}
}

func TestTheCaretStaysWhereTheHandleEnds(t *testing.T) {
	m, _ := typing(writing(t), "@nk")
	m, _ = typing(m, " and thanks")

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

	n, _ := typing(writing(t), "@")
	n, _ = typing(n, "j")
	if out := stripANSI(n.View()); !strings.Contains(out, "@j") {
		t.Errorf("j walked the list instead of being typed:\n%s", out)
	}
}

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
	m, _ = typing(m, "r")
	if out := stripANSI(m.View()); strings.Contains(out, onList) {
		t.Errorf("the next keystroke reopened a dismissed popup:\n%s", out)
	}
}

func TestASpaceClosesTheList(t *testing.T) {
	m, _ := typing(writing(t), "@nk ")

	if out := stripANSI(m.View()); strings.Contains(out, onList) {
		t.Errorf("the popup outlived the word it was answering:\n%s", out)
	}
}

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

func TestTheListGoesAboveTheCaretWhenThereIsNoRoomBelow(t *testing.T) {
	m := composing(120, 24)
	m.SetRepo(loadedRepo())

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

func TestShiftTabLeavesTheListAndStepsToTheButton(t *testing.T) {
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

func TestEnterWithNothingToChooseStillReachesTheBox(t *testing.T) {
	m, _ := typing(writing(t), "@zzz")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	out := stripANSI(m.View())
	if strings.Contains(out, "No match") {
		t.Errorf("the popup outlived the key:\n%s", out)
	}
	m, _ = typing(m, "hello")
	if out := stripANSI(m.View()); strings.Contains(out, "@zzzhello") {
		t.Errorf("enter was swallowed rather than reaching the box:\n%s", out)
	}
}

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

// Stands in for non-key messages, which reach the box through Update's default branch.
type blink struct{}

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

func TestTheListComesBackWhenTheTextTakesFocusAgain(t *testing.T) {
	m, _ := typing(writing(t), "thanks @n")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m, _ = m.Update(blink{})

	if out := stripANSI(m.View()); !strings.Contains(out, onList) {
		t.Errorf("the list did not come back with the caret:\n%s", out)
	}
}

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
