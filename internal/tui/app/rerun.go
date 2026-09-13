package app

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

type checkRerunMsg struct {
	jobID      int64
	name       string
	acceptedAt time.Time
}

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

		acceptedAt, err := client.RerunJob(ctx, msg.Repo, msg.JobID)
		if err != nil {
			return checkRerunFailedMsg{jobID: msg.JobID, name: msg.Name, err: err}
		}
		return checkRerunMsg{jobID: msg.JobID, name: msg.Name, acceptedAt: acceptedAt}
	}
}

func (m Model) checkRerunLanded(msg checkRerunMsg) (tea.Model, tea.Cmd) {
	m.detail.RerunAccepted(msg.jobID, msg.acceptedAt)
	return m, m.toasts.Show(comp.ToastSuccess, "Rerunning "+msg.name)
}

func (m Model) checkRerunFailed(msg checkRerunFailedMsg) (tea.Model, tea.Cmd) {
	m.detail.RerunSettled(msg.jobID)
	return m, m.toasts.Show(comp.ToastError, "Could not rerun "+msg.name+": "+msg.err.Error())
}

// acceptedAt is stamped locally: neither bulk rerun endpoint reports when GitHub accepted it.
type runRerunMsg struct {
	jobIDs     []int64
	name       string
	all        bool
	acceptedAt time.Time
}

type runRerunFailedMsg struct {
	jobIDs []int64
	name   string
	err    error
}

func (m Model) rerunRun(msg prview.RerunRunMsg) (tea.Model, tea.Cmd) {
	client := m.client
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		rerun := client.RerunFailedJobs
		if msg.All {
			rerun = client.RerunAllJobs
		}
		if err := rerun(ctx, msg.Repo, msg.RunID); err != nil {
			return runRerunFailedMsg{jobIDs: msg.JobIDs, name: msg.Name, err: err}
		}
		return runRerunMsg{jobIDs: msg.JobIDs, name: msg.Name, all: msg.All, acceptedAt: time.Now()}
	}
}

func (m Model) runRerunLanded(msg runRerunMsg) (tea.Model, tea.Cmd) {
	m.detail.RunRerunAccepted(msg.jobIDs, msg.acceptedAt)
	what := "failed jobs"
	if msg.all {
		what = "all jobs"
	}
	return m, m.toasts.Show(comp.ToastSuccess, "Rerunning "+what+" in "+msg.name)
}

func (m Model) runRerunFailed(msg runRerunFailedMsg) (tea.Model, tea.Cmd) {
	m.detail.RunRerunSettled(msg.jobIDs)
	return m, m.toasts.Show(comp.ToastError, "Could not rerun "+msg.name+": "+msg.err.Error())
}
