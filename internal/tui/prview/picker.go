package prview

import (
	"image/color"
	"slices"

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
	pickState
	pickAssignees
	pickReviewers
)

// needsRepo is whether a field's choices belong to the repository rather than
// to the detail the screen already holds. Only those cost a round trip before
// the modal can open; the state menu is built from what is on screen.
func (f pickField) needsRepo() bool {
	return f == pickLabels || f == pickAssignees || f == pickReviewers
}

// picking is the picker over the screen, if any.
//
// want is a picker asked for before the repository answered. Opening one needs
// the choices, the screen cannot fetch them, and a modal with an empty list
// reads as a repository with no labels. So the ask is held here, the root is
// told to fetch, and the picker opens when the answer lands.
type picking struct {
	field pickField
	p     comp.Picker

	// labels and users are what the picker was built over, held so applying
	// reads the same list it offered. Rebuilding either at apply time would let
	// a refetch landing while the modal was up change the set under the reader,
	// and a choice that disappeared between opening and applying is one the
	// write would silently drop.
	//
	// One per field rather than one list of something they have in common. They
	// are different types and each apply path wants its own back whole: the rail
	// draws a label from its name and a person from their login.
	//
	// The state menu needs no twin of these. Its ids are the transitions
	// themselves, so applying reads them straight back off the picker.
	labels []gh.Label
	users  []gh.Actor

	// reviewers is the panel the reviewer picker was built against, held for
	// the same reason and needed for a second one: that write applies a delta,
	// so the set it is a delta from has to be the set the reader was looking at
	// when they ticked.
	reviewers []gh.Reviewer

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
//
// Only if the reader is still standing where they asked. A metadata fetch is a
// round trip, and they may have started writing a comment or walked to another
// pane meanwhile. A modal dropping over a box mid-sentence takes the keyboard
// with it, because the picker answers keys ahead of the box.
//
// A menu opened in the meantime cancels the ask rather than queueing behind it.
// The state menu needs no fetch, so it can open while this one is still owed,
// and startPicker clears want along with the rest of picking. That is deliberate
// and it is the safer of the two: a label picker that arrived late would replace
// the menu under the reader's hands between one key and the next. There is no
// third case to worry about, because a picker owns every key while it is up and
// nothing can ask for another one.
func (m *Model) SetRepo(r store.Repo) {
	m.repo = r
	if m.picking.want == pickNone || !r.Loaded {
		return
	}

	want := m.picking.want
	m.picking.want = pickNone

	if m.Composing() || !m.railVisible() || m.focus != paneRail {
		return
	}
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

	// A row in a section and the row that adds to it open the same picker. The
	// picker is the section: it is where something is taken off as well as put
	// on, so pointing at one of them is as good an ask as pointing at the add
	// row under them.
	var want pickField
	switch m.railRing.on.kind {
	case focusLabel, focusAddLabel:
		want = pickLabels
	case focusAssignee, focusAddAssignee:
		want = pickAssignees
	case focusReviewer, focusAddReviewer:
		want = pickReviewers
	case focusState:
		want = pickState
	default:
		return m, nil
	}

	// The choices are the repository's, not this pull request's, so they
	// outlive the screen and are asked for once.
	if want.needsRepo() && !m.repo.Loaded {
		m.picking.want = want
		repo := m.pr.Repository
		return m, func() tea.Msg { return NeedRepoMetaMsg{Repo: repo} }
	}

	m.startPicker(want)
	return m, nil
}

// startPicker builds the picker for a field over the choices already held. A
// field with nothing to offer opens nothing: a modal listing no choices reads
// as a fetch that came back empty.
func (m *Model) startPicker(field pickField) {
	switch field {
	case pickState:
		choices := stateChoices(m.railDetail())
		if len(choices) == 0 {
			return
		}
		m.picking = picking{
			field: field,
			// Nothing pre-checked, and single select: these are moves to make,
			// not a set to hold. No state offers more than two, well under the
			// picker's own threshold for a filter row, so the menu gets none.
			p: comp.NewPicker("State", m.stateItems(choices), nil, false),
		}

	case pickLabels:
		on := m.railDetail().Labels
		choices := labelChoices(m.repo.Meta.Labels, on)
		m.picking = picking{
			field:  field,
			labels: choices,
			p: comp.NewPicker(
				"Labels",
				labelItems(choices, m.theme.Secondary),
				idsOf(on, labelID),
				true,
			),
		}

	case pickAssignees:
		on := m.railDetail().Assignees
		choices := assigneeChoices(m.repo.Meta.Users, on)
		m.picking = picking{
			field: field,
			users: choices,
			p: comp.NewPicker(
				"Assignees",
				m.assigneeItems(choices),
				idsOf(on, actorID),
				true,
			),
		}

	case pickReviewers:
		panel := m.railDetail().Reviewers
		choices := reviewerChoices(m.repo.Meta.Users, m.pr, panel)
		m.picking = picking{
			field:     field,
			users:     choices,
			reviewers: panel,
			p: comp.NewPicker(
				"Reviewers",
				m.reviewerItems(choices),
				// Who is being waited on, not who is on the panel. A tick
				// means a review is requested, so somebody who has already
				// answered opens unchecked and ticking them asks again.
				pendingReviewers(panel),
				true,
			),
		}
	}
}

// labelChoices is every label the picker may show: the repository's, then any
// the pull request already carries that the repository's page did not reach.
//
// The union is what keeps the write honest. Both lists are a first page, one of
// a hundred and one of twenty, and applying replaces the whole set. A label the
// picker never listed is a label nobody could keep checked, so leaving it out
// here deletes it from the pull request with nothing on screen to say so.
func labelChoices(repo, onPR []gh.Label) []gh.Label {
	out := slices.Clone(repo)
	for _, l := range onPR {
		if !slices.ContainsFunc(out, func(c gh.Label) bool { return c.ID == l.ID }) {
			out = append(out, l)
		}
	}
	return out
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

// applyPicker closes the picker and asks the root to write what it chose. The
// modal is gone either way, including on a field that decides there is nothing
// to write.
func (m Model) applyPicker() (Model, tea.Cmd) {
	p := m.picking
	m.picking = picking{}

	switch p.field {
	case pickLabels:
		return m.applyLabels(p)
	case pickAssignees:
		return m.applyAssignees(p)
	case pickReviewers:
		return m.applyReviewers(p)
	case pickState:
		return m.applyState(p)
	}
	return m, nil
}

// applyLabels asks the root to write the set the picker was left holding.
//
// A set equal to what is already on the pull request writes nothing. Applying
// an unchanged picker is how a reader backs out of one they opened by mistake,
// and it should cost neither a request nor a toast.
func (m Model) applyLabels(p picking) (Model, tea.Cmd) {
	labels := byID(p.labels, p.p.Chosen(), labelID)
	if sameByID(labels, m.railDetail().Labels, labelID) {
		return m, nil
	}

	id := m.pr.ID
	return m, func() tea.Msg { return SetLabelsMsg{ID: id, Labels: labels} }
}

// A picker deals in ids and the rail draws whole things, so every field needs
// the same three moves between them. They are written once over any element
// type rather than once per field: the pair for labels and the pair for people
// were identical but for the type, comments included, and a fix to the id
// comparison in one is a fix nothing would carry to the other. The base and
// merge pickers want them too.
//
// The id is a function rather than an interface because the two spellings
// differ: a label and an assignee are chosen by node id, a reviewer by login.

// idsOf is the ids of what a pull request already carries, which is what a
// picker opens checked.
func idsOf[T any](items []T, id func(T) string) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, id(it))
	}
	return out
}

// byID is the chosen ids back as whole choices, in the order they were offered.
// The rail renders a name, so the ids alone would leave the optimistic row with
// nothing to draw.
func byID[T any](all []T, ids []string, id func(T) string) []T {
	want := make(map[string]bool, len(ids))
	for _, i := range ids {
		want[i] = true
	}

	out := make([]T, 0, len(ids))
	for _, c := range all {
		if want[id(c)] {
			out = append(out, c)
		}
	}
	return out
}

// sameByID compares two sets by id, never as sequences. They come from
// different connections: the chosen set is in the repository's order and the
// pull request's is in its own, neither query asks for an ordering, and
// comparing by position would call an untouched picker a change and fire the
// write the check exists to prevent.
func sameByID[T any](a, b []T, id func(T) string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]bool, len(a))
	for _, x := range a {
		seen[id(x)] = true
	}
	for _, y := range b {
		if !seen[id(y)] {
			return false
		}
	}
	return true
}

func labelID(l gh.Label) string { return l.ID }
func actorID(a gh.Actor) string { return a.ID }

// pickerOverlay composites the picker over a rendered frame. It is drawn here
// rather than at the root because the root does not know a picker is open, and
// the status bar stays uncovered: a toast is worth reading while a modal is up.
func (m Model) pickerOverlay(frame string) string {
	if !m.picking.open() {
		return frame
	}
	return comp.Over(frame, m.picking.p.Render(m.theme, m.width), m.width, m.height)
}
