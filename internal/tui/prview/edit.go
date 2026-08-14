package prview

import (
	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
)

// EditCommentMsg asks the root to rewrite a comment. It carries the kind
// because a node id does not name the mutation that edits it, and the thread
// because that is where the answer has to be folded back.
type EditCommentMsg struct {
	ID        string
	CommentID string
	ThreadID  string
	Kind      gh.CommentKind
	Body      string
}

// DeleteCommentMsg asks the root to remove a comment, on the same terms.
type DeleteCommentMsg struct {
	ID        string
	CommentID string
	ThreadID  string
	Kind      gh.CommentKind
}

// SetBodyMsg asks the root to rewrite the pull request's description. It is not
// an EditCommentMsg with a kind of its own: the description reads as a comment
// on the page and is a field of the pull request to GitHub, written by the
// mutation the labels and the base go through.
type SetBodyMsg struct {
	ID   string
	Body string
}

// target is a block of prose the keys can act on: where the box would go, the
// words in it now, and what it takes to write them.
//
// kind is empty on the description, which is the one block here that is not a
// comment. threadID is empty on everything but a comment inside a review
// thread.
type target struct {
	at       focusKey
	body     string
	kind     gh.CommentKind
	threadID string
}

// editable is what e would open on, or false where there is nothing to edit.
//
// Both questions have to answer yes. GitHub says whether the viewer may, and
// viewerCanUpdate is true on everybody's comment in a repository the viewer
// maintains; whose writing it is is the second question, and rewriting somebody
// else's words under their name is not something this client offers however
// entitled the token is. viewerDidAuthor is GitHub's answer to that one too,
// rather than a login compared here.
//
// A comment already answering for a write is not one to open. Two rewrites out
// at once settle in the order the responses arrive, which is not the order they
// were sent, and the second would open on a body the first has not confirmed.
func (m Model) editable() (target, bool) {
	w, ok := m.onRing()
	if !ok {
		return target{}, false
	}

	if w.kind == "" {
		return w, m.ownDescription()
	}

	c, ok := m.heldComment(w)
	return w, ok && c.ViewerDidAuthor && c.CanEdit && !c.Pending && !c.Editing
}

// ownDescription is whether the description is the viewer's own writing and
// GitHub will take the write.
//
// A pull request carries no viewerDidAuthor, so this is the one place the
// question is answered by comparing logins. An unknown viewer answers no: the
// login is asked for once at startup, and a client that could not learn who it
// is has no business rewriting somebody's opening argument.
func (m Model) ownDescription() bool {
	d := m.detail.Detail
	return d.Viewer.CanUpdate && m.who.Login != "" && m.who.Login == d.Author.Login
}

// deletable is what D would open the confirm on, or false where there is
// nothing to delete. Whose writing it is counts here for the reason it counts
// above, and more so: this one cannot be undone.
//
// Two blocks are refused here that editable takes. The description is not a
// comment and cannot be removed at all. A review's body reports
// viewerCanDelete true and cannot be deleted either: GitHub's own page offers
// no control for one, and deletePullRequestReview takes only a review still
// pending, so the flag is answering a question nobody asked.
func (m Model) deletable() (target, bool) {
	w, ok := m.onRing()
	if !ok || w.kind == "" || w.kind == gh.CommentReview {
		return target{}, false
	}

	c, ok := m.heldComment(w)
	return w, ok && c.ViewerDidAuthor && c.CanDelete && !c.Pending && !c.Editing
}

// onRing is the block the focus is holding, whatever may be done to it.
//
// A thread's own card resolves to the comment it was opened with; a reply
// resolves to itself. Both are cards and both are stops, so the focus names the
// comment and this only has to find it.
func (m Model) onRing() (target, bool) {
	if !m.answerable() {
		return target{}, false
	}

	on := m.convRing.on
	switch on.kind {
	case focusDescription:
		return target{at: on, body: m.detail.Detail.Body}, true

	case focusComment, focusReview:
		for _, item := range m.detail.Detail.Timeline {
			if said := item.Said(); said.ID == on.id {
				return target{at: on, body: said.Body, kind: said.Kind}, true
			}
		}

	case focusThread, focusThreadComment:
		t, ok := m.focusedThread()
		if !ok {
			return target{}, false
		}
		within := m.within(t)
		for _, c := range t.Comments {
			if c.ID == within {
				return target{
					at:       threadCommentKey(c),
					body:     c.Body,
					kind:     gh.CommentThread,
					threadID: t.ID,
				}, true
			}
		}
	}
	return target{}, false
}

// heldComment is the comment a block names, as the detail currently holds it.
func (m Model) heldComment(w target) (gh.Comment, bool) {
	if w.threadID != "" {
		for _, t := range m.detail.Detail.Threads {
			if t.ID != w.threadID {
				continue
			}
			for _, c := range t.Comments {
				if c.ID == w.at.id {
					return c, true
				}
			}
		}
		return gh.Comment{}, false
	}

	for _, item := range m.detail.Detail.Timeline {
		if said := item.Said(); said.ID == w.at.id {
			return said, true
		}
	}
	return gh.Comment{}, false
}

// startEdit opens the box over the words the focus is on. It does nothing where
// there is nothing to edit, which is the rule every key that reads the ring
// holds to: the card says whether it answers to e, and pressing it elsewhere is
// a reader finding out.
func (m Model) startEdit() (Model, tea.Cmd) {
	w, ok := m.editable()
	if !ok {
		return m, nil
	}

	// One box takes the keys. The compose card keeps its words: it is furniture,
	// and esc leaves them there too.
	m.compose.stop()
	m.clearMention()

	cmd := m.inline.open(w.at, m.convRing.on, w.body, updateWords)
	m.convRing.on = w.at
	m.conv.ok = false
	m.focus = paneMain

	// The shortest distance, for the reason a reply opens that way: the block is
	// already under the reader's eye and the box is replacing its words, so
	// moving the page to the top row would take the heading it belongs to with
	// it. Far enough to land the box's foot, though, and its button with it.
	m.showOpenedBox()
	return m, cmd
}

// saveEdit hands the buffer to the root and closes the box.
//
// An empty buffer is nothing to save, and the button says so by going faint
// rather than by swallowing the keypress. Clearing a comment is a thing GitHub
// will do and not a thing this key does: the reader who means it can type a
// space, and the alternative is a stray chord wiping a comment with no confirm
// anywhere near it.
func (m Model) saveEdit() (Model, tea.Cmd) {
	// Sent as it stands, where a new comment is sent trimmed. The whitespace
	// around a comment GitHub already has is the author's: four spaces at the
	// front of it are a code block, and a key that quietly reflowed somebody's
	// markdown on the way past would be doing more than it was pressed for.
	// Trimmed is still what says whether there is anything here to send.
	body := m.inline.area.Value()
	at, from := m.inline.at, m.inline.from
	if m.inline.body() == "" {
		return m, nil
	}

	w, ok := m.targetAt(at)
	if !ok {
		return m, nil
	}

	m.inline.close()

	m.convRing.on = from
	m.conv.ok = false
	m.syncContent()

	// The same body renders either way, and the write it takes is not the same.
	if w.kind == "" {
		id := m.pr.ID
		return m, func() tea.Msg { return SetBodyMsg{ID: id, Body: body} }
	}

	msg := EditCommentMsg{
		ID:        m.pr.ID,
		CommentID: w.at.id,
		ThreadID:  w.threadID,
		Kind:      w.kind,
		Body:      body,
	}
	return m, func() tea.Msg { return msg }
}

// targetAt is the block a box was opened on, found again at the moment the
// write leaves.
//
// The box may have been open for minutes and a refetch may have landed behind
// it, so the kind and the thread are read off the detail as it stands rather
// than off what was true when the box opened. A block that is no longer there
// is a write with nowhere to go.
func (m Model) targetAt(at focusKey) (target, bool) {
	if at.kind == focusDescription {
		return target{at: at}, true
	}

	for _, item := range m.detail.Detail.Timeline {
		if said := item.Said(); said.ID == at.id {
			return target{at: at, body: said.Body, kind: said.Kind}, true
		}
	}

	for _, t := range m.detail.Detail.Threads {
		for _, c := range t.Comments {
			if c.ID == at.id {
				return target{at: at, body: c.Body, kind: gh.CommentThread, threadID: t.ID}, true
			}
		}
	}
	return target{}, false
}

// The two answers the confirm offers. The ids are read straight back off the
// picker, the way the state menu reads a transition back.
const (
	confirmCancel = "cancel"
	confirmDelete = "delete"
)

// startDelete opens the confirm over the comment the focus is on.
//
// A confirm rather than the key doing it. Shift is a guard against leaning on a
// key and no guard at all against meaning the wrong card: a delete is the one
// write on this screen GitHub will not undo, and the revert branch answers for
// a call that failed rather than for one that worked and was not meant.
//
// Cancel is the first row, so the cursor opens on it and confirming costs a
// deliberate second key. It is the same reason the picker opens on what is
// already chosen everywhere else: enter with no movement should change nothing.
func (m Model) startDelete() (Model, tea.Cmd) {
	w, ok := m.deletable()
	if !ok {
		return m, nil
	}

	m.picking = picking{
		field: pickDelete,
		on:    w,
		p: comp.NewPicker(
			"Delete this comment?",
			[]comp.PickerItem{
				{ID: confirmCancel, Name: "Keep it", Color: m.theme.Text},
				{ID: confirmDelete, Name: "Delete", Color: m.theme.Error},
			},
			nil,
			false,
		),
	}
	return m, nil
}

// applyDelete asks the root to remove the comment, where the confirm was left
// on the row that says so.
//
// The comment is the one the modal was opened over rather than whatever the
// ring holds now. Nothing can move the focus while a picker is up, but a
// refetch can land behind one, and a delete addressed to a card the reader was
// not looking at is the worst thing this key could do.
func (m Model) applyDelete(p picking) (Model, tea.Cmd) {
	chosen := p.p.Chosen()
	if len(chosen) != 1 || chosen[0] != confirmDelete {
		return m, nil
	}

	msg := DeleteCommentMsg{
		ID:        m.pr.ID,
		CommentID: p.on.at.id,
		ThreadID:  p.on.threadID,
		Kind:      p.on.kind,
	}

	// Focus goes with the card. The block the ring was holding is about to
	// vanish, and a highlight left on it would name something that is no longer
	// on the page; the next brace re-anchors to whatever is on screen.
	m.convRing.clear()
	m.conv.ok = false
	m.syncContent()
	return m, func() tea.Msg { return msg }
}

// RestoreEdit puts a rewrite that failed back in the box, so the words survive
// the network. The comment behind it has already gone back to what GitHub has.
//
// It takes the keyboard back on the terms RestoreReply sets out: only where
// nothing else has it, because an answer landing late must not steal the caret
// from whatever is being written now.
func (m *Model) RestoreEdit(commentID, body string) tea.Cmd {
	at := m.keyFor(commentID)
	return m.restore(at, body, updateWords,
		func() bool { _, ok := m.targetAt(at); return ok },
		func() focusKey { return m.blockFor(at) })
}

// RestoreBody is RestoreEdit for the description, which has no comment id and
// one place it can be.
func (m *Model) RestoreBody(body string) tea.Cmd {
	at := focusKey{kind: focusDescription}
	return m.restore(at, body, updateWords,
		func() bool { return m.detail.Loaded },
		func() focusKey { return at })
}

// keyFor is the box target a comment id names. The kind is what the render site
// keys its box by, so a thread comment and a top-level one cannot be told apart
// by the id alone.
func (m Model) keyFor(commentID string) focusKey {
	for _, item := range m.detail.Detail.Timeline {
		if said := item.Said(); said.ID == commentID {
			if said.Kind == gh.CommentReview {
				return focusKey{kind: focusReview, id: commentID}
			}
			return focusKey{kind: focusComment, id: commentID}
		}
	}
	return focusKey{kind: focusThreadComment, id: commentID}
}

// blockFor is the ring stop a box's target belongs to, which is where esc lands
// after a box reopened on its own. A comment inside a thread is not a stop of
// its own: tab walks the thread it sits in.
func (m Model) blockFor(at focusKey) focusKey {
	if at.kind != focusThreadComment {
		return at
	}
	for _, t := range m.detail.Detail.Threads {
		for _, c := range t.Comments {
			if c.ID == at.id {
				return threadKey(t)
			}
		}
	}
	return focusKey{}
}
