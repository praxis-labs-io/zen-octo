package prview

import (
	"cmp"
	"os"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/key"
	area "charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/keys"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// composeRows is the writing the box shows at once. Enough to see a paragraph
// whole: a box that shows three lines of an eight-line comment is one you write
// in blind, and $EDITOR is for the times that is not enough.
const composeRows = 8

// postPad is the room either side of the label inside the button.
const postPad = 2

// words is what one box calls itself: what an empty one invites, what its
// button says, the word the hint gives the key that sends it, and what leaving
// does. A comment is posted and left; a comment being rewritten is updated and
// cancelled, and calling that "post" would read as a second comment about to
// appear.
//
// The padding around the label is the button: a filled surface reads as
// something to press, where a bare word reads as a caption.
type words struct {
	placeholder string
	button      string
	send        string
	back        string
}

var (
	commentWords = words{placeholder: "Leave a comment", button: "Post", send: "post", back: "done"}
	replyWords   = words{placeholder: "Leave a reply", button: "Post", send: "post", back: "done"}

	// An edit opens on the comment it is rewriting, so the only way to see this
	// placeholder is to clear the box. Discard rather than done or close:
	// nothing is kept, and the key that drops a rewrite should say so before it
	// is pressed.
	updateWords = words{placeholder: "Empty, so far", button: "Save", send: "save", back: "discard"}
)

// composer is where a comment gets written: the last card in the conversation,
// under everything already said, which is where GitHub puts it and where the
// comment being written is going to appear.
//
// It is always on the page. A box that has to be summoned is one nobody knows
// is there, and the card is cheap: eight rows at the end of a thread nobody is
// reading the end of.
//
// typing is whether it has the keyboard. Focus alone is not enough: the ring
// walks onto it the way it walks onto any card, and until enter it is a card
// like the rest, so j still scrolls.
type composer struct {
	area area.Model

	typing bool

	// words is what this box calls its button and the keys around it. The
	// compose card only ever posts; the summoned box is told when it opens,
	// because the same widget answers for a reply and for an edit.
	words words

	// onPost is whether the button holds focus rather than the text. Enter
	// posts from the button and nowhere else: in the text it is a newline, and
	// a key that sends a half-written comment is worse than one more keystroke.
	onPost bool

	// chords is whether the terminal can tell ctrl+enter from enter. Only a
	// terminal speaking the Kitty keyboard protocol can, so on the rest the
	// chord arrives as a bare enter and would add a blank line. The button is
	// what those terminals post with, and the hint names whichever works.
	chords bool
}

func newComposer(th theme.Theme) composer {
	area := textarea(th, composeRows)
	area.Placeholder = commentWords.placeholder
	return composer{area: area, words: commentWords}
}

// textarea is a text box in this screen's colours, at the height the caller
// wants. Both boxes here and the merge form's commit message want the same
// thing, and the styles are set the same way twice: over the focused and the
// blurred state alike, because a blurred box still renders its text and the
// library's default palette is not this theme's.
//
// Every default it overrides is one that would show. The prompt and the line
// numbers are chrome a comment does not want, and the character limit is four
// hundred out of the box, which is a short review comment.
func textarea(th theme.Theme, rows int) area.Model {
	box := area.New()
	box.ShowLineNumbers = false
	box.Prompt = ""
	box.CharLimit = 0
	box.SetHeight(rows)

	styles := box.Styles()
	for _, state := range []*area.StyleState{&styles.Focused, &styles.Blurred} {
		state.Base = lipgloss.NewStyle()
		state.Text = lipgloss.NewStyle().Foreground(th.Text)
		state.Placeholder = lipgloss.NewStyle().Foreground(th.Subtle)
		state.CursorLine = lipgloss.NewStyle()
		state.EndOfBuffer = lipgloss.NewStyle().Foreground(th.Subtle)
	}
	box.SetStyles(styles)
	return box
}

// body is what has been written, with the surrounding whitespace off. A comment
// of nothing but newlines is not one to post.
func (c composer) body() string { return strings.TrimSpace(c.area.Value()) }

// rows is how many lines the writing takes at this width, which is what a box
// that grows with what is typed into it is sized by.
//
// Counted by hand rather than taken from LineCount, which is the logical count:
// the textarea folds a long line rather than scrolling sideways, so a paragraph
// is worth as many rows as it wraps onto, and a box sized by the other number
// sits a row short of what it is showing.
func (c composer) rows(width int) int { return wrappedRows(c.area.Value(), width) }

// caretRow is which of those rows the caret is on, counted the same way and
// less whatever the textarea has scrolled inside itself. It is where the page
// has to look to keep the caret in sight, and it is the only thing that can:
// a box grows with what is typed into it, so it can be taller than the window
// and the card holding it says nothing about where in it the caret sits.
func (c composer) caretRow(width int) int {
	lines := strings.Split(c.area.Value(), "\n")
	row := min(max(c.area.Line(), 0), len(lines)-1)

	n := 0
	if row > 0 {
		n = wrappedRows(strings.Join(lines[:row], "\n"), width)
	}
	return n + c.area.LineInfo().RowOffset - c.area.ScrollYOffset()
}

// wrappedRows is how many rows text folds onto at a width, folded where the
// textarea folds it.
//
// On word boundaries, which is the only count worth taking: a character count
// puts a word straddling the edge on the row it does not fit on, and a line
// exactly the width of the box is one row by that arithmetic and two on the
// screen. Either way the box is sized short of its own writing and scrolls
// where it was supposed to grow.
func wrappedRows(text string, width int) int {
	if width < 1 {
		return strings.Count(text, "\n") + 1
	}
	return strings.Count(wrap(text, width), "\n") + 1
}

// boxRows is the height a box gets: what it is standing in for, or the writing
// in it, whichever is more, and never more than the pane can show at once.
//
// Growing with the content is what stops a box being a window onto its own
// text. The floor is what it opens at, which is eight rows of invitation on the
// compose card and however tall the words were on an edit.
//
// The ceiling beats the floor, and it is the button that makes it necessary
// rather than the writing. A box taller than the pane takes the foot of its own
// card off the screen, and the foot is where the control that sends the words
// is: the reader is then writing into something with no visible end and no way
// out but a chord they cannot see named. Past the ceiling the textarea scrolls
// inside itself, which keeps its own caret in view.
//
// chrome is what the card around the box costs, and it is the caller's because
// only the render site knows how deep the box sits.
func (m Model) boxRows(c composer, floor, width, chrome int) int {
	room := max(1, m.view.Height()-chrome)
	return min(max(floor, c.rows(width)), room)
}

// boxChrome is what a card spends around a box: two borders, a heading, a rule,
// the row the button and its hints ride on, and the blank line the block after
// it wants.
const boxChrome = 6

// threadChrome is what a comment inside a thread pays on top of that: the
// thread's own border, heading and rule, the byline over the comment, and the
// blank line between it and the comment after. A box that spent the pane as
// though it were a card of its own would push the thread's foot off the screen
// with the button on it.
const threadChrome = 6

// start puts the keyboard in the text. The card is already on the page; this is
// the difference between looking at it and writing in it.
func (c *composer) start() tea.Cmd {
	c.typing, c.onPost = true, false
	return c.area.Focus()
}

// stop hands the keyboard back to the screen, keeping every word. The card
// stays where it is: it is part of the conversation, not something summoned.
func (c *composer) stop() {
	c.typing, c.onPost = false, false
	c.area.Blur()
}

// sent empties the box, for a comment on its way. The text is not lost: the
// caller holds it, and puts it back if the post fails.
func (c *composer) sent() {
	c.area.Reset()
	c.stop()
}

// restore puts a failed comment back, so the words survive the network.
//
// It appends rather than assigns when something is already there. A post is
// answered for long after the box emptied, and by then the reader may be
// writing the next comment; overwriting would destroy that one to rescue this
// one, which is the same loss in the other direction. Two comments run together
// is a mess the reader can cut apart. A comment that is gone is gone.
func (c *composer) restore(body string) {
	if held := c.body(); held != "" {
		body = body + "\n\n" + held
	}
	c.area.SetValue(body)
	c.area.MoveToEnd()
}

// step moves between the text and the button, which is what tab means while
// the box has the keyboard.
func (c *composer) step() tea.Cmd {
	c.onPost = !c.onPost
	if c.onPost {
		c.area.Blur()
		return nil
	}
	return c.area.Focus()
}

func (c *composer) setWidth(width int) { c.area.SetWidth(width) }

// card renders the box the way every other entry in the conversation renders:
// a heading, a rule, then the content. It is the same component, so a comment
// being written sits among the comments already made rather than beside them.
func (m *Model) composeCard(width int) rendered {
	key := focusKey{kind: focusCompose}

	// said drops the login when there is none, which is what the heading needs
	// until the viewer query lands, and the empty item keeps a time off a card
	// nothing has happened to yet.
	head := m.said(m.who, "write a comment", m.theme.Subtle, gh.TimelineItem{})

	inner := m.cardWidth(width)
	m.compose.setWidth(inner)
	m.compose.area.SetHeight(m.boxRows(m.compose, composeRows, inner, boxChrome))

	body := m.compose.area.View() + "\n" + m.compose.button(m.theme, inner, m.lit(key))
	block := m.card(head, body, width, m.lit(key), "")

	// Only while it has the keyboard. The card is on the page either way, and a
	// box line recorded for one nobody is typing in would be the last block on
	// the page overwriting whichever box actually holds the caret.
	boxAt, boxCol := 0, 0
	if m.compose.typing {
		boxAt, boxCol = m.cardLead(width, strings.Count(body, "\n")+1), cardIndent
	}

	return rendered{
		block:  block,
		stops:  []focusItem{{focusKey: key, lines: strings.Count(block, "\n") + 1}},
		boxAt:  boxAt,
		boxCol: boxCol,
	}
}

// button is the post control, on the last row of the card and against its right
// edge.
//
// It is a filled surface at every state, because it is a button at every state.
// Muted is the colour, not the shape: the raised background the rail uses for
// its cursor line. Focus swaps it for the accent, and an empty buffer greys the
// label without taking the surface away, so the control does not appear and
// disappear as you type.
func (c composer) button(th theme.Theme, width int, focused bool) string {
	style := lipgloss.NewStyle().
		Padding(0, postPad).
		Foreground(th.Text).
		Background(th.SelectedBackground)

	switch {
	// Nothing to send. A control that takes a press and does nothing is worse
	// than one that says it is not ready.
	case c.body() == "":
		style = style.Foreground(th.Subtle)
	case c.onPost:
		style = style.Foreground(th.Inverted).Background(th.Accent)
	}

	button := style.Render(c.words.button)

	// The hint gives way first. A narrow card that keeps both overflows, and the
	// pane clips from the right, which takes the button rather than the words
	// about it: on a terminal that cannot send the chord that is the only way to
	// post, gone.
	hint := lipgloss.NewStyle().Foreground(th.Subtle).Render(c.hint(focused))
	gap := width - lipgloss.Width(hint) - lipgloss.Width(button)
	if gap < 1 {
		hint, gap = "", max(0, width-lipgloss.Width(button))
	}
	return hint + strings.Repeat(" ", gap) + button
}

// hint is the line beside the button, and it names the key that works from
// where the reader is standing. enter starts writing only once the ring is on
// the box; from anywhere else on the page it is c, and naming the wrong one is
// a key that does nothing to whoever tries it.
//
// The chord is named only where the terminal can send it: on the rest
// ctrl+enter arrives as a plain enter and would add a blank line.
func (c composer) hint(focused bool) string {
	if !c.typing {
		if focused {
			return keys.Detail.Activate.Help().Key + " to write"
		}
		return keys.Detail.Comment.Help().Key + " to write"
	}

	send := "tab · ⏎ " + c.words.send
	if c.chords {
		send = keys.Detail.Post.Help().Key + " " + c.words.send
	}
	return strings.Join([]string{
		send,
		keys.Detail.Editor.Help().Key + " editor",
		keys.Detail.Back.Help().Key + " " + c.words.back,
	}, " · ")
}

// Composing reports whether a box has the keyboard. The root reads it before
// its own bindings: q is a letter in there, and the only way out of that is for
// the root to stand aside.
func (m Model) Composing() bool { return m.compose.typing || m.inline.typing }

// SetChords says whether the terminal can send ctrl+enter. Only the hints read
// it: the binding is live either way, and on a terminal that cannot send the
// chord the key simply never arrives.
func (m *Model) SetChords(v bool) { m.compose.chords, m.inline.chords = v, v }

// SetViewer names who a comment will be from, for the box's heading.
//
// It rebuilds the page. The login lands after the first screen is already open,
// and the heading is a rendered string by then: setting the field alone would
// leave the box headed by nobody until something else happened to redraw.
func (m *Model) SetViewer(a gh.Actor) {
	m.who = a
	m.conv.ok = false
	m.syncContent()
}

// canCompose is whether the box is on the screen to be written in. The
// conversation is the only tab that renders it, and only once there is a
// conversation: the failure and the spinner take its place.
//
// Turning typing on without it is the worst thing this screen can do. The root
// stands aside for Composing(), so every key from then on goes to a textarea
// nobody can see, and the only way out is an esc the reader has no reason to
// press.
func (m Model) canCompose() bool { return m.railTab() && m.detail.Loaded }

// RestoreDraft puts a comment that failed to post back in the box. The words
// are the reader's, and a network that dropped them is not a reason to lose
// them.
//
// It takes the keyboard back only where the box is on screen. A reader who
// moved to another tab while the write was out keeps the tab they chose, and
// the words are waiting the next time they open the box.
func (m *Model) RestoreDraft(body string) tea.Cmd {
	m.conv.ok = false
	m.compose.restore(body)

	if !m.canCompose() {
		return nil
	}

	m.convRing.on = focusKey{kind: focusCompose}
	cmd := m.compose.start()
	m.focus = paneMain
	m.showCompose()
	return cmd
}

// writeComment puts the keyboard in the box and brings it onto the screen. It
// is the last card in the conversation, so on a long thread it is usually
// somewhere below the fold when the key is pressed.
func (m Model) writeComment() (Model, tea.Cmd) { return m.openCompose("") }

// openCompose is writeComment with words to open on. quote is empty for c, and
// the focused block's body for a quote reply to something with no thread under
// it, which the conversation cannot answer any other way.
func (m Model) openCompose(quote string) (Model, tea.Cmd) {
	cmd := m.compose.start()
	m.clearMention()
	if quote != "" {
		m.compose.quote(quote)
	}

	// Focus moves onto the box, so whichever card was lit is not any more.
	m.conv.ok = false
	m.convRing.on = focusKey{kind: focusCompose}

	// The box is in the conversation, so the keys have to be going there. Left
	// on the rail, the accent would name one pane while another took every
	// keystroke.
	m.focus = paneMain

	m.showCompose()
	return m, cmd
}

// composeKey is every key while the box has the keyboard. It answers the
// handful that belong to it and hands the rest to the text.
func (m Model) composeKey(keyMsg tea.KeyPressMsg) (Model, tea.Cmd) {
	// The popup first. Every key it answers means something under it that the
	// reader does not want: esc gives the keyboard back, tab steps to the
	// button, and enter breaks the line in half.
	if next, cmd, took := m.mentionKey(keyMsg); took {
		return next, cmd
	}
	k := keys.Detail

	switch {
	case key.Matches(keyMsg, k.Back):
		m.compose.stop()
		m.clearMention()
		m.conv.ok = false
		m.syncContent()
		return m, nil

	case key.Matches(keyMsg, k.Editor):
		return m, m.compose.editorCmd()

	case key.Matches(keyMsg, keys.Form.Next), key.Matches(keyMsg, keys.Form.Prev):
		cmd := m.compose.step()
		m.syncContent()
		return m, cmd

	case key.Matches(keyMsg, k.Post):
		return m.post()

	// Enter posts from the button and nowhere else. In the text it falls
	// through below and is a newline, which is the only thing it can be.
	case key.Matches(keyMsg, k.Activate) && m.compose.onPost:
		return m.post()
	}

	var cmd tea.Cmd
	m.compose.area, cmd = m.compose.area.Update(keyMsg)
	ask := m.syncMention()

	// The box is content inside a scrolling pane, so the caret leaves the window
	// as readily as any other line would. It is the last block on the page, so
	// holding the page at the foot of it is the whole of keeping the caret in
	// sight.
	m.showCompose()
	return m, tea.Batch(cmd, ask)
}

// showCompose rebuilds the page and brings the box onto it. Typing changes the
// last block, and the pane is holding a string that no longer matches it.
func (m *Model) showCompose() {
	m.syncContent()
	m.view.GotoBottom()
	m.showCaret()
}

// post hands the buffer to the root and empties the box. An empty buffer is
// nothing to post, and the button says so by going faint rather than by
// swallowing the keypress silently.
func (m Model) post() (Model, tea.Cmd) {
	body := m.compose.body()
	if body == "" {
		return m, nil
	}

	id := m.pr.ID
	m.compose.sent()
	m.clearMention()

	// Focus goes with the comment. The box is empty and no longer taking keys,
	// so an accent left on it says the keyboard is somewhere it is not.
	//
	// It does not move onto the comment that just landed, tempting as that is:
	// that card is the placeholder, keyed by an id minted here, and the moment
	// GitHub confirms it the id becomes a real one and the highlight would
	// vanish on its own a few hundred milliseconds later.
	m.convRing.clear()
	m.conv.ok = false
	m.syncContent()
	return m, func() tea.Msg { return PostCommentMsg{ID: id, Body: body} }
}

// editorReturned takes back what the editor wrote. A trailing newline is what
// every editor leaves and no comment wants.
//
// It goes to the box that opened the editor, which is the one still holding the
// keyboard: the program was suspended in between, so nothing can have moved. The
// compose card is not that box whenever a reply opened the editor, and writing
// there would drop the reply into the wrong conversation and take out whatever
// was already in the card on the way.
func (m Model) editorReturned(msg editorDoneMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		err := msg.err
		return m, func() tea.Msg { return EditorFailedMsg{Err: err} }
	}

	box := m.writing()
	if box == nil {
		return m, nil
	}

	box.area.SetValue(strings.TrimRight(msg.body, "\n"))
	box.area.MoveToEnd()
	m.showBox()
	return m, nil
}

// editorCmd hands the buffer to the reader's editor and takes back what comes
// out.
func (c composer) editorCmd() tea.Cmd {
	path, err := draftFile(c.area.Value())
	if err != nil {
		return func() tea.Msg { return editorDoneMsg{err: err} }
	}

	name, args := editorCommand()
	return tea.ExecProcess(exec.Command(name, append(args, path)...), func(runErr error) tea.Msg {
		// A remove that fails leaves one file in the temp directory. There is
		// nowhere on the screen to report that, and it is not worth losing the
		// edit over.
		defer func() { _ = os.Remove(path) }()

		if runErr != nil {
			return editorDoneMsg{err: runErr}
		}
		out, err := os.ReadFile(path)
		return editorDoneMsg{body: string(out), err: err}
	})
}

// draftFile puts the buffer somewhere the editor can open it. The suffix is
// what makes an editor light it up as markdown, which is what a comment is.
func draftFile(body string) (string, error) {
	file, err := os.CreateTemp("", "zen-octo-*.md")
	if err != nil {
		return "", err
	}
	path := file.Name()

	_, writeErr := file.WriteString(body)
	if err := cmp.Or(writeErr, file.Close()); err != nil {
		// Half a draft is worse than none: the editor would open it and the
		// reader would lose the rest without being told.
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

// editorDoneMsg carries back what the editor left behind, or why it did not
// run. One message for both outcomes because the box does the same thing
// either way: it comes back.
type editorDoneMsg struct {
	body string
	err  error
}

// editorCommand is the reader's editor and the arguments in front of the file.
// VISUAL wins over EDITOR, which is the order every other terminal program
// reads them in, and vi is what is there when neither is set.
//
// The value is split on spaces so "code -w" works. That is not a shell, and a
// path with a space in it will not survive: the fix for that is a shell, and a
// shell is a bigger thing to invite in than this is worth.
func editorCommand() (string, []string) {
	fields := strings.Fields(cmp.Or(os.Getenv("VISUAL"), os.Getenv("EDITOR"), "vi"))
	if len(fields) == 0 {
		return "vi", nil
	}
	return fields[0], fields[1:]
}
