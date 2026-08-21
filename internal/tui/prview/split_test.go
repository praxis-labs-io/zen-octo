package prview_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

// codeRows is the drawn rows of source: everything between the hunk heading and
// the first card under it, stripped.
func codeRows(frame string) []string {
	var out []string
	seen := false
	for _, line := range strings.Split(frame, "\n") {
		plain := stripANSI(line)
		switch {
		case strings.Contains(plain, "@@"):
			seen = true
		case !seen:
		case strings.Contains(plain, "╭"):
			return out
		default:
			out = append(out, plain)
		}
	}
	return out
}

// splitRows is the code rows of a diff drawing two columns, which carry the rule
// between them on top of the four borders every row has.
func splitRows(frame string) []string {
	var out []string
	for _, row := range codeRows(frame) {
		if strings.Count(row, "│") == 5 {
			out = append(out, row)
		}
	}
	return out
}

// A run of removals pairs against the additions after it, one row each, and the
// shorter side draws a blank rather than shifting the columns out of step.
func TestSideBySidePairsARunOfRemovalsAgainstTheAdditions(t *testing.T) {
	m := press(onFiles(140, 30), "|")
	rows := splitRows(m.View())
	if len(rows) < 3 {
		t.Fatalf("the diff drew %d two-column rows:\n%s", len(rows), m.View())
	}

	// The fixture removes one line and adds two, under a line of context.
	want := []struct{ left, right string }{
		{"for {", "for {"},
		{"time.Sleep(delay)", "delay = min(delay*2, fetchTimeout)"},
		{"", "time.Sleep(delay)"},
	}
	for i, w := range want {
		left, right, ok := halvesOf(rows[i])
		if !ok {
			t.Fatalf("row %d has no rule in it: %q", i, rows[i])
		}
		if !strings.Contains(left, w.left) || (w.left == "" && strings.TrimSpace(left) != "") {
			t.Errorf("row %d base column is %q, want %q", i, strings.TrimSpace(left), w.left)
		}
		if !strings.Contains(right, w.right) {
			t.Errorf("row %d head column is %q, want %q", i, strings.TrimSpace(right), w.right)
		}
	}
}

// halvesOf splits a drawn row into its two columns. The bars are the tree's two
// borders, the pane's left, the rule, and the pane's right.
func halvesOf(row string) (string, string, bool) {
	var at []int
	for i, r := range []rune(row) {
		if r == '│' {
			at = append(at, i)
		}
	}
	if len(at) != 5 {
		return "", "", false
	}

	cells := []rune(row)
	return string(cells[at[2]+1 : at[3]]), string(cells[at[3]+1 : at[4]]), true
}

// Every row has to be exactly the pane, or the column on the right walks in and
// out as the file goes on. An odd width is where the halving lands wrong.
func TestEverySideBySideRowIsExactlyThePane(t *testing.T) {
	for _, width := range []int{110, 111, 140, 141} {
		m := press(onFiles(width, 30), "|")
		rows := splitRows(m.View())
		if len(rows) == 0 {
			t.Fatalf("width %d drew no two-column rows", width)
		}
		for i, row := range rows {
			if got := lipgloss.Width(row); got != width {
				t.Errorf("width %d: row %d painted %d cells: %q", width, i, got, row)
			}
		}
	}
}

// The key refuses rather than drawing two columns of nothing, and says how many
// columns short the pane is.
func TestSideBySideIsRefusedInAPaneTooNarrowForIt(t *testing.T) {
	m := onFiles(70, 20)
	after, cmd := m.Update(tea.KeyPressMsg{Code: '|', Text: "|"})

	if rows := splitRows(after.View()); len(rows) > 0 {
		t.Errorf("a pane of 70 drew %d two-column rows", len(rows))
	}
	if cmd == nil {
		t.Fatal("the refusal said nothing")
	}
	msg, ok := cmd().(prview.SplitTooNarrowMsg)
	if !ok {
		t.Fatalf("the key answered with %T, want a SplitTooNarrowMsg", cmd())
	}
	if msg.Short <= 0 {
		t.Errorf("the refusal is %d columns short, want a number to widen by", msg.Short)
	}
}

// A terminal shrinking under a split pane keeps the answer, so widening brings
// the columns back without a second press.
func TestANarrowedPaneFallsBackToUnifiedAndComesBack(t *testing.T) {
	m := press(onFiles(140, 30), "|")
	if len(splitRows(m.View())) == 0 {
		t.Fatal("setup: the pane did not split")
	}

	m.SetSize(70, 20)
	if rows := splitRows(m.View()); len(rows) > 0 {
		t.Errorf("the narrowed pane still drew %d two-column rows", len(rows))
	}

	m.SetSize(140, 30)
	if len(splitRows(m.View())) == 0 {
		t.Error("widening did not bring the columns back")
	}
}

// h and l step the columns before they give the focus up, and only the column
// the cursor is in lights.
func TestHAndLStepTheColumnsBeforeTheyLeaveThePane(t *testing.T) {
	m := press(onFiles(140, 30), "|", "}", "j")
	head := barredRow(m.View())
	if head == "" {
		t.Fatal("setup: nothing is barred")
	}

	base := press(m, "h")
	if got := barredRow(base.View()); got == head {
		t.Fatalf("h did not move the bar off the head column: %q", got)
	}

	// The bar is in the base column now, which is left of the rule.
	row := barredRow(base.View())
	if at := strings.Index(row, "▌"); at < 0 || at > strings.Index(row[at:], "│")+at {
		t.Errorf("the bar is not in the base column: %q", row)
	}
	if got := barredRow(press(base, "l").View()); got != head {
		t.Errorf("l back landed on %q, want %q", got, head)
	}
}

// insertOnly is a hunk the base column has no line in at all, which is the one
// shape a column step has nowhere to land in.
func insertOnly() []gh.ChangedFile {
	return []gh.ChangedFile{{
		Path: "internal/gh/client.go", Status: gh.FileModified, Additions: 3,
		Hunks: []gh.Hunk{{
			Header: "@@ -40,0 +41,3 @@ func New() (*Client, error) {",
			Lines: []gh.DiffLine{
				{Kind: gh.DiffAdded, New: 41, Content: "\t\tfirst := 1"},
				{Kind: gh.DiffAdded, New: 42, Content: "\t\tsecond := 2"},
				{Kind: gh.DiffAdded, New: 43, Content: "\t\tthird := 3"},
			},
		}},
	}}
}

// A file with every line on one side has one column worth standing in, and
// walkColumn already walks the reader in it. Stepping to the empty one moves
// m.column and nothing else, so the key showed nothing and left the file column
// a second press away. The render is what says the step took.
func TestHOnAOneSidedBlockLeavesForTheFileColumnOnTheFirstPress(t *testing.T) {
	m := detailed(held(sampleDetail()), 160, 40)
	m.SetFiles(store.Files{Files: insertOnly(), Status: store.StatusReady, Loaded: true})
	m = press(m, "]", "]", "]", "|", "}", "j")

	before := stripANSI(m.View())
	if barredRow(before) == "" {
		t.Fatal("the walk into the head column painted no bar to begin with")
	}
	if after := stripANSI(press(m, "h").View()); after == before {
		t.Error("h changed nothing at all: the key was taken and the frame is identical")
	}
}

// splitting() reads a remembered file and a width, and both outlive a tab
// change. Ungated, h on another tab's main pane is swallowed as a column step
// and moves the Files tab's column behind the reader's back, which they meet on
// the next walk there rather than on the key they pressed.
func TestTheColumnsAreTheFilesTabsAlone(t *testing.T) {
	// The walk is made again after the trip, because a tab change gives the row
	// cursor up on its own and the column is what this is asking about.
	walk := func(keys ...string) string {
		d := sampleDetail()
		d.Commits = sampleCommits()
		m := detailed(held(d), 160, 40)
		m.SetFiles(loadedFiles(sampleFiles(), 0))

		m = press(m, "]", "]", "]", "|")
		m = press(m, "[", "[")
		m = press(m, keys...)
		return barredRow(press(m, "]", "]", "}", "j").View())
	}

	pressed, untouched := walk("l", "h"), walk()
	if untouched == "" {
		t.Fatal("setup: the walk back on the Files tab painted no bar")
	}
	if pressed != untouched {
		t.Errorf("h on the Commits tab moved the Files column:\n pressed   %q\n untouched %q", pressed, untouched)
	}
}

// On a block the two columns draw the same frame, so a column step there is a
// press that shows the reader nothing and leaves the file column one further
// away than it looks.
func TestHOnABlockLeavesForTheFileColumnOnTheFirstPress(t *testing.T) {
	m := press(onFiles(140, 30), "|", "}")

	before := stripANSI(m.View())
	m = press(m, "h")
	if after := stripANSI(m.View()); after == before {
		t.Error("h changed nothing at all: the key was taken and the frame is identical")
	}
}

// The reader asked for a mode, not for a fact about request ordering. The diff
// is a second request, so ] to Files and | straight after is the common case,
// and refusing it silently made the key need a second press once the files
// landed.
func TestSplitPressedBeforeTheDiffLandsAppliesWhenItDoes(t *testing.T) {
	m := press(detailed(held(sampleDetail()), 160, 40), "]", "]", "]")
	m = press(m, "|")

	m.SetFiles(loadedFiles(sampleFiles(), 0))
	if rows := splitRows(m.View()); len(rows) == 0 {
		t.Error("the diff drew unified: the key pressed before it loaded was dropped")
	}
}
