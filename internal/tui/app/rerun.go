package app

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

type checkRerunMsg struct {
	id    string
	jobID int64
	name  string
}

type checkRerunFailedMsg struct {
	id    string
	jobID int64
	name  string
	err   error
}

func (m Model) rerunCheck(msg prview.RerunCheckMsg) (tea.Model, tea.Cmd) {
	client := m.client
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		if err := client.RerunJob(ctx, msg.Repo, msg.JobID); err != nil {
			return checkRerunFailedMsg{id: msg.ID, jobID: msg.JobID, name: msg.Name, err: err}
		}
		return checkRerunMsg{id: msg.ID, jobID: msg.JobID, name: msg.Name}
	}
}

func (m Model) checkRerunLanded(msg checkRerunMsg) (tea.Model, tea.Cmd) {
	m.detail.RerunSettled(msg.jobID)
	return m, tea.Batch(
		m.toasts.Show(comp.ToastSuccess, "Rerunning "+msg.name),
		m.correctDetail(msg.id),
	)
}

func (m Model) checkRerunFailed(msg checkRerunFailedMsg) (tea.Model, tea.Cmd) {
	m.detail.RerunSettled(msg.jobID)
	return m, m.toasts.Show(comp.ToastError, "Could not rerun "+msg.name+": "+msg.err.Error())
}
