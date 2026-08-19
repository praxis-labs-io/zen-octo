package app

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

type checkRerunMsg struct{ name string }

type checkRerunFailedMsg struct {
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
			return checkRerunFailedMsg{jobID: msg.JobID, name: msg.Name, err: err}
		}
		return checkRerunMsg{name: msg.Name}
	}
}

func (m Model) checkRerunLanded(msg checkRerunMsg) (tea.Model, tea.Cmd) {
	// GitHub accepts the write before the replacement attempt reaches the check
	// rollup. Keep the optimistic state through that gap: an immediate detail
	// fetch can still report the failed attempt, or briefly fold an older passing
	// one over it. The Checks poll clears it when the new job id or pending state
	// arrives.
	return m, m.toasts.Show(comp.ToastSuccess, "Rerunning "+msg.name)
}

func (m Model) checkRerunFailed(msg checkRerunFailedMsg) (tea.Model, tea.Cmd) {
	m.detail.RerunSettled(msg.jobID)
	return m, m.toasts.Show(comp.ToastError, "Could not rerun "+msg.name+": "+msg.err.Error())
}
