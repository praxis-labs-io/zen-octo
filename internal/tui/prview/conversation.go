package prview

import (
	"image/color"
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

// conversationBody is the description and everything said since. It renders
// markdown, so it belongs on an Update path: m.md caches what it produces.
//
// A detail that has loaded once keeps rendering through a failed refetch. The
// root raises a toast for that; blanking the screen would be worse news than
// the news.
func (m *Model) conversationBody() string {
	switch {
	case m.detail.Loaded:
		return m.entries()
	case m.detail.Status == store.StatusFailed:
		return m.faint().Render("Could not load the conversation: " + m.detail.Err.Error())
	}
	return m.spinner.Render("Loading the conversation")
}

func (m *Model) entries() string {
	d := m.detail.Detail
	width := m.bodyWidth()

	blocks := []string{m.description(d, width)}

	// A thread whose review never made this page would otherwise never render.
	// Whatever is left after the walk goes at the end rather than nowhere.
	shown := make(map[int]bool, len(d.Threads))

	for _, item := range d.Timeline {
		switch item.Kind {
		case gh.TimelineComment:
			head := m.said(item.Actor, "commented", m.theme.Faint, item)
			blocks = append(blocks, m.card(head, m.body(item.Body, m.cardWidth(width), "No comment."), width))

		case gh.TimelineReview:
			blocks = append(blocks, m.review(item, d.Threads, shown, width))

		default:
			// An event this build has no words for renders to nothing, and an
			// empty block still costs the blank line the join puts after it.
			if line := m.event(item); line != "" {
				blocks = append(blocks, line)
			}
		}
	}

	for i, thread := range d.Threads {
		if !shown[i] {
			blocks = append(blocks, m.thread(thread, width))
		}
	}

	if n := d.MoreComments; n > 0 {
		blocks = append(blocks, wrap(m.faint().Render(comp.Plural(n, "older comment")+" on GitHub"), width))
	}
	if n := d.MoreThreads; n > 0 {
		blocks = append(blocks, wrap(m.faint().Render(comp.Plural(n, "more review thread")+" on GitHub"), width))
	}

	return strings.Join(blocks, "\n\n")
}

// review is the verdict and body in a box, then the threads it opened, set in
// under it. The box is what stops a bot review that runs for forty lines
// reading as loose comments with no telling where it ends.
func (m *Model) review(item gh.TimelineItem, threads []gh.ReviewThread, shown map[int]bool, width int) string {
	label, c := comp.ReviewStateLabel(m.theme, item.Review)
	head := m.said(item.Actor, label, c, item)

	block := m.card(head, m.body(item.Body, m.cardWidth(width), "No comment."), width)

	var owned []string
	for i, thread := range threads {
		if thread.ReviewID != item.ID || thread.ReviewID == "" {
			continue
		}
		shown[i] = true
		owned = append(owned, m.thread(thread, width-treeGutter))
	}
	for i, thread := range owned {
		block += "\n" + m.branch(thread, i == len(owned)-1)
	}
	return block
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
func (m Model) card(head, content string, width int) string {
	pane := comp.NewPane(m.theme).Header(" " + head)
	body := indent(content, cardGutter)
	lines := strings.Count(body, "\n") + 1
	return pane.Size(width, lines+pane.Chrome()).Render(body)
}

// cardWidth is what is left for text once the box has taken its sides and its
// gutter.
func (m Model) cardWidth(width int) int { return max(1, width-2-2*cardGutter) }

// markdown renders a body, folding every <details> block to the line that
// stands for it. GitHub collapses them in the browser for the same reason: a
// bot review pastes a table of every file it looked at, and it is never the
// thing you opened the pull request to read.
func (m *Model) markdown(text string, width int) string {
	if m.expanded {
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
	head := m.said(d.Author, "opened this", m.theme.Faint, gh.TimelineItem{CreatedAt: d.CreatedAt})
	return m.card(head, m.body(d.Body, m.cardWidth(width), "No description."), width)
}

// body renders markdown, falling back to a note rather than a hole in the page.
func (m *Model) body(text string, width int, empty string) string {
	if out := m.markdown(text, width); strings.TrimSpace(out) != "" {
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

// event is one line. Nobody reads a merge twice, and giving it the same block
// treatment as a comment buries the discussion between them.
func (m *Model) event(item gh.TimelineItem) string {
	label, ok := eventLabels[item.Kind]
	if !ok {
		return ""
	}
	return wrap(m.faint().Render("● ")+m.said(item.Actor, label, m.theme.Faint, item), m.bodyWidth())
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
func (m *Model) thread(t gh.ReviewThread, width int) string {
	anchor := t.Path
	if t.Line > 0 {
		anchor += ":" + strconv.Itoa(t.Line)
	}

	if t.IsResolved {
		return wrap(m.faint().Render("✓ "+anchor+" · resolved · "+
			comp.Plural(len(t.Comments), "comment")), width)
	}

	head := lipgloss.NewStyle().Foreground(m.theme.Primary).Render(anchor)
	if t.IsOutdated {
		head += m.faint().Render(" · outdated")
	}

	inner := m.cardWidth(width)
	blocks := make([]string, 0, len(t.Comments))
	for _, c := range t.Comments {
		said := m.said(c.Author, "said", m.theme.Faint, gh.TimelineItem{CreatedAt: c.CreatedAt})
		blocks = append(blocks, wrap(said, inner)+"\n\n"+m.body(c.Body, inner, "No comment."))
	}

	return m.card(head, strings.Join(blocks, "\n\n"), width)
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
