package app

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/tui/comp"
	"github.com/zen-octo/zen-octo/internal/tui/prview"
)

// A reviewer change is applied here before it is sent, so both outcomes name
// the write rather than the pull request alone. Both carry the counts too: the
// toast says what happened, and a delta cannot be read back off the panel.
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

// setReviewers changes who is being waited on, painting the panel on the rail
// before the write leaves.
func (m Model) setReviewers(msg prview.SetReviewersMsg) (tea.Model, tea.Cmd) {
	key := m.store.PendingReviewers(msg.ID, msg.Panel)

	shown := m.detail.SetDetail(m.store.Detail(msg.ID))
	return m, tea.Batch(shown, m.sendReviewers(msg, key))
}

// sendReviewers cancels first and asks second, in one command.
//
// Two calls, because the endpoint has no spelling that means "these and nobody
// else". Only the sides that have something in them: most applies move one
// direction, and the other call would be a round trip that can only cost time.
//
// Removing first is what keeps a swap honest. The requests are a set on
// GitHub's side either way, so the order decides nothing about the result; what
// it decides is which half is already done when the other fails, and a request
// left standing is easier to read on the rail than one silently gone.
//
// Either call failing fails the whole write. The panel goes back to what was
// fetched and the toast carries the reason, which overstates what was undone
// when the first call had already landed. Nothing is refetched on that path, so
// the reason stays on screen rather than being painted over; the next sync is
// what corrects the rail.
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

// reviewersLanded keeps the panel the write put up and then asks for the whole
// detail again.
//
// The refetch is the point, the way it is for a lifecycle change. The endpoint
// answers with the requests the pull request now holds and says nothing about
// who has already reviewed, so it cannot rebuild the panel; and asking somebody
// for a review rewrites the review decision the header renders, which the store
// cannot compute either.
//
// It registers no refresh leg, for the reason stateLanded gives: those belong
// to the sync key, and borrowing one here would raise "Refreshed" behind the
// toast that already said what happened.
func (m Model) reviewersLanded(msg reviewersSetMsg) (tea.Model, tea.Cmd) {
	m.store.ReviewersApplied(msg.id, msg.key)

	cmds := []tea.Cmd{m.toasts.Show(comp.ToastSuccess, reviewerToast(msg.added, msg.removed))}
	if m.showing(msg.id) {
		cmds = append(cmds, m.detail.SetDetail(m.store.Detail(msg.id)))
	}
	return m, tea.Batch(append(cmds, m.correctDetail(msg.id))...)
}

// reviewerToast names what happened rather than that something did. One
// direction at a time is the common case and the one worth spelling out; both
// at once is a swap, and naming two counts in a status bar reads as arithmetic.
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

// reviewersFailed is the revert branch, and it refetches where the other write
// paths do not.
//
// Reverting alone would be a lie here. This is the one write made of two calls,
// so a failure can arrive with the first already applied: the cancellation
// lands, the request is refused, and the fetched panel put back on the rail
// then claims a review is still wanted from somebody GitHub has already
// dropped. The confirmation read makes that the ordinary shape of a failure
// rather than a rare one, because it fails a request whose POST landed.
//
// The refetch does not paint over the reason. It registers no refresh leg, so
// nothing raises a second toast, and the error stays up while the rail corrects
// itself underneath it.
func (m Model) reviewersFailed(msg reviewersFailedMsg) (tea.Model, tea.Cmd) {
	m.store.ReviewersReverted(msg.id, msg.key)

	cmds := []tea.Cmd{m.toasts.Show(comp.ToastError, "Could not change the reviewers: "+msg.err.Error())}
	if m.showing(msg.id) {
		cmds = append(cmds, m.detail.SetDetail(m.store.Detail(msg.id)))
	}
	return m, tea.Batch(append(cmds, m.correctDetail(msg.id))...)
}
