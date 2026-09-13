package prview

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/keys"
	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

// A second composer rather than the compose card retargeted, so opening it keeps a half-written comment.
type inline struct {
	composer

	at focusKey

	// Where focus returns on close, never the posted reply: its locally minted id changes when GitHub answers.
	from focusKey

	drafts map[focusKey]string
}

const replyRows = 4

func newInline(th theme.Theme) inline {
	return inline{composer: newComposer(th)}
}

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

// Keeps a reply's words as a draft but drops an edit's, which would otherwise reopen over a comment changed since.
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

func (r inline) editing() bool { return r.at.kind != focusNone && r.at.kind != focusReply }

func (m Model) boxOn(key focusKey) bool {
	return m.inline.typing && m.inline.at == key
}

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

func (m Model) inlineKey(keyMsg tea.KeyPressMsg) (Model, tea.Cmd) {
	m, cmd, took := m.mentionKey(keyMsg)
	if took {
		return m, cmd
	}
	k := keys.Detail

	switch {
	case key.Matches(keyMsg, k.Back):
		return m.closeInline()

	case key.Matches(keyMsg, k.Editor):
		m.clearMention()
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

	m.inline.area, cmd = m.inline.area.Update(keyMsg)
	ask := m.syncMention()
	m.showInline()
	return m, tea.Batch(cmd, ask)
}

func (m Model) sendInline() (Model, tea.Cmd) {
	m.clearMention()
	if m.inline.editing() {
		return m.saveEdit()
	}
	return m.postReply()
}

func (m Model) closeInline() (Model, tea.Cmd) {
	from := m.inline.from
	m.inline.close()
	m.clearMention()

	m.pageRing.on = from
	m.conv.ok = false
	m.syncContent()
	return m, nil
}

func (m *Model) showInline() {
	m.syncContent()
	m.showCaret()
}

// Lands the box's foot rather than following the caret, which opens on the box's first row.
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

	foot := m.boxLine + box.area.Height() + 1
	switch {
	case foot >= top+height:
		m.view.SetYOffset(contentLead + min(foot-height+1, m.boxLine))
	case m.boxLine < top:
		m.view.SetYOffset(contentLead + m.boxLine)
	}
}

func (m *Model) showCaret() {
	box := m.writing()
	if box == nil || m.boxLine <= 0 {
		return
	}

	row := min(max(box.caretRow(box.area.Width()), 0), max(0, box.area.Height()-1))

	caret := m.boxLine + row
	top, height := bodyTop(&m.view), m.view.Height()
	if height <= 0 {
		return
	}

	switch {
	case caret < top:
		m.view.SetYOffset(contentLead + caret)
	case caret+1 >= top+height:
		m.view.SetYOffset(contentLead + caret + 2 - height)
	}
}

// Sized at render rather than at open, because the wrapped height depends on a width known only here.
func (m *Model) inlineBox(width, floor, chrome int) string {
	m.inline.setWidth(width)
	m.inline.area.SetHeight(m.boxRows(m.inline.composer, floor, width, chrome))
	return m.inline.area.View() + "\n" + m.inline.button(m.theme, width, true)
}
