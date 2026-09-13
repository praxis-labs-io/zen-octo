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

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/keys"
	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

const composeRows = 8

const postPad = 2

type words struct {
	placeholder string
	button      string
	send        string
	back        string
}

var (
	commentWords = words{placeholder: "Leave a comment", button: "Post", send: "post", back: "done"}
	replyWords   = words{placeholder: "Leave a reply", button: "Post", send: "post", back: "done"}

	updateWords = words{placeholder: "Empty, so far", button: "Save", send: "save", back: "discard"}
)

type composer struct {
	area area.Model

	// Distinct from focus: the ring can light the card before enter hands it the keyboard.
	typing bool

	words words

	onPost bool

	chords bool
}

func newComposer(th theme.Theme) composer {
	area := textarea(th, composeRows)
	area.Placeholder = commentWords.placeholder
	return composer{area: area, words: commentWords}
}

func textarea(th theme.Theme, rows int) area.Model {
	box := area.New()
	box.ShowLineNumbers = false
	box.Prompt = ""
	box.CharLimit = 0
	box.SetHeight(rows)
	box.SetVirtualCursor(false)

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

func (c composer) body() string { return strings.TrimSpace(c.area.Value()) }

func (c composer) rows(width int) int { return wrappedRows(c.area.Value(), width) }

func (c composer) caretRow(width int) int {
	lines := strings.Split(c.area.Value(), "\n")
	row := min(max(c.area.Line(), 0), len(lines)-1)

	n := 0
	if row > 0 {
		n = wrappedRows(strings.Join(lines[:row], "\n"), width)
	}
	return n + c.area.LineInfo().RowOffset - c.area.ScrollYOffset()
}

func wrappedRows(text string, width int) int {
	if width < 1 {
		return strings.Count(text, "\n") + 1
	}
	return strings.Count(wrap(text, width), "\n") + 1
}

// Capped at the pane so the send button on the card's foot never leaves the screen.
func (m Model) boxRows(c composer, floor, width, chrome int) int {
	room := max(1, m.view.Height()-chrome)
	return min(max(floor, c.rows(width)), room)
}

// Two borders, a heading, a rule, the button row, and the blank line after the card.
const boxChrome = 6

// The thread's border, heading and rule, the comment's byline, and the blank line after it.
const threadChrome = 6

func (c *composer) start() tea.Cmd {
	c.typing, c.onPost = true, false
	return c.area.Focus()
}

func (c *composer) stop() {
	c.typing, c.onPost = false, false
	c.area.Blur()
}

func (c *composer) sent() {
	c.area.Reset()
	c.stop()
}

// Appends to a draft written since rather than overwriting it: a post fails long after the box emptied.
func (c *composer) restore(body string) {
	if held := c.body(); held != "" {
		body = body + "\n\n" + held
	}
	c.area.SetValue(body)
	c.area.MoveToEnd()
}

func (c *composer) step() tea.Cmd {
	c.onPost = !c.onPost
	if c.onPost {
		c.area.Blur()
		return nil
	}
	return c.area.Focus()
}

func (c *composer) setWidth(width int) { c.area.SetWidth(width) }

func (m *Model) composeCard(width int) rendered {
	key := focusKey{kind: focusCompose}

	head := m.said(m.who, "write a comment", m.theme.Subtle, gh.TimelineItem{})

	inner := m.cardWidth(width)
	m.compose.setWidth(inner)
	m.compose.area.SetHeight(m.boxRows(m.compose, composeRows, inner, boxChrome))

	body := m.compose.area.View() + "\n" + m.compose.button(m.theme, inner, m.lit(key))
	block := m.card(head, body, width, m.lit(key), "")

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

func (c composer) button(th theme.Theme, width int, focused bool) string {
	style := lipgloss.NewStyle().
		Padding(0, postPad).
		Foreground(th.Text).
		Background(th.SelectedBackground)

	switch {
	case c.body() == "":
		style = style.Foreground(th.Subtle)
	case c.onPost:
		style = style.Foreground(th.Inverted).Background(th.Accent)
	}

	button := style.Render(c.words.button)

	hint := lipgloss.NewStyle().Foreground(th.Subtle).Render(c.hint(focused))
	gap := width - lipgloss.Width(hint) - lipgloss.Width(button)
	if gap < 1 {
		hint, gap = "", max(0, width-lipgloss.Width(button))
	}
	return hint + strings.Repeat(" ", gap) + button
}

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

// Composing reports whether a text box has the keyboard; while it does, every key belongs to the screen.
func (m Model) Composing() bool { return m.compose.typing || m.inline.typing }

// SetChords says whether the terminal can send ctrl+enter. It changes only the hints.
func (m *Model) SetChords(v bool) { m.compose.chords, m.inline.chords = v, v }

// SetViewer names who comments are from and rebuilds the page.
func (m *Model) SetViewer(a gh.Actor) {
	m.who = a
	m.conv.ok = false
	m.syncContent()
}

func (m Model) canCompose() bool { return m.railTab() && m.detail.Loaded }

func (m Model) canAct() bool { return m.ringTab() && m.detail.Loaded }

// RestoreDraft puts a failed comment back in the compose box, taking the keyboard only if the box is on screen.
func (m *Model) RestoreDraft(body string) tea.Cmd {
	m.conv.ok = false
	m.compose.restore(body)

	if !m.canCompose() {
		return nil
	}

	m.pageRing.on = focusKey{kind: focusCompose}
	cmd := m.compose.start()
	m.focus = paneMain
	m.showCompose()
	return cmd
}

func (m Model) writeComment() (Model, tea.Cmd) { return m.openCompose("") }

func (m Model) openCompose(quote string) (Model, tea.Cmd) {
	cmd := m.compose.start()
	m.clearMention()
	if quote != "" {
		m.compose.quote(quote)
	}

	m.conv.ok = false
	m.pageRing.on = focusKey{kind: focusCompose}

	m.focus = paneMain

	m.showCompose()
	return m, cmd
}

func (m Model) composeKey(keyMsg tea.KeyPressMsg) (Model, tea.Cmd) {
	m, cmd, took := m.mentionKey(keyMsg)
	if took {
		return m, cmd
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
		m.clearMention()
		return m, m.compose.editorCmd()

	case key.Matches(keyMsg, keys.Form.Next), key.Matches(keyMsg, keys.Form.Prev):
		cmd := m.compose.step()
		m.syncContent()
		return m, cmd

	case key.Matches(keyMsg, k.Post):
		return m.post()

	case key.Matches(keyMsg, k.Activate) && m.compose.onPost:
		return m.post()
	}

	m.compose.area, cmd = m.compose.area.Update(keyMsg)
	ask := m.syncMention()

	m.showCompose()
	return m, tea.Batch(cmd, ask)
}

func (m *Model) showCompose() {
	m.syncContent()
	m.view.GotoBottom()
	m.showCaret()
}

func (m Model) post() (Model, tea.Cmd) {
	body := m.compose.body()
	if body == "" {
		return m, nil
	}

	id := m.pr.ID
	m.compose.sent()
	m.clearMention()

	m.pageRing.clear()
	m.conv.ok = false
	m.syncContent()
	return m, func() tea.Msg { return PostCommentMsg{ID: id, Body: body} }
}

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

func (c composer) editorCmd() tea.Cmd {
	path, err := draftFile(c.area.Value())
	if err != nil {
		return func() tea.Msg { return editorDoneMsg{err: err} }
	}

	name, args := editorCommand()
	return tea.ExecProcess(exec.Command(name, append(args, path)...), func(runErr error) tea.Msg {
		defer func() { _ = os.Remove(path) }()

		if runErr != nil {
			return editorDoneMsg{err: runErr}
		}
		out, err := os.ReadFile(path)
		return editorDoneMsg{body: string(out), err: err}
	})
}

func draftFile(body string) (string, error) {
	file, err := os.CreateTemp("", "zen-octo-*.md")
	if err != nil {
		return "", err
	}
	path := file.Name()

	_, writeErr := file.WriteString(body)
	if err := cmp.Or(writeErr, file.Close()); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

type editorDoneMsg struct {
	body string
	err  error
}

// Split on whitespace rather than parsed as a shell, so an editor path containing a space will not run.
func editorCommand() (string, []string) {
	fields := strings.Fields(cmp.Or(os.Getenv("VISUAL"), os.Getenv("EDITOR"), "vi"))
	if len(fields) == 0 {
		return "vi", nil
	}
	return fields[0], fields[1:]
}
