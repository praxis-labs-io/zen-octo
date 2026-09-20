package prview

import (
	"image/color"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/keys"
	"github.com/praxis-labs-io/zen-octo/internal/tui/paint"
	"github.com/praxis-labs-io/zen-octo/internal/tui/syntax"
)

const treeGutter = 2

const cardGutter = 1

// GitHub returns the whole hunk, and the commented line is its last.
const threadHunkLines = 4

func (m *Model) conversationBody() string {
	if note, ok := m.conversationNote(); ok {
		return note
	}

	if m.conv.ok && m.writing() != nil && m.conv.box == m.inline.at {
		return m.withBox(m.bodyWidth())
	}
	return m.entries()
}

func (m Model) conversationNote() (string, bool) {
	switch {
	case m.detail.Loaded:
		return "", false
	case m.detail.Status == store.StatusFailed:
		return wrap(m.faint().Render("Could not load the conversation: "+m.detail.Err.Error()), m.bodyWidth()), true
	}
	return m.spinner.Render("Loading the conversation"), true
}

func (m *Model) entries() string {
	d := m.detail.Detail
	width := m.bodyWidth()

	var blocks []string
	at := 0

	split, splitAt, headItems, tailFrom := -1, 0, 0, 0
	marking := false
	mark := func() { split, splitAt, headItems, marking = len(blocks), at, len(m.pageRing.items), true }

	pushStops := func(v rendered) {
		blocks = append(blocks, v.block)
		if v.boxAt > 0 {
			m.boxLine, m.boxCol = at+v.boxAt, v.boxCol
		}
		for _, s := range v.stops {
			m.pageRing.add(s.focusKey, at+s.start, s.lines)
		}
		if marking {
			tailFrom, marking = len(m.pageRing.items), false
		}
		at += strings.Count(v.block, "\n") + 2
	}

	push := func(block string, key focusKey) {
		var stops []focusItem
		if key.kind != focusNone {
			stops = []focusItem{{focusKey: key, lines: strings.Count(block, "\n") + 1}}
		}
		pushStops(rendered{block: block, stops: stops})
	}

	if m.boxOn(focusKey{kind: focusDescription}) {
		mark()
	}
	pushStops(m.description(d, width))

	shown := make(map[int]bool, len(d.Threads))

	for i := 0; i < len(d.Timeline); i++ {
		item := d.Timeline[i]
		switch item.Kind {
		case gh.TimelineComment:
			if m.boxOn(focusKey{kind: focusComment, id: item.Said().ID}) {
				mark()
			}
			pushStops(m.commentCard(item, width))

		case gh.TimelineReview:
			if m.boxUnder(d.Threads, item.Said().ID) {
				mark()
			}
			pushStops(m.review(item, d.Threads, shown, width))

		case gh.TimelineCommit:
			run := commitRun(d.Timeline[i:])
			push(m.pushed(run), focusKey{})
			i += len(run) - 1

		default:
			run := metaRun(d.Timeline[i:])
			if line := m.happened(run); line != "" {
				push(line, focusKey{})
			}
			i += len(run) - 1
		}
	}

	for i, thread := range d.Threads {
		if !shown[i] {
			if m.boxIn(thread) {
				mark()
			}
			pushStops(m.thread(thread, width, true))
		}
	}

	if n := d.MoreComments; n > 0 {
		push(wrap(m.faint().Render(comp.Plural(n, "older comment")+" on GitHub"), width), focusKey{})
	}
	if n := d.MoreThreads; n > 0 {
		push(wrap(m.faint().Render(comp.Plural(n, "more review thread")+" on GitHub"), width), focusKey{})
	}
	if n := d.MoreEvents; n > 0 {
		push(wrap(m.faint().Render(comp.Plural(n, "earlier event")+" on GitHub"), width), focusKey{})
	}

	if split < 0 {
		mark()
	}
	pushStops(m.composeCard(width))

	m.conv = convCache{
		head:      strings.Join(blocks[:split], "\n\n"),
		tail:      strings.Join(blocks[split+1:], "\n\n"),
		items:     slices.Clone(m.pageRing.items[:headItems]),
		tailItems: shifted(m.pageRing.items[tailFrom:], splitAt+strings.Count(blocks[split], "\n")+2),
		at:        splitAt,
		box:       m.inline.at,
		ok:        true,
	}
	return strings.Join(blocks, "\n\n")
}

// Built once per open box, so a keystroke re-renders one block: 7ms rather than 27ms on a long thread.
type convCache struct {
	head      string
	tail      string
	items     []focusItem
	tailItems []focusItem

	at int

	box focusKey

	ok bool
}

func shifted(items []focusItem, by int) []focusItem {
	out := slices.Clone(items)
	for i := range out {
		out[i].start -= by
	}
	return out
}

func (m *Model) withBox(width int) string {
	m.pageRing.items = slices.Clone(m.conv.items)

	middle := m.boxBlock(width)
	if middle.boxAt > 0 {
		m.boxLine, m.boxCol = m.conv.at+middle.boxAt, middle.boxCol
	}
	for _, s := range middle.stops {
		m.pageRing.add(s.focusKey, m.conv.at+s.start, s.lines)
	}

	base := m.conv.at + strings.Count(middle.block, "\n") + 2
	for _, it := range m.conv.tailItems {
		m.pageRing.add(it.focusKey, base+it.start, it.lines)
	}

	return joinBlocks(m.conv.head, middle.block, m.conv.tail)
}

// Reviews are matched before threads, because a review draws the threads it opened inside its own block.
func (m *Model) boxBlock(width int) rendered {
	d := m.detail.Detail

	switch on := m.conv.box; on.kind {
	case focusNone:
		return m.composeCard(width)

	case focusDescription:
		return m.description(d, width)

	case focusComment:
		for _, item := range d.Timeline {
			if item.Kind == gh.TimelineComment && item.Said().ID == on.id {
				return m.commentCard(item, width)
			}
		}
		return rendered{}
	}

	for _, item := range d.Timeline {
		if item.Kind != gh.TimelineReview {
			continue
		}
		if m.boxUnder(d.Threads, item.Said().ID) {
			return m.review(item, d.Threads, make(map[int]bool, len(d.Threads)), width)
		}
	}

	for _, t := range d.Threads {
		if m.boxIn(t) {
			return m.thread(t, width, true)
		}
	}

	return rendered{}
}

func (m Model) boxUnder(threads []gh.ReviewThread, reviewID string) bool {
	if reviewID == "" {
		return false
	}
	if m.boxOn(focusKey{kind: focusReview, id: reviewID}) {
		return true
	}
	return slices.ContainsFunc(threads, func(t gh.ReviewThread) bool {
		return t.ReviewID == reviewID && m.boxIn(t)
	})
}

func joinBlocks(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "\n\n")
}

func (m *Model) commentCard(item gh.TimelineItem, width int) rendered {
	said := item.Said()
	key := focusKey{kind: focusComment, id: said.ID}

	head := m.said(item.Actor, "commented", m.theme.Subtle, item)
	switch {
	case m.boxOn(key):
		head = m.editHead("comment")
	case said.Pending:
		head = m.pendingHead(item.Actor, "commented", "posting")
	case said.Editing:
		head = m.pendingHead(item.Actor, "commented", "saving")
	}

	content := m.bodyOrBox(said.Body, m.cardWidth(width), "No comment.", key, boxChrome)
	block := m.card(head, m.withReactions(content, said.Reactions, key, m.cardWidth(width)), width,
		m.lit(key), m.cardHints(key, said, width))

	boxAt, boxCol := m.cardBoxAt(key, width, content)
	return rendered{
		block:  block,
		stops:  []focusItem{{focusKey: key, lines: strings.Count(block, "\n") + 1}},
		boxAt:  boxAt,
		boxCol: boxCol,
	}
}

func (m *Model) editHead(what string) string {
	return m.said(m.who, "edit this "+what, m.theme.Subtle, gh.TimelineItem{})
}

func (m *Model) bodyOrBox(text string, width int, empty string, key focusKey, chrome int) string {
	body := m.body(text, width, empty, key)
	if m.boxOn(key) {
		return m.inlineBox(width, strings.Count(body, "\n")+1, chrome)
	}
	return body
}

func (m *Model) review(item gh.TimelineItem, threads []gh.ReviewThread, shown map[int]bool, width int) rendered {
	label, c := comp.ReviewStateLabel(m.theme, item.Review)
	head := m.said(item.Actor, label, c, item)

	written := item.Said()
	key := focusKey{kind: focusReview, id: written.ID}
	switch {
	case m.boxOn(key):
		head = m.editHead("review")
	case written.Editing:
		head = m.pendingHead(item.Actor, label, "saving")
	}
	content := m.bodyOrBox(written.Body, m.cardWidth(width), "No comment.", key, boxChrome)
	block := m.card(head, m.withReactions(content, written.Reactions, key, m.cardWidth(width)), width,
		m.lit(key), m.cardHints(key, written, width))

	used := strings.Count(block, "\n") + 1
	stops := []focusItem{{focusKey: key, lines: used}}
	boxAt, boxCol := m.cardBoxAt(key, width, content)

	var owned []rendered
	for i, thread := range threads {
		if thread.ReviewID != written.ID || thread.ReviewID == "" {
			continue
		}
		shown[i] = true
		inner := width - treeGutter
		owned = append(owned, m.thread(thread, inner, true))
	}

	for i, t := range owned {
		lines := strings.Count(t.block, "\n") + 1
		for _, s := range t.stops {
			stops = append(stops, focusItem{focusKey: s.focusKey, start: used + s.start, lines: s.lines})
		}
		if t.boxAt > 0 {
			boxAt, boxCol = used+t.boxAt, t.boxCol+treeGutter
		}
		used += lines
		block += "\n" + m.branch(t.block, i == len(owned)-1)
	}
	return rendered{block: block, stops: stops, boxAt: boxAt, boxCol: boxCol}
}

func (m Model) branch(block string, last bool) string {
	style := lipgloss.NewStyle().Foreground(m.theme.BorderMutedOrSubtle())
	down := style.Render("│ ")

	corner, under := style.Render("├─"), down
	if last {
		corner, under = style.Render("╰─"), "  "
	}

	lines := strings.Split(block, "\n")

	elbow := min(1, len(lines)-1)

	out := make([]string, 0, len(lines))
	for i, line := range lines {
		switch {
		case i == elbow:
			out = append(out, corner+line)
		case i < elbow:
			out = append(out, down+line)
		default:
			out = append(out, under+line)
		}
	}
	return strings.Join(out, "\n")
}

func (m Model) card(head, content string, width int, lit bool, hints string) string {
	pane := comp.NewPane(m.theme).Header(" " + head).Focus(lit)
	if lit {
		pane = pane.Footer(hints)
	}

	body := indent(content, cardGutter)
	lines := strings.Count(body, "\n") + 1
	return pane.Size(width, lines+pane.Chrome()).Render(body)
}

func (m Model) threadHints(lit bool, t gh.ReviewThread, width int) string {
	if !lit || m.boxOn(threadCommentKey(opening(t))) {
		return ""
	}
	k := keys.Detail

	if !m.threadOpen(t) {
		return hintLine(width, append([]string{k.Expand.Help().Key + " open"}, m.threadActs(t)...)...)
	}

	parts := m.answerParts(t, opening(t), !t.IsResolved)
	parts = append(parts, m.threadActs(t)...)
	if t.IsResolved {
		parts = append(parts, k.Expand.Help().Key+" close")
	}
	parts = append(parts, reactParts(opening(t).CanReact)...)
	return hintLine(width, append(parts, writeHints(opening(t))...)...)
}

func (m Model) replyHints(lit bool, t gh.ReviewThread, c gh.Comment, width int) string {
	if !lit || m.boxOn(threadCommentKey(c)) {
		return ""
	}
	parts := append(m.answerParts(t, c, true), reactParts(c.CanReact)...)
	return hintLine(width, append(parts, writeHints(c)...)...)
}

func (m Model) answerParts(t gh.ReviewThread, c gh.Comment, folds bool) []string {
	k := keys.Detail

	var parts []string
	if t.CanReply {
		parts = append(parts, k.Reply.Help().Key+" reply", k.QuoteReply.Help().Key+" quote")
	}
	if folds && foldable(c.Body) {
		parts = append(parts, k.Expand.Help().Key+" expand")
	}
	return parts
}

func reactParts(can bool) []string {
	if !can {
		return nil
	}
	return []string{keys.Detail.React.Help().Key + " react"}
}

func writeHints(c gh.Comment) []string {
	k := keys.Detail
	if !c.ViewerDidAuthor || c.Pending || c.Editing {
		return nil
	}

	var parts []string
	if c.CanEdit {
		parts = append(parts, k.Edit.Help().Key+" edit")
	}
	if c.CanDelete && c.Kind != gh.CommentReview {
		parts = append(parts, k.Delete.Help().Key+" delete")
	}
	return parts
}

func (m Model) cardHints(key focusKey, c gh.Comment, width int) string {
	lit := m.lit(key)
	if !lit || m.boxOn(key) {
		return ""
	}
	parts := append(m.quoteParts(c.Body), reactParts(c.CanReact)...)
	return hintLine(width, append(parts, writeHints(c)...)...)
}

func (m Model) descriptionHints(key focusKey, d gh.PullRequestDetail, width int) string {
	lit := m.lit(key)
	if !lit || m.boxOn(key) {
		return ""
	}

	parts := append(m.quoteParts(d.Body), reactParts(d.Viewer.CanReact)...)
	if m.ownDescription() {
		parts = append(parts, keys.Detail.Edit.Help().Key+" edit")
	}
	return hintLine(width, parts...)
}

func (m Model) threadActs(t gh.ReviewThread) []string {
	k := keys.Detail

	var parts []string
	if m.canToggleResolved(t) {
		word := " resolve"
		if t.IsResolved {
			word = " unresolve"
		}
		parts = append(parts, k.Resolve.Help().Key+word)
	}
	if m.jumpable(t) {
		parts = append(parts, k.Activate.Help().Key+" in diff")
	}
	return parts
}

func (m Model) quoteParts(body string) []string {
	k := keys.Detail

	parts := []string{k.QuoteReply.Help().Key + " quote"}
	if foldable(body) {
		parts = append(parts, k.Expand.Help().Key+" expand")
	}
	return parts
}

func foldable(body string) bool {
	return slices.ContainsFunc(comp.SplitDetails(body), func(seg comp.Segment) bool {
		return seg.Summary != ""
	})
}

// Sheds whole hints from the end, because the pane clips an overlong footer mid-word.
func hintLine(width int, parts ...string) string {
	room := hintRoom(width)
	for len(parts) > 0 {
		line := strings.Join(parts, " · ") + " "
		if lipgloss.Width(line) <= room {
			return line
		}
		parts = parts[:len(parts)-1]
	}
	return ""
}

func hintRoom(width int) int { return max(0, width-3) }

func (m Model) lit(key focusKey) bool {
	return m.focus == paneMain && m.mainRing().focused(key)
}

func (m Model) cardWidth(width int) int { return max(1, width-2-2*cardGutter) }

func (m *Model) markdown(text string, width int, key focusKey) string {
	if m.open[key] {
		return m.md.Render(text, width)
	}

	var out []string
	for _, seg := range comp.SplitDetails(text) {
		rendered := m.md.Render(seg.Text, width)
		if seg.Summary != "" {
			rendered = wrap(m.faint().Render("▸ "+seg.Summary+" · "+comp.Plural(seg.Lines, "line")), width)
		}
		if strings.TrimSpace(rendered) == "" {
			continue
		}
		out = append(out, rendered)
	}
	return strings.Join(out, "\n\n")
}

func (m *Model) description(d gh.PullRequestDetail, width int) rendered {
	key := focusKey{kind: focusDescription}
	head := m.said(d.Author, "opened this", m.theme.Subtle, gh.TimelineItem{CreatedAt: d.CreatedAt})
	if m.boxOn(key) {
		head = m.editHead("description")
	}

	content := m.bodyOrBox(d.Body, m.cardWidth(width), "No description.", key, boxChrome)
	block := m.card(head, m.withReactions(content, d.Reactions, key, m.cardWidth(width)), width,
		m.lit(key), m.descriptionHints(key, d, width))

	boxAt, boxCol := m.cardBoxAt(key, width, content)
	return rendered{
		block:  block,
		stops:  []focusItem{{focusKey: key, lines: strings.Count(block, "\n") + 1}},
		boxAt:  boxAt,
		boxCol: boxCol,
	}
}

func (m *Model) body(text string, width int, empty string, key focusKey) string {
	if out := m.markdown(text, width, key); strings.TrimSpace(out) != "" {
		return out
	}
	return m.faint().Render(empty)
}

func (m *Model) said(actor gh.Actor, verb string, c color.Color, item gh.TimelineItem) string {
	parts := make([]string, 0, 3)
	if actor.Login != "" {
		parts = append(parts, lipgloss.NewStyle().Foreground(m.theme.Actor).Render(actor.Login))
	}
	parts = append(parts, lipgloss.NewStyle().Foreground(c).Render(verb))
	if at := comp.RelativeTime(item.CreatedAt); at != "" {
		parts = append(parts, m.faint().Render(at))
	}
	return strings.Join(parts, m.faint().Render(" · "))
}

func (m *Model) pendingHead(actor gh.Actor, verb, doing string) string {
	return m.said(actor, verb, m.theme.Subtle, gh.TimelineItem{}) +
		m.faint().Render(" · ") +
		lipgloss.NewStyle().Foreground(m.theme.Warning).Render(doing)
}

func commitRun(items []gh.TimelineItem) []gh.TimelineItem {
	for i, item := range items {
		if item.Kind != gh.TimelineCommit {
			return items[:i]
		}
	}
	return items
}

func (m *Model) pushed(run []gh.TimelineItem) string {
	last := run[len(run)-1]

	verb := "pushed " + comp.Plural(len(run), "commit")
	if c := last.Commit; len(run) == 1 && c != nil {
		verb = "pushed " + c.Short + " " + c.Headline
	}

	actor := last.Actor
	for _, item := range run {
		if item.Actor != actor {
			actor = gh.Actor{}
			break
		}
	}

	lines := []string{wrap(m.faint().Render("● ")+m.said(actor, verb, m.theme.Subtle, last), m.bodyWidth())}
	if len(run) == 1 {
		return lines[0]
	}

	for _, item := range run {
		if item.Commit != nil {
			lines = append(lines, m.pushedRow(*item.Commit))
		}
	}
	return strings.Join(lines, "\n")
}

// Clips rather than wraps: a headline folded onto a second row reads as two commits.
func (m *Model) pushedRow(c gh.Commit) string {
	const indent = "    "

	sha := lipgloss.NewStyle().Foreground(m.theme.Accent).Render(c.Short)
	line := indent + sha + m.faint().Render("  "+c.Headline)

	if width := m.bodyWidth(); lipgloss.Width(line) > width {
		return paint.Clip(line, width, m.faint())
	}
	return line
}

func (m *Model) thread(t gh.ReviewThread, width int, hunk bool) rendered {
	anchor := t.Path
	if t.Line > 0 {
		anchor += ":" + strconv.Itoa(t.Line)
	}

	key := threadKey(t)

	lit := m.cursorOn(key) || m.lit(threadCommentKey(opening(t)))

	head := lipgloss.NewStyle().Foreground(m.theme.Text).Render(anchor)
	if t.IsResolved {
		head = m.faint().Render("✓ ") + head + m.faint().Render(" · resolved")
	}
	if t.IsOutdated {
		head += m.faint().Render(" · outdated")
	}

	if !m.threadOpen(t) {
		body := wrap(m.faint().Render("▸ "+comp.Plural(len(t.Comments), "comment")), m.cardWidth(width))
		block := m.card(head, body, width, lit, m.threadHints(lit, t, width))
		return rendered{block: block, stops: tile(block, []focusItem{{focusKey: key}})}
	}

	inner := m.cardWidth(width)

	var blocks []string
	at, boxAt, boxCol := 0, 0, 0
	push := func(block string) int {
		start := at
		blocks = append(blocks, block)
		at += strings.Count(block, "\n") + 2
		return start
	}

	if hunk {
		if code := m.threadHunk(t, inner); code != "" {
			push(code)
		}
	}

	if len(t.Comments) > 0 {
		opened := t.Comments[0]
		ck := threadCommentKey(opened)
		byline := wrap(m.byline(opened), inner)
		if m.boxOn(ck) {
			byline = wrap(m.editHead("comment"), inner)
		}
		body := m.bodyOrBox(opened.Body, inner, "No comment.", ck, boxChrome+threadChrome)
		start := push(byline + "\n\n" + m.withReactions(body, opened.Reactions, ck, inner))

		if m.boxOn(ck) {
			boxAt, boxCol = start+strings.Count(byline, "\n")+2, cardIndent
		}
	}

	content := strings.Join(blocks, "\n\n")
	block := m.card(head, content, width, lit, m.threadHints(lit, t, width))
	if boxAt > 0 {
		boxAt += m.cardLead(width, strings.Count(content, "\n")+1)
	}

	answers, stops := m.answers(t, width-treeGutter), []focusItem{{focusKey: key}}
	used := strings.Count(block, "\n") + 1
	for i, v := range answers {
		if v.boxAt > 0 {
			boxAt, boxCol = used+v.boxAt, v.boxCol+treeGutter
		}
		for _, s := range v.stops {
			stops = append(stops, focusItem{focusKey: s.focusKey, start: used + s.start})
		}
		used += strings.Count(v.block, "\n") + 1
		block += "\n" + m.branch(v.block, i == len(answers)-1)
	}

	return rendered{block: block, stops: tile(block, stops), boxAt: boxAt, boxCol: boxCol}
}

func opening(t gh.ReviewThread) gh.Comment {
	if len(t.Comments) == 0 {
		return gh.Comment{}
	}
	return t.Comments[0]
}

func (m *Model) answers(t gh.ReviewThread, width int) []rendered {
	var out []rendered
	if len(t.Comments) > 1 {
		for _, c := range t.Comments[1:] {
			v := m.replyCard(c, t, width)
			v.stops = []focusItem{{focusKey: threadCommentKey(c)}}
			out = append(out, v)
		}
	}

	if m.inline.at == replyKey(t.ID) {
		out = append(out, rendered{
			block:  m.writingCard(width),
			stops:  []focusItem{{focusKey: replyKey(t.ID)}},
			boxAt:  m.cardLead(width, replyRows+1),
			boxCol: cardIndent,
		})
	}
	return out
}

func (m *Model) replyCard(c gh.Comment, t gh.ReviewThread, width int) rendered {
	ck := threadCommentKey(c)
	lit := m.cursorOn(ck)
	hints := m.replyHints(lit, t, c, width)

	head := m.byline(c)
	if m.boxOn(ck) {
		head = m.editHead("comment")
	}

	body := m.bodyOrBox(c.Body, m.cardWidth(width), "No comment.", ck, boxChrome)
	v := rendered{block: m.card(head, m.withReactions(body, c.Reactions, ck, m.cardWidth(width)), width, lit, hints)}
	if m.boxOn(ck) {
		v.boxAt, v.boxCol = m.cardLead(width, strings.Count(body, "\n")+1), cardIndent
	}
	return v
}

type rendered struct {
	block string
	stops []focusItem

	// Zero means no box: a block opens on its border, so no box can land there.
	boxAt  int
	boxCol int
}

const cardIndent = 1 + cardGutter

func (m Model) cardLead(width, lines int) int {
	return comp.NewPane(m.theme).Header(" ").Size(width, lines+cardChrome).Above()
}

// comp.Pane's two borders, heading and rule around a card's content.
const cardChrome = 4

func (m Model) cardBoxAt(key focusKey, width int, content string) (at, col int) {
	if !m.boxOn(key) {
		return 0, 0
	}
	return m.cardLead(width, strings.Count(content, "\n")+1), cardIndent
}

func (m *Model) byline(c gh.Comment) string {
	switch {
	case c.Pending:
		return m.pendingHead(c.Author, "said", "posting")
	case c.Editing:
		return m.pendingHead(c.Author, "said", "saving")
	}
	return m.said(c.Author, "said", m.theme.Subtle, gh.TimelineItem{CreatedAt: c.CreatedAt})
}

func tile(block string, stops []focusItem) []focusItem {
	if len(stops) == 0 {
		return nil
	}

	total := strings.Count(block, "\n") + 1
	stops[0].start = 0
	for i := range stops {
		end := total
		if i+1 < len(stops) {
			end = stops[i+1].start
		}
		stops[i].lines = max(1, end-stops[i].start)
	}
	return stops
}

func (m *Model) threadHunk(t gh.ReviewThread, width int) string {
	if t.Hunk == nil || len(t.Hunk.Lines) == 0 {
		return ""
	}

	lines := t.Hunk.Lines
	if len(lines) > threadHunkLines {
		lines = lines[len(lines)-threadHunkLines:]
	}

	widestLine := 0
	for _, l := range lines {
		widestLine = max(widestLine, l.Old, l.New)
	}
	gutter := paint.Gutter(widestLine)

	tokens := m.syntax.Lines(t.Path, hunkSource(lines))
	out := make([]string, len(lines))
	for i, l := range lines {
		var row []syntax.Token
		if i < len(tokens) {
			row = tokens[i]
		}
		out[i] = m.diffRow(l, row, gutter, width).text
	}
	return strings.Join(out, "\n")
}

// Both sides go in together: a fragment this short is not valid source either way.
func hunkSource(lines []gh.DiffLine) string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.Content
	}
	return strings.Join(out, "\n")
}

func (m Model) faint() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(m.theme.Subtle)
}

func wrap(s string, width int) string {
	return lipgloss.NewStyle().Width(max(1, width)).Render(s)
}

func indent(s string, by int) string {
	pad := strings.Repeat(" ", by)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = pad + line
	}
	return strings.Join(lines, "\n")
}
