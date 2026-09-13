package prview

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
)

// SetReviewersMsg asks the root to request reviews from Add and cancel them for
// Remove, logins on the pull request at Repo and Number, showing Panel meanwhile.
type SetReviewersMsg struct {
	ID     string
	Repo   string
	Number int

	Add    []string
	Remove []string
	Panel  []gh.Reviewer
}

// Copilot is always offered because nothing reports it; pending requests are unioned in or the delta would cancel them.
func reviewerChoices(users []gh.Actor, pr gh.PullRequest, panel []gh.Reviewer) []gh.Actor {
	out := []gh.Actor{{Login: gh.CopilotLogin}}
	for _, u := range users {
		if u.Login != pr.Author.Login {
			out = append(out, u)
		}
	}

	for _, login := range pendingReviewers(panel) {
		if !slices.ContainsFunc(out, func(c gh.Actor) bool { return c.Login == login }) {
			out = append(out, gh.Actor{Login: login})
		}
	}
	return out
}

// Reads Requested, not State: a reviewer can hold a verdict and an open request at once.
func pendingReviewers(reviewers []gh.Reviewer) []string {
	var out []string
	for _, r := range reviewers {
		if !r.Team && r.Requested {
			out = append(out, r.Actor.Login)
		}
	}
	return out
}

func (m Model) reviewerItems(users []gh.Actor) []comp.PickerItem {
	out := make([]comp.PickerItem, 0, len(users))
	for _, u := range users {
		out = append(out, comp.PickerItem{ID: u.Login, Name: actorName(u.Login), Color: m.theme.Actor})
	}
	return out
}

func (m Model) applyReviewers(p picking) (Model, tea.Cmd) {
	want := p.p.Chosen()
	have := pendingReviewers(p.reviewers)

	add := missing(want, have)
	remove := missing(have, want)
	if len(add) == 0 && len(remove) == 0 {
		return m, nil
	}

	msg := SetReviewersMsg{
		ID:     m.pr.ID,
		Repo:   m.pr.Repository,
		Number: m.pr.Number,
		Add:    add,
		Remove: remove,
		Panel:  nextPanel(p.reviewers, want),
	}
	return m, func() tea.Msg { return msg }
}

func missing(a, b []string) []string {
	var out []string
	for _, x := range a {
		if !slices.Contains(b, x) {
			out = append(out, x)
		}
	}
	return out
}

// Keeps team rows and given verdicts: a cancel reaches only an outstanding request.
func nextPanel(held []gh.Reviewer, want []string) []gh.Reviewer {
	out := make([]gh.Reviewer, 0, len(held)+len(want))
	on := make(map[string]bool, len(held)+len(want))

	for _, r := range held {
		switch {
		case r.Team:
		case slices.Contains(want, r.Actor.Login):
			r.Requested = true
		case r.State != "":
			r.Requested = false
		default:
			continue
		}
		out = append(out, r)
		on[r.Actor.Login] = true
	}

	for _, login := range want {
		if !on[login] {
			out = append(out, gh.Reviewer{Actor: gh.Actor{Login: login}, Requested: true})
		}
	}
	return out
}
