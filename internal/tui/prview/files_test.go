package prview_test

import (
	"errors"
	"fmt"
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/store"
	"github.com/zen-octo/zen-octo/internal/tui/prview"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// sampleFiles covers what the tab has to tell apart: nesting deep enough to
// fold, a rename, a file with no patch, and the lines sampleDetail's two review
// threads anchor to, one on each side of the diff.
func sampleFiles() []gh.ChangedFile {
	return []gh.ChangedFile{
		{
			Path: "internal/gh/client.go", Status: gh.FileModified, Additions: 2, Deletions: 1,
			Hunks: []gh.Hunk{{
				Header: "@@ -40,4 +40,5 @@ func New() (*Client, error) {",
				Lines: []gh.DiffLine{
					{Kind: gh.DiffContext, Old: 40, New: 40, Content: "\tfor {"},
					{Kind: gh.DiffRemoved, Old: 41, Content: "\t\ttime.Sleep(delay)"},
					{Kind: gh.DiffAdded, New: 41, Content: "\t\tdelay = min(delay*2, fetchTimeout)"},
					{Kind: gh.DiffAdded, New: 42, Content: "\t\ttime.Sleep(delay)"},
					{Kind: gh.DiffContext, Old: 42, New: 43, Content: "\t}"},
				},
			}},
		},
		{
			Path: "internal/store/store.go", Status: gh.FileModified, Additions: 0, Deletions: 1,
			Hunks: []gh.Hunk{{
				Header: "@@ -87,3 +87,2 @@",
				Lines: []gh.DiffLine{
					{Kind: gh.DiffContext, Old: 87, New: 87, Content: "// Begin marks one section in flight."},
					{Kind: gh.DiffRemoved, Old: 88, Content: "// It refuces a duplicate."},
					{Kind: gh.DiffContext, Old: 89, New: 88, Content: "}"},
				},
			}},
		},
		{
			Path: "internal/tui/prview/files.go", PreviousPath: "internal/tui/prview/diff.go",
			Status: gh.FileRenamed, Additions: 1, Deletions: 0,
			Hunks: []gh.Hunk{{
				Header: "@@ -1,2 +1,3 @@",
				Lines: []gh.DiffLine{
					{Kind: gh.DiffContext, Old: 1, New: 1, Content: "package prview"},
					{Kind: gh.DiffAdded, New: 2, Content: "const tabWidth = 4"},
				},
			}},
		},
		{
			Path: "docs/screenshot.png", Status: gh.FileModified,
			Omitted: "binary, or too large for GitHub to return a diff",
		},
	}
}

func loadedFiles(files []gh.ChangedFile, more int) store.Files {
	return store.Files{Files: files, MoreFiles: more, Status: store.StatusReady, Loaded: true}
}

// onFiles is the screen with a diff, sitting on the Files tab.
func onFiles(width, height int) prview.Model {
	m := detailed(held(sampleDetail()), width, height)
	m.SetFiles(loadedFiles(sampleFiles(), 0))
	return press(m, "]", "]", "]")
}

func TestTheFilesTabRendersTheDiff(t *testing.T) {
	out := stripANSI(onFiles(200, 50).View())

	for _, want := range []string{
		"internal/gh/client.go",
		"@@ -40,4 +40,5 @@ func New() (*Client, error) {",
		"delay = min(delay*2, fetchTimeout)",
		"docs/screenshot.png",
		"binary, or too large for GitHub to return a diff",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the Files tab does not show %q", want)
		}
	}
}

// The file column is what navigation hangs off, and a flat list of full paths
// is not a tree.
func TestTheFileColumnNestsThePathsAndFoldsASingleChildRun(t *testing.T) {
	out := stripANSI(onFiles(200, 50).View())

	for _, want := range []string{"internal/", "gh/", "store/", "tui/prview/", "docs/"} {
		if !strings.Contains(out, want) {
			t.Errorf("the tree does not show %q", want)
		}
	}
	// tui holds one directory, which holds one directory. Three rows and six
	// columns to say tui/prview is what makes a narrow column unreadable.
	if strings.Contains(out, "▾ tui/\n") {
		t.Error("tui/ printed on a row of its own instead of joining the run below it")
	}
}

func TestARenameShowsThePathItCameFrom(t *testing.T) {
	out := stripANSI(onFiles(200, 50).View())
	if !strings.Contains(out, "internal/tui/prview/diff.go → internal/tui/prview/files.go") {
		t.Error("the rename does not show the path it came from")
	}
}

// A line number that does not line up with the one above it is worse than none.
func TestTheGutterHoldsBothSidesLineNumbers(t *testing.T) {
	out := stripANSI(onFiles(200, 50).View())

	// The removed line has an old number and no new one; the added lines the
	// other way round.
	for _, want := range []string{"41    − ", "   41 + ", "40 40   "} {
		if !strings.Contains(out, want) {
			t.Errorf("the gutter does not show %q", want)
		}
	}
}

// A line of code folded onto a second row puts its tail under the gutter and
// every line below it out of step with its own number.
func TestALongCodeLineClipsRatherThanWraps(t *testing.T) {
	long := strings.Repeat("x", 400)
	files := []gh.ChangedFile{{
		Path: "a.go", Additions: 1,
		Hunks: []gh.Hunk{{Header: "@@ -1 +1 @@", Lines: []gh.DiffLine{
			{Kind: gh.DiffAdded, New: 1, Content: long},
		}}},
	}}

	m := detailed(held(sampleDetail()), 120, 30)
	m.SetFiles(loadedFiles(files, 0))
	m = press(m, "]", "]", "]")

	rows := 0
	for _, line := range strings.Split(m.View(), "\n") {
		if lipgloss.Width(line) != 120 {
			t.Fatalf("line %q is %d wide, want 120", stripANSI(line), lipgloss.Width(line))
		}
		if strings.Contains(stripANSI(line), "xxxx") {
			rows++
		}
	}
	if rows != 1 {
		t.Errorf("the long line covers %d rows, want 1", rows)
	}
}

// Reading a review means reading the comments against the lines they were
// written about, not scrolling back to the conversation for them.
func TestAReviewThreadRendersUnderTheLineItAnchorsTo(t *testing.T) {
	out := stripANSI(onFiles(200, 60).View())

	code := strings.Index(out, "delay = min(delay*2, fetchTimeout)")
	comment := strings.Index(out, "This backs off forever.")
	next := strings.Index(out, "internal/store/store.go")

	switch {
	case comment < 0:
		t.Fatal("the open thread is not on the Files tab at all")
	case comment < code:
		t.Error("the thread renders above the line it anchors to")
	case next > 0 && comment > next:
		t.Error("the thread renders under the wrong file")
	}
}

// A comment on a deleted line and one on an added line can carry the same
// number. Only the side tells them apart.
func TestAThreadOnTheLeftAnchorsToTheRemovedLine(t *testing.T) {
	out := stripANSI(onFiles(200, 60).View())

	removed := strings.Index(out, "// It refuces a duplicate.")
	resolved := strings.Index(out, "resolved")

	if removed < 0 || resolved < 0 {
		t.Fatal("the removed line or its thread is missing")
	}
	if resolved < removed {
		t.Error("the left-side thread did not follow the line it was written against")
	}
}

// An outdated thread anchors to a line the pull request has moved past.
// Dropping it loses the only record of what was asked.
func TestAThreadWithNoLineInTheDiffStillRenders(t *testing.T) {
	d := sampleDetail()
	d.Threads = append(d.Threads, gh.ReviewThread{
		Path: "internal/gh/client.go", Line: 900, Side: gh.SideRight, IsOutdated: true,
		Comments: []gh.Comment{{Author: gh.Actor{Login: "nkr"}, Body: "Long since moved."}},
	})

	m := detailed(held(d), 200, 60)
	m.SetFiles(loadedFiles(sampleFiles(), 0))
	m = press(m, "]", "]", "]")

	if !strings.Contains(stripANSI(m.View()), "Long since moved.") {
		t.Error("a thread with no line left in the diff vanished")
	}
}

func TestFoldingAFileTakesItsDiffWithIt(t *testing.T) {
	m := press(onFiles(200, 50), "1")
	if !strings.Contains(stripANSI(m.View()), "@@ -40,4 +40,5 @@") {
		t.Fatal("the first file's hunk is not on screen to fold")
	}

	// The tree opens on docs/, then screenshot.png, internal/, gh/, client.go.
	m = press(m, "j", "j", "j", "j", "o")
	if strings.Contains(stripANSI(m.View()), "@@ -40,4 +40,5 @@") {
		t.Error("folding the file left its hunk on screen")
	}
	if !strings.Contains(stripANSI(m.View()), "internal/gh/client.go") {
		t.Error("folding the file took its heading with it")
	}
}

func TestFoldingADirectoryTakesItsFilesOutOfTheTree(t *testing.T) {
	m := press(onFiles(200, 50), "1", "j", "j", "o")

	out := stripANSI(m.View())
	if strings.Contains(out, "▾ internal/ ") {
		t.Error("the directory still reads as open")
	}
	if strings.Contains(out, "client.go") {
		t.Error("folding internal/ left its files on screen")
	}
}

func TestSelectingAFileScrollsTheDiffToIt(t *testing.T) {
	// A frame short enough that the last file is well past the first screen.
	m := press(onFiles(200, 16), "1")
	if strings.Contains(stripANSI(m.View()), "const tabWidth = 4") {
		t.Fatal("the last file is already on screen; the test proves nothing")
	}

	m = press(m, "G")
	if !strings.Contains(stripANSI(m.View()), "const tabWidth = 4") {
		t.Error("selecting the file did not scroll the diff to it")
	}
}

// The keys that move further than a line have to move the cursor too. A file
// column scrolled away from its own cursor answers nothing.
func TestTheFileColumnKeepsItsCursorUnderTheJumpKeys(t *testing.T) {
	m := press(onFiles(200, 16), "1", "G")

	out := stripANSI(m.View())
	last := strings.Index(out, "files.go")
	if last < 0 {
		t.Fatal("the last file is not in the tree")
	}
	if !strings.Contains(selectedRow(m.View()), "files.go") {
		t.Errorf("the cursor is on %q, want the last file", stripANSI(selectedRow(m.View())))
	}

	if !strings.Contains(selectedRow(press(m, "g").View()), "docs/") {
		t.Error("g did not take the cursor back to the top")
	}
}

// The cursor says which file the diff is showing, which is the question the
// column exists to answer whether or not the keys are pointed at it.
func TestTheFileCursorStaysPaintedWithFocusOnTheDiff(t *testing.T) {
	m := press(onFiles(200, 40), "1", "j", "j")
	if !strings.Contains(selectedRow(m.View()), "internal/") {
		t.Fatal("the cursor is not where the test put it")
	}

	if !strings.Contains(selectedRow(press(m, "2").View()), "internal/") {
		t.Error("focusing the diff took the cursor off the file column")
	}
}

// selectedRow is the line carrying the selection background.
func selectedRow(frame string) string {
	for _, line := range strings.Split(frame, "\n") {
		if strings.Contains(line, selectionSeq()) {
			return line
		}
	}
	return ""
}

// The rail and the tree both want a column, and a diff between them stops
// reading as code well before either of them stops reading.
func TestTheRailStepsAsideForTheTreeOnANarrowFrame(t *testing.T) {
	tests := []struct {
		width int
		rail  bool
		tree  bool
	}{
		{width: 160, rail: true, tree: true},
		{width: 130, rail: false, tree: true},
		{width: 80, rail: false, tree: true},
		{width: 60, rail: false, tree: false},
	}

	for _, tt := range tests {
		m := detailed(held(sampleDetail()), tt.width, 40)
		m.SetFiles(loadedFiles(sampleFiles(), 0))
		out := stripANSI(press(m, "]", "]", "]").View())

		if got := strings.Contains(out, "Details"); got != tt.rail {
			t.Errorf("width %d: rail on screen = %v, want %v", tt.width, got, tt.rail)
		}
		// The tree pane is titled by what it holds; the diff beside it carries
		// the same paths in its own file headings.
		if got := strings.Contains(out, "4 files"); got != tt.tree {
			t.Errorf("width %d: tree on screen = %v, want %v", tt.width, got, tt.tree)
		}
	}
}

// The rail belongs to the conversation, and stepping aside for the diff must
// not read as having been turned off.
func TestTheRailComesBackWhenTheTabDoes(t *testing.T) {
	m := detailed(held(sampleDetail()), 130, 40)
	m.SetFiles(loadedFiles(sampleFiles(), 0))

	if !strings.Contains(stripANSI(m.View()), "Details") {
		t.Fatal("the rail is not up on the conversation at 130 columns")
	}
	m = press(m, "]", "]", "]")
	if strings.Contains(stripANSI(m.View()), "Details") {
		t.Fatal("the rail did not step aside on the Files tab")
	}
	if !strings.Contains(stripANSI(press(m, "]").View()), "Details") {
		t.Error("the rail did not come back when the tab moved on")
	}
}

func TestTheFrameFillsItsSizeExactlyOnTheFilesTab(t *testing.T) {
	sizes := []struct{ width, height int }{
		{200, 60}, {160, 40}, {144, 30}, {120, 24}, {100, 20}, {70, 16}, {60, 12}, {40, 8},
	}

	for _, size := range sizes {
		m := detailed(held(sampleDetail()), size.width, size.height)
		m.SetFiles(loadedFiles(sampleFiles(), 3))
		frame := press(m, "]", "]", "]").View()

		lines := strings.Split(frame, "\n")
		if len(lines) != size.height {
			t.Errorf("%dx%d: %d lines, want %d", size.width, size.height, len(lines), size.height)
		}
		for i, line := range lines {
			if lipgloss.Width(line) != size.width {
				t.Errorf("%dx%d: line %d is %d wide, want %d",
					size.width, size.height, i, lipgloss.Width(line), size.width)
			}
		}
	}
}

// The panes are numbered by where they sit, so the digits have to follow what
// is on screen rather than what each pane holds.
func TestThePanesAreNumberedLeftToRight(t *testing.T) {
	out := stripANSI(onFiles(200, 40).View())

	tree := strings.Index(out, "[1]")
	diff := strings.Index(out, "[2]")
	rail := strings.Index(out, "[3]")

	if tree < 0 || diff < 0 || rail < 0 {
		t.Fatalf("want three numbered panes, got %q", firstLine(out))
	}
	if tree > diff || diff > rail {
		t.Error("the pane numbers do not run left to right")
	}
}

// Leaving the Files tab takes the tree with it, and the movement keys cannot
// keep driving a pane that is no longer on screen.
func TestFocusLeavesTheTreeWhenTheTabDoes(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 20)
	m.SetFiles(loadedFiles(sampleFiles(), 0))

	m = press(m, "]", "]", "]", "1") // to Files, focus the tree
	m = press(m, "]")                // round to the conversation

	before := footerOf(t, m.View())
	if after := footerOf(t, press(m, "j", "j", "j").View()); after == before {
		t.Errorf("the conversation did not scroll: footer stayed at %q", before)
	}
}

// A digit with no pane behind it does nothing rather than focusing something
// that is not there.
func TestADigitPastTheLastPaneIsIgnored(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 20)

	before := footerOf(t, m.View())
	m = press(m, "3", "j", "j", "j")
	if footerOf(t, m.View()) == before {
		t.Error("3 moved focus off the conversation on a screen with two panes")
	}
}

func TestOverflowIsReportedRatherThanDropped(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 60)
	m.SetFiles(loadedFiles(sampleFiles(), 3))

	if !strings.Contains(stripANSI(press(m, "]", "]", "]").View()), "3 more files on GitHub") {
		t.Error("the files the page did not reach go unreported")
	}
}

func TestTheDiffSpinsUntilItLands(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 40)
	m.SetFiles(store.Files{Status: store.StatusLoading})

	if !strings.Contains(stripANSI(press(m, "]", "]", "]").View()), "Loading the diff") {
		t.Error("the Files tab says nothing while the diff is on its way")
	}
}

func TestAFailedDiffSaysWhy(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 40)
	m.SetFiles(store.Files{Status: store.StatusFailed, Err: errors.New("context deadline exceeded")})

	out := stripANSI(press(m, "]", "]", "]").View())
	if !strings.Contains(out, "Could not load the diff: context deadline exceeded") {
		t.Error("a failed diff reads as an empty one")
	}
}

func TestAPullRequestWithNoFilesSaysSo(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 40)
	m.SetFiles(loadedFiles(nil, 0))

	if !strings.Contains(stripANSI(press(m, "]", "]", "]").View()), "No files changed.") {
		t.Error("an empty diff renders as a blank pane")
	}
}

// Selection is baked into every cell of the row. Wrapping a joined row instead
// paints only its first cell, because every styled run ends in a reset that
// clears the background with it.
func TestTheSelectedFileIsPaintedCellByCell(t *testing.T) {
	m := press(onFiles(200, 40), "1")

	selected := selectedRow(m.View())
	if selected == "" {
		t.Fatal("no row carries the selection background")
	}
	if got := strings.Count(selected, selectionSeq()); got < 3 {
		t.Errorf("the selection appears %d times on the row, want it on every cell", got)
	}
}

func selectionSeq() string {
	r, g, b, _ := theme.RosePineMoon.SelectedBackground.RGBA()
	return fmt.Sprintf("48;2;%d;%d;%d", r>>8, g>>8, b>>8)
}

// A changed line is read as a block, not a character at a time. The tint has to
// run the whole width, and it is painted per cell because every styled run ends
// in a reset that clears the background with it.
func TestAChangedLineIsTintedEdgeToEdge(t *testing.T) {
	frame := onFiles(200, 50).View()

	added, removed := "", ""
	for _, line := range strings.Split(frame, "\n") {
		switch {
		case strings.Contains(line, bgSeq(theme.RosePineMoon.AddedBackground)) && added == "":
			added = line
		case strings.Contains(line, bgSeq(theme.RosePineMoon.RemovedBackground)) && removed == "":
			removed = line
		}
	}

	if added == "" || removed == "" {
		t.Fatal("no line carries an added or removed background")
	}
	for _, tt := range []struct {
		name string
		line string
		seq  string
	}{
		{"added", added, bgSeq(theme.RosePineMoon.AddedBackground)},
		{"removed", removed, bgSeq(theme.RosePineMoon.RemovedBackground)},
	} {
		if got := strings.Count(tt.line, tt.seq); got < 5 {
			t.Errorf("the %s tint appears %d times, want it on every cell", tt.name, got)
		}
		// The frame is three panes wide, so the tint covers the diff's own
		// width rather than the line's. Anything short of that is a hole.
		if got := lipgloss.Width(tinted(tt.line, tt.seq)); got < 100 {
			t.Errorf("the %s tint covers %d cells, want it running to the border", tt.name, got)
		}
	}
}

// A context line has no tint to run out, so it must not be filled: the trailing
// spaces would be indistinguishable from a change with no color.
func TestAContextLineIsNotTinted(t *testing.T) {
	frame := stripANSI(onFiles(200, 50).View())

	for _, line := range strings.Split(frame, "\n") {
		if !strings.Contains(line, "40 40   ") {
			continue
		}
		if strings.Contains(line, bgSeq(theme.RosePineMoon.AddedBackground)) {
			t.Error("a context line came back tinted")
		}
		return
	}
	t.Fatal("the context line is not on screen")
}

// A run of hunks with nothing between them reads as one file. The box is what
// says where one ends.
func TestEachFileSitsInABoxWithItsOwnHeader(t *testing.T) {
	lines := strings.Split(stripANSI(onFiles(200, 50).View()), "\n")

	head, rule := -1, -1
	for i, line := range lines {
		if head < 0 && strings.Contains(line, "internal/gh/client.go") && strings.Contains(line, "+2") {
			head = i
			continue
		}
		if head >= 0 && strings.Contains(line, "├─") {
			rule = i
			break
		}
	}

	if head < 0 {
		t.Fatal("no heading row carries the path and the churn")
	}
	if rule != head+1 {
		t.Errorf("the rule is %d rows under the heading, want 1", rule-head)
	}
	if !strings.Contains(lines[head-1], "╭") {
		t.Error("the heading has no box around it")
	}
}

// tinted is the text on a line the given background does cover. It walks the
// SGR runs rather than the text, which is the only way to tell a painted cell
// from a bare one.
func tinted(line, seq string) string {
	var out strings.Builder
	painted := false

	for len(line) > 0 {
		i := strings.IndexByte(line, 0x1b)
		if i < 0 {
			if painted {
				out.WriteString(line)
			}
			break
		}
		if painted {
			out.WriteString(line[:i])
		}

		end := strings.IndexByte(line[i:], 'm')
		if end < 0 {
			break
		}
		painted = strings.Contains(line[i:i+end], seq)
		line = line[i+end+1:]
	}
	return out.String()
}

func bgSeq(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("48;2;%d;%d;%d", r>>8, g>>8, b>>8)
}
