package prview

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

// Quotes the raw body rather than rendered markdown, so a fenced block survives.
func (c *composer) quote(body string) {
	q := quoted(body)
	if q == "" {
		return
	}
	c.area.SetValue(joinDraft(c.body(), q) + "\n\n")
	c.area.MoveToEnd()
}

// An empty line takes a bare '>', since GitHub would store the trailing space of "> ".
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

func replyKey(threadID string) focusKey {
	return focusKey{kind: focusReply, id: threadID}
}

func (m *Model) writingCard(width int) string {
	key := replyKey(m.inline.at.id)

	inner := m.cardWidth(width)
	head := m.said(m.who, "write a reply", m.theme.Subtle, gh.TimelineItem{})
	return m.card(head, m.inlineBox(inner, replyRows, boxChrome), width, m.lit(key), "")
}

func (m Model) within(t gh.ReviewThread) string {
	on := m.mainRing().on
	if on.kind == focusThreadComment && slices.ContainsFunc(t.Comments,
		func(c gh.Comment) bool { return c.ID == on.id }) {
		return on.id
	}
	if len(t.Comments) == 0 {
		return ""
	}
	return t.Comments[0].ID
}

func (m Model) threadOpen(t gh.ReviewThread) bool {
	return !t.IsResolved || m.open[threadKey(t)]
}

func (m Model) focusedThread() (gh.ReviewThread, bool) {
	t, ok := m.threadHolding()
	return t, ok && m.threadOpen(t)
}

func (m Model) threadOnRing() (gh.ReviewThread, bool) {
	on := m.mainRing().on
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

func (m Model) threadHolding() (gh.ReviewThread, bool) {
	if t, ok := m.threadOnRing(); ok {
		return t, ok
	}
	if on := m.mainRing().on; m.answerable() && on.kind == focusThreadComment {
		for _, t := range m.detail.Detail.Threads {
			if slices.ContainsFunc(t.Comments, func(c gh.Comment) bool { return c.ID == on.id }) {
				return t, true
			}
		}
	}
	return gh.ReviewThread{}, false
}

func (m Model) answerable() bool {
	return m.canAct() && m.focus == paneMain &&
		m.mainRing().live(bodyTop(&m.view), m.view.Height())
}

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

// GitHub does not thread top-level comments, so answering one is a new comment at the foot of the page.
func (m Model) replyBody() (string, bool) {
	on := m.mainRing().on

	if !m.answerable() || !m.canCompose() {
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

func (m Model) startReply(quote bool) (Model, tea.Cmd) {
	if t, c, ok := m.replyThread(); ok {
		return m.openReply(t, c, quote)
	}

	if body, ok := m.replyBody(); ok && quote {
		return m.openCompose(body)
	}
	return m, nil
}

func (m Model) openReply(t gh.ReviewThread, c gh.Comment, quote bool) (Model, tea.Cmd) {
	m.compose.stop()
	m.clearMention()

	at := replyKey(t.ID)
	cmd := m.inline.open(at, m.pageRing.on, "", replyWords)
	if quote {
		m.inline.quote(c.Body)
	}

	m.pageRing.on = at
	m.conv.ok = false

	m.showOpenedBox()
	return m, cmd
}

func (m Model) postReply() (Model, tea.Cmd) {
	body := m.inline.body()
	if body == "" {
		return m, nil
	}

	id, thread, from := m.pr.ID, m.inline.at.id, m.inline.from

	m.inline.area.Reset()
	m.inline.close()

	m.pageRing.on = from
	m.conv.ok = false
	m.syncContent()
	return m, func() tea.Msg { return PostReplyMsg{ID: id, ThreadID: thread, Body: body} }
}

// RestoreReply files a failed reply as its thread's draft and reopens the box only if nothing else has the keyboard.
func (m *Model) RestoreReply(threadID, body string) tea.Cmd {
	return m.restore(replyKey(threadID), body, replyWords,
		func() bool { return m.hasThread(threadID) },
		func() focusKey { return threadFocus(m.detail.Detail.Threads, threadID) })
}

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

	if m.writing() != nil || !m.canAct() || !present() {
		m.syncContent()
		return nil
	}

	cmd := m.inline.open(at, from(), "", w)
	m.pageRing.on = at
	m.focus = paneMain
	m.showOpenedBox()
	return cmd
}

func (m Model) hasThread(id string) bool {
	return slices.ContainsFunc(m.detail.Detail.Threads, func(t gh.ReviewThread) bool {
		return t.ID == id
	})
}

func threadFocus(threads []gh.ReviewThread, id string) focusKey {
	for _, t := range threads {
		if t.ID == id {
			return threadKey(t)
		}
	}
	return focusKey{}
}
