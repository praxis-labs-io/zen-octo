package app

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

// A pulse re-asks the volatile fields alone, and answers in a few hundred bytes
// where the detail query answers in megabytes.
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

// pulseSettled pushes the answer onto the screen still showing it, and pushes
// nothing where nothing moved: on a timer, a relayout a beat is a hitch a beat.
func (m Model) pulseSettled(id string, moved bool) (tea.Model, tea.Cmd) {
	// Overtaken by a write, so the store dropped it. Nothing else would ask
	// again: the probe spends one wait, and the row would latch on "Checking".
	var owed tea.Cmd
	if m.store.StalePulse(id) {
		owed = m.pulse(id)
	}
	// A dropped pulse reports the same false, and means it: it wrote nothing.
	if !moved {
		return m, owed
	}

	// The fold wrote the pull request back over the row search returned.
	m.list.SetSections(m.store.Sections())

	if m.screen != screenDetail || m.detail.PullRequest().ID != id {
		return m, owed
	}
	// A comment or a review is not on this wire, so the page may owe a real
	// fetch. The tab on screen is what decides whether to spend it.
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(id)), m.correctTimeline(id, time.Now()), owed)
}
