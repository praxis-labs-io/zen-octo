package prview

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
)

// SetAssigneesMsg asks the root to write an assignee set on this pull request.
// It carries the whole set rather than what changed, for the reason
// SetLabelsMsg gives: the picker applies a set and the mutation takes one.
type SetAssigneesMsg struct {
	ID        string
	Assignees []gh.Actor
}

// assigneeChoices is everyone the picker may show: the repository's assignable
// users, then anyone already assigned that the repository's page did not reach.
//
// The union is what keeps the write honest, the same way labelChoices does.
// Both lists are a first page and applying replaces the whole set, so someone
// the picker never listed is someone nobody could keep checked, and leaving
// them out here unassigns them with nothing on screen to say so.
func assigneeChoices(repo, onPR []gh.Actor) []gh.Actor {
	out := slices.Clone(repo)
	for _, a := range onPR {
		if !slices.ContainsFunc(out, func(c gh.Actor) bool { return c.ID == a.ID }) {
			out = append(out, a)
		}
	}
	return out
}

// assigneeItems is the people as choices, written the way the rail writes them
// so the picker reads the same as the rows it rewrites. The id is the node id,
// which is the only spelling updatePullRequest takes.
func (m Model) assigneeItems(users []gh.Actor) []comp.PickerItem {
	out := make([]comp.PickerItem, 0, len(users))
	for _, u := range users {
		out = append(out, comp.PickerItem{ID: u.ID, Name: comp.Handle(u.Login), Color: m.theme.Actor})
	}
	return out
}

func actorIDs(actors []gh.Actor) []string {
	out := make([]string, 0, len(actors))
	for _, a := range actors {
		out = append(out, a.ID)
	}
	return out
}

// applyAssignees asks the root to write the set the picker was left holding.
//
// A set equal to the one already on the pull request writes nothing. Applying
// an unchanged picker is how a reader backs out of one they opened by mistake,
// and it should cost neither a request nor a toast.
func (m Model) applyAssignees(p picking) (Model, tea.Cmd) {
	assignees := actorsByID(p.users, p.p.Chosen())
	if sameActors(assignees, m.railDetail().Assignees) {
		return m, nil
	}

	id := m.pr.ID
	return m, func() tea.Msg { return SetAssigneesMsg{ID: id, Assignees: assignees} }
}

// actorsByID is the chosen ids back as whole people, in the order the picker
// offered them. The rail renders a login, so the ids alone would leave the
// optimistic row with nothing to draw.
func actorsByID(all []gh.Actor, ids []string) []gh.Actor {
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}

	out := make([]gh.Actor, 0, len(ids))
	for _, a := range all {
		if want[a.ID] {
			out = append(out, a)
		}
	}
	return out
}

// sameActors compares the two as sets of ids, never as sequences, for the
// reason sameLabels gives: the chosen set is in the repository's order and the
// pull request's is in its own, and comparing by position would call an
// untouched picker a change.
func sameActors(a, b []gh.Actor) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]bool, len(a))
	for _, x := range a {
		seen[x.ID] = true
	}
	for _, y := range b {
		if !seen[y.ID] {
			return false
		}
	}
	return true
}
