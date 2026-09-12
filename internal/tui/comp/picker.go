package comp

import (
	"image/color"
	"strconv"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/paint"
	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

const (
	pickerRows = 10

	// Below this every choice is already on screen, so a filter row is only one more thing to read.
	pickerFilterFrom = 8

	// Clears the longest hint any picker draws, so single and multi select open at one width.
	pickerMinWidth = 52
	pickerMaxWidth = 72

	pickerMark = "✓ "
	pickerGap  = "  "
)

// PickerItem is one choice. A nil Color renders the name in the theme's Text.
type PickerItem struct {
	ID   string
	Name string
	// Always a theme color, never GitHub's: a label hex chosen for a white page vanishes on a dark terminal.
	Color color.Color
}

// Picker is a modal list of choices, single or multi select, over a filter. It holds no keymap;
// the screen maps keys onto its methods.
type Picker struct {
	title string
	multi bool

	note string

	items   []PickerItem
	checked map[string]bool

	filter    string
	filtering bool

	// Index the filtered list, not items.
	cursor int
	top    int
}

// NewPicker builds a picker over items with checked pre-selected by ID. A single-select picker
// opens with its cursor on the checked row.
func NewPicker(title string, items []PickerItem, checked []string, multi bool) Picker {
	on := make(map[string]bool, len(checked))
	for _, id := range checked {
		on[id] = true
	}

	p := Picker{
		title:     title,
		multi:     multi,
		items:     items,
		checked:   on,
		filtering: len(items) >= pickerFilterFrom,
	}

	if !multi {
		for i, it := range items {
			if on[it.ID] {
				p.cursor = i
				break
			}
		}
		p.scroll()
	}
	return p
}

func (p Picker) Multi() bool { return p.multi }

// Replace swaps the choices for items and sets note, keeping the filter, the filter row, what is
// checked, and the cursor on the same ID where the new list still has it.
func (p *Picker) Replace(items []PickerItem, note string) {
	var on string
	if it, ok := p.at(); ok {
		on = it.ID
	}

	p.items, p.note = items, note
	p.reanchor()

	if on == "" {
		return
	}
	for i, it := range p.shown() {
		if it.ID == on {
			p.cursor = i
			p.scroll()
			return
		}
	}
}

// SetNote sets what the title says about the list as a whole, leaving the list and cursor alone.
func (p *Picker) SetNote(note string) { p.note = note }

// Move walks the cursor by delta, stopping at either end.
func (p *Picker) Move(delta int) {
	shown := p.shown()
	if len(shown) == 0 {
		p.cursor, p.top = 0, 0
		return
	}
	p.cursor = min(max(p.cursor+delta, 0), len(shown)-1)
	p.scroll()
}

// Toggle checks or unchecks the row under the cursor. It does nothing on a single-select picker.
func (p *Picker) Toggle() {
	if !p.multi {
		return
	}
	it, ok := p.at()
	if !ok {
		return
	}
	if p.checked[it.ID] {
		delete(p.checked, it.ID)
		return
	}
	p.checked[it.ID] = true
}

// Insert folds a keypress into the filter and reports whether it took it. Without a filter row it
// takes nothing, and on a multi-select picker space is left to Toggle.
func (p *Picker) Insert(msg tea.KeyPressMsg) bool {
	if !p.filtering {
		return false
	}

	switch msg.String() {
	case "backspace":
		if p.filter != "" {
			r := []rune(p.filter)
			p.filter = string(r[:len(r)-1])
			p.reanchor()
		}
		return true
	case "ctrl+u":
		if p.filter != "" {
			p.filter = ""
			p.reanchor()
		}
		return true
	}

	if utf8.RuneCountInString(msg.Text) != 1 || msg.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper) != 0 {
		return false
	}
	if p.multi && msg.Text == " " {
		return false
	}

	p.filter += msg.Text
	p.reanchor()
	return true
}

// Filtering reports whether the picker shows a filter row, so a bare letter is text rather than a binding.
func (p Picker) Filtering() bool { return p.filtering }

// NoFilter removes the filter row, so printable keys reach movement bindings.
func (p *Picker) NoFilter() {
	p.filtering = false
	p.filter = ""
}

func (p Picker) Filter() string { return p.filter }

// Chosen is what applying selects, by ID, in item order: a multi picker's checked set, or the row
// under a single picker's cursor, empty when the filter matched nothing.
func (p Picker) Chosen() []string {
	if !p.multi {
		if it, ok := p.at(); ok {
			return []string{it.ID}
		}
		return nil
	}

	out := make([]string, 0, len(p.checked))
	for _, it := range p.items {
		if p.checked[it.ID] {
			out = append(out, it.ID)
		}
	}
	return out
}

func (p Picker) shown() []PickerItem {
	if p.filter == "" {
		return p.items
	}
	needle := strings.ToLower(p.filter)
	out := make([]PickerItem, 0, len(p.items))
	for _, it := range p.items {
		if strings.Contains(strings.ToLower(it.Name), needle) {
			out = append(out, it)
		}
	}
	return out
}

func (p Picker) at() (PickerItem, bool) {
	shown := p.shown()
	if p.cursor < 0 || p.cursor >= len(shown) {
		return PickerItem{}, false
	}
	return shown[p.cursor], true
}

func (p *Picker) reanchor() {
	p.cursor, p.top = 0, 0
}

// Shortest distance, not top row: a cursor stepping a row at a time is not being taken anywhere.
func (p *Picker) scroll() {
	if p.cursor < p.top {
		p.top = p.cursor
		return
	}
	if p.cursor >= p.top+pickerRows {
		p.top = p.cursor - pickerRows + 1
	}
}

// Render draws the picker as a modal sized to fit a frame frameWidth wide, for Over to place.
func (p Picker) Render(th theme.Theme, frameWidth int) string {
	inner := p.width(frameWidth)
	shown := p.shown()

	return Modal(th, p.heading(), strings.Join(p.rows(th, inner, shown), "\n"))
}

func (p Picker) rows(th theme.Theme, inner int, shown []PickerItem) []string {
	var rows []string
	if p.filtering {
		rows = append(rows, p.filterRow(th, inner))
	}
	rows = append(rows, "")
	rows = append(rows, p.list(th, shown, inner)...)
	return append(rows, "", p.hint(th, shown, inner))
}

// Cursor is the terminal cursor at the filter row, relative to a frame of the given size, or nil without a filter row.
func (p Picker) Cursor(th theme.Theme, frameWidth, frameHeight int) *tea.Cursor {
	if !p.filtering {
		return nil
	}

	inner := p.width(frameWidth)
	over := Modal(th, p.heading(), strings.Join(p.rows(th, inner, p.shown()), "\n"))
	x, y := OverOrigin(over, frameWidth, frameHeight)

	col := min(lipgloss.Width(p.filter), max(0, inner-1))
	return Cursor(th, x+ModalLead+col, y+1)
}

func (p Picker) heading() string {
	if p.note == "" {
		return p.title
	}
	return p.title + " · " + p.note
}

// Measures the hint at its longest, counter included, or long lists clip it.
func (p Picker) width(frameWidth int) int {
	longest := max(lipgloss.Width(p.heading()), lipgloss.Width(p.hintText(len(p.items))))
	for _, it := range p.items {
		longest = max(longest, lipgloss.Width(it.Name)+lipgloss.Width(pickerMark))
	}

	want := min(max(longest, pickerMinWidth), pickerMaxWidth)

	if room := frameWidth - 4; room > 0 {
		want = min(want, room)
	}
	return max(want, 1)
}

func (p Picker) filterRow(th theme.Theme, width int) string {
	plain := lipgloss.NewStyle()
	if p.filter == "" {
		return pad(plain.Foreground(th.Subtle).Render("Type to filter"), width, plain)
	}
	text := plain.Foreground(th.Text).Render(p.filter)
	return pad(paint.Clip(text, width, plain.Foreground(th.Subtle)), width, plain)
}

func (p Picker) list(th theme.Theme, shown []PickerItem, width int) []string {
	plain := lipgloss.NewStyle()
	if len(shown) == 0 {
		return []string{pad(plain.Foreground(th.Subtle).Render("No match"), width, plain)}
	}

	end := min(p.top+pickerRows, len(shown))
	out := make([]string, 0, end-p.top)
	for i := p.top; i < end; i++ {
		it := shown[i]

		base := plain
		if i == p.cursor {
			base = plain.Background(th.SelectedBackground)
		}

		mark := base.Foreground(th.Subtle).Render(pickerGap)
		if p.checked[it.ID] {
			mark = base.Foreground(th.Success).Render(pickerMark)
		}

		c := it.Color
		if c == nil {
			c = th.Text
		}
		name := base.Foreground(c).Render(it.Name)
		room := width - lipgloss.Width(pickerMark)
		if lipgloss.Width(it.Name) > room {
			name = paint.Clip(name, room, base.Foreground(th.Subtle))
		}

		out = append(out, pad(mark+name, width, base))
	}
	return out
}

func (p Picker) hint(th theme.Theme, shown []PickerItem, width int) string {
	plain := lipgloss.NewStyle()
	faint := plain.Foreground(th.Subtle)

	text := faint.Render(p.hintText(len(shown)))
	if lipgloss.Width(p.hintText(len(shown))) > width {
		text = paint.Clip(text, width, faint)
	}
	return pad(text, width, plain)
}

func (p Picker) hintText(shown int) string {
	text := "⏎ pick · esc cancel"
	if p.multi {
		text = "space toggle · ⏎ apply · esc cancel"
	}
	if more := shown - pickerRows; more > 0 {
		return strconv.Itoa(more) + " more · " + text
	}
	return text
}

func pad(content string, width int, style lipgloss.Style) string {
	if gap := width - lipgloss.Width(content); gap > 0 {
		return content + style.Render(strings.Repeat(" ", gap))
	}
	return content
}
