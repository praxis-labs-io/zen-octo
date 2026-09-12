package prview

import (
	"image/color"
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/keys"
)

type pickField int

const (
	pickNone pickField = iota
	pickLabels
	pickState
	pickAssignees
	pickReviewers
	pickBase
	// pickMerge opens a form, not a picker, but waits on the same repository fetch.
	pickMerge
	pickDelete
	pickReact
)

// pickBase is absent: its choices are a search, and SetBranches resumes it.
func (f pickField) needsRepo() bool {
	return f == pickLabels || f == pickAssignees || f == pickReviewers || f == pickMerge
}

type picking struct {
	field pickField
	p     comp.Picker

	// Snapshotted at open so a refetch behind the modal cannot change what applying writes.
	labels    []gh.Label
	users     []gh.Actor
	reviewers []gh.Reviewer
	on        target
	react     reactTarget

	want   pickField
	wantOn focusKey
}

func (p picking) open() bool { return p.field != pickNone }

type NeedRepoMetaMsg struct{ Repo string }

type SetLabelsMsg struct {
	ID     string
	Labels []gh.Label
}

// Capturing reports whether a box, picker, merge form or job log search owns
// the keyboard, in which case the root must pass every key through.
func (m Model) Capturing() bool {
	return m.Composing() || m.picking.open() || m.merging.open || m.check.searching
}

// SetRepo holds the repository's metadata, refills an open mention list, and
// opens a picker or merge form waiting on it if the reader is still on the rail
// row that asked. Returns any command the mention list or merge form needs.
func (m *Model) SetRepo(r store.Repo) tea.Cmd {
	m.repo = r

	ask := m.refillMentions()

	if !m.picking.want.needsRepo() || !r.Loaded {
		return ask
	}

	want, on := m.picking.want, m.picking.wantOn
	m.picking.want, m.picking.wantOn = pickNone, focusKey{}

	if m.Capturing() || !m.railVisible() || m.focus != paneRail {
		return ask
	}

	if m.railRing.on != on {
		return ask
	}

	if want == pickMerge {
		return tea.Batch(ask, m.startMerge())
	}
	m.startPicker(want)
	return ask
}

func (m Model) openRailPicker() (Model, tea.Cmd) {
	if !m.detail.Loaded || !m.railRing.live(bodyTop(&m.railView), m.railView.Height()) {
		return m, nil
	}

	if m.railRing.on.kind == focusCheck {
		return m.showCheckFromRail()
	}

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
	case focusBase:
		want = pickBase
	case focusMerge:
		want = pickMerge
	default:
		return m, nil
	}

	if want.needsRepo() && !m.repo.Loaded {
		m.picking.want, m.picking.wantOn = want, m.railRing.on
		repo := m.pr.Repository
		return m, func() tea.Msg { return NeedRepoMetaMsg{Repo: repo} }
	}

	if want == pickBase && (!m.branches.Loaded || m.branches.Query != "") {
		m.picking.want = want
		repo := m.pr.Repository
		return m, func() tea.Msg { return NeedBranchesMsg{Repo: repo} }
	}

	if want == pickMerge {
		return m, m.startMerge()
	}
	m.startPicker(want)
	return m, nil
}

func (m *Model) startPicker(field pickField) {
	switch field {
	case pickState:
		choices := stateChoices(m.railDetail())
		if len(choices) == 0 {
			return
		}
		m.picking = picking{
			field: field,
			p:     comp.NewPicker("State", m.stateItems(choices), nil, false),
		}

	case pickLabels:
		on := m.railDetail().Labels
		choices := labelChoices(m.repo.Meta.Labels, on)
		m.picking = picking{
			field:  field,
			labels: choices,
			p: comp.NewPicker(
				"Labels",
				labelItems(choices, m.theme.Accent),
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
				pendingReviewers(panel),
				true,
			),
		}

	case pickBase:
		base := m.railDetail().BaseRefName

		p := comp.NewPicker(
			"Merge into",
			baseItems(m.branches, m.pr, base, m.theme.Text),
			[]string{base},
			false,
		)
		p.SetNote(branchNote(m.branches.More))
		m.picking = picking{field: field, p: p}
	}
}

// Unions in the pull request's own labels: applying replaces the set, so one never listed would be silently removed.
func labelChoices(repo, onPR []gh.Label) []gh.Label {
	out := slices.Clone(repo)
	for _, l := range onPR {
		if !slices.ContainsFunc(out, func(c gh.Label) bool { return c.ID == l.ID }) {
			out = append(out, l)
		}
	}
	return out
}

func labelItems(labels []gh.Label, accent color.Color) []comp.PickerItem {
	out := make([]comp.PickerItem, 0, len(labels))
	for _, l := range labels {
		out = append(out, comp.PickerItem{ID: l.ID, Name: l.Name, Color: accent})
	}
	return out
}

// Order is load-bearing: keys that are never text, then the filter takes every printable key, then movement.
func (m Model) pickerKey(keyMsg tea.KeyPressMsg) (Model, tea.Cmd) {
	k := keys.Detail

	switch {
	case key.Matches(keyMsg, k.Back):
		m.picking = picking{}
		return m, nil

	case key.Matches(keyMsg, k.Activate):
		return m.applyPicker()

	case key.Matches(keyMsg, keys.Form.Toggle) && m.picking.p.Multi():
		m.picking.p.Toggle()
		return m, nil
	}

	if m.picking.p.Insert(keyMsg) {
		if m.picking.field == pickBase {
			return m, m.armBranches()
		}
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
	case pickBase:
		return m.applyBase(p)
	case pickState:
		return m.applyState(p)
	case pickDelete:
		return m.applyDelete(p)
	case pickReact:
		return m.applyReact(p)
	}
	return m, nil
}

func (m Model) applyLabels(p picking) (Model, tea.Cmd) {
	labels := byID(p.labels, p.p.Chosen(), labelID)
	if sameByID(labels, m.railDetail().Labels, labelID) {
		return m, nil
	}

	id := m.pr.ID
	return m, func() tea.Msg { return SetLabelsMsg{ID: id, Labels: labels} }
}

func idsOf[T any](items []T, id func(T) string) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, id(it))
	}
	return out
}

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

// By id, not position: the two sets come from connections with different orders.
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

func (m Model) pickerOverlay(frame string) string {
	if !m.picking.open() {
		return frame
	}
	return comp.Over(frame, m.picking.p.Render(m.theme, m.width), m.width, m.height)
}
