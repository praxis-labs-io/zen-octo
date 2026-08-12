package prview

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
	"github.com/zen-octo/zen-octo/internal/tui/keys"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// MergeMsg asks the root to merge the pull request, and to delete the head
// branch afterwards when RefID names one.
//
// RefID is empty where the reader did not ask, where GitHub says they may not,
// and where the repository deletes the branch itself. It is a node id because
// deleteRef takes no branch name.
type MergeMsg struct {
	ID      string
	Options gh.MergeOptions
	RefID   string
}

const (
	// mergeMinWidth clears the hint line, so the form opens at one width over a
	// short commit headline and a long one rather than at two.
	mergeMinWidth = 46
	mergeMaxWidth = 72

	// mergeBodyRows is how much of the commit message the box shows at once,
	// and the first thing to give way on a short terminal. A squash body is one
	// line per commit, so six of them is a small branch whole.
	mergeBodyRows = 6

	// mergeBodyFloor is as small as the box goes before the modal starts
	// clipping instead. Two lines is enough to see that there is a message.
	mergeBodyFloor = 2

	// mergeLabelWidth is the column the method rows hang off, so the tick and
	// the names line up under the caption rather than beside it.
	mergeLabelWidth = 9

	mergeButton    = "Merge"
	mergeButtonPad = 2
	mergeMark      = "✓ "
	mergeGap       = "  "
	mergeHint      = "tab next · enter merge · esc cancel"
)

// mergeRowKind is one focusable row in the form.
type mergeRowKind int

const (
	mergeMethodRow mergeRowKind = iota
	mergeHeadlineRow
	mergeBodyRow
	mergeDeleteRow
	mergeButtonRow
)

// merging is the merge form over the screen, empty when there is none.
//
// Everything it needs is taken when it opens rather than read back off the
// model. The head commit is why: expectedHeadOid is the commit the reader was
// looking at, and a refetch landing behind the modal must not quietly change
// which commit gets merged. The rest follows it for the reason the picker holds
// its own choices, so that what applies is what was on offer.
type merging struct {
	open bool

	// methods is what the repository allows, in GitHub's own order, and at
	// which of them is chosen. A method it forbids is absent rather than
	// greyed: there is nothing to be done about it from here.
	methods []gh.MergeMethod
	at      int

	headline textinput.Model
	body     textarea.Model

	// typedHeadline and typedBody mark a field the reader has changed.
	// Switching method rewrites the untouched ones, because a merge commit and
	// a squash want different sentences, and leaves theirs alone.
	typedHeadline, typedBody bool

	// mergeText and squashText are what GitHub would commit for those two, as
	// of when the form opened. A rebase has neither.
	mergeText, squashText gh.MergeMessage

	// del is whether to delete the head branch afterwards, and branch what it
	// is called. An empty branch is the row not being on offer at all: the
	// repository deletes it itself, GitHub says the viewer may not, or it is
	// already gone.
	del    bool
	branch string

	// bypass is a merge that overrides branch protection, which is the one
	// merge here that has to say so.
	bypass bool

	id, oid, refID, base string
	number               int

	// row is which of rows() has the keyboard.
	row int
}

// mergeOrder is the order GitHub lists the methods in, which is the order they
// are offered in.
var mergeOrder = []gh.MergeMethod{gh.MergeMethodMerge, gh.MergeMethodSquash, gh.MergeMethodRebase}

// mergeName is what each method is called, in GitHub's own words.
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

// method is the one chosen.
func (f merging) method() gh.MergeMethod {
	if f.at < 0 || f.at >= len(f.methods) {
		return ""
	}
	return f.methods[f.at]
}

// writes is whether the chosen method commits a message of its own. A rebase
// replays the branch's own commits, so the two text fields are neither rendered
// nor walked: a field whose contents are discarded is a lie about what is going
// to be committed.
func (f merging) writes() bool { return f.method() != gh.MergeMethodRebase }

// rows is the focus order, which is the rows actually on the form.
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

// on is the row holding the keyboard.
func (f merging) on() mergeRowKind {
	rows := f.rows()
	if f.row < 0 || f.row >= len(rows) {
		return mergeButtonRow
	}
	return rows[f.row]
}

// typing is whether the focused row takes keys as text. Everything the form
// answers for itself has to be settled before this is read, because in a commit
// message j is a j and space is a space.
func (f merging) typing() bool {
	return f.on() == mergeHeadlineRow || f.on() == mergeBodyRow
}

// text is what GitHub would commit for the chosen method.
func (f merging) text() gh.MergeMessage {
	switch f.method() {
	case gh.MergeMethodMerge:
		return f.mergeText
	case gh.MergeMethodSquash:
		return f.squashText
	}
	return gh.MergeMessage{}
}

// ready is whether the button does anything. A method that commits needs a
// headline: GitHub writes no commit with no subject, and a button that takes
// the press and comes back refused is worse than one saying it is not ready.
func (f merging) ready() bool {
	return !f.writes() || strings.TrimSpace(f.headline.Value()) != ""
}

// startMerge opens the form over the pull request on screen, and opens nothing
// where the repository allows no method: GitHub does not permit that, and a
// modal with no way to act is worse than a key that did nothing.
func (m *Model) startMerge() tea.Cmd {
	d := m.railDetail()

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
		headline:   newMergeInput(m.theme),
		body:       newMergeBody(m.theme),
	}
	f.del = f.branch != ""
	f.prefill()

	m.merging = f
	return m.merging.focus()
}

// mergeDefault is the method the form opens on: squash where the repository
// allows it, otherwise the first it does allow.
//
// A preference this client holds rather than one GitHub publishes. A squash is
// the one method that lands a pull request as a single commit, which is the
// shape a branch reads as afterwards, and the fallback is what keeps it from
// being an opinion imposed on a repository with a different one.
func mergeDefault(methods []gh.MergeMethod) int {
	for i, method := range methods {
		if method == gh.MergeMethodSquash {
			return i
		}
	}
	return 0
}

// mergeBranch is the head branch to offer deleting, empty where there is
// nothing to offer.
//
// A repository that deletes on merge gets no offer. GitHub deletes the branch
// itself a moment after the merge, and a second call racing that fails on a ref
// already gone: an error toast about a thing that worked.
//
// A fork's head gets none either. It is somebody else's branch, and deleting it
// from here is the one refusal worth making without being asked to.
//
// Nothing else is gated, because GitHub publishes no field that answers the
// question at the moment it has to be asked. viewerCanDeleteHeadRef reads like
// one and is false on every open pull request, so a row gated on it never
// appears at all. This is the review request's shape instead: the row is
// offered, the call decides, and its failure says the branch is still there.
func mergeBranch(d gh.PullRequestDetail, methods gh.MergeMethods) string {
	if methods.DeleteOnMerge || d.CrossRepository || d.HeadRefID == "" {
		return ""
	}
	return d.HeadRefName
}

// mergeBypass is whether merging here overrides a protection rule. Not the same
// question as whether the row opens at all: an administrator merging a clean
// pull request overrides nothing.
func mergeBypass(d gh.PullRequestDetail) bool {
	return d.Merge == gh.MergeBlocked || d.Merge == gh.MergeBehind
}

func newMergeInput(th theme.Theme) textinput.Model {
	in := textinput.New()
	in.Prompt = ""
	in.CharLimit = 0

	styles := in.Styles()
	for _, state := range []*textinput.StyleState{&styles.Focused, &styles.Blurred} {
		state.Text = lipgloss.NewStyle().Foreground(th.Primary)
		state.Placeholder = lipgloss.NewStyle().Foreground(th.Faint)
	}
	in.SetStyles(styles)
	return in
}

func newMergeBody(th theme.Theme) textarea.Model {
	area := textarea.New()
	area.ShowLineNumbers = false
	area.Prompt = ""
	area.CharLimit = 0
	area.SetHeight(mergeBodyRows)

	styles := area.Styles()
	for _, state := range []*textarea.StyleState{&styles.Focused, &styles.Blurred} {
		state.Base = lipgloss.NewStyle()
		state.Text = lipgloss.NewStyle().Foreground(th.Primary)
		state.Placeholder = lipgloss.NewStyle().Foreground(th.Faint)
		state.CursorLine = lipgloss.NewStyle()
		state.EndOfBuffer = lipgloss.NewStyle().Foreground(th.Faint)
	}
	area.SetStyles(styles)
	return area
}

// prefill writes GitHub's own message for the chosen method into whichever
// fields the reader has not changed.
func (f *merging) prefill() {
	text := f.text()
	if !f.typedHeadline {
		f.headline.SetValue(text.Headline)
		f.headline.CursorEnd()
	}
	if !f.typedBody {
		f.body.SetValue(text.Body)
		f.body.MoveToEnd()
	}
}

// focus puts the caret in the field holding the keyboard and takes it out of
// the other. The caret is the whole of how a text field says it has the
// keyboard: a background would end at the widget's own reset and paint the
// padding alone.
func (f *merging) focus() tea.Cmd {
	switch f.on() {
	case mergeHeadlineRow:
		f.body.Blur()
		return f.headline.Focus()
	case mergeBodyRow:
		f.headline.Blur()
		return f.body.Focus()
	}
	f.headline.Blur()
	f.body.Blur()
	return nil
}

// step moves the keyboard one row, wrapping. The form is short enough that
// wrapping past the button is quicker than turning round.
func (f *merging) step(delta int) tea.Cmd {
	n := len(f.rows())
	f.row = (f.row + delta + n) % n
	return f.focus()
}

// choose moves the method, which is the same act as selecting it: the tick
// follows the cursor, because there is nothing else for the cursor to be.
//
// It re-prefills, which is the point of it. A merge commit and a squash want
// different sentences, so carrying "Merge pull request #23 from …" into a
// squash would commit the wrong one.
func (f *merging) choose(delta int) tea.Cmd {
	f.at = min(max(f.at+delta, 0), len(f.methods)-1)
	f.prefill()

	// A rebase renders neither text field, so the rows under the method have
	// moved up and the keyboard has to come with them.
	f.row = min(f.row, len(f.rows())-1)
	return f.focus()
}

// mergeKey answers every key while the form is up. Nothing below it gets a
// look, the same rule a picker holds to.
//
// The order is the whole of it. Leaving, merging and stepping are settled
// first, because those have to work from a text field too; everything left goes
// into the text where a text field has the keyboard, and is answered by the
// form where one does not. That is what lets j walk the method rows and type a
// j into a commit headline without either being a special case.
func (m Model) mergeKey(keyMsg tea.KeyPressMsg) (Model, tea.Cmd) {
	k := keys.Detail

	switch {
	case key.Matches(keyMsg, k.Back):
		m.merging = merging{}
		return m, nil

	case key.Matches(keyMsg, k.Post):
		return m.applyMerge()

	case key.Matches(keyMsg, k.FocusNext):
		return m, m.merging.step(1)
	case key.Matches(keyMsg, k.FocusPrev):
		return m, m.merging.step(-1)
	}

	if m.merging.typing() {
		var cmd tea.Cmd

		// Changed, not merely pressed. A caret moved across a field is not the
		// reader claiming its words, and marking it there would freeze the
		// prefill on whatever the method was when they looked.
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
	// Enter presses whatever the row holds. On the button that is the merge, on
	// the delete row the checkbox; on a method row there is nothing left to
	// confirm, since the cursor being there is what chose it, so it moves on
	// rather than sitting dead.
	case key.Matches(keyMsg, k.Activate):
		switch m.merging.on() {
		case mergeButtonRow:
			return m.applyMerge()
		case mergeDeleteRow:
			m.merging.del = !m.merging.del
		default:
			return m, m.merging.step(1)
		}

	case key.Matches(keyMsg, k.Toggle) && m.merging.on() == mergeDeleteRow:
		m.merging.del = !m.merging.del

	case key.Matches(keyMsg, k.Up) && m.merging.on() == mergeMethodRow:
		return m, m.merging.choose(-1)
	case key.Matches(keyMsg, k.Down) && m.merging.on() == mergeMethodRow:
		return m, m.merging.choose(1)
	}
	return m, nil
}

// applyMerge closes the form and asks the root to merge. A method that needs a
// headline and has none closes nothing and writes nothing: the button already
// says it is not ready, and pressing it is how a reader finds that out.
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

// mergeOverlay composites the form over a rendered frame, the way the picker
// does and for the same reason: the root does not know one is open, and the
// status bar stays uncovered because a toast is worth reading while it is.
func (m Model) mergeOverlay(frame string) string {
	if !m.merging.open {
		return frame
	}
	return comp.Over(frame, m.merging.render(m.theme, m.width, m.height), m.width, m.height)
}

// render draws the form as a modal sized to fit inside a frame.
//
// The message box is what gives way on a short terminal, down to a floor. The
// picker can afford to be clipped by the compositor; this cannot, and the row
// it loses first is not the one to watch. Clipping takes the bottom border, so
// a form one row too tall keeps its button and stops looking like a box.
func (f *merging) render(th theme.Theme, frameWidth, frameHeight int) string {
	width := f.width(frameWidth)
	f.body.SetHeight(f.bodyHeight(frameHeight))
	f.body.SetWidth(width)
	f.headline.SetWidth(width)

	rows := append([]string{""}, f.methodRows(th, width)...)

	if f.writes() {
		rows = append(rows, "", mergeCaption(th, "Headline", f.on() == mergeHeadlineRow, width))
		rows = append(rows, mergePad(f.headline.View(), width, lipgloss.NewStyle()))
		rows = append(rows, "", mergeCaption(th, "Message", f.on() == mergeBodyRow, width))
		for _, line := range strings.Split(f.body.View(), "\n") {
			rows = append(rows, mergePad(line, width, lipgloss.NewStyle()))
		}
	}

	if f.branch != "" {
		rows = append(rows, "", f.deleteRow(th, width))
	}
	if f.bypass {
		warning := lipgloss.NewStyle().Foreground(th.Warning).
			Render("Bypasses branch protection on " + f.base)
		rows = append(rows, "", mergePad(warning, width, lipgloss.NewStyle()))
	}

	rows = append(rows, "", f.footer(th, width))
	return comp.Modal(th, "Merge #"+strconv.Itoa(f.number), strings.Join(rows, "\n"))
}

// width is what the modal gets inside its border: the widest thing it has to
// show, held between a floor and a ceiling and never wider than the frame.
func (f merging) width(frameWidth int) int {
	longest := lipgloss.Width(mergeHint) + lipgloss.Width(mergeButton) + 2*mergeButtonPad
	for _, method := range f.methods {
		longest = max(longest, mergeLabelWidth+lipgloss.Width(mergeMark+mergeName(method)))
	}
	if f.branch != "" {
		longest = max(longest, lipgloss.Width(mergeMark+f.deleteText()))
	}

	want := min(max(longest, mergeMinWidth), mergeMaxWidth)

	// The modal spends four columns on its border and padding, and the
	// compositor clips what will not fit rather than growing the frame.
	if room := frameWidth - 4; room > 0 {
		want = min(want, room)
	}
	return max(want, 1)
}

// bodyHeight is how tall the message box may be on this frame.
func (f merging) bodyHeight(frameHeight int) int {
	if frameHeight <= 0 {
		return mergeBodyRows
	}

	// Everything the form draws except the message itself: two border rows, the
	// row it opens with, the method rows, the two captions and the two blanks
	// over them, the headline, and the blank and the footer at the foot. Then
	// the delete row and the warning where there are any, each with its own
	// blank. Counted rather than measured, because the height has to be decided
	// before the box it decides is rendered.
	chrome := 2 + 1 + len(f.methods) + 4 + 1 + 2
	if f.branch != "" {
		chrome += 2
	}
	if f.bypass {
		chrome += 2
	}
	return min(mergeBodyRows, max(mergeBodyFloor, frameHeight-chrome))
}

// methodRows is the choices with the tick on the one that will be used. Every
// cell in the lit row sets the background itself: a styled run ends in a reset
// that clears it, so painting the joined row afterwards would color the first
// cell and nothing else.
func (f merging) methodRows(th theme.Theme, width int) []string {
	out := make([]string, 0, len(f.methods))
	for i, method := range f.methods {
		base := lipgloss.NewStyle()
		if i == f.at && f.on() == mergeMethodRow {
			base = base.Background(th.SelectedBackground)
		}

		caption := ""
		if i == 0 {
			caption = "Method"
		}
		lead := base.Foreground(th.Faint).Render(mergeColumn(caption))

		mark := base.Foreground(th.Faint).Render(mergeGap)
		if i == f.at {
			mark = base.Foreground(th.Success).Render(mergeMark)
		}

		name := base.Foreground(th.Primary).Render(mergeName(method))
		out = append(out, mergePad(lead+mark+name, width, base))
	}
	return out
}

// mergeColumn is a caption padded out to the column the method rows hang off.
func mergeColumn(s string) string {
	if len(s) >= mergeLabelWidth {
		return s + " "
	}
	return s + strings.Repeat(" ", mergeLabelWidth-len(s))
}

// mergeCaption names a text field, and says whether it has the keyboard. The
// caption carries that rather than the field: a background set behind a widget
// ends at the widget's own reset and paints the padding alone.
func mergeCaption(th theme.Theme, text string, lit bool, width int) string {
	c := th.Faint
	if lit {
		c = th.Secondary
	}
	plain := lipgloss.NewStyle()
	return mergePad(plain.Foreground(c).Render(text), width, plain)
}

func (f merging) deleteText() string { return "Delete " + f.branch + " after merging" }

func (f merging) deleteRow(th theme.Theme, width int) string {
	base := lipgloss.NewStyle()
	if f.on() == mergeDeleteRow {
		base = base.Background(th.SelectedBackground)
	}

	mark := base.Foreground(th.Faint).Render(mergeGap)
	if f.del {
		mark = base.Foreground(th.Success).Render(mergeMark)
	}
	return mergePad(mark+base.Foreground(th.Primary).Render(f.deleteText()), width, base)
}

// footer is the keys that work from here and the button, on one row.
//
// The hint gives way first on a narrow modal, for the reason the compose card's
// does: the pane clips from the right, which would take the button rather than
// the words about it, and on a terminal that cannot send the chord the button
// is the only way to merge.
func (f merging) footer(th theme.Theme, width int) string {
	style := lipgloss.NewStyle().
		Padding(0, mergeButtonPad).
		Foreground(th.Primary).
		Background(th.SelectedBackground)

	switch {
	case !f.ready():
		style = style.Foreground(th.Faint)
	case f.on() == mergeButtonRow:
		style = style.Foreground(th.Inverted).Background(th.Secondary)
	}
	button := style.Render(mergeButton)

	hint := lipgloss.NewStyle().Foreground(th.Faint).Render(mergeHint)
	gap := width - lipgloss.Width(mergeHint) - lipgloss.Width(button)
	if gap < 1 {
		hint, gap = "", max(0, width-lipgloss.Width(button))
	}
	return hint + strings.Repeat(" ", gap) + button
}

// mergePad runs a row out to the full width in its own style, so a lit row's
// background reaches the border instead of stopping at the last word.
func mergePad(content string, width int, style lipgloss.Style) string {
	if gap := width - lipgloss.Width(content); gap > 0 {
		return content + style.Render(strings.Repeat(" ", gap))
	}
	return content
}
