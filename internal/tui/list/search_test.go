package list_test

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/config"
	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/list"
	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

// searchPromptText is the box's prompt, spelled here rather than imported: the
// test reads the rendered row the way a person does.
const searchPromptText = "/ "

var (
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
	esc   = tea.KeyPressMsg{Code: tea.KeyEscape}
)

// typed presses one key per rune, which is what the bar sees.
func typed(m list.Model, s string) list.Model {
	for _, r := range s {
		m = press(m, key(r))
	}
	return m
}

// searchBox is the query line inside the box, or "" when the pane draws none.
// The box is three lines and only the middle one holds anything.
func searchBox(frame string) string {
	for _, line := range strings.Split(stripANSI(frame), "\n") {
		if strings.Contains(line, "│ / ") {
			return line
		}
	}
	return ""
}

// boxBorders is the box's own top and bottom, which is what says a box was
// drawn at all rather than a row of text.
func boxBorders(frame string) int {
	n := 0
	for _, line := range strings.Split(stripANSI(frame), "\n") {
		if strings.Contains(line, "╭─") && strings.Contains(line, "─╮ ") {
			n++
		}
		if strings.Contains(line, "╰─") && strings.Contains(line, "─╯ ") {
			n++
		}
	}
	return n
}

// mixed is a section whose rows differ in every field the search reads, so one
// query can be aimed at exactly one of them.
func mixed() []gh.PullRequest {
	prs := numbered(4)
	prs[0].Number = 1204
	prs[1].Repository = "praxis-labs-io/other"
	prs[2].Title = "Fix the auth retry"
	prs[3].Author = gh.Actor{Login: "octocat"}
	prs[3].HeadRefName = "feature/zno-94-search"
	return prs
}

func TestSlashOpensTheSearchBarAndEscTakesItAway(t *testing.T) {
	m := newList(90, 20, numbered(4))

	if bar := searchBox(m.View()); bar != "" {
		t.Fatalf("the bar is drawn before it was asked for: %q", bar)
	}

	m = press(m, key('/'))
	if bar := searchBox(m.View()); bar == "" {
		t.Errorf("no bar after /\n%s", stripANSI(m.View()))
	}
	if !m.Capturing() {
		t.Error("the bar is drawn but the root was not told it has the keyboard")
	}

	m = press(m, esc)
	if bar := searchBox(m.View()); bar != "" {
		t.Errorf("bar = %q, want it gone after esc", bar)
	}
	if m.Capturing() {
		t.Error("the bar is gone and the root still thinks it has the keyboard")
	}
}

func TestTypingNarrowsTheSectionAndTheBarCountsWhatItLeftOut(t *testing.T) {
	m := typed(press(newList(90, 20, mixed()), key('/')), "other")

	out := stripANSI(m.View())
	if strings.Contains(out, "Change 0") {
		t.Errorf("a row the query does not match is still on the list\n%s", out)
	}
	if !strings.Contains(out, "Change 1") {
		t.Errorf("the row the query matches is not on the list\n%s", out)
	}
	if bar := searchBox(m.View()); !strings.Contains(bar, "other") || !strings.Contains(bar, "1 of 4") {
		t.Errorf("bar = %q, want the query and 1 of 4 on it", bar)
	}
}

// The query is one substring over the whole row rather than a field at a time:
// a reader types what they can see, and what they can see is a line.
func TestTheSearchReadsTheNumberRepoTitleAuthorAndBranch(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{name: "number", query: "#1204", want: "Change 0"},
		{name: "number without the hash", query: "1204", want: "Change 0"},
		{name: "repository", query: "labs-io/other", want: "Change 1"},
		{name: "title", query: "auth retry", want: "Fix the auth retry"},
		{name: "author", query: "@octocat", want: "Change 3"},
		{name: "head branch", query: "zno-94", want: "Change 3"},
		{name: "case folded", query: "AUTH RETRY", want: "Fix the auth retry"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := typed(press(newList(110, 20, mixed()), key('/')), c.query)

			if bar := searchBox(m.View()); !strings.Contains(bar, "1 of 4") {
				t.Fatalf("bar = %q, want %q to match one row", bar, c.query)
			}
			if out := stripANSI(m.View()); !strings.Contains(out, c.want) {
				t.Errorf("%q matched a row, and it is not %q\n%s", c.query, c.want, out)
			}
		})
	}
}

// The bar takes every key while it has the keyboard. "]" and "s" are characters
// in a search, and a key that both types and changes tab does the wrong one.
func TestTheBarTakesTheKeysThatWouldOtherwiseActOnTheList(t *testing.T) {
	m := list.New(theme.RosePineMoon)
	m.SetSize(110, 20)
	m.SetSections(ready([]string{"Mine", "Review"}, numbered(4), numbered(6)[4:]))

	m = press(m, key('/'))
	for _, k := range []tea.KeyPressMsg{key(']'), key('['), key('s'), key('j')} {
		next, cmd := m.Update(k)
		if cmd != nil {
			t.Errorf("%q left the bar and asked the root for something", k.String())
		}
		m = next
	}

	if bar := searchBox(m.View()); !strings.Contains(bar, "][sj") {
		t.Errorf("bar = %q, want the four keys typed into it", bar)
	}
	if top := strings.Split(stripANSI(m.View()), "\n")[0]; !strings.Contains(top, "Mine (4)") {
		t.Errorf("tab strip = %q, want the tab unchanged", top)
	}
}

// Enter hands the keyboard back and leaves the filter standing. The bar stays
// drawn: a filter nothing accounts for is a list that looks like it lost rows.
func TestEnterKeepsTheFilterAndHandsTheKeyboardBack(t *testing.T) {
	m := press(typed(press(newList(110, 20, mixed()), key('/')), "change"), enter)

	if m.Capturing() {
		t.Error("enter left the bar holding the keyboard")
	}
	if bar := searchBox(m.View()); !strings.Contains(bar, "change") {
		t.Errorf("bar = %q, want the filter still named on it", bar)
	}

	if out := stripANSI(m.View()); strings.Contains(out, "Fix the auth retry") {
		t.Errorf("a row the query does not match came back with the keyboard\n%s", out)
	}
	if got := selectedRow(t, m.View()); !strings.Contains(got, "Change 1") {
		t.Fatalf("selection = %q, want the first row the filter left", got)
	}

	m = press(m, key('j'))
	if got := selectedRow(t, m.View()); !strings.Contains(got, "Change 0") {
		t.Errorf("selection = %q, want j to walk what the search left", got)
	}
	if pr, ok := m.Selected(); !ok || pr.Title != "Change 0" {
		t.Errorf("selected = %+v, want the row the cursor is painted on", pr)
	}
}

func TestEscClearsAFilterThatHasAlreadyBeenApplied(t *testing.T) {
	m := press(typed(press(newList(110, 20, mixed()), key('/')), "other"), enter)

	m = press(m, esc)
	if bar := searchBox(m.View()); bar != "" {
		t.Errorf("bar = %q, want it gone in the one press", bar)
	}
	if out := stripANSI(m.View()); !strings.Contains(out, "Change 0") {
		t.Errorf("the rows the filter took are still gone\n%s", out)
	}
}

// The query is the reader's, not the section's. It survives a tab and applies
// to whatever is arrived at.
func TestTheQuerySurvivesATabSwitch(t *testing.T) {
	second := numbered(4)
	for i := range second {
		second[i].ID = "OTHER_" + second[i].ID
		second[i].Repository = "praxis-labs-io/other"
	}

	m := list.New(theme.RosePineMoon)
	m.SetSize(110, 20)
	m.SetSections(ready([]string{"Mine", "Review"}, mixed(), second))

	m = press(press(typed(press(m, key('/')), "other"), enter), key(']'))

	if bar := searchBox(m.View()); !strings.Contains(bar, "4 of 4") {
		t.Errorf("bar = %q, want the query re-counted against the section arrived at", bar)
	}
}

func TestASearchThatMatchesNothingSaysSoRatherThanTheSection(t *testing.T) {
	m := typed(press(newList(110, 20, mixed()), key('/')), "zzz")

	out := stripANSI(m.View())
	if !strings.Contains(out, "Nothing in this section matches that search.") {
		t.Errorf("body says nothing about the search\n%s", out)
	}
	if strings.Contains(out, "Nothing matches this section.") {
		t.Errorf("an empty result reads as an empty section\n%s", out)
	}
	if bar := searchBox(m.View()); !strings.Contains(bar, "0 of 4") {
		t.Errorf("bar = %q, want 0 of 4", bar)
	}
}

// A filter can take the row the cursor was parked on. It lands on one the
// filter left rather than on a pull request nothing is drawing.
func TestTheCursorLandsOnARowTheFilterLeft(t *testing.T) {
	m := press(newList(110, 20, mixed()), key('j'), key('j'), key('j'))
	if got := selectedRow(t, m.View()); !strings.Contains(got, "Change 3") {
		t.Fatalf("setup: selection = %q, want it on the last row", got)
	}

	m = typed(press(m, key('/')), "auth retry")

	got := selectedRow(t, m.View())
	if !strings.Contains(got, "Fix the auth retry") {
		t.Errorf("selection = %q, want the one row the filter left", got)
	}
	if pr, ok := m.Selected(); !ok || pr.Title != "Fix the auth retry" {
		t.Errorf("selected = %+v, want the row on screen", pr)
	}
}

// A section showing a block instead of its rows has nothing to narrow, which is
// the rule every other key on this screen already answers to.
func TestTheBarIsRefusedWhileTheSectionIsNotShowingItsRows(t *testing.T) {
	m := list.New(theme.RosePineMoon)
	m.SetSize(110, 20)
	m.SetSections([]store.Section{
		{Section: config.Section{Title: "Broken"}, Status: store.StatusFailed, Err: errors.New("boom")},
	})

	m = press(m, key('/'))
	if bar := searchBox(m.View()); bar != "" {
		t.Errorf("bar = %q, want the key refused over a section showing an error", bar)
	}
	if m.Capturing() {
		t.Error("the keyboard was taken by a bar nothing drew")
	}
}

// A poll landing under an open bar must not drop the filter, and the count it
// carries moves with the section even where the rows it shows do not.
func TestASnapshotUnderTheBarKeepsTheFilterAndMovesItsCount(t *testing.T) {
	m := typed(press(newList(110, 20, mixed()), key('/')), "other")
	if bar := searchBox(m.View()); !strings.Contains(bar, "1 of 4") {
		t.Fatalf("setup: bar = %q", bar)
	}

	grown := append(mixed(), pr("Nothing like the query"))
	grown[4].ID = "PR_grown"
	m.SetSections(ready([]string{"My PRs"}, grown))

	bar := searchBox(m.View())
	if !strings.Contains(bar, "other") {
		t.Errorf("bar = %q, want the query held through the snapshot", bar)
	}
	if !strings.Contains(bar, "1 of 5") {
		t.Errorf("bar = %q, want the section's own count moved to 5", bar)
	}
	if out := stripANSI(m.View()); strings.Contains(out, "Nothing like the query") {
		t.Errorf("a row arriving under the bar went round the filter\n%s", out)
	}
}

// The border says where the keys are. This screen is one pane and never takes
// focus, so the box is the only thing on it that can say so.
func TestTheBoxBorderFollowsTheKeyboard(t *testing.T) {
	boxTop := func(frame string) string {
		plain := strings.Split(stripANSI(frame), "\n")
		for i, line := range plain {
			if strings.Contains(line, "╭─") && strings.Contains(line, "─╮ ") {
				return strings.Split(frame, "\n")[i]
			}
		}
		t.Fatalf("no box in the frame\n%s", stripANSI(frame))
		return ""
	}

	m := press(newList(90, 20, mixed()), key('/'))
	if top := boxTop(m.View()); !strings.Contains(top, fgSeq(theme.RosePineMoon.Accent)) {
		t.Error("the box holds the keyboard and its border does not say so")
	}

	m = press(typed(m, "auth"), enter)
	top := boxTop(m.View())
	if strings.Contains(top, fgSeq(theme.RosePineMoon.Accent)) {
		t.Error("the box gave the keyboard back and its border still claims it")
	}
	if !strings.Contains(top, fgSeq(theme.RosePineMoon.BorderSubtleOrBorder())) {
		t.Error("the settled box does not carry the border colour")
	}
}

// A filter is the reader's own state rather than the section's, so esc lets go
// of it wherever the section stands. Below the rows guard this key was inert
// exactly where the screen is hardest to read: the box still names a filter,
// the rows behind it are gone, and nothing explains either.
func TestEscClearsASettledFilterOverASectionShowingAnError(t *testing.T) {
	m := press(typed(press(newList(110, 20, mixed()), key('/')), "other"), enter)

	held := ready([]string{"My PRs"}, mixed())
	held[0].Status = store.StatusFailed
	held[0].Err = errors.New("boom")
	m.SetSections(held)

	if box := searchBox(m.View()); !strings.Contains(box, "other") {
		t.Fatalf("setup: box = %q, want the settled filter still named", box)
	}

	m = press(m, esc)
	if box := searchBox(m.View()); box != "" {
		t.Errorf("box = %q, want esc to clear it over a section showing an error", box)
	}
	if out := stripANSI(m.View()); !strings.Contains(out, "boom") {
		t.Errorf("clearing the filter took the error report with it\n%s", out)
	}
}

// The pane clips overflow silently and mid-cell, so the box has to fit before
// the pane sees it. Only a width where the query and the count cannot both fit
// exercises the clip at all.
func TestEveryLineFillsThePaneWidthWithTheBoxOpen(t *testing.T) {
	for _, width := range []int{200, 140, 100, 90, 70, 56, 50, 40, 30, 20} {
		t.Run(fmt.Sprintf("%d", width), func(t *testing.T) {
			m := typed(press(newList(width, 14, mixed()), key('/')), "auth retry")

			for i, line := range strings.Split(m.View(), "\n") {
				if w := lipgloss.Width(line); w != width {
					t.Errorf("line %d is %d cells wide, want %d\n%s", i, w, width, stripANSI(line))
				}
			}
		})
	}
}

// The box is three of the pane's own lines and gives them back. Nothing about
// it reaches past the frame the shell handed down.
func TestTheBoxCostsItsThreeLinesAndGivesThemBack(t *testing.T) {
	const width, height = 90, 20

	m := newList(width, height, numbered(20))
	body := func() int {
		return strings.Count(stripANSI(m.View()), "Change ")
	}
	// A pull request is two lines with a third under it, so the box's three
	// take one row off the pane and leave a line the padding absorbs.
	before := body()

	m = press(m, key('/'))
	if got := boxBorders(m.View()); got != 2 {
		t.Errorf("the box drew %d of its own borders, want a top and a bottom", got)
	}
	if got := body(); got != before-1 {
		t.Errorf("the box took %d rows off the pane, want one", before-got)
	}

	open := m.View()
	m = press(m, esc)
	for _, frame := range []string{open, m.View()} {
		lines := strings.Split(stripANSI(frame), "\n")
		if len(lines) != height {
			t.Errorf("frame is %d lines, want %d", len(lines), height)
		}
		for i, line := range lines {
			if got := len([]rune(line)); got != width {
				t.Errorf("line %d is %d cells, want %d: %q", i, got, width, line)
			}
		}
	}
	if got := body(); got != before {
		t.Errorf("esc gave back %d rows, want the pane as it was", got)
	}
	if got := boxBorders(m.View()); got != 0 {
		t.Errorf("the box left %d border lines behind after esc", got)
	}
}

// The row keeps its right side for the count, so a query long enough to clip
// stops short of the box's border. A caret measured against that border walks
// past the ellipsis and sits on the count, which is a cursor pointing at text
// nobody is editing.
//
// The window is a few characters wide: below it nothing clips, above it the
// caret runs out of box entirely. That is why this is a test and not a check
// somebody performs, and why holding a key down finds nothing.
func TestTheCursorNeverSitsPastTheQueryItIsIn(t *testing.T) {
	for _, width := range []int{56, 80, 120} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			for n := 1; n < width; n++ {
				m := typed(press(newList(width, 20, numbered(20)), key('/')), strings.Repeat("a", n))

				c := m.Cursor()
				if c == nil {
					continue
				}
				row := searchBox(m.View())
				if row == "" {
					t.Fatalf("a cursor with no box to sit in, at %d characters", n)
				}

				// The end of the query as drawn, not the end of the row: the
				// count sits further right with a gap before it, and measuring
				// to the row's last cell calls a cursor sitting on the count
				// in bounds.
				at := strings.Index(row, searchPromptText)
				if at < 0 {
					t.Fatalf("no prompt on the box row at %d characters:\n%s", n, row)
				}
				end := at
				for end < len(row) && row[end] != ' ' || end == at+1 {
					end++
				}
				if last := lipgloss.Width(row[:end]); c.X > last {
					t.Fatalf("at %d characters the cursor is at %d, past the query's last cell %d\n%s",
						n, c.X, last, row)
				}
			}
		})
	}
}
