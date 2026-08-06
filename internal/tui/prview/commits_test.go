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

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/store"
	"github.com/zen-octo/zen-octo/internal/tui/prview"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// sampleCommits covers what the column has to tell apart: the three check
// states, and an author GitHub has no account for.
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

// onCommits is the screen with a detail loaded, sitting on the Commits tab.
func onCommits(width, height int) prview.Model {
	d := sampleDetail()
	d.Commits = sampleCommits()
	return press(detailed(held(d), width, height), "]")
}

func enter(m prview.Model) (prview.Model, tea.Cmd) {
	return m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
}

// commitDiff is a commit's diff the way the store hands one over.
func commitDiff(files []gh.ChangedFile) store.Files {
	return store.Files{Files: files, Status: store.StatusReady, Loaded: true}
}

func TestTheCommitColumnNamesEveryCommit(t *testing.T) {
	out := stripANSI(onCommits(160, 24).View())

	for _, want := range []string{
		"a3f91c2 Cap the backoff",
		"@drucial · 19h",
		"7b20ef4 Drop the count",
		"@nkr · 18h",
		"c1d8a04 Fix the typo",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the column is missing %q", want)
		}
	}

	if !strings.Contains(out, "3 commits") {
		t.Error("the column is not titled with its count")
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

// The marker is the one cell the column spends on where a commit's checks got
// to, so the color is the whole of the signal.
func TestTheCheckMarkerTakesEachCommitsOwnState(t *testing.T) {
	out := onCommits(160, 24).View()

	th := theme.RosePineMoon
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

// marked reports whether a dot is painted in a foreground. The selected row
// carries a background in the same sequence, so the color is not always the
// last thing before the m.
func marked(frame, fg string) bool {
	return regexp.MustCompile(regexp.QuoteMeta(fg) + `(;[0-9;]+)?m●`).MatchString(frame)
}

func TestSelectingACommitAsksForItsDiff(t *testing.T) {
	m := onCommits(160, 24)
	m = press(m, "1", "j")

	_, cmd := enter(m)
	if cmd == nil {
		t.Fatal("enter produced no command, want a request for the diff")
	}

	msg, ok := cmd().(prview.NeedCommitMsg)
	if !ok {
		t.Fatalf("enter produced %T, want a NeedCommitMsg", cmd())
	}
	if msg.SHA != "7b20ef4a11" {
		t.Errorf("asked for %q, want the commit under the cursor", msg.SHA)
	}
}

func TestSelectingTheCommitAlreadyShowingAsksAgainForNothing(t *testing.T) {
	m, _ := enter(onCommits(160, 24))

	if _, cmd := enter(m); cmd != nil {
		t.Error("enter on the commit already showing asked for it a second time")
	}
}

func TestMovingTheCursorAsksForNothing(t *testing.T) {
	m := press(onCommits(160, 24), "1", "j", "j", "k")
	if m.View() == "" {
		t.Fatal("the screen rendered nothing")
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"}); cmd != nil {
		t.Error("moving the cursor asked for a diff")
	}
}

func TestTheCommitDiffRendersThroughTheFilesViewer(t *testing.T) {
	m, _ := enter(onCommits(160, 24))
	m.SetCommitFiles("a3f91c2d5e", commitDiff(sampleFiles()))

	out := stripANSI(m.View())
	if !strings.Contains(out, "internal/gh/client.go") {
		t.Error("the commit's diff did not render its file heading")
	}
	if !strings.Contains(out, "delay = min(delay*2, fetchTimeout)") {
		t.Error("the commit's diff did not render its code")
	}
}

// A diff for a commit the cursor has moved on from must not land on the screen:
// the reader asked for a different one and is waiting on it.
func TestADiffForAnotherCommitIsDropped(t *testing.T) {
	m, _ := enter(onCommits(160, 24))
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
			m, _ := enter(onCommits(160, 24))
			m.SetCommitFiles("a3f91c2d5e", c.held)

			if out := stripANSI(m.View()); !strings.Contains(out, c.want) {
				t.Errorf("the diff pane does not say %q", c.want)
			}
		})
	}
}

func TestNothingSelectedSaysWhichKeySelects(t *testing.T) {
	out := stripANSI(onCommits(160, 24).View())
	if !strings.Contains(out, "Press enter to show a commit's diff.") {
		t.Error("the diff pane does not say how to fill it")
	}
}

// Every styled run ends in a reset that clears the background with it, so a row
// painted as one string would carry its selection only as far as the first
// token. Both lines of the row have to hold it the whole way across.
func TestTheSelectedCommitIsPaintedCellByCellAcrossBothLines(t *testing.T) {
	m := onCommits(160, 24)
	seq := bgSeq(theme.RosePineMoon.SelectedBackground)

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

// The cursor walks rows, and a row is two lines. An offset that lands between
// them opens the column on a row's second line with its sha cut off above.
//
// The odd height is the one that catches it: an even one lands on a boundary by
// accident. The list runs past the window on both so the scroll is a real one
// rather than a clamp to the end, which lands on a boundary by accident too.
func TestTheCommitCursorScrollsAWholeRowAtATime(t *testing.T) {
	for _, height := range []int{6, 7} {
		t.Run(strconv.Itoa(height), func(t *testing.T) {
			d := sampleDetail()
			d.Commits = append(sampleCommits(), sampleCommits()...)
			m := press(detailed(held(d), 160, height), "]", "1", "j", "j")

			column := columnLines(m.View())
			if len(column) < 4 {
				t.Fatalf("the column rendered %d lines, want two whole rows", len(column))
			}

			// The window opens on a row's first line, not the author line under it.
			if !strings.Contains(column[0], "7b20ef4") {
				t.Errorf("the column opens on %q, want the top of a row", column[0])
			}
			if !strings.Contains(column[2], "c1d8a04") || !strings.Contains(column[3], "Drew White") {
				t.Error("the cursor's row is not on screen whole")
			}
		})
	}
}

func TestBraceWalksTheFilesInACommitDiff(t *testing.T) {
	m, _ := enter(onCommits(160, 12))
	m.SetCommitFiles("a3f91c2d5e", commitDiff(sampleFiles()))

	// The first } lands on the first file, since the pane opens on the blank
	// line above it. The second is the one that moves a file.
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

// The column is the only thing on the tab naming a commit. Hiding it would
// leave the diff beside it stuck on whatever it happened to be showing, so it
// narrows at every width rather than going away.
func TestTheCommitColumnStaysAtEveryWidth(t *testing.T) {
	for _, width := range []int{160, 100, 70, 60} {
		if !strings.Contains(stripANSI(onCommits(width, 24).View()), "a3f91c2") {
			t.Errorf("the column is gone at %d columns", width)
		}
	}
}

func TestTheFrameFillsItsSizeExactlyOnTheCommitsTab(t *testing.T) {
	sizes := []struct{ width, height int }{
		{width: 200, height: 40},
		{width: 160, height: 24},
		{width: 100, height: 20},
		{width: 60, height: 10},
	}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			m, _ := enter(onCommits(size.width, size.height))
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

// A run of pushes folds to one line the way GitHub folds it. Forty commits
// spelled out one per line bury the discussion they sit between.
func TestARunOfPushesFoldsToOneLine(t *testing.T) {
	run := sampleCommits()
	run[1].Author = run[0].Author

	d := sampleDetail()
	d.Commits = run
	d.Timeline = []gh.TimelineItem{
		{Kind: gh.TimelineComment, Actor: gh.Actor{Login: "nkr"},
			CreatedAt: time.Now().Add(-20 * time.Hour), Body: "Looks close."},
		commitItem(run[0]),
		commitItem(run[1]),
	}

	out := stripANSI(detailed(held(d), 160, 30).View())
	if !strings.Contains(out, "drucial · pushed 2 commits · 18h") {
		t.Error("a run of pushes did not fold to one line")
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

// Crediting one person for someone else's commits is worse than crediting
// nobody, so a mixed run drops the name and keeps the count.
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

// columnLines is the left column's rows, with the borders and the pane beside
// it cut away.
func columnLines(frame string) []string {
	var out []string
	for _, line := range strings.Split(stripANSI(frame), "\n") {
		cells := []rune(line)
		if len(cells) < 2 || cells[0] != '│' {
			continue
		}
		if at := strings.IndexRune(string(cells[1:]), '│'); at >= 0 {
			out = append(out, string(cells[1:1+at]))
		}
	}
	return out
}
