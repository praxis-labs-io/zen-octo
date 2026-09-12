package prview

import (
	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
)

// EditCommentMsg asks the root to rewrite a comment. Kind picks the mutation and ThreadID is where the answer folds back.
type EditCommentMsg struct {
	ID        string
	CommentID string
	ThreadID  string
	Kind      gh.CommentKind
	Body      string
}

// DeleteCommentMsg asks the root to remove a comment.
type DeleteCommentMsg struct {
	ID        string
	CommentID string
	ThreadID  string
	Kind      gh.CommentKind
}

// SetBodyMsg asks the root to rewrite the pull request's description.
type SetBodyMsg struct {
	ID   string
	Body string
}

type target struct {
	at       focusKey
	body     string
	kind     gh.CommentKind
	threadID string
}

// Requires viewerDidAuthor as well as viewerCanUpdate: a maintainer may edit anyone's comment, and this client does not offer that.
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

// Compares logins because a pull request carries no viewerDidAuthor.
func (m Model) ownDescription() bool {
	d := m.detail.Detail
	return d.Viewer.CanUpdate && m.who.Login != "" && m.who.Login == d.Author.Login
}

// Refuses a review's body: viewerCanDelete is true on one, but only a pending review can be deleted.
func (m Model) deletable() (target, bool) {
	w, ok := m.onRing()
	if !ok || w.kind == "" || w.kind == gh.CommentReview {
		return target{}, false
	}

	c, ok := m.heldComment(w)
	return w, ok && c.ViewerDidAuthor && c.CanDelete && !c.Pending && !c.Editing
}

func (m Model) onRing() (target, bool) {
	if !m.answerable() {
		return target{}, false
	}

	on := m.mainRing().on
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

func (m Model) startEdit() (Model, tea.Cmd) {
	w, ok := m.editable()
	if !ok {
		return m, nil
	}

	m.compose.stop()
	m.clearMention()

	cmd := m.inline.open(w.at, m.pageRing.on, w.body, updateWords)
	m.pageRing.on = w.at
	m.conv.ok = false
	m.focus = paneMain

	m.showOpenedBox()
	return m, cmd
}

// Sends the body untrimmed, since leading whitespace in existing markdown can be a code block.
func (m Model) saveEdit() (Model, tea.Cmd) {
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

	m.pageRing.on = from
	m.conv.ok = false
	m.syncContent()

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

const (
	confirmCancel = "cancel"
	confirmDelete = "delete"
)

// Opens on the row that keeps the comment, so enter with no movement deletes nothing.
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

	m.pageRing.clear()
	m.conv.ok = false
	m.syncContent()
	return m, func() tea.Msg { return msg }
}

// RestoreEdit files a failed rewrite as the comment's draft and reopens the box only if nothing else has the keyboard.
func (m *Model) RestoreEdit(commentID, body string) tea.Cmd {
	at := m.keyFor(commentID)
	return m.restore(at, body, updateWords,
		func() bool { _, ok := m.targetAt(at); return ok },
		func() focusKey { return m.blockFor(at) })
}

// RestoreBody is RestoreEdit for the pull request's description.
func (m *Model) RestoreBody(body string) tea.Cmd {
	at := focusKey{kind: focusDescription}
	return m.restore(at, body, updateWords,
		func() bool { return m.detail.Loaded },
		func() focusKey { return at })
}

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
