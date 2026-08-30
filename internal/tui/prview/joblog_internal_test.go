package prview

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
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

func TestJobLabelsCannotCarryTerminalControlsOrNewRows(t *testing.T) {
	got := cleanJobLabel("CI\nowned \x1b]52;c;secret\a\x1b[31mred")
	if got != "CI owned red" {
		t.Errorf("clean label = %q", got)
	}
}

func TestRawC1TerminalControlsAreDropped(t *testing.T) {
	got := cleanJobLogLine("before\x9b2Jmiddle\x9d52;c;owned\x9cafter")
	if strings.Contains(got, "\x9b") || strings.Contains(got, "\x9d") ||
		strings.Contains(got, "\x9c") || got != "beforemiddleafter" {
		t.Errorf("cleaned C1 log = %q", got)
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

// Where the terminal answered nothing the fill is nil, and RGBA() on one panics:
// opening a job log took the client down.
func TestASelectedJobLogLineSurvivesAThemeWithNoSurface(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   theme.Theme
	}{
		{"palette reported", testTheme},
		{"transparent", theme.Terminal(testSurface, true)},
		{"nothing answered", theme.Terminal(theme.Surface{}, false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			faint := lipgloss.NewStyle().Foreground(tc.th.Subtle)
			got := selectedJobLogLine("go test ./...", 40, tc.th.SelectedBackground, faint)

			if plain := xansi.Strip(got); !strings.HasPrefix(plain, "go test ./...") {
				t.Errorf("the line reads %q, want the log line it was given", plain)
			}
			if tc.th.SelectedBackground == nil && strings.Contains(got, "\x1b[48;2;") {
				t.Errorf("a theme with no surface painted a background: %q", got)
			}
		})
	}
}
