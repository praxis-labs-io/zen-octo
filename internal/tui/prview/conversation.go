package prview

import (
	"image/color"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/store"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
)

// treeGutter is the rail hanging a review's threads off the review that opened
// them. GitHub draws a line down the side to say the same thing; two columns is
// what a terminal has to spend on it.
const treeGutter = 2

// cardGutter is the space between a card's border and what it holds. Text
// against the border reads as a rendering fault rather than as a box.
const cardGutter = 1

// threadHunkLines caps the context shown above a review comment. GitHub returns
// the whole hunk, which on a large change is a screenful, and the line the
// comment is about is the last one in it.
const threadHunkLines = 4

// cardHeadLines is what a card spends above its first line of content: the top
// border, the heading row, and the rule under it. comp.Pane.Chrome counts those
// three and the bottom border together, and a caller placing something inside a
// card needs the two numbers apart.
const cardHeadLines = 3

// conversationBody is the description and everything said since. It renders
// markdown, so it belongs on an Update path: m.md caches what it produces.
//
// A detail that has loaded once keeps rendering through a failed refetch. The
// root raises a toast for that; blanking the screen would be worse news than
// the news.
func (m *Model) conversationBody() string {
	// The ring is rebuilt from the blocks below. A tab that renders none of them
	// leaves nothing behind for tab to land on.
	m.convRing.reset()

	switch {
	case m.detail.Loaded:
		// Everything around the box is fixed while it is being written in, so a
		// freshly rendered box is joined between the two halves rather than the
		// page being rebuilt around one.
		if m.conv.ok && m.writing() != nil && m.conv.thread == m.reply.thread {
			return m.withBox(m.bodyWidth())
		}
		return m.entries()
	case m.detail.Status == store.StatusFailed:
		// Wrapped here rather than by the viewport. This tab does not soft wrap,
		// and an error is the one block on it not already measured to a width, so
		// it would be clipped at whatever the pane is wide: the reader would be
		// told the fetch failed and not told why.
		return wrap(m.faint().Render("Could not load the conversation: "+m.detail.Err.Error()), m.bodyWidth())
	}
	return m.spinner.Render("Loading the conversation")
}

func (m *Model) entries() string {
	d := m.detail.Detail
	width := m.bodyWidth()

	var blocks []string
	at := m.convRing.lead

	// split is the block the box being written in sits inside, and where it
	// begins. Everything before it is the head and everything after is the tail;
	// a keystroke rebuilds only what is between them.
	//
	// tailFrom is taken once the split block's own stops are recorded, not when
	// it is marked. Taken early it would leave every block between the box and
	// the end of the page out of the tail, and hand the split block's own stops
	// to a tail that gets them a second time from the rebuild.
	split, splitAt, headItems, tailFrom := -1, 0, 0, 0
	marking := false
	mark := func() { split, splitAt, headItems, marking = len(blocks), at, len(m.convRing.items), true }

	// pushStops records where a block landed before the join puts a blank line
	// under it. The starts come in relative to the block and go out absolute.
	pushStops := func(v rendered) {
		blocks = append(blocks, v.block)
		for _, s := range v.stops {
			m.convRing.add(s.focusKey, at+s.start, s.lines)
		}
		if marking {
			tailFrom, marking = len(m.convRing.items), false
		}
		at += strings.Count(v.block, "\n") + 2
	}

	// push is pushStops for the blocks carrying one stop or none. A key of no
	// kind is a block tab walks past: a merge or a run of commits is something
	// to read, not something to act on.
	push := func(block string, key focusKey) {
		var stops []focusItem
		if key.kind != focusNone {
			stops = []focusItem{{focusKey: key, lines: strings.Count(block, "\n") + 1}}
		}
		pushStops(rendered{block: block, stops: stops})
	}

	push(m.description(d, width), focusKey{kind: focusDescription})

	// A thread whose review never made this page would otherwise never render.
	// Whatever is left after the walk goes at the end rather than nowhere.
	shown := make(map[int]bool, len(d.Threads))

	for i := 0; i < len(d.Timeline); i++ {
		item := d.Timeline[i]
		switch item.Kind {
		case gh.TimelineComment:
			written := item.Said()
			key := focusKey{kind: focusComment, id: written.ID}
			head := m.said(item.Actor, "commented", m.theme.Faint, item)
			if written.Pending {
				head = m.posting(item.Actor, "commented")
			}
			push(m.card(head, m.body(written.Body, m.cardWidth(width), "No comment.", key), width, m.lit(key)), key)

		case gh.TimelineReview:
			// A review renders as its own card with every thread it opened hung
			// underneath, joined into one block. That block is what the box
			// splits the page at when the thread it is open on is one of them:
			// the branch gutter runs down the outside of all of them, and
			// cutting between two would mean splicing it back together.
			if m.replyUnder(d.Threads, item.Said().ID) {
				mark()
			}
			pushStops(m.review(item, d.Threads, shown, width))

		case gh.TimelineCommit:
			// A push arrives as one item per commit. They fold back into the one
			// line here rather than in the gh package, because how many rows a
			// run is worth is a rendering question and the Commits tab wants
			// them apart.
			run := commitRun(d.Timeline[i:])
			push(m.pushed(run), focusKey{})
			i += len(run) - 1

		default:
			// An event this build has no words for renders to nothing, and an
			// empty block still costs the blank line the join puts after it.
			if line := m.event(item); line != "" {
				push(line, focusKey{})
			}
		}
	}

	for i, thread := range d.Threads {
		if !shown[i] {
			if m.reply.thread == thread.ID {
				mark()
			}
			pushStops(m.thread(thread, width, true, m.replyBox(thread, width)))
		}
	}

	if n := d.MoreComments; n > 0 {
		push(wrap(m.faint().Render(comp.Plural(n, "older comment")+" on GitHub"), width), focusKey{})
	}
	if n := d.MoreThreads; n > 0 {
		push(wrap(m.faint().Render(comp.Plural(n, "more review thread")+" on GitHub"), width), focusKey{})
	}

	// The compose card is the last block, always on the page, and the split when
	// no reply box took it. A reply open on a thread this page does not render
	// falls here too: there is no box among the blocks to cut at.
	if split < 0 {
		mark()
	}
	pushStops(m.composeCard(width))

	m.conv = convCache{
		head:      strings.Join(blocks[:split], "\n\n"),
		tail:      strings.Join(blocks[split+1:], "\n\n"),
		items:     slices.Clone(m.convRing.items[:headItems]),
		tailItems: shifted(m.convRing.items[tailFrom:], splitAt+strings.Count(blocks[split], "\n")+2),
		at:        splitAt,
		thread:    m.reply.thread,
		ok:        true,
	}
	return strings.Join(blocks, "\n\n")
}

// convCache is the conversation around the box being written in: everything
// above it and everything below, each built once.
//
// While the box has the keyboard neither half can change: the detail is not
// being refetched, the width is not moving, and the keys are all going into a
// textarea. So a keystroke re-renders the one block the box sits in and joins it
// between two strings it already has. On a hundred-and-forty-comment thread that
// is the difference between re-bordering a hundred and forty cards per character
// and re-bordering one: 27ms against 7ms, which is the difference between typing
// and waiting.
//
// tailItems are relative to the tail's own first line, where items are absolute.
// Absolute would work today, since a box is a fixed height whatever is typed
// into it, but it would make a silent invariant out of a fact that has to be
// re-proved every time the box changes shape.
//
// It is only ever read while typing, and everything that can invalidate it ends
// typing or cannot happen during it. Anything else added here has to clear ok,
// or the page will render a card that has moved on.
type convCache struct {
	head      string
	tail      string
	items     []focusItem
	tailItems []focusItem

	// at is the line the middle block starts on.
	at int

	// thread is the review thread the box is open on, empty for the compose
	// card. A cache built around one box cannot be joined around another.
	thread string

	ok bool
}

// shifted moves a run of ring items to be relative to a line.
func shifted(items []focusItem, by int) []focusItem {
	out := slices.Clone(items)
	for i := range out {
		out[i].start -= by
	}
	return out
}

// withBox joins a freshly rendered box between the two halves already built, and
// rebuilds the ring around it.
func (m *Model) withBox(width int) string {
	m.convRing.items = append(m.convRing.items[:0], m.conv.items...)

	middle := m.boxBlock(width)
	for _, s := range middle.stops {
		m.convRing.add(s.focusKey, m.conv.at+s.start, s.lines)
	}

	base := m.conv.at + strings.Count(middle.block, "\n") + 2
	for _, it := range m.conv.tailItems {
		m.convRing.add(it.focusKey, base+it.start, it.lines)
	}

	return joinBlocks(m.conv.head, middle.block, m.conv.tail)
}

// boxBlock re-renders the one block the box sits in. It is found by identity
// rather than by the place it had, for the same reason the ring is: nothing on
// this page is addressed by where it landed.
func (m *Model) boxBlock(width int) rendered {
	if m.conv.thread == "" {
		return m.composeCard(width)
	}

	d := m.detail.Detail
	for _, item := range d.Timeline {
		if item.Kind != gh.TimelineReview || !m.replyUnder(d.Threads, item.Said().ID) {
			continue
		}
		return m.review(item, d.Threads, make(map[int]bool, len(d.Threads)), width)
	}

	for _, t := range d.Threads {
		if t.ID == m.conv.thread {
			return m.thread(t, width, true, m.replyBox(t, width))
		}
	}

	// The thread went away under an open box. Nothing to draw where it was, and
	// the halves either side still join.
	return rendered{}
}

// replyUnder is whether the box is open on a thread this review opened.
func (m Model) replyUnder(threads []gh.ReviewThread, reviewID string) bool {
	if m.reply.thread == "" || reviewID == "" {
		return false
	}
	return slices.ContainsFunc(threads, func(t gh.ReviewThread) bool {
		return t.ID == m.reply.thread && t.ReviewID == reviewID
	})
}

// joinBlocks joins what is there, so an empty half does not cost a blank line.
func joinBlocks(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "\n\n")
}

// review is the verdict and body in a box, then the threads it opened, set in
// under it. The box is what stops a bot review that runs for forty lines
// reading as loose comments with no telling where it ends.
func (m *Model) review(item gh.TimelineItem, threads []gh.ReviewThread, shown map[int]bool, width int) rendered {
	label, c := comp.ReviewStateLabel(m.theme, item.Review)
	head := m.said(item.Actor, label, c, item)

	written := item.Said()
	key := focusKey{kind: focusReview, id: written.ID}
	block := m.card(head, m.body(written.Body, m.cardWidth(width), "No comment.", key), width, m.lit(key))

	used := strings.Count(block, "\n") + 1
	stops := []focusItem{{focusKey: key, lines: used}}

	var owned []rendered
	for i, thread := range threads {
		if thread.ReviewID != written.ID || thread.ReviewID == "" {
			continue
		}
		shown[i] = true
		inner := width - treeGutter
		owned = append(owned, m.thread(thread, inner, true, m.replyBox(thread, inner)))
	}

	for i, t := range owned {
		lines := strings.Count(t.block, "\n") + 1
		// The rail opens with a line of its own above the thread's first, so the
		// thread starts one below where the branch does.
		for _, s := range t.stops {
			stops = append(stops, focusItem{focusKey: s.focusKey, start: used + 1 + s.start, lines: s.lines})
		}
		used += lines + 1
		block += "\n" + m.branch(t.block, i == len(owned)-1)
	}
	return rendered{block: block, stops: stops}
}

// branch hangs one thread off the review above it. The last closes the run, so
// the rail stops rather than trailing into whatever comes next.
func (m Model) branch(block string, last bool) string {
	style := lipgloss.NewStyle().Foreground(m.theme.BorderFaintOrSecondary())
	down := style.Render("│ ")

	corner, under := style.Render("├─"), down
	if last {
		corner, under = style.Render("╰─"), "  "
	}

	lines := strings.Split(block, "\n")

	// The elbow meets the card's heading row rather than its top border, which
	// is where GitHub joins the two as well. A resolved thread is one line and
	// has no heading to meet.
	elbow := min(1, len(lines)-1)

	out := make([]string, 0, len(lines)+1)
	out = append(out, down)
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

// card is one entry: a heading row, a rule, then what was written. The heading
// is already styled piece by piece, so the pane takes it as-is.
//
// The gutter is the caller's rather than the pane's, because the rail already
// indents its own entries and would end up with two.
//
// A focused card takes its border in the accent, the same signal the panes
// around it already use for where the keys go.
func (m Model) card(head, content string, width int, lit bool) string {
	pane := comp.NewPane(m.theme).Header(" " + head).Focus(lit)
	body := indent(content, cardGutter)
	lines := strings.Count(body, "\n") + 1
	return pane.Size(width, lines+pane.Chrome()).Render(body)
}

// lit is whether a block holds the conversation's focus. A card is only lit on
// the pane the keys are going to, which is neither the Files tab, where the
// same threads render under a ring tab does not walk, nor the conversation
// while the rail has focus. A card lit on a pane the key does nothing to is a
// lie about the key, and two panes lit at once is the same lie twice.
func (m Model) lit(key focusKey) bool {
	return m.railTab() && m.focus == paneMain && m.convRing.focused(key)
}

// litAny is lit for a card holding more than one focusable thing. A thread card
// takes the accent for whichever of its comments the ring is on.
func (m Model) litAny(stops []focusItem) bool {
	return slices.ContainsFunc(stops, func(it focusItem) bool { return m.lit(it.focusKey) })
}

// cardWidth is what is left for text once the box has taken its sides and its
// gutter.
func (m Model) cardWidth(width int) int { return max(1, width-2-2*cardGutter) }

// markdown renders a body, folding every <details> block to the line that
// stands for it. GitHub collapses them in the browser for the same reason: a
// bot review pastes a table of every file it looked at, and it is never the
// thing you opened the pull request to read.
func (m *Model) markdown(text string, width int, key focusKey) string {
	if m.expanded[key] {
		return m.md.Render(text, width)
	}

	var out []string
	for _, seg := range comp.SplitDetails(text) {
		rendered := m.md.Render(seg.Text, width)
		if seg.Summary != "" {
			rendered = wrap(m.faint().Render("▸ "+seg.Summary+" · "+comp.Plural(seg.Lines, "line")), width)
		}
		// A segment that renders to nothing still costs the blank line the join
		// puts after it. Bot comments open with a hidden marker often enough
		// that the gap shows up as a hole under the heading.
		if strings.TrimSpace(rendered) == "" {
			continue
		}
		out = append(out, rendered)
	}
	return strings.Join(out, "\n\n")
}

func (m *Model) description(d gh.PullRequestDetail, width int) string {
	key := focusKey{kind: focusDescription}
	head := m.said(d.Author, "opened this", m.theme.Faint, gh.TimelineItem{CreatedAt: d.CreatedAt})
	return m.card(head, m.body(d.Body, m.cardWidth(width), "No description.", key), width, m.lit(key))
}

// body renders markdown, falling back to a note rather than a hole in the page.
func (m *Model) body(text string, width int, empty string, key focusKey) string {
	if out := m.markdown(text, width, key); strings.TrimSpace(out) != "" {
		return out
	}
	return m.faint().Render(empty)
}

// said is the line above a block of writing: who, what they did, and when. A
// deleted account has no login, so the verb carries the line on its own.
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

// posting is the heading over a comment on its way. It says so where the time
// would go: a card that has landed and one that still might not read the same
// otherwise, and only one of the two can disappear.
//
// The item is empty so said writes no time. There is none to write, and "now"
// would be a claim about a comment GitHub has not seen.
func (m *Model) posting(actor gh.Actor, verb string) string {
	return m.said(actor, verb, m.theme.Faint, gh.TimelineItem{}) +
		m.faint().Render(" · ") +
		lipgloss.NewStyle().Foreground(m.theme.Warning).Render("posting")
}

// event is one line. Nobody reads a merge twice, and giving it the same block
// treatment as a comment buries the discussion between them.
func (m *Model) event(item gh.TimelineItem) string {
	label, ok := eventLabels[item.Kind]
	if !ok {
		return ""
	}
	return wrap(m.faint().Render("● ")+m.said(item.Actor, label, m.theme.Faint, item), m.bodyWidth())
}

// commitRun is the commits that landed together, from the head of a timeline.
func commitRun(items []gh.TimelineItem) []gh.TimelineItem {
	for i, item := range items {
		if item.Kind != gh.TimelineCommit {
			return items[:i]
		}
	}
	return items
}

// pushed is a run of commits that landed together: a line saying how many and
// who, then the run itself, one commit to a row. A lone commit names its sha
// and headline on the line and has no rows under it.
//
// The rows are what makes the branch readable from the conversation. A long
// rebase is a long block, which is the honest shape of a long rebase; the
// alternative hides work that happened between two comments.
//
// A run written by one person names them. A mixed one drops the login rather
// than crediting the wrong author, and said carries the line without it.
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

	lines := []string{wrap(m.faint().Render("● ")+m.said(actor, verb, m.theme.Faint, last), m.bodyWidth())}
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

// pushedRow is one commit under a push. It is set in past the marker above it,
// and clips rather than wrapping: a headline folded onto a second row reads as
// two commits.
func (m *Model) pushedRow(c gh.Commit) string {
	const indent = "    "

	sha := lipgloss.NewStyle().Foreground(m.theme.Secondary).Render(c.Short)
	line := indent + sha + m.faint().Render("  "+c.Headline)

	if width := m.bodyWidth(); lipgloss.Width(line) > width {
		return comp.Clip(line, width, m.faint())
	}
	return line
}

var eventLabels = map[gh.TimelineKind]string{
	gh.TimelineMerged:         "merged this",
	gh.TimelineClosed:         "closed this",
	gh.TimelineReopened:       "reopened this",
	gh.TimelineReadyForReview: "marked this ready for review",
	gh.TimelineDraft:          "converted this to a draft",
	gh.TimelineForcePushed:    "force-pushed",
}

// thread renders a line-anchored discussion in a box of its own, its anchor in
// the top border, which is where GitHub puts the file name too.
//
// A resolved one collapses to a single line instead. GitHub hides them by
// default, and on a heavily reviewed pull request the settled nits bury the
// live ones.
//
// hunk asks for the code the thread was written against. The conversation
// wants it: a comment about a line nobody can see is an assertion about
// nothing. The Files tab does not, because the line is already on the screen.
func (m *Model) thread(t gh.ReviewThread, width int, hunk bool, reply string) rendered {
	anchor := t.Path
	if t.Line > 0 {
		anchor += ":" + strconv.Itoa(t.Line)
	}

	if t.IsResolved {
		key := threadKey(t)

		// One line has no border to take the accent, so the text carries it.
		style := m.faint()
		if m.lit(key) {
			style = lipgloss.NewStyle().Foreground(m.theme.Secondary)
		}
		block := wrap(style.Render("✓ "+anchor+" · resolved · "+
			comp.Plural(len(t.Comments), "comment")), width)
		return rendered{block: block, stops: tile(block, []focusItem{{focusKey: key}})}
	}

	head := lipgloss.NewStyle().Foreground(m.theme.Primary).Render(anchor)
	if t.IsOutdated {
		head += m.faint().Render(" · outdated")
	}

	inner := m.cardWidth(width)

	// at is the content line the next block will start on. The join puts one
	// blank line between blocks, so a block costs its own lines plus that.
	var blocks []string
	at := 0
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

	stops := make([]focusItem, 0, len(t.Comments)+1)
	for _, c := range t.Comments {
		key := threadCommentKey(c)
		stops = append(stops, focusItem{
			focusKey: key,
			start:    push(wrap(m.byline(c, key), inner) + "\n\n" + m.body(c.Body, inner, "No comment.", key)),
		})
	}

	if reply != "" {
		stops = append(stops, focusItem{focusKey: m.replyFocus(), start: push(reply)})
	}

	block := m.card(head, strings.Join(blocks, "\n\n"), width, m.litAny(stops))
	for i := range stops {
		stops[i].start += cardHeadLines
	}
	return rendered{block: block, stops: tile(block, stops)}
}

// rendered is one block of the conversation and where each of its own ring
// stops landed inside it. The starts are relative to the block's first line: a
// block does not know where it was placed, and every caller that places one
// does.
type rendered struct {
	block string
	stops []focusItem
}

// byline is the line above one comment in a thread. A focused one takes the
// accent on its verb, because a comment inside a card has no border of its own
// and the card is already lit for the thread as a whole.
func (m *Model) byline(c gh.Comment, key focusKey) string {
	if c.Pending {
		return m.posting(c.Author, "said")
	}

	verb := m.theme.Faint
	if m.lit(key) {
		verb = m.theme.Secondary
	}
	return m.said(c.Author, "said", verb, gh.TimelineItem{CreatedAt: c.CreatedAt})
}

// tile spreads stops over every line of the block they came out of: the first
// takes everything above it, each one runs to the next, and the last runs to the
// end.
//
// The gaps are not spare. A stop that covered only its own text would leave the
// card's borders and heading belonging to nothing, and the ring answers two
// questions off these numbers: whether the focus is still on screen, and where
// to scroll to bring it back. Both want the first stop to carry the top border,
// or scrolling to the first comment in a thread cuts the anchor off above it.
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

// threadHunk is the tail of the diff the thread hangs off, rendered the same
// way the Files tab renders one. Only the last few lines: GitHub returns up to
// a screenful of leading context, and the line the comment is about is the last
// one.
func (m *Model) threadHunk(t gh.ReviewThread, width int) string {
	if t.Hunk == nil || len(t.Hunk.Lines) == 0 {
		return ""
	}

	lines := t.Hunk.Lines
	if len(lines) > threadHunkLines {
		lines = lines[len(lines)-threadHunkLines:]
	}

	gutter := gutterMin
	for _, l := range lines {
		gutter = max(gutter, len(strconv.Itoa(max(l.Old, l.New))))
	}

	tokens := m.syntax.Lines(t.Path, hunkSource(lines))
	out := make([]string, len(lines))
	for i, l := range lines {
		var row []comp.Token
		if i < len(tokens) {
			row = tokens[i]
		}
		out[i] = m.diffLine(l, row, gutter, width)
	}
	return strings.Join(out, "\n")
}

// hunkSource is the code behind a hunk, for the lexer. Both sides go in
// together here: a fragment this short is not valid source either way, and
// splitting it would leave the removed line with no context at all.
func hunkSource(lines []gh.DiffLine) string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.Content
	}
	return strings.Join(out, "\n")
}

func (m Model) faint() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(m.theme.Faint)
}

// wrap breaks a line at a width. Only markdown comes back already wrapped;
// every line this file builds by hand would otherwise be clipped by the card
// around it, or run the full width of a wide terminal past the measure.
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
