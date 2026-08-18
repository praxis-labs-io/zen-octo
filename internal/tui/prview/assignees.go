package prview

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
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

// applyAssignees asks the root to write the set the picker was left holding.
//
// A set equal to the one already on the pull request writes nothing. Applying
// an unchanged picker is how a reader backs out of one they opened by mistake,
// and it should cost neither a request nor a toast.
func (m Model) applyAssignees(p picking) (Model, tea.Cmd) {
	assignees := byID(p.users, p.p.Chosen(), actorID)
	if sameByID(assignees, m.railDetail().Assignees, actorID) {
		return m, nil
	}

	id := m.pr.ID
	return m, func() tea.Msg { return SetAssigneesMsg{ID: id, Assignees: assignees} }
}
