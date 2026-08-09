package prview

import (
	"cmp"
	"os"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/tui/comp"
	"github.com/zen-octo/zen-octo/internal/tui/keys"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// composeRows is the writing the pane shows at once. Three lines is enough to
// see the shape of a comment without taking the conversation off the screen,
// and anything longer belongs in $EDITOR, which is one key away.
const composeRows = 3

// composeChrome is the button row under the text. The hints go in the bottom
// border, but a control the reader can move onto earns a line of its own.
const composeChrome = 1

// postLabel is the button. The brackets are what make it read as something to
// press rather than a word sitting in the pane.
const postLabel = " Post "

// composer is where a comment gets written. It docks under the panes and takes
// its height off them, so the thread being answered stays on the screen.
//
// The buffer outlives the pane being closed. esc is the same reflex here as
// everywhere else on this screen, and losing three paragraphs to it once is
// enough to stop anyone using the feature.
type composer struct {
	area textarea.Model
	pane comp.Pane

	open bool

	// onPost is whether the button holds focus rather than the text. Enter
	// posts from the button and nowhere else: in the text it is a newline, and
	// a key that sends a half-written comment is worse than one more keystroke.
	onPost bool

	// chords is whether the terminal can tell ctrl+enter from enter. Only a
	// terminal speaking the Kitty keyboard protocol can, so on the rest the
	// chord arrives as a bare enter and would add a blank line. The button is
	// what those terminals post with, and the footer names whichever works.
	chords bool
}

func newComposer(th theme.Theme) composer {
	area := textarea.New()
	area.Placeholder = "Leave a comment"
	area.ShowLineNumbers = false
	area.Prompt = ""

	// The default is four hundred characters, which is a short review comment.
	area.CharLimit = 0

	area.SetHeight(composeRows)

	styles := area.Styles()
	for _, state := range []*textarea.StyleState{&styles.Focused, &styles.Blurred} {
		state.Base = lipgloss.NewStyle()
		state.Text = lipgloss.NewStyle().Foreground(th.Primary)
		state.Placeholder = lipgloss.NewStyle().Foreground(th.Faint)
		state.CursorLine = lipgloss.NewStyle()
		state.EndOfBuffer = lipgloss.NewStyle().Foreground(th.Faint)
	}
	area.SetStyles(styles)

	return composer{area: area, pane: comp.NewPane(th)}
}

// height is what the composer takes off the frame. Zero when it is closed, so
// the panes above get the whole screen back.
//
// It gives way on a frame too short to hold it whole rather than pushing the
// panes past the bottom. A screen that renders more lines than it was handed
// writes over the status bar, which is where the key that quits is named.
func (c composer) height(frame int) int {
	if !c.open {
		return 0
	}
	return min(composeRows+composeChrome+c.pane.Chrome(), max(0, frame))
}

// show opens the pane on whatever is already in the buffer, with the cursor in
// the text rather than on the button: the reader pressed c to write.
func (c *composer) show() tea.Cmd {
	c.open, c.onPost = true, false
	return c.area.Focus()
}

// hide closes the pane and keeps the buffer. Discarding is what posting does.
func (c *composer) hide() {
	c.open, c.onPost = false, false
	c.area.Blur()
}

// body is what has been written, with the surrounding whitespace off. A comment
// of nothing but newlines is not one to post.
func (c composer) body() string { return strings.TrimSpace(c.area.Value()) }

// sent empties the buffer and closes the pane, for a comment on its way. The
// text is not lost: the caller holds it, and puts it back if the post fails.
func (c *composer) sent() {
	c.area.Reset()
	c.hide()
}

// restore puts a failed comment back in the pane, so the words survive the
// network. It reopens: a revert nobody can see reads as the comment vanishing.
func (c *composer) restore(body string) tea.Cmd {
	c.area.SetValue(body)
	c.area.MoveToEnd()
	return c.show()
}

// step moves between the text and the button, which is what tab means here.
func (c *composer) step() tea.Cmd {
	c.onPost = !c.onPost
	if c.onPost {
		c.area.Blur()
		return nil
	}
	return c.area.Focus()
}

func (c *composer) setSize(width, frame int) {
	height := c.height(frame)
	c.pane = c.pane.Size(width, height)
	c.area.SetWidth(c.pane.InnerWidth())

	// One line of writing is the floor. Under that the pane is borders and a
	// button, which is barely a pane, but it is still the answer to the key
	// that was pressed and it still names the one that closes it.
	c.area.SetHeight(max(1, height-composeChrome-c.pane.Chrome()))
}

// view renders the pane. The title says what is being written and where it
// goes, because a bare text box at the foot of the screen says neither.
func (c composer) view(th theme.Theme, title string) string {
	if !c.open {
		return ""
	}

	// Faint until there is something to send. A button that takes a press and
	// does nothing is worse than one that says it is not ready.
	button := lipgloss.NewStyle().Foreground(th.Faint)
	switch {
	case c.body() == "":
	case c.onPost:
		button = lipgloss.NewStyle().Foreground(th.Inverted).Background(th.Secondary)
	default:
		button = lipgloss.NewStyle().Foreground(th.Inverted).Background(th.Faint)
	}

	body := c.area.View() + "\n" + button.Render(postLabel)
	return c.pane.Title(title).Footer(c.hints()).Focus(true).Render(body)
}

// hints is the footer. It names the chord only where the terminal can send it:
// on the rest ctrl+enter arrives as a plain enter, and advertising it would be
// promising a key that adds a blank line.
func (c composer) hints() string {
	parts := make([]string, 0, 4)
	if c.chords {
		parts = append(parts, keys.Detail.Post.Help().Key+" post")
	} else {
		parts = append(parts, "tab · enter post")
	}
	return strings.Join(append(parts,
		keys.Detail.Editor.Help().Key+" editor",
		keys.Detail.Back.Help().Key+" close",
	), " · ")
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
// run. One message for both outcomes because the pane does the same thing
// either way: it comes back.
type editorDoneMsg struct {
	body string
	err  error
}

// Composing reports whether the pane has the keyboard. The root reads it before
// its own bindings: q is a letter in here, and the only way out of that is for
// the root to stand aside.
func (m Model) Composing() bool { return m.compose.open }

// SetChords says whether the terminal can send ctrl+enter. Only the footer
// reads it: the binding is live either way, and on a terminal that cannot send
// the chord the key simply never arrives.
func (m *Model) SetChords(v bool) { m.compose.chords = v }

// RestoreDraft puts a comment that failed to post back in the pane. The words
// are the reader's, and a network that dropped them is not a reason to.
func (m *Model) RestoreDraft(body string) tea.Cmd {
	cmd := m.compose.restore(body)
	m.layout()
	return cmd
}

// composeKey is every key while the pane is open. It answers the handful that
// belong to the pane and hands the rest to the text.
func (m Model) composeKey(keyMsg tea.KeyPressMsg) (Model, tea.Cmd) {
	k := keys.Detail

	switch {
	case key.Matches(keyMsg, k.Back):
		m.compose.hide()
		m.layout()
		return m, nil

	case key.Matches(keyMsg, k.Editor):
		return m, m.compose.editorCmd()

	case key.Matches(keyMsg, k.FocusNext), key.Matches(keyMsg, k.FocusPrev):
		return m, m.compose.step()

	case key.Matches(keyMsg, k.Post):
		return m.post()

	// Enter posts from the button and nowhere else. In the text it falls
	// through below and is a newline, which is the only thing it can be.
	case key.Matches(keyMsg, k.Activate) && m.compose.onPost:
		return m.post()
	}

	var cmd tea.Cmd
	m.compose.area, cmd = m.compose.area.Update(keyMsg)
	return m, cmd
}

// post hands the buffer to the root and empties the pane. An empty buffer is
// nothing to post, and the button says so by going faint rather than by
// swallowing the keypress silently.
func (m Model) post() (Model, tea.Cmd) {
	body := m.compose.body()
	if body == "" {
		return m, nil
	}

	id := m.pr.ID
	m.compose.sent()
	m.layout()
	return m, func() tea.Msg { return PostCommentMsg{ID: id, Body: body} }
}

// editorReturned takes back what the editor wrote. A trailing newline is what
// every editor leaves and no comment wants.
func (m Model) editorReturned(msg editorDoneMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		err := msg.err
		return m, func() tea.Msg { return EditorFailedMsg{Err: err} }
	}

	m.compose.area.SetValue(strings.TrimRight(msg.body, "\n"))
	m.compose.area.MoveToEnd()
	return m, nil
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
