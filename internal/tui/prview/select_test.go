package prview_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/praxis-labs-io/zen-octo/internal/tui/keys"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

// The file column paints its own row the same way, and the two panes share a line.
func mainPane(line string) string {
	at := 0
	for range 3 {
		next := strings.Index(line[at:], "│")
		if next < 0 {
			return ""
		}
		at += next + len("│")
	}
	return line[at:]
}

// The bar comes and goes as the cursor moves over a row that stays selected, so it is not part of one.
func plain(body string) string {
	return strings.TrimSpace(strings.ReplaceAll(stripANSI(body), "▌", " "))
}

// The fill is what measures a selection: the bar marks the cursor's row alone, whatever it covers.
func filledRows(frame string) []string {
	fill := bgSeq(testTheme.SelectedBackground)

	var out []string
	for _, line := range strings.Split(frame, "\n") {
		body := mainPane(line)
		if body == "" || strings.Contains(stripANSI(body), "@@") {
			continue
		}
		if strings.Contains(body, fill) {
			out = append(out, plain(body))
		}
	}
	return out
}

func barredCode(frame string) string {
	for _, line := range strings.Split(frame, "\n") {
		if body := mainPane(line); strings.Contains(body, "▌") {
			return plain(body)
		}
	}
	return ""
}

func barredRows(frame string) int {
	n := 0
	for _, line := range strings.Split(frame, "\n") {
		bare := stripANSI(line)
		if strings.HasPrefix(bare, "│") && strings.Contains(bare, "▌") {
			n++
		}
	}
	return n
}

func onCode(then ...string) prview.Model {
	return press(onFiles(200, 50), append([]string{"}", "j"}, then...)...)
}

func offers(m prview.Model, want string) bool {
	for _, b := range m.ShortHelp() {
		if b.Help().Desc == want {
			return true
		}
	}
	return false
}

func TestVSelectsTheRowUnderTheCursor(t *testing.T) {
	m := onCode()
	if got := filledRows(m.View()); len(got) != 1 {
		t.Fatalf("the cursor alone filled %d rows, want 1: %q", len(got), got)
	}

	rows := filledRows(press(m, "v").View())
	if len(rows) != 1 {
		t.Fatalf("v filled %d rows, want the one under the cursor: %q", len(rows), rows)
	}
	if want := barredCode(m.View()); rows[0] != want {
		t.Errorf("v filled %q, want %q", rows[0], want)
	}
}

func TestMovingGrowsTheSelectionFromItsAnchor(t *testing.T) {
	for _, tt := range []struct {
		name  string
		start []string
		grow  []string
	}{
		{"down from the first row", nil, []string{"j", "j"}},
		{"up from the third", []string{"j", "j"}, []string{"k", "k"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := press(onCode(tt.start...), "v")
			anchor := filledRows(m.View())[0]

			m = press(m, tt.grow...)
			rows := filledRows(m.View())
			if len(rows) != 3 {
				t.Fatalf("two steps covered %d rows, want 3: %q", len(rows), rows)
			}
			if !slices.Contains(rows, anchor) {
				t.Errorf("the anchor %q fell out of %q", anchor, rows)
			}
			if got := barredCode(m.View()); !slices.Contains(rows, got) {
				t.Errorf("the bar sits on %q, outside the selection %q", got, rows)
			}
		})
	}
}

func TestOnlyTheCursorsRowInASelectionCarriesTheBar(t *testing.T) {
	m := press(onCode(), "v", "j", "j")
	if rows := filledRows(m.View()); len(rows) != 3 {
		t.Fatalf("setup: %d rows are filled, want 3", len(rows))
	}
	if got := barredRows(m.View()); got != 1 {
		t.Errorf("%d rows carry a bar, want the cursor's alone", got)
	}
}

func TestASecondVClearsTheSelection(t *testing.T) {
	m := press(onCode(), "v", "j", "j")
	if rows := filledRows(m.View()); len(rows) != 3 {
		t.Fatalf("setup: %d rows are filled, want 3", len(rows))
	}

	if rows := filledRows(press(m, "v").View()); len(rows) != 1 {
		t.Errorf("a second v left %d rows filled, want the cursor's alone: %q", len(rows), rows)
	}
}

func TestEscClearsTheSelectionRatherThanLeavingTheScreen(t *testing.T) {
	m := press(onCode(), "v", "j")
	if rows := filledRows(m.View()); len(rows) != 2 {
		t.Fatalf("setup: %d rows are filled, want 2", len(rows))
	}

	after, cmd := key(m, "esc")
	if cmd != nil {
		if _, ok := cmd().(prview.BackMsg); ok {
			t.Fatal("esc left the screen instead of clearing the selection")
		}
	}
	if rows := filledRows(after.View()); len(rows) != 1 {
		t.Errorf("esc left %d rows filled, want the cursor's alone: %q", len(rows), rows)
	}
}

func TestEscWithNothingSelectedStillLeaves(t *testing.T) {
	_, cmd := key(onCode(), "esc")
	if cmd == nil {
		t.Fatal("esc on a bare cursor sent nothing")
	}
	if _, ok := cmd().(prview.BackMsg); !ok {
		t.Errorf("esc sent %T, want a BackMsg", cmd())
	}
}

func TestVOnAHunkHeadingSelectsNothing(t *testing.T) {
	m := press(onFiles(200, 50), "}")
	if litHunk(m.View()) == "" {
		t.Fatal("setup: the brace lit no heading")
	}

	if rows := filledRows(press(m, "v").View()); len(rows) != 0 {
		t.Errorf("v on the heading filled %q", rows)
	}
}

func TestTheAnchorIsLeftBehindWithTheHunkThatHeldIt(t *testing.T) {
	for _, tt := range []struct {
		name string
		away []string
		back []string
	}{
		{"stepping off its last row", []string{"j", "j", "j", "j"}, []string{"k"}},
		{"a brace onto the next block", []string{"}"}, []string{"{", "j"}},
		{"splitting the pane", []string{"|"}, []string{"|", "j"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := press(onCode(), "v")
			if len(filledRows(m.View())) != 1 {
				t.Fatal("setup: v selected nothing")
			}

			m = press(m, tt.away...)
			m = press(m, tt.back...)
			if rows := filledRows(m.View()); len(rows) > 1 {
				t.Errorf("walking back revived a selection over %q", rows)
			}
		})
	}
}

func TestTheOtherColumnLeavesTheAnchorBehind(t *testing.T) {
	m := press(onFiles(140, 30), "|", "}", "j", "v", "j")
	if rows := filledRows(m.View()); len(rows) != 2 {
		t.Fatalf("setup: %d rows are filled side by side, want 2", len(rows))
	}

	m = press(m, "h")
	if rows := filledRows(m.View()); len(rows) != 1 {
		t.Errorf("h kept %d rows filled, want the cursor's alone: %q", len(rows), rows)
	}
	if rows := filledRows(press(m, "l").View()); len(rows) != 1 {
		t.Errorf("stepping back revived a selection over %q", rows)
	}
}

func TestNarrowingPastTheSplitLeavesTheAnchorBehind(t *testing.T) {
	m := press(onFiles(140, 30), "|", "}", "j", "v", "j")
	if rows := filledRows(m.View()); len(rows) != 2 {
		t.Fatalf("setup: %d rows are filled side by side, want 2", len(rows))
	}

	m.SetSize(70, 30)
	if rows := filledRows(m.View()); len(rows) != 1 {
		t.Errorf("the unsplit pane read the anchor as %d rows, want the cursor's alone: %q", len(rows), rows)
	}

	m.SetSize(140, 30)
	if rows := filledRows(m.View()); len(rows) != 1 {
		t.Errorf("widening back revived a selection over %q", rows)
	}
}

func TestTheAnchorRidesWithTheCursorAcrossAFile(t *testing.T) {
	m := press(onCode(), "v", "j")
	was, rows := barredCode(m.View()), filledRows(m.View())
	if len(rows) != 2 {
		t.Fatalf("setup: %d rows are filled, want 2", len(rows))
	}

	away := press(m, "tab")
	if got := filledRows(away.View()); len(got) != 0 {
		t.Errorf("the next file drew %q, which belongs to the one before it", got)
	}

	back := press(away, "shift+tab")
	if got := barredCode(back.View()); got != was {
		t.Fatalf("the cursor came back on %q, want %q", got, was)
	}
	if got := filledRows(back.View()); len(got) != 2 {
		t.Errorf("the cursor came back without its anchor: %d rows filled, want 2", len(got))
	}
}

// Raw offsets throughout: stripping the escapes takes the fill with them.
func TestASelectionFillsOneColumnOfASplitDiff(t *testing.T) {
	m := press(onFiles(140, 30), "|", "}", "j", "v", "j")
	fill := bgSeq(testTheme.SelectedBackground)

	found := 0
	for _, line := range strings.Split(m.View(), "\n") {
		body := mainPane(line)
		at := strings.Index(body, fill)
		if at < 0 || strings.Contains(stripANSI(body), "@@") {
			continue
		}

		rule := strings.Index(body, "│")
		if rule < 0 {
			t.Fatalf("a filled row carries no column rule: %q", stripANSI(body))
		}
		if at < rule {
			t.Errorf("the fill landed before the rule, in the column the cursor left: %q", stripANSI(body))
		}
		found++
	}
	if found != 2 {
		t.Errorf("measured %d filled rows, want 2", found)
	}
}

func TestALeftPaneTakesTheSelectionWithTheDiffOffTheKeyboard(t *testing.T) {
	m := press(onCode(), "v", "j")
	if rows := filledRows(m.View()); len(rows) != 2 {
		t.Fatalf("setup: %d rows are filled, want 2", len(rows))
	}

	away := press(m, "h")
	if rows := filledRows(away.View()); len(rows) != 0 {
		t.Fatalf("setup: the file column still draws %q", rows)
	}
	if offers(away, "clear selection") {
		t.Error("the status bar offers a way out of a selection that is not on the frame")
	}

	_, cmd := key(away, "esc")
	if cmd == nil {
		t.Fatal("esc was taken by a selection nobody can see")
	}
	if _, ok := cmd().(prview.BackMsg); !ok {
		t.Errorf("esc sent %T, want a BackMsg", cmd())
	}
}

func TestTheStatusBarOffersTheSelectionAndThenTheWayOut(t *testing.T) {
	m := onCode()
	if !offers(m, "select lines") {
		t.Error("the status bar does not offer v on a row of code")
	}
	if offers(m, "clear selection") {
		t.Error("the status bar offers a way out of a selection nobody started")
	}

	if m = press(m, "v"); !offers(m, "clear selection") {
		t.Error("the status bar does not say how to clear a live selection")
	}
}

func TestTheSelectionIsOfferedOnlyWhereThereIsCodeToSelect(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 50)
	m.SetFiles(loadedFiles(sampleFiles(), 0))

	if offers(press(m, "}"), "select lines") {
		t.Error("the conversation offers v, which acts on nothing there")
	}
	if offers(press(m, "]", "]", "]", "}"), "select lines") {
		t.Error("a hunk heading offers v, which selects no row")
	}
}

func TestAFoldedHunkOffersTheSelectionNoRows(t *testing.T) {
	m := press(onCode(), "v", "space")
	if rows := filledRows(m.View()); len(rows) != 0 {
		t.Errorf("folding the hunk left %q filled", rows)
	}
}

func TestSelectIsBoundToVAndTheJumpToEnter(t *testing.T) {
	if got := keys.Detail.Select.Help().Key; got != "v" {
		t.Errorf("select is offered as %q, want v", got)
	}
	if got := keys.Detail.Activate; !strings.Contains(got.Help().Desc, "show in the diff") {
		t.Errorf("enter is offered as %q, want the diff jump named in it", got.Help().Desc)
	}
}
