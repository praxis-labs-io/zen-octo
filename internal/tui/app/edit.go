package app

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

// A rewrite is applied here before it is sent, so both outcomes name the write
// rather than the pull request alone. The failure carries the comment and the
// words back: the box closed when the write left, and the words are the only
// thing in this program that cannot be fetched again.
type commentEditedMsg struct {
	id  string
	key string
	res gh.CommentResult
}

type commentEditFailedMsg struct {
	id      string
	key     string
	comment string
	body    string
	err     error
}

// A delete writes no words, so its failure carries only what the toast has to
// say. The store puts the comment back on its own.
type commentDeletedMsg struct {
	id  string
	key string
}

type commentDeleteFailedMsg struct {
	id  string
	key string
	err error
}

// The description settles through the edit queue rather than the comment
// writes, because that is the queue the mutation behind it belongs to.
type bodySetMsg struct {
	id  string
	key string
	res gh.BodyResult
}

type bodyFailedMsg struct {
	id   string
	key  string
	body string
	err  error
}

// editComment rewrites a comment, showing the new words before they are sent.
// The card is the acknowledgement; a toast saying "saving" would be a second
// one for the same fact.
func (m Model) editComment(msg prview.EditCommentMsg) (tea.Model, tea.Cmd) {
	key := m.store.PendingCommentEdit(msg.ID, msg.CommentID, msg.ThreadID, msg.Body)

	shown := m.detail.SetDetail(m.store.Detail(msg.ID))
	return m, tea.Batch(shown, m.sendEdit(msg, key))
}

func (m Model) sendEdit(msg prview.EditCommentMsg, key string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		res, err := client.UpdateComment(ctx, msg.Kind, msg.CommentID, msg.Body)
		if err != nil {
			return commentEditFailedMsg{
				id: msg.ID, key: key, comment: msg.CommentID, body: msg.Body, err: err,
			}
		}
		return commentEditedMsg{id: msg.ID, key: key, res: res}
	}
}

// editLanded writes GitHub's version over the optimistic one.
func (m Model) editLanded(msg commentEditedMsg) (tea.Model, tea.Cmd) {
	m.store.CommentEditApplied(msg.id, msg.key, msg.res)

	toast := m.toasts.Show(comp.ToastSuccess, "Saved")
	if !m.showing(msg.id) {
		return m, toast
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(msg.id)), toast)
}

// editFailed is the revert branch. The comment goes back to the words GitHub
// has, and the words that were typed go back in a box on it.
func (m Model) editFailed(msg commentEditFailedMsg) (tea.Model, tea.Cmd) {
	m.store.CommentWriteReverted(msg.id, msg.key)

	toast := m.toasts.Show(comp.ToastError, "Could not save the edit: "+msg.err.Error())

	// A reader who left has no box to put the words back into. The toast still
	// goes up: they are about to find the comment unchanged.
	if !m.showing(msg.id) {
		return m, toast
	}

	shown := m.detail.SetDetail(m.store.Detail(msg.id))
	restored := m.detail.RestoreEdit(msg.comment, msg.body)
	m.resize()
	return m, tea.Batch(shown, restored, toast)
}

// deleteComment removes a comment, taking it off the page before the write
// leaves. The gap is the acknowledgement.
func (m Model) deleteComment(msg prview.DeleteCommentMsg) (tea.Model, tea.Cmd) {
	key := m.store.PendingCommentDelete(msg.ID, msg.CommentID, msg.ThreadID)

	shown := m.detail.SetDetail(m.store.Detail(msg.ID))
	return m, tea.Batch(shown, m.sendDelete(msg, key))
}

func (m Model) sendDelete(msg prview.DeleteCommentMsg, key string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		if err := client.DeleteComment(ctx, msg.Kind, msg.CommentID); err != nil {
			return commentDeleteFailedMsg{id: msg.ID, key: key, err: err}
		}
		return commentDeletedMsg{id: msg.ID, key: key}
	}
}

// deleteLanded keeps the comment off the page, which is where the optimistic
// write already put it.
func (m Model) deleteLanded(msg commentDeletedMsg) (tea.Model, tea.Cmd) {
	m.store.CommentDeleteApplied(msg.id, msg.key)

	toast := m.toasts.Show(comp.ToastSuccess, "Deleted")
	if !m.showing(msg.id) {
		return m, toast
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(msg.id)), toast)
}

// deleteFailed is the revert branch. The comment comes back where it was, and
// the toast is the only thing that says the press went anywhere.
func (m Model) deleteFailed(msg commentDeleteFailedMsg) (tea.Model, tea.Cmd) {
	m.store.CommentWriteReverted(msg.id, msg.key)

	toast := m.toasts.Show(comp.ToastError, "Could not delete the comment: "+msg.err.Error())
	if !m.showing(msg.id) {
		return m, toast
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(msg.id)), toast)
}

// setBody rewrites the description, painting it before the write leaves.
func (m Model) setBody(msg prview.SetBodyMsg) (tea.Model, tea.Cmd) {
	key := m.store.PendingBody(msg.ID, msg.Body)

	shown := m.detail.SetDetail(m.store.Detail(msg.ID))
	return m, tea.Batch(shown, m.sendBody(msg, key))
}

func (m Model) sendBody(msg prview.SetBodyMsg, key string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		res, err := client.SetBody(ctx, msg.ID, msg.Body)
		if err != nil {
			return bodyFailedMsg{id: msg.ID, key: key, body: msg.Body, err: err}
		}
		return bodySetMsg{id: msg.ID, key: key, res: res}
	}
}

func (m Model) bodyLanded(msg bodySetMsg) (tea.Model, tea.Cmd) {
	m.store.BodyApplied(msg.id, msg.key, msg.res)

	toast := m.toasts.Show(comp.ToastSuccess, "Saved")
	if !m.showing(msg.id) {
		return m, toast
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(msg.id)), toast)
}

// bodyFailed is the revert branch, and it puts the words back the way a failed
// comment edit does: the description is the reader's writing too.
func (m Model) bodyFailed(msg bodyFailedMsg) (tea.Model, tea.Cmd) {
	m.store.EditReverted(msg.id, msg.key)

	toast := m.toasts.Show(comp.ToastError, "Could not save the description: "+msg.err.Error())
	if !m.showing(msg.id) {
		return m, toast
	}

	shown := m.detail.SetDetail(m.store.Detail(msg.id))
	restored := m.detail.RestoreBody(msg.body)
	m.resize()
	return m, tea.Batch(shown, restored, toast)
}
