package prview

import (
	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

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

// A thread with a write in flight is inert: two resolves settle in response order, not press order.
func (m Model) canToggleResolved(t gh.ReviewThread) bool {
	if t.Pending {
		return false
	}
	if t.IsResolved {
		return t.CanUnresolve
	}
	return t.CanResolve
}
