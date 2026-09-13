package prview

import (
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

// MergeMsg asks the root to merge the pull request with Options, then delete the
// head branch when RefID, its node id, is set.
type MergeMsg struct {
	ID      string
	Options gh.MergeOptions
	RefID   string
}

const (
	mergeMinWidth = 52
	// Matches the picker's, so the two modals open at the same width.
	mergeMaxWidth = 72

	mergeBodyRows = 6

	// Two, because a squash headline is the title with "(#N)" appended.
	mergeHeadlineRows = 2

	mergeBodyFloor = 1

	mergeButton    = "Merge"
	mergeButtonPad = 2
	mergeMark      = "✓ "
	mergeGap       = "  "
)

const (
	mergeHintMethod  = "tab next · j/k method · esc cancel"
	mergeHintDelete  = "tab next · space toggle · esc cancel"
	mergeHintButton  = "tab next · ⏎ merge · esc cancel"
	mergeHintNoChord = "tab to the button to merge · esc cancel"
)

var mergeHintChord = "tab next · " + keys.Detail.Post.Help().Key + " merge · esc cancel"

// The widest hint, so the modal keeps one width as focus moves between rows.
var mergeHintWidth = func() int {
	widest := 0
	for _, hint := range []string{mergeHintMethod, mergeHintDelete, mergeHintButton, mergeHintNoChord, mergeHintChord} {
		widest = max(widest, lipgloss.Width(hint))
	}
	return widest
}()

type mergeRowKind int

const (
	mergeMethodRow mergeRowKind = iota
	mergeHeadlineRow
	mergeBodyRow
	mergeDeleteRow
	mergeButtonRow
)

// Snapshotted at open so a refetch behind the modal cannot change which commit is merged.
type merging struct {
	open bool

	methods []gh.MergeMethod
	at      int

	headline area.Model
	body     area.Model

	typedHeadline, typedBody bool

	mergeText, squashText gh.MergeMessage

	del    bool
	branch string

	bypass bool

	chords bool

	id, oid, refID, base string
	number               int

	row int
}

var mergeOrder = []gh.MergeMethod{gh.MergeMethodMerge, gh.MergeMethodSquash, gh.MergeMethodRebase}

func mergeName(m gh.MergeMethod) string {
	switch m {
	case gh.MergeMethodMerge:
		return "Create a merge commit"
	case gh.MergeMethodSquash:
		return "Squash and merge"
	case gh.MergeMethodRebase:
		return "Rebase and merge"
	}
	return string(m)
}

func (f merging) method() gh.MergeMethod {
	if f.at < 0 || f.at >= len(f.methods) {
		return ""
	}
	return f.methods[f.at]
}

func (f merging) writes() bool { return f.method() != gh.MergeMethodRebase }

func (f merging) rows() []mergeRowKind {
	out := []mergeRowKind{mergeMethodRow}
	if f.writes() {
		out = append(out, mergeHeadlineRow, mergeBodyRow)
	}
	if f.branch != "" {
		out = append(out, mergeDeleteRow)
	}
	return append(out, mergeButtonRow)
}

func (f merging) on() mergeRowKind {
	rows := f.rows()
	if f.row < 0 || f.row >= len(rows) {
		return mergeButtonRow
	}
	return rows[f.row]
}

func (f merging) typing() bool {
	return f.on() == mergeHeadlineRow || f.on() == mergeBodyRow
}

func (f merging) text() gh.MergeMessage {
	switch f.method() {
	case gh.MergeMethodMerge:
		return f.mergeText
	case gh.MergeMethodSquash:
		return f.squashText
	}
	return gh.MergeMessage{}
}

func (f merging) ready() bool {
	return !f.writes() || strings.TrimSpace(f.headline.Value()) != ""
}

func (m *Model) startMerge() tea.Cmd {
	d := m.railDetail()

	if !mergeable(d) {
		return nil
	}

	methods := make([]gh.MergeMethod, 0, len(mergeOrder))
	for _, method := range mergeOrder {
		if m.repo.Meta.Methods.Allows(method) {
			methods = append(methods, method)
		}
	}
	if len(methods) == 0 {
		return nil
	}

	f := merging{
		open:       true,
		methods:    methods,
		at:         mergeDefault(methods),
		mergeText:  d.MergeMessage(gh.MergeMethodMerge),
		squashText: d.MergeMessage(gh.MergeMethodSquash),
		branch:     mergeBranch(d, m.repo.Meta.Methods),
		bypass:     mergeBypass(d),
		id:         m.pr.ID,
		oid:        d.HeadRefOid,
		refID:      d.HeadRefID,
		base:       d.BaseRefName,
		number:     m.pr.Number,
		chords:     m.compose.chords,
		headline:   newMergeInput(m.theme),
		body:       newMergeBody(m.theme),
	}
	f.del = f.branch != ""
	f.prefill()

	m.merging = f
	m.merging.resize(m.width, m.height)
	return m.merging.focus()
}

// Squash is this client's preference, not GitHub's; the fallback defers to the repository.
func mergeDefault(methods []gh.MergeMethod) int {
	for i, method := range methods {
		if method == gh.MergeMethodSquash {
			return i
		}
	}
	return 0
}

// Not gated on viewerCanDeleteHeadRef, which is false on every open pull request whoever the viewer.
func mergeBranch(d gh.PullRequestDetail, methods gh.MergeMethods) string {
	if methods.DeleteOnMerge || d.CrossRepository || d.HeadRefID == "" {
		return ""
	}
	return d.HeadRefName
}

func mergeBypass(d gh.PullRequestDetail) bool {
	return d.Merge == gh.MergeBlocked || d.Merge == gh.MergeBehind
}

// A textarea, not a text input: textinput reports its caret unscrolled, so past the edge the cursor lands wrong.
func newMergeInput(th theme.Theme) area.Model {
	in := textarea(th, mergeHeadlineRows)
	in.ShowLineNumbers = false
	return in
}

func newMergeBody(th theme.Theme) area.Model {
	return textarea(th, mergeBodyRows)
}

func (f *merging) prefill() {
	text := f.text()
	if !f.typedHeadline {
		f.headline.SetValue(text.Headline)
		headStart(&f.headline)
	}
	if !f.typedBody {
		f.body.SetValue(text.Body)
		f.body.MoveToBegin()
	}
}

// End then start: the textarea recomputes its visible window only once the caret leaves it.
func headStart(in *area.Model) {
	in.CursorEnd()
	in.CursorStart()
}

func (f *merging) focus() tea.Cmd {
	switch f.on() {
	case mergeHeadlineRow:
		f.body.Blur()
		f.headline.CursorEnd()
		return f.headline.Focus()
	case mergeBodyRow:
		f.headline.Blur()
		f.body.MoveToEnd()
		return f.body.Focus()
	}
	f.headline.Blur()
	f.body.Blur()
	return nil
}

// Both boxes rather than the focused one: a blurred textarea already drops paste and blink.
func (f *merging) update(msg tea.Msg) tea.Cmd {
	var headline, body tea.Cmd
	f.headline, headline = f.headline.Update(msg)
	f.body, body = f.body.Update(msg)

	f.typedHeadline = f.typedHeadline || f.headline.Value() != f.text().Headline
	f.typedBody = f.typedBody || f.body.Value() != f.text().Body

	return tea.Batch(headline, body)
}

func (f *merging) step(delta int) tea.Cmd {
	n := len(f.rows())
	f.row = (f.row + delta + n) % n
	return f.focus()
}

func (f *merging) choose(delta int) tea.Cmd {
	f.at = min(max(f.at+delta, 0), len(f.methods)-1)
	f.prefill()

	f.row = min(f.row, len(f.rows())-1)
	return f.focus()
}

// Order is load-bearing: leaving, merging and stepping first, then text in a field, then form keys.
func (m Model) mergeKey(keyMsg tea.KeyPressMsg) (Model, tea.Cmd) {
	k := keys.Detail

	switch {
	case key.Matches(keyMsg, k.Back):
		m.merging = merging{}
		return m, nil

	case key.Matches(keyMsg, k.Post):
		return m.applyMerge()

	case key.Matches(keyMsg, keys.Form.Next):
		return m, m.merging.step(1)
	case key.Matches(keyMsg, keys.Form.Prev):
		return m, m.merging.step(-1)
	}

	if m.merging.typing() {
		var cmd tea.Cmd

		if m.merging.on() == mergeHeadlineRow {
			was := m.merging.headline.Value()
			m.merging.headline, cmd = m.merging.headline.Update(keyMsg)
			m.merging.typedHeadline = m.merging.typedHeadline || m.merging.headline.Value() != was
			return m, cmd
		}

		was := m.merging.body.Value()
		m.merging.body, cmd = m.merging.body.Update(keyMsg)
		m.merging.typedBody = m.merging.typedBody || m.merging.body.Value() != was
		return m, cmd
	}

	switch {
	case key.Matches(keyMsg, k.Activate):
		switch m.merging.on() {
		case mergeButtonRow:
			return m.applyMerge()
		case mergeDeleteRow:
			m.merging.del = !m.merging.del
		default:
			return m, m.merging.step(1)
		}

	case key.Matches(keyMsg, keys.Form.Toggle) && m.merging.on() == mergeDeleteRow:
		m.merging.del = !m.merging.del

	case key.Matches(keyMsg, k.Up) && m.merging.on() == mergeMethodRow:
		return m, m.merging.choose(-1)
	case key.Matches(keyMsg, k.Down) && m.merging.on() == mergeMethodRow:
		return m, m.merging.choose(1)
	}
	return m, nil
}

func (m Model) applyMerge() (Model, tea.Cmd) {
	f := m.merging
	if !f.ready() {
		return m, nil
	}
	m.merging = merging{}

	opts := gh.MergeOptions{Method: f.method(), ExpectedHeadOid: f.oid}
	if f.writes() {
		opts.Headline = strings.TrimSpace(f.headline.Value())
		opts.Body = strings.TrimSpace(f.body.Value())
	}

	var refID string
	if f.del {
		refID = f.refID
	}

	id := f.id
	return m, func() tea.Msg { return MergeMsg{ID: id, Options: opts, RefID: refID} }
}

func (m Model) mergeOverlay(frame string) string {
	if !m.merging.open {
		return frame
	}
	return comp.Over(frame, m.merging.render(m.theme, m.width, m.height), m.width, m.height)
}

// Never called from render, which runs on a value copy and would drop the sizes.
func (f *merging) resize(frameWidth, frameHeight int) {
	if !f.open {
		return
	}
	width := fieldText(f.width(frameWidth))
	f.headline.SetWidth(width)
	f.headline.SetHeight(f.headlineHeight(frameHeight))
	f.body.SetWidth(width)
	f.body.SetHeight(f.bodyHeight(frameHeight))
}

func (f merging) render(th theme.Theme, frameWidth, frameHeight int) string {
	rows, _ := f.formRows(th, f.width(frameWidth))
	return comp.Modal(th, f.title(), strings.Join(rows, "\n"))
}

func (f merging) title() string { return "Merge #" + strconv.Itoa(f.number) }

const mergeFieldLead = 2

func (f merging) formRows(th theme.Theme, width int) (rows []string, fieldTop int) {
	fieldTop = -1
	rows = append([]string{""}, f.methodRows(th, width)...)

	if f.writes() {
		rows = append(rows, "")
		if f.on() == mergeHeadlineRow {
			fieldTop = len(rows)
		}
		rows = append(rows, f.field(th, "Headline", f.headline.View(), f.headline.Height(), width, mergeHeadlineRow)...)
		rows = append(rows, "")
		if f.on() == mergeBodyRow {
			fieldTop = len(rows)
		}
		rows = append(rows, f.field(th, "Message", f.body.View(), f.body.Height(), width, mergeBodyRow)...)
	}

	if f.branch != "" {
		rows = append(rows, "", f.deleteRow(th, width))
	}
	if f.bypass {
		plain := lipgloss.NewStyle()
		warning := plain.Foreground(th.Warning).Render("Bypasses branch protection on " + f.base)
		rows = append(rows, "", mergePad(clipTo(warning, width, plain.Foreground(th.Subtle)), width, plain))
	}

	rows = append(rows, "", f.footer(th, width))
	return rows, fieldTop
}

func (f merging) cursor(th theme.Theme, frameWidth, frameHeight int) *tea.Cursor {
	if !f.open {
		return nil
	}

	var local *tea.Cursor
	switch f.on() {
	case mergeHeadlineRow:
		local = f.headline.Cursor()
	case mergeBodyRow:
		local = f.body.Cursor()
	}
	if local == nil {
		return nil
	}

	rows, fieldTop := f.formRows(th, f.width(frameWidth))
	if fieldTop < 0 {
		return nil
	}
	x, y := comp.OverOrigin(comp.Modal(th, f.title(), strings.Join(rows, "\n")), frameWidth, frameHeight)

	return comp.Cursor(th,
		x+comp.ModalLead+mergeFieldLead+local.X,
		y+1+fieldTop+1+local.Y)
}

func (f merging) width(frameWidth int) int {
	longest := mergeHintWidth + lipgloss.Width(mergeButton) + 2*mergeButtonPad
	for _, method := range f.methods {
		longest = max(longest, lipgloss.Width(mergeMark+mergeName(method)))
	}

	want := min(max(longest, mergeMinWidth), mergeMaxWidth)

	if room := frameWidth - 4; room > 0 {
		want = min(want, room)
	}
	return max(want, 1)
}

func (f merging) bodyHeight(frameHeight int) int {
	if frameHeight <= 0 {
		return mergeBodyRows
	}

	return min(mergeBodyRows, max(mergeBodyFloor,
		frameHeight-f.chromeRows(f.headlineHeight(frameHeight))))
}

// Counted rather than measured: the height is decided before the box it sizes is rendered.
func (f merging) chromeRows(headlineRows int) int {
	chrome := 2 + 1 + 1 + len(f.methods) + 1 + (headlineRows + 2) + 1 + 2 + 2
	if f.branch != "" {
		chrome += 2
	}
	if f.bypass {
		chrome += 2
	}
	return chrome
}

func (f merging) headlineHeight(frameHeight int) int {
	if frameHeight <= 0 {
		return mergeHeadlineRows
	}
	if frameHeight-f.chromeRows(1)-mergeBodyFloor >= 1 {
		return mergeHeadlineRows
	}
	return 1
}

func (f merging) methodRows(th theme.Theme, width int) []string {
	plain := lipgloss.NewStyle()

	heading := plain.Foreground(th.Text).Bold(true).Render("Method")

	out := make([]string, 0, len(f.methods)+1)
	out = append(out, mergePad(heading, width, plain))

	for i, method := range f.methods {
		chosen := i == f.at

		base := plain
		if chosen && f.on() == mergeMethodRow {
			base = base.Background(th.SelectedBackground)
		}

		mark, c := base.Foreground(th.Subtle).Render(mergeGap), th.Subtle
		if chosen {
			mark, c = base.Foreground(th.Success).Render(mergeMark), th.Text
		}

		out = append(out, mergePad(mark+base.Foreground(c).Render(mergeName(method)), width, base))
	}
	return out
}

func (f merging) field(th theme.Theme, title, content string, lines, width int, row mergeRowKind) []string {
	pane := comp.NewPane(th).Title(title).Focus(f.on() == row).Size(width, lines+2)
	padded := lipgloss.NewStyle().Padding(0, 1).Render(content)
	return strings.Split(pane.Render(padded), "\n")
}

func fieldText(width int) int { return max(1, width-4) }

func (f merging) deleteText(room int) string {
	text := "Delete " + f.branch + " after merging"
	if lipgloss.Width(text) <= room {
		return text
	}

	tail := " after merging"
	branch := max(1, room-lipgloss.Width("Delete ")-lipgloss.Width(tail)-1)
	return "Delete " + lipgloss.NewStyle().MaxWidth(branch).Render(f.branch) + "…" + tail
}

func (f merging) deleteRow(th theme.Theme, width int) string {
	base := lipgloss.NewStyle()
	if f.on() == mergeDeleteRow {
		base = base.Background(th.SelectedBackground)
	}

	mark := base.Foreground(th.Subtle).Render(mergeGap)
	if f.del {
		mark = base.Foreground(th.Success).Render(mergeMark)
	}

	text := f.deleteText(width - lipgloss.Width(mergeMark))
	return mergePad(mark+base.Foreground(th.Text).Render(text), width, base)
}

// The hint gives way before the button, the only way to merge where the terminal cannot send the chord.
func (f merging) footer(th theme.Theme, width int) string {
	style := lipgloss.NewStyle().
		Padding(0, mergeButtonPad).
		Foreground(th.Text).
		Background(th.SelectedBackground)

	switch {
	case !f.ready():
		style = style.Foreground(th.Subtle)
	case f.on() == mergeButtonRow:
		style = style.Foreground(th.Inverted).Background(th.Accent)
	}
	button := style.Render(mergeButton)

	text := f.footerText()
	hint := lipgloss.NewStyle().Foreground(th.Subtle).Render(text)
	gap := width - lipgloss.Width(text) - lipgloss.Width(button)
	if gap < 1 {
		hint, gap = "", max(0, width-lipgloss.Width(button))
	}
	return hint + strings.Repeat(" ", gap) + button
}

func (f merging) footerText() string {
	if f.typing() {
		if f.chords {
			return mergeHintChord
		}
		return mergeHintNoChord
	}

	switch f.on() {
	case mergeMethodRow:
		return mergeHintMethod
	case mergeDeleteRow:
		return mergeHintDelete
	}
	return mergeHintButton
}

func mergePad(content string, width int, style lipgloss.Style) string {
	if gap := width - lipgloss.Width(content); gap > 0 {
		return content + style.Render(strings.Repeat(" ", gap))
	}
	return content
}
