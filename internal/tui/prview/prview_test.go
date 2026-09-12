package prview_test

import (
	"errors"
	"fmt"
	"image/color"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/paint"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
	"github.com/praxis-labs-io/zen-octo/internal/tui/syntax"
)

const sampleURL = "https://github.com/acme/rocket/pull/412"

func samplePR() gh.PullRequest {
	return gh.PullRequest{
		ID: "PR_412", Number: 412, Title: "Fix the auth retry backoff loop",
		URL:        sampleURL,
		Repository: "acme/rocket", Author: gh.Actor{Login: "drucial"},
		State: gh.PRStateOpen, BaseRefName: "main", HeadRefName: "fix-auth-retry",
		Additions: 42, Deletions: 7, ChangedFiles: 3, Comments: 24,
		Checks: gh.CheckStateFailure, ReviewDecision: gh.ReviewDecisionChangesRequested,
		CreatedAt: time.Now().Add(-50 * time.Hour),
	}
}

func colorizer() syntax.Syntax {
	s, _ := syntax.New(testTheme.Syntax)
	return s
}

func screen(width, height int) prview.Model { return press(onOpen(width, height), "2") }

func onOpen(width, height int) prview.Model { return sized(samplePR(), width, height) }

func sized(pr gh.PullRequest, width, height int) prview.Model {
	m := prview.New(testTheme, pr, prview.RailPreference{}, colorizer())
	m.SetSize(width, height)
	return m
}

func press(m prview.Model, keys ...string) prview.Model {
	for _, k := range keys {
		m, _ = m.Update(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
	}
	return m
}

func fgSeq(c color.Color) string { return sgrParams(lipgloss.NewStyle().Foreground(c)) }

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
	m := detailed(held(sampleDetail()), 160, 24)

	if got := currentTab(t, m.View()); got != "Conversation" {
		t.Errorf("the current tab is %q on open, want Conversation", got)
	}

	next := press(m, "]")
	if got := currentTab(t, next.View()); got != "Commits" {
		t.Errorf("] moved to %q, want Commits", got)
	}
	if !strings.Contains(stripANSI(next.View()), "No commits.") {
		t.Error("the body did not follow the tab")
	}

	if got := currentTab(t, press(m, "[").View()); got != "Files" {
		t.Errorf("[ from the first tab wrapped to %q, want Files", got)
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

func TestTheRailCarriesEverySectionEmptyOrNot(t *testing.T) {
	out := screen(200, 36).View()

	for _, want := range []string{"State", "Checks", "Author", "Changes", "Base", "Merge"} {
		if !strings.Contains(out, want) {
			t.Errorf("rail is missing %q", want)
		}
	}

	rows := railRows(t, screen(200, 40).View())
	empty := map[string]string{
		"Reviewers": "+ Add reviewer",
		"Assignees": "+ Add assignee",
		"Labels":    "+ Add label",
		"Checks":    "None yet",
	}
	for heading, want := range empty {
		found := false
		for i, row := range rows {
			if row != heading {
				continue
			}
			found = true
			if got := rows[i+1]; got != want {
				t.Errorf("%s = %q, want %q", heading, got, want)
			}
		}
		if !found {
			t.Errorf("rail dropped the %q section rather than showing it empty", heading)
		}
	}

	for i, row := range rows {
		if row != "State" {
			continue
		}
		if got := rows[i+1]; got != "\uf407 Open" {
			t.Errorf("state row = %q, want it marked with the lifecycle glyph", got)
		}
		return
	}
	t.Fatalf("no State section in the rail: %q", rows)
}

func TestTheHeaderCarriesChecksAndReviewWhateverTheRailDoes(t *testing.T) {
	wide := screen(200, 30).View()
	if !strings.Contains(wide, "Author") {
		t.Fatal("setup: the rail is not up at 200 columns")
	}

	narrow := screen(100, 30).View()
	if strings.Contains(narrow, "Author") {
		t.Fatal("setup: the rail is still up at 100 columns")
	}

	for _, frame := range []string{wide, narrow} {
		rows := strings.Join(headerRows(t, frame), "\n")
		for _, want := range []string{"failing", "changes requested"} {
			if !strings.Contains(rows, want) {
				t.Errorf("the header does not carry %q:\n%s", want, rows)
			}
		}
	}

	if strings.Count(narrow, "fix-auth-retry") != 1 {
		t.Error("the header repeats the branch")
	}
}

func TestFocusMovesBetweenThePanes(t *testing.T) {
	var (
		focused = fgSeq(testTheme.Accent)
		idle    = fgSeq(testTheme.BorderSubtle)
	)

	m := screen(200, 30)
	if got := conversationBorder(t, m.View()); got != focused {
		t.Fatalf("conversation border = %s on open, want the focused accent", got)
	}

	rail := press(m, "h")
	if got := conversationBorder(t, rail.View()); got != idle {
		t.Errorf("conversation border = %s after h, want it to recede", got)
	}

	if got := conversationBorder(t, press(rail, "l").View()); got != focused {
		t.Errorf("conversation border = %s after l, want focus back on the right pane", got)
	}
	if got := conversationBorder(t, press(rail, "2").View()); got != focused {
		t.Errorf("conversation border = %s after 2, want focus jumped straight back", got)
	}
}

func TestFocusLeavesTheRailWhenTheRailDoes(t *testing.T) {
	hidden := press(screen(200, 30), "h", "d")

	if got := conversationBorder(t, hidden.View()); got != fgSeq(testTheme.Accent) {
		t.Errorf("conversation border = %s, want focus back on it once the rail went away", got)
	}
}

func TestTheRailScrollsOnceItHasFocus(t *testing.T) {
	m := screen(200, 18)

	if railHas(t, m.View(), "Checks") {
		t.Fatal("setup: the rail already fits, so there is nothing to scroll")
	}
	if railHas(t, press(m, "G").View(), "Checks") {
		t.Error("G moved the rail while the conversation had focus")
	}

	rail := press(m, "h", "G")
	if !railHas(t, rail.View(), "Checks") {
		t.Error("the rail did not scroll once it had focus")
	}
	if !strings.Contains(stripANSI(rail.View()), "/") {
		t.Error("the rail carries no position counter, so there is nothing saying it scrolls")
	}
}

func railHas(t *testing.T, frame, heading string) bool {
	t.Helper()

	for _, row := range railRows(t, frame) {
		if strings.HasPrefix(row, heading) {
			return true
		}
	}
	return false
}

func TestADeletedAuthorLeavesNoGapInACardHeading(t *testing.T) {
	d := sampleDetail()
	d.Author = gh.Actor{}
	d.Timeline[0].Actor = gh.Actor{}

	frame := detailed(held(d), 200, 30).View()
	left, right := paneEdges(t, frame)

	for i, line := range strings.Split(stripANSI(frame), "\n") {
		body := strings.TrimSpace(strings.Trim(paneBody(line, left, right), "│ "))
		if strings.HasPrefix(body, "·") {
			t.Errorf("line %d = %q, want no separator where the login would be", i, body)
		}
	}

	out := stripANSI(frame)
	for _, want := range []string{"opened this", "commented"} {
		if !strings.Contains(out, want) {
			t.Errorf("the heading lost %q along with the author", want)
		}
	}
}

func TestTheBranchLineClipsTheHeadRatherThanWrapping(t *testing.T) {
	pr := samplePR()
	pr.HeadRefName = "feature/eng-9547-marketing-and-dashboard-share-one-globalscss-so-base-element-styles-leak"

	d := sampleDetail()
	d.PullRequest = pr

	m := prview.New(testTheme, pr, prview.RailPreference{}, colorizer())
	m.SetDetail(held(d))

	m.SetSize(80, 30)

	frame := m.View()
	rows := 0
	for _, row := range headerRows(t, frame) {
		if !strings.Contains(row, "main ←") {
			continue
		}
		rows++
		if !strings.Contains(row, "…") {
			t.Errorf("branch line = %q, want the head marked where it was cut", row)
		}
	}
	if rows != 1 {
		t.Errorf("the branch takes %d lines, want 1", rows)
	}
}

func TestAShortBaseLeavesItsRoomToTheHead(t *testing.T) {
	pr := samplePR()
	pr.BaseRefName = "main"
	pr.HeadRefName = "feature/znn-16-a-release-skill-to-carry-the-judgement-the-workflow-cannot"

	base, head := branchHalves(t, sized(pr, 200, 30).View())
	if base != "main" {
		t.Errorf("the base reads %q, want main whole", base)
	}
	if head != pr.HeadRefName {
		t.Errorf("the head reads %q, want the room main did not take", head)
	}
}

func TestTwoLongBranchesTakeHalfEach(t *testing.T) {
	pr := samplePR()
	pr.BaseRefName = "feature/znn-15-cut-releases-from-a-tag-and-install-the-binary-from-one"
	pr.HeadRefName = "feature/znn-16-a-release-skill-to-carry-the-judgement-the-workflow-cannot"

	base, head := branchHalves(t, sized(pr, 200, 30).View())
	for _, half := range []struct{ what, name string }{{"base", base}, {"head", head}} {
		if !strings.HasSuffix(half.name, "…") {
			t.Errorf("the %s is not marked where it was cut: %q", half.what, half.name)
		}
		if !strings.Contains(half.name, "znn-1") {
			t.Errorf("the cut took the %s's ticket key with it: %q", half.what, half.name)
		}
	}
	if got := lipgloss.Width(base) - lipgloss.Width(head); got > 1 || got < -1 {
		t.Errorf("the halves differ by %d columns, want them even", got)
	}
}

func TestTheBranchLineStopsAtItsMeasure(t *testing.T) {
	pr := samplePR()
	pr.BaseRefName = strings.Repeat("a", 200)
	pr.HeadRefName = strings.Repeat("b", 200)

	base, head := branchHalves(t, sized(pr, 400, 30).View())
	if got := lipgloss.Width(base + " ← " + head); got != 96 {
		t.Errorf("the branch line is %d columns on a 400-column frame, want 96", got)
	}
}

func TestTheBranchLineGivesWayToANarrowFrame(t *testing.T) {
	pr := samplePR()
	pr.BaseRefName = strings.Repeat("a", 200)
	pr.HeadRefName = strings.Repeat("b", 200)

	base, head := branchHalves(t, sized(pr, 60, 30).View())
	if got := lipgloss.Width(base + " ← " + head); got > 60-headGutterCols*2 {
		t.Errorf("the branch line is %d columns on a 60-column frame", got)
	}
}

const headGutterCols = 1

func branchHalves(t *testing.T, frame string) (string, string) {
	t.Helper()

	for _, row := range headerRows(t, frame) {
		base, head, ok := strings.Cut(row, " ← ")
		if !ok {
			continue
		}
		if at, _, cut := strings.Cut(head, "  "); cut {
			head = at
		}
		return base, head
	}
	t.Fatal("no branch line on screen")
	return "", ""
}

func TestTheBranchesStillTakeOneLine(t *testing.T) {
	rows := 0
	for _, row := range headerRows(t, detailed(held(sampleDetail()), 200, 30).View()) {
		if !strings.Contains(row, "←") {
			continue
		}
		rows++

		base, head := branchHalves(t, detailed(held(sampleDetail()), 200, 30).View())
		if base+" ← "+head != "main ← fix-auth-retry" {
			t.Errorf("the branch half is %q ← %q, want the branches whole", base, head)
		}
	}
	if rows != 1 {
		t.Errorf("the branches take %d lines, want 1", rows)
	}
}

func TestTheFarEdgeShedsInOrderAndAlwaysKeepsTheState(t *testing.T) {
	for width := 200; width >= 40; width-- {
		row := titleRow(t, detailed(held(sampleDetail()), width, 30).View())

		churn := strings.Contains(row, "+42 −7")
		checks := strings.Contains(row, "✗ failing")
		review := strings.Contains(row, "changes requested")

		if !strings.Contains(row, "Open") {
			t.Fatalf("width %d: %q sheds the state, which is the group that never goes", width, row)
		}

		if strings.Contains(row, "✗") != checks {
			t.Errorf("width %d: %q carries a cut check state", width, row)
		}
		if strings.Contains(row, "changes") != review {
			t.Errorf("width %d: %q carries a cut review decision", width, row)
		}

		if churn && !checks {
			t.Errorf("width %d: %q kept the churn over the checks", width, row)
		}
		if checks && !review {
			t.Errorf("width %d: %q kept the checks over the review decision", width, row)
		}
	}
}

func TestAClippedHeaderGivesItsSeparatorBack(t *testing.T) {
	frame := detailed(held(sampleDetail()), 200, 6).View()

	if at := paneTopAt(frame); at != 2 {
		t.Errorf("the panes open on frame line %d, want line 2", at)
	}
	if lines := strings.Split(frame, "\n"); len(lines) != 6 {
		t.Errorf("frame is %d lines, want the 6 it was given", len(lines))
	}
}

func TestTheBranchLineCarriesNothingElse(t *testing.T) {
	for _, row := range headerRows(t, detailed(held(sampleDetail()), 200, 30).View()) {
		if !strings.Contains(row, "←") {
			continue
		}
		if row != "main ← fix-auth-retry" {
			t.Errorf("branch line = %q, want the branches alone", row)
		}
		return
	}
	t.Fatal("no branch line on screen")
}

func paneTop(frame string) string {
	lines := strings.Split(frame, "\n")
	if at := paneTopAt(frame); at >= 0 {
		return lines[at]
	}
	return ""
}

func paneTopAt(frame string) int {
	for i, line := range strings.Split(frame, "\n") {
		if strings.HasPrefix(stripANSI(line), "╭") {
			return i
		}
	}
	return -1
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

func conversationBorder(t *testing.T, frame string) string {
	t.Helper()

	_, right := paneEdges(t, frame)

	line, sgr, visible := paneTop(frame), "", 0
	for i := 0; i < len(line); {
		if strings.HasPrefix(line[i:], "\x1b[") {
			end := strings.IndexByte(line[i:], 'm')
			if end < 0 {
				break
			}
			sgr = line[i+2 : i+end]
			i += end + 1
			continue
		}
		if visible == right {
			return sgr
		}
		_, size := utf8.DecodeRuneInString(line[i:])
		i += size
		visible++
	}
	t.Fatalf("no styled border at the conversation's right edge: %q", line)
	return ""
}

func sampleDetail() gh.PullRequestDetail {
	ago := func(d time.Duration) time.Time { return time.Now().Add(-d) }

	return gh.PullRequestDetail{
		PullRequest: samplePR(),
		Body:        "Caps the backoff at 30s, matching the fetch timeout.",

		Labels:    []gh.Label{{ID: "LA_1", Name: "bug"}},
		Assignees: []gh.Actor{{ID: "U_1", Login: "drucial"}},
		Reviewers: []gh.Reviewer{
			{Actor: gh.Actor{Login: "nkr"}, State: gh.ReviewStateChangesRequested},
			{Actor: gh.Actor{Login: "octobot"}, State: gh.ReviewStateApproved},
			{Actor: gh.Actor{Login: "acme/maintainers"}, Requested: true, Team: true},
		},
		Rollup: gh.CheckRollup{
			State: gh.CheckStateFailure,
			Checks: []gh.Check{
				{Name: "test", Workflow: "Rails Unit Tests", State: gh.CheckStateSuccess},
				{Name: "test", Workflow: "Rails Lint", State: gh.CheckStateFailure},
				{Name: "e2e", Workflow: "E2E Tests", State: gh.CheckStatePending},
				{Name: "codecov", State: gh.CheckStateSkipped},
			},
			Passed: 1, Failed: 1, Pending: 1, Skipped: 1,
		},
		Merge:    gh.MergeBlocked,
		BehindBy: 4,

		Viewer: gh.ViewerActions{CanUpdate: true, CanClose: true, CanAssign: true},

		Timeline: []gh.TimelineItem{
			commented("octobot", ago(3*time.Hour), "Coverage held at 84.2%."),
			reviewed("REV_1", "nkr", gh.ReviewStateChangesRequested, ago(2*time.Hour),
				"Two things on the retry path."),
			{Kind: gh.TimelineForcePushed, Actor: gh.Actor{Login: "drucial"}, CreatedAt: ago(time.Hour)},
		},

		Threads: []gh.ReviewThread{
			{ID: "RT_1", ReviewID: "REV_1", Path: "internal/gh/client.go", Line: 42, Side: gh.SideRight,
				CanReply: true, CanResolve: true,
				Hunk: &gh.Hunk{
					Header: "@@ -40,3 +40,4 @@",
					Lines: []gh.DiffLine{
						{Kind: gh.DiffContext, Old: 40, New: 40, Content: "\tfor {"},
						{Kind: gh.DiffRemoved, Old: 41, Content: "\t\ttime.Sleep(delay)"},
						{Kind: gh.DiffAdded, New: 41, Content: "\t\tdelay = min(delay*2, fetchTimeout)"},
					},
				},
				Comments: []gh.Comment{
					{Kind: gh.CommentThread, ID: "RC_1", Author: gh.Actor{Login: "nkr"},
						CreatedAt: ago(2 * time.Hour), Body: "This backs off forever."},
					{Kind: gh.CommentThread, ID: "RC_4", Author: gh.Actor{Login: "octobot"},
						CreatedAt: ago(90 * time.Minute), Body: "Seconded, the cap is the fix."},
				}},
			{ID: "RT_2", ReviewID: "REV_1", Path: "internal/store/store.go", Line: 88, Side: gh.SideLeft,
				IsResolved: true, CanReply: true, CanUnresolve: true,
				Comments: []gh.Comment{
					{Kind: gh.CommentThread, ID: "RC_2", Author: gh.Actor{Login: "nkr"},
						CreatedAt: ago(2 * time.Hour), Body: "Typo."},
					{Kind: gh.CommentThread, ID: "RC_3", Author: gh.Actor{Login: "drucial"},
						CreatedAt: ago(time.Hour), Body: "Fixed."},
				}},
			{ID: "RT_4", Path: "internal/tui/app/app.go", Line: 12, Side: gh.SideRight,
				Comments: []gh.Comment{
					{Kind: gh.CommentThread, ID: "RC_5", Author: gh.Actor{Login: "nkr"},
						CreatedAt: ago(time.Hour), Body: "Locked, so no reply."},
				}},
			{ID: "RT_5", Path: "internal/tui/keys/keys.go", Line: 7, Side: gh.SideRight,
				CanReply: true, CanResolve: true,
				Comments: []gh.Comment{
					{Kind: gh.CommentThread, ID: "RC_6", Author: gh.Actor{Login: "octobot"},
						CreatedAt: ago(time.Hour), Body: "Is r free after the move?"},
				}},
		},
	}
}

func commented(who string, at time.Time, body string) gh.TimelineItem {
	return gh.TimelineItem{
		Kind: gh.TimelineComment, Actor: gh.Actor{Login: who}, CreatedAt: at,
		Comment: &gh.Comment{
			Kind: gh.CommentIssue, ID: "IC_" + who, Author: gh.Actor{Login: who},
			CreatedAt: at, Body: body,
		},
	}
}

func reviewed(id, who string, state gh.ReviewState, at time.Time, body string) gh.TimelineItem {
	return gh.TimelineItem{
		Kind: gh.TimelineReview, Actor: gh.Actor{Login: who}, CreatedAt: at, Review: state,
		Comment: &gh.Comment{
			Kind: gh.CommentReview, ID: id, Author: gh.Actor{Login: who},
			CreatedAt: at, Body: body,
		},
	}
}

func held(d gh.PullRequestDetail) store.Detail {
	return store.Detail{Detail: d, Status: store.StatusReady, Loaded: true}
}

func detailed(d store.Detail, width, height int) prview.Model {
	return press(opened(d, width, height), "2")
}

func opened(d store.Detail, width, height int) prview.Model {
	m := prview.New(testTheme, samplePR(), prview.RailPreference{}, colorizer())
	m.SetDetail(d)
	m.SetSize(width, height)
	return m
}

func TestTheConversationCarriesTheDescriptionAndEverythingSaidSince(t *testing.T) {
	out := stripANSI(detailed(held(sampleDetail()), 200, 60).View())

	for _, want := range []string{
		"drucial · opened this",
		"Caps the backoff at 30s",
		"octobot · commented",
		"Coverage held at 84.2%",
		"nkr · requested changes",
		"Two things on the retry path",
		"internal/gh/client.go:42",
		"This backs off forever",
		"drucial · force-pushed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the conversation is missing %q", want)
		}
	}
}

func TestAResolvedThreadCollapsesAndAnOpenOneDoesNot(t *testing.T) {
	out := stripANSI(detailed(held(sampleDetail()), 200, 60).View())

	if !strings.Contains(out, "✓ internal/store/store.go:88 · resolved") {
		t.Error("the resolved thread does not name itself as resolved")
	}
	if !strings.Contains(out, "▸ 2 comments") {
		t.Error("the resolved thread does not say what is behind it")
	}
	for _, body := range []string{"Typo.", "Fixed."} {
		if strings.Contains(out, body) {
			t.Errorf("the resolved thread rendered %q rather than staying collapsed", body)
		}
	}

	if !strings.Contains(out, "This backs off forever") {
		t.Error("the open thread collapsed too")
	}
}

func TestAConversationWithNothingInItCentresWhatItSaysInstead(t *testing.T) {
	tests := []struct {
		name  string
		held  store.Detail
		want  string
		short bool
	}{
		{
			name:  "loading",
			held:  store.Detail{Status: store.StatusLoading},
			want:  "Loading the conversation",
			short: true,
		},
		{
			name: "failed",
			held: store.Detail{Status: store.StatusFailed, Err: errors.New("no such host")},
			want: "Could not load the conversation",
		},
	}

	const width, height = 140, 24

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := detailed(tt.held, width, height).View()
			left, right := paneEdges(t, frame)

			lines := strings.Split(stripANSI(frame), "\n")
			at := -1
			for i, line := range lines {
				if strings.Contains(paneBody(line, left, right), tt.want) {
					at = i
					break
				}
			}
			if at < 0 {
				t.Fatalf("no %q in the frame\n%s", tt.want, stripANSI(frame))
			}

			top := paneTopAt(frame)
			above, below := at-(top+2), (height-2)-at
			if above <= 0 || abs(above-below) > 1 {
				t.Errorf("%q has %d lines above it and %d below, want it centred in the pane\n%s",
					tt.want, above, below, stripANSI(frame))
			}

			if !tt.short {
				return
			}

			body := paneBody(lines[at], left, right)
			lead := len(body) - len(strings.TrimLeft(body, " "))
			trail := len(body) - len(strings.TrimRight(body, " "))
			if lead == 0 || abs(lead-trail) > 2 {
				t.Errorf("%q sits %d in from the left and %d from the right, want it centred", tt.want, lead, trail)
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

func TestTheBodyStatesReadAsThemselves(t *testing.T) {
	tests := []struct {
		name string
		held store.Detail
		want string
	}{
		{
			name: "nothing yet",
			held: store.Detail{Status: store.StatusLoading},
			want: "Loading the conversation",
		},
		{
			name: "never loaded and failed",
			held: store.Detail{Status: store.StatusFailed, Err: errors.New("no such host")},
			want: "Could not load the conversation: no such host",
		},
		{
			name: "loaded, then a refetch failed",
			held: store.Detail{Detail: sampleDetail(), Status: store.StatusFailed,
				Loaded: true, Err: errors.New("no such host")},
			want: "Caps the backoff at 30s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if out := stripANSI(detailed(tt.held, 200, 40).View()); !strings.Contains(out, tt.want) {
				t.Errorf("the body does not carry %q", tt.want)
			}
		})
	}
}

func TestScrollPositionSurvivesATabSwitch(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 100, 12), "G")

	before := footerOf(t, m.View())
	if before == "" {
		t.Fatal("setup: the conversation fits, so there is nothing to park")
	}

	if after := footerOf(t, press(m, "]", "[").View()); after != before {
		t.Errorf("position = %q after leaving and coming back, want %q", after, before)
	}
}

func TestTheConversationEndsOnABlankLine(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 100, 12), "G")

	lines := strings.Split(stripANSI(m.View()), "\n")
	if len(lines) < 3 {
		t.Fatalf("the frame is %d lines", len(lines))
	}

	last := lines[len(lines)-2]
	if strings.TrimSpace(strings.Trim(last, "│")) != "" {
		t.Errorf("the pane ends on %q, want a blank line above the border", last)
	}
}

func refreshed(t *testing.T, m prview.Model) prview.RefreshMsg {
	t.Helper()

	_, cmd := key(m, "s")
	if cmd == nil {
		t.Fatal("s asked for nothing")
	}
	asked := cmd()
	msg, ok := asked.(prview.RefreshMsg)
	if !ok {
		t.Fatalf("s produced %T, want a RefreshMsg", asked)
	}
	return msg
}

func TestRefreshAsksForTheDiffTheTabIsShowing(t *testing.T) {
	d := sampleDetail()
	d.Commits = sampleCommits()

	onCommit := press(detailed(held(d), 160, 40), "]")
	onCommit.SetCommitFiles("a3f91c2d5e", commitDiff(sampleFiles()))

	tests := []struct {
		name string
		m    prview.Model
		want prview.RefreshMsg
	}{
		{"conversation", detailed(held(d), 160, 40), prview.RefreshMsg{ID: "PR_412"}},
		{"commits", onCommit, prview.RefreshMsg{ID: "PR_412", SHA: "a3f91c2d5e"}},
		{"checks", press(detailed(held(d), 160, 40), "]", "]"), prview.RefreshMsg{ID: "PR_412"}},
		{"files", press(detailed(held(d), 160, 40), "]", "]", "]"), prview.RefreshMsg{ID: "PR_412", Files: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := refreshed(t, tt.m); got != tt.want {
				t.Errorf("r asked for %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRefreshOnCommitsAsksForNothingBeforeADiffIsOnThePane(t *testing.T) {
	if got := refreshed(t, onCommits(160, 40)); got.SHA != "" {
		t.Errorf("r asked for %q, want no commit while the pane is still empty", got.SHA)
	}
}

func TestTheRailTakesTheDetailOnceItLands(t *testing.T) {
	out := stripANSI(detailed(held(sampleDetail()), 200, 40).View())

	for _, want := range []string{
		"Labels", "bug",
		"Assignees", "drucial",
		"Reviewers", "nkr",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the rail is missing %q", want)
		}
	}
}

func TestTheRailNamesEveryCheck(t *testing.T) {
	out := stripANSI(detailed(held(sampleDetail()), 200, 40).View())

	for _, want := range []string{
		"✓ Rails Unit Tests / test",
		"✗ Rails Lint / test",
		"● E2E Tests / e2e",
		"○ codecov",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the rail is missing %q", want)
		}
	}
	if strings.Contains(out, "1 passed") {
		t.Error("the rail still carries the counts as well as the names")
	}

	marks := railMarks(t, detailed(held(sampleDetail()), 200, 44).View())
	for _, want := range []struct {
		state string
		color color.Color
	}{
		{state: "passing", color: testTheme.Success},
		{state: "running", color: testTheme.Warning},
		{state: "skipped", color: testTheme.Subtle},
	} {
		if !marks[fgSeq(want.color)] {
			t.Errorf("no %s check is marked in its own color", want.state)
		}
	}
}

func TestTheRailLeavesTheBranchToTheHeader(t *testing.T) {
	frame := detailed(held(sampleDetail()), 200, 40).View()

	if strings.Contains(stripANSI(frame), "Branch") {
		t.Error("the rail still has a Branch section")
	}
	if rows := headerRows(t, frame); !strings.Contains(rows[1], "main ← fix-auth-retry") {
		t.Errorf("header line 1 = %q, want the branches", rows[1])
	}
}

func TestTheChangesRowIsOneLineMarkedWithAGlyph(t *testing.T) {
	frame := detailed(held(sampleDetail()), 200, 40).View()

	rows := railRows(t, frame)
	for i, row := range rows {
		if row != "Changes" {
			continue
		}
		if got := rows[i+1]; got != "+42 −7  3 \uea7b" {
			t.Errorf("changes row = %q, want the churn and the count on one line", got)
		}
		return
	}
	t.Fatalf("no Changes section in the rail: %q", rows)
}

func TestALabelTakesTheThemesAccent(t *testing.T) {
	if !strings.Contains(detailed(held(sampleDetail()), 200, 40).View(), fgSeq(testTheme.Accent)) {
		t.Error("the label is not in the theme's accent")
	}
}

func TestTheFrameStillFillsItsSizeWithADetailLoaded(t *testing.T) {
	sizes := []struct{ width, height int }{
		{width: 200, height: 40},
		{width: 160, height: 24},
		{width: 100, height: 20},
		{width: 60, height: 10},
	}

	for _, size := range sizes {
		name := fmt.Sprintf("%dx%d", size.width, size.height)
		t.Run(name, func(t *testing.T) {
			lines := strings.Split(detailed(held(sampleDetail()), size.width, size.height).View(), "\n")

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

func footerOf(t *testing.T, frame string) string {
	t.Helper()

	lines := strings.Split(stripANSI(frame), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if !strings.HasPrefix(lines[i], "╰") {
			continue
		}
		digits := strings.Trim(lines[i], "╰╯─")
		return digits
	}
	return ""
}

func tokens(prefix string, n int) string {
	words := make([]string, n)
	for i := range words {
		words[i] = fmt.Sprintf("%s%03d", prefix, i)
	}
	return strings.Join(words, " ")
}

func TestTheConversationWrapsToFitRatherThanBeingClipped(t *testing.T) {
	body, comment := tokens("body", 120), tokens("note", 60)

	d := sampleDetail()
	d.Body = body
	d.Threads[0].Comments[0].Body = comment

	out := stripANSI(detailed(held(d), 100, 60).View())

	for _, want := range []string{body, comment} {
		for _, word := range strings.Fields(want) {
			if !strings.Contains(out, word) {
				t.Fatalf("%q never reached the screen, so the text is being clipped", word)
			}
		}
	}
}

func TestTheSpinnerRunsUntilThereIsSomethingToRead(t *testing.T) {
	m := detailed(store.Detail{Status: store.StatusLoading}, 120, 20)

	start := m.Init()
	if start == nil {
		t.Fatal("Init started no spinner")
	}

	before := stripANSI(m.View())
	m, next := m.Update(start())
	if next == nil {
		t.Fatal("the chain ended while there was still nothing to read")
	}
	if stripANSI(m.View()) == before {
		t.Error("the frame did not move, so nothing on screen says it is working")
	}

	m.SetDetail(held(sampleDetail()))
	if _, cmd := m.Update(next()); cmd != nil {
		t.Error("the spinner kept ticking over a conversation that had landed")
	}
}

func measureGutters(t *testing.T, frame string) (lead, measure, trail int) {
	t.Helper()

	left, right := paneEdges(t, frame)
	for _, line := range strings.Split(stripANSI(frame), "\n") {
		body := []rune(paneBody(line, left, right))
		if !strings.Contains(string(body), "╭─") {
			continue
		}

		start, end := -1, -1
		for i, r := range body {
			if r == '╭' || r == '─' || r == '╮' {
				if start < 0 {
					start = i
				}
				end = i
			}
		}
		if start < 0 {
			continue
		}
		return start, end - start + 1, len(body) - end - 1
	}

	t.Fatal("no card border in the frame")
	return 0, 0, 0
}

func TestTheConversationIsSetToAMeasureAndCentred(t *testing.T) {
	lead, rule, trail := measureGutters(t, detailed(held(sampleDetail()), 200, 40).View())

	if lead == 0 || trail == 0 {
		t.Errorf("gutters = %d and %d, want the content held off both edges", lead, trail)
	}
	if gap := trail - lead; gap < 0 || gap > 1 {
		t.Errorf("gutters = %d and %d, want them even", lead, trail)
	}

	_, wider, _ := measureGutters(t, detailed(held(sampleDetail()), 300, 40).View())
	if wider != rule {
		t.Errorf("the measure grew from %d to %d with the terminal", rule, wider)
	}
}

func TestANarrowPaneKeepsEveryColumn(t *testing.T) {
	lead, _, trail := measureGutters(t, detailed(held(sampleDetail()), 60, 20).View())

	if lead != 0 || trail != 0 {
		t.Errorf("gutters = %d and %d on a pane under the measure, want none", lead, trail)
	}
}

func TestNothingInTheBodyRunsPastTheMeasure(t *testing.T) {
	d := sampleDetail()
	d.Body = tokens("body", 200)
	d.Threads[0].Comments[0].Body = tokens("note", 100)

	assertWithinMeasure(t, detailed(held(d), 200, 60).View())
}

func TestALongHeaderHoldsItsMeasure(t *testing.T) {
	pr := samplePR()
	pr.HeadRefName = "feature/eng-9547-marketing-and-dashboard-share-one-globalscss-so-base-element-styles-leak-across-both"

	d := sampleDetail()
	d.PullRequest = pr

	m := prview.New(testTheme, pr, prview.RailPreference{}, colorizer())
	m.SetDetail(held(d))
	m.SetSize(150, 30)

	assertWithinMeasure(t, m.View())

	out := stripANSI(m.View())
	if strings.Contains(out, "styles-leak-across-both") {
		t.Error("the branch ran past the measure on a frame with room for it")
	}
	if !strings.Contains(out, "main ← feature/eng-9547-marketing") {
		t.Errorf("the branch lost the front of its name rather than the tail:\n%s", out)
	}
}

func TestTheNumberLeadsTheTitleInTheAccent(t *testing.T) {
	out := detailed(held(sampleDetail()), 200, 30).View()

	if !strings.Contains(out, "1;"+fgSeq(testTheme.Accent)+"m#412") {
		t.Error("the number does not lead the title in the accent")
	}
	if !strings.Contains(stripANSI(out), "#412 Fix the auth retry backoff loop") {
		t.Error("the number is not before the title")
	}
}

func TestThreadsHangOffTheReviewThatOpenedThem(t *testing.T) {
	out := stripANSI(detailed(held(sampleDetail()), 200, 60).View())

	if !strings.Contains(out, "├─│ internal/gh/client.go:42") {
		t.Error("the branch marker does not meet the thread's heading")
	}
	if !strings.Contains(out, "│ ╭") {
		t.Error("the rail does not run past the card's top border")
	}
	if !strings.Contains(out, "╰─│ ✓ internal/store/store.go:88") {
		t.Error("the last thread does not close the run")
	}
}

func TestASegmentThatRendersToNothingLeavesNoGap(t *testing.T) {
	d := sampleDetail()
	d.Body = "<!-- linear-preview -->\n\nReview in Linear\n"

	frame := detailed(held(d), 200, 40).View()
	left, right := paneEdges(t, frame)
	lines := strings.Split(stripANSI(frame), "\n")

	for i, line := range lines {
		if !strings.Contains(line, "drucial · opened this") {
			continue
		}
		if got := strings.Trim(paneBody(lines[i+2], left, right), "│ "); got != "Review in Linear" {
			t.Errorf("first body line = %q, want the text with no gap above it", got)
		}
		return
	}
	t.Fatal("no description card on screen")
}

func TestAThreadCommentIsSpacedFromItsByline(t *testing.T) {
	frame := detailed(held(sampleDetail()), 200, 40).View()
	left, right := paneEdges(t, frame)
	lines := strings.Split(stripANSI(frame), "\n")

	for i, line := range lines {
		if !strings.Contains(line, "nkr · said · ") {
			continue
		}
		if i+1 >= len(lines) {
			t.Fatal("the byline is the last line on screen")
		}
		if got := strings.Trim(paneBody(lines[i+1], left, right), "│ "); got != "" {
			t.Errorf("line after the byline = %q, want a blank one", got)
		}
		return
	}
	t.Fatal("no thread byline on screen")
}

func TestDetailsFoldToALineAndOpenOnTheKey(t *testing.T) {
	d := sampleDetail()
	d.Body = "The problem.\n\n<details>\n<summary>ENG-9547 Marketing and dashboard share one globals.css, so base element styles leak across both of them</summary>\n\n| a.go | did a thing |\n| b.go | did another |\n\n</details>\n"

	m := detailed(held(d), 200, 40)
	assertWithinMeasure(t, m.View())

	folded := stripANSI(m.View())
	if !strings.Contains(folded, "▸ ENG-9547 Marketing") || !strings.Contains(folded, "· 2 lines") {
		t.Error("the fold does not name what is behind it")
	}
	if !strings.Contains(folded, "across both of them") {
		t.Error("the fold line was cut rather than wrapped")
	}
	if strings.Contains(folded, "did a thing") {
		t.Error("the folded table is on screen anyway")
	}

	m = press(m, "space")
	if !strings.Contains(stripANSI(m.View()), "did a thing") {
		t.Error("o did not open the fold")
	}

	m = press(m, "space")
	if !strings.Contains(stripANSI(m.View()), "▸ ENG-9547 Marketing") {
		t.Error("o a second time did not fold it back")
	}
}

func paneEdges(t *testing.T, frame string) (left, right int) {
	t.Helper()

	left, right = -1, -1
	for i, r := range []rune(stripANSI(paneTop(frame))) {
		switch r {
		case '╭':
			left = i
		case '╮':
			right = i
		}
	}
	if left < 0 || right < 0 {
		t.Fatalf("the frame carries no pane border: %q", stripANSI(paneTop(frame)))
	}
	return left, right
}

func paneBody(line string, left, right int) string {
	runes := []rune(line)
	if len(runes) <= right || left >= len(runes) || runes[left] != '│' {
		return ""
	}
	return string(runes[left+1 : right])
}

func assertWithinMeasure(t *testing.T, frame string) {
	t.Helper()

	lead, rule, _ := measureGutters(t, frame)
	left, right := paneEdges(t, frame)
	for i, line := range strings.Split(stripANSI(frame), "\n") {
		body := []rune(paneBody(line, left, right))
		if len(body) <= lead+rule {
			continue
		}
		if got := strings.TrimSpace(string(body[lead+rule:])); got != "" {
			t.Errorf("line %d runs %q past the measure", i, got)
		}
	}
}

func TestACardKeepsItsTextOffTheBorder(t *testing.T) {
	frame := detailed(held(sampleDetail()), 200, 40).View()
	left, right := paneEdges(t, frame)

	cards := 0
	for i, line := range strings.Split(stripANSI(frame), "\n") {
		body := []rune(paneBody(line, left, right))

		start := strings.IndexRune(string(body), '│')
		if start < 0 || strings.ContainsAny(string(body), "╭├╰") {
			continue
		}
		cards++

		after := []rune(strings.TrimPrefix(string(body[start:]), "│"))
		if len(after) > 0 && after[0] != ' ' {
			t.Errorf("line %d puts %q against the card border", i, string(after[0]))
		}
	}
	if cards == 0 {
		t.Fatal("no card content on screen")
	}
}

func titleRow(t *testing.T, frame string) string {
	t.Helper()

	for _, row := range headerRows(t, frame) {
		if strings.Contains(row, "#412") {
			return row
		}
	}
	t.Fatal("no title on screen")
	return ""
}

func TestTheChurnSitsAtTheEndOfTheTitleLine(t *testing.T) {
	out := detailed(held(sampleDetail()), 200, 30).View()

	if got := titleRow(t, out); !strings.HasSuffix(got, "+42 −7") {
		t.Errorf("title line = %q, want the churn pushed to the far edge", got)
	}
	if !strings.Contains(out, fgSeq(testTheme.Success)+"m+42") {
		t.Error("additions are not in the success color")
	}
	if !strings.Contains(out, fgSeq(testTheme.Error)+"m−7") {
		t.Error("deletions are not in the error color")
	}

	for _, row := range headerRows(t, out) {
		if strings.Contains(row, "drucial · main") && strings.Contains(row, "+42") {
			t.Errorf("meta line = %q, want the churn gone from it", row)
		}
	}
}

func TestALongTitleClipsRatherThanPushingTheChurnOff(t *testing.T) {
	pr := samplePR()
	pr.Title = strings.Repeat("a very long title ", 12)

	d := sampleDetail()
	d.PullRequest = pr

	m := prview.New(testTheme, pr, prview.RailPreference{}, colorizer())
	m.SetDetail(held(d))
	m.SetSize(200, 30)

	row := titleRow(t, m.View())
	if !strings.HasSuffix(row, "+42 −7") {
		t.Errorf("title line = %q, want the churn still at the end", row)
	}
	if !strings.Contains(row, "…") {
		t.Errorf("title line = %q, want the title marked where it was cut", row)
	}
}

func headerRows(t *testing.T, frame string) []string {
	t.Helper()

	var rows []string
	for _, line := range strings.Split(stripANSI(frame), "\n") {
		if strings.HasPrefix(line, "╭") {
			if n := len(rows); n > 0 && rows[n-1] == "" {
				rows = rows[:n-1]
			}
			return rows
		}
		rows = append(rows, strings.TrimSpace(line))
	}
	t.Fatal("no pane on screen to close the header")
	return nil
}

func stripRow(t *testing.T, frame string) string {
	t.Helper()

	var last string
	for _, line := range strings.Split(frame, "\n") {
		if strings.HasPrefix(stripANSI(line), "╭") {
			return last
		}
		if strings.TrimSpace(stripANSI(line)) != "" {
			last = line
		}
	}
	t.Fatal("no pane on screen to close the header")
	return ""
}

func currentTab(t *testing.T, frame string) string {
	t.Helper()

	var out strings.Builder
	row, under := stripRow(t, frame), false
	for len(row) > 0 {
		if !strings.HasPrefix(row, "\x1b[") {
			r := []rune(row)[0]
			if under {
				out.WriteRune(r)
			}
			row = row[len(string(r)):]
			continue
		}
		end := strings.Index(row, "m")
		if end < 0 {
			break
		}
		under = slices.Contains(strings.Split(row[2:end], ";"), "4")
		row = row[end+1:]
	}
	return strings.TrimSpace(out.String())
}

func TestTheHeaderReadsAsTwoBlocks(t *testing.T) {
	rows := headerRows(t, detailed(held(sampleDetail()), 200, 30).View())

	want := []string{
		"#412 Fix the auth retry backoff loop",
		"main ← fix-auth-retry",
		"",
		"Conversation",
	}
	if len(rows) != len(want) {
		t.Fatalf("header is %d lines, want %d: %q", len(rows), len(want), rows)
	}
	for i, w := range want {
		if w == "" && rows[i] != "" {
			t.Errorf("header line %d = %q, want it blank", i, rows[i])
		}
		if !strings.Contains(rows[i], w) {
			t.Errorf("header line %d = %q, want it to carry %q", i, rows[i], w)
		}
	}
}

func TestTheStripDropsItsCountsBeforeATabName(t *testing.T) {
	d := sampleDetail()
	for range 128 {
		d.Commits = append(d.Commits, sampleCommits()...)
	}

	for width := 56; width <= 200; width += 8 {
		strip := stripANSI(stripRow(t, detailed(held(d), width, 30).View()))
		for _, tab := range []string{"Conversation", "Commits", "Checks", "Files"} {
			if !strings.Contains(strip, tab) {
				t.Errorf("width %d: %q is cut: %q", width, tab, strip)
			}
		}
	}

	if narrow := stripANSI(stripRow(t, detailed(held(d), 56, 40).View())); strings.Contains(narrow, "(") {
		t.Errorf("the strip kept its counts where they did not fit: %q", narrow)
	}
	if wide := stripANSI(stripRow(t, detailed(held(d), 200, 40).View())); !strings.Contains(wide, "(24)") {
		t.Errorf("the strip carries no counts where they fit: %q", wide)
	}
}

func TestTheStripHoldsItsColumnsWhicheverTabIsCurrent(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 30)

	var first []int
	for _, tab := range []string{"Conversation", "Commits", "Checks", "Files"} {
		strip := stripANSI(stripRow(t, m.View()))

		var at []int
		for _, name := range []string{"Conversation", "Commits", "Checks", "Files"} {
			at = append(at, len([]rune(strip[:strings.Index(strip, name)])))
		}
		if first == nil {
			first = at
		}
		if !slices.Equal(at, first) {
			t.Errorf("on %s the labels start at %v, want %v as on the first tab", tab, at, first)
		}
		m = press(m, "]")
	}
}

func TestAnUnansweredTabCountIsAbsentRatherThanZero(t *testing.T) {
	strip := stripANSI(stripRow(t, onOpen(200, 30).View()))

	if strings.Contains(strip, "(0)") {
		t.Errorf("the strip reads %q, want no count on what has not answered", strip)
	}
	for _, want := range []string{"Conversation (24)", "Files (3)"} {
		if !strings.Contains(strip, want) {
			t.Errorf("the strip reads %q, want %q off the row", strip, want)
		}
	}
}

func TestEachTabNamesBothOfItsPanes(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 40)
	m.SetFiles(loadedFiles(sampleFiles(), 0))

	for _, tt := range []struct{ tab, side, main string }{
		{"Conversation", "Details", "Feed"},
		{"Commits", "Commits", "Diff"},
		{"Checks", "Checks", "Log"},
		{"Files", "Files", "Diff"},
	} {
		top := stripANSI(paneTop(m.View()))
		if !strings.Contains(top, "[1]─"+tt.side) {
			t.Errorf("%s: the left pane reads %q, want %q", tt.tab, top, tt.side)
		}
		if !strings.Contains(top, "[2]─"+tt.main) {
			t.Errorf("%s: the right pane reads %q, want %q", tt.tab, top, tt.main)
		}
		m = press(m, "]")
	}
}

func TestTheHeaderIsOnEveryTab(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 40)

	rows := -1

	for _, tab := range []string{"Conversation", "Commits", "Checks", "Files"} {
		frame := m.View()
		if !onTab(t, frame, tab) {
			t.Fatalf("the strip does not read %q as the current tab", tab)
		}

		head := headerRows(t, frame)
		if len(head) == 0 || !strings.Contains(head[0], "#412") {
			t.Errorf("%s carries no header: %q", tab, head)
		}
		if rows < 0 {
			rows = len(head)
		}
		if len(head) != rows {
			t.Errorf("%s has a %d-line header, want %d as on the Conversation", tab, len(head), rows)
		}
		m = press(m, "]")
	}
}

func TestTheTitleLineCarriesTheStatusAndTheChurn(t *testing.T) {
	row := titleRow(t, detailed(held(sampleDetail()), 200, 30).View())

	if !strings.HasSuffix(row, "Open · ✗ failing · changes requested  +42 −7") {
		t.Errorf("title line = %q, want the state ahead of the churn", row)
	}

	if strings.Contains(row, "") {
		t.Errorf("title line = %q, want no file count on it", row)
	}
}

func TestTheHeaderHoldsItsColumnAcrossTabs(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 40)
	m.SetFiles(loadedFiles(sampleFiles(), 0))

	startsAt := func(frame string) int {
		line := stripANSI(strings.Split(frame, "\n")[0])
		return len(line) - len(strings.TrimLeft(line, " "))
	}

	files := press(m, "]", "]", "]")
	if !strings.Contains(stripANSI(paneTop(files.View())), "Files") {
		t.Fatal("setup: the Files tab opened no column, so there is nothing to hold against")
	}

	if conv, at := startsAt(m.View()), startsAt(files.View()); conv != at {
		t.Errorf("the header starts at column %d on the Conversation and %d on Files, want it held", conv, at)
	}
}

func TestTheHeaderDoesNotScrollWithTheConversation(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 200, 24), "G")

	if rows := headerRows(t, m.View()); len(rows) == 0 || !strings.Contains(rows[0], "#412") {
		t.Errorf("the header scrolled away with the conversation: %q", rows)
	}
}

func TestTheReadoutNamesWhoOpenedItAndWhen(t *testing.T) {
	tests := []struct {
		name    string
		created time.Time
		want    string
	}{
		{name: "days", created: time.Now().Add(-50 * time.Hour), want: "@drucial · 2d"},
		{name: "hours", created: time.Now().Add(-19 * time.Hour), want: "@drucial · 19h"},
		{name: "minutes", created: time.Now().Add(-34 * time.Minute), want: "@drucial · 34m"},
		{name: "moments", created: time.Now(), want: "@drucial · now"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := sampleDetail()
			d.CreatedAt = tt.created

			if got := detailed(held(d), 200, 30).Readout(); got != tt.want {
				t.Errorf("readout = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTheReadoutDropsWhicheverHalfIsMissing(t *testing.T) {
	tests := []struct {
		name  string
		login string
		when  time.Time
		want  string
	}{
		{name: "no timestamp", login: "drucial", want: "@drucial"},
		{name: "no author", when: time.Now().Add(-50 * time.Hour), want: "2d"},
		{name: "neither", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := sampleDetail()
			d.CreatedAt = tt.when
			d.Author = gh.Actor{Login: tt.login}

			if got := detailed(held(d), 200, 30).Readout(); got != tt.want {
				t.Errorf("readout = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTheHeaderLeavesTheOpenedByToTheBar(t *testing.T) {
	for _, row := range headerRows(t, detailed(held(sampleDetail()), 200, 30).View()) {
		if strings.Contains(row, "Opened") || strings.Contains(row, "@drucial") {
			t.Errorf("header line %q still carries who opened the pull request", row)
		}
	}
}

func railRows(t *testing.T, frame string) []string {
	t.Helper()

	left, _ := paneEdges(t, frame)
	var rows []string
	for _, line := range strings.Split(stripANSI(frame), "\n") {
		runes := []rune(line)
		if left == 0 || len(runes) < left {
			continue
		}
		rows = append(rows, strings.Trim(string(runes[:left]), "│╭╮╰╯─ "+paint.BarGlyph))
	}
	return rows
}

func TestChecksSitBelowEverythingOfAFixedSize(t *testing.T) {
	rows := railRows(t, detailed(held(sampleDetail()), 200, 40).View())

	at := -1
	for i, row := range rows {
		if strings.HasPrefix(row, "Checks") {
			at = i
			break
		}
	}
	if at < 0 {
		t.Fatalf("no Checks section in the rail: %q", rows)
	}

	for _, row := range rows[at+1:] {
		switch row {
		case "State", "Author", "Reviewers", "Assignees", "Labels", "Changes":
			t.Errorf("%q sits below Checks, where the long list pushes it off the bottom", row)
		}
	}
}

func TestTheRailEndsWithTheBaseAndTheMergeState(t *testing.T) {
	rows := railRows(t, detailed(held(sampleDetail()), 200, 44).View())

	want := []struct{ heading, value string }{
		{"Base", "4 commits behind main"},
		{"Merge", "Blocked"},
	}
	for _, w := range want {
		found := false
		for i, row := range rows {
			if row != w.heading {
				continue
			}
			found = true
			if got := rows[i+1]; got != w.value {
				t.Errorf("%s = %q, want %q", w.heading, got, w.value)
			}
		}
		if !found {
			t.Errorf("no %q section in the rail: %q", w.heading, rows)
		}
	}

	var last string
	for _, row := range rows {
		if row != "" {
			last = row
		}
	}
	if last != "Blocked" {
		t.Errorf("the rail ends on %q, want the merge state", last)
	}
}

func TestAnUnstableMergeReadsTheRollupRatherThanAssumingAFailure(t *testing.T) {
	tests := []struct {
		name   string
		checks gh.CheckState
		want   string
	}{
		{"running", gh.CheckStatePending, "Checks running"},
		{"queued", gh.CheckStateExpected, "Checks queued"},
		{"failed", gh.CheckStateFailure, "Checks failing"},
		{"green rollup", gh.CheckStateSuccess, "Checks failing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := sampleDetail()
			d.Merge = gh.MergeUnstable
			d.Checks = tt.checks

			rows := railRows(t, detailed(held(d), 200, 44).View())
			for i, row := range rows {
				if row != "Merge" {
					continue
				}
				if got := rows[i+1]; got != tt.want {
					t.Errorf("merge row = %q, want %q", got, tt.want)
				}
				return
			}
			t.Fatalf("no Merge section in the rail: %q", rows)
		})
	}
}

func TestABranchLevelWithItsBaseSaysSo(t *testing.T) {
	d := sampleDetail()
	d.BehindBy = 0

	rows := railRows(t, detailed(held(d), 200, 44).View())
	for i, row := range rows {
		if row != "Base" {
			continue
		}
		if got := rows[i+1]; got != "Up to date with main" {
			t.Errorf("base row = %q, want it to say the branch is level", got)
		}
		return
	}
	t.Fatalf("no Base section in the rail: %q", rows)
}

func TestALongCheckNameClipsRatherThanWrapping(t *testing.T) {
	d := sampleDetail()
	d.Rollup.Checks = []gh.Check{
		{Name: "test (ubuntu-latest, postgres 16)", Workflow: "Rails Integration Tests", State: gh.CheckStateSuccess},
	}

	rows := railRows(t, detailed(held(d), 200, 44).View())

	at := -1
	for i, row := range rows {
		if strings.HasPrefix(row, "Checks") {
			at = i
			break
		}
	}
	if at < 0 {
		t.Fatalf("no Checks section in the rail: %q", rows)
	}

	marked := 0
	for _, row := range rows[at+1:] {
		if row == "" || !strings.ContainsRune("●○✓✗", []rune(row)[0]) {
			break
		}
		marked++
		if !strings.HasSuffix(row, "…") {
			t.Errorf("check row = %q, want it marked where it was cut", row)
		}
	}
	if marked != 1 {
		t.Errorf("the check takes %d rows, want 1", marked)
	}
}

func TestEveryReviewerIsMarkedWithTheirVerdict(t *testing.T) {
	frame := detailed(held(sampleDetail()), 200, 44).View()

	rows := railRows(t, frame)
	at := -1
	for i, row := range rows {
		if row == "Reviewers" {
			at = i
			break
		}
	}
	if at < 0 {
		t.Fatalf("no Reviewers section in the rail: %q", rows)
	}

	want := []struct {
		row   string
		state string
		color color.Color
	}{
		{row: "● @nkr", state: "waiting on a change", color: testTheme.Error},
		{row: "● @octobot", state: "done with it", color: testTheme.Success},
		{row: "● @acme/maintainers", state: "in flight", color: testTheme.Warning},
	}
	for i, w := range want {
		if got := rows[at+1+i]; got != w.row {
			t.Errorf("reviewer %d = %q, want %q", i, got, w.row)
		}
		if got := markSGR(t, frame, w.row); got != fgSeq(w.color) {
			t.Errorf("%s is marked %s, want the %s color", w.row, got, w.state)
		}
	}
}

func TestAReviewerWithAnOpenThreadReadsAsWaiting(t *testing.T) {
	tests := []struct {
		name     string
		reviewer gh.Reviewer
		color    color.Color
	}{
		{
			name:     "commented with nothing outstanding",
			reviewer: gh.Reviewer{State: gh.ReviewStateCommented},
			color:    testTheme.Subtle,
		},
		{
			name:     "commented with a thread still open",
			reviewer: gh.Reviewer{State: gh.ReviewStateCommented, Unresolved: 2},
			color:    testTheme.Error,
		},
		{
			name:     "approved",
			reviewer: gh.Reviewer{State: gh.ReviewStateApproved},
			color:    testTheme.Success,
		},
		{
			name:     "asked and silent",
			reviewer: gh.Reviewer{},
			color:    testTheme.Subtle,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := sampleDetail()
			tt.reviewer.Actor = gh.Actor{Login: "solo"}
			d.Reviewers = []gh.Reviewer{tt.reviewer}

			frame := detailed(held(d), 200, 44).View()
			if got := markSGR(t, frame, "● @solo"); got != fgSeq(tt.color) {
				t.Errorf("marked %s, want %v", got, tt.color)
			}
		})
	}
}

func TestALongReviewerNameClipsRatherThanWrapping(t *testing.T) {
	d := sampleDetail()
	d.Reviewers = []gh.Reviewer{{Actor: gh.Actor{Login: "acme/copilot-pull-request-reviewers"}}}

	rows := railRows(t, detailed(held(d), 200, 44).View())
	for i, row := range rows {
		if row != "Reviewers" {
			continue
		}
		if got := rows[i+1]; !strings.HasSuffix(got, "…") {
			t.Errorf("reviewer row = %q, want it marked where it was cut", got)
		}
		if got := rows[i+2]; strings.HasPrefix(got, "●") {
			t.Errorf("the name wrapped onto %q rather than clipping", got)
		}
		return
	}
	t.Fatalf("no Reviewers section in the rail: %q", rows)
}

func TestTheRailIsPaddedAndOpensWithABlankRow(t *testing.T) {
	frame := detailed(held(sampleDetail()), 200, 44).View()

	raw := railRaw(t, frame)
	if len(raw) < 3 {
		t.Fatalf("rail has %d lines, want a border, a blank and content", len(raw))
	}
	if got := strings.TrimRight(stripANSI(raw[1]), " │"); strings.TrimSpace(got) != "" {
		t.Errorf("rail line 1 = %q, want it blank", got)
	}

	for i, line := range raw[2:] {
		body := stripANSI(line)
		if strings.TrimSpace(strings.Trim(body, "│─╭╮╰╯")) == "" {
			continue
		}
		inner := strings.TrimSuffix(strings.TrimPrefix(body, "│"), "│")
		if !strings.HasPrefix(inner, " ") {
			t.Errorf("rail line %d has no left padding: %q", i+2, inner)
		}
		if !strings.HasSuffix(inner, " ") {
			t.Errorf("rail line %d has no right padding: %q", i+2, inner)
		}
	}
}

func markSGR(t *testing.T, frame, text string) string {
	t.Helper()

	for _, raw := range railRaw(t, frame) {
		if strings.Trim(stripANSI(raw), "│╭╮╰╯›─ "+paint.BarGlyph) != text {
			continue
		}
		seq, ok := rowMark(raw)
		if !ok {
			t.Fatalf("rail row %q carries no mark", text)
		}
		return seq
	}

	t.Fatalf("no rail row reading %q", text)
	return ""
}

func railMarks(t *testing.T, frame string) map[string]bool {
	t.Helper()

	out := map[string]bool{}
	for _, raw := range railRaw(t, frame) {
		if seq, ok := rowMark(raw); ok {
			out[seq] = true
		}
	}
	return out
}

func rowMark(raw string) (string, bool) {
	at := -1
	for _, glyph := range []string{"m✓", "m✗", "m●", "m○"} {
		if i := strings.Index(raw, glyph); i >= 0 && (at < 0 || i < at) {
			at = i
		}
	}
	if at < 0 {
		return "", false
	}
	start := strings.LastIndex(raw[:at], "\x1b[")
	if start < 0 {
		return "", false
	}
	return raw[start+2 : at], true
}

func railRaw(t *testing.T, frame string) []string {
	t.Helper()

	left, _ := paneEdges(t, frame)
	top := paneTopAt(frame)
	var rows []string

	for at, line := range strings.Split(frame, "\n") {
		if at < top {
			continue
		}
		visible, i := 0, 0
		for i < len(line) && visible < left {
			if strings.HasPrefix(line[i:], "\x1b[") {
				end := strings.IndexByte(line[i:], 'm')
				if end < 0 {
					break
				}
				i += end + 1
				continue
			}
			_, size := utf8.DecodeRuneInString(line[i:])
			i += size
			visible++
		}
		if visible == left && left > 0 {
			rows = append(rows, line[:i])
		}
	}
	return rows
}

func TestTheRailNamesPeopleAsHandles(t *testing.T) {
	rows := railRows(t, detailed(held(sampleDetail()), 200, 44).View())

	want := map[string]string{
		"Author":    "@drucial",
		"Assignees": "@drucial",
	}
	for heading, value := range want {
		found := false
		for i, row := range rows {
			if row != heading {
				continue
			}
			found = true
			if got := rows[i+1]; got != value {
				t.Errorf("%s = %q, want %q", heading, got, value)
			}
		}
		if !found {
			t.Errorf("no %q section in the rail: %q", heading, rows)
		}
	}
}

func TestAnEventWithNoWordsForItLeavesNoGap(t *testing.T) {
	ago := func(d time.Duration) time.Time { return time.Now().Add(-d) }

	d := sampleDetail()
	d.Threads = nil
	d.Timeline = []gh.TimelineItem{
		commented("octobot", ago(2*time.Hour), "First."),
		{Kind: "SOMETHING_GITHUB_ADDED_LATER", Actor: gh.Actor{Login: "drucial"}, CreatedAt: ago(time.Hour)},
		commented("nkr", ago(time.Minute), "Second."),
	}

	frame := detailed(held(d), 200, 44).View()
	left, right := paneEdges(t, frame)
	lines := strings.Split(stripANSI(frame), "\n")

	closed, gaps := -1, 0
	for i, line := range lines {
		body := paneBody(line, left, right)
		switch {
		case strings.Contains(body, "╰"):
			closed = i
		case closed >= 0 && strings.Contains(body, "╭"):
			gaps++
			if gap := i - closed - 1; gap != 1 {
				t.Errorf("%d blank lines between cards %d and %d, want 1", gap, gaps, gaps+1)
			}
			closed = -1
		}
	}
	if gaps < 2 {
		t.Fatalf("found %d gaps between cards, want the two either side of the event", gaps)
	}
}

func TestAThreadShowsTheCodeItWasWrittenAgainst(t *testing.T) {
	out := stripANSI(detailed(held(sampleDetail()), 200, 80).View())

	anchor := strings.Index(out, "internal/gh/client.go:42")
	code := strings.Index(out, "delay = min(delay*2, fetchTimeout)")
	comment := strings.Index(out, "This backs off forever.")

	switch {
	case code < 0:
		t.Fatal("the thread shows no diff at all")
	case code < anchor:
		t.Error("the diff renders above the thread it belongs to")
	case comment > 0 && code > comment:
		t.Error("the diff renders under the comment rather than over it")
	}
}

func TestALongThreadHunkIsCutToItsTail(t *testing.T) {
	d := sampleDetail()
	long := make([]gh.DiffLine, 0, 30)
	for i := 1; i <= 30; i++ {
		long = append(long, gh.DiffLine{Kind: gh.DiffContext, Old: i, New: i,
			Content: "line" + strconv.Itoa(i)})
	}
	d.Threads[0].Hunk = &gh.Hunk{Header: "@@ -1,30 +1,30 @@", Lines: long}

	out := stripANSI(detailed(held(d), 200, 200).View())
	if strings.Contains(out, "line1\n") {
		t.Error("the whole hunk rendered, want only its tail")
	}
	if !strings.Contains(out, "line30") {
		t.Error("the line the comment is about is missing")
	}
}

func sgrParams(s lipgloss.Style) string {
	out := s.Render("x")
	end := strings.Index(out, "m")
	if end < 0 {
		return ""
	}
	return out[len("\x1b["):end]
}
