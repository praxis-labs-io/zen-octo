package app

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

type reviewersSetMsg struct {
	id      string
	key     string
	added   int
	removed int
}

type reviewersFailedMsg struct {
	id  string
	key string
	err error
}

func (m Model) setReviewers(msg prview.SetReviewersMsg) (tea.Model, tea.Cmd) {
	key := m.store.PendingReviewers(msg.ID, msg.Panel)

	shown := m.detail.SetDetail(m.store.Detail(msg.ID))
	return m, tea.Batch(shown, m.sendReviewers(msg, key))
}

// Two calls because the endpoint cannot say "these and nobody else"; cancellations go first.
func (m Model) sendReviewers(msg prview.SetReviewersMsg, key string) tea.Cmd {
	client := m.client

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		if len(msg.Remove) > 0 {
			if err := client.RemoveReviewRequests(ctx, msg.Repo, msg.Number, msg.Remove); err != nil {
				return reviewersFailedMsg{id: msg.ID, key: key, err: err}
			}
		}
		if len(msg.Add) > 0 {
			if err := client.RequestReviews(ctx, msg.Repo, msg.Number, msg.Add); err != nil {
				return reviewersFailedMsg{id: msg.ID, key: key, err: err}
			}
		}
		return reviewersSetMsg{
			id: msg.ID, key: key,
			added: len(msg.Add), removed: len(msg.Remove),
		}
	}
}

// Refetches: the endpoint reports outstanding requests and nothing about who already reviewed.
func (m Model) reviewersLanded(msg reviewersSetMsg) (tea.Model, tea.Cmd) {
	m.store.ReviewersApplied(msg.id, msg.key)

	cmds := []tea.Cmd{m.toasts.Show(comp.ToastSuccess, reviewerToast(msg.added, msg.removed))}
	if m.showing(msg.id) {
		cmds = append(cmds, m.detail.SetDetail(m.store.Detail(msg.id)))
	}
	return m, tea.Batch(append(cmds, m.correctDetail(msg.id))...)
}

func reviewerToast(added, removed int) string {
	switch {
	case added > 0 && removed > 0:
		return "Reviewers updated"
	case added > 0:
		return "Requested " + comp.Plural(added, "review")
	case removed > 0:
		return "Cancelled " + comp.Plural(removed, "review request")
	}
	return "Reviewers updated"
}

// Refetches, unlike other reverts: a failure can arrive with the cancellation already applied.
func (m Model) reviewersFailed(msg reviewersFailedMsg) (tea.Model, tea.Cmd) {
	m.store.EditRevertedStale(msg.id, msg.key)

	cmds := []tea.Cmd{m.toasts.Show(comp.ToastError, "Could not change the reviewers: "+msg.err.Error())}
	if m.showing(msg.id) {
		cmds = append(cmds, m.detail.SetDetail(m.store.Detail(msg.id)))
	}
	return m, tea.Batch(append(cmds, m.correctDetail(msg.id))...)
}
