package app

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

type assigneesSetMsg struct {
	id  string
	key string
	res gh.AssigneesResult
}

type assigneesFailedMsg struct {
	id  string
	key string
	err error
}

func (m Model) setAssignees(msg prview.SetAssigneesMsg) (tea.Model, tea.Cmd) {
	key := m.store.PendingAssignees(msg.ID, msg.Assignees)

	shown := m.detail.SetDetail(m.store.Detail(msg.ID))
	return m, tea.Batch(shown, m.sendAssignees(msg, key))
}

func (m Model) sendAssignees(msg prview.SetAssigneesMsg, key string) tea.Cmd {
	client := m.client

	ids := make([]string, 0, len(msg.Assignees))
	for _, a := range msg.Assignees {
		ids = append(ids, a.ID)
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		res, err := client.SetAssignees(ctx, msg.ID, ids)
		if err != nil {
			return assigneesFailedMsg{id: msg.ID, key: key, err: err}
		}
		return assigneesSetMsg{id: msg.ID, key: key, res: res}
	}
}

// No refetch: assigning changes nothing the store cannot already see.
func (m Model) assigneesLanded(msg assigneesSetMsg) (tea.Model, tea.Cmd) {
	m.store.AssigneesApplied(msg.id, msg.key, msg.res)

	toast := m.toasts.Show(comp.ToastSuccess, "Assignees updated")
	if !m.showing(msg.id) {
		return m, toast
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(msg.id)), toast)
}

func (m Model) assigneesFailed(msg assigneesFailedMsg) (tea.Model, tea.Cmd) {
	m.store.EditReverted(msg.id, msg.key)

	toast := m.toasts.Show(comp.ToastError, "Could not set the assignees: "+msg.err.Error())
	if !m.showing(msg.id) {
		return m, toast
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(msg.id)), toast)
}
