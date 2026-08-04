package list_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/config"
	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/list"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

func sections() []config.Section {
	return []config.Section{{Title: "My PRs", Filters: "is:open is:pr author:@me"}}
}

func screen(t *testing.T, width, height int, prs []gh.PullRequest) string {
	t.Helper()

	m := list.New(theme.RosePineMoon, sections())
	m.SetSize(width, height)
	m.SetPullRequests(prs)
	return m.View()
}

func pr(title string) gh.PullRequest {
	return gh.PullRequest{
		ID: "PR_" + title, Number: 412, Title: title, Repository: "zen-octo/zen-octo",
		Author: gh.Actor{Login: "drucial"}, State: gh.PRStateOpen,
		Checks: gh.CheckStateSuccess, UpdatedAt: time.Now().Add(-2 * time.Hour),
	}
}

func rowContaining(t *testing.T, frame, want string) string {
	t.Helper()

	for _, line := range strings.Split(frame, "\n") {
		if strings.Contains(line, want) {
			return line
		}
	}
	t.Fatalf("no rendered line contains %q", want)
	return ""
}

// Style.Width wraps before it clips, so a title longer than its column used to
// become a second row and push everything below it down.
func TestALongTitleTruncatesRatherThanWrapping(t *testing.T) {
	long := strings.Repeat("a very long pull request title ", 12)

	out := screen(t, 120, 12, []gh.PullRequest{pr(long)})

	rows := strings.Count(out, "#412")
	if rows != 1 {
		t.Errorf("the pull request occupies %d rows, want 1", rows)
	}
	if !strings.Contains(out, "…") {
		t.Error("a clipped title does not say it was clipped")
	}
}

// The title column stops growing so the columns after it stay put. Without the
// cap a wide terminal leaves a hundred columns of empty title cell.
func TestTheColumnsAfterTheTitleHoldTheirPlaceOnAWideTerminal(t *testing.T) {
	offsets := map[int]int{}

	for _, width := range []int{120, 160, 200} {
		row := stripANSI(rowContaining(t, screen(t, width, 8, []gh.PullRequest{pr("Fix auth retry")}), "#412"))

		idx := strings.Index(row, "drucial")
		offsets[width] = len([]rune(row)) - len([]rune(row[:idx]))
	}

	if offsets[120] != offsets[160] || offsets[160] != offsets[200] {
		t.Errorf("the author column sits %v cells from the right edge, want the same at every width", offsets)
	}
}

func TestEveryLineFillsThePaneWidth(t *testing.T) {
	const width = 140
	out := screen(t, width, 10, []gh.PullRequest{pr("Fix auth retry"), pr("Bump deps")})

	for i, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w != width {
			t.Errorf("line %d is %d cells wide, want %d", i, w, width)
		}
	}
}

// The selection has to be baked into every cell. Wrapping the joined row paints
// only the first: each cell ends in a full SGR reset, which clears the
// background along with the foreground.
func TestTheSelectedRowIsPaintedCellByCell(t *testing.T) {
	out := screen(t, 140, 10, []gh.PullRequest{pr("Fix auth retry"), pr("Bump deps")})

	r, g, b, _ := theme.RosePineMoon.SelectedBackground.RGBA()
	seq := fmt.Sprintf("48;2;%d;%d;%d", r>>8, g>>8, b>>8)

	row := rowContaining(t, out, seq)
	if got := strings.Count(row, seq); got < 7 {
		t.Errorf("selection background appears %d times, want one per cell (>=7)\n%q", got, row)
	}
	if !strings.Contains(row, "Fix auth retry") {
		t.Errorf("the selection is not on the first row\n%q", row)
	}
}

func TestTheFooterCountsTheRows(t *testing.T) {
	out := screen(t, 120, 10, []gh.PullRequest{pr("one"), pr("two"), pr("three")})

	if !strings.Contains(stripANSI(out), "1 of 3") {
		t.Error("the bottom border does not carry the position and count")
	}
}

func TestAnEmptySectionSaysSoRatherThanShowingNothing(t *testing.T) {
	out := screen(t, 120, 10, nil)

	if !strings.Contains(out, "Nothing matches this section.") {
		t.Error("an empty section renders as a blank pane")
	}
	if strings.Contains(stripANSI(out), " of ") {
		t.Error("an empty section still shows a row counter")
	}
}

// stripANSI drops SGR sequences so a test can reason about layout positions.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
