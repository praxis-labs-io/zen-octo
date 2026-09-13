package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

const (
	MinWidth  = minWidth
	MinHeight = minHeight
)

func PollTick(at time.Time) tea.Msg { return pollTickMsg{at: at} }

func ChecksTick(at time.Time) tea.Msg { return checksTickMsg{at: at} }

const PollIdle = pollIdle

const ChecksBeat = checksBeat

// The harness drops a tea.Tick rather than sleeping on it, so the probe is fired by hand.
func MergeProbe(id string) tea.Msg { return mergeProbeMsg{id: id} }

func StubLinks(t *testing.T, writeClipboard, openBrowser func(string) error) {
	t.Helper()
	prevCopy, prevBrowse := copyLink, browse
	copyLink, browse = writeClipboard, openBrowser
	t.Cleanup(func() { copyLink, browse = prevCopy, prevBrowse })
}
