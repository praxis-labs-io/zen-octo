package list

import (
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/paint"
	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

const (
	leftMargin  = 1
	rightMargin = 1
	stateWidth  = 2
	gutter      = 1
	indentWidth = leftMargin + stateWidth + gutter

	numberWidth = 6
	headWidth   = leftMargin + stateWidth + gutter + numberWidth + gutter

	statusWidth = 8

	additionsWidth = 5
	deletionsWidth = 5
	filesWidth     = 5

	minIdentWidth = 8

	minTitleWidth = 12
)

const (
	glyphFiles    = "\uea7b"
	glyphComments = "\uf41f"
	glyphReview   = "\uedc6"
	glyphChecks   = "\uf0ae"
)

type layout struct {
	title int

	ident  int
	diff   bool
	files  bool
	status bool
}

func fit(width int) layout {
	l := layout{diff: true, files: true, status: true}

	l.title = width - headWidth - rightMargin
	if l.title < minTitleWidth {
		l.title = max(0, l.title)
		l.ident = max(0, width-indentWidth)
		return layout{title: l.title, ident: l.ident}
	}

	metaTail := func() int {
		n := rightMargin
		if l.diff {
			n += gutter + additionsWidth + gutter + deletionsWidth
		}
		if l.files {
			n += gutter + filesWidth
		}
		if l.status {
			n += gutter + statusWidth
		}
		return n
	}

	for indentWidth+minIdentWidth+metaTail() > width {
		switch {
		case l.files:
			l.files = false
		case l.diff:
			l.diff = false
		case l.status:
			l.status = false
		default:
			l.ident = max(0, width-indentWidth)
			return l
		}
	}

	l.ident = width - indentWidth - metaTail()
	return l
}

// Selection is styled per cell: each cell's SGR reset clears a background set around the joined line.
func renderRow(th theme.Theme, it item, width int, selected bool) []string {
	pr, l := it.pr, fit(width)

	base := lipgloss.NewStyle()
	if selected {
		base = base.Background(th.SelectedBackground)
	}

	stateIcon, stateColor := comp.PRStateIcon(th, pr)
	_, checkColor := comp.CheckStateIcon(th, pr.Checks)
	reviewColor := comp.ReviewColor(th, pr.ReviewDecision)

	head := []string{
		cell(stateWidth, stateIcon, base.Foreground(stateColor)),
		cell(numberWidth, "#"+strconv.Itoa(pr.Number), base.Foreground(th.Accent)),
		titled(th, pr, l.title, base),
	}

	tail := []string{identity(th, pr, l.ident, base)}
	if l.diff {
		tail = append(tail,
			cell(additionsWidth, alignRight(churn("+", pr.Additions), additionsWidth), base.Foreground(th.Success)),
			cell(deletionsWidth, alignRight(churn("−", pr.Deletions), deletionsWidth), base.Foreground(th.Error)),
		)
	}
	if l.files {
		tail = append(tail, cell(filesWidth, counted(glyphFiles, pr.ChangedFiles, filesWidth), base.Foreground(th.Subtle)))
	}
	if l.status {
		tail = append(tail, base.Render(" ")+
			statusMark(th, base, glyphReview, reviewColor)+base.Render(" ")+
			statusMark(th, base, glyphChecks, checkColor))
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

func statusMark(th theme.Theme, base lipgloss.Style, glyph string, c color.Color) string {
	return base.Foreground(c).Render("●") + base.Render(" ") + base.Foreground(th.Subtle).Render(glyph)
}

func counted(glyph string, n, width int) string {
	if n == 0 {
		return ""
	}
	return alignRight(strconv.Itoa(n)+" "+glyph, width)
}

// Abbreviates past four digits, since a clipped number reads as a different number.
func churn(sign string, n int) string {
	switch {
	case n < 10_000:
		return sign + strconv.Itoa(n)
	case n < 1_000_000:
		return sign + strconv.Itoa(n/1_000) + "k"
	default:
		return sign + strconv.Itoa(n/1_000_000) + "M"
	}
}

func alignRight(s string, width int) string {
	return strings.Repeat(" ", max(0, width-lipgloss.Width(s))) + s
}

func titled(th theme.Theme, pr gh.PullRequest, width int, base lipgloss.Style) string {
	title := base.Foreground(th.Text)
	count := glyphComments + " " + strconv.Itoa(pr.Comments)

	room := width - lipgloss.Width(count) - 2
	if pr.Comments == 0 || room < lipgloss.Width(count) {
		return cell(width, pr.Title, title)
	}

	text := pr.Title
	if lipgloss.Width(text) > room {
		text = paint.Clip(text, room, lipgloss.NewStyle())
	}
	pad := width - lipgloss.Width(text) - lipgloss.Width(count) - 2

	return title.Render(text) + base.Render("  ") +
		base.Foreground(th.Accent).Render(count) + base.Render(strings.Repeat(" ", pad))
}

func identity(th theme.Theme, pr gh.PullRequest, width int, base lipgloss.Style) string {
	age := ""
	if at := comp.RelativeTime(pr.UpdatedAt); at != "" {
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
	return cell(width, text, base.Foreground(th.Subtle))
}

func renderHeader(th theme.Theme, it item, width int) []string {
	rule := lipgloss.NewStyle().Foreground(th.BorderMutedOrSubtle())

	left := rule.Render("─ ") +
		lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render(it.header) + " " +
		lipgloss.NewStyle().Foreground(th.MutedOrSubtle()).Render("("+strconv.Itoa(it.count)+")") + " "

	fill := max(0, width-lipgloss.Width(left))
	rendered := lipgloss.NewStyle().MaxWidth(width).Render(left + rule.Render(strings.Repeat("─", fill)))

	lines := make([]string, it.gapAbove, it.gapAbove+1)
	for i := range lines {
		lines[i] = strings.Repeat(" ", width)
	}
	return append(lines, rendered)
}

func line(indent int, cells []string, width int, base lipgloss.Style) string {
	s := base.Render(strings.Repeat(" ", indent)) + strings.Join(cells, base.Render(strings.Repeat(" ", gutter)))

	switch w := lipgloss.Width(s); {
	case w < width:
		return s + base.Render(strings.Repeat(" ", width-w))
	case w > width:
		return paint.Clip(s, width, base)
	}
	return s
}

// Clips explicitly because Style.Width wraps before it clips, turning a long title into two rows.
func cell(width int, content string, style lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(content) > width {
		content = paint.Clip(content, width, lipgloss.NewStyle())
	}
	pad := max(0, width-lipgloss.Width(content))
	return style.Render(content + strings.Repeat(" ", pad))
}
