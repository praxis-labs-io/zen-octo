package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// MergeProbe is the wait the mergeability probe arms, for a test outside this
// package to fire by hand.
//
// The harness drops a tea.Tick rather than sleeping on it, so a timer's message
// never arrives on its own; the branch search's settle is driven the same way,
// and that one is a screen message this package already exports.
func MergeProbe(id string) tea.Msg { return mergeProbeMsg{id: id} }

// StubLinks stands in front of the clipboard and the browser for one test, and
// restores both when it ends. No test here runs in parallel.
func StubLinks(t *testing.T, writeClipboard, openBrowser func(string) error) {
	t.Helper()
	prevCopy, prevBrowse := copyLink, browse
	copyLink, browse = writeClipboard, openBrowser
	t.Cleanup(func() { copyLink, browse = prevCopy, prevBrowse })
}
