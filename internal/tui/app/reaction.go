package app

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

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

// No toast: the pill is already on the card.
func (m Model) reactionLanded(msg reactedMsg) (tea.Model, tea.Cmd) {
	m.store.ReactionApplied(msg.id, msg.key, msg.res)

	if !m.showing(msg.id) {
		return m, nil
	}
	return m, m.detail.SetDetail(m.store.Detail(msg.id))
}

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
