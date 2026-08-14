package prview

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
)

// quote puts a comment in the buffer with the cursor under it, the way the
// browser's quote reply does. The raw body goes in, not the rendered markdown: a
// quote of a fenced block has to come out the other side as a fenced block.
//
// A comment with no body quotes to nothing, and the box opens on whatever was
// already in it rather than on a blockquote marker standing over an empty line.
func (c *composer) quote(body string) {
	q := quoted(body)
	if q == "" {
		return
	}
	c.area.SetValue(joinDraft(c.body(), q) + "\n\n")
	c.area.MoveToEnd()
}

// quoted is a body as a blockquote. An empty line takes a bare marker: the
// trailing space a uniform "> " would leave is invisible here and real in the
// comment GitHub stores.
func quoted(body string) string {
	body = strings.TrimRight(body, "\n")
	if body == "" {
		return ""
	}

	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if line == "" {
			lines[i] = ">"
			continue
		}
		lines[i] = "> " + line
	}
	return strings.Join(lines, "\n")
}

// replyKey is what a box answering a thread is open on, and the stop it takes
// in the ring. Tab never walks onto it: the box has the keyboard from the
// moment it appears. It is in the ring so the page can be scrolled to keep it
// in view.
func replyKey(threadID string) focusKey {
	return focusKey{kind: focusReply, id: threadID}
}

// writingCard is the box, rendered through the same card every reply renders
// through and hung off the same rail, so an answer being written sits among the
// answers already made rather than beside them.
//
// An edit inside the thread needs none of this. It is drawn where the words it
// replaces were, so the card holding them renders it.
func (m *Model) writingCard(width int) string {
	key := replyKey(m.inline.at.id)

	inner := m.cardWidth(width)
	head := m.said(m.who, "write a reply", m.theme.Subtle, gh.TimelineItem{})
	return m.card(head, m.inlineBox(inner, replyRows, boxChrome), width, m.lit(key), "")
}

// within is the comment the focused card holds: the one that opened the thread
// when the ring is on the thread's own card, and the reply itself when it is on
// one of the cards hanging off it.
//
// Every card is a ring stop, so the focus already names a comment and nothing
// has to be remembered beside it. This is only the lookup.
func (m Model) within(t gh.ReviewThread) string {
	on := m.convRing.on
	if on.kind == focusThreadComment && slices.ContainsFunc(t.Comments,
		func(c gh.Comment) bool { return c.ID == on.id }) {
		return on.id
	}
	if len(t.Comments) == 0 {
		return ""
	}
	return t.Comments[0].ID
}

// threadOpen is whether a thread is showing its comments. Everything unresolved
// is; a resolved one is closed until o opens it, and while it is closed there is
// nothing inside it to step through, quote, or hang a box off.
func (m Model) threadOpen(t gh.ReviewThread) bool {
	return !t.IsResolved || m.expanded[threadKey(t)]
}

// focusedThread is the open thread the ring is anywhere inside, on the screen.
// A reply answers with the thread it hangs off: r adds to that thread, which is
// the only reply GitHub has, and e and D pick the card out of it by id.
func (m Model) focusedThread() (gh.ReviewThread, bool) {
	t, ok := m.threadHolding()
	return t, ok && m.threadOpen(t)
}

// threadOnRing is the thread whose own card the ring is on, open or not, which
// is what o needs: closed is the state it exists to change.
//
// A reply is not it. The card holding the anchor, the code and the comment that
// opened the thread is the thread; the cards hanging off it are answers to that
// comment, and x and v mean the code, not the answer.
func (m Model) threadOnRing() (gh.ReviewThread, bool) {
	on := m.convRing.on
	if !m.answerable() || on.kind != focusThread {
		return gh.ReviewThread{}, false
	}
	for _, t := range m.detail.Detail.Threads {
		if t.ID == on.id {
			return t, true
		}
	}
	return gh.ReviewThread{}, false
}

// threadHolding is the thread the ring is anywhere inside: its own card, or one
// of the replies hanging off it. It is what the keys that answer or rewrite a
// comment read, because those mean whichever card is under them.
func (m Model) threadHolding() (gh.ReviewThread, bool) {
	if t, ok := m.threadOnRing(); ok {
		return t, ok
	}
	if on := m.convRing.on; m.answerable() && on.kind == focusThreadComment {
		for _, t := range m.detail.Detail.Threads {
			if slices.ContainsFunc(t.Comments, func(c gh.Comment) bool { return c.ID == on.id }) {
				return t, true
			}
		}
	}
	return gh.ReviewThread{}, false
}

// answerable is whether the ring is holding something the reply keys can act
// on at all: a block of prose, on the tab that draws boxes, on the screen.
//
// A focus scrolled out of the window is not one to act on, which is the rule
// every key that reads the ring holds to. Opening a box on a comment the reader
// has already scrolled past would haul them back to it.
func (m Model) answerable() bool {
	return m.canCompose() && m.focus == paneMain &&
		m.convRing.live(bodyTop(&m.view), m.view.Height())
}

// replyThread is the thread the ring is on and the comment inside it a quote
// would take, when GitHub will take a reply to it.
func (m Model) replyThread() (gh.ReviewThread, gh.Comment, bool) {
	t, ok := m.focusedThread()
	if !ok || !t.CanReply {
		return gh.ReviewThread{}, gh.Comment{}, false
	}

	within := m.within(t)
	for _, c := range t.Comments {
		if c.ID == within {
			return t, c, true
		}
	}
	return t, gh.Comment{}, true
}

// replyBody is what the keys would answer when the ring is on a block with no
// thread under it: a top-level comment, a review's own words, or the
// description.
//
// There is nothing to hang a reply off in the API for any of them. GitHub does
// not thread the conversation, so answering one is a new comment at the foot of
// the page, which is what the browser's quote reply does too. That is a
// narrower thing than answering a review thread, and it is not nothing: without
// it r is a key that does nothing on most of the page.
func (m Model) replyBody() (string, bool) {
	on := m.convRing.on
	if !m.answerable() {
		return "", false
	}

	switch on.kind {
	case focusDescription:
		return m.detail.Detail.Body, true
	case focusComment, focusReview:
		for _, item := range m.detail.Detail.Timeline {
			if said := item.Said(); said.ID == on.id {
				return said.Body, true
			}
		}
	}
	return "", false
}

// startReply opens a box on whatever the ring is holding. quote puts the focused
// words in it first, which is the whole difference between the two keys.
func (m Model) startReply(quote bool) (Model, tea.Cmd) {
	if t, c, ok := m.replyThread(); ok {
		return m.openReply(t, c, quote)
	}

	// No thread under it, so there is nothing to reply to and r says so by doing
	// nothing. A quote is a different matter: it carries the words it answers, so
	// it stands on its own as a comment on the pull request, which is what the
	// browser's quote reply produces too.
	if body, ok := m.replyBody(); ok && quote {
		return m.openCompose(body)
	}
	return m, nil
}

// openReply puts the box inside the thread, under the comment it answers.
func (m Model) openReply(t gh.ReviewThread, c gh.Comment, quote bool) (Model, tea.Cmd) {
	// One box takes the keys. The compose card keeps its words: it is furniture,
	// and esc leaves them there too.
	m.compose.stop()

	at := replyKey(t.ID)
	cmd := m.inline.open(at, m.convRing.on, "", replyWords)
	if quote {
		m.inline.quote(c.Body)
	}

	m.convRing.on = at
	m.conv.ok = false

	// Opening scrolls the same shortest way typing does, and for a stronger
	// reason. Moving the box to the top row takes the thread off the screen with
	// it: the code, the comments, and the one being answered all sit above the
	// box, so topping it leaves the reader writing a reply to something they can
	// no longer see. It goes far enough to land the box's foot, though, or the
	// reader is writing into something with no visible end.
	m.showOpenedBox()
	return m, cmd
}

// postReply hands the buffer to the root and closes the box.
func (m Model) postReply() (Model, tea.Cmd) {
	body := m.inline.body()
	if body == "" {
		return m, nil
	}

	id, thread, from := m.pr.ID, m.inline.at.id, m.inline.from

	// Emptied before the close, because closing files whatever is in the box as
	// the thread's draft. These words are on their way to GitHub, and a draft of
	// them would come back the next time the box opened here.
	m.inline.area.Reset()
	m.inline.close()

	m.convRing.on = from
	m.conv.ok = false
	m.syncContent()
	return m, func() tea.Msg { return PostReplyMsg{ID: id, ThreadID: thread, Body: body} }
}

// RestoreReply puts a reply that failed to post back where it was written. The
// words are the reader's, and a network that dropped them is not a reason to
// lose them.
//
// It only takes the keyboard back when nothing else has it. A reply answered for
// long after it left can arrive while the reader is writing somewhere else, and
// stealing the caret mid-sentence to report old news is worse than the toast
// that reports it. The words wait as the thread's draft either way.
func (m *Model) RestoreReply(threadID, body string) tea.Cmd {
	return m.restore(replyKey(threadID), body, replyWords,
		func() bool { return m.hasThread(threadID) },
		func() focusKey { return threadFocus(m.detail.Detail.Threads, threadID) })
}

// restore is the tail of both RestoreReply and RestoreEdit: file the words
// against what they were written for, and reopen the box on them where the
// reader is still standing somewhere it can appear.
//
// The box already open on this target takes the words straight back, caret and
// all: nobody has moved, and the failure is about what is under their hands.
//
// It opens on no fallback text, unlike a key press. The words came back from a
// write that failed and were just filed as the draft, so an edit reopens on
// what the reader typed rather than on the comment they typed it over.
func (m *Model) restore(at focusKey, body string, w words,
	present func() bool, from func() focusKey,
) tea.Cmd {
	m.conv.ok = false
	m.inline.keep(at, body)

	if m.inline.at == at {
		m.inline.area.SetValue(m.inline.drafts[at])
		m.inline.area.MoveToEnd()
		m.showInline()
		return nil
	}

	if m.writing() != nil || !m.canCompose() || !present() {
		m.syncContent()
		return nil
	}

	cmd := m.inline.open(at, from(), "", w)
	m.convRing.on = at
	m.focus = paneMain
	m.showOpenedBox()
	return cmd
}

func (m Model) hasThread(id string) bool {
	return slices.ContainsFunc(m.detail.Detail.Threads, func(t gh.ReviewThread) bool {
		return t.ID == id
	})
}

// threadFocus is where esc lands after a box reopened on its own: the thread it
// was written against, which is the block tab would have been on.
func threadFocus(threads []gh.ReviewThread, id string) focusKey {
	for _, t := range threads {
		if t.ID == id {
			return threadKey(t)
		}
	}
	return focusKey{}
}
