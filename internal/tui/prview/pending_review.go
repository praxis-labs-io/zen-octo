package prview

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/keys"
)

const pendingNote = "Only you can see this until you submit it."

// Holds its threads the way a submitted review does, so submitting moves nothing on the page.
func (m *Model) pendingReview(d gh.PullRequestDetail, shown map[int]bool, width int) rendered {
	key := focusKey{kind: focusPendingReview, id: d.DraftReview.ID}
	inner := m.cardWidth(width)

	head := m.said(m.who, "pending review", m.theme.Warning, gh.TimelineItem{})
	content := wrap(m.faint().Render(pendingNote), inner)
	if strings.TrimSpace(d.DraftReview.Body) != "" {
		content = m.markdown(d.DraftReview.Body, inner, key) + "\n\n" + content
	}

	block := m.card(head, content, width, m.lit(key), m.pendingHints(key, width))
	card := rendered{
		block: block,
		stops: []focusItem{{focusKey: key, lines: strings.Count(block, "\n") + 1}},
	}
	return m.withThreads(card, d.DraftReview.ID, d.Threads, shown, width)
}

func (m Model) pendingHints(key focusKey, width int) string {
	if !m.lit(key) {
		return ""
	}
	k := keys.Detail
	return hintLine(width, k.Activate.Help().Key+" submit", k.Delete.Help().Key+" discard")
}

func (m Model) onPendingReview() bool {
	on := m.mainRing().on
	return m.canCompose() && m.focus == paneMain && on.kind == focusPendingReview &&
		on.id == m.detail.Detail.DraftReview.ID
}

func (m Model) pendingBadge() string {
	d := m.detail.Detail
	if !m.detail.Loaded || d.DraftReview.ID == "" {
		return ""
	}
	text := "pending review"
	if n := draftComments(d); n > 0 {
		text = comp.Plural(n, "pending comment")
	}
	return lipgloss.NewStyle().Foreground(m.theme.Warning).Render(text)
}

const confirmDiscard = "discard"

// Opens on the row that keeps the review, so enter with no movement discards nothing.
func (m Model) startDiscard() (Model, tea.Cmd) {
	question := "Discard your review?"
	if n := draftComments(m.detail.Detail); n > 0 {
		question = "Discard your review and its " + comp.Plural(n, "comment") + "?"
	}

	m.picking = picking{
		field: pickDiscard,
		on:    target{at: m.mainRing().on},
		p: comp.NewPicker(
			question,
			[]comp.PickerItem{
				{ID: confirmCancel, Name: "Keep it", Color: m.theme.Text},
				{ID: confirmDiscard, Name: "Discard", Color: m.theme.Error},
			},
			nil,
			false,
		),
	}
	return m, nil
}

func (m Model) applyDiscard(p picking) (Model, tea.Cmd) {
	chosen := p.p.Chosen()
	if len(chosen) != 1 || chosen[0] != confirmDiscard {
		return m, nil
	}

	msg := DiscardReviewMsg{ID: m.pr.ID, ReviewID: p.on.at.id}

	m.pageRing.clear()
	m.page.ok = false
	m.syncContent()
	return m, func() tea.Msg { return msg }
}
