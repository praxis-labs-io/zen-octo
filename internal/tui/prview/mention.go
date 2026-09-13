package prview

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/keys"
	"github.com/praxis-labs-io/zen-octo/internal/tui/paint"
	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

// Leaves out Copilot and teams, whose mentions notify nobody, and never reads Subject, which can be a label name.
func participants(d gh.PullRequestDetail) []string {
	var out []string
	seen := make(map[string]bool)

	add := func(login string) {
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
		if !r.Team {
			add(r.Actor.Login)
		}
	}
	for _, it := range d.Timeline {
		add(it.Actor.Login)
		add(it.Said().Author.Login)
	}
	for _, t := range d.Threads {
		for _, c := range t.Comments {
			add(c.Author.Login)
		}
	}
	for _, c := range d.Commits {
		add(c.Author.Login)
	}
	return out
}

// Participants come first because the repository's list is one page and may not reach them.
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

const mentionRows = 6

const mentionChrome = 2

type mention struct {
	on   focusKey
	open bool

	// In runes from the start of the caret's logical line, the one coordinate the textarea reports directly.
	at int

	query string

	// The '@' esc was pressed on, or -1, so the popup does not reopen over the same token.
	dismissed int

	cursor int
	top    int
	rows   []gh.Mention
}

// GitHub logins are alphanumerics and hyphens.
func mentionRune(r rune) bool {
	return r == '-' || (r >= '0' && r <= '9') ||
		(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// The '@' must open a word, so an email address opens nothing.
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

// Logins match by prefix so the handle being typed ranks first; names match by substring.
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

func (n *mention) move(delta int) {
	if len(n.rows) == 0 {
		return
	}
	n.cursor = min(max(n.cursor+delta, 0), len(n.rows)-1)
}

func (n mention) chosen() (gh.Mention, bool) {
	if !n.open || n.cursor < 0 || n.cursor >= len(n.rows) {
		return gh.Mention{}, false
	}
	return n.rows[n.cursor], true
}

func (m Model) boxKey() focusKey {
	if m.compose.typing {
		return focusKey{kind: focusCompose}
	}
	return m.inline.at
}

func (m *Model) clearMention() { m.mention = mention{dismissed: -1} }

func (m Model) mentionPeople() []gh.Mention {
	return mentionChoices(m.repo.Meta.Mentions, m.railDetail(), m.who.Login)
}

func (m *Model) syncMention() tea.Cmd {
	box := m.writing()
	if box == nil || !m.ringTab() {
		m.clearMention()
		return nil
	}

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

	if !m.mention.open || m.mention.on != on || m.mention.at != start {
		m.mention.cursor, m.mention.top = 0, 0
	}
	m.mention.on, m.mention.at, m.mention.query, m.mention.open = on, start, query, true
	m.mention.rows = matchMentions(m.mentionPeople(), query)
	m.mention.cursor = min(m.mention.cursor, max(0, len(m.mention.rows)-1))

	return m.needMentions()
}

func (m *Model) needMentions() tea.Cmd {
	if m.pr.Repository == "" || m.repo.Loaded || m.mentionsAsked ||
		m.repo.Status == store.StatusLoading {
		return nil
	}
	m.mentionsAsked = true

	repo := m.pr.Repository
	return func() tea.Msg { return NeedRepoMetaMsg{Repo: repo} }
}

func (m *Model) refillMentions() tea.Cmd {
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

	m.mention.rows = matchMentions(m.mentionPeople(), m.mention.query)
	m.mention.cursor = min(m.mention.cursor, max(0, len(m.mention.rows)-1))
	return nil
}

// x counts cells via LineInfo().CharOffset, not runes via Column().
func (m Model) caretAt(lead int) (x, y int, ok bool) {
	box := m.writing()
	if box == nil || m.boxLine <= 0 || !m.ringTab() || m.view.Height() <= 0 {
		return 0, 0, false
	}

	row := min(max(box.caretRow(box.area.Width()), 0), max(0, box.area.Height()-1))

	paneRow := m.boxLine + row - bodyTop(&m.view)
	if paneRow < 0 || paneRow >= m.view.Height() {
		return 0, 0, false
	}
	y = lead + m.main.Above() + paneRow

	x = m.mainLeft() + 1 + m.bodyGutter() + m.boxCol + box.area.LineInfo().CharOffset
	return x, y, true
}

// Walking back one cell per rune is exact only because a login holds no wide characters.
func (m Model) mentionAnchor(lead int) (x, y int, ok bool) {
	x, y, ok = m.caretAt(lead)
	if !ok {
		return 0, 0, false
	}
	box := m.writing()
	return x - max(0, box.area.Column()-m.mention.at), y, true
}

func (m Model) composeCursor(lead int) *tea.Cursor {
	x, y, ok := m.caretAt(lead)
	if !ok {
		return nil
	}
	return comp.Cursor(m.theme, x, y)
}

func (m Model) mainLeft() int {
	switch {
	case m.sideVisible():
		return m.side.InnerWidth() + 2
	case m.railVisible() && m.railColumn():
		return m.rail.InnerWidth() + 2
	}
	return 0
}

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

	x := max(m.mainLeft()+1, min(ax, m.mainLeft()+m.main.InnerWidth()-w+1))

	y := ay + 1
	if up {
		y = ay - h
	}
	return comp.At(frame, over, x, y, m.width, m.height)
}

// Inserts the tail first because the textarea has no exported way to set the cursor's row.
func (c *composer) insertMention(at int, login string) {
	lines := strings.Split(c.area.Value(), "\n")
	row := min(max(c.area.Line(), 0), len(lines)-1)

	line := []rune(lines[row])
	col := min(max(c.area.Column(), 0), len(line))
	if at < 0 || at > col {
		return
	}

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

func (m Model) applyMention() (Model, tea.Cmd) {
	row, ok := m.mention.chosen()
	box := m.writing()
	if !ok || box == nil {
		m.clearMention()
		return m, nil
	}

	box.insertMention(m.mention.at, row.Login)
	m.clearMention()

	m.showBox()
	return m, nil
}

func (m Model) mentionKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if !m.mention.open {
		return m, nil, false
	}
	k := keys.Detail

	switch {
	case key.Matches(msg, k.Back):
		m.mention.open, m.mention.dismissed = false, m.mention.at
		return m, nil, true

	case key.Matches(msg, k.Activate), key.Matches(msg, keys.Form.Next):
		if _, ok := m.mention.chosen(); !ok {
			m.clearMention()
			return m, nil, false
		}
		next, cmd := m.applyMention()
		return next, cmd, true

	case key.Matches(msg, keys.Form.Prev):
		m.clearMention()
		return m, nil, false

	case !typedKey(msg) && key.Matches(msg, k.Up):
		m.mention.move(-1)
		return m, nil, true
	case !typedKey(msg) && key.Matches(msg, k.Down):
		m.mention.move(1)
		return m, nil, true
	}
	return m, nil, false
}

func typedKey(msg tea.KeyPressMsg) bool {
	return utf8.RuneCountInString(msg.Text) == 1 &&
		msg.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper) == 0
}

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

func (m Model) mentionWaiting() bool {
	return m.mention.open && !m.repo.Loaded && m.repo.Status == store.StatusLoading
}

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

	handle := 0
	for _, p := range shown {
		handle = max(handle, lipgloss.Width("@"+p.Login))
	}

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

	if note != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(th.Subtle).Render(fit(note, width)))
	}
	return comp.Modal(th, "", strings.Join(lines, "\n"))
}

// paint.Clip marks whatever it is handed, so clipping unconditionally puts an ellipsis on a line that fits.
func fit(content string, width int) string {
	if lipgloss.Width(content) <= width {
		return content
	}
	return paint.Clip(content, width, lipgloss.NewStyle())
}
