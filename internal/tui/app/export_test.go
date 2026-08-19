package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// MinWidth and MinHeight are the floor, so a test sizes itself against the real
// numbers rather than one it chose and left behind when the floor moved.
const (
	MinWidth  = minWidth
	MinHeight = minHeight
)

// PollTick is one beat of the background poll, for a test outside this package
// to fire by hand. It carries its instant, so a test names when it fired.
func PollTick(at time.Time) tea.Msg { return pollTickMsg{at: at} }

// ChecksTick fires one tick of the Checks tab's own chain by hand.
func ChecksTick(at time.Time) tea.Msg { return checksTickMsg{at: at} }

// PollIdle is the interval a settled pull request and the list are re-asked on,
// so a test can step past it rather than restate the number.
const PollIdle = pollIdle

// ChecksBeat is the Checks tab's own interval.
const ChecksBeat = checksBeat

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
