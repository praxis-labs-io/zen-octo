package prview

import (
	"image/color"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/store"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
	"github.com/zen-octo/zen-octo/internal/tui/keys"
)

// pickField is which rail row a picker was opened from, and so which write
// applying it starts. The zero value is no picker.
type pickField int

const (
	pickNone pickField = iota
	pickLabels
)

// picking is the picker over the screen, if any.
//
// want is a picker asked for before the repository answered. Opening one needs
// the choices, the screen cannot fetch them, and a modal with an empty list
// reads as a repository with no labels. So the ask is held here, the root is
// told to fetch, and the picker opens when the answer lands.
type picking struct {
	field pickField
	p     comp.Picker

	want pickField
}

func (p picking) open() bool { return p.field != pickNone }

// NeedRepoMetaMsg asks the root for the choices a picker offers. The screen
// reads the repository from the pull request it is showing; the root owns the
// fetch and the cache, the same way it does for the detail.
type NeedRepoMetaMsg struct{ Repo string }

// SetLabelsMsg asks the root to write a label set on this pull request. It
// carries the whole set rather than what changed: the picker applies a set, the
// mutation takes one, and a delta would have to be recomputed at both ends.
type SetLabelsMsg struct {
	ID     string
	Labels []gh.Label
}

// Capturing is whether something on this screen owns the keyboard. The root
// stands aside when it does, because a picker's filter takes q as a letter the
// same way a comment box does.
func (m Model) Capturing() bool { return m.Composing() || m.picking.open() }

// SetRepo hands the screen the choices its pickers draw from, and opens the one
// that was waiting on them.
func (m *Model) SetRepo(r store.Repo) {
	m.repo = r
	if m.picking.want == pickNone || !r.Loaded {
		return
	}
	want := m.picking.want
	m.picking.want = pickNone
	m.startPicker(want)
}

// openRailPicker opens whatever the rail row under the focus holds. It does
// nothing on a row with no picker behind it, and nothing while the focus is
// scrolled out of the window: acting on a row the reader cannot see is the rule
// every key that reads a ring holds to.
func (m Model) openRailPicker() (Model, tea.Cmd) {
	if !m.detail.Loaded || !m.railRing.live(bodyTop(&m.railView), m.railView.Height()) {
		return m, nil
	}

	var want pickField
	switch m.railRing.on.kind {
	case focusLabel, focusAddLabel:
		want = pickLabels
	default:
		return m, nil
	}

	// The choices are the repository's, not this pull request's, so they
	// outlive the screen and are asked for once.
	if !m.repo.Loaded {
		m.picking.want = want
		repo := m.pr.Repository
		return m, func() tea.Msg { return NeedRepoMetaMsg{Repo: repo} }
	}

	m.startPicker(want)
	return m, nil
}

// startPicker builds the picker for a field over the choices already held.
func (m *Model) startPicker(field pickField) {
	switch field {
	case pickLabels:
		m.picking = picking{field: field, p: comp.NewPicker(
			"Labels",
			labelItems(m.repo.Meta.Labels, m.theme.Secondary),
			labelIDs(m.railDetail().Labels),
			true,
		)}
	}
}

// labelItems is the repository's labels as choices, in the accent the rail
// gives them, so the picker reads the same as the rows it writes.
func labelItems(labels []gh.Label, accent color.Color) []comp.PickerItem {
	out := make([]comp.PickerItem, 0, len(labels))
	for _, l := range labels {
		out = append(out, comp.PickerItem{ID: l.ID, Name: l.Name, Color: accent})
	}
	return out
}

func labelIDs(labels []gh.Label) []string {
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		out = append(out, l.ID)
	}
	return out
}

// pickerKey answers every key while a picker is up. Nothing below it gets a
// look: a modal that let keys through would scroll the page behind it.
//
// The order is the whole of it. The keys that can never be text go first, then
// the filter claims every printable one, and movement takes what is left. That
// is what lets j walk a state menu and type a j into a label filter without
// either of them being a special case.
func (m Model) pickerKey(keyMsg tea.KeyPressMsg) (Model, tea.Cmd) {
	k := keys.Detail

	switch {
	case key.Matches(keyMsg, k.Back):
		m.picking = picking{}
		return m, nil

	case key.Matches(keyMsg, k.Activate):
		return m.applyPicker()

	// Only where it means something. On a single-select picker space is a
	// character, and swallowing it here would leave the filter unable to take
	// one.
	case key.Matches(keyMsg, k.Toggle) && m.picking.p.Multi():
		m.picking.p.Toggle()
		return m, nil
	}

	if m.picking.p.Insert(keyMsg) {
		return m, nil
	}

	switch {
	case key.Matches(keyMsg, k.Up):
		m.picking.p.Move(-1)
	case key.Matches(keyMsg, k.Down):
		m.picking.p.Move(1)
	}
	return m, nil
}

// applyPicker closes the picker and asks the root to write what it chose.
//
// A set equal to what is already on the pull request writes nothing. Applying
// an unchanged picker is how a reader backs out of one they opened by mistake,
// and it should cost neither a request nor a toast.
func (m Model) applyPicker() (Model, tea.Cmd) {
	field, chosen := m.picking.field, m.picking.p.Chosen()
	m.picking = picking{}

	if field != pickLabels {
		return m, nil
	}

	labels := labelsByID(m.repo.Meta.Labels, chosen)
	if sameLabels(labels, m.railDetail().Labels) {
		return m, nil
	}

	id := m.pr.ID
	return m, func() tea.Msg { return SetLabelsMsg{ID: id, Labels: labels} }
}

// labelsByID is the chosen ids back as whole labels, in the repository's own
// order. The rail renders a name and a color, so the ids alone would leave the
// optimistic row with nothing to draw.
func labelsByID(all []gh.Label, ids []string) []gh.Label {
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}

	out := make([]gh.Label, 0, len(ids))
	for _, l := range all {
		if want[l.ID] {
			out = append(out, l)
		}
	}
	return out
}

// sameLabels compares by id and order. Both sides come out of the repository's
// own list, so the order is the same whenever the set is.
func sameLabels(a, b []gh.Label) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID {
			return false
		}
	}
	return true
}

// pickerOverlay composites the picker over a rendered frame. It is drawn here
// rather than at the root because the root does not know a picker is open, and
// the status bar stays uncovered: a toast is worth reading while a modal is up.
func (m Model) pickerOverlay(frame string) string {
	if !m.picking.open() {
		return frame
	}
	return comp.Over(frame, m.picking.p.Render(m.theme, m.width), m.width, m.height)
}
