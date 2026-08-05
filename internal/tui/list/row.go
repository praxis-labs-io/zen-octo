package list

import (
	"image/color"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// Fixed column widths. The title takes what is left of the first line, up to a
// cap: past that it stops growing and the slack goes to a spacer, so the status
// glyphs stay the same distance from the right edge on a wide terminal.
//
// indentWidth is what puts the second line under the title rather than under
// the state glyph, so it is the first line's prefix and not a number of its own.
const (
	leftMargin  = 1
	rightMargin = 1
	stateWidth  = 2
	gutter      = 1
	indentWidth = leftMargin + stateWidth + gutter

	checksWidth = 2
	reviewWidth = 2
	ageWidth    = 4

	additionsWidth = 5
	deletionsWidth = 5
	filesWidth     = 5
	commentsWidth  = 5

	// The identity group is the number at its narrowest. Everything else on that
	// line is something it can do without.
	minIdentWidth = 6

	minTitleWidth = 12
	maxTitleWidth = 90
)

// The counts on the second line are marked by glyph rather than by a word, from
// the same Nerd Fonts ranges as the state badges.
const (
	glyphFiles    = "\uea7b" // nf-cod-file
	glyphComments = "\uf41f" // nf-oct-comment
)

// layout is which columns a width can carry, and how much of it the title gets.
// The two lines shed columns independently, because they hold different things
// and run out of room at different widths.
type layout struct {
	title  int
	slack  int
	review bool
	age    bool

	ident    int // width for the identity group, which flows rather than columns
	diff     bool
	files    bool
	comments bool
}

// fit drops columns until both lines fit the width they were given.
//
// Letting a line overflow instead means the pane clips it blind: the trailing
// columns vanish mid-cell with no ellipsis, and the selection background stops
// short of the edge because the line is no longer as wide as its pane.
//
// The first line gives up review before age, since age is the narrower of the
// two to keep. The second line drops the comment count, then the file count,
// then the churn, and gives what is left to the identity group, which sheds its
// own parts by content.
func fit(width int) layout {
	l := layout{review: true, age: true, diff: true, files: true, comments: true}

	tail := func() int {
		n := rightMargin + gutter + checksWidth
		if l.review {
			n += gutter + reviewWidth
		}
		if l.age {
			n += gutter + ageWidth
		}
		return n
	}

	for indentWidth+minTitleWidth+tail() > width {
		switch {
		case l.review:
			l.review = false
		case l.age:
			l.age = false
		default:
			// Nothing left to drop. The title takes what remains, which at this
			// point may be nothing at all.
			l.title = max(0, width-indentWidth-tail())
			l.diff, l.files, l.comments = false, false, false
			l.ident = max(0, width-indentWidth)
			return l
		}
	}

	avail := width - indentWidth - tail()
	l.title = min(avail, maxTitleWidth)
	l.slack = avail - l.title

	// The churn and the file count sit at the right edge, so the identity group
	// takes everything left over rather than capping and leaving a hole.
	metaTail := func() int {
		n := rightMargin
		if l.diff {
			n += gutter + additionsWidth + gutter + deletionsWidth
		}
		if l.files {
			n += gutter + filesWidth
		}
		if l.comments {
			n += gutter + commentsWidth
		}
		return n
	}

	for indentWidth+minIdentWidth+metaTail() > width {
		switch {
		case l.comments:
			l.comments = false
		case l.files:
			l.files = false
		case l.diff:
			l.diff = false
		default:
			l.ident = max(0, width-indentWidth)
			return l
		}
	}

	l.ident = width - indentWidth - metaTail()
	return l
}

// renderRow draws one pull request as its two lines: the title and its status
// on the first, everything that identifies it on the second.
//
// Selection is baked into every cell's own style, on both lines. Wrapping a
// joined line instead paints only its first cell: each cell ends in a full SGR
// reset, which clears the background along with the foreground.
func renderRow(th theme.Theme, pr gh.PullRequest, width int, selected bool) []string {
	l := fit(width)

	base := lipgloss.NewStyle()
	if selected {
		base = base.Background(th.SelectedBackground)
	}

	stateIcon, stateColor := comp.PRStateIcon(th, pr)
	checkIcon, checkColor := comp.CheckStateIcon(th, pr.Checks)
	reviewIcon, reviewColor := comp.ReviewIcon(th, pr.ReviewDecision)

	head := []string{
		cell(stateWidth, stateIcon, base.Foreground(stateColor)),
		cell(l.title, pr.Title, base.Foreground(th.Primary)) + base.Render(strings.Repeat(" ", l.slack)),
		cell(checksWidth, checkIcon, base.Foreground(checkColor)),
	}
	if l.review {
		head = append(head, cell(reviewWidth, reviewIcon, base.Foreground(reviewColor)))
	}
	if l.age {
		head = append(head, cell(ageWidth, alignRight(relativeTime(pr.UpdatedAt), ageWidth), base.Foreground(th.Faint)))
	}

	meta := []string{identity(th, pr, l.ident, base)}
	if l.diff {
		// Two cells rather than one string: additions and deletions carry their
		// own color, and a cell is one style all the way through.
		meta = append(meta,
			cell(additionsWidth, alignRight("+"+strconv.Itoa(pr.Additions), additionsWidth), base.Foreground(th.Success)),
			cell(deletionsWidth, alignRight("−"+strconv.Itoa(pr.Deletions), deletionsWidth), base.Foreground(th.Error)),
		)
	}
	if l.files {
		meta = append(meta, cell(filesWidth, counted(glyphFiles, pr.ChangedFiles, filesWidth), base.Foreground(th.Faint)))
	}
	if l.comments {
		meta = append(meta, cell(commentsWidth, counted(glyphComments, pr.Comments, commentsWidth), base.Foreground(th.Faint)))
	}

	return []string{
		line(leftMargin, head, width, base),
		line(indentWidth, meta, width, base),
	}
}

// counted is a glyph and its number. The glyph holds its column while the digits
// grow leftward from the right edge, so neither runs ragged down a screenful.
func counted(glyph string, n, width int) string {
	return glyph + " " + alignRight(strconv.Itoa(n), width-2)
}

// alignRight pads on the left, so a column of numbers lines up on its last
// digit rather than its first.
func alignRight(s string, width int) string {
	return strings.Repeat(" ", max(0, width-lipgloss.Width(s))) + s
}

// span is a run of text with its own color, for a column whose parts butt up
// against each other instead of sitting in fixed slots.
type span struct {
	text  string
	color color.Color
}

func spansWidth(spans []span) int {
	n := 0
	for _, s := range spans {
		n += lipgloss.Width(s.text)
	}
	return n
}

// identity names the pull request: repository, number, and who opened it, read
// as a phrase rather than three columns with gaps between them.
//
// It takes the widest form that fits. The number is what survives, because it
// is how you say which pull request you mean out loud.
func identity(th theme.Theme, pr gh.PullRequest, width int, base lipgloss.Style) string {
	number := span{text: "#" + strconv.Itoa(pr.Number), color: th.Faint}
	repo := span{text: pr.Repository + " ", color: th.Secondary}

	forms := [][]span{{number}, {repo, number}}
	// A deleted account has no login, and a dangling "by @" is worse than no
	// attribution at all.
	if pr.Author.Login != "" {
		forms = append(forms, []span{repo, number,
			{text: " by ", color: th.Faint},
			{text: "@" + pr.Author.Login, color: th.Actor},
		})
	}

	spans := forms[0]
	for _, form := range forms {
		if spansWidth(form) <= width {
			spans = form
		}
	}

	var out strings.Builder
	for _, s := range spans {
		out.WriteString(base.Foreground(s.color).Render(s.text))
	}

	switch w := spansWidth(spans); {
	case w < width:
		return out.String() + base.Render(strings.Repeat(" ", width-w))
	case w > width:
		return clip(out.String(), width)
	}
	return out.String()
}

// renderHeader draws a group's rule. It carries how many rows are under it,
// which is the count the section tab cannot show once the list is grouped.
func renderHeader(th theme.Theme, it item, width int) string {
	rule := lipgloss.NewStyle().Foreground(th.BorderFaintOrSecondary())

	left := rule.Render("─ ") +
		lipgloss.NewStyle().Foreground(th.Secondary).Bold(true).Render(it.header) + " " +
		lipgloss.NewStyle().Foreground(th.Faint).Render(strconv.Itoa(it.count)) + " "

	fill := max(0, width-lipgloss.Width(left))
	return lipgloss.NewStyle().MaxWidth(width).Render(left + rule.Render(strings.Repeat("─", fill)))
}

// line indents, joins with a gutter, then pads or clips to exactly width. Short
// and the selection background stops before the edge; long and the pane cuts it
// mid-cell with nothing saying it was cut.
func line(indent int, cells []string, width int, base lipgloss.Style) string {
	s := base.Render(strings.Repeat(" ", indent)) + strings.Join(cells, base.Render(strings.Repeat(" ", gutter)))

	switch w := lipgloss.Width(s); {
	case w < width:
		return s + base.Render(strings.Repeat(" ", width-w))
	case w > width:
		return clip(s, width)
	}
	return s
}

// cell pads to width and truncates anything longer, so columns stay aligned
// whatever the content.
//
// Truncation is explicit because Style.Width wraps first and only then clips,
// which turns one long title into two rows instead of one clipped one.
func cell(width int, content string, style lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(content) > width {
		content = clip(content, width)
	}
	pad := max(0, width-lipgloss.Width(content))
	return style.Render(content + strings.Repeat(" ", pad))
}

// clip truncates to width, marking the cut. A single column has room for the
// mark and nothing else, and MaxWidth(0) means no limit rather than no room.
func clip(content string, width int) string {
	switch {
	case width <= 0:
		return ""
	case width == 1:
		return "…"
	}
	return lipgloss.NewStyle().MaxWidth(width-1).Render(content) + "…"
}

// relativeTime renders a compact age: 34m, 5h, 12d, 3y. Anything in the future
// reads as "now" rather than a negative number.
func relativeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}

	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h"
	case d < 365*24*time.Hour:
		return strconv.Itoa(int(d.Hours()/24)) + "d"
	default:
		return strconv.Itoa(int(d.Hours()/24/365)) + "y"
	}
}
