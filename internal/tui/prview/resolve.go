package prview

import (
	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

// toggleResolved settles the thread the ring is on, or opens a settled one.
//
// It reads threadOnRing rather than focusedThread: a resolved thread is
// collapsed, and closed is the state this key exists to change. The screen
// never writes IsResolved itself. The store holds the write beside the fetched
// detail and it comes back through SetDetail, which is what keeps one source of
// truth for a thread and makes the revert a single call.
func (m Model) toggleResolved() (Model, tea.Cmd) {
	t, ok := m.threadOnRing()
	if !ok || !m.canToggleResolved(t) {
		return m, nil
	}

	id, thread, want := m.pr.ID, t.ID, !t.IsResolved
	return m, func() tea.Msg {
		return ResolveThreadMsg{ID: id, ThreadID: thread, Resolved: want}
	}
}

// canToggleResolved is whether GitHub will take the press. The two permissions
// are separate, so a viewer allowed to close a thread and not to reopen it has
// the key on one card and not on the other.
//
// A thread already answering for a write is inert whatever the permissions say.
// Two out at once settle in the order the responses arrive rather than the
// order they were pressed, and the card would then read the opposite of the
// last press until a refetch.
func (m Model) canToggleResolved(t gh.ReviewThread) bool {
	if t.Pending {
		return false
	}
	if t.IsResolved {
		return t.CanUnresolve
	}
	return t.CanResolve
}
