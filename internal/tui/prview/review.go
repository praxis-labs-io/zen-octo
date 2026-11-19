package prview

import (
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	area "charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/keys"
	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

// SubmitReviewMsg asks the root to submit a review. ReviewID is the pending review to submit, or empty to
// open a review and submit it in the same call.
type SubmitReviewMsg struct {
	ID       string
	ReviewID string
	Event    gh.ReviewEvent
	Body     string
}

// ReviewWaitMsg reports a submit or discard held back while a comment bound for the review is on its way.
type ReviewWaitMsg struct{}

type DiscardReviewMsg struct {
	ID       string
	ReviewID string
}

const (
	reviewSummaryRows  = 6
	reviewSummaryFloor = 1

	reviewButton = "Submit"
)

const (
	reviewHintVerdict = "tab next · j/k verdict · esc cancel"
	reviewHintButton  = "tab next · ⏎ submit · esc cancel"
	reviewHintComment = "a comment needs a summary · esc cancel"
	reviewHintChanges = "changes need a summary · esc cancel"
)

var reviewHintWriting = keys.Detail.Post.Help().Key + " submit · " +
	keys.Detail.Editor.Help().Key + " editor · esc cancel"

var reviewHintWidth = func() int {
	widest := 0
	for _, hint := range []string{
		reviewHintVerdict, reviewHintButton, reviewHintComment,
		reviewHintChanges, reviewHintWriting,
	} {
		widest = max(widest, lipgloss.Width(hint))
	}
	return widest
}()

var reviewOrder = []gh.ReviewEvent{gh.ReviewEventComment, gh.ReviewEventApprove, gh.ReviewEventRequestChanges}

func verdictName(e gh.ReviewEvent) string {
	switch e {
	case gh.ReviewEventApprove:
		return "Approve"
	case gh.ReviewEventRequestChanges:
		return "Request changes"
	}
	return "Comment"
}

type reviewRowKind int

const (
	reviewVerdictRow reviewRowKind = iota
	reviewSummaryRow
	reviewButtonRow
)

// Snapshotted at open so a refetch behind the modal cannot change which review is submitted.
type reviewing struct {
	open bool

	verdicts []gh.ReviewEvent
	at       int

	summary area.Model

	drafts int

	id, reviewID string
	number       int

	row int
}

type reviewDraft struct {
	event gh.ReviewEvent
	body  string
	held  bool
}

func (f reviewing) event() gh.ReviewEvent {
	if f.at < 0 || f.at >= len(f.verdicts) {
		return gh.ReviewEventComment
	}
	return f.verdicts[f.at]
}

func (f reviewing) rows() []reviewRowKind {
	if len(f.verdicts) > 1 {
		return []reviewRowKind{reviewVerdictRow, reviewSummaryRow, reviewButtonRow}
	}
	return []reviewRowKind{reviewSummaryRow, reviewButtonRow}
}

func (f reviewing) on() reviewRowKind {
	rows := f.rows()
	if f.row < 0 || f.row >= len(rows) {
		return reviewButtonRow
	}
	return rows[f.row]
}

func (f reviewing) body() string { return strings.TrimSpace(f.summary.Value()) }

// Comment needs something to publish; GitHub refuses one with neither a summary nor a comment in it.
func (f reviewing) ready() bool {
	if f.body() != "" {
		return true
	}
	switch f.event() {
	case gh.ReviewEventApprove:
		return true
	case gh.ReviewEventComment:
		return f.drafts > 0
	}
	return false
}

// An author can only comment on their own pull request. A settled one takes a comment unless a review was
// already pending on it, which is left for GitHub to accept or refuse.
func (m Model) reviewVerdicts(d gh.PullRequestDetail) []gh.ReviewEvent {
	own := m.who.Login != "" && m.who.Login == d.Author.Login
	settled := d.State != gh.PRStateOpen && d.DraftReview.ID == ""
	if own || settled {
		return []gh.ReviewEvent{gh.ReviewEventComment}
	}
	return reviewOrder
}

func (m Model) canReview() bool { return m.detail.Loaded }

func (m *Model) startReview() tea.Cmd {
	if !m.canReview() {
		return nil
	}
	d := m.detail.Detail
	if draftsLanding(d) {
		return func() tea.Msg { return ReviewWaitMsg{} }
	}

	f := reviewing{
		open:     true,
		verdicts: m.reviewVerdicts(d),
		summary:  textarea(m.theme, reviewSummaryRows),
		drafts:   draftComments(d),
		id:       m.pr.ID,
		reviewID: d.DraftReview.ID,
		number:   m.pr.Number,
	}
	f.summary.Placeholder = "Leave a summary"

	body := d.DraftReview.Body
	if kept := m.reviewKept; kept.held {
		body = kept.body
		f.at = max(0, slices.Index(f.verdicts, kept.event))
	}
	f.summary.SetValue(body)

	m.clearMention()
	m.reviewing = f
	m.reviewing.resize(m.width, m.height)
	return m.reviewing.focus()
}

func (f *reviewing) focus() tea.Cmd {
	if f.on() == reviewSummaryRow {
		f.summary.MoveToEnd()
		return f.summary.Focus()
	}
	f.summary.Blur()
	return nil
}

func (f *reviewing) step(delta int) tea.Cmd {
	n := len(f.rows())
	f.row = (f.row + delta + n) % n
	return f.focus()
}

func (f *reviewing) choose(delta int) {
	f.at = min(max(f.at+delta, 0), len(f.verdicts)-1)
}

func (f *reviewing) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	f.summary, cmd = f.summary.Update(msg)
	return cmd
}

// Order is load-bearing: leaving, submitting and stepping first, then text in the summary, then form keys.
func (m Model) reviewKey(keyMsg tea.KeyPressMsg) (Model, tea.Cmd) {
	k := keys.Detail

	switch {
	case key.Matches(keyMsg, k.Back):
		f := m.reviewing
		m.reviewKept = reviewDraft{event: f.event(), body: f.summary.Value(), held: true}
		m.reviewing = reviewing{}
		return m, nil

	case key.Matches(keyMsg, k.Post):
		return m.applyReview()

	case key.Matches(keyMsg, keys.Form.Next):
		return m, m.reviewing.step(1)
	case key.Matches(keyMsg, keys.Form.Prev):
		return m, m.reviewing.step(-1)
	}

	if m.reviewing.on() == reviewSummaryRow {
		if key.Matches(keyMsg, k.Editor) {
			return m, editorCmd(m.reviewing.summary.Value())
		}
		var cmd tea.Cmd
		m.reviewing.summary, cmd = m.reviewing.summary.Update(keyMsg)
		return m, cmd
	}

	switch {
	case key.Matches(keyMsg, k.Activate):
		if m.reviewing.on() == reviewButtonRow {
			return m.applyReview()
		}
		return m, m.reviewing.step(1)

	case key.Matches(keyMsg, k.Up) && m.reviewing.on() == reviewVerdictRow:
		m.reviewing.choose(-1)
	case key.Matches(keyMsg, k.Down) && m.reviewing.on() == reviewVerdictRow:
		m.reviewing.choose(1)
	}
	return m, nil
}

func (m Model) applyReview() (Model, tea.Cmd) {
	f := m.reviewing
	if !f.ready() {
		return m, nil
	}
	m.reviewing = reviewing{}
	m.reviewKept = reviewDraft{}

	msg := SubmitReviewMsg{ID: f.id, ReviewID: f.reviewID, Event: f.event(), Body: f.body()}
	return m, func() tea.Msg { return msg }
}

// RestoreReview holds a refused review's verdict and summary for the next time the form opens.
func (m *Model) RestoreReview(event gh.ReviewEvent, body string) {
	m.reviewKept = reviewDraft{event: event, body: body, held: true}
}

func (m Model) reviewOverlay(frame string) string {
	if !m.reviewing.open {
		return frame
	}
	return comp.Over(frame, m.reviewing.render(m.theme, m.width, m.height), m.width, m.height)
}

// Never called from render, which runs on a value copy and would drop the size.
func (f *reviewing) resize(frameWidth, frameHeight int) {
	if !f.open {
		return
	}
	f.summary.SetWidth(fieldText(f.width(frameWidth)))
	f.summary.SetHeight(f.summaryHeight(frameHeight))
}

func (f reviewing) title() string { return "Review #" + strconv.Itoa(f.number) }

func (f reviewing) render(th theme.Theme, frameWidth, frameHeight int) string {
	rows, _ := f.formRows(th, f.width(frameWidth))
	return comp.Modal(th, f.title(), strings.Join(rows, "\n"))
}

func (f reviewing) formRows(th theme.Theme, width int) (rows []string, fieldTop int) {
	rows = []string{""}

	if len(f.verdicts) > 1 {
		names := make([]string, len(f.verdicts))
		for i, v := range f.verdicts {
			names[i] = verdictName(v)
		}
		rows = append(rows, choiceRows(th, "Verdict", names, f.at, f.on() == reviewVerdictRow, width)...)
		rows = append(rows, "")
	}

	fieldTop = len(rows)
	rows = append(rows, formField(th, "Summary", f.summary.View(), f.summary.Height(), width,
		f.on() == reviewSummaryRow)...)

	if f.drafts > 0 {
		note := "Publishes " + comp.Plural(f.drafts, "pending comment")
		rows = append(rows, "", formNote(th, note, lipgloss.NewStyle().Foreground(th.Subtle), width))
	}

	rows = append(rows, "", formFooter(th, reviewButton, f.footerText(), width, f.ready(),
		f.on() == reviewButtonRow))
	return rows, fieldTop
}

func (f reviewing) footerText() string {
	if !f.ready() {
		if f.event() == gh.ReviewEventRequestChanges {
			return reviewHintChanges
		}
		return reviewHintComment
	}

	switch f.on() {
	case reviewSummaryRow:
		return reviewHintWriting
	case reviewVerdictRow:
		return reviewHintVerdict
	}
	return reviewHintButton
}

func (f reviewing) cursor(th theme.Theme, frameWidth, frameHeight int) *tea.Cursor {
	if !f.open || f.on() != reviewSummaryRow {
		return nil
	}
	local := f.summary.Cursor()
	if local == nil {
		return nil
	}

	rows, fieldTop := f.formRows(th, f.width(frameWidth))
	x, y := comp.OverOrigin(comp.Modal(th, f.title(), strings.Join(rows, "\n")), frameWidth, frameHeight)

	return comp.Cursor(th,
		x+comp.ModalLead+formFieldLead+local.X,
		y+1+fieldTop+1+local.Y)
}

func (f reviewing) width(frameWidth int) int {
	longest := reviewHintWidth + 1 + lipgloss.Width(reviewButton) + 2*formButtonPad
	for _, v := range f.verdicts {
		longest = max(longest, lipgloss.Width(formMark+verdictName(v)))
	}
	return formWidth(longest, frameWidth)
}

func (f reviewing) summaryHeight(frameHeight int) int {
	if frameHeight <= 0 {
		return reviewSummaryRows
	}
	return min(reviewSummaryRows, max(reviewSummaryFloor, frameHeight-f.chromeRows()))
}

// Counted rather than measured: the height is decided before the box it sizes is rendered.
func (f reviewing) chromeRows() int {
	chrome := 2 + 1 + 2 + 2
	if len(f.verdicts) > 1 {
		chrome += 1 + len(f.verdicts) + 1
	}
	if f.drafts > 0 {
		chrome += 2
	}
	return chrome
}

// Threads stop at a page, and the review's own count lags a comment still landing, so the larger is right.
func draftComments(d gh.PullRequestDetail) int {
	n := 0
	for _, t := range d.Threads {
		for _, c := range t.Comments {
			if c.Draft {
				n++
			}
		}
	}
	return max(n, d.DraftReview.Comments)
}

// Sent ahead of a comment bound for it, a review would publish without the comment or open a second one.
func draftsLanding(d gh.PullRequestDetail) bool {
	for _, t := range d.Threads {
		for _, c := range t.Comments {
			if c.Draft && c.Pending {
				return true
			}
		}
	}
	return false
}
