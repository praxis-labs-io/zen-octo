package prview

import (
	"image/color"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
)

type SetStateMsg struct {
	ID string
	To gh.PRTransition
}

// Reads State and IsDraft rather than the row label, which says Draft on a closed draft.
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

func (m Model) stateItems(choices []gh.PRTransition) []comp.PickerItem {
	out := make([]comp.PickerItem, 0, len(choices))
	for _, to := range choices {
		name, c := m.stateChoice(to)
		out = append(out, comp.PickerItem{ID: string(to), Name: name, Color: c})
	}
	return out
}

func (m Model) stateChoice(to gh.PRTransition) (string, color.Color) {
	switch to {
	case gh.TransitionReady:
		return "Ready for review", m.theme.Success
	case gh.TransitionDraft:
		return "Convert to draft", m.theme.Subtle
	case gh.TransitionClose:
		return "Close", m.theme.Error
	case gh.TransitionReopen:
		return "Reopen", m.theme.Success
	}
	return string(to), m.theme.Subtle
}

func (m Model) applyState(p picking) (Model, tea.Cmd) {
	chosen := p.p.Chosen()
	if len(chosen) != 1 {
		return m, nil
	}

	id := m.pr.ID
	to := gh.PRTransition(chosen[0])
	return m, func() tea.Msg { return SetStateMsg{ID: id, To: to} }
}
