package app

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

type pulseFetchedMsg struct {
	id  string
	res gh.PulseResult
}

type pulseFailedMsg struct{ id string }

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

// Pushes nothing where nothing moved: a relayout on every beat is a hitch on every beat.
func (m Model) pulseSettled(id string, moved bool) (tea.Model, tea.Cmd) {
	var owed tea.Cmd
	if m.store.StalePulse(id) {
		owed = m.pulse(id)
	}
	if !moved {
		return m, owed
	}

	m.list.SetSections(m.store.Sections())

	if m.screen != screenDetail || m.detail.PullRequest().ID != id {
		return m, owed
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(id)), m.correctTimeline(id, time.Now()), owed)
}
