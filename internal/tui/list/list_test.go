package list_test

import (
	"errors"
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/config"
	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/list"
)

func ready(titles []string, prs ...[]gh.PullRequest) []store.Section {
	sections := make([]store.Section, len(titles))
	for i, title := range titles {
		sections[i] = store.Section{
			Section: config.Section{Title: title, Filters: "is:open is:pr author:@me"},
			PRs:     prs[i],
			Status:  store.StatusReady,
			Loaded:  true,
		}
	}
	return sections
}

func newList(width, height int, prs []gh.PullRequest) list.Model {
	m := list.New(testTheme)
	m.SetSize(width, height)
	m.SetSections(ready([]string{"My PRs"}, prs))
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

var fixtureTime = time.Now().Add(-2 * time.Hour)

func pr(title string) gh.PullRequest {
	return gh.PullRequest{
		ID: "PR_" + title, Number: 412, Title: title, Repository: "acme/rocket",
		URL:    "https://github.com/acme/rocket/pull/412",
		Author: gh.Actor{Login: "drucial"}, State: gh.PRStateOpen,
		Additions: 42, Deletions: 7, ChangedFiles: 3, Comments: 6,
		Checks: gh.CheckStateSuccess, ReviewDecision: gh.ReviewDecisionApproved,
		UpdatedAt: fixtureTime,
	}
}

func numbered(n int) []gh.PullRequest {
	prs := make([]gh.PullRequest, n)
	for i := range prs {
		prs[i] = pr(fmt.Sprintf("Change %d", i))
		prs[i].ID = fmt.Sprintf("PR_%d", i)
		prs[i].Number = i
		prs[i].URL = fmt.Sprintf("https://github.com/acme/rocket/pull/%d", i)
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

const (
	fileGlyph    = "\uea7b"
	commentGlyph = "\uf41f"
	reviewGlyph  = "\uedc6"
	checksGlyph  = "\uf0ae"
)

func selectionSeq() string {
	return sgrParams(lipgloss.NewStyle().Background(testTheme.SelectedBackground))
}

func fgSeq(c color.Color) string { return sgrParams(lipgloss.NewStyle().Foreground(c)) }

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

	out := stripANSI(screen(t, 120, 24, []gh.PullRequest{closed, pr("Fix auth retry"), merged, draft}))

	at := map[string]int{}
	for _, label := range []string{"Ready", "Draft", "Merged", "Closed"} {
		at[label] = strings.Index(out, "─ "+label+" (1)")
		if at[label] < 0 {
			t.Fatalf("no header for the %s group\n%s", label, out)
		}
	}

	if at["Ready"] > at["Draft"] || at["Draft"] > at["Merged"] || at["Merged"] > at["Closed"] {
		t.Errorf("headers sit at %v, want ready, draft, merged, closed", at)
	}
}

func TestTheFirstGroupTakesAThinnerGapThanTheRest(t *testing.T) {
	draft := pr("Bump charm deps")
	draft.ID, draft.IsDraft = "PR_draft", true

	lines := strings.Split(stripANSI(screen(t, 120, 24, []gh.PullRequest{pr("Fix auth retry"), draft})), "\n")

	if gap := strings.Trim(lines[1], "│ "); gap != "" {
		t.Errorf("no line above the first group: %q", lines[1])
	}
	if !strings.Contains(lines[2], "─ Ready (1)") {
		t.Errorf("the gap above the first group is more than a line: %q", lines[2])
	}

	for i, l := range lines {
		if !strings.Contains(l, "─ Draft (1)") {
			continue
		}
		for n := 1; n <= 2; n++ {
			if above := strings.Trim(lines[i-n], "│ "); above != "" {
				t.Errorf("line %d above the draft group is not blank: %q", n, lines[i-n])
			}
		}
		return
	}
	t.Fatal("no draft header in the frame")
}

func TestRowsAreOneLineApartAndGroupsAreMore(t *testing.T) {
	last := pr("Bump deps")
	last.ID = "PR_bump"
	draft := pr("Comment anchoring")
	draft.ID, draft.IsDraft = "PR_draft", true

	lines := strings.Split(stripANSI(screen(t, 140, 24, []gh.PullRequest{pr("Fix auth retry"), last, draft})), "\n")

	at := func(title string, offset int) string {
		for i, l := range lines {
			if strings.Contains(l, title) {
				return strings.Trim(lines[i+offset], "│ ")
			}
		}
		t.Fatalf("no row titled %q", title)
		return ""
	}

	if gap := at("Fix auth retry", 2); gap != "" {
		t.Errorf("no blank line under the first row: %q", gap)
	}
	if row := at("Fix auth retry", 3); !strings.Contains(row, "Bump deps") {
		t.Errorf("rows are more than a line apart: %q", row)
	}

	for _, offset := range []int{2, 3} {
		if gap := at("Bump deps", offset); gap != "" {
			t.Errorf("line %d of the group gap is not blank: %q", offset-1, gap)
		}
	}
	if header := at("Bump deps", 4); !strings.HasPrefix(header, "─ Draft") {
		t.Errorf("the group gap is not two lines: %q", header)
	}
}

func TestReturningToTheTopBringsTheFirstHeaderBack(t *testing.T) {
	walked := newList(120, 13, numbered(20))
	for range 19 {
		walked = press(walked, key('j'))
	}
	for range 19 {
		walked = press(walked, key('k'))
	}

	for name, m := range map[string]list.Model{
		"g from the bottom": press(newList(120, 13, numbered(20)), key('G'), key('g')),
		"walked back up":    walked,
	} {
		lines := strings.Split(stripANSI(m.View()), "\n")

		if gap := strings.Trim(lines[1], "│ "); gap != "" {
			t.Errorf("%s: the line above the first group did not come back: %q", name, lines[1])
		}
		if !strings.Contains(lines[2], "─ Ready") {
			t.Errorf("%s: the first group's header did not come back: %q", name, lines[2])
		}
	}
}

func TestScrollingToAGroupsFirstRowShowsItsHeader(t *testing.T) {
	prs := numbered(12)
	for i := range prs[6:] {
		prs[6+i].IsDraft = true
	}

	m := newList(120, 13, prs)
	for range 11 {
		m = press(m, key('j'))
	}
	for range 5 {
		m = press(m, key('k'))
	}

	out := stripANSI(m.View())
	if !strings.Contains(out, "─ Draft") {
		t.Errorf("the draft header is not on screen with its first row\n%s", out)
	}
}

func TestTheStatusPairClosesTheSecondLine(t *testing.T) {
	row := stripANSI(rowContaining(t, screen(t, 140, 12, []gh.PullRequest{pr("Fix auth retry")}), "acme/rocket"))

	if inner := strings.TrimRight(strings.Trim(row, "│"), " "); !strings.HasSuffix(inner, fileGlyph+"  ● "+reviewGlyph+" ● "+checksGlyph) {
		t.Errorf("the second line does not close with the file count and the status pair: %q", inner)
	}
}

func TestTheBorderNeverReadsAsFocused(t *testing.T) {
	top := strings.Split(screen(t, 120, 10, []gh.PullRequest{pr("Fix auth retry")}), "\n")[0]

	end := strings.Index(top, "m")
	if !strings.HasPrefix(top, "\x1b[") || end < 0 {
		t.Fatalf("the frame does not open with a styled border: %q", top)
	}
	if got, want := top[2:end], fgSeq(testTheme.BorderSubtle); got != want {
		t.Errorf("the border opens as %s, want the idle colour %s", got, want)
	}
}

func TestAClosedDraftGroupsAsClosed(t *testing.T) {
	p := pr("Abandoned experiment")
	p.State, p.IsDraft = gh.PRStateClosed, true

	out := stripANSI(screen(t, 120, 20, []gh.PullRequest{p}))

	if !strings.Contains(out, "─ Closed (1)") {
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

func TestTheSelectionHasNoGaps(t *testing.T) {
	for _, width := range []int{140, 12} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			out := screen(t, width, 12, []gh.PullRequest{pr("Fix auth retry")})

			for i, line := range strings.Split(selectedRow(t, out), "\n") {
				if gap := strings.Trim(unpainted(line), "│"); gap != "" {
					t.Errorf("line %d of the selection has %d unpainted cells (%q)\n%q", i, len(gap), gap, line)
				}
			}
		})
	}
}

func unpainted(line string) string {
	var out strings.Builder
	painted := false

	for i := 0; i < len(line); {
		if line[i] != 0x1b {
			if !painted {
				out.WriteByte(line[i])
			}
			i++
			continue
		}

		j := i
		for j < len(line) && line[j] != 'm' {
			j++
		}
		painted = strings.Contains(line[i:j], selectionSeq())
		i = j + 1
	}
	return out.String()
}

func TestEveryLineFillsThePaneWidthAtEveryWidth(t *testing.T) {
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

func TestARowTooNarrowForItsColumnsClipsItself(t *testing.T) {
	row := stripANSI(selectedRow(t, screen(t, 14, 8, []gh.PullRequest{pr("Fix the auth retry backoff loop")})))

	if !strings.Contains(row, "…") {
		t.Errorf("the row was cut with nothing saying so\n%q", row)
	}
}

func TestALineTooNarrowForItsFixedColumnsSaysItWasCut(t *testing.T) {
	row := stripANSI(selectedRow(t, screen(t, 8, 8, []gh.PullRequest{pr("Fix the auth retry backoff loop")})))

	for i, l := range strings.Split(row, "\n") {
		if !strings.Contains(l, "…") {
			t.Errorf("line %d was cut with nothing saying so: %q", i, l)
		}
	}
}

func TestColumnsDropInOrderAsTheTerminalNarrows(t *testing.T) {
	tests := []struct {
		width                                      int
		author, age, diff, files, comments, status bool
	}{
		{width: 140, author: true, age: true, diff: true, files: true, comments: true, status: true},
		{width: 58, author: false, age: true, diff: true, files: true, comments: true, status: true},
		{width: 46, author: false, age: false, diff: true, files: true, comments: true, status: true},
		{width: 38, author: false, age: false, diff: true, files: false, comments: true, status: true},
		{width: 30, author: false, age: false, diff: false, files: false, comments: true, status: true},
		{width: 20, author: false, age: false, diff: false, files: false, comments: false, status: false},
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
				{name: "status", text: reviewGlyph, want: tt.status},
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

func TestTheIdentityReadsAsOnePhrase(t *testing.T) {
	lines := strings.Split(stripANSI(selectedRow(t, screen(t, 140, 10, []gh.PullRequest{pr("Fix auth retry")}))), "\n")

	if !strings.Contains(lines[0], "#412") {
		t.Errorf("the number is not on the title line\n%q", lines[0])
	}
	if !strings.Contains(lines[1], "acme/rocket by @drucial · 2h") {
		t.Errorf("the identity is spread across columns\n%q", lines[1])
	}
}

func TestADeletedAuthorDropsTheWholeClause(t *testing.T) {
	p := pr("Fix auth retry")
	p.Author = gh.Actor{}

	row := stripANSI(rowContaining(t, screen(t, 140, 10, []gh.PullRequest{p}), "acme/rocket"))

	if strings.Contains(row, "by ") || strings.Contains(row, "@") {
		t.Errorf("a deleted author still leaves an attribution\n%q", row)
	}
	if !strings.Contains(row, "acme/rocket · 2h") {
		t.Errorf("the rest of the identity went with the author\n%q", row)
	}
}

func TestTheChurnColoursAdditionsAndDeletionsApart(t *testing.T) {
	second := pr("Bump deps")
	second.Number, second.Additions, second.Deletions = 408, 11, 3
	second.Author = gh.Actor{Login: "octobot"}

	row := rowContaining(t, screen(t, 140, 14, []gh.PullRequest{pr("Fix auth retry"), second}), "@octobot")

	if got := styleOf(t, row, "+11"); !strings.Contains(got, fgSeq(testTheme.Success)) {
		t.Errorf("additions render as %s, want the success colour", got)
	}
	if got := styleOf(t, row, "−3"); !strings.Contains(got, fgSeq(testTheme.Error)) {
		t.Errorf("deletions render as %s, want the error colour", got)
	}
}

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

func TestBothStatusDotsAlwaysDraw(t *testing.T) {
	checks := []gh.CheckState{
		gh.CheckStateNone, gh.CheckStateExpected, gh.CheckStatePending,
		gh.CheckStateSuccess, gh.CheckStateFailure, gh.CheckStateError,
	}
	reviews := []gh.ReviewDecision{
		gh.ReviewDecisionNone, gh.ReviewDecisionApproved,
		gh.ReviewDecisionChangesRequested, gh.ReviewDecisionReviewRequired,
	}

	for _, c := range checks {
		for _, d := range reviews {
			p := pr("Fix auth retry")
			p.Checks, p.ReviewDecision = c, d

			row := stripANSI(rowContaining(t, screen(t, 140, 12, []gh.PullRequest{p}), "acme/rocket"))
			if want := "● " + reviewGlyph + " ● " + checksGlyph; !strings.Contains(row, want) {
				t.Errorf("checks %q with review %q does not draw both readings: %q", c, d, row)
			}
		}
	}
}

func TestTheReviewDotColoursTellTheDecisionsApart(t *testing.T) {
	tests := []struct {
		decision gh.ReviewDecision
		want     color.Color
	}{
		{decision: gh.ReviewDecisionApproved, want: testTheme.Success},
		{decision: gh.ReviewDecisionChangesRequested, want: testTheme.Error},
		{decision: gh.ReviewDecisionReviewRequired, want: testTheme.Warning},
		{decision: gh.ReviewDecisionNone, want: testTheme.Success},
	}

	for _, tt := range tests {
		p := pr("Fix auth retry")
		p.ReviewDecision, p.Checks = tt.decision, gh.CheckStateNone

		row := rowContaining(t, screen(t, 140, 12, []gh.PullRequest{p}), "acme/rocket")
		if got := styleOf(t, row, "●"); !strings.Contains(got, fgSeq(tt.want)) {
			t.Errorf("%s renders its dot as %s, want %s", tt.decision, got, fgSeq(tt.want))
		}
	}
}

func TestALongTitleRunsToTheEdge(t *testing.T) {
	const width = 200

	row := stripANSI(rowContaining(t, screen(t, width, 10, []gh.PullRequest{pr(strings.Repeat("word ", 50))}), "word"))

	if got := len([]rune(strings.TrimRight(strings.Trim(row, "│"), " "))); got < width-12 {
		t.Errorf("the title stops at column %d of %d, want it to run to the edge", got, width)
	}
}

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

	for i := len(prs) - 2; i >= 0; i-- {
		m = press(m, key('k'))
		if got := stripANSI(selectedRow(t, m.View())); !strings.Contains(got, fmt.Sprintf("#%d ", i)) {
			t.Fatalf("walking back up, the visible selection is %q, want #%d", got, i)
		}
	}
}

func TestTheWindowNeverOpensMidRow(t *testing.T) {
	tests := []struct {
		name string
		keys []tea.KeyPressMsg
	}{
		{name: "walked into the middle", keys: repeat(key('j'), 12)},
		{name: "walked to the bottom", keys: repeat(key('j'), 19)},
		{name: "jumped to the bottom", keys: []tea.KeyPressMsg{key('G')}},
		{name: "bottom then back up one", keys: []tea.KeyPressMsg{key('G'), key('k')}},
	}

	for _, height := range []int{12, 13, 14, 15} {
		for _, tt := range tests {
			t.Run(fmt.Sprintf("%s at %d", tt.name, height), func(t *testing.T) {
				m := press(newList(120, height, numbered(20)), tt.keys...)

				body := strings.Split(stripANSI(m.View()), "\n")[1]
				if strings.Contains(body, "acme/rocket") {
					t.Errorf("the window opens on a row's second line: %q", body)
				}
			})
		}
	}
}

func repeat(k tea.KeyPressMsg, n int) []tea.KeyPressMsg {
	ks := make([]tea.KeyPressMsg, n)
	for i := range ks {
		ks[i] = k
	}
	return ks
}

func TestPageKeysLandOnAPullRequestAndStopAtTheEnds(t *testing.T) {
	prs := numbered(20)

	tests := []struct {
		name string
		keys []tea.KeyPressMsg
		want string
	}{
		{name: "page down", keys: []tea.KeyPressMsg{ctrl('f')}, want: "#4 "},
		{name: "page down then up returns", keys: []tea.KeyPressMsg{ctrl('f'), ctrl('b')}, want: "#0 "},
		{name: "half page down", keys: []tea.KeyPressMsg{ctrl('d')}, want: "#2 "},
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

func TestAMessageWithNoRowsBehindItSitsInTheMiddleOfThePane(t *testing.T) {
	const width, height = 120, 12

	tests := []struct {
		name     string
		sections []store.Section
		want     string
	}{
		{
			name:     "empty",
			sections: ready([]string{"My PRs"}, nil),
			want:     "Nothing matches this section.",
		},
		{
			name:     "loading",
			sections: []store.Section{{Section: config.Section{Title: "My PRs"}, Status: store.StatusLoading}},
			want:     "Loading pull requests",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := list.New(testTheme)
			m.SetSize(width, height)
			m.SetSections(tt.sections)

			lines := strings.Split(stripANSI(m.View()), "\n")
			at := -1
			for i, line := range lines {
				if strings.Contains(line, tt.want) {
					at = i
					break
				}
			}
			if at < 0 {
				t.Fatalf("no %q in the frame\n%s", tt.want, strings.Join(lines, "\n"))
			}

			above, below := at-1, (height-2)-at
			if abs(above-below) > 1 {
				t.Errorf("%q has %d rows above it and %d below, want it centred\n%s",
					tt.want, above, below, strings.Join(lines, "\n"))
			}

			text := strings.Trim(lines[at], "│")
			gap := len(text) - len(strings.TrimLeft(text, " "))
			if right := len(text) - len(strings.TrimRight(text, " ")); gap == 0 || abs(gap-right) > 1 {
				t.Errorf("%q sits %d in from the left and %d from the right, want it centred", tt.want, gap, right)
			}
		})
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func TestAShortPaneKeepsTheHeaderOverTheFirstRow(t *testing.T) {
	prs := numbered(8)
	for i := range prs[4:] {
		prs[4+i].IsDraft = true
	}

	m := press(newList(120, 6, prs), append(repeat(key('j'), 7), repeat(key('k'), 3)...)...)

	if out := stripANSI(m.View()); !strings.Contains(out, "─ Draft") {
		t.Errorf("the draft header is not over its first row\n%s", out)
	}
}

func TestTheHalfPageKeysMoveOnAShortPane(t *testing.T) {
	m := press(newList(120, 6, numbered(8)), ctrl('d'))

	if got := stripANSI(selectedRow(t, m.View())); strings.Contains(got, "#0 ") {
		t.Errorf("ctrl+d left the cursor where it was: %q", got)
	}
}

func TestALargeChurnAbbreviatesRatherThanClipping(t *testing.T) {
	big := pr("Vendor the dependency tree")
	big.Additions, big.Deletions = 12045, 340000

	row := stripANSI(rowContaining(t, screen(t, 140, 12, []gh.PullRequest{big}), "acme/rocket"))

	for _, want := range []string{"+12k", "−340k"} {
		if !strings.Contains(row, want) {
			t.Errorf("row = %q, want %s in it", row, want)
		}
	}
}

func TestAnUnknownCheckStateDoesNotReadAsAPass(t *testing.T) {
	unknown := pr("Fix auth retry")
	unknown.Checks = gh.CheckState("SOMETHING_GITHUB_ADDED")

	_, after, _ := strings.Cut(selectedRow(t, screen(t, 140, 12, []gh.PullRequest{unknown})), reviewGlyph)

	if got := styleOf(t, after, "●"); strings.Contains(got, fgSeq(testTheme.Success)) {
		t.Errorf("an unknown check state renders as a pass: %q", got)
	}
}

func TestTabsCarryTheirOwnCountAndMarkAFailure(t *testing.T) {
	m := list.New(testTheme)
	m.SetSize(160, 20)
	m.SetSections([]store.Section{
		{Section: config.Section{Title: "Mine"}, PRs: numbered(7), Status: store.StatusReady, Loaded: true},
		{Section: config.Section{Title: "Review"}, PRs: numbered(2), Status: store.StatusReady, Loaded: true},
		{Section: config.Section{Title: "Involved"}, Status: store.StatusLoading},
		{Section: config.Section{Title: "Broken"}, Status: store.StatusFailed, Err: errors.New("boom")},
	})

	top := strings.Split(stripANSI(m.View()), "\n")[0]
	for _, want := range []string{"Mine (7)", "Review (2)", "Involved - ", "Broken !"} {
		if !strings.Contains(top, want) {
			t.Errorf("tab strip = %q, want %q in it", top, want)
		}
	}
}

func TestAReloadKeepsTheCountItAlreadyHad(t *testing.T) {
	m := list.New(testTheme)
	m.SetSize(160, 20)
	m.SetSections(ready([]string{"Mine", "Review"}, numbered(7), numbered(2)))

	reloading := ready([]string{"Mine", "Review"}, numbered(7), numbered(2))
	for i := range reloading {
		reloading[i].Status = store.StatusLoading
	}
	m.SetSections(reloading)

	top := strings.Split(stripANSI(m.View()), "\n")[0]
	for _, want := range []string{"Mine (7)", "Review (2)"} {
		if !strings.Contains(top, want) {
			t.Errorf("tab strip = %q, want %q held through the reload", top, want)
		}
	}
}

func TestKeysDoNothingWhileTheSectionIsNotShowingItsRows(t *testing.T) {
	m := newList(140, 20, numbered(10))
	m = press(m, key('j'), key('j'))

	held := ready([]string{"My PRs"}, numbered(10))
	held[0].Status = store.StatusFailed
	held[0].Err = errors.New("boom")
	m.SetSections(held)

	inert := []struct {
		key  tea.KeyPressMsg
		said string
	}{
		{key: tea.KeyPressMsg{Code: tea.KeyEnter}, said: "enter opened"},
		{key: key('y'), said: "y copied the link to"},
		{key: key('O'), said: "O browsed to"},
	}
	for _, in := range inert {
		if _, cmd := m.Update(in.key); cmd != nil {
			t.Errorf("%s a pull request that was not on screen", in.said)
		}
	}

	m = press(m, key('j'))
	m.SetSections(ready([]string{"My PRs"}, numbered(10)))
	if got := selectedRow(t, m.View()); !strings.Contains(got, "Change 2") {
		t.Errorf("selection = %q, want it where it was left before the failure", got)
	}
}

func TestAReloadKeepsItsRowsAndItsKeys(t *testing.T) {
	m := newList(140, 20, numbered(10))
	m = press(m, key('j'), key('j'))

	held := ready([]string{"My PRs"}, numbered(10))
	held[0].Status = store.StatusLoading
	m.SetSections(held)

	if got := stripANSI(m.View()); !strings.Contains(got, "Change 1") {
		t.Errorf("body = %q, want the rows still on it through the reload", got)
	}

	live := []struct {
		key  tea.KeyPressMsg
		said string
	}{
		{key: tea.KeyPressMsg{Code: tea.KeyEnter}, said: "enter"},
		{key: key('y'), said: "y"},
		{key: key('O'), said: "O"},
	}
	for _, in := range live {
		if _, cmd := m.Update(in.key); cmd == nil {
			t.Errorf("%s did nothing on rows that are on the screen", in.said)
		}
	}
}

func TestCopyAndBrowseCarryTheSelectedPullRequest(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyPressMsg
		want tea.Msg
	}{
		{name: "y", key: key('y'), want: list.CopyLinkMsg{PR: numbered(10)[2]}},
		{name: "O", key: key('O'), want: list.BrowseMsg{PR: numbered(10)[2]}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := press(newList(140, 20, numbered(10)), key('j'), key('j'))

			_, cmd := m.Update(tt.key)
			if cmd == nil {
				t.Fatalf("%s said nothing about the pull request under the cursor", tt.name)
			}
			if got := cmd(); got != tt.want {
				t.Fatalf("%s produced %+v, want %+v", tt.name, got, tt.want)
			}
		})
	}
}

func TestTheParkedCursorFollowsThePullRequestNotTheRow(t *testing.T) {
	m := list.New(testTheme)
	m.SetSize(140, 20)
	m.SetSections(ready([]string{"Mine", "Review"}, numbered(4), numbered(6)))

	m = press(m, key(']'), key('j'), key('j'), key('j'))
	if got := selectedRow(t, m.View()); !strings.Contains(got, "Change 3") {
		t.Fatalf("setup: selection = %q, want it on Change 3", got)
	}

	m = press(m, key('['))
	m.SetSections(ready([]string{"Mine", "Review"}, numbered(4), numbered(6)[2:]))

	if got := selectedRow(t, press(m, key(']')).View()); !strings.Contains(got, "Change 3") {
		t.Errorf("selection = %q, want the pull request it was parked on", got)
	}
}

func TestSwitchingSectionsAndBackKeepsTheCursor(t *testing.T) {
	m := list.New(testTheme)
	m.SetSize(140, 20)
	m.SetSections(ready([]string{"My PRs", "Needs My Review"}, numbered(10), numbered(10)[6:]))

	m = press(m, key('j'), key('j'), key('j'))
	if got := selectedRow(t, m.View()); !strings.Contains(got, "Change 3") {
		t.Fatalf("setup: selection = %q, want it on Change 3", got)
	}

	m = press(m, key(']'))
	if got := selectedRow(t, m.View()); !strings.Contains(got, "Change 6") {
		t.Fatalf("setup: selection = %q, want the second section's first row", got)
	}

	if got := selectedRow(t, press(m, key('[')).View()); !strings.Contains(got, "Change 3") {
		t.Errorf("selection = %q, want the cursor back where it was left", got)
	}
}

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

func sgrParams(s lipgloss.Style) string {
	out := s.Render("x")
	end := strings.Index(out, "m")
	if end < 0 {
		return ""
	}
	return out[len("\x1b["):end]
}

func TestTheFixtureReposSortInTheOrderTheFixturesAssume(t *testing.T) {
	main := pr("Fix auth retry").Repository

	odd := mixed()[1].Repository
	if odd == main {
		t.Fatalf("setup: the odd row shares %q with the rest, so it aims at nothing", main)
	}
	if odd >= main {
		t.Errorf("the odd fixture repo %q sorts at or after %q, so the row it marks "+
			"no longer comes first and the tests that walk to it are counting rows "+
			"in a different order", odd, main)
	}
}
