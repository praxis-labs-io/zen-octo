// Package comp holds the widgets shared across screens: the pane chrome, the
// status bar, the overlay compositor, and the badges that render a pull
// request's state the same way wherever it appears.
package comp

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// Tab is one entry in a pane's top border. Badge renders faint after the label
// and is skipped when empty, so a count that hasn't loaded shows nothing rather
// than a zero.
type Tab struct {
	Label string
	Badge string
}

// Pane is a bordered region. It carries a tab strip or a title in its top
// border and a counter in its bottom, colors the border by focus, and reports
// the size left over for content.
//
// The border lines are built rather than drawn by lipgloss and then edited,
// because setting styled text into a rendered border means splicing ANSI in
// place.
type Pane struct {
	theme   theme.Theme
	title   string
	index   int
	tabs    []Tab
	active  int
	footer  string
	focused bool
	width   int
	height  int
}

// NewPane returns an unsized pane. Callers set size, content, and focus as the
// model changes and render last.
func NewPane(th theme.Theme) Pane {
	return Pane{theme: th}
}

// Title sets the text in the top border. Tabs take precedence over it.
func (p Pane) Title(s string) Pane {
	p.title = s
	return p
}

// Index sets the bracketed number that leads the top border and jumps focus
// here. Zero leaves it off, which is right for a screen with one pane.
func (p Pane) Index(n int) Pane {
	p.index = n
	return p
}

// Tabs sets the strip in the top border and which entry is current.
func (p Pane) Tabs(tabs []Tab, active int) Pane {
	p.tabs, p.active = tabs, active
	return p
}

// Footer sets the text in the bottom border, right-aligned.
func (p Pane) Footer(s string) Pane {
	p.footer = s
	return p
}

// Focus colors the border and is the only visual signal of where keys go.
func (p Pane) Focus(v bool) Pane {
	p.focused = v
	return p
}

// Size sets the pane's outer dimensions, borders included.
func (p Pane) Size(width, height int) Pane {
	p.width, p.height = width, height
	return p
}

// InnerWidth is the width available to content.
func (p Pane) InnerWidth() int { return max(0, p.width-2) }

// InnerHeight is the height available to content.
func (p Pane) InnerHeight() int { return max(0, p.height-2) }

// Render frames content. Content shorter than the pane is padded, longer is
// clipped: the pane is the authority on its own size.
//
// Padding uses plain spaces, so content that needs a background running to the
// edge has to emit lines at the full inner width itself.
func (p Pane) Render(content string) string {
	if p.width < 2 || p.height < 2 {
		return ""
	}

	var b strings.Builder
	b.WriteString(p.topBorder())
	b.WriteString("\n")
	b.WriteString(p.body(content))
	b.WriteString("\n")
	b.WriteString(p.bottomBorder())
	return b.String()
}

func (p Pane) borderStyle() lipgloss.Style {
	c := p.theme.BorderSecondaryOrBorder()
	if p.focused {
		c = p.theme.Secondary
	}
	return lipgloss.NewStyle().Foreground(c)
}

// topBorder lays the index and the tab strip flush against the left corner,
// separated by border runes rather than padded with spaces. That placement is
// lazygit's and it reads tighter than a floated label.
func (p Pane) topBorder() string {
	style := p.borderStyle()
	mid := p.width - 2

	segments := []string{style.Render("─")}
	used := 1

	if p.index > 0 {
		badge := lipgloss.NewStyle().Foreground(p.theme.Secondary).Render("[" + strconv.Itoa(p.index) + "]")
		segments = append(segments, badge, style.Render("─"))
		used += lipgloss.Width(badge) + 1
	}

	label := p.tabStrip()
	if label == "" && p.title != "" {
		label = lipgloss.NewStyle().Foreground(p.theme.Primary).Bold(true).Render(p.title)
	}
	if label != "" {
		label = lipgloss.NewStyle().MaxWidth(max(0, mid-used)).Render(label)
		segments = append(segments, label)
		used += lipgloss.Width(label)
	}

	segments = append(segments, style.Render(strings.Repeat("─", max(0, mid-used))))
	return style.Render("╭") + strings.Join(segments, "") + style.Render("╮")
}

// bottomBorder carries the counter, right-aligned one rune in from the corner.
func (p Pane) bottomBorder() string {
	style := p.borderStyle()
	mid := p.width - 2

	if p.footer == "" {
		return style.Render("╰" + strings.Repeat("─", mid) + "╯")
	}

	footer := lipgloss.NewStyle().Foreground(p.theme.Primary).
		MaxWidth(max(0, mid-1)).Render(p.footer)
	fill := max(0, mid-lipgloss.Width(footer)-1)

	return style.Render("╰"+strings.Repeat("─", fill)) + footer + style.Render("─╯")
}

// tabStrip renders the tabs. The current one is bright and the rest recede;
// there is no marker glyph. It returns empty when there are none, so the caller
// can fall back to the title.
func (p Pane) tabStrip() string {
	if len(p.tabs) == 0 {
		return ""
	}

	activeStyle := lipgloss.NewStyle().Foreground(p.theme.Primary).Bold(true)
	idleStyle := lipgloss.NewStyle().Foreground(p.theme.Faint)
	sep := idleStyle.Render(" - ")

	parts := make([]string, 0, len(p.tabs))
	for i, tab := range p.tabs {
		style := idleStyle
		if i == p.active {
			style = activeStyle
		}
		part := style.Render(tab.Label)
		if tab.Badge != "" {
			part += idleStyle.Render(" " + tab.Badge)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, sep)
}

// body pads or clips content to the pane's interior.
func (p Pane) body(content string) string {
	inner, rows := p.InnerWidth(), p.InnerHeight()
	side := p.borderStyle().Render("│")

	lines := strings.Split(content, "\n")
	out := make([]string, 0, rows)
	for i := range rows {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		line = lipgloss.NewStyle().MaxWidth(inner).Render(line)
		line += strings.Repeat(" ", max(0, inner-lipgloss.Width(line)))
		out = append(out, side+line+side)
	}
	return strings.Join(out, "\n")
}
