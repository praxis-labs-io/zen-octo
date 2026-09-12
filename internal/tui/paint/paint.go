// Package paint renders single diff rows and hunk headers as pure functions of line and width.
package paint

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/syntax"
	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

// A raw tab spans a variable number of cells and would put every later column out of step.
const defaultTabWidth = 4

// Kind is the side of the change a line belongs to.
type Kind int

const (
	Context Kind = iota
	Added
	Removed
)

// Line is one row ready to paint. A zero Old or New means that side has none; its column is still held open.
type Line struct {
	Kind     Kind
	Old, New int
	Tokens   []syntax.Token

	// Fill overrides the kind's tint; nil keeps it.
	Fill color.Color

	// Bar paints the leading cell with BarGlyph; nil leaves it blank.
	Bar color.Color
}

// Painter paints rows from one theme. A zero TabWidth expands tabs to four cells.
type Painter struct {
	Theme    theme.Theme
	TabWidth int
}

// Line paints one unified row of code padded to width, clipping anything wider rather than wrapping.
func (p Painter) Line(l Line, gutter, width int) string {
	marker, c, tint := p.weight(l.Kind)
	if l.Fill != nil {
		tint = l.Fill
	}

	base := background(lipgloss.NewStyle(), tint)
	kind := base.Foreground(c)
	faint := base.Foreground(p.Theme.Subtle)

	oldNum, newNum := faint, faint
	switch l.Kind {
	case Added:
		newNum = kind
	case Removed:
		oldNum = kind
	}

	row := Lead(l.Bar, base) +
		oldNum.Render(number(l.Old, gutter)) + base.Render(" ") +
		newNum.Render(number(l.New, gutter)) + base.Render(" ") +
		kind.Render(marker) + base.Render(" ") + p.code(l.Tokens, base)

	if w := lipgloss.Width(row); w > width {
		return Clip(row, width, faint)
	} else if tint != nil {
		row += base.Render(strings.Repeat(" ", width-w))
	}
	return row
}

// Half paints one column of a side-by-side row, padded to width. It shows whichever of Old and New is set.
func (p Painter) Half(l Line, gutter, width int) string {
	marker, c, tint := p.weight(l.Kind)
	if l.Fill != nil {
		tint = l.Fill
	}

	base := background(lipgloss.NewStyle(), tint)
	kind := base.Foreground(c)

	num := base.Foreground(p.Theme.Subtle)
	if l.Kind != Context {
		num = kind
	}

	row := Lead(l.Bar, base) + num.Render(number(max(l.Old, l.New), gutter)) +
		base.Render(" ") + kind.Render(marker) + base.Render(" ") +
		p.code(l.Tokens, base)

	if w := lipgloss.Width(row); w > width {
		return Clip(row, width, base.Foreground(p.Theme.Subtle))
	} else if w < width {
		row += base.Render(strings.Repeat(" ", width-w))
	}
	return row
}

func (p Painter) weight(k Kind) (string, color.Color, color.Color) {
	switch k {
	case Added:
		return "+", p.Theme.Success, p.Theme.AddedBackground
	case Removed:
		return "−", p.Theme.Error, p.Theme.RemovedBackground
	}
	return " ", p.Theme.Subtle, nil
}

// Header is the @@ line ready to paint.
type Header struct {
	Text string

	// Marker sits in the +/− column; "" leaves it blank and anything past two cells is clipped.
	Marker string

	// Badge is a glyph left of the marker, for a state the heading carries regardless of the cursor.
	Badge string

	// BadgeColor paints the badge; nil paints it Accent.
	BadgeColor color.Color

	// TextColor paints the text and marker; nil paints both Accent.
	TextColor color.Color

	// Fill is the row's background; nil paints none.
	Fill color.Color

	// Bar paints the leading cell the same way Line.Bar does.
	Bar color.Color
}

// HunkHeader paints the @@ line indented to a unified row's code column.
func (p Painter) HunkHeader(h Header, gutter, width int) string {
	return p.header(h, CodeColumn(gutter), width)
}

// HalfHeader paints the @@ line indented to a side-by-side row's code column.
func (p Painter) HalfHeader(h Header, gutter, width int) string {
	return p.header(h, HalfColumn(gutter), width)
}

func (p Painter) header(h Header, code, width int) string {
	base := background(lipgloss.NewStyle(), h.Fill)
	accent := base.Foreground(p.Theme.Accent)

	text := accent
	if h.TextColor != nil {
		text = base.Foreground(h.TextColor)
	}

	badge := accent
	if h.BadgeColor != nil {
		badge = base.Foreground(h.BadgeColor)
	}

	row := Lead(h.Bar, base) +
		base.Render(strings.Repeat(" ", max(0, code-2*markerSlot-1))) +
		slot(h.Badge, base, badge) + slot(h.Marker, base, text) +
		text.Render(h.Text)

	if w := lipgloss.Width(row); w > width {
		return Clip(row, width, base.Foreground(p.Theme.Subtle))
	} else if h.Fill != nil {
		row += base.Render(strings.Repeat(" ", width-w))
	}
	return row
}

// BarGlyph marks the row the cursor is on, in the leading cell every row holds open.
const BarGlyph = "▌"

// Lead is a row's first cell over base: BarGlyph colored bar, or a blank where bar is nil.
func Lead(bar color.Color, base lipgloss.Style) string {
	if bar == nil {
		return base.Render(" ")
	}
	return base.Foreground(bar).Render(BarGlyph)
}

func slot(glyph string, base, on lipgloss.Style) string {
	if glyph == "" {
		return base.Render(strings.Repeat(" ", markerSlot))
	}
	g := lipgloss.NewStyle().MaxWidth(markerSlot).Render(glyph)
	return on.Render(g) + base.Render(strings.Repeat(" ", markerSlot-lipgloss.Width(g)))
}

func (p Painter) code(tokens []syntax.Token, base lipgloss.Style) string {
	tab := strings.Repeat(" ", p.tabWidth())

	var b strings.Builder
	for _, t := range tokens {
		text := strings.ReplaceAll(t.Text, "\t", tab)
		if t.Color == nil {
			b.WriteString(base.Render(text))
			continue
		}
		b.WriteString(base.Foreground(t.Color).Render(text))
	}
	return b.String()
}

func (p Painter) tabWidth() int {
	if p.TabWidth <= 0 {
		return defaultTabWidth
	}
	return p.TabWidth
}

func background(s lipgloss.Style, c color.Color) lipgloss.Style {
	if c == nil {
		return s
	}
	return s.Background(c)
}
