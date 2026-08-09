package prview

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/keys"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// replier is the box r opens inside a review thread, under the comment being
// answered. It is a second composer rather than the compose card retargeted:
// one textarea drawn at two places is one buffer, and opening a reply would
// then wipe out a top-level comment half written at the foot of the page.
//
// Unlike the compose card it is summoned. It exists only while it is open, so
// the box is on the page exactly when it has the keyboard.
type replier struct {
	composer

	// thread is the review thread the box is open on, empty when it is closed.
	thread string

	// from is the comment the box was opened from, and where focus goes when it
	// closes. Not the reply that was written: that card is a placeholder keyed by
	// an id minted locally, and the highlight would vanish on its own the moment
	// GitHub answered with the real one.
	from focusKey

	// drafts is what was written into a box and not posted, by thread. Closing
	// the box takes it off the page, and a reader who pressed esc to look at the
	// code above their answer has not thrown the answer away.
	drafts map[string]string
}

// replyRows is the writing the box shows at once. Half the compose card's,
// because a reply is an answer rather than an opening argument, and the box sits
// inside a card that is already carrying the discussion above it. $EDITOR is
// there for the ones that need more.
const replyRows = 4

func newReplier(th theme.Theme) replier {
	c := newComposer(th)
	c.area.Placeholder = "Leave a reply"
	c.area.SetHeight(replyRows)
	return replier{composer: c}
}

// open points the box at a thread, filling it with whatever was left there.
func (r *replier) open(threadID string, from focusKey) tea.Cmd {
	r.thread, r.from = threadID, from
	r.area.SetValue(r.drafts[threadID])
	r.area.MoveToEnd()
	return r.start()
}

// close takes the box off the page, keeping the words against the thread they
// were written for.
func (r *replier) close() {
	r.keep(r.thread, "")
	r.thread = ""
	r.area.Reset()
	r.stop()
}

// keep holds words for a thread, behind whatever is already held for it. Two
// answers to one thread run together is a mess the reader can cut apart, and it
// is the smaller loss: everything else on this screen can be fetched again.
func (r *replier) keep(threadID, body string) {
	if threadID == "" {
		return
	}

	if threadID == r.thread {
		body = joinDraft(body, r.body())
	} else {
		body = joinDraft(r.drafts[threadID], body)
	}

	if body == "" {
		delete(r.drafts, threadID)
		return
	}
	if r.drafts == nil {
		r.drafts = make(map[string]string)
	}
	r.drafts[threadID] = body
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

// quote puts a comment in the buffer with the cursor under it, the way the
// browser's quote reply does. The raw body goes in, not the rendered markdown: a
// quote of a fenced block has to come out the other side as a fenced block.
func (c *composer) quote(body string) {
	c.area.SetValue(joinDraft(c.body(), quoted(body)) + "\n\n")
	c.area.MoveToEnd()
}

// quoted is a body as a blockquote. An empty line takes a bare marker: the
// trailing space a uniform "> " would leave is invisible here and real in the
// comment GitHub stores.
func quoted(body string) string {
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	for i, line := range lines {
		if line == "" {
			lines[i] = ">"
			continue
		}
		lines[i] = "> " + line
	}
	return strings.Join(lines, "\n")
}

// replyFocus is what the open box answers to in the ring. Tab never walks onto
// it: the box has the keyboard from the moment it appears, and tab is the post
// button then. It is in the ring so the page can be scrolled to keep it in view.
func (m Model) replyFocus() focusKey {
	return focusKey{kind: focusReply, id: m.reply.thread}
}

// threadWithReply is a thread and, when a box is open on it, the box under it.
//
// The box is a card of its own rather than a block inside the thread's. A
// thread card holds what people said; the box is where you say something, which
// is the compose card's job at the foot of the page and the same shape here. It
// keeps the two apart on the page and out of each other's way in the code: no
// gutter to dodge, no rule to draw, and the accent lands on whichever of the two
// the keys are going to.
func (m *Model) threadWithReply(t gh.ReviewThread, width int) rendered {
	v := m.thread(t, width, true)
	if m.reply.thread != t.ID {
		return v
	}

	// Stacked against the thread rather than spaced off it. Every other pair of
	// blocks on the page has a blank line between them because they are separate
	// things; these two are one thing being answered and the answer, and the gap
	// read as the box belonging to the page rather than to the thread above it.
	box := m.replyCard(width)
	at := strings.Count(v.block, "\n") + 1
	block := v.block + "\n" + box

	return rendered{block: block, stops: tile(block, []focusItem{
		{focusKey: threadKey(t)},
		{focusKey: m.replyFocus(), start: at},
	})}
}

// replyCard is the box, rendered through the same card every comment renders
// through, so an answer being written sits among the answers already made.
func (m *Model) replyCard(width int) string {
	key := m.replyFocus()

	inner := m.cardWidth(width)
	m.reply.setWidth(inner)

	head := m.said(m.who, "write a reply", m.theme.Faint, gh.TimelineItem{})
	body := m.reply.area.View() + "\n\n" + m.reply.button(m.theme, inner, m.lit(key))
	return m.card(head, body, width, m.lit(key), "")
}

// within is the comment a quote reply would take from a thread, and the one the
// gutter bar points at.
//
// Tab walks whole threads, so landing on one says nothing about which of its
// comments the reader means. The last is the answer until they say otherwise:
// it is the one at the bottom of the card, the newest, and the one an answer
// follows on from. StepWithin moves it, and it is remembered per thread rather
// than reset by every tab past.
func (m Model) within(t gh.ReviewThread) string {
	if len(t.Comments) == 0 {
		return ""
	}
	if held, ok := m.sub[t.ID]; ok && slices.ContainsFunc(t.Comments,
		func(c gh.Comment) bool { return c.ID == held }) {
		return held
	}
	return t.Comments[len(t.Comments)-1].ID
}

// stepWithin moves the sub-cursor through the focused thread's comments. It
// clamps rather than wrapping: a run off either end of a thread is a reader
// asking for the thread above or below, and tab is the key for that.
func (m Model) stepWithin(delta int) (Model, tea.Cmd) {
	t, ok := m.focusedThread()
	if !ok || len(t.Comments) < 2 {
		return m, nil
	}

	at := slices.IndexFunc(t.Comments, func(c gh.Comment) bool { return c.ID == m.within(t) })
	next := min(max(at+delta, 0), len(t.Comments)-1)
	if next == at {
		return m, nil
	}

	if m.sub == nil {
		m.sub = make(map[string]string)
	}
	m.sub[t.ID] = t.Comments[next].ID

	m.conv.ok = false
	m.syncContent()
	return m, nil
}

// threadOpen is whether a thread is showing its comments. Everything unresolved
// is; a resolved one is closed until o opens it, and while it is closed there is
// nothing inside it to step through, quote, or hang a box off.
func (m Model) threadOpen(t gh.ReviewThread) bool {
	return !t.IsResolved || m.expanded[threadKey(t)]
}

// focusedThread is the open thread the ring is on, on the screen.
func (m Model) focusedThread() (gh.ReviewThread, bool) {
	t, ok := m.threadOnRing()
	return t, ok && m.threadOpen(t)
}

// threadOnRing is the thread the ring is on whether it is open or not, which is
// what o needs: closed is the state it exists to change.
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

	from := m.convRing.on
	cmd := m.reply.open(t.ID, from)
	if quote {
		m.reply.quote(c.Body)
	}

	m.convRing.on = m.replyFocus()
	m.conv.ok = false

	// Opening scrolls the same shortest way typing does, and for a stronger
	// reason. Moving the box to the top row takes the thread off the screen with
	// it: the code, the comments, and the one being answered all sit above the
	// box, so topping it leaves the reader writing a reply to something they can
	// no longer see.
	m.showReply()
	return m, cmd
}

// closeReply hands the keyboard back and takes the box off the page. Focus goes
// back to the comment it was opened from, so esc leaves the reader where they
// were rather than nowhere.
func (m Model) closeReply() (Model, tea.Cmd) {
	from := m.reply.from
	m.reply.close()

	m.convRing.on = from
	m.conv.ok = false
	m.syncContent()
	return m, nil
}

// replyKey is every key while the box has the keyboard. It answers the handful
// that belong to it and hands the rest to the text.
func (m Model) replyKey(keyMsg tea.KeyPressMsg) (Model, tea.Cmd) {
	k := keys.Detail

	switch {
	case key.Matches(keyMsg, k.Back):
		return m.closeReply()

	case key.Matches(keyMsg, k.Editor):
		return m, m.reply.editorCmd()

	case key.Matches(keyMsg, k.FocusNext), key.Matches(keyMsg, k.FocusPrev):
		cmd := m.reply.step()
		m.syncContent()
		return m, cmd

	case key.Matches(keyMsg, k.Post):
		return m.postReply()

	case key.Matches(keyMsg, k.Activate) && m.reply.onPost:
		return m.postReply()
	}

	var cmd tea.Cmd
	m.reply.area, cmd = m.reply.area.Update(keyMsg)
	m.showReply()
	return m, cmd
}

// postReply hands the buffer to the root and closes the box.
func (m Model) postReply() (Model, tea.Cmd) {
	body := m.reply.body()
	if body == "" {
		return m, nil
	}

	id, thread, from := m.pr.ID, m.reply.thread, m.reply.from

	// Emptied before the close, because closing files whatever is in the box as
	// the thread's draft. These words are on their way to GitHub, and a draft of
	// them would come back the next time the box opened here.
	m.reply.area.Reset()
	m.reply.close()

	m.convRing.on = from
	m.conv.ok = false
	m.syncContent()
	return m, func() tea.Msg { return PostReplyMsg{ID: id, ThreadID: thread, Body: body} }
}

// showReply rebuilds the page and keeps the box in view.
//
// It moves the shortest distance, which is the wrong move everywhere else on
// this screen and the right one here. A key that lands on a block is taking the
// reader somewhere, so the block goes to the top row with its own content under
// it. A keystroke inside a box is not taking them anywhere: the eye is on a
// caret that has not moved, and hauling the page to put the box on the top row
// because a character was typed is the worse of the two wrongs.
//
// The caret needs no arithmetic of its own. The box is a fixed height and the
// textarea scrolls inside it, so a box in view is a caret in view.
func (m *Model) showReply() {
	m.syncContent()

	at := m.convRing.index()
	if at < 0 {
		return
	}
	it := m.convRing.items[at]

	top, height := bodyTop(&m.view), m.view.Height()
	switch {
	case it.start < top:
		m.view.SetYOffset(contentLead + it.start)
	case it.start+it.lines > top+height:
		m.view.SetYOffset(contentLead + it.start + it.lines - height)
	}
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
	m.conv.ok = false
	m.reply.keep(threadID, body)

	if m.reply.thread == threadID {
		m.reply.area.SetValue(m.reply.drafts[threadID])
		m.reply.area.MoveToEnd()
		m.showReply()
		return nil
	}

	if m.writing() != nil || !m.canCompose() || !m.hasThread(threadID) {
		m.syncContent()
		return nil
	}

	cmd := m.reply.open(threadID, threadFocus(m.detail.Detail.Threads, threadID))
	m.convRing.on = m.replyFocus()
	m.focus = paneMain
	m.syncContent()

	top := bodyTop(&m.view)
	m.view.SetYOffset(contentLead + m.convRing.show(top, m.view.Height()))
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
