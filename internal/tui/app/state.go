package app

import (
	"context"
	"strings"

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

	// GitHub's answer, not the ask. They part company when somebody else moves
	// the pull request first, and the rail is already showing what came back.
	held := m.store.Detail(msg.id).Detail.PullRequest
	landed, _ := comp.PRStateLabel(m.theme, held)

	cmds := []tea.Cmd{m.toasts.Show(comp.ToastSuccess, stateToast(msg.to, held, landed))}
	if m.showing(msg.id) {
		cmds = append(cmds, m.detail.SetDetail(m.store.Detail(msg.id)))
	}
	return m, tea.Batch(append(cmds, m.correctDetail(msg.id))...)
}

// correctDetail asks for the detail again after a write, and is nil when it
// cannot.
//
// A fetch already in flight was asked for before this write, so it will answer
// with the state from before it. The store drops that response rather than
// storing it, and this owes another fetch once it has: detailSettled calls back
// here, and by then BeginDetail will take it.
func (m Model) correctDetail(id string) tea.Cmd {
	pr := m.store.Detail(id).Detail.PullRequest
	if pr.ID == "" || !m.store.BeginDetail(id) {
		return nil
	}
	return m.fetchDetail(id, pr.HeadRefName)
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

// stateToast says what happened, naming the move only when the pull request
// actually made it.
//
// The ask and the answer part company whenever somebody moves it first: asking
// to convert one to a draft that has just been closed in the browser answers
// CLOSED, and the rail is already showing that. Naming the ask there would
// contradict the row under it and report a write that did not happen, so the
// state it landed in is named instead. landed is the word the rail uses, passed
// in rather than rebuilt, so the two cannot drift.
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

// took reports whether the pull request ended up where the transition meant to
// put it. Reopening says nothing about the draft flag, which it gives back
// whatever it was, so it asks only about the state.
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
