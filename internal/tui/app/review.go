package app

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

type reviewSubmittedMsg struct {
	id  string
	key string
	res gh.ReviewResult
}

type reviewFailedMsg struct {
	id    string
	key   string
	held  bool
	event gh.ReviewEvent
	body  string
	err   error
}

type reviewDiscardedMsg struct {
	id  string
	key string
}

type discardFailedMsg struct {
	id  string
	key string
	err error
}

func (m Model) submitReview(msg prview.SubmitReviewMsg) (tea.Model, tea.Cmd) {
	key := m.store.PendingReview(msg.ID, msg.Event, msg.Body, m.store.Viewer(), time.Now())

	shown := m.detail.SetDetail(m.store.Detail(msg.ID))
	return m, tea.Batch(shown, m.sendReview(msg, key))
}

func (m Model) sendReview(msg prview.SubmitReviewMsg, key string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		var res gh.ReviewResult
		var err error
		if msg.ReviewID != "" {
			res, err = client.SubmitReview(ctx, msg.ReviewID, msg.Event, msg.Body)
		} else {
			res, err = client.StartReview(ctx, msg.ID, msg.Event, msg.Body)
		}
		if err != nil {
			return reviewFailedMsg{
				id: msg.ID, key: key, held: msg.ReviewID != "", event: msg.Event, body: msg.Body, err: err,
			}
		}
		return reviewSubmittedMsg{id: msg.ID, key: key, res: res}
	}
}

// Refetches, as a merge does: the review decision is GitHub's to compute and is never predicted here.
func (m Model) reviewLanded(msg reviewSubmittedMsg) (tea.Model, tea.Cmd) {
	m.store.ReviewApplied(msg.id, msg.key, msg.res)

	cmds := []tea.Cmd{
		m.toasts.Show(comp.ToastSuccess, reviewToast(msg.res.Review.State)),
		m.correctDetail(msg.id),
	}
	if m.showing(msg.id) {
		cmds = append(cmds, m.detail.SetDetail(m.store.Detail(msg.id)))
	}
	return m, tea.Batch(cmds...)
}

func reviewToast(s gh.ReviewState) string {
	switch s {
	case gh.ReviewStateApproved:
		return "Approved"
	case gh.ReviewStateChangesRequested:
		return "Requested changes"
	}
	return "Reviewed"
}

// Only a refused pending review refetches: one opened and submitted in a single call left nothing behind.
func (m Model) reviewFailed(msg reviewFailedMsg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{m.toasts.Show(comp.ToastError, "Could not submit the review: "+msg.err.Error())}
	if msg.held {
		m.store.EditRevertedStale(msg.id, msg.key)
		cmds = append(cmds, m.correctDetail(msg.id))
	} else {
		m.store.EditReverted(msg.id, msg.key)
	}

	if m.showing(msg.id) {
		cmds = append(cmds, m.detail.SetDetail(m.store.Detail(msg.id)))
		m.detail.RestoreReview(msg.event, msg.body)
	}
	return m, tea.Batch(cmds...)
}

func (m Model) discardReview(msg prview.DiscardReviewMsg) (tea.Model, tea.Cmd) {
	key := m.store.PendingDiscard(msg.ID, msg.ReviewID)

	shown := m.detail.SetDetail(m.store.Detail(msg.ID))
	return m, tea.Batch(shown, m.sendDiscard(msg, key))
}

func (m Model) sendDiscard(msg prview.DiscardReviewMsg, key string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		if err := client.DiscardReview(ctx, msg.ReviewID); err != nil {
			return discardFailedMsg{id: msg.ID, key: key, err: err}
		}
		return reviewDiscardedMsg{id: msg.ID, key: key}
	}
}

func (m Model) discardLanded(msg reviewDiscardedMsg) (tea.Model, tea.Cmd) {
	m.store.DiscardApplied(msg.id, msg.key)

	toast := m.toasts.Show(comp.ToastSuccess, "Discarded")
	if !m.showing(msg.id) {
		return m, toast
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(msg.id)), toast)
}

func (m Model) discardFailed(msg discardFailedMsg) (tea.Model, tea.Cmd) {
	m.store.EditReverted(msg.id, msg.key)

	toast := m.toasts.Show(comp.ToastError, "Could not discard the review: "+msg.err.Error())
	if !m.showing(msg.id) {
		return m, toast
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(msg.id)), toast)
}
