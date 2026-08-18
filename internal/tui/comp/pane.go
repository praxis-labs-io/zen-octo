// Package comp holds the widgets shared across screens: the pane chrome, the
// status bar, the overlay compositor, and the badges that render a pull
// request's state the same way wherever it appears.
package comp

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

// Tab is one entry in a pane's top border. Badge renders muted after the label
// and is skipped when empty, so a count that hasn't loaded shows nothing rather
// than a zero. Its punctuation is the caller's: a count is worth bracketing and
// a failure mark is not.
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
	header  string
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

// Header sets a heading row inside the pane, ruled off from the content below
// it. The text is taken as-is: a heading colored piece by piece would be cut
// short by the first reset inside it if the pane restyled it.
//
// This is not Title. A title sits in the top border and names the pane; a
// header is the first thing in the pane, and it is what a comment card wants.
func (p Pane) Header(s string) Pane {
	p.header = s
	return p
}

// Chrome is the lines the pane spends on itself: two borders, plus the heading
// row and its rule when there is one. A caller sizing a pane to its content
// adds this.
func (p Pane) Chrome() int {
	if p.header == "" {
		return 2
	}
	return 4
}

// Above is the lines the pane draws before its content: the top border, and
// the heading row with its rule when there is one and there is room for it.
// Anything mapping a line of content to a line on the screen has to clear
// these, and reading it off the pane is what keeps the two in step when the
// heading changes.
func (p Pane) Above() int {
	// Render draws nothing at all at this size, so there is no line for a
	// caller's arithmetic to clear.
	if p.width < 2 || p.height < 2 {
		return 0
	}
	if p.header == "" || p.InnerHeight() < 3 {
		return 1
	}
	return 3
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

	lines := make([]string, 0, p.height)
	lines = append(lines, p.topBorder())

	// The heading and its rule are two of the pane's own lines. A pane with no
	// room for both of them plus a line of content is better off showing the
	// content, which is the part that carries the meaning.
	rows := p.InnerHeight()
	if p.header != "" && rows >= 3 {
		lines = append(lines, p.row(p.header), p.rule())
		rows -= 2
	}

	// At a height of two the borders are the whole pane. Writing the body
	// unconditionally costs a third line and overflows the frame.
	if body := p.body(content, rows); body != "" {
		lines = append(lines, body)
	}
	return strings.Join(append(lines, p.bottomBorder()), "\n")
}

// rule divides the heading from the content, joining the side borders rather
// than floating inside them.
func (p Pane) rule() string {
	style := p.borderStyle()
	return style.Render("├" + strings.Repeat("─", p.InnerWidth()) + "┤")
}

// row frames one line of content, clipping and padding it to the interior.
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

// topBorder lays the index and the tab strip flush against the left corner,
// separated by border runes rather than padded with spaces. That placement is
// lazygit's and it reads tighter than a floated label.
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

// bottomBorder carries the counter, right-aligned one rune in from the corner.
func (p Pane) bottomBorder() string {
	style := p.borderStyle()
	mid := p.width - 2

	if p.footer == "" {
		return style.Render("╰" + strings.Repeat("─", mid) + "╯")
	}

	// The footer is chrome: a scroll counter, a line of key hints. It reads at
	// the same weight as the border it sits in rather than at the weight of the
	// content above it, and it stays there whichever pane has focus. Which pane
	// that is the border already says, and saying it twice is a second encoding
	// of a fact the reader can already see.
	footer := lipgloss.NewStyle().Foreground(p.theme.MutedOrSubtle()).
		MaxWidth(max(0, mid-1)).Render(p.footer)
	fill := max(0, mid-lipgloss.Width(footer)-1)

	return style.Render("╰"+strings.Repeat("─", fill)) + footer + style.Render("─╯")
}

// tabStrip renders the tabs. The current one carries the accent and the rest
// recede; there is no marker glyph. It returns empty when there are none, so
// the caller can fall back to the title.
//
// The badge stays muted on the current tab as well. It is a count either way,
// and accenting it puts the eye on the number rather than on the name of the
// place the reader is standing.
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

// body pads or clips content to the rows it was left.
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
