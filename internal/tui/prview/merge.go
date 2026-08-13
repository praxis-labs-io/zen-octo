package prview

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	area "charm.land/bubbles/v2/textarea"
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
	// mergeMinWidth clears the hint line and the button beside it, which is the
	// widest row the form is obliged to draw whole. Nothing variable-length is
	// measured against it, so this is the width the form opens at every time.
	//
	// mergeMaxWidth is the ceiling for a frame narrow enough to matter and
	// nothing else. It matches the picker's, so the two modals never open at
	// visibly different widths over the same pull request.
	mergeMinWidth = 52
	mergeMaxWidth = 72

	// mergeBodyRows is how much of the commit message the box shows at once,
	// and the first thing to give way on a short terminal. A squash body is one
	// line per commit, so six of them is a small branch whole.
	mergeBodyRows = 6

	// mergeBodyFloor is as small as the box goes before the modal starts
	// clipping instead. One line, because the box borders its own text now and
	// those two rows have to come from somewhere. Nineteen rows is the shortest
	// frame the form fits at all, and only at this floor; below that it clips and
	// there is nothing left to give. A single line still scrolls, and still says
	// there is a message there.
	mergeBodyFloor = 1

	mergeButton    = "Merge"
	mergeButtonPad = 2
	mergeMark      = "✓ "
	mergeGap       = "  "
)

// The lines the footer can carry, one per row the form has. They are declared
// together because the modal is measured against the longest of them: a hint
// wider than the width taken from it is dropped rather than shown, so a variant
// added out here without a look at the others goes silently missing.
const (
	mergeHintMethod  = "tab next · j/k method · esc cancel"
	mergeHintDelete  = "tab next · space toggle · esc cancel"
	mergeHintButton  = "tab next · ⏎ merge · esc cancel"
	mergeHintNoChord = "tab to the button to merge · esc cancel"
)

// mergeHintChord is the one that depends on a binding rather than reading as
// prose, so it is built rather than written.
var mergeHintChord = "tab next · " + keys.Detail.Post.Help().Key + " merge · esc cancel"

// mergeHintWidth is the longest the hint gets, which is what the modal is
// measured against. The hint itself changes with the row, and a modal that
// changed width with it would jump as the reader tabbed through.
//
// Measured rather than counted: the separator is a middle dot, which is one
// cell and two bytes, so len would buy two columns nothing draws in.
var mergeHintWidth = func() int {
	widest := 0
	for _, hint := range []string{mergeHintMethod, mergeHintDelete, mergeHintButton, mergeHintNoChord, mergeHintChord} {
		widest = max(widest, lipgloss.Width(hint))
	}
	return widest
}()

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
	body     area.Model

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

	// chords is whether the terminal can tell ctrl+enter from enter, which
	// decides whether the hint may name it. The composer holds the same answer
	// for the same reason.
	chords bool

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

	// And nothing where there is no merge to make. The row keeps its key while
	// its own write is out, so that a reader standing on it keeps their place
	// on the ring, and this is what makes enter inert for that window rather
	// than opening a form over a pull request already being merged.
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
		state.Text = lipgloss.NewStyle().Foreground(th.Text)
		state.Placeholder = lipgloss.NewStyle().Foreground(th.Subtle)
	}
	in.SetStyles(styles)
	return in
}

func newMergeBody(th theme.Theme) area.Model {
	return textarea(th, mergeBodyRows)
}

// prefill writes GitHub's own message for the chosen method into whichever
// fields the reader has not changed.
func (f *merging) prefill() {
	// The caret goes to the start of what it wrote, not the end. Both fields
	// scroll, and a commit subject longer than the box would otherwise open
	// showing its last words with the first ones off the left: the reader has
	// typed nothing and wants to read what is going to be committed, which
	// begins at the beginning.
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

// headStart shows the headline from its first character.
//
// It goes to the end and back, which is not the fussiness it looks like. The
// widget recomputes its visible window only when the caret leaves that window,
// so a caret dropped straight to the start of a stale one keeps it: after a
// longer value is written the box goes on showing exactly as many characters as
// the old one had. Leaving the window is what forces the recompute; coming back
// is what puts it at the beginning.
//
// The beginning is where a field nobody has typed in is read from. focus moves
// it to the end on the way in, which is where one is edited from.
func headStart(in *textinput.Model) {
	in.CursorEnd()
	in.CursorStart()
}

// focus puts the caret in the field holding the keyboard and takes it out of
// the other. The caret is the whole of how a text field says it has the
// keyboard: a background would end at the widget's own reset and paint the
// padding alone.
// The caret goes to the end of the text on the way in, which is where a
// prefilled field is edited from and where every other box puts it. Sitting
// unfocused they show their beginning instead, which is where one is read from;
// prefill is what does that half.
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

// update hands a message that is not a keypress to both text boxes.
//
// Both, rather than the focused one. A paste belongs to whichever has the
// keyboard, and the blurred box drops it; the caret blink is the other traffic
// through here and it belongs to whichever is focused, which is the same
// answer. Choosing between them would be a second copy of that rule for no gain.
func (f *merging) update(msg tea.Msg) tea.Cmd {
	var headline, body tea.Cmd
	f.headline, headline = f.headline.Update(msg)
	f.body, body = f.body.Update(msg)

	// Anything that reached a box may have changed what is in it, and a method
	// switch must not then write over the reader's own words.
	f.typedHeadline = f.typedHeadline || f.headline.Value() != f.text().Headline
	f.typedBody = f.typedBody || f.body.Value() != f.text().Body

	return tea.Batch(headline, body)
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

	case key.Matches(keyMsg, keys.Form.Next):
		return m, m.merging.step(1)
	case key.Matches(keyMsg, keys.Form.Prev):
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

// resize gives the two text boxes the room the frame leaves them.
//
// It is called when the form opens and whenever the screen is resized, and
// never from render. render is reached from View through value receivers, so
// anything it sets is set on a copy that is thrown away: a headline sized there
// keeps a width of zero for real, which makes textinput render from the first
// character and never scroll. The caret then leaves the box on a long commit
// message and every keystroke past the edge is invisible.
//
// The message box is what gives way on a short terminal, down to a floor. The
// picker can afford to be clipped by the compositor; this cannot, and the row
// it loses first is not the one to watch. Clipping takes the bottom border, so
// a form one row too tall keeps its button and stops looking like a box.
func (f *merging) resize(frameWidth, frameHeight int) {
	if !f.open {
		return
	}
	width := fieldText(f.width(frameWidth))
	f.headline.SetWidth(width)
	f.body.SetWidth(width)
	f.body.SetHeight(f.bodyHeight(frameHeight))
}

// render draws the form as a modal, at the size resize last gave it.
func (f merging) render(th theme.Theme, frameWidth, frameHeight int) string {
	width := f.width(frameWidth)
	rows := append([]string{""}, f.methodRows(th, width)...)

	if f.writes() {
		rows = append(rows, "")
		rows = append(rows, f.field(th, "Headline", f.headline.View(), 1, width, mergeHeadlineRow)...)
		rows = append(rows, "")
		rows = append(rows, f.field(th, "Message", f.body.View(), f.body.Height(), width, mergeBodyRow)...)
	}

	if f.branch != "" {
		rows = append(rows, "", f.deleteRow(th, width))
	}
	if f.bypass {
		// Clipped, for the reason the delete row's branch is. The base is the
		// other variable-length name on this form, and left to itself it would
		// widen the whole modal past the ceiling every other row is held to.
		plain := lipgloss.NewStyle()
		warning := plain.Foreground(th.Warning).Render("Bypasses branch protection on " + f.base)
		rows = append(rows, "", mergePad(clipTo(warning, width, plain.Foreground(th.Subtle)), width, plain))
	}

	rows = append(rows, "", f.footer(th, width))
	return comp.Modal(th, "Merge #"+strconv.Itoa(f.number), strings.Join(rows, "\n"))
}

// width is what the modal gets inside its border: the widest row it has to draw
// whole, held between a floor and a ceiling and never wider than the frame.
//
// Whole is the word doing the work. Only the fixed rows are measured, which is
// the footer and the method names, so the form is the same width over every
// pull request. The branch name is the one variable-length thing on it and it is
// truncated to the row instead: measured here, a long feature branch drags the
// whole modal out past the conversation behind it, and a branch is a name the
// header two panes over is already carrying in full.
//
// The commit message is not measured either, for a different reason: both boxes
// scroll their own text, so neither has a width it needs.
func (f merging) width(frameWidth int) int {
	longest := mergeHintWidth + lipgloss.Width(mergeButton) + 2*mergeButtonPad
	for _, method := range f.methods {
		longest = max(longest, lipgloss.Width(mergeMark+mergeName(method)))
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

	// Everything the form draws except the message itself: the modal's two
	// border rows, the row it opens with, the Method heading and its rows, a
	// blank and the
	// title box's three rows, a blank and the message box's own two borders,
	// then the blank and the footer at the foot. After that the delete row and
	// the warning where there are any, each with its own blank.
	//
	// Counted rather than measured, because the height has to be decided before
	// the box it decides is rendered.
	chrome := 2 + 1 + 1 + len(f.methods) + 1 + 3 + 1 + 2 + 2
	if f.branch != "" {
		chrome += 2
	}
	if f.bypass {
		chrome += 2
	}
	return min(mergeBodyRows, max(mergeBodyFloor, frameHeight-chrome))
}

// methodRows is the choices under their heading, with the tick on the one that
// will be used.
//
// The heading sits above them rather than in a column beside them, which is
// where the boxes below carry theirs. A label in the left margin buys nothing
// and pushes every name in nine columns from the edge the rest of the form
// starts at.
//
// Every cell in the lit row sets the background itself: a styled run ends in a
// reset that clears it, so painting the joined row afterwards would color the
// first cell and nothing else.
func (f merging) methodRows(th theme.Theme, width int) []string {
	plain := lipgloss.NewStyle()

	// The heading reads as the boxes' titles do, because it names its block the
	// same way they name theirs.
	heading := plain.Foreground(th.Text).Bold(true).Render("Method")

	out := make([]string, 0, len(f.methods)+1)
	out = append(out, mergePad(heading, width, plain))

	for i, method := range f.methods {
		chosen := i == f.at

		base := plain
		if chosen && f.on() == mergeMethodRow {
			base = base.Background(th.SelectedBackground)
		}

		// The chosen one is the one that will be used, so it is the one that
		// reads. The rest are muted: three names at equal weight make the tick
		// the only thing carrying the answer, and it is two cells wide.
		mark, c := base.Foreground(th.Subtle).Render(mergeGap), th.Subtle
		if chosen {
			mark, c = base.Foreground(th.Success).Render(mergeMark), th.Text
		}

		out = append(out, mergePad(mark+base.Foreground(c).Render(mergeName(method)), width, base))
	}
	return out
}

// field frames a text box, titled with what it holds and bordered like every
// other region on this screen.
//
// The border is where a text field says it has the keyboard. A background
// cannot: it ends at the widget's own reset and would paint the padding alone.
// The pane already colours itself by focus, so the two boxes read the same way
// the panes behind them do.
func (f merging) field(th theme.Theme, title, content string, lines, width int, row mergeRowKind) []string {
	pane := comp.NewPane(th).Title(title).Focus(f.on() == row).Size(width, lines+2)
	padded := lipgloss.NewStyle().Padding(0, 1).Render(content)
	return strings.Split(pane.Render(padded), "\n")
}

// fieldText is the room a boxed field leaves its own text: the modal's row,
// less the box's borders and the padding inside them.
func fieldText(width int) int { return max(1, width-4) }

// deleteText names the branch and what is going to happen to it. The branch
// keeps its head and loses its tail when there is not room for both, which is
// how every other name on this screen is cut: the words after it say what the
// row does, and losing those instead would leave a row that only names a branch.
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

// footer is the keys that work from here and the button, on one row.
//
// The hint gives way first on a narrow modal, for the reason the compose card's
// does: the pane clips from the right, which would take the button rather than
// the words about it, and on a terminal that cannot send the chord the button
// is the only way to merge.
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

// footerText names the keys that work from where the reader is standing, which
// is the compose card's rule and matters more here: this is the one form whose
// key ends the pull request, so a line naming it from a row where it does
// something else is the worst thing the footer can say.
//
// Enter merges on the button alone. On the method row it steps to the next row
// and on the delete row it ticks the box, and both of those said "⏎ merge"
// until somebody pressed it and watched the form move instead.
//
// In a text field enter belongs to the field: a newline in the message, and
// nothing at all in the headline. The chord is what merges from those two rows,
// and it is named only where the terminal can send it; on the rest the button
// is the way, and tab is how they reach it.
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

// mergePad runs a row out to the full width in its own style, so a lit row's
// background reaches the border instead of stopping at the last word.
func mergePad(content string, width int, style lipgloss.Style) string {
	if gap := width - lipgloss.Width(content); gap > 0 {
		return content + style.Render(strings.Repeat(" ", gap))
	}
	return content
}
