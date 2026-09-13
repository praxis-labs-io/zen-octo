package app

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

type mergedMsg struct {
	id    string
	key   string
	refID string
	res   gh.MergeResult
}

type mergeFailedMsg struct {
	id  string
	key string
	err error
}

// No success counterpart: its toast would land behind the merge's and take the status bar off it.
type refDeleteFailedMsg struct {
	branch string
	err    error
}

type mergeProbeMsg struct{ id string }

const mergeProbeDelay = 1500 * time.Millisecond

func (m Model) merge(msg prview.MergeMsg) (tea.Model, tea.Cmd) {
	key := m.store.PendingMerge(msg.ID)

	shown := m.detail.SetDetail(m.store.Detail(msg.ID))
	return m, tea.Batch(shown, m.sendMerge(msg, key))
}

func (m Model) sendMerge(msg prview.MergeMsg, key string) tea.Cmd {
	client := m.client

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		res, err := client.Merge(ctx, msg.ID, msg.Options)
		if err != nil {
			return mergeFailedMsg{id: msg.ID, key: key, err: err}
		}
		return mergedMsg{id: msg.ID, key: key, refID: msg.RefID, res: res}
	}
}

// Deletes the branch only when GitHub answered merged, since gh.Merge takes any state back as success.
func (m Model) mergeLanded(msg mergedMsg) (tea.Model, tea.Cmd) {
	m.store.MergeApplied(msg.id, msg.key, msg.res)

	held := m.store.Detail(msg.id).Detail
	if msg.res.State != gh.PRStateMerged {
		toast := m.toasts.Show(comp.ToastError,
			"GitHub answered "+strings.ToLower(string(msg.res.State))+" rather than merged")
		return m, tea.Batch(toast, m.correctDetail(msg.id))
	}

	cmds := []tea.Cmd{m.toasts.Show(comp.ToastSuccess, "Merged into "+held.BaseRefName)}
	if m.showing(msg.id) {
		cmds = append(cmds, m.detail.SetDetail(m.store.Detail(msg.id)))
	}
	return m, tea.Batch(append(cmds, m.correctDetail(msg.id), m.deleteRef(msg))...)
}

func (m Model) deleteRef(msg mergedMsg) tea.Cmd {
	if msg.refID == "" {
		return nil
	}

	client, refID := m.client, msg.refID
	branch := m.store.Detail(msg.id).Detail.HeadRefName

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		if err := client.DeleteRef(ctx, refID); err != nil {
			return refDeleteFailedMsg{branch: branch, err: err}
		}
		return nil
	}
}

// Claims nothing about the branch: a request that timed out may still have deleted it.
func (m Model) refDeleteFailed(msg refDeleteFailedMsg) (tea.Model, tea.Cmd) {
	return m, m.toasts.Show(comp.ToastError,
		"Merged. Could not confirm "+msg.branch+" was deleted: "+msg.err.Error())
}

// Refetches, unlike other reverts: the commonest refusal is a head that moved since the fetch.
func (m Model) mergeFailed(msg mergeFailedMsg) (tea.Model, tea.Cmd) {
	m.store.EditRevertedStale(msg.id, msg.key)

	cmds := []tea.Cmd{
		m.toasts.Show(comp.ToastError, "Could not merge: "+msg.err.Error()),
		m.correctDetail(msg.id),
	}
	if m.showing(msg.id) {
		cmds = append(cmds, m.detail.SetDetail(m.store.Detail(msg.id)))
	}
	return m, tea.Batch(cmds...)
}

// Call before the response is stored: GitHub computes mergeability lazily, and only a first landing is probed.
func (m Model) probeMergeability(id string, res gh.DetailResult) tea.Cmd {
	if m.store.Detail(id).Loaded {
		return nil
	}
	if res.Detail.Merge != gh.MergeUnknown || res.Detail.State != gh.PRStateOpen {
		return nil
	}
	return tea.Tick(mergeProbeDelay, func(time.Time) tea.Msg { return mergeProbeMsg{id: id} })
}

// Re-arms under a fetch in flight, which was asked before GitHub had the answer and will land UNKNOWN too.
func (m Model) mergeProbe(msg mergeProbeMsg) (tea.Model, tea.Cmd) {
	if m.store.Detail(msg.id).Detail.Merge != gh.MergeUnknown {
		return m, nil
	}
	if cmd := m.pulse(msg.id); cmd != nil {
		return m, cmd
	}
	return m, tea.Tick(mergeProbeDelay, func(time.Time) tea.Msg { return msg })
}
