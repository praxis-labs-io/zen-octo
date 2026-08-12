package prview

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
)

// SetReviewersMsg asks the root to change who is being waited on for a review.
//
// It carries a delta rather than a set, unlike its two neighbours, because the
// endpoint behind it takes one: requesting and cancelling are separate calls,
// and there is no spelling of either that means "these and nobody else".
//
// Panel is what the rail should show meanwhile. The delta cannot be folded into
// a rendered panel without rebuilding it, and rebuilding it at the root would
// read a detail that may have been refetched since the modal opened.
//
// Repo and Number rather than the node id everything else writes by: this is
// the one write in the app that goes over REST, and REST addresses a pull
// request by where it lives.
type SetReviewersMsg struct {
	ID     string
	Repo   string
	Number int

	Add    []string
	Remove []string
	Panel  []gh.Reviewer
}

// reviewerChoices is everyone the picker offers: Copilot, then the people the
// repository lists, minus the pull request's own author.
//
// Copilot first and always. GitHub publishes no connection that reports it as a
// requestable reviewer, so it cannot be discovered; suggestedActors answers
// with the coding agent instead, which is a different bot that gets assigned
// issues. A repository without Copilot review turned on refuses the write, and
// the revert branch is what says so.
//
// The author is dropped because GitHub refuses a review requested of them, and
// a row that can only fail is worse than no row. Nobody else is filtered:
// anyone with read access can be asked, and the assignable list is narrower
// than that, so what it leaves out is a request nobody could start rather than
// one GitHub would take.
//
// Then anyone already being waited on that the repository's page did not reach.
// This is the union labelChoices builds, and it matters more here than there,
// because this picker applies a delta rather than a set: the picker checks a
// login, Chosen reports only ids it was given items for, and a checked login
// with no item silently becomes a cancellation. An outside collaborator or
// anyone past the hundredth assignable user would have their review request
// dropped by a reader who never saw them offered.
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

// pendingReviewers is who a review is currently being waited on from.
//
// It reads Requested rather than an empty State. The two are not each other's
// inverse: submitting a review clears the request and the review can then be
// asked for again, so somebody can carry a verdict and an open request at once.
// Reading the state would leave that request invisible, with no way to cancel
// it and a tick that asks for a review already pending.
//
// Teams are left out, and that is what keeps them safe. The picker offers users
// alone, so a team could never be checked, and counting one here would put it
// in the remove set and cancel a request nothing on screen offered to cancel.
func pendingReviewers(reviewers []gh.Reviewer) []string {
	var out []string
	for _, r := range reviewers {
		if !r.Team && r.Requested {
			out = append(out, r.Actor.Login)
		}
	}
	return out
}

// reviewerItems is the people as choices. Copilot is named for itself rather
// than by its login, which is the one the rail shows and not a word anybody
// would think to look for.
func (m Model) reviewerItems(users []gh.Actor) []comp.PickerItem {
	out := make([]comp.PickerItem, 0, len(users))
	for _, u := range users {
		name := comp.Handle(u.Login)
		if u.Login == gh.CopilotLogin {
			name = "Copilot"
		}
		out = append(out, comp.PickerItem{ID: u.Login, Name: name, Color: m.theme.Actor})
	}
	return out
}

// applyReviewers works out what changed and asks the root to write it.
//
// A tick means a review is requested, not that somebody is on the pull request.
// So the set it compares against is who is still being waited on, and a
// reviewer who has already answered opens unchecked: ticking them again is a
// fresh request, which is what GitHub's own re-request button does.
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

// missing is everything in a that b does not carry.
func missing(a, b []string) []string {
	var out []string
	for _, x := range a {
		if !slices.Contains(b, x) {
			out = append(out, x)
		}
	}
	return out
}

// nextPanel is the reviewer list as the rail should read it while the write is
// out: the panel it had, minus the requests this cancels, plus a row for
// everyone newly asked.
//
// A verdict already given stays whatever the picker says, and so does every
// team. Neither is something this write can take off: cancelling reaches an
// outstanding request and nothing else, and a submitted review stays on the
// panel until somebody dismisses it somewhere else entirely. What moves on such
// a row is the request beside the verdict, which is why the two are separate
// fields.
//
// A row that was only a request and is no longer wanted goes altogether. There
// is nothing left of it once the request is cancelled.
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
