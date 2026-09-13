package prview_test

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

func sampleCommits() []gh.Commit {
	ago := func(d time.Duration) time.Time { return time.Now().Add(-d) }

	return []gh.Commit{
		{SHA: "a3f91c2d5e", Short: "a3f91c2", Headline: "Cap the backoff",
			Author: gh.Actor{Login: "drucial"}, CommittedAt: ago(19 * time.Hour),
			Checks: gh.CheckStateSuccess},
		{SHA: "7b20ef4a11", Short: "7b20ef4", Headline: "Drop the count",
			Author: gh.Actor{Login: "nkr"}, CommittedAt: ago(18 * time.Hour),
			Checks: gh.CheckStateFailure},
		{SHA: "c1d8a04bb9", Short: "c1d8a04", Headline: "Fix the typo",
			AuthorName: "Drew White", CommittedAt: ago(17 * time.Hour),
			Checks: gh.CheckStatePending},
	}
}

func onCommits(width, height int) prview.Model {
	d := sampleDetail()
	d.Commits = sampleCommits()
	return press(detailed(held(d), width, height), "]")
}

func settled(m prview.Model, sha string) (prview.Model, tea.Cmd) {
	return m.Update(prview.CommitSettleMsg{SHA: sha})
}

func armed(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("the key armed no wait at all")
	}
	return cmd()
}

func key(m prview.Model, k string) (prview.Model, tea.Cmd) {
	return m.Update(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
}

func commitDiff(files []gh.ChangedFile) store.Files {
	return store.Files{Files: files, Status: store.StatusReady, Loaded: true}
}

func TestTheCommitColumnNamesEveryCommit(t *testing.T) {
	out := stripANSI(onCommits(160, 24).View())

	for _, want := range []string{
		"● Cap the backoff",
		"a3f91c2 · @drucial · 19h",
		"● Drop the count",
		"7b20ef4 · @nkr · 18h",
		"● Fix the typo",
		"c1d8a04 · Drew White · 17h",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the column is missing %q", want)
		}
	}

	if !strings.Contains(out, "Commits") {
		t.Error("the column is not titled")
	}
}

func TestTheCommitHeadlineHasTheTopLineToItself(t *testing.T) {
	column := columnLines(onCommits(160, 24).View())

	at := -1
	for i, line := range column {
		if strings.Contains(line, "Cap the backoff") {
			at = i
			break
		}
	}
	if at < 0 {
		t.Fatal("the column never named the first commit")
	}

	if strings.Contains(column[at], "a3f91c2") {
		t.Errorf("the headline line is %q, want the sha off it", column[at])
	}
	if !strings.Contains(column[at+1], "a3f91c2") {
		t.Errorf("the line under the headline is %q, want the sha on it", column[at+1])
	}
}

func TestACommitWithNoAccountFallsBackToTheNameGitRecorded(t *testing.T) {
	out := stripANSI(onCommits(160, 24).View())

	if !strings.Contains(out, "Drew White · 17h") {
		t.Error("a commit with no GitHub account left its author blank")
	}
	if strings.Contains(out, "@Drew White") {
		t.Error("a git name was written as a handle")
	}
}

func TestTheCheckMarkerTakesEachCommitsOwnState(t *testing.T) {
	out := onCommits(160, 24).View()

	th := testTheme
	for _, want := range []struct {
		name string
		seq  string
	}{
		{"passing", fgSeq(th.Success)},
		{"failing", fgSeq(th.Error)},
		{"running", fgSeq(th.Warning)},
	} {
		if !marked(out, want.seq) {
			t.Errorf("no %s commit marker in the column", want.name)
		}
	}
}

func marked(frame, fg string) bool {
	return regexp.MustCompile(regexp.QuoteMeta(fg) + `(;[0-9;]+)?m●`).MatchString(frame)
}

func TestTheCursorStoppingAsksForTheCommitsDiff(t *testing.T) {
	m := press(onCommits(160, 24), "1", "j")

	_, cmd := settled(m, "7b20ef4a11")
	if cmd == nil {
		t.Fatal("the cursor settling produced no command, want a request for the diff")
	}

	msg, ok := cmd().(prview.NeedCommitMsg)
	if !ok {
		t.Fatalf("the cursor settling produced %T, want a NeedCommitMsg", cmd())
	}
	if msg.SHA != "7b20ef4a11" {
		t.Errorf("asked for %q, want the commit under the cursor", msg.SHA)
	}
}

func TestSettlingOnTheCommitAlreadyShowingAsksAgainForNothing(t *testing.T) {
	m, _ := settled(onCommits(160, 24), "a3f91c2d5e")
	m.SetCommitFiles("a3f91c2d5e", commitDiff(sampleFiles()))

	if _, cmd := settled(m, "a3f91c2d5e"); cmd != nil {
		t.Error("the commit already showing was asked for a second time")
	}
}

func TestAskingForACommitLeavesTheOneOnScreenAlone(t *testing.T) {
	m, _ := settled(onCommits(160, 24), "a3f91c2d5e")
	m.SetCommitFiles("a3f91c2d5e", commitDiff(sampleFiles()))

	m, _ = settled(press(m, "j"), "7b20ef4a11")

	out := stripANSI(m.View())
	if strings.Contains(out, "Loading the diff") {
		t.Error("the pane spun before the store had been asked")
	}
	if !strings.Contains(out, "internal/gh/client.go") {
		t.Error("the pane dropped the diff it was showing")
	}
}

func TestACommitBeingFetchedSpins(t *testing.T) {
	m, _ := settled(onCommits(160, 24), "a3f91c2d5e")
	m.SetCommitFiles("a3f91c2d5e", store.Files{Status: store.StatusLoading})

	if !strings.Contains(stripANSI(m.View()), "Loading the diff") {
		t.Error("a commit with its request still out does not say so")
	}
}

func TestOnlyTheCommitTheCursorStoppedOnIsAskedFor(t *testing.T) {
	m := press(onCommits(160, 24), "1", "j", "j", "k")

	for _, stale := range []string{"a3f91c2d5e", "c1d8a04bb9"} {
		if _, cmd := settled(m, stale); cmd != nil {
			t.Errorf("a wait armed on %q fetched after the cursor moved off it", stale)
		}
	}

	if _, cmd := settled(m, "7b20ef4a11"); cmd == nil {
		t.Error("the commit the cursor stopped on was never asked for")
	}
}

func TestMovingTheCursorArmsTheWaitForThatCommit(t *testing.T) {
	m, cmd := key(press(onCommits(160, 24), "1"), "j")

	msg, ok := armed(t, cmd).(prview.CommitSettleMsg)
	if !ok {
		t.Fatalf("the key armed %T, want a CommitSettleMsg", armed(t, cmd))
	}
	if msg.SHA != "7b20ef4a11" {
		t.Errorf("the wait names %q, want the commit the cursor landed on", msg.SHA)
	}
	if _, cmd := settled(m, msg.SHA); cmd == nil {
		t.Error("the wait it armed asked for nothing")
	}
}

func TestAWaitThatRunsOutOnAnotherTabAsksForNothing(t *testing.T) {
	m := press(onCommits(160, 24), "1", "j")
	m = press(m, "]")

	if _, cmd := settled(m, "7b20ef4a11"); cmd != nil {
		t.Error("a wait that ran out after the reader left the tab fetched anyway")
	}
}

func TestAFailedCommitArmsARetryWithNowhereToWalk(t *testing.T) {
	d := sampleDetail()
	d.Commits = sampleCommits()[:1]

	m := press(detailed(held(d), 160, 24), "]", "1")
	m, _ = settled(m, "a3f91c2d5e")
	m.SetCommitFiles("a3f91c2d5e", store.Files{Status: store.StatusFailed, Err: errors.New("no such host")})

	m, cmd := key(m, "j")
	msg, ok := armed(t, cmd).(prview.CommitSettleMsg)
	if !ok || msg.SHA != "a3f91c2d5e" {
		t.Fatalf("the key armed %v, want a retry of the failed commit", armed(t, cmd))
	}
	if _, cmd := settled(m, msg.SHA); cmd == nil {
		t.Error("the retry asked for nothing")
	}
}

func TestCommitsArrivingAfterTheTabArmTheirOwnFetch(t *testing.T) {
	d := sampleDetail()
	d.Commits = sampleCommits()

	m := press(detailed(store.Detail{Status: store.StatusLoading}, 160, 24), "]")
	cmd := m.SetDetail(held(d))

	msg, ok := armed(t, cmd).(prview.CommitSettleMsg)
	if !ok || msg.SHA != "a3f91c2d5e" {
		t.Fatalf("the arriving detail armed %v, want the first commit", armed(t, cmd))
	}
}

func TestThePaneSpinsThroughTheSettleWindow(t *testing.T) {
	out := stripANSI(onCommits(160, 24).View())

	if !strings.Contains(out, "Loading the diff") {
		t.Error("the pane sits blank while the wait runs")
	}
}

func TestACommitLandingOffTabKeepsTheReadersPlace(t *testing.T) {
	m, _ := settled(onCommits(160, 40), "a3f91c2d5e")
	m = press(m, "[")
	m = press(m, "j", "j", "j", "j", "j", "j")

	before := m.View()
	m.SetCommitFiles("a3f91c2d5e", commitDiff(sampleFiles()))

	if m.View() != before {
		t.Error("a commit landing off the tab moved the pane the reader was on")
	}
}

func TestASecondRetryOfAFailedCommitStillAsks(t *testing.T) {
	d := sampleDetail()
	d.Commits = sampleCommits()[:1]
	failed := store.Files{Status: store.StatusFailed, Err: errors.New("no such host")}

	m := press(detailed(held(d), 160, 24), "]", "1")
	m, _ = settled(m, "a3f91c2d5e")
	m.SetCommitFiles("a3f91c2d5e", failed)

	m, cmd := settled(m, "a3f91c2d5e")
	if cmd == nil {
		t.Fatal("the first retry asked for nothing")
	}
	m.SetCommitFiles("a3f91c2d5e", store.Files{Status: store.StatusLoading})
	m.SetCommitFiles("a3f91c2d5e", failed)

	if _, cmd := settled(m, "a3f91c2d5e"); cmd == nil {
		t.Error("the second retry was swallowed: pending latched on the first")
	}
}

func TestADiffThatLandsAfterTheCursorWalksBackIsDropped(t *testing.T) {
	m, _ := settled(onCommits(160, 24), "a3f91c2d5e")
	m.SetCommitFiles("a3f91c2d5e", commitDiff(sampleFiles()))

	m, _ = settled(press(m, "j"), "7b20ef4a11")
	m = press(m, "k")
	m.SetCommitFiles("7b20ef4a11", commitDiff(sampleFiles()))

	out := stripANSI(m.View())
	if strings.Contains(out, "7b20ef4a11") {
		t.Error("the card names a commit the column is not pointing at")
	}
	if !strings.Contains(out, "a3f91c2d5e") {
		t.Error("the pane lost the commit the cursor is actually on")
	}
}

func TestTheCommitDiffRendersThroughTheFilesViewer(t *testing.T) {
	m, _ := settled(onCommits(160, 30), "a3f91c2d5e")
	m.SetCommitFiles("a3f91c2d5e", commitDiff(sampleFiles()))

	out := stripANSI(m.View())
	if !strings.Contains(out, "internal/gh/client.go") {
		t.Error("the commit's diff did not render its file heading")
	}
	if !strings.Contains(out, "delay = min(delay*2, fetchTimeout)") {
		t.Error("the commit's diff did not render its code")
	}
}

func TestADiffForAnotherCommitIsDropped(t *testing.T) {
	m, _ := settled(onCommits(160, 24), "a3f91c2d5e")
	m.SetCommitFiles("7b20ef4a11", commitDiff(sampleFiles()))

	if strings.Contains(stripANSI(m.View()), "internal/gh/client.go") {
		t.Error("a diff for a commit that is not selected rendered anyway")
	}
}

func TestTheCommitDiffStatesReadAsThemselves(t *testing.T) {
	cases := []struct {
		name string
		held store.Files
		want string
	}{
		{name: "loading", held: store.Files{Status: store.StatusLoading}, want: "Loading the diff"},
		{name: "failed", held: store.Files{Status: store.StatusFailed, Err: errors.New("no such host")},
			want: "Could not load the diff: no such host"},
		{name: "empty", held: commitDiff(nil), want: "No files changed."},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, _ := settled(onCommits(160, 24), "a3f91c2d5e")
			m.SetCommitFiles("a3f91c2d5e", c.held)

			if out := stripANSI(m.View()); !strings.Contains(out, c.want) {
				t.Errorf("the diff pane does not say %q", c.want)
			}
		})
	}
}

func TestTheCommitsTabAsksForItsFirstDiffOnTheWayIn(t *testing.T) {
	d := sampleDetail()
	d.Commits = sampleCommits()

	m, _ := settled(press(detailed(held(d), 160, 24), "]"), "a3f91c2d5e")
	m.SetCommitFiles("a3f91c2d5e", commitDiff(sampleFiles()))

	if !strings.Contains(stripANSI(m.View()), "internal/gh/client.go") {
		t.Error("the tab opened without asking for the first commit's diff")
	}
}

func TestAnEmptyCommitListLeavesThePaneEmpty(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 160, 24), "]")

	out := stripANSI(m.View())
	if !strings.Contains(out, "No commits.") {
		t.Error("the column does not say the branch is empty")
	}
	if strings.Contains(out, "diff") {
		t.Error("the pane beside an empty column has something to say about a diff")
	}
}

func TestTheSelectedCommitIsPaintedCellByCellAcrossBothLines(t *testing.T) {
	m := onCommits(160, 24)
	seq := bgSeq(testTheme.SelectedBackground)

	var painted []string
	for _, line := range strings.Split(m.View(), "\n") {
		if strings.Contains(line, seq) {
			painted = append(painted, line)
		}
	}

	if len(painted) != 2 {
		t.Fatalf("%d lines carry the selection, want the two of one row", len(painted))
	}
	for i, line := range painted {
		if count := strings.Count(line, seq); count < 2 {
			t.Errorf("line %d paints the selection %d times, want it cell by cell", i, count)
		}
	}
}

func TestTheCommitCursorScrollsAWholeRowAtATime(t *testing.T) {
	for _, height := range []int{11, 12} {
		t.Run(strconv.Itoa(height), func(t *testing.T) {
			d := sampleDetail()
			d.Commits = append(sampleCommits(), sampleCommits()...)
			m := press(detailed(held(d), 160, height), "]", "1", "j", "j")

			column := columnLines(m.View())
			if len(column) < 4 {
				t.Fatalf("the column rendered %d lines, want two whole rows", len(column))
			}

			if !strings.Contains(column[0], "Drop the count") {
				t.Errorf("the column opens on %q, want the top of a row", column[0])
			}
			if !strings.Contains(column[1], "7b20ef4") {
				t.Errorf("the second line is %q, want the sha under its headline", column[1])
			}
			if !strings.Contains(column[2], "Fix the typo") || !strings.Contains(column[3], "Drew White") {
				t.Error("the cursor's row is not on screen whole")
			}
		})
	}
}

func TestACommitDiffCarriesNoReviewThreads(t *testing.T) {
	m, _ := settled(onCommits(160, 40), "a3f91c2d5e")
	m.SetCommitFiles("a3f91c2d5e", commitDiff(sampleFiles()))

	out := stripANSI(m.View())
	for _, gone := range []string{"This backs off forever.", "Typo.", "Fixed."} {
		if strings.Contains(out, gone) {
			t.Errorf("the commit's diff carries the review comment %q", gone)
		}
	}
}

func TestTheSelectedCommitIsNamedAboveItsDiff(t *testing.T) {
	d := sampleDetail()
	d.Commits = sampleCommits()
	d.Commits[0].Body = "The retry loop had no ceiling, so a dead endpoint backed off forever."

	m, _ := settled(press(detailed(held(d), 160, 40), "]"), "a3f91c2d5e")
	m.SetCommitFiles("a3f91c2d5e", commitDiff(sampleFiles()))
	out := stripANSI(m.View())

	for _, want := range []string{
		"Cap the backoff",
		"The retry loop had no ceiling",
		"a3f91c2d5e",
		"@drucial · 19h",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the card is missing %q", want)
		}
	}

	card := strings.Index(out, "The retry loop had no ceiling")
	first := strings.Index(out, "internal/gh/client.go")
	if card < 0 || first < 0 || card > first {
		t.Error("the card is not above the first file")
	}
}

func TestTheCardHoldsUpWithNoMessageBody(t *testing.T) {
	m, _ := settled(onCommits(160, 40), "a3f91c2d5e")
	m.SetCommitFiles("a3f91c2d5e", commitDiff(sampleFiles()))

	out := stripANSI(m.View())
	if !strings.Contains(out, "a3f91c2d5e") {
		t.Error("the card lost the full sha")
	}
	if !strings.Contains(out, "internal/gh/client.go") {
		t.Error("the diff under the card is gone")
	}
}

func TestPagingTheCommitColumnMovesByRows(t *testing.T) {
	d := sampleDetail()
	d.Commits = manyCommits(40)
	m := press(detailed(held(d), 160, 24), "]", "1")

	before := shownHeadlines(m.View())
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	after := shownHeadlines(m.View())

	if len(before) == 0 || len(after) == 0 {
		t.Fatalf("the column showed %d rows then %d", len(before), len(after))
	}
	if before[0] == after[0] {
		t.Fatal("page down did not move the column")
	}

	if at := indexOf(before, after[0]); at < 0 {
		t.Errorf("the window jumped from %q to %q, skipping every commit between",
			before[len(before)-1], after[0])
	}
}

func manyCommits(n int) []gh.Commit {
	out := make([]gh.Commit, 0, n)
	for i := range n {
		out = append(out, gh.Commit{
			SHA:         fmt.Sprintf("%010d", i),
			Short:       fmt.Sprintf("%07d", i),
			Headline:    "Commit number " + strconv.Itoa(i),
			Author:      gh.Actor{Login: "drucial"},
			CommittedAt: time.Now().Add(-time.Duration(n-i) * time.Hour),
		})
	}
	return out
}

func shownHeadlines(frame string) []string {
	var out []string
	for i, line := range columnLines(frame) {
		if i%2 == 0 && strings.TrimSpace(line) != "" {
			out = append(out, strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "●")))
		}
	}
	return out
}

func indexOf(lines []string, want string) int {
	for i, line := range lines {
		if line == want {
			return i
		}
	}
	return -1
}

func TestSwitchingTabsOpensTheCommitColumnOnARow(t *testing.T) {
	for _, height := range []int{9, 10, 11, 12, 13} {
		t.Run(strconv.Itoa(height), func(t *testing.T) {
			d := sampleDetail()
			d.Commits = manyCommits(40)

			m := detailed(held(d), 160, height)
			m.SetFiles(store.Files{Files: sampleFiles(), Status: store.StatusReady, Loaded: true})

			m = press(m, "]", "]", "]", "1")
			for range 9 {
				m = press(m, "j")
			}
			m = press(m, "]", "]")

			lines := columnLines(m.View())
			if len(lines) == 0 {
				t.Fatal("the commit column rendered nothing")
			}
			if !strings.Contains(lines[0], "●") {
				t.Errorf("the column opens on %q, want the top of a row", lines[0])
			}
		})
	}
}

func TestTheCommitsTabOpensWithTheColumnFocused(t *testing.T) {
	d := sampleDetail()
	d.Commits = sampleCommits()

	_, cmd := settled(press(detailed(held(d), 160, 24), "]", "j"), d.Commits[1].SHA)
	if cmd == nil {
		t.Fatal("no diff was asked for, so j never reached the column")
	}

	msg, ok := cmd().(prview.NeedCommitMsg)
	if !ok {
		t.Fatalf("settling yielded %T, want a request for a commit's diff", cmd())
	}
	if msg.SHA != d.Commits[1].SHA {
		t.Errorf("asked for %q, want the second commit", msg.SHA)
	}
}

func TestBraceWalksTheFilesInACommitDiff(t *testing.T) {
	m, _ := settled(onCommits(160, 12), "a3f91c2d5e")
	m.SetCommitFiles("a3f91c2d5e", commitDiff(sampleFiles()))

	first := stripANSI(press(m, "}").View())
	second := stripANSI(press(m, "}", "}").View())
	if first == second {
		t.Fatal("} did not move the commit's diff a file on")
	}

	if back := stripANSI(press(m, "}", "}", "{").View()); back != first {
		t.Error("{ did not come back to the file } left")
	}
}

func TestTheRailIsOffOnTheCommitsTab(t *testing.T) {
	for _, width := range []int{200, 160, 120} {
		if strings.Contains(stripANSI(onCommits(width, 24).View()), "Reviewers") {
			t.Errorf("the rail is on screen at %d columns", width)
		}
	}
}

func TestTheCommitColumnHidesOnANarrowFrame(t *testing.T) {
	for _, width := range []int{160, 100, 70} {
		if !strings.Contains(stripANSI(onCommits(width, 24).View()), "a3f91c2") {
			t.Errorf("the column is gone at %d columns", width)
		}
	}
	for _, width := range []int{69, 60, 40, 20} {
		if strings.Contains(stripANSI(onCommits(width, 24).View()), "a3f91c2") {
			t.Errorf("the column is still on screen at %d columns", width)
		}
	}
}

func TestTheCommitColumnOpensOnAWholeRow(t *testing.T) {
	for _, height := range []int{10, 11, 12, 13} {
		t.Run(strconv.Itoa(height), func(t *testing.T) {
			d := sampleDetail()
			d.Commits = append(sampleCommits(), sampleCommits()...)
			m := press(detailed(held(d), 160, height), "]", "1", "G")

			lines := columnLines(m.View())
			if len(lines) < 2 {
				t.Fatalf("the column rendered %d lines, want a row", len(lines))
			}
			for i, line := range lines {
				if strings.TrimSpace(line) == "" {
					break
				}
				if headline := strings.Contains(line, "●"); headline != (i%2 == 0) {
					t.Errorf("line %d is %q, want the column on a row boundary", i, line)
				}
			}
		})
	}
}

func TestTheFrameFillsItsSizeExactlyOnTheCommitsTab(t *testing.T) {
	sizes := []struct{ width, height int }{
		{width: 200, height: 40},
		{width: 160, height: 24},
		{width: 100, height: 20},
		{width: 60, height: 10},
		{width: 40, height: 10},
		{width: 30, height: 12},
		{width: 20, height: 8},
	}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			m, _ := settled(onCommits(size.width, size.height), "a3f91c2d5e")
			m.SetCommitFiles("a3f91c2d5e", commitDiff(sampleFiles()))

			lines := strings.Split(m.View(), "\n")
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

func TestARunOfPushesNamesEveryCommitUnderIt(t *testing.T) {
	run := sampleCommits()
	run[1].Author = run[0].Author

	d := sampleDetail()
	d.Commits = run
	d.Timeline = []gh.TimelineItem{
		commented("nkr", time.Now().Add(-20*time.Hour), "Looks close."),
		commitItem(run[0]),
		commitItem(run[1]),
	}

	out := stripANSI(detailed(held(d), 160, 30).View())
	if !strings.Contains(out, "drucial · pushed 2 commits · 18h") {
		t.Error("the run is not headed by its count")
	}
	for _, want := range []string{
		"a3f91c2  Cap the backoff",
		"7b20ef4  Drop the count",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the run is missing %q", want)
		}
	}

	head := strings.Index(out, "pushed 2 commits")
	first := strings.Index(out, "a3f91c2  Cap the backoff")
	if head < 0 || first < 0 || head > first {
		t.Error("the commits are not under the line that counts them")
	}
}

func TestALonePushHasNoRowUnderIt(t *testing.T) {
	d := sampleDetail()
	d.Commits = sampleCommits()[:1]
	d.Timeline = []gh.TimelineItem{commitItem(d.Commits[0])}

	out := stripANSI(detailed(held(d), 160, 30).View())
	if strings.Count(out, "a3f91c2") != 1 {
		t.Errorf("a lone push named its sha %d times, want once", strings.Count(out, "a3f91c2"))
	}
}

func TestALonePushNamesItsShaAndHeadline(t *testing.T) {
	d := sampleDetail()
	d.Commits = sampleCommits()[:1]
	d.Timeline = []gh.TimelineItem{commitItem(d.Commits[0])}

	out := stripANSI(detailed(held(d), 160, 30).View())
	if !strings.Contains(out, "drucial · pushed a3f91c2 Cap the backoff · 19h") {
		t.Error("a lone push did not name its commit")
	}
}

func TestARunByMoreThanOnePersonNamesNobody(t *testing.T) {
	d := sampleDetail()
	d.Commits = sampleCommits()
	d.Timeline = []gh.TimelineItem{commitItem(d.Commits[0]), commitItem(d.Commits[1])}

	out := stripANSI(detailed(held(d), 160, 30).View())
	if !strings.Contains(out, "● pushed 2 commits") {
		t.Error("a mixed run did not drop the author")
	}
	if strings.Contains(out, "drucial · pushed") || strings.Contains(out, "nkr · pushed") {
		t.Error("a mixed run credited one of its authors with the lot")
	}
}

func commitItem(c gh.Commit) gh.TimelineItem {
	return gh.TimelineItem{
		Kind:      gh.TimelineCommit,
		Actor:     c.Author,
		CreatedAt: c.CommittedAt,
		Commit:    &c,
	}
}

func columnLines(frame string) []string {
	var out []string
	for _, line := range strings.Split(stripANSI(frame), "\n") {
		cells := []rune(line)
		if len(cells) < 2 || cells[0] != '│' {
			continue
		}
		for i, r := range cells[1:] {
			if r == '│' {
				out = append(out, string(cells[1:1+i]))
				break
			}
		}
	}
	return out
}

func TestTheCommitDiffTakesNoRowCursor(t *testing.T) {
	m, _ := settled(onCommits(200, 24), "a3f91c2d5e")
	m.SetCommitFiles("a3f91c2d5e", commitDiff(sampleFiles()))
	m = press(m, "j", "j", "j")

	if !strings.Contains(stripANSI(m.View()), "delay = min(delay*2, fetchTimeout)") {
		t.Fatal("setup: the commit's diff is not on the screen")
	}
	if got := barredRow(m.View()); got != "" {
		t.Errorf("the commit diff barred %q, want no cursor at all", got)
	}
}

func TestTheCommitsHeadingKeepsItsIndentWhileFilesIsSplit(t *testing.T) {
	heading := func(split bool) string {
		t.Helper()

		d := sampleDetail()
		d.Commits = sampleCommits()
		m := detailed(held(d), 160, 40)
		m.SetFiles(loadedFiles(sampleFiles(), 0))

		m = press(m, "]", "]", "]")
		if split {
			m = press(m, "|")
		}
		m = press(m, "[", "[")

		m, _ = settled(m, "a3f91c2d5e")
		m.SetCommitFiles("a3f91c2d5e", commitDiff(sampleFiles()))
		m, _ = settled(m, "a3f91c2d5e")

		for _, line := range strings.Split(stripANSI(m.View()), "\n") {
			if strings.Contains(line, "@@ -40,4 +40,5 @@") {
				return strings.TrimRight(line, " │")
			}
		}
		t.Fatal("the commit's diff drew no @@ heading")
		return ""
	}

	if on, off := heading(true), heading(false); on != off {
		t.Errorf("the Commits heading moved because Files is split:\n split %q\n plain %q", on, off)
	}
}
