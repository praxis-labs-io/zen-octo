package prview

import (
	"image/color"

	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
)

// SetStateMsg asks the root to move a pull request through its lifecycle. It
// carries the transition the reader picked rather than the state it lands on,
// for the reason ResolveThreadMsg gives: the root writes what it was handed and
// never reads the page back.
type SetStateMsg struct {
	ID string
	To gh.PRTransition
}

// stateChoices is every transition this pull request will take, in the order
// the menu offers them: the one that changes what it is first, then the one
// that ends it.
//
// It reads State and IsDraft rather than the row's own words. They are two
// independent fields and a closed draft is both, but PRStateLabel shows drafts
// ahead of everything else, so that row says "Draft" whether or not it is still
// open. Offering the draft toggle off that word would put "Ready for review" on
// a pull request that is closed.
//
// Permission is GitHub's answer, not a guess. A menu item that opens a write
// GitHub refuses is worse than no item, and viewerCanUpdate is the flag those
// two draft mutations actually track.
//
// Merged is the end of the line: nothing moves it, so nothing is offered and
// the row stops being somewhere tab stops.
func stateChoices(d gh.PullRequestDetail) []gh.PRTransition {
	if d.State == gh.PRStateMerged {
		return nil
	}

	var out []gh.PRTransition

	if d.State == gh.PRStateClosed {
		if d.Viewer.CanReopen {
			out = append(out, gh.TransitionReopen)
		}
		return out
	}

	if d.Viewer.CanUpdate {
		if d.IsDraft {
			out = append(out, gh.TransitionReady)
		} else {
			out = append(out, gh.TransitionDraft)
		}
	}
	if d.Viewer.CanClose {
		out = append(out, gh.TransitionClose)
	}
	return out
}

// stateItems is the menu as the picker takes it. The id is the transition
// itself, so applying reads one back without a lookup table.
func (m Model) stateItems(choices []gh.PRTransition) []comp.PickerItem {
	out := make([]comp.PickerItem, 0, len(choices))
	for _, to := range choices {
		name, c := m.stateChoice(to)
		out = append(out, comp.PickerItem{ID: string(to), Name: name, Color: c})
	}
	return out
}

// stateChoice names a transition and colors it as the state it produces, so the
// menu reads the way the row it will rewrite does.
func (m Model) stateChoice(to gh.PRTransition) (string, color.Color) {
	switch to {
	case gh.TransitionReady:
		return "Ready for review", m.theme.Success
	case gh.TransitionDraft:
		return "Convert to draft", m.theme.Faint
	case gh.TransitionClose:
		return "Close", m.theme.Error
	case gh.TransitionReopen:
		return "Reopen", m.theme.Success
	}
	return string(to), m.theme.Faint
}

// applyState asks the root for the transition the menu was left on.
//
// No unchanged check, unlike labels: the menu never offers a move the pull
// request has already made, so every pick is a change.
//
// No check that the transition is one the menu offered either. The ids come
// from the items, the items were built from the choices, and a picker cannot
// return one it was not given. GitHub is the authority on whether the move is
// still available, and the revert branch is what answers when it is not.
func (m Model) applyState(p picking) (Model, tea.Cmd) {
	chosen := p.p.Chosen()
	if len(chosen) != 1 {
		return m, nil
	}

	id := m.pr.ID
	to := gh.PRTransition(chosen[0])
	return m, func() tea.Msg { return SetStateMsg{ID: id, To: to} }
}
