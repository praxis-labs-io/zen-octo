package app

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
	"github.com/zen-octo/zen-octo/internal/tui/prview"
)

// A merge is applied here before it is sent, so both outcomes carry the key the
// store reconciles against.
//
// The landed one carries the branch to delete, because that call is made off
// the back of this response rather than beside the merge: a branch deleted for
// a merge that then failed is a branch deleted for nothing.
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

// The branch delete answers only when it fails. It carries no key, because it
// settles nothing: the merge it followed has already settled, and there is no
// optimistic anything on the screen about a branch.
//
// Success says nothing, and that is deliberate. It lands a moment behind the
// merge's own toast and would take the status bar off it, which is the more
// important of the two by some way; the box that was ticked already said the
// branch was going.
type refDeleteFailedMsg struct {
	branch string
	err    error
}

// mergeProbeMsg is a wait that ran out on a pull request GitHub had not
// finished working out the mergeability of.
type mergeProbeMsg struct{ id string }

// mergeProbeDelay is how long to leave GitHub to answer a question the first
// query is what asked. Long enough for the computation it started, short enough
// that the Merge row is live before the reader has read the description.
const mergeProbeDelay = 1500 * time.Millisecond

// merge lands a pull request, painting it merged on the rail before the write
// leaves.
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

// mergeLanded takes GitHub's answer, asks for everything the merge rewrote, and
// deletes the head branch where the form asked for it.
//
// The refetch is not optional. A merge writes the merged event onto the
// timeline, settles the checks, ends the review and moves what the viewer may
// do next, and the store can compute none of it.
//
// The diff is left alone. A merge changes nothing about the difference between
// the two branches, which is what the Files tab is showing.
//
// It reads the state GitHub answered with rather than the one that was asked
// for, and deletes the branch only where that answer is a merge. gh.Merge takes
// any pull request back as a success, so an answer in another state would
// otherwise be toasted as merged and have the branch deleted off the back of
// it, which is the half of this that cannot be undone. The lifecycle write next
// door refuses to name a move its answer does not confirm, for the same reason.
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

// deleteRef removes the head branch, and is nil where nothing asked.
//
// It goes after the merge rather than beside it. The two are one intention and
// two calls, and the second cannot undo the first: a branch deleted for a merge
// that came back refused is work nobody can get back.
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

// refDeleteFailed says so and leaves the merge standing. There is nothing to
// revert: the pull request is merged either way, and the only thing left undone
// is a branch the reader can delete themselves.
//
// It reports what is not known rather than what is. An error is not proof the
// branch survived, since the request can time out over a delete GitHub has
// already made, and the one honest thing to say is that nothing here can tell.
func (m Model) refDeleteFailed(msg refDeleteFailedMsg) (tea.Model, tea.Cmd) {
	return m, m.toasts.Show(comp.ToastError,
		"Merged. Could not confirm "+msg.branch+" was deleted: "+msg.err.Error())
}

// mergeFailed is the revert branch, and it refetches, which is what tells it
// apart from every other one here.
//
// A refused merge is evidence about the screen rather than about the pull
// request. The commonest refusal is the head having moved since the detail was
// fetched, and there the fetched row is the very thing that lost the merge:
// putting "Ready to merge" straight back says the branch is as it was and
// invites the same press again. The rest are the same shape, a rule or a
// permission the screen is a round trip behind on. So the state comes off and
// the detail is asked for again.
//
// The toast carries GitHub's own sentence rather than a house one. A stale head
// comes back as "Head branch was modified. Review and try the merge again",
// which already says what to do about it.
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

// probeMergeability asks for the detail again a moment after one that came back
// not knowing whether the pull request can be merged.
//
// GitHub computes that lazily, and the query that reads it is what starts the
// computation. So a pull request nothing has looked at recently answers UNKNOWN
// the first time and has a real answer a second later, and without this the
// Merge row is inert until somebody presses the sync key for reasons the screen
// never gives them.
//
// It reads the detail the store is still holding, so it has to be called before
// the response replaces it: a first landing is one with nothing loaded behind
// it. That is what keeps it to one extra request. A refetch lands over a loaded
// detail, so a pull request GitHub keeps answering UNKNOWN for is asked twice
// and then left alone.
//
// Merged and closed pull requests are not asked about at all. Nothing is going
// to be merged, so an answer would change nothing on the screen.
func (m Model) probeMergeability(id string, res gh.DetailResult) tea.Cmd {
	if m.store.Detail(id).Loaded {
		return nil
	}
	if res.Detail.Merge != gh.MergeUnknown || res.Detail.State != gh.PRStateOpen {
		return nil
	}
	return tea.Tick(mergeProbeDelay, func(time.Time) tea.Msg { return mergeProbeMsg{id: id} })
}

// mergeProbe asks the question again, as a pulse: mergeability is one of the
// fields it carries, and the whole page costs twenty-eight times as much.
//
// A wait that runs out on a question already answered asks nothing: the answer
// can arrive from the sync key or from a write's own refetch while this is still
// out, and a request for something already on the screen is one the reader never
// made.
//
// A fetch already in flight is the other way this comes back empty, and there
// the wait is armed again rather than dropped. That fetch was asked for before
// GitHub had worked the answer out, so it will land carrying the same UNKNOWN,
// and nothing else would ever ask: the row would sit on "Checking" until
// somebody pressed sync for reasons the screen never gave them. It cannot spin,
// because each turn needs a fetch in flight and every fetch lands.
func (m Model) mergeProbe(msg mergeProbeMsg) (tea.Model, tea.Cmd) {
	if m.store.Detail(msg.id).Detail.Merge != gh.MergeUnknown {
		return m, nil
	}
	if cmd := m.pulse(msg.id); cmd != nil {
		return m, cmd
	}
	return m, tea.Tick(mergeProbeDelay, func(time.Time) tea.Msg { return msg })
}
