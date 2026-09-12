package prview

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
)

type SetAssigneesMsg struct {
	ID        string
	Assignees []gh.Actor
}

// Unions in the current assignees: applying replaces the set, so one never listed would be silently unassigned.
func assigneeChoices(repo, onPR []gh.Actor) []gh.Actor {
	out := slices.Clone(repo)
	for _, a := range onPR {
		if !slices.ContainsFunc(out, func(c gh.Actor) bool { return c.ID == a.ID }) {
			out = append(out, a)
		}
	}
	return out
}

func (m Model) assigneeItems(users []gh.Actor) []comp.PickerItem {
	out := make([]comp.PickerItem, 0, len(users))
	for _, u := range users {
		out = append(out, comp.PickerItem{ID: u.ID, Name: comp.Handle(u.Login), Color: m.theme.Actor})
	}
	return out
}

func (m Model) applyAssignees(p picking) (Model, tea.Cmd) {
	assignees := byID(p.users, p.p.Chosen(), actorID)
	if sameByID(assignees, m.railDetail().Assignees, actorID) {
		return m, nil
	}

	id := m.pr.ID
	return m, func() tea.Msg { return SetAssigneesMsg{ID: id, Assignees: assignees} }
}
