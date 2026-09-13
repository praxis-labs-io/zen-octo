package app

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/link"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
)

var (
	copyLink = link.Copy
	browse   = link.Browse
)

type linkCopiedMsg struct {
	number int
	url    string
	native bool
}

type browseFailedMsg struct{ err error }

func copyLinkCmd(pr gh.PullRequest) tea.Cmd {
	return func() tea.Msg {
		err := copyLink(pr.URL)
		return linkCopiedMsg{number: pr.Number, url: pr.URL, native: err == nil}
	}
}

func browseCmd(pr gh.PullRequest) tea.Cmd {
	return func() tea.Msg {
		if err := browse(pr.URL); err != nil {
			return browseFailedMsg{err: err}
		}
		return nil
	}
}

// A failed native write is not reported: OSC52 still carries the copy.
func (m Model) linkCopied(msg linkCopiedMsg) (tea.Model, tea.Cmd) {
	toast := m.toasts.Show(comp.ToastSuccess, "Copied the link to #"+strconv.Itoa(msg.number))
	if msg.native {
		return m, toast
	}
	return m, tea.Batch(tea.SetClipboard(msg.url), toast)
}
