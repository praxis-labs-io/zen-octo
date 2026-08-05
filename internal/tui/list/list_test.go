package list_test

import (
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/config"
	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/list"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

func sections() []config.Section {
	return []config.Section{{Title: "My PRs", Filters: "is:open is:pr author:@me"}}
}

func newList(width, height int, prs []gh.PullRequest) list.Model {
	m := list.New(theme.RosePineMoon, sections())
	m.SetSize(width, height)
	m.SetPullRequests(prs)
	return m
}

func screen(t *testing.T, width, height int, prs []gh.PullRequest) string {
	t.Helper()
	return newList(width, height, prs).View()
}

func press(m list.Model, keys ...tea.KeyPressMsg) list.Model {
	for _, k := range keys {
		m, _ = m.Update(k)
	}
	return m
}

func key(r rune) tea.KeyPressMsg  { return tea.KeyPressMsg{Code: r, Text: string(r)} }
func ctrl(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl} }

// fixtureTime is read once, so rows built moments apart do not sort by how long
// the test took to make them. Ties are what let a fixture state its own order.
var fixtureTime = time.Now().Add(-2 * time.Hour)

func pr(title string) gh.PullRequest {
	return gh.PullRequest{
		ID: "PR_" + title, Number: 412, Title: title, Repository: "zen-octo/zen-octo",
		Author: gh.Actor{Login: "drucial"}, State: gh.PRStateOpen,
		Additions: 42, Deletions: 7, ChangedFiles: 3, Comments: 6,
		Checks: gh.CheckStateSuccess, ReviewDecision: gh.ReviewDecisionApproved,
		UpdatedAt: fixtureTime,
	}
}

// numbered builds a run of ready pull requests in a known order: one repo, one
// timestamp, so nothing in the sort can reorder them.
func numbered(n int) []gh.PullRequest {
	prs := make([]gh.PullRequest, n)
	for i := range prs {
		prs[i] = pr(fmt.Sprintf("Change %d", i))
		prs[i].ID = fmt.Sprintf("PR_%d", i)
		prs[i].Number = i
	}
	return prs
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

// The count glyphs, checked by shape because that is what the row shows.
const (
	fileGlyph    = "\uea7b" // nf-cod-file
	commentGlyph = "\uf41f" // nf-oct-comment
)

// selectionSeq is the SGR sequence that sets the selection background.
func selectionSeq() string {
	r, g, b, _ := theme.RosePineMoon.SelectedBackground.RGBA()
	return fmt.Sprintf("48;2;%d;%d;%d", r>>8, g>>8, b>>8)
}

func fgSeq(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("38;2;%d;%d;%d", r>>8, g>>8, b>>8)
}

// selectedRow returns every line painted with the selection background. A row
// is two lines now, and the number lives on the second, so a helper returning
// the first would answer half the question.
func selectedRow(t *testing.T, frame string) string {
	t.Helper()

	var out []string
	for _, line := range strings.Split(frame, "\n") {
		if strings.Contains(line, selectionSeq()) {
			out = append(out, line)
		}
	}
	if len(out) == 0 {
		t.Fatal("no line in the frame carries the selection background")
	}
	return strings.Join(out, "\n")
}

func TestRowsGroupByStateWithAHeaderOverEach(t *testing.T) {
	closed := pr("Probe libghostty binds")
	closed.State = gh.PRStateClosed
	merged := pr("Theme registry")
	merged.State = gh.PRStateMerged
	draft := pr("Bump charm deps")
	draft.IsDraft = true

	// Deliberately out of order: the arrangement is the thing under test.
	out := stripANSI(screen(t, 120, 24, []gh.PullRequest{closed, pr("Fix auth retry"), merged, draft}))

	at := map[string]int{}
	for _, label := range []string{"Ready", "Draft", "Merged", "Closed"} {
		at[label] = strings.Index(out, "─ "+label+" 1")
		if at[label] < 0 {
			t.Fatalf("no header for the %s group\n%s", label, out)
		}
	}

	if at["Ready"] > at["Draft"] || at["Draft"] > at["Merged"] || at["Merged"] > at["Closed"] {
		t.Errorf("headers sit at %v, want ready, draft, merged, closed", at)
	}
}

// Groups need air between them. Above the first one it would only push the
// list down a row, so that one opens the pane.
func TestEveryGroupButTheFirstOpensWithABlankLine(t *testing.T) {
	draft := pr("Bump charm deps")
	draft.ID, draft.IsDraft = "PR_draft", true

	// The pane's top border is line zero, so the first header lands on line one.
	lines := strings.Split(stripANSI(screen(t, 120, 24, []gh.PullRequest{pr("Fix auth retry"), draft})), "\n")
	if !strings.Contains(lines[1], "─ Ready 1") {
		t.Fatalf("the first group does not open the pane: %q", lines[1])
	}

	for i, l := range lines {
		if !strings.Contains(l, "─ Draft 1") {
			continue
		}
		if above := strings.Trim(lines[i-1], "│ "); above != "" {
			t.Errorf("the draft group has no blank line above it: %q", lines[i-1])
		}
		return
	}
	t.Fatal("no draft header in the frame")
}

// One pane means there is no focus to report, so the border never takes the
// accent that says "your keys reach this one".
func TestTheBorderNeverReadsAsFocused(t *testing.T) {
	top := strings.Split(screen(t, 120, 10, []gh.PullRequest{pr("Fix auth retry")}), "\n")[0]

	end := strings.Index(top, "m")
	if !strings.HasPrefix(top, "\x1b[") || end < 0 {
		t.Fatalf("the frame does not open with a styled border: %q", top)
	}
	if got, want := top[2:end], fgSeq(theme.RosePineMoon.BorderSecondary); got != want {
		t.Errorf("the border opens as %s, want the idle colour %s", got, want)
	}
}

// A draft that was closed is closed. Grouping it as a draft puts abandoned work
// above merged work, which is the wrong way round.
func TestAClosedDraftGroupsAsClosed(t *testing.T) {
	p := pr("Abandoned experiment")
	p.State, p.IsDraft = gh.PRStateClosed, true

	out := stripANSI(screen(t, 120, 20, []gh.PullRequest{p}))

	if !strings.Contains(out, "─ Closed 1") {
		t.Errorf("a closed draft did not land in the closed group\n%s", out)
	}
	if strings.Contains(out, "─ Draft") {
		t.Error("a closed draft opened a draft group")
	}
}

func TestRepositoriesStayTogetherNewestFirstInsideAGroup(t *testing.T) {
	build := func(title, repo string, age time.Duration) gh.PullRequest {
		p := pr(title)
		p.Repository, p.UpdatedAt = repo, time.Now().Add(-age)
		return p
	}

	out := stripANSI(screen(t, 140, 24, []gh.PullRequest{
		build("Older alpha", "alpha/one", 5*time.Hour),
		build("Only beta", "beta/two", time.Hour),
		build("Newer alpha", "alpha/one", time.Hour),
	}))

	want := []string{"Newer alpha", "Older alpha", "Only beta"}
	at := make([]int, len(want))
	for i, title := range want {
		if at[i] = strings.Index(out, title); at[i] < 0 {
			t.Fatalf("%q is not in the frame\n%s", title, out)
		}
	}
	if at[0] > at[1] || at[1] > at[2] {
		t.Errorf("titles sit at %v, want %v: grouped by repo, newest first inside it", at, want)
	}
}

// Style.Width wraps before it clips, so a title longer than its column used to
// become a third line and push everything below it down.
func TestALongTitleTruncatesRatherThanWrapping(t *testing.T) {
	long := strings.Repeat("a very long pull request title ", 12)

	out := screen(t, 120, 12, []gh.PullRequest{pr(long)})

	if rows := strings.Count(out, "#412"); rows != 1 {
		t.Errorf("the pull request occupies %d rows, want 1", rows)
	}
	if !strings.Contains(out, "…") {
		t.Error("a clipped title does not say it was clipped")
	}
	if lines := strings.Split(selectedRow(t, out), "\n"); len(lines) != 2 {
		t.Errorf("the row is %d lines, want 2", len(lines))
	}
}

// The cursor indexes the selectable rows, so a header is not something it can
// land on and then have to move off.
func TestMovingDownCrossesAGroupHeaderWithoutStoppingOnIt(t *testing.T) {
	draft := pr("Bump charm deps")
	draft.ID, draft.IsDraft = "PR_draft", true

	m := press(newList(120, 24, []gh.PullRequest{pr("Fix auth retry"), draft}), key('j'))

	row := stripANSI(selectedRow(t, m.View()))
	if !strings.Contains(row, "Bump charm deps") {
		t.Errorf("j across the group boundary selected %q, want the draft", row)
	}
	if strings.Contains(row, "Draft 1") {
		t.Error("the selection landed on the group header")
	}
	if !strings.Contains(stripANSI(m.View()), "2 of 2") {
		t.Error("the counter counts headers as rows")
	}
}

// The selection has to be baked into every cell of both lines. Wrapping a
// joined line paints only its first cell: each one ends in a full SGR reset,
// which clears the background along with the foreground.
func TestTheSelectedRowIsPaintedCellByCellOnBothLines(t *testing.T) {
	out := screen(t, 140, 14, []gh.PullRequest{pr("Fix auth retry"), pr("Bump deps")})

	lines := strings.Split(selectedRow(t, out), "\n")
	if len(lines) != 2 {
		t.Fatalf("the selection covers %d lines, want both lines of the row", len(lines))
	}
	for i, line := range lines {
		if got := strings.Count(line, selectionSeq()); got < 5 {
			t.Errorf("line %d carries the selection background %d times, want one per cell (>=5)\n%q", i, got, line)
		}
	}
	if !strings.Contains(lines[0], "Fix auth retry") {
		t.Errorf("the selection is not on the first row\n%q", lines[0])
	}
}

// A line wider than its pane gets clipped blind: trailing columns vanish
// mid-cell with no ellipsis and the selection background stops short of the
// edge. So both lines have to fit at every width, not just roomy ones.
func TestEveryLineFillsThePaneWidthAtEveryWidth(t *testing.T) {
	// The fixed columns need 19 cells between them, so the last few widths are
	// past the point where anything can be dropped and the line has to clip.
	for _, width := range []int{200, 140, 100, 90, 70, 50, 40, 30, 20, 16, 10} {
		t.Run(fmt.Sprintf("%d", width), func(t *testing.T) {
			out := screen(t, width, 12, []gh.PullRequest{
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
// line has to clip itself. Letting it run over means the pane cuts it blind:
// the trailing column ends mid-cell with nothing saying it was cut, and the
// width test above passes anyway because the pane still fills its line.
func TestARowTooNarrowForItsColumnsClipsItself(t *testing.T) {
	row := stripANSI(selectedRow(t, screen(t, 14, 8, []gh.PullRequest{pr("Fix the auth retry backoff loop")})))

	if !strings.Contains(row, "…") {
		t.Errorf("the row was cut with nothing saying so\n%q", row)
	}
}

// Past the width even the fixed columns need, a line has to cut itself and say
// so. Letting it run over hands the job to the pane, which ends it mid-cell
// with no mark, and the width test above passes either way because the pane
// fills its line regardless.
func TestALineTooNarrowForItsFixedColumnsSaysItWasCut(t *testing.T) {
	row := stripANSI(selectedRow(t, screen(t, 8, 8, []gh.PullRequest{pr("Fix the auth retry backoff loop")})))

	for i, l := range strings.Split(row, "\n") {
		if !strings.Contains(l, "…") {
			t.Errorf("line %d was cut with nothing saying so: %q", i, l)
		}
	}
}

// Columns drop in a fixed order rather than overflowing. The first line gives
// up review before age; the second drops the comment count, then the files,
// then the churn, and the identity sheds the author before the repository is
// left to clip. The number is on the first line and never goes.
func TestColumnsDropInOrderAsTheTerminalNarrows(t *testing.T) {
	tests := []struct {
		width                         int
		author, diff, files, comments bool
		review, age                   bool
	}{
		{width: 140, author: true, diff: true, files: true, comments: true, review: true, age: true},
		{width: 58, author: false, diff: true, files: true, comments: true, review: true, age: true},
		{width: 36, author: false, diff: true, files: true, comments: false, review: false, age: true},
		{width: 30, author: false, diff: true, files: false, comments: false, review: false, age: false},
		{width: 16, author: false, diff: false, files: false, comments: false, review: false, age: false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d", tt.width), func(t *testing.T) {
			out := stripANSI(screen(t, tt.width, 10, []gh.PullRequest{pr("Fix auth")}))

			for _, col := range []struct {
				name, text string
				want       bool
			}{
				{name: "author", text: "by @drucial", want: tt.author},
				{name: "additions", text: "+42", want: tt.diff},
				{name: "deletions", text: "−7", want: tt.diff},
				{name: "file count", text: fileGlyph, want: tt.files},
				{name: "comment count", text: commentGlyph, want: tt.comments},
				{name: "review", text: "✔", want: tt.review},
				{name: "age", text: "2h", want: tt.age},
			} {
				if got := strings.Contains(out, col.text); got != col.want {
					t.Errorf("%s column present = %v, want %v\n%s", col.name, got, col.want, out)
				}
			}
			if !strings.Contains(out, "#412") {
				t.Errorf("the number column was dropped, want it always kept\n%s", out)
			}
		})
	}
}

// The repository, number and author read as one phrase. Laying them out as
// three columns leaves gaps you have to jump, which is what gh-dash gets right.
func TestTheIdentityReadsAsOnePhrase(t *testing.T) {
	lines := strings.Split(stripANSI(selectedRow(t, screen(t, 140, 10, []gh.PullRequest{pr("Fix auth retry")}))), "\n")

	if !strings.Contains(lines[0], "#412") {
		t.Errorf("the number is not on the title line\n%q", lines[0])
	}
	if !strings.Contains(lines[1], "zen-octo/zen-octo by @drucial") {
		t.Errorf("the identity is spread across columns\n%q", lines[1])
	}
}

// Author is nil on GitHub once an account is deleted, so the login can be empty
// on a real pull request. A dangling "by @" is worse than no attribution.
func TestADeletedAuthorDropsTheWholeClause(t *testing.T) {
	p := pr("Fix auth retry")
	p.Author = gh.Actor{}

	row := stripANSI(rowContaining(t, screen(t, 140, 10, []gh.PullRequest{p}), "zen-octo/zen-octo"))

	if strings.Contains(row, "by ") || strings.Contains(row, "@") {
		t.Errorf("a deleted author still leaves an attribution\n%q", row)
	}
	if !strings.Contains(row, "zen-octo/zen-octo") {
		t.Errorf("the rest of the identity went with the author\n%q", row)
	}
}

// Additions and deletions carry their own colour, which one cell cannot do: a
// cell is one style all the way through.
func TestTheChurnColoursAdditionsAndDeletionsApart(t *testing.T) {
	second := pr("Bump deps")
	second.Number, second.Additions, second.Deletions = 408, 11, 3
	second.Author = gh.Actor{Login: "octobot"}

	// The row the cursor is not on. An unselected cell carries the foreground
	// alone, so the sequence names a colour with no background spliced into it.
	row := rowContaining(t, screen(t, 140, 14, []gh.PullRequest{pr("Fix auth retry"), second}), "@octobot")

	if got := styleOf(t, row, "+11"); !strings.Contains(got, fgSeq(theme.RosePineMoon.Success)) {
		t.Errorf("additions render as %s, want the success colour", got)
	}
	if got := styleOf(t, row, "−3"); !strings.Contains(got, fgSeq(theme.RosePineMoon.Error)) {
		t.Errorf("deletions render as %s, want the error colour", got)
	}
}

// styleOf is the SGR parameters of the styled run carrying want. Matching the
// sequence and the text as one string breaks the moment a cell pads.
func styleOf(t *testing.T, row, want string) string {
	t.Helper()

	for _, run := range strings.Split(row, "\x1b[") {
		if end := strings.Index(run, "m"); end >= 0 && strings.Contains(run[end+1:], want) {
			return run[:end]
		}
	}
	t.Fatalf("no styled run carries %q\n%q", want, row)
	return ""
}

// Review sits next to the check rollup, so the two have to be told apart by
// shape. Colour alone stops working the moment they disagree.
func TestTheReviewGlyphTellsTheDecisionsApart(t *testing.T) {
	tests := []struct {
		decision gh.ReviewDecision
		want     string
	}{
		{decision: gh.ReviewDecisionApproved, want: "✔"},
		{decision: gh.ReviewDecisionChangesRequested, want: "✎"},
		{decision: gh.ReviewDecisionReviewRequired, want: "◇"},
	}

	seen := map[string]bool{}
	for _, tt := range tests {
		p := pr("Fix auth retry")
		p.ReviewDecision = tt.decision

		out := stripANSI(screen(t, 140, 10, []gh.PullRequest{p}))
		if !strings.Contains(out, tt.want) {
			t.Errorf("%s renders without %q\n%s", tt.decision, tt.want, out)
		}
		if seen[tt.want] {
			t.Errorf("%s reuses a glyph another decision already has", tt.decision)
		}
		seen[tt.want] = true
	}

	none := pr("Fix auth retry")
	none.ReviewDecision = gh.ReviewDecisionNone
	for _, glyph := range []string{"✔", "✎", "◇"} {
		if strings.Contains(stripANSI(screen(t, 140, 10, []gh.PullRequest{none})), glyph) {
			t.Errorf("a pull request needing no review still shows %q", glyph)
		}
	}
}

// The title stops growing so the status glyphs stay put. Without the cap a wide
// terminal leaves a hundred columns of empty title cell before the checks.
func TestTheStatusGlyphsHoldTheirPlaceOnAWideTerminal(t *testing.T) {
	offsets := map[int]int{}

	for _, width := range []int{120, 160, 200} {
		row := stripANSI(rowContaining(t, screen(t, width, 10, []gh.PullRequest{pr("Fix auth retry")}), "Fix auth retry"))
		offsets[width] = len([]rune(row)) - len([]rune(row[:strings.Index(row, "2h")]))
	}

	if offsets[120] != offsets[160] || offsets[160] != offsets[200] {
		t.Errorf("the age column sits %v cells from the right edge, want the same at every width", offsets)
	}
}

// viewport.EnsureVisible acts only once a line is already outside the window
// and then puts it on the top row, so one press down scrolls a whole page and
// the next nine move nothing. Variable row heights make the arithmetic that
// replaces it easy to get wrong in the other direction too.
func TestWalkingTheListKeepsEverySelectionOnScreen(t *testing.T) {
	prs := numbered(20)
	m := newList(120, 14, prs)

	for i := range prs {
		if i > 0 {
			m = press(m, key('j'))
		}
		if got := stripANSI(selectedRow(t, m.View())); !strings.Contains(got, fmt.Sprintf("#%d ", i)) {
			t.Fatalf("after %d presses the visible selection is %q, want #%d", i, got, i)
		}
	}

	// And back up. Scrolling down leaves the window below every earlier row, so
	// this is the direction the offset arithmetic gets wrong on its own.
	for i := len(prs) - 2; i >= 0; i-- {
		m = press(m, key('k'))
		if got := stripANSI(selectedRow(t, m.View())); !strings.Contains(got, fmt.Sprintf("#%d ", i)) {
			t.Fatalf("walking back up, the visible selection is %q, want #%d", got, i)
		}
	}
}

// The window never opens on a row's second line with its title scrolled off
// above it, which is what plain offset arithmetic does as soon as a row is
// taller than one line.
func TestTheWindowNeverOpensMidRow(t *testing.T) {
	// An odd number of content lines, or the offsets land on row boundaries by
	// arithmetic and the alignment never has to do anything.
	m := newList(120, 13, numbered(20))
	for range 12 {
		m = press(m, key('j'))
	}

	// The repository only appears on a row's second line, so finding it on the
	// first line inside the pane means a row was cut in half by the window.
	body := strings.Split(stripANSI(m.View()), "\n")[1]
	if strings.Contains(body, "zen-octo/zen-octo") {
		t.Errorf("the window opens on a row's second line: %q", body)
	}
}

func TestPageKeysLandOnAPullRequestAndStopAtTheEnds(t *testing.T) {
	prs := numbered(20)

	tests := []struct {
		name string
		keys []tea.KeyPressMsg
		want string
	}{
		{name: "page down", keys: []tea.KeyPressMsg{ctrl('f')}, want: "#6 "},
		{name: "page down then up returns", keys: []tea.KeyPressMsg{ctrl('f'), ctrl('b')}, want: "#0 "},
		{name: "half page down", keys: []tea.KeyPressMsg{ctrl('d')}, want: "#3 "},
		{name: "page down past the end clamps", keys: []tea.KeyPressMsg{ctrl('f'), ctrl('f'), ctrl('f'), ctrl('f'), ctrl('f')}, want: "#19 "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripANSI(selectedRow(t, press(newList(120, 14, prs), tt.keys...).View()))

			if !strings.Contains(got, tt.want) {
				t.Errorf("selection = %q, want %s", got, tt.want)
			}
			if strings.Contains(got, "─ Ready") {
				t.Errorf("the selection landed on a group header: %q", got)
			}
		})
	}
}

func TestTheFooterCountsPullRequestsNotHeaders(t *testing.T) {
	merged := pr("three")
	merged.State = gh.PRStateMerged

	out := stripANSI(screen(t, 120, 16, []gh.PullRequest{pr("one"), pr("two"), merged}))

	if !strings.Contains(out, "1 of 3") {
		t.Errorf("the bottom border does not carry the position and count\n%s", out)
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
