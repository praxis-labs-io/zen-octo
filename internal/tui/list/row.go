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

// Fixed column widths. The title takes what is left, up to a cap: past that it
// stops growing and the slack goes to a spacer, so a wide terminal does not
// leave a hundred columns of empty title cell before the author.
const (
	stateWidth    = 2
	checksWidth   = 2
	numberWidth   = 6
	repoWidth     = 22
	authorWidth   = 14
	updatedWidth  = 5
	gutter        = 1
	minTitleWidth = 16
	maxTitleWidth = 80
)

// layout is which columns a width can carry, and how much of it the title gets.
type layout struct {
	title  int
	slack  int
	repo   bool
	author bool
	age    bool
}

// fit drops columns until the row fits the width it was given.
//
// Letting the row overflow instead means the pane clips it blind: the trailing
// columns vanish mid-cell with no ellipsis, and the selection background stops
// short of the edge because the row is no longer as wide as its pane.
//
// Author goes first, then repo, then age. Author is the widest thing that is
// usually identical on every row of a section, and age is the cheapest to keep.
func fit(width int) layout {
	l := layout{repo: true, author: true, age: true}
	base := stateWidth + checksWidth + numberWidth + 3*gutter

	optional := func() int {
		n := 0
		if l.repo {
			n += repoWidth + gutter
		}
		if l.author {
			n += authorWidth + gutter
		}
		if l.age {
			n += updatedWidth + gutter
		}
		return n
	}

	for base+optional()+minTitleWidth > width {
		switch {
		case l.author:
			l.author = false
		case l.repo:
			l.repo = false
		case l.age:
			l.age = false
		default:
			// Nothing left to drop. The title takes whatever remains, which at
			// this point may be nothing at all.
			l.title = max(0, width-base)
			return l
		}
	}

	avail := width - base - optional()
	l.title = min(avail, maxTitleWidth)
	l.slack = avail - l.title
	return l
}

// renderRow draws one pull request.
//
// Selection is baked into every cell's own style. Wrapping the joined row
// instead paints only the first cell: each cell ends in a full SGR reset, which
// clears the background along with the foreground.
func renderRow(th theme.Theme, pr gh.PullRequest, width int, selected bool) string {
	l := fit(width)

	base := lipgloss.NewStyle()
	if selected {
		base = base.Background(th.SelectedBackground)
	}

	stateIcon, stateColor := comp.PRStateIcon(th, pr)
	checkIcon, checkColor := comp.CheckStateIcon(th, pr.Checks)

	cells := []string{
		cell(stateWidth, stateIcon, base.Foreground(stateColor)),
		cell(checksWidth, checkIcon, base.Foreground(checkColor)),
		cell(numberWidth, "#"+strconv.Itoa(pr.Number), base.Foreground(th.Faint)),
	}
	if l.repo {
		cells = append(cells, cell(repoWidth, pr.Repository, base.Foreground(th.Secondary)))
	}
	cells = append(cells, cell(l.title, pr.Title, base.Foreground(th.Primary))+base.Render(strings.Repeat(" ", l.slack)))
	if l.author {
		cells = append(cells, cell(authorWidth, pr.Author.Login, base.Foreground(th.Actor)))
	}
	if l.age {
		cells = append(cells, cell(updatedWidth, relativeTime(pr.UpdatedAt), base.Foreground(th.Faint)))
	}

	return strings.Join(cells, base.Render(strings.Repeat(" ", gutter)))
}

// cell pads to width and truncates anything longer, so columns stay aligned
// whatever the content.
//
// Truncation is explicit because Style.Width wraps first and only then clips,
// which turns one long title into two rows instead of one clipped one.
func cell(width int, content string, style lipgloss.Style) string {
	if width > 1 && lipgloss.Width(content) > width {
		content = lipgloss.NewStyle().MaxWidth(width-1).Render(content) + "…"
	}
	pad := max(0, width-lipgloss.Width(content))
	return style.Render(content + strings.Repeat(" ", pad))
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
