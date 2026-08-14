package app

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
	"github.com/zen-octo/zen-octo/internal/tui/prview"
)

// A reaction settles the way a resolve does and writes no words, so the failure
// carries only what the toast has to say: which direction the press was going.
// The store puts the pill back on its own.
type reactedMsg struct {
	id  string
	key string
	res gh.ReactionResult
}

type reactFailedMsg struct {
	id  string
	key string
	on  bool
	err error
}

// react toggles a reaction, moving the pill before the write leaves. The pill
// is the acknowledgement, the way the optimistic comment is one.
func (m Model) react(msg prview.ReactMsg) (tea.Model, tea.Cmd) {
	key := m.store.PendingReaction(msg.ID, msg.CommentID, msg.ThreadID, msg.Content, msg.On)

	shown := m.detail.SetDetail(m.store.Detail(msg.ID))
	return m, tea.Batch(shown, m.sendReaction(msg, key))
}

func (m Model) sendReaction(msg prview.ReactMsg, key string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		res, err := client.SetReaction(ctx, msg.SubjectID, msg.Content, msg.On)
		if err != nil {
			return reactFailedMsg{id: msg.ID, key: key, on: msg.On, err: err}
		}
		return reactedMsg{id: msg.ID, key: key, res: res}
	}
}

// reactionLanded takes GitHub's answer, which is the subject's whole set: a
// reaction somebody else gave between the press and the response lands with it.
//
// It says nothing. The pill is already on the card and a toast per reaction
// would spend the status bar on the smallest write on this screen, taking the
// line off whatever else was using it.
func (m Model) reactionLanded(msg reactedMsg) (tea.Model, tea.Cmd) {
	m.store.ReactionApplied(msg.id, msg.key, msg.res)

	if !m.showing(msg.id) {
		return m, nil
	}
	return m, m.detail.SetDetail(m.store.Detail(msg.id))
}

// reactionFailed is the revert branch. Nothing was typed and no box changed
// height, so the pill going back where it was is the whole of it.
func (m Model) reactionFailed(msg reactFailedMsg) (tea.Model, tea.Cmd) {
	m.store.ReactionReverted(msg.id, msg.key)

	doing := "remove"
	if msg.on {
		doing = "add"
	}

	toast := m.toasts.Show(comp.ToastError, "Could not "+doing+" the reaction: "+msg.err.Error())
	if !m.showing(msg.id) {
		return m, toast
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(msg.id)), toast)
}
