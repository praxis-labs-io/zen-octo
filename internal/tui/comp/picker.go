package comp

import (
	"image/color"
	"strconv"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

const (
	// pickerRows is how many choices show at once. Ten is a screenful to scan
	// without the modal growing tall enough to cover what it was opened from.
	pickerRows = 10

	// pickerFilterFrom is the shortest list that earns a filter row. Below it
	// every choice is already on screen and a text field is one more thing to
	// read before pressing a key that was always going to be j.
	pickerFilterFrom = 8

	// pickerMinWidth clears the longest hint any picker draws, so single and
	// multi select open at the same width over short content rather than at the
	// two their own hints would give them.
	pickerMinWidth = 40
	pickerMaxWidth = 56

	// pickerMark is the two cells in front of every choice. Checked or not, the
	// name starts in the same column: a list whose rows begin at two different
	// offsets reads as two lists.
	pickerMark = "✓ "
	pickerGap  = "  "
)

// PickerItem is one choice.
//
// Color is what the name renders in, and it comes from the caller's theme so
// each kind of choice reads the way it does on the rail it was opened from.
// Nil takes the theme's Primary. It is never a color GitHub supplied: a hex
// chosen against a white browser page vanishes on a dark terminal, and a
// terminal speaking only ANSI cannot show it at all.
type PickerItem struct {
	ID    string
	Name  string
	Color color.Color
}

// Picker is a modal list of choices, single or multi select, over a filter.
//
// It holds no keymap. Bindings live in internal/tui/keys and a widget package
// cannot reach sideways for them, so this exposes verbs and the screen decides
// which key means which. Filter typing is the one exception, and Insert takes
// the keypress because a rune is not a verb.
//
// The filter is a plain string rather than a text input. Nobody edits the
// middle of a picker filter: they type, they backspace, they clear. A textinput
// would buy a blinking caret and cost a cursor-blink command to plumb through
// two packages.
type Picker struct {
	title string
	multi bool

	// note is what the title says about the list as a whole, and it is where a
	// picker whose choices come from a search reports what the search did not
	// return. The hint line under the list is already spoken for by how much of
	// the list sits below the window, and "20 more" and "36 more matches" are
	// different numbers that must not share a line.
	note string

	items   []PickerItem
	checked map[string]bool

	filter    string
	filtering bool

	// cursor and top index the filtered list, not items. Filtering rewrites
	// what is on screen, and an index into the whole set would point at a row
	// the reader cannot see.
	cursor int
	top    int
}

// NewPicker builds a picker over items, with checked pre-selected by ID.
//
// multi decides both the keys the caller should offer and what applying means:
// a multi picker applies the whole set it is showing, a single one applies the
// row the cursor is on.
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

	// A single-select picker opens on what is already chosen, so enter with no
	// movement is a no-op rather than a change to whatever sorted first.
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

// Multi reports whether this picker toggles a set or picks one row. The screen
// reads it to know whether the toggle key means anything here.
func (p Picker) Multi() bool { return p.multi }

// Replace swaps the choices under the filter, keeping what has been typed and
// keeping the cursor on the row it was on when that row survives.
//
// A picker whose list comes from the server needs this. The filter is the
// search, so every keystroke brings a different set back, and rebuilding
// through NewPicker would clear the field that caused the fetch.
//
// The cursor is held by id rather than reanchored, which is the opposite of
// what a filter keystroke does and for the opposite reason. Typing is the
// reader narrowing a list and looking at what is left; a response landing is
// not something they did. Moving onto a row while the request is still out and
// having it answer under them would send the write to whichever branch the new
// list sorted first. Only a row the answer no longer carries goes to the top.
//
// The filter row stays whether or not the new list is short enough to have
// earned one: a search that narrows the choices to two must not take away the
// field the reader is typing in. What is checked stays too, since the write
// behind the picker has not changed, and an id the new list does not carry
// simply matches nothing.
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

// SetNote says what the title should say about the list as a whole, leaving the
// list and the cursor where they are.
//
// Replace carries one too and cannot stand in for this. A picker opening over a
// search already answered has something to report before anybody has typed, and
// replacing the list to say it would move the cursor off the row the picker
// deliberately opened on.
func (p *Picker) SetNote(note string) { p.note = note }

// Move walks the cursor. It stops at the ends rather than wrapping: a list
// behind a filter has no fixed length, and a wrap from an empty result set has
// nowhere to land.
func (p *Picker) Move(delta int) {
	shown := p.shown()
	if len(shown) == 0 {
		p.cursor, p.top = 0, 0
		return
	}
	p.cursor = min(max(p.cursor+delta, 0), len(shown)-1)
	p.scroll()
}

// Toggle checks or unchecks the row the cursor is on. It does nothing on a
// single-select picker, where applying is what chooses.
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

// Insert folds a keypress into the filter and reports whether it was one. A
// picker with no filter row takes nothing, so a stray letter falls through to
// the caller rather than filtering a list nobody can see being filtered.
//
// Space never types on a multi-select picker: it is the toggle key there, and a
// filter that swallowed it would leave the reader unable to check anything.
// Substring matching costs them little, since "good" already finds
// "good first issue".
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

	// Text is non-empty only for a keypress that stands for printable
	// characters, and one keypress into a filter is one character: anything
	// longer is a key name that arrived in the wrong field, and typing it would
	// put "down" into the filter when the reader pressed an arrow.
	//
	// The modifier guard is for terminals that report text anyway: ctrl+d is a
	// binding on this screen, not a d.
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

// Filtering reports whether this picker shows a filter row, which is what tells
// the screen whether a bare letter is text or a binding.
func (p Picker) Filtering() bool { return p.filtering }

// Filter is what has been typed. A picker whose choices come from the server
// reads it to know what to ask for: there the filter is the search.
func (p Picker) Filter() string { return p.filter }

// Chosen is what applying selects, by ID. A multi picker gives the whole
// checked set; a single one gives the row under the cursor, or nothing when the
// filter matched nothing.
//
// The set comes back in the order the items were handed over, never the order
// they were checked in. A caller comparing it against what it already holds
// would otherwise see a change every time the reader worked bottom-up.
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

// shown is the items the filter leaves, in the order they were given. The match
// is a case-insensitive substring: a picker filter is for narrowing a list
// already on screen, and fuzzy matching in thirty columns puts rows in an order
// the reader cannot predict.
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

// at is the item under the cursor, or false when the filter matched nothing.
func (p Picker) at() (PickerItem, bool) {
	shown := p.shown()
	if p.cursor < 0 || p.cursor >= len(shown) {
		return PickerItem{}, false
	}
	return shown[p.cursor], true
}

// reanchor puts the cursor back in range after the filter changed what is on
// screen. It goes to the top: the reader typed to narrow the list, and the row
// they want is the one they are now looking at rather than wherever the old
// cursor happens to land.
func (p *Picker) reanchor() {
	p.cursor, p.top = 0, 0
}

// scroll moves the window the least distance that brings the cursor onto it.
// The shortest distance is right here for the reason the compose box gives: a
// cursor moving a row at a time is not being taken anywhere, and hauling the
// list to put it on the top row loses the reader their place.
func (p *Picker) scroll() {
	if p.cursor < p.top {
		p.top = p.cursor
		return
	}
	if p.cursor >= p.top+pickerRows {
		p.top = p.cursor - pickerRows + 1
	}
}

// Render draws the picker as a modal, sized to fit inside a frame of the given
// width. The caller composites it with Over.
//
// The choices sit between two blank rows, and the top one is there whether or
// not a filter row is above it. Every picker then opens with its first choice
// on the same line, and the title in the border does not read as the first
// thing in the list.
func (p Picker) Render(th theme.Theme, frameWidth int) string {
	inner := p.width(frameWidth)
	shown := p.shown()

	var rows []string
	if p.filtering {
		rows = append(rows, p.filterRow(th, inner))
	}
	rows = append(rows, "")
	rows = append(rows, p.list(th, shown, inner)...)
	rows = append(rows, "", p.hint(th, shown, inner))

	return Modal(th, p.heading(), strings.Join(rows, "\n"))
}

// heading is the title with whatever the list has to say about itself.
func (p Picker) heading() string {
	if p.note == "" {
		return p.title
	}
	return p.title + " · " + p.note
}

// width is what the modal gets inside its border: the widest thing it has to
// show, held between a floor and a ceiling and never wider than the frame.
//
// The hint counts, at the longest it can render rather than the shortest. It
// grows a counter once the list outruns the window, and measuring it without
// one clips "esc cancel" off exactly the long lists where the hint is worth
// having.
func (p Picker) width(frameWidth int) int {
	longest := max(lipgloss.Width(p.heading()), lipgloss.Width(p.hintText(len(p.items))))
	for _, it := range p.items {
		longest = max(longest, lipgloss.Width(it.Name)+lipgloss.Width(pickerMark))
	}

	want := min(max(longest, pickerMinWidth), pickerMaxWidth)

	// The modal spends four columns on its border and padding. Below the floor
	// there is nothing worth drawing, and the compositor clips what will not
	// fit rather than growing the frame.
	if room := frameWidth - 4; room > 0 {
		want = min(want, room)
	}
	return max(want, 1)
}

// filterRow is what has been typed, with a block for the caret. The caret is
// drawn rather than blinked: the picker owns the keyboard whenever it is up, so
// there is nothing for a blink to disambiguate.
func (p Picker) filterRow(th theme.Theme, width int) string {
	plain := lipgloss.NewStyle()
	if p.filter == "" {
		return pad(plain.Foreground(th.Faint).Render("Type to filter"), width, plain)
	}
	text := plain.Foreground(th.Primary).Render(p.filter) + plain.Foreground(th.Secondary).Render("▌")
	return pad(Clip(text, width, plain.Foreground(th.Faint)), width, plain)
}

// list is the visible window of choices. Every cell in the cursor row sets the
// background itself: a styled run ends in a reset that clears it, so painting
// the joined row afterwards would color the first cell and nothing else.
func (p Picker) list(th theme.Theme, shown []PickerItem, width int) []string {
	plain := lipgloss.NewStyle()
	if len(shown) == 0 {
		return []string{pad(plain.Foreground(th.Faint).Render("No match"), width, plain)}
	}

	end := min(p.top+pickerRows, len(shown))
	out := make([]string, 0, end-p.top)
	for i := p.top; i < end; i++ {
		it := shown[i]

		base := plain
		if i == p.cursor {
			base = plain.Background(th.SelectedBackground)
		}

		mark := base.Foreground(th.Faint).Render(pickerGap)
		if p.checked[it.ID] {
			mark = base.Foreground(th.Success).Render(pickerMark)
		}

		c := it.Color
		if c == nil {
			c = th.Primary
		}
		name := base.Foreground(c).Render(it.Name)
		room := width - lipgloss.Width(pickerMark)
		if lipgloss.Width(it.Name) > room {
			name = Clip(name, room, base.Foreground(th.Faint))
		}

		out = append(out, pad(mark+name, width, base))
	}
	return out
}

// hint names the keys that work from here, and how much of the list is out of
// sight. Both belong on one line: a modal this size cannot spend two.
func (p Picker) hint(th theme.Theme, shown []PickerItem, width int) string {
	plain := lipgloss.NewStyle()
	faint := plain.Foreground(th.Faint)

	text := faint.Render(p.hintText(len(shown)))
	if lipgloss.Width(p.hintText(len(shown))) > width {
		text = Clip(text, width, faint)
	}
	return pad(text, width, plain)
}

// hintText is the hint as plain characters, which is what the width has to be
// measured against before anything is styled.
func (p Picker) hintText(shown int) string {
	text := "enter pick · esc cancel"
	if p.multi {
		text = "space toggle · enter apply · esc cancel"
	}
	if more := shown - pickerRows; more > 0 {
		return strconv.Itoa(more) + " more · " + text
	}
	return text
}

// pad runs a row out to the full width in its own style, so a cursor row's
// background reaches the border instead of stopping at the last word.
func pad(content string, width int, style lipgloss.Style) string {
	if gap := width - lipgloss.Width(content); gap > 0 {
		return content + style.Render(strings.Repeat(" ", gap))
	}
	return content
}
