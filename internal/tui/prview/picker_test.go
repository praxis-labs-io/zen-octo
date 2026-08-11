package prview_test

import (
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/store"
	"github.com/zen-octo/zen-octo/internal/tui/prview"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// repoLabels is the repository's whole set. The first is the one the fixture
// pull request already carries, so a picker over these opens with one checked.
func repoLabels() []gh.Label {
	return []gh.Label{
		{ID: "LA_1", Name: "bug"},
		{ID: "LA_2", Name: "enhancement"},
		{ID: "LA_3", Name: "documentation"},
	}
}

func loadedRepo() store.Repo {
	return store.Repo{
		Meta:   gh.RepoMeta{Labels: repoLabels()},
		Status: store.StatusReady,
		Loaded: true,
	}
}

// onRailRow tabs the rail until its cursor is on the row named, so a test names
// what it is acting on rather than counting tab presses to it.
func onRailRow(t *testing.T, m prview.Model, want string) prview.Model {
	t.Helper()

	m = press(m, "2") // the rail is the second pane on the conversation tab
	for range 30 {
		m = press(m, "tab")
		if markedRailRow(t, m.View()) == want {
			return m
		}
	}
	t.Fatalf("tab never reached the rail row %q", want)
	return m
}

// openPicker walks to a rail row, presses enter, and answers the metadata the
// screen asks for. It returns the screen with the picker up.
func openPicker(t *testing.T, row string) prview.Model {
	t.Helper()

	m := onRailRow(t, detailed(held(sampleDetail()), 200, 60), row)

	_, cmd := key(m, "enter")
	if _, ok := runCmd(cmd).(prview.NeedRepoMetaMsg); !ok {
		t.Fatalf("enter on %q sent %T, want a NeedRepoMetaMsg", row, runCmd(cmd))
	}

	m, _ = key(m, "enter")
	m.SetRepo(loadedRepo())
	return m
}

// pickerFrame is the rendered screen with the modal over it.
func pickerFrame(m prview.Model) string { return stripANSI(m.View()) }

func TestEnterOnALabelRowAsksForTheRepositorysChoices(t *testing.T) {
	m := onRailRow(t, detailed(held(sampleDetail()), 200, 60), "bug")

	got := asked(t, m, "enter")
	want := prview.NeedRepoMetaMsg{Repo: "zen-octo/zen-octo"}
	if got != want {
		t.Fatalf("enter sent %#v, want %#v", got, want)
	}
}

// The choices are the repository's, so a screen already holding them opens the
// picker without asking again.
func TestEnterOpensStraightAwayOnceTheChoicesAreHeld(t *testing.T) {
	m := onRailRow(t, detailed(held(sampleDetail()), 200, 60), "bug")
	m.SetRepo(loadedRepo())

	m, cmd := key(m, "enter")
	if got := runCmd(cmd); got != nil {
		t.Errorf("enter sent %T, want nothing", got)
	}
	if !strings.Contains(pickerFrame(m), "Labels") {
		t.Error("the picker did not open")
	}
}

func TestThePickerOpensOnTheLabelsAlreadyOnThePullRequest(t *testing.T) {
	frame := pickerFrame(openPicker(t, "bug"))

	for _, want := range []string{"bug", "enhancement", "documentation"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the picker does not offer %q:\n%s", want, frame)
		}
	}

	// One checked, two not. The mark is the whole of what says which.
	if !strings.Contains(frame, "✓ bug") {
		t.Errorf("the label already on the pull request is not checked:\n%s", frame)
	}
	for _, off := range []string{"✓ enhancement", "✓ documentation"} {
		if strings.Contains(frame, off) {
			t.Errorf("%q is checked and should not be:\n%s", off, frame)
		}
	}
}

// The add row and a label row open the same picker. Removing is unchecking, so
// there is no second mode for the add row to be.
func TestTheAddRowOpensTheSamePicker(t *testing.T) {
	frame := pickerFrame(openPicker(t, "+ Add label"))

	if !strings.Contains(frame, "Labels") {
		t.Fatalf("the add row did not open the picker:\n%s", frame)
	}
	if !strings.Contains(frame, "✓ bug") {
		t.Errorf("the picker opened without the label already on the pull request:\n%s", frame)
	}
}

func TestCheckingALabelAndApplyingAsksForTheWholeSet(t *testing.T) {
	m := openPicker(t, "bug")

	m = press(m, "down")
	m = press(m, " ") // check enhancement

	got, ok := asked(t, m, "enter").(prview.SetLabelsMsg)
	if !ok {
		t.Fatalf("enter sent %T, want a SetLabelsMsg", asked(t, m, "enter"))
	}

	if got.ID != "PR_412" {
		t.Errorf("ID = %q, want PR_412", got.ID)
	}
	names := make([]string, 0, len(got.Labels))
	for _, l := range got.Labels {
		names = append(names, l.Name)
	}
	if want := []string{"bug", "enhancement"}; !slices.Equal(names, want) {
		t.Errorf("labels = %q, want %q", names, want)
	}
}

// Unchecking the last label is a write, not a no-op. The rail has to be able to
// go empty.
func TestUncheckingEveryLabelAsksForAnEmptySet(t *testing.T) {
	m := openPicker(t, "bug")

	m = press(m, " ") // uncheck bug

	got, ok := asked(t, m, "enter").(prview.SetLabelsMsg)
	if !ok {
		t.Fatal("enter did not ask for a write")
	}
	if len(got.Labels) != 0 {
		t.Errorf("labels = %v, want none", got.Labels)
	}
}

// Applying a picker nobody changed is how a reader backs out of one they opened
// by mistake. It should cost neither a request nor a toast.
func TestApplyingAnUnchangedPickerWritesNothing(t *testing.T) {
	m := openPicker(t, "bug")

	if got := asked(t, m, "enter"); got != nil {
		t.Errorf("an unchanged picker sent %T, want nothing", got)
	}
}

func TestEscClosesThePickerAndLeavesTheScreenWhereItWas(t *testing.T) {
	m := openPicker(t, "bug")

	m, cmd := key(m, "esc")
	if got := runCmd(cmd); got != nil {
		t.Errorf("esc in a picker sent %T, want nothing", got)
	}

	frame := pickerFrame(m)
	if strings.Contains(frame, "space toggle") {
		t.Errorf("the picker is still up:\n%s", frame)
	}
	// Esc closed the modal and nothing else. Backing out of the screen as well
	// would take the reader somewhere they did not ask to go.
	if !strings.Contains(frame, "Details") {
		t.Error("the detail screen went away with the picker")
	}
}

// A picker owns the keyboard. A key that reached the page underneath would
// scroll it out from behind the modal.
func TestThePickerSwallowsKeysMeantForTheScreen(t *testing.T) {
	m := openPicker(t, "bug")

	for _, k := range []string{"]", "d", "s"} {
		if got := asked(t, m, k); got != nil {
			t.Errorf("%q reached the screen behind the picker and sent %T", k, got)
		}
	}
}

func TestCapturingIsTrueWhileAPickerIsUp(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, 60)
	if m.Capturing() {
		t.Fatal("Capturing is true with nothing open")
	}

	if !openPicker(t, "bug").Capturing() {
		t.Error("Capturing is false with a picker up")
	}
}

// The modal is composited over the frame, so it must not change its size.
func TestThePickerDoesNotGrowTheFrame(t *testing.T) {
	sizes := []struct{ width, height int }{
		{width: 200, height: 40},
		{width: 160, height: 24},
		{width: 130, height: 20},
	}

	for _, size := range sizes {
		m := onRailRow(t, detailed(held(sampleDetail()), size.width, size.height), "bug")
		m.SetRepo(loadedRepo())
		m, _ = key(m, "enter")

		lines := strings.Split(m.View(), "\n")
		if len(lines) != size.height {
			t.Errorf("%dx%d: frame is %d lines, want %d", size.width, size.height, len(lines), size.height)
		}
		for i, line := range lines {
			if w := lipgloss.Width(line); w != size.width {
				t.Errorf("%dx%d: line %d is %d cells, want %d", size.width, size.height, i, w, size.width)
			}
		}
	}
}

// The picker reads the same as the rows it writes.
func TestThePickerColorsLabelsFromTheTheme(t *testing.T) {
	if !strings.Contains(openPicker(t, "bug").View(), fgSeq(theme.RosePineMoon.Secondary)) {
		t.Error("the picker does not color its labels from the theme")
	}
}

// Enter on a rail row with no picker behind it does nothing. Author and Changes
// state a fact, and there is nothing to do to them.
func TestEnterOnARowWithNoPickerDoesNothing(t *testing.T) {
	m := onRailRow(t, detailed(held(sampleDetail()), 200, 60), "@nkr")

	if got := asked(t, m, "enter"); got != nil {
		t.Errorf("enter on a reviewer sent %T, want nothing until reviewers land", got)
	}
}

// Nothing opens before the detail answers. The rail draws from the list row
// then, and its label section is empty.
func TestNoPickerOpensBeforeTheDetailLands(t *testing.T) {
	m := press(screen(200, 60), "2")
	m.SetRepo(loadedRepo())

	m = press(m, "tab")
	if got := asked(t, m, "enter"); got != nil {
		t.Errorf("enter before the detail landed sent %T, want nothing", got)
	}
}

func TestTheFilterRowAppearsOnlyOnAListWorthFiltering(t *testing.T) {
	short := pickerFrame(openPicker(t, "bug"))
	if strings.Contains(short, "Type to filter") {
		t.Errorf("a three-label picker shows a filter row:\n%s", short)
	}

	m := onRailRow(t, detailed(held(sampleDetail()), 200, 60), "bug")
	m.SetRepo(store.Repo{Meta: gh.RepoMeta{Labels: manyLabels(12)}, Status: store.StatusReady, Loaded: true})
	m, _ = key(m, "enter")

	if long := pickerFrame(m); !strings.Contains(long, "Type to filter") {
		t.Errorf("a twelve-label picker shows no filter row:\n%s", long)
	}
}

func manyLabels(n int) []gh.Label {
	out := make([]gh.Label, n)
	for i := range out {
		name := "label-" + string(rune('a'+i))
		out[i] = gh.Label{ID: "LA_" + name, Name: name}
	}
	return out
}

// The rail paints its cursor only while it has the keys. With a picker up the
// keys are the picker's, and two lit rows say they go to both.
func TestTheRailKeepsItsCursorPaintedUnderThePicker(t *testing.T) {
	m := openPicker(t, "bug")

	if got := markedRailRow(t, m.View()); got != "bug" {
		t.Errorf("the rail cursor is on %q, want it still on the row the picker was opened from", got)
	}
}

func TestTheSelectedBackgroundIsTheThemes(t *testing.T) {
	if theme.RosePineMoon.SelectedBackground == nil {
		t.Fatal("the theme has no selected background for the picker to paint with")
	}
}
