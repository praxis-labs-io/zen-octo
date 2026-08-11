package app

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
	"github.com/zen-octo/zen-octo/internal/tui/prview"
)

// A lifecycle change is applied here before it is sent, so both outcomes name
// the write rather than the pull request alone. Both carry the transition too:
// the toast says what happened, and "Closed" is not something the state GitHub
// answers with can be read back into on its own.
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

// setState moves a pull request through its lifecycle, painting the new state
// on the rail before the write leaves.
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

// stateLanded takes GitHub's answer and then asks for the whole detail again.
//
// The refetch is the point. Half the rail hangs off the state through fields
// the store cannot compute: converting to draft makes the pull request
// unmergeable, closing it settles every check, and either one puts an event on
// the timeline. Writing the two fields alone would leave "Ready to merge" under
// a row that now says "Draft".
//
// It registers no refresh leg. Those belong to the sync key, which reports a
// summary when they all answer; borrowing one here would raise "Refreshed"
// behind the toast that already said what happened. Unclaimed, the response
// falls through detailSettled and lands on the screen the ordinary way.
//
// The pull request comes from the store rather than the screen, so a reader who
// has walked back to the list still gets the correction.
func (m Model) stateLanded(msg stateSetMsg) (tea.Model, tea.Cmd) {
	m.store.StateApplied(msg.id, msg.key, msg.res)

	cmds := []tea.Cmd{m.toasts.Show(comp.ToastSuccess, stateToast(msg.to))}
	if m.showing(msg.id) {
		cmds = append(cmds, m.detail.SetDetail(m.store.Detail(msg.id)))
	}

	pr := m.store.Detail(msg.id).Detail.PullRequest
	if pr.ID != "" && m.store.BeginDetail(msg.id) {
		cmds = append(cmds, m.fetchDetail(msg.id, pr.HeadRefName))
	}
	return m, tea.Batch(cmds...)
}

// stateFailed is the revert branch. Nothing was typed, so the fetched state
// going back on the rail is the whole of it.
func (m Model) stateFailed(msg stateFailedMsg) (tea.Model, tea.Cmd) {
	m.store.EditReverted(msg.id, msg.key)

	toast := m.toasts.Show(comp.ToastError, "Could not "+stateVerb(msg.to)+": "+msg.err.Error())
	if !m.showing(msg.id) {
		return m, toast
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(msg.id)), toast)
}

// stateToast says what happened, in the tense it happened in.
func stateToast(to gh.PRTransition) string {
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
	return "State changed"
}

// stateVerb is the same move as the thing that could not be done, so the
// failure names what was asked for rather than "change the state".
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
