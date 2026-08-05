package list

import (
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
// indentWidth puts the second line under the number, which is where the eye
// already is. headWidth is everything on the first line before the title.
const (
	leftMargin  = 1
	rightMargin = 1
	stateWidth  = 2
	gutter      = 1
	indentWidth = leftMargin + stateWidth + gutter

	numberWidth = 6
	headWidth   = leftMargin + stateWidth + gutter + numberWidth + gutter

	checksWidth = 2
	reviewWidth = 2

	additionsWidth = 5
	deletionsWidth = 5
	filesWidth     = 5

	// Below this the repository is more ellipsis than name, so the counts give
	// up their columns first.
	minIdentWidth = 8

	minTitleWidth = 12
	maxTitleWidth = 90
)

// The counts are marked by glyph rather than by a word, from the same Nerd
// Fonts ranges as the state badges.
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

	ident int // width for the identity group, which flows rather than columns
	diff  bool
	files bool
}

// fit drops columns until both lines fit the width they were given.
//
// Letting a line overflow instead means the pane clips it blind: the trailing
// columns vanish mid-cell with no ellipsis, and the selection background stops
// short of the edge because the line is no longer as wide as its pane.
//
// The first line has only review to give before the title starts clipping. The
// second drops the file count, then the churn, and gives what is left to the
// identity group, which sheds its own parts by content.
func fit(width int) layout {
	l := layout{review: true, diff: true, files: true}

	tail := func() int {
		n := rightMargin + gutter + checksWidth
		if l.review {
			n += gutter + reviewWidth
		}
		return n
	}

	for headWidth+minTitleWidth+tail() > width {
		switch {
		case l.review:
			l.review = false
		default:
			// Nothing left to drop. The title takes what remains, which at this
			// point may be nothing at all.
			l.title = max(0, width-headWidth-tail())
			l.diff, l.files = false, false
			l.ident = max(0, width-indentWidth)
			return l
		}
	}

	avail := width - headWidth - tail()
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
		return n
	}

	for indentWidth+minIdentWidth+metaTail() > width {
		switch {
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
// on the first, everything that identifies it on the second, with the blank
// line that separates it from the next row under those.
//
// Selection is baked into every cell's own style, on both lines. Wrapping a
// joined line instead paints only its first cell: each cell ends in a full SGR
// reset, which clears the background along with the foreground.
func renderRow(th theme.Theme, it item, width int, selected bool) []string {
	pr, l := it.pr, fit(width)

	base := lipgloss.NewStyle()
	if selected {
		base = base.Background(th.SelectedBackground)
	}

	stateIcon, stateColor := comp.PRStateIcon(th, pr)
	checkIcon, checkColor := comp.CheckStateIcon(th, pr.Checks)
	reviewIcon, reviewColor := comp.ReviewIcon(th, pr.ReviewDecision)

	head := []string{
		cell(stateWidth, stateIcon, base.Foreground(stateColor)),
		cell(numberWidth, "#"+strconv.Itoa(pr.Number), base.Foreground(th.Secondary)),
		titled(th, pr, l.title, base) + base.Render(strings.Repeat(" ", l.slack)),
	}
	// Review sits inside checks, and both glyphs sit at the right of their cell.
	// Checks is the one nearly every pull request fills, so putting it outermost
	// is what keeps the edge of the line from being blank on most rows, and the
	// alignment is what makes it end where the counts below it do.
	if l.review {
		head = append(head, cell(reviewWidth, alignRight(reviewIcon, reviewWidth), base.Foreground(reviewColor)))
	}
	head = append(head, cell(checksWidth, alignRight(checkIcon, checksWidth), base.Foreground(checkColor)))

	tail := []string{identity(th, pr, l.ident, base)}
	if l.diff {
		// Two cells rather than one string: additions and deletions carry their
		// own color, and a cell is one style all the way through.
		tail = append(tail,
			cell(additionsWidth, alignRight("+"+strconv.Itoa(pr.Additions), additionsWidth), base.Foreground(th.Success)),
			cell(deletionsWidth, alignRight("−"+strconv.Itoa(pr.Deletions), deletionsWidth), base.Foreground(th.Error)),
		)
	}
	if l.files {
		tail = append(tail, cell(filesWidth, counted(glyphFiles, pr.ChangedFiles, filesWidth), base.Foreground(th.Faint)))
	}

	lines := []string{
		line(leftMargin, head, width, base),
		line(indentWidth, tail, width, base),
	}
	if it.blankBelow {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return lines
}

// counted is a glyph and its number with nothing between them, pushed to the
// right of its column so a screenful lines up on the last digit.
//
// Nothing to count renders as nothing at all: an icon next to a zero reads as a
// reading worth noticing. The column stays either way, so the rows still line up.
func counted(glyph string, n, width int) string {
	if n == 0 {
		return ""
	}
	return alignRight(glyph+strconv.Itoa(n), width)
}

// alignRight pads on the left, so a column of numbers lines up on its last
// digit rather than its first.
func alignRight(s string, width int) string {
	return strings.Repeat(" ", max(0, width-lipgloss.Width(s))) + s
}

// titled is the title with its comment count trailing it, the way the identity
// line carries the age after the author. The count sits where the title ends
// rather than in a column of its own.
//
// It keeps its room out of the title's: a clipped title still says how much
// discussion is on it. Only a column with nothing left for the title itself
// gives the count up.
func titled(th theme.Theme, pr gh.PullRequest, width int, base lipgloss.Style) string {
	title := base.Foreground(th.Primary)
	count := glyphComments + " " + strconv.Itoa(pr.Comments)

	room := width - lipgloss.Width(count) - 2
	if pr.Comments == 0 || room < 1 {
		return cell(width, pr.Title, title)
	}

	text := pr.Title
	if lipgloss.Width(text) > room {
		text = clip(text, room)
	}
	pad := width - lipgloss.Width(text) - lipgloss.Width(count) - 2

	return title.Render(text) + base.Render("  ") +
		base.Foreground(th.Secondary).Render(count) + base.Render(strings.Repeat(" ", pad))
}

// identity is where the pull request lives, who opened it, and when it last
// moved, read as a phrase rather than three columns with gaps between them. The
// number is on the line above, next to the state glyph.
//
// It takes the widest form that fits. The author is the first thing to go: it
// is the widest, and on your own sections it is the same name every row. A
// deleted account has no login, where a dangling "by @" would be worse than no
// attribution at all.
func identity(th theme.Theme, pr gh.PullRequest, width int, base lipgloss.Style) string {
	age := ""
	if at := relativeTime(pr.UpdatedAt); at != "" {
		age = " · " + at
	}

	forms := []string{pr.Repository, pr.Repository + age}
	if pr.Author.Login != "" {
		forms = append(forms, pr.Repository+" by @"+pr.Author.Login+age)
	}

	text := forms[0]
	for _, form := range forms {
		if lipgloss.Width(form) <= width {
			text = form
		}
	}
	return cell(width, text, base.Foreground(th.Faint))
}

// renderHeader draws a group's rule, with the gap above it that keeps the
// groups apart. It carries how many rows are under it, which is the count
// the section tab cannot show once the list is grouped.
func renderHeader(th theme.Theme, it item, width int) []string {
	rule := lipgloss.NewStyle().Foreground(th.BorderFaintOrSecondary())

	left := rule.Render("─ ") +
		lipgloss.NewStyle().Foreground(th.Secondary).Bold(true).Render(it.header) + " " +
		lipgloss.NewStyle().Foreground(th.Faint).Render(strconv.Itoa(it.count)) + " "

	fill := max(0, width-lipgloss.Width(left))
	rendered := lipgloss.NewStyle().MaxWidth(width).Render(left + rule.Render(strings.Repeat("─", fill)))

	lines := make([]string, it.gapAbove, it.gapAbove+1)
	for i := range lines {
		lines[i] = strings.Repeat(" ", width)
	}
	return append(lines, rendered)
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
