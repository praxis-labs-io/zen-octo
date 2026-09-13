package app

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/update"
)

// ReleaseCheck reports whether a newer release is published. A failure shows nothing.
type ReleaseCheck func(ctx context.Context) (update.Result, error)

type newerReleaseMsg struct {
	tag string
}

func checkRelease(check ReleaseCheck) tea.Cmd {
	if check == nil {
		return nil
	}
	return func() tea.Msg {
		result, err := check(context.Background())
		if err != nil || !result.Available {
			return nil
		}
		return newerReleaseMsg{tag: result.Latest}
	}
}

func (m Model) releaseNotice() string {
	if m.newerRelease == "" {
		return ""
	}
	return lipgloss.NewStyle().Foreground(m.theme.Subtle).
		Render(fmt.Sprintf("%s is available. Run zen-octo update.", m.newerRelease))
}
