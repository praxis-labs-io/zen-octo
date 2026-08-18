package app

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/link"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
)

// The two side effects that leave the process, behind vars so a test can stand
// in front of a real clipboard and a real browser.
var (
	copyLink = link.Copy
	browse   = link.Browse
)

// linkCopiedMsg reports a URL on the clipboard. Native says whether the
// platform's own tool took it; where it did not, OSC52 is what does.
type linkCopiedMsg struct {
	number int
	url    string
	native bool
}

// browseFailedMsg reports a browser that would not launch. Success says
// nothing: the browser taking focus is its own account of what happened.
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

// linkCopied toasts, and falls back to OSC52 where the native write failed. The
// failure is not reported: it says only that this machine has no clipboard tool.
func (m Model) linkCopied(msg linkCopiedMsg) (tea.Model, tea.Cmd) {
	toast := m.toasts.Show(comp.ToastSuccess, "Copied the link to #"+strconv.Itoa(msg.number))
	if msg.native {
		return m, toast
	}
	return m, tea.Batch(tea.SetClipboard(msg.url), toast)
}
