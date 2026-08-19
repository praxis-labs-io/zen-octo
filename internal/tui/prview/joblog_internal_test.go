package prview

import (
	"strings"
	"testing"
	"time"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

func TestJobLogSanitizingKeepsSGRAndDropsTerminalControls(t *testing.T) {
	got := cleanJobLogLine("\x1b[31mfailed\x1b[0m \x1b[2J\x1b[Hstill here \x1b]2;owned\a")
	if plain := xansi.Strip(got); plain != "failed still here " {
		t.Errorf("plain log = %q", plain)
	}
	if !strings.Contains(got, "\x1b[31m") {
		t.Errorf("the safe foreground color was dropped: %q", got)
	}
	for _, unsafe := range []string{"\x1b[2J", "\x1b[H", "\x1b]2;", "\a"} {
		if strings.Contains(got, unsafe) {
			t.Errorf("unsafe sequence %q survived in %q", unsafe, got)
		}
	}
	if !strings.HasSuffix(got, xansi.ResetStyle) {
		t.Errorf("styled log does not reset at its edge: %q", got)
	}
}

func TestMalformedAndPrivateEscapesAreDropped(t *testing.T) {
	got := cleanJobLogLine("before\x1b[?25lmiddle\x1b[31after")
	if strings.Contains(got, "\x1b") || got != "beforemiddlefter" {
		t.Errorf("cleaned malformed log = %q", got)
	}
}

func TestJobLogSanitizingKeepsUnicodeWithNoCellOfItsOwn(t *testing.T) {
	got := cleanJobLogLine("e\u0301")
	if got != "e\u0301" {
		t.Errorf("combining text = %q, want e with its accent", got)
	}
}

func TestLogLinesSkipAStatusOnlyStepAndReachTheOneAfterIt(t *testing.T) {
	at := time.Date(2026, 8, 19, 14, 0, 0, 0, time.UTC)
	job := gh.Job{Steps: []gh.JobStep{
		{Number: 1, Name: "one", StartedAt: at},
		{Number: 2, Name: "skipped", State: gh.CheckStateSkipped},
		{Number: 3, Name: "three", StartedAt: at.Add(2 * time.Second)},
	}}
	sections := splitJobLog(job,
		"2026-08-19T14:00:01Z first\n2026-08-19T14:00:03Z third\n")
	if len(sections[0].lines) != 1 || sections[0].lines[0] != "first" {
		t.Errorf("first = %q", sections[0].lines)
	}
	if len(sections[1].lines) != 0 {
		t.Errorf("skipped = %q, want no log", sections[1].lines)
	}
	if len(sections[2].lines) != 1 || sections[2].lines[0] != "third" {
		t.Errorf("third = %q", sections[2].lines)
	}
}
