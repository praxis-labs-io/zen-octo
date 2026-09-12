package app

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

type stateSetMsg struct {
	id  string
	key string
	to  gh.PRTransition
	res gh.PRStateResult
}

type stateFailedMsg struct {
	id  string
	key string
	to  gh.PRTransition
	err error
}

func (m Model) setState(msg prview.SetStateMsg) (tea.Model, tea.Cmd) {
	key := m.store.PendingState(msg.ID, msg.To)

	shown := m.detail.SetDetail(m.store.Detail(msg.ID))
	return m, tea.Batch(shown, m.sendState(msg, key))
}

func (m Model) sendState(msg prview.SetStateMsg, key string) tea.Cmd {
	client := m.client

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		res, err := client.SetState(ctx, msg.ID, msg.To)
		if err != nil {
			return stateFailedMsg{id: msg.ID, key: key, to: msg.To, err: err}
		}
		return stateSetMsg{id: msg.ID, key: key, to: msg.To, res: res}
	}
}

// Registers no refresh leg, which would raise "Refreshed" behind the toast that already said what happened.
func (m Model) stateLanded(msg stateSetMsg) (tea.Model, tea.Cmd) {
	m.store.StateApplied(msg.id, msg.key, msg.res)

	held := m.store.Detail(msg.id).Detail.PullRequest
	landed, _ := comp.PRStateLabel(m.theme, held)

	cmds := []tea.Cmd{m.toasts.Show(comp.ToastSuccess, stateToast(msg.to, held, landed))}
	if m.showing(msg.id) {
		cmds = append(cmds, m.detail.SetDetail(m.store.Detail(msg.id)))
	}
	return m, tea.Batch(append(cmds, m.correctDetail(msg.id))...)
}

func (m Model) correctDetail(id string) tea.Cmd {
	pr := m.store.Detail(id).Detail.PullRequest
	if pr.ID == "" || !m.store.BeginDetail(id) {
		return nil
	}
	return m.fetchDetail(id, pr.HeadRefName)
}

func (m Model) stateFailed(msg stateFailedMsg) (tea.Model, tea.Cmd) {
	m.store.EditReverted(msg.id, msg.key)

	toast := m.toasts.Show(comp.ToastError, "Could not "+stateVerb(msg.to)+": "+msg.err.Error())
	if !m.showing(msg.id) {
		return m, toast
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(msg.id)), toast)
}

func stateToast(to gh.PRTransition, pr gh.PullRequest, landed string) string {
	if !took(to, pr) {
		return "Now " + strings.ToLower(landed)
	}

	switch to {
	case gh.TransitionReady:
		return "Marked ready for review"
	case gh.TransitionDraft:
		return "Converted to draft"
	case gh.TransitionClose:
		return "Closed"
	case gh.TransitionReopen:
		return "Reopened"
	}
	return "Now " + strings.ToLower(landed)
}

// Reopening gives the draft flag back as it was, so only the state is checked.
func took(to gh.PRTransition, pr gh.PullRequest) bool {
	switch to {
	case gh.TransitionReady:
		return pr.State == gh.PRStateOpen && !pr.IsDraft
	case gh.TransitionDraft:
		return pr.State == gh.PRStateOpen && pr.IsDraft
	case gh.TransitionClose:
		return pr.State == gh.PRStateClosed
	case gh.TransitionReopen:
		return pr.State == gh.PRStateOpen
	}
	return false
}

func stateVerb(to gh.PRTransition) string {
	switch to {
	case gh.TransitionReady:
		return "mark it ready for review"
	case gh.TransitionDraft:
		return "convert it to a draft"
	case gh.TransitionClose:
		return "close it"
	case gh.TransitionReopen:
		return "reopen it"
	}
	return "change the state"
}
