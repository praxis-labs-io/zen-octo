package prview_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

func typed(m prview.Model, text string) prview.Model {
	for _, r := range text {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

func pressed(m prview.Model, keys ...string) (prview.Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, k := range keys {
		m, cmd = m.Update(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
	}
	return m, cmd
}

func runCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func chord(m prview.Model) (prview.Model, tea.Cmd) {
	return m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
}

func composing(width, height int) prview.Model {
	return press(detailed(held(sampleDetail()), width, height), "c")
}

func TestTheCommentBoxIsAlwaysOnThePage(t *testing.T) {
	out := stripANSI(press(detailed(held(sampleDetail()), 200, 60), "G").View())

	if !strings.Contains(out, "Leave a comment") {
		t.Errorf("no comment box in the conversation:\n%s", out)
	}
	if !strings.Contains(out, "write a comment") {
		t.Error("the box has no heading saying what it is for")
	}
	if !strings.Contains(out, "c to write") {
		t.Error("the box does not say how to start writing in it")
	}
}

func TestTheCommentBoxRendersAsACard(t *testing.T) {
	frame := press(detailed(held(sampleDetail()), 200, 60), "G").View()
	lines := strings.Split(stripANSI(frame), "\n")

	column := func(want string) int {
		for i, line := range lines {
			if strings.Contains(line, want) && i > 0 {
				return strings.Index(lines[i-1], "╭")
			}
		}
		return -1
	}

	box, comment := column("write a comment"), column("internal/tui/keys/keys.go:7")
	if box < 0 || comment < 0 {
		t.Fatalf("could not find both cards: box %d, comment %d", box, comment)
	}
	if box != comment {
		t.Errorf("the box opens at column %d and a comment card at %d", box, comment)
	}
}

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

func TestTheComposerTakesTheKeysTheScreenWouldHaveAnswered(t *testing.T) {
	m := typed(composing(200, 40), "jkdorq")

	if got := stripANSI(m.View()); !strings.Contains(got, "jkdorq") {
		t.Errorf("the screen answered keys meant for the text:\n%s", got)
	}
}

func TestEscapeKeepsTheWordsAndLeavesTheBox(t *testing.T) {
	m := press(typed(composing(200, 60), "half written"), "esc")

	out := stripANSI(m.View())
	if !strings.Contains(out, "half written") {
		t.Error("esc took the words away")
	}
	if !strings.Contains(out, "⏎ to write") {
		t.Error("the box does not say the keyboard went back to the screen")
	}

	if got := stripANSI(press(m, "j").View()); strings.Contains(got, "half writtenj") {
		t.Error("the box is still taking letters after esc")
	}
}

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

func lit(frame string) bool {
	return strings.Contains(frame, bgSeq(testTheme.Accent)+"mPost")
}

func TestThePostButtonIsAFilledSurfaceAtEveryState(t *testing.T) {
	empty := composing(200, 40)
	written := typed(empty, "ship it")

	states := map[string]string{
		"with nothing written": empty.View(),
		"ready to send":        written.View(),
		"holding focus":        press(written, "tab").View(),
	}
	for name, frame := range states {
		if !strings.Contains(frame, bgSeq(testTheme.SelectedBackground)+"mPost") &&
			!lit(frame) {
			t.Errorf("the button has no background %s", name)
		}
	}
}

func TestThePostButtonSitsInTheBottomRight(t *testing.T) {
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

	if !strings.Contains(lines[at+1], "╰") {
		t.Errorf("the button is not on the pane's last row: %q", lines[at+1])
	}

	tail := strings.TrimLeft(lines[at][strings.Index(lines[at], "Post")+len("Post"):], " ")
	if !strings.HasPrefix(tail, "│") {
		t.Errorf("the button is followed by %q, want the card's border next", tail)
	}
}

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

func TestTheFooterNamesTheChordOnlyWhereItWorks(t *testing.T) {
	plain := stripANSI(composing(200, 40).View())
	if strings.Contains(plain, "ctrl+⏎") {
		t.Errorf("the footer names ctrl+enter on a terminal that cannot send it:\n%s", plain)
	}
	if !strings.Contains(plain, "tab · ⏎ post") {
		t.Errorf("the footer does not name the button path:\n%s", plain)
	}

	m := detailed(held(sampleDetail()), 200, 40)
	m.SetChords(true)
	if got := stripANSI(press(m, "c").View()); !strings.Contains(got, "ctrl+⏎ post") {
		t.Errorf("the footer does not name the chord where it works:\n%s", got)
	}
}

func TestARestoredDraftReopensTheComposerWithTheWords(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 40)
	m.RestoreDraft("this did not send")

	if got := stripANSI(m.View()); !strings.Contains(got, "this did not send") {
		t.Errorf("the draft did not come back:\n%s", got)
	}
}

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

// Asserts only that a command comes back: the round trip needs a real editor and terminal.
func TestCtrlEHandsTheBufferOff(t *testing.T) {
	_, cmd := pressed(typed(composing(200, 40), "draft"), "ctrl+e")
	if cmd == nil {
		t.Error("ctrl+e did nothing")
	}
}

func TestWritingACommentDoesNotMoveTheLayout(t *testing.T) {
	resting := detailed(held(sampleDetail()), 200, 40)
	writing := typed(press(resting, "c"), "ship it")

	if before, after := railRows(t, resting.View()), railRows(t, writing.View()); len(before) != len(after) {
		t.Errorf("the rail is %d rows while a comment is being written and %d otherwise", len(after), len(before))
	}

	top := func(m prview.Model) string {
		return strings.Split(stripANSI(m.View()), "\n")[0]
	}
	if top(resting) != top(writing) {
		t.Errorf("the panes moved:\n%s\nwant\n%s", top(writing), top(resting))
	}
}

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

func TestTheBoxStaysOnScreenWhileItIsWrittenIn(t *testing.T) {
	d := sampleDetail()
	d.Body = strings.Repeat("The retry path backs off forever.\n\n", 25)

	m := typed(press(detailed(held(d), 100, 30), "c"), "still here")

	if got := stripANSI(m.View()); !strings.Contains(got, "still here") {
		t.Errorf("the box scrolled away while it was being written in:\n%s", got)
	}
}

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

func TestTakingTheBoxUnlightsTheCardThatHadFocus(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 60)
	if got := focusedCard(t, m.View()); !strings.HasPrefix(got, cardDescription) {
		t.Fatalf("the ring focused %q, want the description", got)
	}

	if got := focusedCard(t, press(m, "c").View()); !strings.HasPrefix(got, cardCompose) {
		t.Errorf("c left the accent on %q, want it on the box", got)
	}
}

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

func TestTheHintNamesTheKeyThatWorksFromHere(t *testing.T) {
	away := detailed(held(sampleDetail()), 200, 60)
	if got := stripANSI(press(away, "G").View()); !strings.Contains(got, "c to write") {
		t.Errorf("with focus elsewhere the box names the wrong key:\n%s", got)
	}

	onIt := press(away, strings.Fields(strings.Repeat("} ", 9))...)
	out := stripANSI(onIt.View())
	if !strings.Contains(out, "⏎ to write") {
		t.Errorf("with the ring on the box it does not name enter:\n%s", out)
	}
	if strings.Contains(out, "c to write") {
		t.Error("the box still names c once the ring is on it")
	}
}

func TestAPastedCommentReachesTheBox(t *testing.T) {
	m := typed(composing(200, 60), "before ")

	m, _ = m.Update(tea.PasteMsg{Content: "pasted words"})

	if got := stripANSI(m.View()); !strings.Contains(got, "before pasted words") {
		t.Errorf("the paste never reached the box:\n%s", got)
	}
}

func TestAPasteWhileReadingIsIgnored(t *testing.T) {
	m, _ := detailed(held(sampleDetail()), 200, 60).Update(tea.PasteMsg{Content: "pasted words"})

	if got := stripANSI(m.View()); strings.Contains(got, "pasted words") {
		t.Errorf("a paste landed in the box with the keyboard elsewhere:\n%s", got)
	}
}

func TestTypingNeverStartsWhereThereIsNoBox(t *testing.T) {
	t.Run("enter on a tab with a column", func(t *testing.T) {
		m := press(detailed(held(sampleDetail()), 200, 60), "c", "esc", "]", "enter")

		if m.Composing() {
			t.Error("enter started the composer on the Commits tab")
		}
		if got := stripANSI(press(m, "q").View()); strings.Contains(got, "q") && m.Composing() {
			t.Error("keys are being swallowed by a box that is not on screen")
		}
	})

	t.Run("c on a conversation that never loaded", func(t *testing.T) {
		failed := store.Detail{Status: store.StatusFailed, Err: errors.New("network is down")}
		m := press(detailed(failed, 200, 60), "c")

		if m.Composing() {
			t.Error("c started the composer on a conversation that failed to load")
		}
	})
}

func TestARestoredDraftDoesNotCaptureAnUnrelatedTab(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 200, 60), "]")
	m.RestoreDraft("this did not send")

	if m.Composing() {
		t.Error("the restore captured the keyboard on a tab drawing no box")
	}

	if got := stripANSI(press(m, "[", "c").View()); !strings.Contains(got, "this did not send") {
		t.Errorf("the words did not survive the trip back:\n%s", got)
	}
}

func TestARestoredDraftDoesNotDestroyOneWrittenSince(t *testing.T) {
	m := typed(composing(200, 60), "the next comment")
	m.RestoreDraft("the one that failed")

	out := stripANSI(m.View())
	for _, want := range []string{"the one that failed", "the next comment"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q is gone:\n%s", want, out)
		}
	}
}

func TestWritingFromTheRailMovesTheKeysToTheConversation(t *testing.T) {
	rail := press(detailed(held(sampleDetail()), 200, 60), "h", "}")
	if markedRailRow(t, rail.View()) == "" {
		t.Fatal("the rail has no cursor line to begin with")
	}

	m := press(rail, "c")
	if !m.Composing() {
		t.Fatal("c did not start the composer from the rail")
	}
	if got := markedRailRow(t, m.View()); got != "" {
		t.Errorf("the rail still paints %q as the cursor line while the box takes the keys", got)
	}
}

func TestThePostButtonSurvivesANarrowCard(t *testing.T) {
	for _, width := range []int{80, 70, 60, 50, 44} {
		m := typed(press(detailed(held(sampleDetail()), width, 40), "c"), "ship it")
		if got := stripANSI(m.View()); !strings.Contains(got, "Post") {
			t.Errorf("at %d columns the button is gone:\n%s", width, got)
		}
	}
}

func TestTheLoadFailureSaysWhyAtEveryWidth(t *testing.T) {
	err := errors.New("Post \"https://api.github.com/graphql\": dial tcp 140.82.121.6:443: connect: operation timed out")
	failed := store.Detail{Status: store.StatusFailed, Err: err}

	for _, width := range []int{200, 120, 90, 70} {
		out := stripANSI(detailed(failed, width, 30).View())
		if !strings.Contains(out, "operation timed out") {
			t.Errorf("at %d columns the reason is clipped away:\n%s", width, out)
		}
	}
}
