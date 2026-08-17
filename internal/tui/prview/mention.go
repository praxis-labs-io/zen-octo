package prview

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/store"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
	"github.com/zen-octo/zen-octo/internal/tui/keys"
	"github.com/zen-octo/zen-octo/internal/tui/paint"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// participants is everybody who has touched this pull request, in the order the
// page reads them: whoever raised it, then who it is assigned to and who is
// being waited on, then everybody who has said anything, then whoever wrote the
// commits. Deduped by login, ignoring case.
//
// Logins alone, with no name beside them. The only name this side carries is
// Commit.AuthorName, which is what git recorded rather than what the account
// calls itself, and pinning it to a login is an attribution nobody made.
// mentionChoices takes the names off the repository's list instead.
func participants(d gh.PullRequestDetail) []string {
	var out []string
	seen := make(map[string]bool)

	add := func(login string) {
		// Copilot is on every pull request it has reviewed and mentioning it
		// does nothing. It never reaches here from the repository's list, which
		// returns users and not bots, so this is the only place it can arrive.
		if login == "" || strings.EqualFold(login, gh.CopilotLogin) {
			return
		}
		key := strings.ToLower(login)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, login)
	}

	add(d.Author.Login)
	for _, a := range d.Assignees {
		add(a.Login)
	}
	for _, r := range d.Reviewers {
		// A team's handle is built here from an org and a slug rather than
		// returned as one, and mentionableUsers has no team to check it
		// against. A handle guessed wrong posts a mention that notifies nobody
		// and reads exactly like one that worked.
		if !r.Team {
			add(r.Actor.Login)
		}
	}
	for _, it := range d.Timeline {
		// Subject is deliberately not read: it is a handle on two event kinds
		// and a label's name on two others, so harvesting it offers @bug.
		add(it.Actor.Login)
		add(it.Said().Author.Login)
	}
	for _, t := range d.Threads {
		for _, c := range t.Comments {
			add(c.Author.Login)
		}
	}
	for _, c := range d.Commits {
		// Author alone. AuthorName is git's record of who wrote it, so a commit
		// whose email matches no account contributes nothing rather than
		// @Drew White.
		add(c.Author.Login)
	}
	return out
}

// mentionChoices is everybody the popup may offer: the people on this pull
// request first, then the rest of the repository's mentionable users.
//
// The conversation leads because it is who the reader means. It is also the
// correctness half: the repository's list is one page of a hundred, so a
// participant past it would be a name the popup could not offer on a comment
// that is about them. The same union backstops the label and assignee pickers.
//
// The viewer is left out. GitHub sends nobody a notification of their own
// mention, so it is the one row that could never do what the list is for, and
// it would usually sort first: the reader is the author of most pull requests
// they are writing on.
func mentionChoices(repo []gh.Mention, d gh.PullRequestDetail, viewer string) []gh.Mention {
	names := make(map[string]string, len(repo))
	for _, u := range repo {
		names[strings.ToLower(u.Login)] = u.Name
	}

	out := make([]gh.Mention, 0, len(repo))
	seen := make(map[string]bool, len(repo))

	add := func(login, name string) {
		if login == "" || strings.EqualFold(login, viewer) {
			return
		}
		key := strings.ToLower(login)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, gh.Mention{Login: login, Name: name})
	}

	for _, login := range participants(d) {
		add(login, names[strings.ToLower(login)])
	}
	for _, u := range repo {
		add(u.Login, u.Name)
	}
	return out
}

// mentionRows is how many choices the popup shows at once. Six is a screenful
// of names without being a modal: the comment under it is what is being read.
const mentionRows = 6

// mentionChrome is the popup's own two border rows.
const mentionChrome = 2

// mention is the @-autocomplete over an open box: which box it belongs to,
// which token it is answering, and which row the cursor is on.
type mention struct {
	// on is the box the popup belongs to. Both boxes can be on the page and
	// only one can be typed in, so a popup that did not say which would draw
	// over the wrong caret the moment the keyboard moved.
	on   focusKey
	open bool

	// at is where the '@' stands, in runes from the start of the logical line
	// the caret is on. The line rather than the buffer, because that is the one
	// coordinate the textarea answers for directly.
	at int

	// query is what has been typed after it, without the '@'.
	query string

	// dismissed is the '@' esc was pressed on, or -1. Without it the next
	// keystroke reopens the popup over the same token and there is no way to
	// finish a word that begins with an at sign.
	dismissed int

	cursor int
	top    int
	rows   []gh.Mention
}

// mentionRune is what keeps a token alive. A GitHub login is alphanumerics and
// hyphens, so a dot or a slash ends the token where it stands rather than
// carrying it into a path or an address.
func mentionRune(r rune) bool {
	return r == '-' || (r >= '0' && r <= '9') ||
		(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// tokenAt is the @token the caret is sitting in the tail of: where its '@'
// stands, in runes from the start of the line, and what has been typed after it.
//
// The '@' has to open a word. At the start of a line or after a space it does;
// anywhere else it is punctuation inside one, which is what keeps an email
// address from opening a list of logins over it.
//
// It scans back from the caret rather than forward from the '@', because the
// caret is the only thing that says which token is live: a line with three
// handles on it has three, and two of them are finished.
func tokenAt(line []rune, col int) (start int, query string, ok bool) {
	col = min(max(col, 0), len(line))

	at := col
	for at > 0 && mentionRune(line[at-1]) {
		at--
	}
	if at == 0 || line[at-1] != '@' {
		return 0, "", false
	}
	at--

	if at > 0 && !unicode.IsSpace(line[at-1]) {
		return 0, "", false
	}
	return at, string(line[at+1 : col]), true
}

// matchMentions is who a token matches, in the order they were handed over.
//
// The login is matched by prefix and the name by substring. A handle is typed
// left to right, so a substring match on it would put @andrew above @drew for
// "dre" and the row under the cursor would not be the one being typed.
func matchMentions(people []gh.Mention, query string) []gh.Mention {
	if query == "" {
		return people
	}
	q := strings.ToLower(query)

	out := make([]gh.Mention, 0, len(people))
	for _, p := range people {
		if strings.HasPrefix(strings.ToLower(p.Login), q) ||
			(p.Name != "" && strings.Contains(strings.ToLower(p.Name), q)) {
			out = append(out, p)
		}
	}
	return out
}

// move walks the list, stopping at the ends. The rows are a list of names
// rather than a ring: wrapping from the last to the first would insert the
// wrong handle for one press too many.
func (n *mention) move(delta int) {
	if len(n.rows) == 0 {
		return
	}
	n.cursor = min(max(n.cursor+delta, 0), len(n.rows)-1)
}

// chosen is the row the cursor is on.
func (n mention) chosen() (gh.Mention, bool) {
	if !n.open || n.cursor < 0 || n.cursor >= len(n.rows) {
		return gh.Mention{}, false
	}
	return n.rows[n.cursor], true
}

// boxKey names the box with the keyboard, the way writing() hands back the box
// itself. The compose card has a key of its own; the summoned box is named by
// whatever it was opened on.
func (m Model) boxKey() focusKey {
	if m.compose.typing {
		return focusKey{kind: focusCompose}
	}
	return m.inline.at
}

// clearMention closes the popup. Called wherever the box it belongs to stops
// being the box with the keyboard.
func (m *Model) clearMention() { m.mention = mention{dismissed: -1} }

// mentionPeople is who the popup offers, rebuilt from what the screen holds.
func (m Model) mentionPeople() []gh.Mention {
	return mentionChoices(m.repo.Meta.Mentions, m.railDetail(), m.who.Login)
}

// syncMention re-reads the token under the caret after the buffer moved. It is
// the only thing that opens the popup, and short of esc the only thing that
// closes it.
//
// It does not close on a token nobody matches. A list that vanished would be
// indistinguishable from a key that does nothing, which is the one thing this
// has to avoid: the popup says "no match" instead, and closes when the word it
// is answering ends.
func (m *Model) syncMention() tea.Cmd {
	box := m.writing()
	if box == nil || !m.railTab() {
		m.clearMention()
		return nil
	}

	// Nothing is being written while the button holds focus, so there is no word
	// to answer. This runs on every message rather than every keystroke, and the
	// blink that follows a step to the button is one: without this it finds the
	// token still sitting under the caret and puts the list back up over a box
	// the reader has just left, a moment after the step took it down.
	//
	// The dismissal is kept rather than cleared, so a list escaped away does not
	// come back by stepping to the button and back again.
	if box.onPost {
		m.mention.open = false
		return nil
	}

	lines := strings.Split(box.area.Value(), "\n")
	row := min(max(box.area.Line(), 0), len(lines)-1)

	start, query, ok := tokenAt([]rune(lines[row]), box.area.Column())
	if !ok {
		m.mention.open = false
		m.mention.dismissed = -1
		return nil
	}

	on := m.boxKey()
	if !m.mention.open && m.mention.dismissed == start && m.mention.on == on {
		return nil
	}

	// A different token, or a different box, is a different list: the cursor
	// starts at the top rather than wherever the last one left it.
	if !m.mention.open || m.mention.on != on || m.mention.at != start {
		m.mention.cursor, m.mention.top = 0, 0
	}
	m.mention.on, m.mention.at, m.mention.query, m.mention.open = on, start, query, true
	m.mention.rows = matchMentions(m.mentionPeople(), query)
	m.mention.cursor = min(m.mention.cursor, max(0, len(m.mention.rows)-1))

	return m.needMentions()
}

// needMentions asks the root for the repository's people the first time a box
// wants them. The screen cannot fetch, so it leaves as a message, and it is the
// same message the rail's pickers send: the list belongs to the repository and
// the cache is the root's, so a picker opened earlier has already paid for it.
func (m *Model) needMentions() tea.Cmd {
	if m.pr.Repository == "" || m.repo.Loaded || m.mentionsAsked ||
		m.repo.Status == store.StatusLoading {
		return nil
	}
	m.mentionsAsked = true

	repo := m.pr.Repository
	return func() tea.Msg { return NeedRepoMetaMsg{Repo: repo} }
}

// refillMentions hands an open popup whatever the screen now holds, and asks
// again for a repository whose choices were dropped under it.
//
// SetRepo calls it before any of its own guards, and the guards are why. They
// refuse to drop a modal on somebody mid-sentence, and this cannot be one: the
// box the reader is typing in is what opened the popup, and the list under
// their caret is what the fetch was for.
func (m *Model) refillMentions() tea.Cmd {
	// A sync drops the held choices. The popup goes back to saying the list is
	// coming, and the latch has to come off or nothing would ever ask again.
	if !m.repo.Loaded && m.repo.Status == store.StatusIdle {
		m.mentionsAsked = false
		if m.mention.open {
			m.mention.rows = nil
			return m.needMentions()
		}
		return nil
	}

	if !m.mention.open {
		return nil
	}

	// The rows are rebuilt and the cursor is kept where the reader left it. The
	// answer lands mid-word every time, and a cursor sent back to the top would
	// move the row under enter after they had already chosen it.
	m.mention.rows = matchMentions(m.mentionPeople(), m.mention.query)
	m.mention.cursor = min(m.mention.cursor, max(0, len(m.mention.rows)-1))
	return nil
}

// mentionAnchor is the frame cell the caret is drawn in, and whether there is
// one to draw at. It walks the path showCaret walks, then takes the two steps
// showCaret has no use for: the pane the page sits in, and the column.
func (m Model) mentionAnchor(lead int) (x, y int, ok bool) {
	box := m.writing()
	if box == nil || m.boxLine <= 0 || !m.railTab() || m.view.Height() <= 0 {
		return 0, 0, false
	}

	// Clamped to the rows the box is showing, for the reason showCaret clamps
	// it: past the cap the textarea scrolls inside itself and the caret cannot
	// be drawn outside the box.
	row := min(max(box.caretRow(box.area.Width()), 0), max(0, box.area.Height()-1))

	paneRow := m.boxLine + row - bodyTop(&m.view)
	if paneRow < 0 || paneRow >= m.view.Height() {
		return 0, 0, false
	}
	y = lead + m.main.Above() + paneRow

	// The pane's left border, the gutter that centres the measure inside it,
	// the indent the block holding the box carries, and the caret's own offset
	// within its wrapped row. CharOffset rather than Column, because this is
	// cells on a screen where that is runes in a line.
	x = m.mainLeft() + 1 + m.bodyGutter() + m.boxCol + box.area.LineInfo().CharOffset

	// Back to the '@' rather than the caret, so the handles in the list stand
	// under the handle being typed and the popup holds still while it is. One
	// cell per rune is exact here and nowhere else: a login is alphanumerics and
	// hyphens, which is all that can lie between the two.
	x -= max(0, box.area.Column()-m.mention.at)
	return x, y, true
}

// mainLeft is the frame column the conversation pane's left border stands on.
// Zero on the tab that has a box, and read rather than assumed.
func (m Model) mainLeft() int {
	if m.sideVisible() {
		return m.side.InnerWidth() + 2
	}
	return 0
}

// mentionOverlay draws the popup at the caret. lead is the header's height,
// handed down rather than measured again: View has already built it.
//
// Under the line being typed, and above it only when the whole list will not
// fit below. A popup that changed sides as the page scrolled would be somewhere
// new on every keystroke.
func (m Model) mentionOverlay(frame string, lead int) string {
	if !m.mention.open {
		return frame
	}
	note := m.mentionNote()
	if note == "" && len(m.mention.rows) == 0 {
		return frame
	}

	ax, ay, ok := m.mentionAnchor(lead)
	if !ok {
		return frame
	}

	top := lead + m.main.Above()
	bottom := top + m.view.Height() - 1

	// The note is a line of its own under the rows, so it costs one of them.
	spare := mentionChrome
	if note != "" {
		spare++
	}

	want := min(len(m.mention.rows), mentionRows)
	below, above := bottom-ay-spare, ay-top-spare

	rows, up := want, false
	switch {
	case below >= want:
	case above >= want:
		up = true
	case above > below:
		rows, up = above, true
	default:
		rows = below
	}
	if rows < 0 || (rows == 0 && note == "") {
		return frame
	}

	over := m.mention.render(m.theme, note, rows, m.main.InnerWidth()-cardIndent*2)
	w, h := lipgloss.Size(over)

	// Clamped to the conversation pane rather than to the frame: on a wide
	// terminal the rail sits to the right of it, and a popup clamped to the
	// screen would hang over a column it has nothing to do with.
	x := max(m.mainLeft()+1, min(ax, m.mainLeft()+m.main.InnerWidth()-w+1))

	y := ay + 1
	if up {
		y = ay - h
	}
	return comp.At(frame, over, x, y, m.width, m.height)
}

// insertMention swaps the @token under the caret for the login and a space, and
// leaves the caret after it.
//
// It rebuilds the buffer back to front. SetValue empties the box and inserts,
// which leaves the caret at the end of what it inserted rather than at the end
// of the buffer, and there is no exported way to set the cursor's row. So the
// tail goes in first, the caret goes back to the top, and the head is inserted
// in front of it: the caret comes to rest exactly where the head ends.
//
// The trailing space is the point of the whole thing. A handle run together
// with the next word is a mention GitHub does not resolve.
func (c *composer) insertMention(at int, login string) {
	lines := strings.Split(c.area.Value(), "\n")
	row := min(max(c.area.Line(), 0), len(lines)-1)

	line := []rune(lines[row])
	col := min(max(c.area.Column(), 0), len(line))
	if at < 0 || at > col {
		return
	}

	// The whole word goes, not the part in front of the caret. A reader who put
	// the caret back inside a handle to correct it means to replace the handle,
	// and keeping the tail turns @nikita into "@nkr kita".
	end := col
	for end < len(line) && mentionRune(line[end]) {
		end++
	}

	head := strings.Join(append(append([]string{}, lines[:row]...),
		string(line[:at])+"@"+login+" "), "\n")
	tail := strings.Join(append([]string{string(line[end:])}, lines[row+1:]...), "\n")

	c.area.SetValue(tail)
	c.area.MoveToBegin()
	c.area.InsertString(head)
}

// applyMention writes the row under the cursor into the box and closes the
// popup.
func (m Model) applyMention() (Model, tea.Cmd) {
	row, ok := m.mention.chosen()
	box := m.writing()
	if !ok || box == nil {
		m.clearMention()
		return m, nil
	}

	box.insertMention(m.mention.at, row.Login)
	m.clearMention()

	// SetValue scrolls the box back to the top and does not reposition, so the
	// caret's row is only right once a render has sized the textarea again.
	// showBox is what does that, and it has to run before anything reads it.
	m.showBox()
	return m, nil
}

// mentionKey answers the keys the popup owns while it is up, and reports
// whether it took the press. Both boxes call it before anything of their own:
// every key it answers means something underneath it that the reader does not
// want. esc closes the box and throws away an edit's draft, tab blurs the
// textarea and steps to the button, and enter breaks the line in half.
func (m Model) mentionKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if !m.mention.open {
		return m, nil, false
	}
	k := keys.Detail

	switch {
	case key.Matches(msg, k.Back):
		m.mention.open, m.mention.dismissed = false, m.mention.at
		return m, nil, true

	// Both keys insert, which is what the box's own tab does one level out:
	// tab moves to the thing that finishes what is being written, and while a
	// list is up that is the handle under the cursor.
	case key.Matches(msg, k.Activate), key.Matches(msg, keys.Form.Next):
		// Only where there is a row to write. A popup saying the list is on its
		// way has nothing to insert, and taking the key anyway swallowed it: the
		// reader pressed enter for a newline, got a closed popup, and had to
		// press it again.
		if _, ok := m.mention.chosen(); !ok {
			m.clearMention()
			return m, nil, false
		}
		next, cmd := m.applyMention()
		return next, cmd, true

	// shift+tab is the reader leaving the list for the button, so the list goes
	// and the press carries on to the box that steps there. Taken and dropped,
	// it left the popup open over a blurred box and the enter that follows
	// wrote a handle instead of posting: on a terminal that cannot send the
	// chord, that button is the only way a comment is sent at all.
	case key.Matches(msg, keys.Form.Prev):
		m.clearMention()
		return m, nil, false

	// The arrows alone. Up and Down carry k and j, which are letters in a box,
	// so the printable guard comp.Picker.Insert uses is what makes this safe.
	case !typedKey(msg) && key.Matches(msg, k.Up):
		m.mention.move(-1)
		return m, nil, true
	case !typedKey(msg) && key.Matches(msg, k.Down):
		m.mention.move(1)
		return m, nil, true
	}
	return m, nil, false
}

// typedKey is a keypress standing for one printable character, which is the
// test comp.Picker.Insert already uses to tell a letter from a key's name.
func typedKey(msg tea.KeyPressMsg) bool {
	return utf8.RuneCountInString(msg.Text) == 1 &&
		msg.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper) == 0
}

// mentionNote is the line under the rows, and it is empty only when there is
// nothing left to say.
//
// The people on the pull request are known without a fetch, so the popup opens
// with them whatever the repository has answered. The note is what stops that
// reading as the whole list: a handle missing from three rows is a key that
// looks broken, where a handle missing under "Loading people" is a wait.
//
// Loaded first, so a list that answered once survives a failed refetch, the way
// both diff panes read their own status.
func (m Model) mentionNote() string {
	switch {
	case !m.repo.Loaded && m.repo.Status == store.StatusFailed:
		return "Could not read the repository"
	case !m.repo.Loaded:
		return m.spinner.Render("Loading people")
	case len(m.mention.rows) > 0:
		return ""
	case m.mention.query == "":
		return "Nobody to mention"
	}
	return "No match"
}

// mentionWaiting is a people list on its way with a popup open on it. The glyph
// is on the screen for the whole fetch, so the tick chain has to be alive for
// the whole fetch or it freezes on its first frame.
func (m Model) mentionWaiting() bool {
	return m.mention.open && !m.repo.Loaded && m.repo.Status == store.StatusLoading
}

// render is the popup: a row per person, the handle then the name. No title, no
// filter row, no hints. The box under it is the filter, and a frame of chrome
// over the line being typed would cover more of the comment than it explained.
func (n mention) render(th theme.Theme, note string, visible, width int) string {
	visible = min(max(visible, 0), len(n.rows))
	if visible == 0 {
		return comp.Modal(th, "", lipgloss.NewStyle().Foreground(th.Subtle).Render(fit(note, width)))
	}

	top := min(max(n.top, 0), max(0, len(n.rows)-visible))
	if n.cursor < top {
		top = n.cursor
	}
	if n.cursor >= top+visible {
		top = n.cursor - visible + 1
	}

	shown := n.rows[top : top+visible]

	// The handles are padded to the widest of them so the names line up in a
	// column. Measured over the rows on screen rather than the whole list: a
	// filter that leaves one long handle behind should not indent every row
	// under it.
	handle := 0
	for _, p := range shown {
		handle = max(handle, lipgloss.Width("@"+p.Login))
	}

	// Each row stays in two pieces to the end, because the two take different
	// colours and only the pieces know where one stops. Clipped as one string
	// and split again afterwards, a cut that reached into the handle left the
	// prefix matching nothing, and the handle was drawn twice at a width past
	// anything the pane had to give.
	heads := make([]string, len(shown))
	tails := make([]string, len(shown))
	widest := 0
	for i, p := range shown {
		head, tail := "@"+p.Login, ""
		if p.Name != "" {
			tail = strings.Repeat(" ", handle-lipgloss.Width(head)) + "  " + p.Name
		}

		if room := width - lipgloss.Width(head); room < 0 {
			head, tail = fit(head, width), ""
		} else if lipgloss.Width(tail) > room {
			tail = fit(tail, room)
		}

		heads[i], tails[i] = head, tail
		widest = max(widest, lipgloss.Width(head)+lipgloss.Width(tail))
	}

	lines := make([]string, len(shown))
	for i := range shown {
		// Every cell carries the background itself. A styled run ends in a full
		// reset, so setting it once around the joined row paints the handle and
		// leaves the name beside it bare. The pad is a cell for the same
		// reason: a lit row stopping at its last character would be a highlight
		// the width of one handle.
		lit := top+i == n.cursor
		cell := lipgloss.NewStyle()
		if lit {
			cell = cell.Background(th.SelectedBackground)
		}

		rest := cell.Foreground(th.Subtle)
		if lit {
			rest = cell.Foreground(th.Text)
		}

		row := cell.Foreground(th.Text).Render(heads[i])
		if tails[i] != "" {
			row += rest.Render(tails[i])
		}
		row += cell.Render(strings.Repeat(" ",
			max(0, widest-lipgloss.Width(heads[i])-lipgloss.Width(tails[i]))))
		lines[i] = row
	}

	// Under the rows rather than over them, so the row the cursor opens on is
	// the first line of the box and a note arriving late moves nothing.
	if note != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(th.Subtle).Render(fit(note, width)))
	}
	return comp.Modal(th, "", strings.Join(lines, "\n"))
}

// fit is content cut to a width, and left alone when it already fits. comp.Clip
// marks whatever it is handed, so a caller that clips unconditionally puts an
// ellipsis on a line with room to spare.
func fit(content string, width int) string {
	if lipgloss.Width(content) <= width {
		return content
	}
	return paint.Clip(content, width, lipgloss.NewStyle())
}
