package comp_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/tui/comp"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

func items(names ...string) []comp.PickerItem {
	out := make([]comp.PickerItem, len(names))
	for i, n := range names {
		out[i] = comp.PickerItem{ID: "id-" + n, Name: n}
	}
	return out
}

// many builds a list long enough to earn a filter row and outrun the window.
func many(n int) []comp.PickerItem {
	out := make([]comp.PickerItem, n)
	for i := range out {
		name := "label-" + strconv.Itoa(i)
		out[i] = comp.PickerItem{ID: "id-" + name, Name: name}
	}
	return out
}

func typeInto(t *testing.T, p *comp.Picker, text string) {
	t.Helper()
	for _, r := range text {
		if !p.Insert(tea.KeyPressMsg{Code: r, Text: string(r)}) {
			t.Fatalf("Insert(%q) was refused", string(r))
		}
	}
}

func render(p comp.Picker) string { return p.Render(theme.RosePineMoon, 80) }

func TestAMultiPickerAppliesTheWholeCheckedSet(t *testing.T) {
	p := comp.NewPicker("Labels", items("bug", "docs", "urgent"), []string{"id-bug"}, true)

	p.Move(2) // urgent
	p.Toggle()

	if got, want := p.Chosen(), []string{"id-bug", "id-urgent"}; !slices.Equal(got, want) {
		t.Errorf("Chosen = %q, want %q", got, want)
	}
}

// The set comes back in the order the items were given, not the order they were
// checked. A caller diffing it against what it holds would otherwise see a
// change every time the reader worked bottom-up.
func TestTheChosenSetKeepsTheOrderItemsWereGivenIn(t *testing.T) {
	p := comp.NewPicker("Labels", items("bug", "docs", "urgent"), nil, true)

	p.Move(2)
	p.Toggle() // urgent first
	p.Move(-2)
	p.Toggle() // then bug

	if got, want := p.Chosen(), []string{"id-bug", "id-urgent"}; !slices.Equal(got, want) {
		t.Errorf("Chosen = %q, want %q", got, want)
	}
}

func TestTogglingOffLeavesAnEmptySet(t *testing.T) {
	p := comp.NewPicker("Labels", items("bug"), []string{"id-bug"}, true)

	p.Toggle()

	if got := p.Chosen(); len(got) != 0 {
		t.Errorf("Chosen = %q, want none", got)
	}
}

func TestASinglePickerOpensOnWhatIsAlreadyChosen(t *testing.T) {
	p := comp.NewPicker("Base", items("main", "develop", "release"), []string{"id-develop"}, false)

	if got, want := p.Chosen(), []string{"id-develop"}; !slices.Equal(got, want) {
		t.Errorf("Chosen = %q, want %q with no movement", got, want)
	}
}

func TestASinglePickerIgnoresToggle(t *testing.T) {
	p := comp.NewPicker("Base", items("main", "develop"), []string{"id-main"}, false)

	p.Toggle()

	if got, want := p.Chosen(), []string{"id-main"}; !slices.Equal(got, want) {
		t.Errorf("Chosen = %q, want %q", got, want)
	}
}

func TestTheCursorStopsAtTheEnds(t *testing.T) {
	p := comp.NewPicker("Labels", items("a", "b", "c"), nil, false)

	p.Move(-5)
	if got, want := p.Chosen(), []string{"id-a"}; !slices.Equal(got, want) {
		t.Errorf("Chosen = %q at the top, want %q", got, want)
	}

	p.Move(99)
	if got, want := p.Chosen(), []string{"id-c"}; !slices.Equal(got, want) {
		t.Errorf("Chosen = %q at the bottom, want %q", got, want)
	}
}

func TestAShortListGetsNoFilterRow(t *testing.T) {
	p := comp.NewPicker("State", items("ready", "draft", "close"), nil, false)

	if p.Filtering() {
		t.Error("a three-item picker shows a filter row")
	}
	if p.Insert(tea.KeyPressMsg{Code: 'r', Text: "r"}) {
		t.Error("a picker with no filter row took a letter")
	}
	if strings.Contains(render(p), "Type to filter") {
		t.Error("the filter placeholder is rendered on a picker that does not filter")
	}
}

func TestALongListFiltersAndReanchors(t *testing.T) {
	p := comp.NewPicker("Labels", many(20), nil, false)

	p.Move(5)
	typeInto(t, &p, "label-19")

	if got, want := p.Chosen(), []string{"id-label-19"}; !slices.Equal(got, want) {
		t.Errorf("Chosen = %q, want the one match with the cursor back at the top", got)
	}

	frame := render(p)
	if !strings.Contains(frame, "label-19") {
		t.Error("the match is not on screen")
	}
	if strings.Contains(frame, "label-2 ") {
		t.Error("a filtered-out row is still on screen")
	}
}

func TestAFilterMatchingNothingChoosesNothing(t *testing.T) {
	p := comp.NewPicker("Labels", many(20), nil, false)

	typeInto(t, &p, "zzz")

	if got := p.Chosen(); got != nil {
		t.Errorf("Chosen = %q, want nothing", got)
	}
	if !strings.Contains(render(p), "No match") {
		t.Error("an empty result set does not say so")
	}
}

func TestBackspaceAndClearWidenTheFilterAgain(t *testing.T) {
	p := comp.NewPicker("Labels", many(20), nil, false)
	typeInto(t, &p, "label-1")

	if !p.Insert(tea.KeyPressMsg{Code: tea.KeyBackspace}) {
		t.Fatal("backspace was refused")
	}
	if got, want := p.Chosen(), []string{"id-label-0"}; !slices.Equal(got, want) {
		t.Errorf("Chosen = %q after backspace, want %q", got, want)
	}

	if !p.Insert(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}) {
		t.Fatal("ctrl+u was refused")
	}
	if !strings.Contains(render(p), "Type to filter") {
		t.Error("clearing the filter does not bring the placeholder back")
	}
}

// Space is the toggle key on a multi picker. A filter that swallowed it would
// leave the reader unable to check anything.
func TestSpaceDoesNotTypeIntoAMultiPickersFilter(t *testing.T) {
	p := comp.NewPicker("Labels", many(20), nil, true)

	if p.Insert(tea.KeyPressMsg{Code: ' ', Text: " "}) {
		t.Error("space typed into a multi picker's filter")
	}
}

func TestASinglePickersFilterTakesSpace(t *testing.T) {
	p := comp.NewPicker("Base", many(20), nil, false)

	if !p.Insert(tea.KeyPressMsg{Code: ' ', Text: " "}) {
		t.Error("space was refused by a single picker's filter")
	}
}

// A modified key is a binding on the screen underneath, not text.
func TestAModifiedKeyIsNotFilterText(t *testing.T) {
	p := comp.NewPicker("Labels", many(20), nil, false)

	if p.Insert(tea.KeyPressMsg{Code: 'd', Text: "d", Mod: tea.ModCtrl}) {
		t.Error("ctrl+d typed into the filter")
	}
}

// One keypress into a filter is one character. A key reporting a whole name in
// its text is an arrow that arrived in the wrong field, and typing it would put
// "down" into the filter when the reader pressed an arrow.
func TestAKeyNameIsNotFilterText(t *testing.T) {
	p := comp.NewPicker("Labels", many(20), nil, false)

	for _, name := range []string{"down", "up", "pgdown"} {
		if p.Insert(tea.KeyPressMsg{Code: rune(name[0]), Text: name}) {
			t.Errorf("%q was typed into the filter", name)
		}
	}
}

// A real arrow key carries no text at all, and must still move the cursor
// rather than being swallowed by the filter.
func TestAnArrowMovesTheCursorWhileFiltering(t *testing.T) {
	p := comp.NewPicker("Labels", many(20), nil, false)

	if p.Insert(tea.KeyPressMsg{Code: tea.KeyDown}) {
		t.Fatal("the filter swallowed an arrow key")
	}
	p.Move(1)

	if got, want := p.Chosen(), []string{"id-label-1"}; !slices.Equal(got, want) {
		t.Errorf("Chosen = %q, want %q", got, want)
	}
}

func TestTheWindowScrollsWithTheCursorAndSaysWhatIsHidden(t *testing.T) {
	p := comp.NewPicker("Labels", many(20), nil, false)

	if got := render(p); !strings.Contains(got, "10 more") {
		t.Errorf("the hint does not count the hidden rows:\n%s", got)
	}

	p.Move(15)

	frame := render(p)
	if !strings.Contains(frame, "label-15") {
		t.Errorf("the cursor row is not on screen:\n%s", frame)
	}
	if strings.Contains(frame, "label-0 ") {
		t.Error("the window did not scroll off the first row")
	}
}

// The modal must never grow the frame it is composited into.
func TestThePickerFitsTheFrameItIsGiven(t *testing.T) {
	long := []comp.PickerItem{{ID: "id", Name: strings.Repeat("verylongname", 12)}}
	p := comp.NewPicker("Labels", long, nil, true)

	for _, width := range []int{20, 40, 80} {
		got := p.Render(theme.RosePineMoon, width)
		if w := lipgloss.Width(got); w > width {
			t.Errorf("at frame width %d the picker rendered %d columns wide", width, w)
		}
	}
}

func TestTheHintNamesTheKeysThatWork(t *testing.T) {
	multi := render(comp.NewPicker("Labels", items("bug"), nil, true))
	if !strings.Contains(multi, "space toggle") {
		t.Errorf("a multi picker does not name the toggle key:\n%s", multi)
	}

	single := render(comp.NewPicker("Base", items("main"), nil, false))
	if strings.Contains(single, "space toggle") {
		t.Errorf("a single picker names a toggle key that does nothing:\n%s", single)
	}
	if !strings.Contains(single, "enter pick") {
		t.Errorf("a single picker does not name the pick key:\n%s", single)
	}
}

func TestACheckedRowIsMarked(t *testing.T) {
	p := comp.NewPicker("Labels", items("bug", "docs"), []string{"id-bug"}, true)

	frame := render(p)
	lines := strings.Split(frame, "\n")

	var bug, docs string
	for _, l := range lines {
		if strings.Contains(l, "bug") {
			bug = l
		}
		if strings.Contains(l, "docs") {
			docs = l
		}
	}

	if !strings.Contains(bug, "✓") {
		t.Errorf("the checked row carries no mark:\n%s", bug)
	}
	if strings.Contains(docs, "✓") {
		t.Errorf("an unchecked row carries a mark:\n%s", docs)
	}
}

// The hint grows a counter once the list outruns the window, and the modal is
// sized for the longest it can render. Sizing to the short form clips the keys
// off exactly the long lists where the hint is worth having.
func TestTheHintIsNotClippedOnAListWithACounter(t *testing.T) {
	frame := render(comp.NewPicker("Labels", many(20), nil, true))

	if !strings.Contains(frame, "10 more") {
		t.Fatalf("the hint does not count the hidden rows:\n%s", frame)
	}
	if !strings.Contains(frame, "esc cancel") {
		t.Errorf("the hint is clipped before it names the cancel key:\n%s", frame)
	}
}
