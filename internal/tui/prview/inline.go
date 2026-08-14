package prview

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/keys"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// inline is the box the page summons: an answer under a review thread, or a
// comment being rewritten where it sits. One type for both, because they are
// the same box at every point that matters. It appears where it is needed
// rather than living on the page, it holds what was written against whatever it
// was opened on, and it is the only box that can share a screen with the
// compose card.
//
// It is a second composer rather than the compose card retargeted: one textarea
// drawn at two places is one buffer, and opening this would then wipe out a
// top-level comment half written at the foot of the page.
//
// Only what it is open on tells a reply from an edit, which is why at is a
// focusKey rather than an id. A thread key posts an answer under the thread; a
// comment's own key rewrites that comment in place.
type inline struct {
	composer

	// at is what the box is open on, zero when it is closed.
	at focusKey

	// from is where focus goes when the box closes. Not what was written: a
	// posted reply is a placeholder keyed by an id minted locally, and the
	// highlight would vanish on its own the moment GitHub answered with the real
	// one.
	from focusKey

	// drafts is what was written into a box and not sent, by target. Closing the
	// box takes it off the page, and a reader who pressed esc to look at the code
	// above their answer has not thrown the answer away.
	drafts map[focusKey]string
}

// replyRows is the writing a reply box shows at once. Half the compose card's,
// because an answer is shorter than an opening argument and what it answers is
// on the screen above it. $EDITOR is there for the ones that need more.
//
// An edit takes no fixed height. It is standing in for words already on the
// page, and a box of its own size would resize the card the moment it opened.
const replyRows = 4

func newInline(th theme.Theme) inline {
	return inline{composer: newComposer(th)}
}

// open points the box at something, filling it with whatever was left there and
// falling back to the text handed in.
//
// The fallback is what an edit opens on: the comment as it stands. A reply has
// none, so a thread nobody has written to opens empty. A draft wins over both,
// because it is the newer of the two and the reader typed it.
func (r *inline) open(at, from focusKey, fallback string, w words) tea.Cmd {
	r.at, r.from, r.words = at, from, w
	r.area.Placeholder = w.placeholder

	body := fallback
	if held, ok := r.drafts[at]; ok {
		body = held
	}
	r.area.SetValue(body)
	r.area.MoveToEnd()
	return r.start()
}

// close takes the box off the page.
//
// A reply's words are kept against the thread they answer: esc is how a reader
// looks at the code above what they are writing. An edit's are dropped, which
// is what its own hint says the key does. The comment is on GitHub either way,
// so nothing is lost that cannot be read again, and a draft held here would
// open over a comment that has moved on since and be saved as if it were the
// newer of the two.
func (r *inline) close() {
	if r.editing() {
		delete(r.drafts, r.at)
	} else {
		r.keep(r.at, "")
	}

	r.at = focusKey{}
	r.area.Reset()
	r.stop()
}

// keep holds words for a target, behind whatever is already held for it. Two
// answers to one thread run together is a mess the reader can cut apart, and it
// is the smaller loss: everything else on this screen can be fetched again.
func (r *inline) keep(at focusKey, body string) {
	if at.kind == focusNone {
		return
	}

	if at == r.at {
		body = joinDraft(body, r.body())
	} else {
		body = joinDraft(r.drafts[at], body)
	}

	if body == "" {
		delete(r.drafts, at)
		return
	}
	if r.drafts == nil {
		r.drafts = make(map[focusKey]string)
	}
	r.drafts[at] = body
}

func joinDraft(first, second string) string {
	switch {
	case first == "":
		return second
	case second == "":
		return first
	}
	return first + "\n\n" + second
}

// editing is whether the box is rewriting a comment rather than answering a
// thread. The kind is the whole of the answer: a reply is opened on the thread
// it hangs under, and everything else the box opens on is a block of prose it
// is replacing.
func (r inline) editing() bool { return r.at.kind != focusNone && r.at.kind != focusReply }

// boxOn is whether the summoned box is open on a block. The render sites read
// it to draw the textarea where the words would be.
func (m Model) boxOn(key focusKey) bool {
	return m.inline.typing && m.inline.at == key
}

// boxIn is whether the summoned box belongs to a thread: an answer hanging
// under it, or one of its comments being rewritten. It is what the page splits
// at while somebody types.
func (m Model) boxIn(t gh.ReviewThread) bool {
	on := m.inline.at
	switch on.kind {
	case focusNone:
		return false
	case focusReply:
		return on.id == t.ID
	case focusThreadComment:
		for _, c := range t.Comments {
			if c.ID == on.id {
				return true
			}
		}
	}
	return false
}

// inlineKey is every key while the box has the keyboard. It answers the handful
// that belong to it and hands the rest to the text.
func (m Model) inlineKey(keyMsg tea.KeyPressMsg) (Model, tea.Cmd) {
	// The popup first, for the reason the compose card takes it first: esc here
	// closes the box and throws an edit's draft away with it.
	if next, cmd, took := m.mentionKey(keyMsg); took {
		return next, cmd
	}
	k := keys.Detail

	switch {
	case key.Matches(keyMsg, k.Back):
		return m.closeInline()

	case key.Matches(keyMsg, k.Editor):
		return m, m.inline.editorCmd()

	case key.Matches(keyMsg, keys.Form.Next), key.Matches(keyMsg, keys.Form.Prev):
		cmd := m.inline.step()
		m.syncContent()
		return m, cmd

	case key.Matches(keyMsg, k.Post):
		return m.sendInline()

	case key.Matches(keyMsg, k.Activate) && m.inline.onPost:
		return m.sendInline()
	}

	var cmd tea.Cmd
	m.inline.area, cmd = m.inline.area.Update(keyMsg)
	ask := m.syncMention()
	m.showInline()
	return m, tea.Batch(cmd, ask)
}

// sendInline hands the buffer to whichever write the box was opened for.
func (m Model) sendInline() (Model, tea.Cmd) {
	m.clearMention()
	if m.inline.editing() {
		return m.saveEdit()
	}
	return m.postReply()
}

// closeInline hands the keyboard back and takes the box off the page. Focus
// goes back to where it was opened from, so esc leaves the reader where they
// were rather than nowhere.
func (m Model) closeInline() (Model, tea.Cmd) {
	from := m.inline.from
	m.inline.close()
	m.clearMention()

	m.convRing.on = from
	m.conv.ok = false
	m.syncContent()
	return m, nil
}

// showInline rebuilds the page and keeps the box in view.
//
// It moves the shortest distance, which is the wrong move everywhere else on
// this screen and the right one here. A key that lands on a block is taking the
// reader somewhere, so the block goes to the top row with its own content under
// it. A keystroke inside a box is not taking them anywhere: the eye is on a
// caret that has not moved, and hauling the page to put the box on the top row
// because a character was typed is the worse of the two wrongs.
//
// The caret is the whole of it, and the card holding the box is not consulted.
// The reply box hangs off a thread and an edit can be the comment that opened
// one, and neither is a ring stop: a scroll that went looking for the block
// would find nothing and do nothing in exactly the two cases the box is nested.
// The caret is somewhere in every one of them, and the button rides one row
// below it.
func (m *Model) showInline() {
	m.syncContent()
	m.showCaret()
}

// showOpenedBox brings a box that has just appeared onto the page whole, foot
// and all. Opening one is a journey where typing in one is not: the reader
// asked for a control, and the control is the writing area, the button under it
// and the border that closes them.
//
// showCaret cannot do it. The caret opens on the box's first row, so following
// it leaves everything below that row off the screen, which on a fresh reply is
// three rows of box, the button and the border. Its own claim that a caret
// brings the button with it holds only once the writing has reached the last
// row.
//
// It still moves the shortest distance, and never past the caret's own row. A
// box taller than the window cannot be shown whole, and there the caret is the
// part to keep: the reader is about to type into it.
func (m *Model) showOpenedBox() {
	m.syncContent()

	box := m.writing()
	if box == nil || m.boxLine <= 0 {
		return
	}
	top, height := bodyTop(&m.view), m.view.Height()
	if height <= 0 {
		return
	}

	// The button rides one row under the writing area and the border one under
	// that, and neither is in the height the textarea reports.
	foot := m.boxLine + box.area.Height() + 1
	switch {
	case foot >= top+height:
		m.view.SetYOffset(contentLead + min(foot-height+1, m.boxLine))
	case m.boxLine < top:
		m.view.SetYOffset(contentLead + m.boxLine)
	}
}

// showCaret brings the line being written on back onto the page, and it has the
// last word over anything that scrolled to the block.
//
// A box grows with what is typed into it, so it can be taller than the window,
// and then the block holding it says nothing about where the caret is: bringing
// the whole card into view is impossible and bringing its foot into view can
// leave a caret in the middle of a long comment above the top row. This is the
// one scroll on this screen that follows the caret rather than a block.
//
// It moves the shortest distance, for the reason typing does at all: a
// character is not a journey, and hauling the page for one is worse than the
// line arriving at an edge.
func (m *Model) showCaret() {
	box := m.writing()
	if box == nil || m.boxLine <= 0 {
		return
	}

	// Clamped to the rows the box is showing. The caret is drawn inside the box
	// and cannot be outside it: past the cap the textarea scrolls within itself,
	// and a count taken before it has repositioned would send the page chasing a
	// line that is not on it.
	row := min(max(box.caretRow(box.area.Width()), 0), max(0, box.area.Height()-1))

	caret := m.boxLine + row
	top, height := bodyTop(&m.view), m.view.Height()
	if height <= 0 {
		return
	}

	// Down to the row under the caret rather than the caret's own, so a caret on
	// the last row of the box brings the button with it. The control that sends
	// the words sits directly beneath them, and a box whose foot is one line
	// below the fold leaves the reader writing into something with no visible
	// end and no way out but a chord nothing on the screen has named.
	switch {
	case caret < top:
		m.view.SetYOffset(contentLead + caret)
	case caret+1 >= top+height:
		m.view.SetYOffset(contentLead + caret + 2 - height)
	}
}

// inlineBox is the box drawn where the block it answers for goes, at floor rows
// or however many the writing in it has grown to.
//
// An edit's floor is what the words it replaced occupied, less the row the
// button takes, so opening one leaves the card exactly the height it was and
// nothing below it moves. A card that resized as the box opened would haul the
// page under the reader for a key that has changed nothing yet.
//
// Sizing here rather than at open is what keeps that true through a resize and
// through every keystroke: the same words wrap to a different height at a
// different width, and both are only known where the block is drawn.
func (m *Model) inlineBox(width, floor, chrome int) string {
	m.inline.setWidth(width)
	m.inline.area.SetHeight(m.boxRows(m.inline.composer, floor, width, chrome))
	return m.inline.area.View() + "\n" + m.inline.button(m.theme, width, true)
}
