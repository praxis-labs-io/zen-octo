// Package comp holds the widgets shared across screens.
package comp

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

// Tab is one entry in a pane's top border. Badge follows the label muted and is skipped when empty.
type Tab struct {
	Label string
	Badge string
}

// Pane is a bordered region with a tab strip or title in its top border and a footer in its bottom.
type Pane struct {
	theme   theme.Theme
	title   string
	header  string
	index   int
	tabs    []Tab
	active  int
	footer  string
	focused bool
	width   int
	height  int
}

// NewPane returns an unsized pane.
func NewPane(th theme.Theme) Pane {
	return Pane{theme: th}
}

// Title sets the text in the top border. Tabs take precedence over it.
func (p Pane) Title(s string) Pane {
	p.title = s
	return p
}

// Header sets a heading row inside the pane, ruled off from the content. s is rendered as given.
func (p Pane) Header(s string) Pane {
	p.header = s
	return p
}

// Chrome is the lines the pane spends on borders and any heading row and rule.
func (p Pane) Chrome() int {
	if p.header == "" {
		return 2
	}
	return 4
}

// Above is the lines drawn before the content: the top border, and the heading and rule where they fit.
func (p Pane) Above() int {
	if p.width < 2 || p.height < 2 {
		return 0
	}
	if p.header == "" || p.InnerHeight() < 3 {
		return 1
	}
	return 3
}

// Index sets the bracketed number leading the top border. Zero leaves it off.
func (p Pane) Index(n int) Pane {
	p.index = n
	return p
}

func (p Pane) Tabs(tabs []Tab, active int) Pane {
	p.tabs, p.active = tabs, active
	return p
}

// Footer sets the right-aligned text in the bottom border.
func (p Pane) Footer(s string) Pane {
	p.footer = s
	return p
}

func (p Pane) Focus(v bool) Pane {
	p.focused = v
	return p
}

// Size sets the pane's outer dimensions, borders included.
func (p Pane) Size(width, height int) Pane {
	p.width, p.height = width, height
	return p
}

func (p Pane) InnerWidth() int { return max(0, p.width-2) }

func (p Pane) InnerHeight() int { return max(0, p.height-2) }

// Render frames content, padding it with plain spaces or clipping it to the pane's size.
func (p Pane) Render(content string) string {
	if p.width < 2 || p.height < 2 {
		return ""
	}

	lines := make([]string, 0, p.height)
	lines = append(lines, p.topBorder())

	rows := p.InnerHeight()
	if p.header != "" && rows >= 3 {
		lines = append(lines, p.row(p.header), p.rule())
		rows -= 2
	}

	if body := p.body(content, rows); body != "" {
		lines = append(lines, body)
	}
	return strings.Join(append(lines, p.bottomBorder()), "\n")
}

func (p Pane) rule() string {
	style := p.borderStyle()
	return style.Render("├" + strings.Repeat("─", p.InnerWidth()) + "┤")
}

func (p Pane) row(line string) string {
	side := p.borderStyle().Render("│")
	line = lipgloss.NewStyle().MaxWidth(p.InnerWidth()).Render(line)
	return side + line + strings.Repeat(" ", max(0, p.InnerWidth()-lipgloss.Width(line))) + side
}

func (p Pane) borderStyle() lipgloss.Style {
	c := p.theme.BorderSubtleOrBorder()
	if p.focused {
		c = p.theme.Accent
	}
	return lipgloss.NewStyle().Foreground(c)
}

func (p Pane) topBorder() string {
	style := p.borderStyle()
	mid := p.width - 2

	segments := []string{style.Render("─")}
	used := 1

	if p.index > 0 {
		badge := lipgloss.NewStyle().Foreground(p.theme.Accent).Render("[" + strconv.Itoa(p.index) + "]")
		segments = append(segments, badge, style.Render("─"))
		used += lipgloss.Width(badge) + 1
	}

	label := p.tabStrip()
	if label == "" && p.title != "" {
		label = lipgloss.NewStyle().Foreground(p.theme.Text).Bold(true).Render(p.title)
	}
	if label != "" {
		label = lipgloss.NewStyle().MaxWidth(max(0, mid-used)).Render(label)
		segments = append(segments, label)
		used += lipgloss.Width(label)
	}

	segments = append(segments, style.Render(strings.Repeat("─", max(0, mid-used))))
	return style.Render("╭") + strings.Join(segments, "") + style.Render("╮")
}

func (p Pane) bottomBorder() string {
	style := p.borderStyle()
	mid := p.width - 2

	if p.footer == "" {
		return style.Render("╰" + strings.Repeat("─", mid) + "╯")
	}

	footer := lipgloss.NewStyle().Foreground(p.theme.MutedOrSubtle()).
		MaxWidth(max(0, mid-1)).Render(p.footer)
	fill := max(0, mid-lipgloss.Width(footer)-1)

	return style.Render("╰"+strings.Repeat("─", fill)) + footer + style.Render("─╯")
}

func (p Pane) tabStrip() string {
	if len(p.tabs) == 0 {
		return ""
	}

	activeStyle := lipgloss.NewStyle().Foreground(p.theme.Accent).Bold(true)
	idleStyle := lipgloss.NewStyle().Foreground(p.theme.Subtle)
	badgeStyle := lipgloss.NewStyle().Foreground(p.theme.MutedOrSubtle())
	sep := badgeStyle.Render(" - ")

	parts := make([]string, 0, len(p.tabs))
	for i, tab := range p.tabs {
		style := idleStyle
		if i == p.active {
			style = activeStyle
		}
		part := style.Render(tab.Label)
		if tab.Badge != "" {
			part += badgeStyle.Render(" " + tab.Badge)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, sep)
}

func (p Pane) body(content string, rows int) string {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, rows)
	for i := range rows {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		out = append(out, p.row(line))
	}
	return strings.Join(out, "\n")
}
