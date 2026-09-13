package app

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

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

type commentDeletedMsg struct {
	id  string
	key string
}

type commentDeleteFailedMsg struct {
	id  string
	key string
	err error
}

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

func (m Model) editLanded(msg commentEditedMsg) (tea.Model, tea.Cmd) {
	m.store.CommentEditApplied(msg.id, msg.key, msg.res)

	toast := m.toasts.Show(comp.ToastSuccess, "Saved")
	if !m.showing(msg.id) {
		return m, toast
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(msg.id)), toast)
}

func (m Model) editFailed(msg commentEditFailedMsg) (tea.Model, tea.Cmd) {
	m.store.CommentWriteReverted(msg.id, msg.key)

	toast := m.toasts.Show(comp.ToastError, "Could not save the edit: "+msg.err.Error())

	if !m.showing(msg.id) {
		return m, toast
	}

	shown := m.detail.SetDetail(m.store.Detail(msg.id))
	restored := m.detail.RestoreEdit(msg.comment, msg.body)
	m.resize()
	return m, tea.Batch(shown, restored, toast)
}

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

func (m Model) deleteLanded(msg commentDeletedMsg) (tea.Model, tea.Cmd) {
	m.store.CommentDeleteApplied(msg.id, msg.key)

	toast := m.toasts.Show(comp.ToastSuccess, "Deleted")
	if !m.showing(msg.id) {
		return m, toast
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(msg.id)), toast)
}

func (m Model) deleteFailed(msg commentDeleteFailedMsg) (tea.Model, tea.Cmd) {
	m.store.CommentWriteReverted(msg.id, msg.key)

	toast := m.toasts.Show(comp.ToastError, "Could not delete the comment: "+msg.err.Error())
	if !m.showing(msg.id) {
		return m, toast
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(msg.id)), toast)
}

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
