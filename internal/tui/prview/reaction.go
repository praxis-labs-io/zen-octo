package prview

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
)

// ReactMsg asks the root to give a reaction or take it back.
//
// SubjectID is what GitHub is addressed with and it is not always a comment:
// the description is a field of the pull request, so a reaction to it names the
// pull request. CommentID and ThreadID are what the store folds against, and
// both are empty there for the same reason.
//
// On is the direction the key asked for, not the state it lands on.
type ReactMsg struct {
	ID        string
	SubjectID string
	CommentID string
	ThreadID  string
	Content   gh.ReactionContent
	On        bool
}

// reactionGlyph is the emoji a reaction draws as, and reactionName is what the
// picker calls it. Two tables rather than one row type: the card reads only the
// first and the modal reads both, and a struct here would be a struct with one
// field used twice.
//
// The names are GitHub's own words for its own buttons. Nothing here invents a
// friendlier one, because the reader who is looking for the rocket is looking
// for the word GitHub taught them.
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

// reactTarget is the block + would open the list over: where it is, the node
// GitHub is addressed with, and the reactions already on it.
//
// subjectID is separate from at.id because the two disagree on the one block
// that is not a comment. The description's focus key carries no id at all, and
// the node a reaction to it names is the pull request.
type reactTarget struct {
	at        focusKey
	subjectID string
	threadID  string
	on        []gh.Reaction
}

// reactable is what + would open on, or false where there is nothing to react
// to.
//
// It reads onRing, so it answers for whichever card the focus holds, a reply
// included: a reaction is a thing every block on the page carries, unlike the
// keys that settle a thread or go to its line.
//
// GitHub's flag is the whole of the question. Whose writing it is does not come
// into it, the way it does for an edit: reacting to your own comment is
// something GitHub's own page offers.
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

// startReact opens the list over the block the focus is on. It does nothing
// where there is nothing to react to, which is the rule every key that reads the
// ring holds to.
func (m Model) startReact() (Model, tea.Cmd) {
	w, ok := m.reactable()
	if !ok {
		return m, nil
	}

	// Opened on what the viewer has already given, which is the opposite of the
	// reason every other single-select picker opens that way. There, enter with
	// no movement changes nothing; here it takes the reaction back off. That is
	// what a toggle is, and it is what GitHub's own pill does when it is
	// clicked a second time. The tick is what says which way the press goes.
	m.picking = picking{
		field: pickReact,
		react: w,
		p:     comp.NewPicker("React", m.reactionItems(w.on), viewerGave(w.on), false),
	}
	return m, nil
}

// reactionItems is the eight, always all of them and always in GitHub's order.
// A list that offered only what somebody had already given would have nothing on
// it the first time it opened.
//
// The count rides on the row so the reader can see what they are joining. A
// reaction still out is named rather than hidden: the row is inert until it
// settles, and a row that vanished would read as a reaction that failed.
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

// reactionNamePad is the column the counts line up in: the widest glyph and name
// with a gap after it. A count that sat straight after each name would put the
// numbers in eight different places down one short list.
const reactionNamePad = 16

// viewerGave is the reactions the viewer is in, as picker ids. It is what the
// list opens ticked, and a reaction still out is left off: the row is inert, and
// a tick on it would say the write has landed.
func viewerGave(on []gh.Reaction) []string {
	var out []string
	for _, r := range on {
		if r.Viewer && !r.Pending {
			out = append(out, string(r.Content))
		}
	}
	return out
}

// applyReact asks the root to toggle the reaction the list was left on.
//
// Against the target the modal was opened over rather than whatever the ring
// holds now. Nothing can move the focus while a picker is up, but a refetch can
// land behind one.
//
// A reaction already answering for a write is refused here rather than at the
// key: two toggles on one reaction settle in the order the responses arrive,
// which is not the order they were pressed.
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

// reactionRow is the pills under a block's words, or empty where nobody has
// reacted.
//
// It renders on every card rather than on the lit one alone. A row that came
// and went with the focus would change the card's height on every press of the
// motion key, which reflows the page under a reader who was only walking it.
//
// The viewer's own read in the accent, the way a focused border does: it is the
// same question the tick answers in the list, asked of a card.
func (m Model) reactionRow(on []gh.Reaction) string {
	if len(on) == 0 {
		return ""
	}

	base := lipgloss.NewStyle().Foreground(m.theme.Subtle)
	mine := lipgloss.NewStyle().Foreground(m.theme.Accent)

	var pills []string
	for _, r := range on {
		// A pill at zero is one being taken back. It stays for as long as the
		// write is out, because it is what the key reads to stay off it, and it
		// says so rather than showing a count nobody gave.
		count := strconv.Itoa(r.Count)
		if r.Pending && r.Count == 0 {
			count = "·"
		}

		style := base
		if r.Viewer {
			style = mine
		}
		pills = append(pills, style.Render(reactionGlyph[r.Content]+" "+count))
	}
	return strings.Join(pills, "  ")
}

// withReactions is a block's words with its pills under them, which is where
// they go on every card that has any.
//
// Under rather than over, and that is load-bearing twice. GitHub puts them
// under, and cardBoxAt measures down to an open box: anything added below it
// leaves that arithmetic alone.
//
// Not while a box is over the block. The card has stopped showing what somebody
// said, the way its byline has, and the box carries its own button on its last
// row: pills under that read as a comment to react to rather than as one being
// written. Dropping them is also what keeps the box's own height honest, since
// what a card spends around one is a constant and a pill row is not in it.
func (m Model) withReactions(content string, on []gh.Reaction, key focusKey) string {
	row := m.reactionRow(on)
	if row == "" || m.boxOn(key) {
		return content
	}
	return content + "\n\n" + row
}
