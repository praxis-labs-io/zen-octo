package prview_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

const (
	tabDescription = 1
	tabComment     = 2
	tabReview      = 3
)

// A review's body and its comment both report viewerCanDelete, and only the comment can be deleted.
func writable() gh.PullRequestDetail {
	d := sampleDetail()

	for _, item := range d.Timeline {
		switch item.Kind {
		case gh.TimelineComment:
			mine(item.Comment)
			item.Comment.Body += "\n\nThe cap holds through a retry storm.\n\nRunbook updated."
		case gh.TimelineReview:
			mine(item.Comment)
		}
	}

	for i := range d.Threads[0].Comments {
		mine(&d.Threads[0].Comments[i])
	}
	return d
}

func mine(c *gh.Comment) {
	c.ViewerDidAuthor, c.CanEdit, c.CanDelete = true, true, true
}

func editing(n int) prview.Model {
	return press(onWritable(n), "e")
}

func onWritable(n int) prview.Model {
	return walked(viewing(writable(), 200, 60), n)
}

func viewing(d gh.PullRequestDetail, width, height int) prview.Model {
	m := detailed(held(d), width, height)
	m.SetViewer(gh.Actor{Login: "drucial"})
	return m
}

func TestTheEditBoxOpensInPlaceOfTheWordsItReplaces(t *testing.T) {
	out := stripANSI(editing(tabComment).View())

	head := strings.Index(out, "edit this comment")
	box := strings.Index(out, "Save")
	foot := strings.Index(out, "write a comment")

	switch {
	case box < 0:
		t.Fatalf("e opened no box:\n%s", out)
	case head < 0 || box < head:
		t.Error("the box is above the heading of the card it is inside")
	case foot >= 0 && box > foot:
		t.Error("the box is at the foot of the page rather than in the card")
	}

	if !strings.Contains(out, "edit this comment") {
		t.Errorf("the card does not say it is being edited:\n%s", out)
	}
	if !strings.Contains(out, "Coverage held at 84.2%") {
		t.Errorf("the box did not open on the comment's own words:\n%s", out)
	}
}

func TestTheEditBoxIsTheHeightOfTheWordsItReplaces(t *testing.T) {
	m := onWritable(tabComment)
	below := "nkr · requested changes"

	before := lineOf(t, m.View(), below)
	after := lineOf(t, press(m, "e").View(), below)

	if got := after - before; got != 1 {
		t.Errorf("opening the box moved the card below it by %d lines, want 1 for the button", got)
	}
}

func TestTheEditBoxGrowsWithWhatIsTypedIntoIt(t *testing.T) {
	m := press(onWritable(tabComment), "e")
	below := "nkr · requested changes"

	before := lineOf(t, m.View(), below)
	after := lineOf(t, typed(m, "\n\n").View(), below)

	if got := after - before; got != 2 {
		t.Errorf("two new lines moved the card below by %d lines, want 2", got)
	}
}

func TestOpeningTheEditBoxKeepsAHalfWrittenComment(t *testing.T) {
	m := typed(press(viewing(writable(), 200, 60), "c"), "half a thought")
	m = press(m, "esc")

	m = press(walked(fromTop(m), tabComment), "e")
	if !strings.Contains(stripANSI(m.View()), "Coverage held at 84.2%") {
		t.Fatal("the edit box did not open on the comment's own words")
	}

	out := stripANSI(press(m, "esc", "G").View())
	if !strings.Contains(out, "half a thought") {
		t.Errorf("opening the edit box took the comment being written:\n%s", out)
	}
}

func TestABoxNeverGrowsPastThePane(t *testing.T) {
	d := writable()
	for _, item := range d.Timeline {
		if item.Kind == gh.TimelineComment {
			item.Comment.Body = strings.Repeat("A line of it.\n\n", 40)
		}
	}

	m := press(walked(viewing(d, 200, 24), tabComment), "e")

	out := stripANSI(m.View())
	if !strings.Contains(out, "edit this comment") {
		t.Errorf("the card's heading is off the screen:\n%s", out)
	}
	if !strings.Contains(out, "Save") {
		t.Errorf("the card's foot is off the screen:\n%s", out)
	}
}

func TestTheCaretStaysOnTheScreenInALongComment(t *testing.T) {
	d := writable()
	for _, item := range d.Timeline {
		if item.Kind == gh.TimelineComment {
			item.Comment.Body = strings.Repeat("A line of it.\n\n", 40)
		}
	}

	m := press(walked(viewing(d, 200, 24), tabComment), "e")
	m = typed(m, "the caret is here")

	if out := stripANSI(m.View()); !strings.Contains(out, "the caret is here") {
		t.Errorf("the line being written is off the screen:\n%s", out)
	}
}

func TestTheCommentBoxGrowsAndKeepsItsCaretOnTheScreen(t *testing.T) {
	m := press(viewing(writable(), 200, 24), "c")
	m = typed(m, strings.Repeat("A line of it.\n", 30)+"the caret is here")

	if out := stripANSI(m.View()); !strings.Contains(out, "the caret is here") {
		t.Errorf("the line being written is off the screen:\n%s", out)
	}
}

func TestEscapeClosesTheEditBoxAndLeavesTheComment(t *testing.T) {
	m := onWritable(tabComment)
	before := m.View()

	if after := press(press(m, "e"), "esc").View(); after != before {
		t.Errorf("esc left the card changed:\n%s", stripANSI(after))
	}
}

func TestTheEditKeySendsTheNewWordsForTheFocusedComment(t *testing.T) {
	m := editing(tabComment)
	m = typed(m, " Updated.")

	_, cmd := chord(m)
	msg, ok := runCmd(cmd).(prview.EditCommentMsg)
	if !ok {
		t.Fatalf("the chord asked for %T, want an EditCommentMsg", runCmd(cmd))
	}

	if msg.CommentID != "IC_octobot" {
		t.Errorf("CommentID = %q, want the focused comment", msg.CommentID)
	}
	if msg.Kind != gh.CommentIssue {
		t.Errorf("Kind = %q, want an issue comment", msg.Kind)
	}
	if msg.ThreadID != "" {
		t.Errorf("ThreadID = %q, want none on a top-level comment", msg.ThreadID)
	}
	if !strings.HasSuffix(msg.Body, "Updated.") {
		t.Errorf("Body = %q, want what the box was left holding", msg.Body)
	}
}

func TestEditingAReviewSendsTheReviewKind(t *testing.T) {
	m := typed(editing(tabReview), "!")

	_, cmd := chord(m)
	msg, ok := runCmd(cmd).(prview.EditCommentMsg)
	if !ok {
		t.Fatalf("the chord asked for %T, want an EditCommentMsg", runCmd(cmd))
	}
	if msg.Kind != gh.CommentReview {
		t.Errorf("Kind = %q, want a review", msg.Kind)
	}
	if msg.CommentID != "REV_1" {
		t.Errorf("CommentID = %q, want the review", msg.CommentID)
	}
}

func TestEditingAThreadTakesTheCommentTheSubCursorIsOn(t *testing.T) {
	m := typed(editing(tabThread), "!")

	_, cmd := chord(m)
	msg, ok := runCmd(cmd).(prview.EditCommentMsg)
	if !ok {
		t.Fatalf("the chord asked for %T, want an EditCommentMsg", runCmd(cmd))
	}

	if msg.CommentID != "RC_1" {
		t.Errorf("CommentID = %q, want the comment that opened the thread", msg.CommentID)
	}
	if msg.ThreadID != "RT_1" {
		t.Errorf("ThreadID = %q, want the thread it sits in", msg.ThreadID)
	}
	if msg.Kind != gh.CommentThread {
		t.Errorf("Kind = %q, want a review comment", msg.Kind)
	}

	_, cmd = chord(typed(press(onWritable(tabThread), "}", "e"), "!"))
	if msg, _ := runCmd(cmd).(prview.EditCommentMsg); msg.CommentID != "RC_4" {
		t.Errorf("CommentID = %q on the reply, want the reply itself", msg.CommentID)
	}
}

func TestEditingTheDescriptionSendsAPullRequestWrite(t *testing.T) {
	m := typed(editing(tabDescription), " Rewritten.")

	_, cmd := chord(m)
	msg, ok := runCmd(cmd).(prview.SetBodyMsg)
	if !ok {
		t.Fatalf("the chord asked for %T, want a SetBodyMsg", runCmd(cmd))
	}
	if !strings.HasSuffix(msg.Body, "Rewritten.") {
		t.Errorf("Body = %q, want what the box was left holding", msg.Body)
	}
}

func TestEditIsInertWhereGitHubSaysTheViewerMayNot(t *testing.T) {
	m := walked(detailed(held(sampleDetail()), 200, 60), tabComment)

	before := m.View()
	if after := press(m, "e").View(); after != before {
		t.Errorf("e opened a box on a comment the viewer may not edit:\n%s", stripANSI(after))
	}
}

func TestEditIsInertOnACommentAlreadyBeingWritten(t *testing.T) {
	d := writable()
	for _, item := range d.Timeline {
		if item.Kind == gh.TimelineComment {
			item.Comment.Editing = true
		}
	}

	m := walked(viewing(d, 200, 60), tabComment)
	before := m.View()
	if after := press(m, "e").View(); after != before {
		t.Errorf("e opened a box on a comment with a write already out:\n%s", stripANSI(after))
	}
}

func TestNeitherKeyTouchesSomebodyElsesComment(t *testing.T) {
	d := writable()
	for _, item := range d.Timeline {
		if item.Kind == gh.TimelineComment {
			item.Comment.ViewerDidAuthor = false
		}
	}

	m := walked(viewing(d, 200, 60), tabComment)
	before := m.View()

	if after := press(m, "e").View(); after != before {
		t.Errorf("e opened a box on somebody else's comment:\n%s", stripANSI(after))
	}
	if after := press(m, "D").View(); after != before {
		t.Errorf("D opened a confirm over somebody else's comment:\n%s", stripANSI(after))
	}
	if out := stripANSI(before); strings.Contains(out, "D delete") {
		t.Error("the card names keys it does not answer to")
	}
}

func TestTheDescriptionIsOnlyEditableByWhoeverOpenedIt(t *testing.T) {
	d := writable()
	d.Author = gh.Actor{Login: "nkr"}

	m := walked(viewing(d, 200, 60), tabDescription)
	before := m.View()

	if after := press(m, "e").View(); after != before {
		t.Errorf("e opened a box on somebody else's description:\n%s", stripANSI(after))
	}
}

// On Commits, because the conversation always lands a cursor.
func TestEditNeedsARingToRead(t *testing.T) {
	m := press(viewing(writable(), 200, 60), "]")

	if after := press(m, "e").View(); after != m.View() {
		t.Error("e opened a box on a tab with no ring")
	}
}

func TestDeleteAsksBeforeItWrites(t *testing.T) {
	m := press(onWritable(tabComment), "D")

	out := stripANSI(m.View())
	if !strings.Contains(out, "Delete this comment?") {
		t.Fatalf("D opened no confirm:\n%s", out)
	}

	m, cmd := pressed(m, "enter")
	if msg := runCmd(cmd); msg != nil {
		t.Errorf("enter on the first row asked for %T, want nothing", msg)
	}
	if strings.Contains(stripANSI(m.View()), "Delete this comment?") {
		t.Error("the confirm is still up after an answer")
	}
	if !strings.Contains(stripANSI(m.View()), "Coverage held at 84.2%") {
		t.Error("the comment went with a cancelled delete")
	}
}

func TestConfirmingTheDeleteSendsTheWrite(t *testing.T) {
	m := press(onWritable(tabComment), "D", "j")

	_, cmd := pressed(m, "enter")
	msg, ok := runCmd(cmd).(prview.DeleteCommentMsg)
	if !ok {
		t.Fatalf("the confirm asked for %T, want a DeleteCommentMsg", runCmd(cmd))
	}

	if msg.CommentID != "IC_octobot" {
		t.Errorf("CommentID = %q, want the focused comment", msg.CommentID)
	}
	if msg.Kind != gh.CommentIssue {
		t.Errorf("Kind = %q, want an issue comment", msg.Kind)
	}
}

func TestEscapeClosesTheDeleteConfirm(t *testing.T) {
	m := onWritable(tabComment)
	before := m.View()

	if after := press(m, "D", "esc").View(); after != before {
		t.Errorf("esc left the confirm changed:\n%s", stripANSI(after))
	}
}

func TestDeleteIsInertOnAReviewsOwnWords(t *testing.T) {
	m := onWritable(tabReview)

	before := m.View()
	if after := press(m, "D").View(); after != before {
		t.Errorf("D opened a confirm over a review body:\n%s", stripANSI(after))
	}
}

func TestDeleteIsInertOnTheDescription(t *testing.T) {
	m := onWritable(tabDescription)

	before := m.View()
	if after := press(m, "D").View(); after != before {
		t.Errorf("D opened a confirm over the description:\n%s", stripANSI(after))
	}
}

func TestTheCardNamesTheWriteKeysItAnswersTo(t *testing.T) {
	out := stripANSI(onWritable(tabComment).View())
	if !strings.Contains(out, "e edit") || !strings.Contains(out, "D delete") {
		t.Errorf("the focused card names neither write key:\n%s", out)
	}

	review := stripANSI(onWritable(tabReview).View())
	if !strings.Contains(review, "e edit") {
		t.Error("the review card does not name the key that edits it")
	}
	if strings.Contains(review, "D delete") {
		t.Error("the review card names a delete key that does nothing")
	}

	plain := stripANSI(walked(detailed(held(sampleDetail()), 200, 60), tabComment).View())
	if strings.Contains(plain, "e edit") || strings.Contains(plain, "D delete") {
		t.Errorf("a card the viewer may not write to names the keys anyway:\n%s", plain)
	}
}

func TestTheCardsHintsGiveWayToTheBoxs(t *testing.T) {
	out := stripANSI(editing(tabComment).View())

	if strings.Contains(out, "D delete") {
		t.Error("the card still names the keys it answered to before the box opened")
	}
	if !strings.Contains(out, "esc discard") {
		t.Errorf("the box does not name the key that closes it:\n%s", out)
	}
	if !strings.Contains(out, "⏎ save") {
		t.Errorf("the box does not name the key that saves it:\n%s", out)
	}
}

func TestTheFrameHoldsItsSizeWithAnEditBoxOpen(t *testing.T) {
	for _, size := range []struct{ width, height int }{
		{200, 60}, {160, 24}, {100, 20},
	} {
		m := press(walked(detailed(held(writable()), size.width, size.height), tabComment), "e")

		lines := strings.Split(m.View(), "\n")
		if len(lines) != size.height {
			t.Errorf("%dx%d rendered %d lines, want %d",
				size.width, size.height, len(lines), size.height)
		}
		for i, line := range lines {
			if w := lipgloss.Width(line); w > size.width {
				t.Errorf("%dx%d line %d is %d wide, want at most %d",
					size.width, size.height, i, w, size.width)
			}
		}
	}
}

func TestTheEditBoxTakesEveryOtherKeyAsText(t *testing.T) {
	m := typed(editing(tabComment), "q")

	if !strings.Contains(stripANSI(m.View()), "Save") {
		t.Error("q closed the box rather than being typed into it")
	}

	if _, cmd := pressed(press(m, "esc"), "q"); runCmd(cmd) != nil {
		t.Error("the screen answered q while the box still had the keyboard")
	}
}

func TestAFailedEditPutsTheWordsBackInTheBox(t *testing.T) {
	m := editing(tabComment)
	m = typed(m, " Updated.")
	m, _ = chord(m)

	m.RestoreEdit("IC_octobot", "Coverage held at 84.2%. Updated.")

	out := stripANSI(m.View())
	if !strings.Contains(out, "Updated.") {
		t.Errorf("the words did not come back:\n%s", out)
	}
	if !strings.Contains(out, "Save") {
		t.Error("the box did not reopen on the comment")
	}
}

func TestEscapeThrowsAwayWhatWasTypedIntoAnEditBox(t *testing.T) {
	m := press(typed(press(onWritable(tabComment), "e"), " Discard me."), "esc")

	out := stripANSI(press(m, "e").View())
	if strings.Contains(out, "Discard me.") {
		t.Errorf("the box reopened on words esc said it would discard:\n%s", out)
	}
	if !strings.Contains(out, "Coverage held at 84.2%") {
		t.Errorf("the box did not reopen on the comment as GitHub has it:\n%s", out)
	}
}

func TestABoxInsideAThreadKeepsItsButtonOnTheScreen(t *testing.T) {
	d := writable()
	for i := range d.Threads[0].Comments {
		d.Threads[0].Comments[i].Body = strings.Repeat("A line of it.\n\n", 40)
	}

	m := press(walked(viewing(d, 200, 24), tabThread), "e")

	if out := stripANSI(m.View()); !strings.Contains(out, "Save") {
		t.Errorf("the box's button is off the screen:\n%s", out)
	}
}

func TestTypingInAThreadKeepsTheButtonOnTheScreen(t *testing.T) {
	d := writable()
	for i := range d.Threads[0].Comments {
		d.Threads[0].Comments[i].Body = strings.Repeat("A line of it.\n\n", 40)
	}

	m := typed(press(walked(viewing(d, 200, 24), tabThread), "e"), "\nthe caret is here")

	out := stripANSI(m.View())
	if !strings.Contains(out, "the caret is here") {
		t.Errorf("the line being written is off the screen:\n%s", out)
	}
	if !strings.Contains(out, "Save") {
		t.Errorf("the button went off the screen as the box grew:\n%s", out)
	}
}

func TestAnEditKeepsTheWhitespaceAroundTheWordsItSends(t *testing.T) {
	d := writable()
	for _, item := range d.Timeline {
		if item.Kind == gh.TimelineComment {
			item.Comment.Body = "    indented := true"
		}
	}

	m := typed(press(walked(viewing(d, 200, 60), tabComment), "e"), " // and stays")
	_, cmd := chord(m)

	msg, ok := runCmd(cmd).(prview.EditCommentMsg)
	if !ok {
		t.Fatalf("the chord asked for %T, want an EditCommentMsg", runCmd(cmd))
	}
	if msg.Body != "    indented := true // and stays" {
		t.Errorf("Body = %q, want the leading indent kept", msg.Body)
	}
}
