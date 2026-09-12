package prview

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
)

// ReactMsg asks the root to give or take back a reaction; On is the requested direction.
// On the description SubjectID is the pull request and CommentID and ThreadID are empty.
type ReactMsg struct {
	ID        string
	SubjectID string
	CommentID string
	ThreadID  string
	Content   gh.ReactionContent
	On        bool
}

var reactionGlyph = map[gh.ReactionContent]string{
	gh.ReactionThumbsUp:   "👍",
	gh.ReactionThumbsDown: "👎",
	gh.ReactionLaugh:      "😄",
	gh.ReactionHooray:     "🎉",
	gh.ReactionConfused:   "😕",
	gh.ReactionHeart:      "❤️",
	gh.ReactionRocket:     "🚀",
	gh.ReactionEyes:       "👀",
}

var reactionName = map[gh.ReactionContent]string{
	gh.ReactionThumbsUp:   "Thumbs up",
	gh.ReactionThumbsDown: "Thumbs down",
	gh.ReactionLaugh:      "Laugh",
	gh.ReactionHooray:     "Hooray",
	gh.ReactionConfused:   "Confused",
	gh.ReactionHeart:      "Heart",
	gh.ReactionRocket:     "Rocket",
	gh.ReactionEyes:       "Eyes",
}

// subjectID differs from at.id on the description, whose key carries no id.
type reactTarget struct {
	at        focusKey
	subjectID string
	threadID  string
	on        []gh.Reaction
}

func (m Model) reactable() (reactTarget, bool) {
	w, ok := m.onRing()
	if !ok {
		return reactTarget{}, false
	}

	if w.kind == "" {
		d := m.detail.Detail
		return reactTarget{at: w.at, subjectID: d.ID, on: d.Reactions}, d.Viewer.CanReact
	}

	c, ok := m.heldComment(w)
	if !ok || !c.CanReact {
		return reactTarget{}, false
	}
	return reactTarget{
		at:        w.at,
		subjectID: c.ID,
		threadID:  w.threadID,
		on:        c.Reactions,
	}, true
}

func (m Model) startReact() (Model, tea.Cmd) {
	w, ok := m.reactable()
	if !ok {
		return m, nil
	}

	p := comp.NewPicker("React", m.reactionItems(w.on), viewerGave(w.on), false)

	p.NoFilter()

	m.picking = picking{field: pickReact, react: w, p: p}
	return m, nil
}

func (m Model) reactionItems(on []gh.Reaction) []comp.PickerItem {
	held := make(map[gh.ReactionContent]gh.Reaction, len(on))
	for _, r := range on {
		held[r.Content] = r
	}

	out := make([]comp.PickerItem, 0, len(gh.ReactionOrder))
	for _, c := range gh.ReactionOrder {
		name := reactionGlyph[c] + "  " + reactionName[c]

		switch r := held[c]; {
		case r.Pending:
			name += strings.Repeat(" ", max(1, reactionNamePad-lipgloss.Width(name))) + "writing"
		case r.Count > 0:
			name += strings.Repeat(" ", max(1, reactionNamePad-lipgloss.Width(name))) +
				strconv.Itoa(r.Count)
		}

		out = append(out, comp.PickerItem{ID: string(c), Name: name, Color: m.theme.Text})
	}
	return out
}

const reactionNamePad = 16

// A reaction still in flight is left unticked, or the tick would claim the write landed.
func viewerGave(on []gh.Reaction) []string {
	var out []string
	for _, r := range on {
		if r.Viewer && !r.Pending {
			out = append(out, string(r.Content))
		}
	}
	return out
}

// Refuses a reaction already in flight: two toggles settle in response order, not press order.
func (m Model) applyReact(p picking) (Model, tea.Cmd) {
	chosen := p.p.Chosen()
	if len(chosen) != 1 {
		return m, nil
	}

	content := gh.ReactionContent(chosen[0])
	var on bool
	for _, r := range p.react.on {
		if r.Content != content {
			continue
		}
		if r.Pending {
			return m, nil
		}
		on = r.Viewer
	}

	msg := ReactMsg{
		ID:        m.pr.ID,
		SubjectID: p.react.subjectID,
		CommentID: p.react.at.id,
		ThreadID:  p.react.threadID,
		Content:   content,
		On:        !on,
	}
	return m, func() tea.Msg { return msg }
}

// Drawn on every card rather than the lit one, so walking the ring never changes a card's height.
func (m Model) reactionRow(on []gh.Reaction, width int) string {
	if len(on) == 0 {
		return ""
	}

	base := lipgloss.NewStyle().Foreground(m.theme.Subtle)
	mine := lipgloss.NewStyle().Foreground(m.theme.Accent)

	var rows []string
	row, used := "", 0
	for _, r := range on {
		count := strconv.Itoa(r.Count)
		if r.Pending && r.Count == 0 {
			count = "·"
		}

		style := base
		if r.Viewer {
			style = mine
		}

		pill := style.Render(reactionGlyph[r.Content] + " " + count)
		cells := lipgloss.Width(pill)

		switch {
		case row == "":
			row, used = pill, cells
		case used+len(reactionGap)+cells <= width:
			row, used = row+reactionGap+pill, used+len(reactionGap)+cells
		default:
			rows = append(rows, row)
			row, used = pill, cells
		}
	}
	return strings.Join(append(rows, row), "\n")
}

const reactionGap = "  "

// Pills go under the words and drop while a box is open, which keeps cardBoxAt's measure constant.
func (m Model) withReactions(content string, on []gh.Reaction, key focusKey, width int) string {
	if m.boxOn(key) {
		return content
	}
	row := m.reactionRow(on, width)
	if row == "" {
		return content
	}
	return content + "\n\n" + row
}
