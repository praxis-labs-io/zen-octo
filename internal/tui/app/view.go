package app

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
)

// Fixed column widths. The title column takes whatever is left.
const (
	stateWidth   = 2
	checksWidth  = 2
	numberWidth  = 6
	repoWidth    = 22
	authorWidth  = 14
	updatedWidth = 5
	gutter       = 1

	minTitleWidth = 20
	fallbackWidth = 100
)

// chromeLines counts every line render emits that isn't a pull request row:
// the header, the blank line under it, the blank line above the footer, and the
// footer itself.
const chromeLines = 4

func (m Model) render() string {
	width := m.width
	if width <= 0 {
		width = fallbackWidth
	}

	var b strings.Builder
	b.WriteString(m.renderHeader(width))
	b.WriteString("\n\n")

	switch {
	case m.err != nil:
		b.WriteString(m.renderError())
	case m.loading:
		b.WriteString(m.spinner.View() + " " + m.faint().Render("Loading pull requests"))
	case len(m.prs) == 0:
		b.WriteString(m.faint().Render("Nothing matches this section."))
	default:
		b.WriteString(m.renderRows(width))
	}

	b.WriteString("\n\n")
	b.WriteString(m.renderFooter())
	return b.String()
}

// visibleRows is how many pull requests fit between the chrome. Zero height
// means the terminal hasn't reported a size yet, so render everything rather
// than nothing.
func (m Model) visibleRows() int {
	if m.height <= 0 {
		return len(m.prs)
	}
	noticeLines := 0
	if m.notice != "" {
		noticeLines = 1
	}
	return max(0, m.height-chromeLines-noticeLines)
}

func (m Model) renderHeader(width int) string {
	title := lipgloss.NewStyle().
		Foreground(m.theme.InvertedOrPrimary()).
		Background(m.theme.Secondary).
		Bold(true).
		Padding(0, 1).
		Render(m.section.Title)

	count := ""
	if !m.loading && m.err == nil {
		count = m.faint().Render(fmt.Sprintf(" %d", len(m.prs)))
	}

	head := title + count
	rule := lipgloss.NewStyle().Foreground(m.theme.BorderFaintOrSecondary()).
		Render(strings.Repeat("─", max(0, width-lipgloss.Width(head))))
	return head + rule
}

func (m Model) renderError() string {
	label := lipgloss.NewStyle().Foreground(m.theme.Error).Bold(true).Render("Failed to load")
	// Scope errors carry a multi-line fix; keep the newlines the error wrote.
	return label + "\n" + m.faint().Render(m.err.Error())
}

func (m Model) renderRows(width int) string {
	titleWidth := width - (stateWidth + checksWidth + numberWidth + repoWidth + authorWidth + updatedWidth + 6*gutter)
	if titleWidth < minTitleWidth {
		titleWidth = minTitleWidth
	}

	end := min(m.offset+m.visibleRows(), len(m.prs))
	rows := make([]string, 0, max(0, end-m.offset))
	for i := m.offset; i < end; i++ {
		rows = append(rows, m.renderRow(m.prs[i], titleWidth, i == m.cursor))
	}
	return strings.Join(rows, "\n")
}

func (m Model) renderRow(pr gh.PullRequest, titleWidth int, selected bool) string {
	stateIcon, stateColor := prStateIcon(pr)
	checkIcon, checkColor := checkStateIcon(pr.Checks)

	// Selection has to be baked into every cell's own style. Wrapping the
	// joined row instead only paints the first cell: each cell ends in a full
	// SGR reset, which clears the background along with the foreground.
	base := lipgloss.NewStyle()
	if selected {
		base = base.Background(m.theme.SelectedBackground)
	}

	cells := []string{
		cell(stateWidth, stateIcon, base.Foreground(m.color(stateColor))),
		cell(checksWidth, checkIcon, base.Foreground(m.color(checkColor))),
		cell(numberWidth, "#"+strconv.Itoa(pr.Number), base.Foreground(m.theme.Faint)),
		cell(repoWidth, pr.Repository, base.Foreground(m.theme.Secondary)),
		cell(titleWidth, pr.Title, base.Foreground(m.theme.Primary)),
		cell(authorWidth, pr.Author.Login, base.Foreground(m.theme.Actor)),
		cell(updatedWidth, relativeTime(pr.UpdatedAt), base.Foreground(m.theme.Faint)),
	}

	return strings.Join(cells, base.Render(strings.Repeat(" ", gutter)))
}

// cell pads to width and truncates anything longer, so columns stay aligned
// whatever the content.
func cell(width int, content string, style lipgloss.Style) string {
	return style.Width(width).MaxWidth(width).Render(content)
}

func (m Model) renderFooter() string {
	keys := []string{"j/k move", "r refresh", "q quit"}
	footer := m.faint().Render(strings.Join(keys, "  ·  "))

	if hidden := len(m.prs) - (m.offset + m.visibleRows()); hidden > 0 {
		footer += m.faint().Render(fmt.Sprintf("  ·  %d more below", hidden))
	}
	if m.notice != "" {
		footer = lipgloss.NewStyle().Foreground(m.theme.Warning).Render(m.notice) + "\n" + footer
	}
	return footer
}

func (m Model) faint() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(m.theme.Faint)
}

// semantic names the theme role a cell should take, so icon choice and color
// lookup stay separable.
type semantic int

const (
	semanticFaint semantic = iota
	semanticPrimary
	semanticSuccess
	semanticWarning
	semanticError
	semanticSecondary
)

func (m Model) color(s semantic) color.Color {
	switch s {
	case semanticSuccess:
		return m.theme.Success
	case semanticWarning:
		return m.theme.Warning
	case semanticError:
		return m.theme.Error
	case semanticSecondary:
		return m.theme.Secondary
	case semanticPrimary:
		return m.theme.Primary
	case semanticFaint:
		return m.theme.Faint
	}
	return m.theme.Faint
}

func prStateIcon(pr gh.PullRequest) (string, semantic) {
	if pr.IsDraft {
		return "◌", semanticFaint
	}
	switch pr.State {
	case gh.PRStateMerged:
		return "⬤", semanticSecondary
	case gh.PRStateClosed:
		return "✕", semanticError
	case gh.PRStateOpen:
		return "●", semanticSuccess
	}
	return "●", semanticFaint
}

func checkStateIcon(s gh.CheckState) (string, semantic) {
	switch s {
	case gh.CheckStateSuccess:
		return "✓", semanticSuccess
	case gh.CheckStateFailure, gh.CheckStateError:
		return "✗", semanticError
	case gh.CheckStatePending, gh.CheckStateExpected:
		return "●", semanticWarning
	case gh.CheckStateNone:
		return " ", semanticFaint
	}
	return " ", semanticFaint
}

// relativeTime renders a compact age: 34m, 5h, 12d, 3y. Anything in the
// future reads as "now" rather than a negative number.
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
