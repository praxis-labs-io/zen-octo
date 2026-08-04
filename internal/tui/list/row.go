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
// indentWidth is what puts the second line under the title rather than under
// the state glyph, so it is the first line's prefix and not a number of its own.
const (
	leftMargin  = 1
	stateWidth  = 2
	gutter      = 1
	indentWidth = leftMargin + stateWidth + gutter

	checksWidth = 2
	reviewWidth = 2
	ageWidth    = 4

	numberWidth = 6
	repoWidth   = 22
	authorWidth = 14
	diffWidth   = 10
	filesWidth  = 8

	minTitleWidth = 12
	maxTitleWidth = 90
)

// layout is which columns a width can carry, and how much of it the title gets.
// The two lines shed columns independently, because they hold different things
// and run out of room at different widths.
type layout struct {
	title  int
	slack  int
	review bool
	age    bool

	repo   bool
	author bool
	diff   bool
	files  bool
}

// fit drops columns until both lines fit the width they were given.
//
// Letting a line overflow instead means the pane clips it blind: the trailing
// columns vanish mid-cell with no ellipsis, and the selection background stops
// short of the edge because the line is no longer as wide as its pane.
//
// The first line gives up review before age, since age is the narrower of the
// two to keep. The second line goes widest-first from its end: files, diff
// stat, author, then repo. The number stays whatever happens, because it is how
// you name the thing out loud.
func fit(width int) layout {
	l := layout{review: true, age: true, repo: true, author: true, diff: true, files: true}

	tail := func() int {
		n := gutter + checksWidth
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
			l.repo, l.author, l.diff, l.files = false, false, false, false
			return l
		}
	}

	avail := width - indentWidth - tail()
	l.title = min(avail, maxTitleWidth)
	l.slack = avail - l.title

	meta := func() int {
		n := indentWidth + numberWidth
		if l.repo {
			n += gutter + repoWidth
		}
		if l.author {
			n += gutter + authorWidth
		}
		if l.diff {
			n += gutter + diffWidth
		}
		if l.files {
			n += gutter + filesWidth
		}
		return n
	}

	for meta() > width {
		switch {
		case l.files:
			l.files = false
		case l.diff:
			l.diff = false
		case l.author:
			l.author = false
		case l.repo:
			l.repo = false
		default:
			return l
		}
	}
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
		head = append(head, cell(ageWidth, relativeTime(pr.UpdatedAt), base.Foreground(th.Faint)))
	}

	meta := []string{cell(numberWidth, "#"+strconv.Itoa(pr.Number), base.Foreground(th.Faint))}
	if l.repo {
		meta = append(meta, cell(repoWidth, pr.Repository, base.Foreground(th.Secondary)))
	}
	if l.author {
		meta = append(meta, cell(authorWidth, pr.Author.Login, base.Foreground(th.Actor)))
	}
	if l.diff {
		meta = append(meta, cell(diffWidth, diffStat(pr), base.Foreground(th.Faint)))
	}
	if l.files {
		meta = append(meta, cell(filesWidth, fileCount(pr.ChangedFiles), base.Foreground(th.Faint)))
	}

	return []string{
		line(leftMargin, head, width, base),
		line(indentWidth, meta, width, base),
	}
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
	if width == 1 {
		return "…"
	}
	return lipgloss.NewStyle().MaxWidth(width-1).Render(content) + "…"
}

func diffStat(pr gh.PullRequest) string {
	return "+" + strconv.Itoa(pr.Additions) + " −" + strconv.Itoa(pr.Deletions)
}

func fileCount(n int) string {
	if n == 1 {
		return "1 file"
	}
	return strconv.Itoa(n) + " files"
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
