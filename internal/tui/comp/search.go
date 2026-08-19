package comp

import (
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Search is a small in-content search state. It never hides rows: callers ask
// it to highlight the text they already render and use Cursor to decide which
// matching line to bring into view.
type Search struct {
	query  string
	cursor int
}

func (s Search) Query() string { return s.query }
func (s Search) Empty() bool   { return s.query == "" }
func (s Search) Cursor() int   { return s.cursor }

// Insert folds printable text and the two editing keys into the query.
func (s *Search) Insert(msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case "backspace":
		if s.query != "" {
			r := []rune(s.query)
			s.query = string(r[:len(r)-1])
			s.cursor = 0
		}
		return true
	case "ctrl+u":
		s.query, s.cursor = "", 0
		return true
	}
	if utf8.RuneCountInString(msg.Text) != 1 || msg.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper) != 0 {
		return false
	}
	s.query += msg.Text
	s.cursor = 0
	return true
}

func (s Search) Matches(text string) bool {
	return s.query != "" && strings.Contains(strings.ToLower(text), strings.ToLower(s.query))
}

// Move advances through count matching lines and wraps at either end, the way
// repeated n does in an editor.
func (s *Search) Move(delta, count int) bool {
	if count <= 0 || s.query == "" {
		return false
	}
	s.cursor = (s.cursor + delta%count + count) % count
	return true
}

// Highlight paints every case-insensitive occurrence without changing any
// text around it.
func (s Search) Highlight(text string, mark lipgloss.Style) string {
	if s.query == "" {
		return text
	}
	lower, query := strings.ToLower(text), strings.ToLower(s.query)
	var out strings.Builder
	for {
		at := strings.Index(lower, query)
		if at < 0 {
			out.WriteString(text)
			return out.String()
		}
		end := at + len(query)
		if end > len(text) {
			out.WriteString(text)
			return out.String()
		}
		out.WriteString(text[:at])
		out.WriteString(mark.Render(text[at:end]))
		text, lower = text[end:], lower[end:]
	}
}
