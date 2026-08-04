package prview_test

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/prview"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

func samplePR() gh.PullRequest {
	return gh.PullRequest{
		ID: "PR_412", Number: 412, Title: "Fix the auth retry backoff loop",
		Repository: "zen-octo/zen-octo", Author: gh.Actor{Login: "drucial"},
		State: gh.PRStateOpen, BaseRefName: "main", HeadRefName: "fix-auth-retry",
		Additions: 42, Deletions: 7, ChangedFiles: 3,
		Checks: gh.CheckStateFailure, ReviewDecision: gh.ReviewDecisionChangesRequested,
	}
}

func screen(width, height int) prview.Model {
	m := prview.New(theme.RosePineMoon, samplePR())
	m.SetSize(width, height)
	return m
}

func press(m prview.Model, keys ...string) prview.Model {
	for _, k := range keys {
		m, _ = m.Update(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
	}
	return m
}

func fgSeq(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("38;2;%d;%d;%d", r>>8, g>>8, b>>8)
}

func TestTheFrameFillsItsSizeExactly(t *testing.T) {
	sizes := []struct{ width, height int }{
		{width: 200, height: 40},
		{width: 160, height: 24},
		{width: 100, height: 20},
		{width: 60, height: 10},
	}

	for _, size := range sizes {
		name := fmt.Sprintf("%dx%d", size.width, size.height)
		t.Run(name, func(t *testing.T) {
			lines := strings.Split(screen(size.width, size.height).View(), "\n")

			if len(lines) != size.height {
				t.Errorf("frame is %d lines, want %d", len(lines), size.height)
			}
			for i, line := range lines {
				if w := lipgloss.Width(line); w != size.width {
					t.Errorf("line %d is %d cells wide, want %d", i, w, size.width)
				}
			}
		})
	}
}

func TestTabsSwitchAndOnlyOneReadsAsCurrent(t *testing.T) {
	m := screen(160, 24)

	active := fgSeq(theme.RosePineMoon.Primary)
	if top := firstLine(m.View()); !strings.Contains(top, active+"mConversation") {
		t.Error("Conversation is not the current tab on open")
	}

	next := press(m, "]")
	top := firstLine(next.View())
	if !strings.Contains(top, active+"mCommits") {
		t.Error("] did not move to the Commits tab")
	}
	if strings.Contains(top, active+"mConversation") {
		t.Error("Conversation still reads as current after switching")
	}
	if !strings.Contains(next.View(), "Commits arrive with the full pull request query.") {
		t.Error("the body did not follow the tab")
	}

	// Four tabs, so [ from the first wraps round to the last.
	if !strings.Contains(firstLine(press(m, "[").View()), active+"mFiles") {
		t.Error("[ from the first tab did not wrap to the last")
	}
}

func TestEscapeAsksToGoBack(t *testing.T) {
	_, cmd := screen(160, 24).Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("escape produced no command, want a request to go back")
	}
	if _, ok := cmd().(prview.BackMsg); !ok {
		t.Errorf("escape produced %T, want a BackMsg", cmd())
	}
}

func TestTheRailCarriesWhatIsKnownAndOmitsTheRest(t *testing.T) {
	out := screen(200, 30).View()

	for _, want := range []string{"State", "Checks", "failing", "Review", "changes requested", "Branch", "main", "Changes", "3 files"} {
		if !strings.Contains(out, want) {
			t.Errorf("rail is missing %q", want)
		}
	}

	// Nothing fetches labels or assignees yet, so those headings must not
	// appear as empty sections.
	for _, absent := range []string{"Labels", "Assignee", "Milestone"} {
		if strings.Contains(out, absent) {
			t.Errorf("rail shows a %q section with nothing behind it", absent)
		}
	}
}

// Focus is only visible in the border color, so that is what these assert on.
// The conversation pane's own corner opens the frame, which makes it the one
// unambiguous place to read it.
func TestFocusMovesBetweenThePanes(t *testing.T) {
	var (
		focused = fgSeq(theme.RosePineMoon.Secondary)
		idle    = fgSeq(theme.RosePineMoon.BorderSecondary)
	)

	m := screen(200, 30)
	if got := conversationBorder(t, m.View()); got != focused {
		t.Fatalf("conversation border = %s on open, want the focused accent", got)
	}

	rail := press(m, "l")
	if got := conversationBorder(t, rail.View()); got != idle {
		t.Errorf("conversation border = %s after l, want it to recede", got)
	}

	if got := conversationBorder(t, press(rail, "h").View()); got != focused {
		t.Errorf("conversation border = %s after h, want focus back on the left pane", got)
	}
	if got := conversationBorder(t, press(rail, "1").View()); got != focused {
		t.Errorf("conversation border = %s after 1, want focus jumped straight back", got)
	}
}

func TestFocusLeavesTheRailWhenTheRailDoes(t *testing.T) {
	hidden := press(screen(200, 30), "l", "d") // focus the rail, then hide it

	if got := conversationBorder(t, hidden.View()); got != fgSeq(theme.RosePineMoon.Secondary) {
		t.Errorf("conversation border = %s, want focus back on it once the rail went away", got)
	}
}

func firstLine(s string) string { return strings.Split(s, "\n")[0] }

// conversationBorder reads the SGR foreground the frame opens with, which is
// the conversation pane's top-left corner.
func conversationBorder(t *testing.T, frame string) string {
	t.Helper()

	line := firstLine(frame)
	end := strings.Index(line, "m")
	if !strings.HasPrefix(line, "\x1b[") || end < 0 {
		t.Fatalf("frame does not open with a styled border: %q", line)
	}
	return strings.TrimPrefix(line[:end], "\x1b[")
}
