package app

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
)

// A pulse re-asks the volatile fields alone, for two points against the detail
// query's fifty-six.
type pulseFetchedMsg struct {
	id  string
	res gh.PulseResult
}

// pulseFailedMsg carries no error. Nothing on the screen was waiting on this,
// so there is nothing to say the answer never came.
type pulseFailedMsg struct{ id string }

// pulse starts one, and is nil when the store will not take it: a full fetch is
// already out, one is already out, or the detail was never fetched at all.
func (m Model) pulse(id string) tea.Cmd {
	if !m.store.BeginPulse(id) {
		return nil
	}
	return m.fetchPulse(id)
}

func (m Model) fetchPulse(id string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		res, err := client.Pulse(ctx, id)
		if err != nil {
			return pulseFailedMsg{id: id}
		}
		return pulseFetchedMsg{id: id, res: res}
	}
}

// pulseSettled pushes the answer onto the screen still showing it. It registers
// no refresh leg, so nothing spins and nothing toasts.
func (m Model) pulseSettled(id string) (tea.Model, tea.Cmd) {
	// The fold wrote the pull request back over the row search returned.
	m.list.SetSections(m.store.Sections())

	// Overtaken by a write, so the store dropped it. Nothing else would ask
	// again: the probe spends one wait, and the row would latch on "Checking".
	var owed tea.Cmd
	if m.store.StalePulse(id) {
		owed = m.pulse(id)
	}

	if m.screen != screenDetail || m.detail.PullRequest().ID != id {
		return m, owed
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(id)), owed)
}
