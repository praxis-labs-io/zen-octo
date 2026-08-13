package prview_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/prview"
)

// Where the ring stops on the fixture, for the blocks the write keys act on.
const (
	tabDescription = 1
	tabComment     = 2
	tabReview      = 3
)

// writable is the fixture with GitHub's permissions on it. The sample carries
// none, which is a viewer who may read and nothing else, so every key here
// would be inert without this.
//
// The review's own body is deletable and its comment is not, which is the pair
// the delete key has to tell apart: GitHub answers true on both and has a call
// for only one of them.
func writable() gh.PullRequestDetail {
	d := sampleDetail()

	for _, item := range d.Timeline {
		switch item.Kind {
		case gh.TimelineComment:
			item.Comment.CanEdit, item.Comment.CanDelete = true, true
			// Three paragraphs, so the card has a height an opening box could
			// change.
			item.Comment.Body += "\n\nThe cap holds through a retry storm.\n\nRunbook updated."
		case gh.TimelineReview:
			item.Comment.CanEdit, item.Comment.CanDelete = true, true
		}
	}

	for i := range d.Threads[0].Comments {
		d.Threads[0].Comments[i].CanEdit = true
		d.Threads[0].Comments[i].CanDelete = true
	}
	return d
}

// editing walks the ring to a block and opens the box over it.
func editing(n int) prview.Model {
	return press(onWritable(n), "e")
}

func onWritable(n int) prview.Model {
	return walked(detailed(held(writable()), 200, 60), n)
}

// The box goes where the words were, inside the card they were in. A comment
// being rewritten is not a new comment, and putting the box at the foot of the
// page would leave the old words on screen above it.
func TestTheEditBoxOpensInPlaceOfTheWordsItReplaces(t *testing.T) {
	out := stripANSI(editing(tabComment).View())

	head := strings.Index(out, "octobot · commented")
	box := strings.Index(out, "Update")
	foot := strings.Index(out, "write a comment")

	switch {
	case box < 0:
		t.Fatalf("e opened no box:\n%s", out)
	case head < 0 || box < head:
		t.Error("the box is above the heading of the card it is inside")
	case foot >= 0 && box > foot:
		t.Error("the box is at the foot of the page rather than in the card")
	}

	// The card keeps its heading, so the reader can see whose words they are
	// rewriting.
	if !strings.Contains(out, "octobot · commented") {
		t.Error("the card lost its heading when the box opened")
	}
	// The words are in the box rather than rendered under it.
	if !strings.Contains(out, "Coverage held at 84.2%") {
		t.Errorf("the box did not open on the comment's own words:\n%s", out)
	}
}

// The box is the height of the words it replaces, so opening one costs the
// single row its button takes and nothing more. A box of its own fixed height
// would shrink a long comment to a window onto itself and balloon a short one.
func TestTheEditBoxIsTheHeightOfTheWordsItReplaces(t *testing.T) {
	m := onWritable(tabComment)
	below := "nkr · requested changes"

	before := lineOf(t, m.View(), below)
	after := lineOf(t, press(m, "e").View(), below)

	if got := after - before; got != 1 {
		t.Errorf("opening the box moved the card below it by %d lines, want 1 for the button", got)
	}
}

// And it grows as the writing does, rather than scrolling inside a fixed frame.
func TestTheEditBoxGrowsWithWhatIsTypedIntoIt(t *testing.T) {
	m := press(onWritable(tabComment), "e")
	below := "nkr · requested changes"

	before := lineOf(t, m.View(), below)
	after := lineOf(t, typed(m, "\n\n").View(), below)

	if got := after - before; got != 2 {
		t.Errorf("two new lines moved the card below by %d lines, want 2", got)
	}
}

// The compose card holds whatever was typed into it. If the two shared one
// buffer, opening this box would throw a half-written comment away.
func TestOpeningTheEditBoxKeepsAHalfWrittenComment(t *testing.T) {
	m := typed(press(detailed(held(writable()), 200, 60), "c"), "half a thought")
	m = press(m, "esc")

	// The ring is on the compose card, so one step is the description and two
	// is the comment.
	m = press(walked(m, tabComment), "e")
	if !strings.Contains(stripANSI(m.View()), "Coverage held at 84.2%") {
		t.Fatal("the edit box did not open on the comment's own words")
	}

	// G, because the compose card closes a conversation longer than the window
	// and the box being open has scrolled it out of sight.
	out := stripANSI(press(m, "esc", "G").View())
	if !strings.Contains(out, "half a thought") {
		t.Errorf("opening the edit box took the comment being written:\n%s", out)
	}
}

// A box grows with what is typed into it, so it can be taller than the window
// it is drawn in. The caret has to be on the screen anyway: the page follows it
// rather than the card it sits in, which is the one place on this screen the
// scroll is not about a block.
func TestTheCaretStaysOnTheScreenInABoxTallerThanTheWindow(t *testing.T) {
	d := writable()
	for _, item := range d.Timeline {
		if item.Kind == gh.TimelineComment {
			item.Comment.Body = strings.Repeat("A line of it.\n\n", 40)
		}
	}

	// A window shorter than the comment, so the box opens taller than the pane
	// and the caret lands at the end of it.
	m := press(walked(detailed(held(d), 200, 24), tabComment), "e")
	m = typed(m, "the caret is here")

	if out := stripANSI(m.View()); !strings.Contains(out, "the caret is here") {
		t.Errorf("the line being written is off the screen:\n%s", out)
	}
}

// The compose card grows the same way, and its caret is kept the same way.
func TestTheCommentBoxGrowsAndKeepsItsCaretOnTheScreen(t *testing.T) {
	m := press(detailed(held(writable()), 200, 24), "c")
	m = typed(m, strings.Repeat("A line of it.\n", 30)+"the caret is here")

	if out := stripANSI(m.View()); !strings.Contains(out, "the caret is here") {
		t.Errorf("the line being written is off the screen:\n%s", out)
	}
}

// esc leaves the comment as it was. Nothing was written, so nothing is saved,
// and the card goes back to rendering its markdown.
func TestEscapeClosesTheEditBoxAndLeavesTheComment(t *testing.T) {
	m := onWritable(tabComment)
	before := m.View()

	if after := press(press(m, "e"), "esc").View(); after != before {
		t.Errorf("esc left the card changed:\n%s", stripANSI(after))
	}
}

// The chord sends the write. It carries what the box holds, addressed to the
// comment by node id and kind: the kind is what picks the mutation.
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

// A review's own words go through a third mutation, so the kind has to be the
// one the detail carried rather than the one a comment usually is.
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

// Tab walks whole threads, so the write keys act on the comment the sub-cursor
// is on, which is the one a quote reply takes too: the last, until J or K says
// otherwise.
func TestEditingAThreadTakesTheCommentTheSubCursorIsOn(t *testing.T) {
	m := typed(editing(tabThread), "!")

	_, cmd := chord(m)
	msg, ok := runCmd(cmd).(prview.EditCommentMsg)
	if !ok {
		t.Fatalf("the chord asked for %T, want an EditCommentMsg", runCmd(cmd))
	}

	if msg.CommentID != "RC_4" {
		t.Errorf("CommentID = %q, want the last comment in the thread", msg.CommentID)
	}
	if msg.ThreadID != "RT_1" {
		t.Errorf("ThreadID = %q, want the thread it sits in", msg.ThreadID)
	}
	if msg.Kind != gh.CommentThread {
		t.Errorf("Kind = %q, want a review comment", msg.Kind)
	}

	// K steps back a comment, and the key follows it.
	_, cmd = chord(typed(press(onWritable(tabThread), "K", "e"), "!"))
	if msg, _ := runCmd(cmd).(prview.EditCommentMsg); msg.CommentID != "RC_1" {
		t.Errorf("CommentID = %q after K, want the comment above", msg.CommentID)
	}
}

// The description is not a comment to GitHub, so it goes through the mutation
// that writes the pull request.
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

// The permission is GitHub's answer. Without it the key does nothing, rather
// than opening a box on a write that comes back refused.
func TestEditIsInertWhereGitHubSaysTheViewerMayNot(t *testing.T) {
	m := walked(detailed(held(sampleDetail()), 200, 60), tabComment)

	before := m.View()
	if after := press(m, "e").View(); after != before {
		t.Errorf("e opened a box on a comment the viewer may not edit:\n%s", stripANSI(after))
	}
}

// A comment already answering for a write is not one to open. Two rewrites out
// at once settle in whatever order the responses arrive.
func TestEditIsInertOnACommentAlreadyBeingWritten(t *testing.T) {
	d := writable()
	for _, item := range d.Timeline {
		if item.Kind == gh.TimelineComment {
			item.Comment.Editing = true
		}
	}

	m := walked(detailed(held(d), 200, 60), tabComment)
	before := m.View()
	if after := press(m, "e").View(); after != before {
		t.Errorf("e opened a box on a comment with a write already out:\n%s", stripANSI(after))
	}
}

// The key reads the ring, so it does nothing with nothing focused.
func TestEditNeedsSomethingFocused(t *testing.T) {
	m := detailed(held(writable()), 200, 60)

	if after := press(m, "e").View(); after != m.View() {
		t.Error("e opened a box with nothing focused")
	}
}

// D asks first. The cursor opens on the answer that changes nothing, so enter
// with no movement closes the modal and writes nothing.
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
	// The comment is still there, which is the whole of what cancelling means.
	if !strings.Contains(stripANSI(m.View()), "Coverage held at 84.2%") {
		t.Error("the comment went with a cancelled delete")
	}
}

// Confirming is a second, deliberate key: down onto the row that says delete,
// then enter.
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

// esc backs out of the confirm the way it backs out of every other modal.
func TestEscapeClosesTheDeleteConfirm(t *testing.T) {
	m := onWritable(tabComment)
	before := m.View()

	if after := press(m, "D", "esc").View(); after != before {
		t.Errorf("esc left the confirm changed:\n%s", stripANSI(after))
	}
}

// viewerCanDelete comes back true on a submitted review and there is no call
// that deletes one. A key that opens a confirm on a write GitHub cannot take is
// worse than one that does nothing.
func TestDeleteIsInertOnAReviewsOwnWords(t *testing.T) {
	m := onWritable(tabReview)

	before := m.View()
	if after := press(m, "D").View(); after != before {
		t.Errorf("D opened a confirm over a review body:\n%s", stripANSI(after))
	}
}

// The description cannot be deleted at all: it is a field of the pull request
// rather than something somebody said.
func TestDeleteIsInertOnTheDescription(t *testing.T) {
	m := onWritable(tabDescription)

	before := m.View()
	if after := press(m, "D").View(); after != before {
		t.Errorf("D opened a confirm over the description:\n%s", stripANSI(after))
	}
}

// A card names the keys it answers to and no others.
func TestTheCardNamesTheWriteKeysItAnswersTo(t *testing.T) {
	out := stripANSI(onWritable(tabComment).View())
	if !strings.Contains(out, "e edit") || !strings.Contains(out, "D delete") {
		t.Errorf("the focused card names neither write key:\n%s", out)
	}

	// The review has one of the two, so the line has to differ.
	review := stripANSI(onWritable(tabReview).View())
	if !strings.Contains(review, "e edit") {
		t.Error("the review card does not name the key that edits it")
	}
	if strings.Contains(review, "D delete") {
		t.Error("the review card names a delete key that does nothing")
	}

	// Nothing named where GitHub says no.
	plain := stripANSI(walked(detailed(held(sampleDetail()), 200, 60), tabComment).View())
	if strings.Contains(plain, "e edit") || strings.Contains(plain, "D delete") {
		t.Errorf("a card the viewer may not write to names the keys anyway:\n%s", plain)
	}
}

// The box carries its own hints, so the card's line goes: two lines of keys
// over one card would name keys that are not live.
func TestTheCardsHintsGiveWayToTheBoxs(t *testing.T) {
	out := stripANSI(editing(tabComment).View())

	// "e edit" would match inside the box's own "ctrl+e editor", so the delete
	// key is what says whether the card's line is still there.
	if strings.Contains(out, "D delete") {
		t.Error("the card still names the keys it answered to before the box opened")
	}
	if !strings.Contains(out, "esc cancel") {
		t.Errorf("the box does not name the key that closes it:\n%s", out)
	}
	if !strings.Contains(out, "⏎ update") {
		t.Errorf("the box does not name the key that saves it:\n%s", out)
	}
}

// The frame is the size it was given, box or no box. A card that grows a
// textarea inside it is the one place a block changes height without the page
// being rebuilt around it.
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

// A key the box does not answer is text, the way it is in the compose card:
// the screen stands aside while a box has the keyboard.
func TestTheEditBoxTakesEveryOtherKeyAsText(t *testing.T) {
	m := typed(editing(tabComment), "q")

	if !strings.Contains(stripANSI(m.View()), "Update") {
		t.Error("q closed the box rather than being typed into it")
	}

	// And the screen has it back once the box is closed.
	if _, cmd := pressed(press(m, "esc"), "q"); runCmd(cmd) != nil {
		t.Error("the screen answered q while the box still had the keyboard")
	}
}

// The words survive a write that failed, which is the one thing on this screen
// that cannot be fetched again.
func TestAFailedEditPutsTheWordsBackInTheBox(t *testing.T) {
	m := editing(tabComment)
	m = typed(m, " Updated.")
	m, _ = chord(m)

	// The card is back to what GitHub has while the write is out and after it
	// fails; the box is what carries the words.
	m.RestoreEdit("IC_octobot", "Coverage held at 84.2%. Updated.")

	out := stripANSI(m.View())
	if !strings.Contains(out, "Updated.") {
		t.Errorf("the words did not come back:\n%s", out)
	}
	if !strings.Contains(out, "Update") {
		t.Error("the box did not reopen on the comment")
	}
}
