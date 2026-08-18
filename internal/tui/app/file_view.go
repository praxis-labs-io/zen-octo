package app

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/tui/prview"
)

func (m Model) toggleFileViewed(msg prview.ToggleFileViewedMsg) (tea.Model, tea.Cmd) {
	key := m.store.PendingFileView(msg.ID, msg.Path, msg.Viewed)
	shown := m.detail.SetFiles(m.store.Files(msg.ID))
	return m, tea.Batch(shown, m.sendFileViewed(msg, key))
}

func (m Model) sendFileViewed(msg prview.ToggleFileViewedMsg, key string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		if err := client.SetFileViewed(ctx, msg.ID, msg.Path, msg.Viewed); err != nil {
			return fileViewFailedMsg{id: msg.ID, key: key, path: msg.Path, err: err}
		}
		return fileViewedMsg{id: msg.ID, key: key}
	}
}
