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

// A row wider than its pane gets clipped blind: trailing columns vanish
// mid-cell with no ellipsis and the selection background stops short of the
// edge. So the row has to fit at every width, not just roomy ones.
func TestEveryLineFillsThePaneWidthAtEveryWidth(t *testing.T) {
	// The fixed columns need 13 cells between them, so the last few widths are
	// past the point where anything can be dropped and the row has to clip.
	for _, width := range []int{200, 140, 100, 90, 70, 50, 40, 30, 20, 16, 10} {
		t.Run(fmt.Sprintf("%d", width), func(t *testing.T) {
			out := screen(t, width, 10, []gh.PullRequest{
				pr("Fix the auth retry backoff loop"),
				pr("Bump charm deps to v2.0.8"),
			})

			for i, line := range strings.Split(out, "\n") {
				if w := lipgloss.Width(line); w != width {
					t.Errorf("line %d is %d cells wide, want %d\n%s", i, w, width, stripANSI(line))
				}
			}
		})
	}
}

// Under the width the fixed columns need there is nothing left to drop, so the
// row has to clip itself. Letting it run over means the pane cuts it blind: the
// trailing column ends mid-cell with nothing saying it was cut, and the width
// test above passes anyway because the pane still fills its line.
func TestARowTooNarrowForItsColumnsClipsItself(t *testing.T) {
	row := stripANSI(rowContaining(t, screen(t, 16, 6, []gh.PullRequest{pr("Fix the auth retry backoff loop")}), "#412"))

	if !strings.Contains(row, "…") {
		t.Errorf("the row was cut with nothing saying so\n%q", row)
	}
}

// Columns drop in a fixed order rather than overflowing. Author is the widest
// thing usually identical across a section, and age is the cheapest to keep.
func TestColumnsDropInOrderAsTheTerminalNarrows(t *testing.T) {
	tests := []struct {
		width             int
		repo, author, age bool
	}{
		{width: 140, repo: true, author: true, age: true},
		{width: 74, repo: true, author: false, age: true},
		{width: 55, repo: false, author: false, age: true},
		{width: 34, repo: false, author: false, age: false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d", tt.width), func(t *testing.T) {
			row := stripANSI(rowContaining(t, screen(t, tt.width, 8, []gh.PullRequest{pr("Fix auth retry")}), "#412"))

			if got := strings.Contains(row, "zen-octo/zen-octo"); got != tt.repo {
				t.Errorf("repo column present = %v, want %v\n%s", got, tt.repo, row)
			}
			if got := strings.Contains(row, "drucial"); got != tt.author {
				t.Errorf("author column present = %v, want %v\n%s", got, tt.author, row)
			}
			if got := strings.Contains(row, "2h"); got != tt.age {
				t.Errorf("age column present = %v, want %v\n%s", got, tt.age, row)
			}
			if !strings.Contains(row, "#412") {
				t.Errorf("the number column was dropped, want it always kept\n%s", row)
			}
		})
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
