package comp

import (
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Search is an in-content search query and match cursor. It highlights matches and never hides rows.
type Search struct {
	query  string
	cursor int
}

func (s Search) Query() string { return s.query }
func (s Search) Empty() bool   { return s.query == "" }
func (s Search) Cursor() int   { return s.cursor }

// Insert applies a printable key, backspace, or ctrl+u to the query, reporting whether it took the key.
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
	if s.query == "" {
		return false
	}
	for at := range text {
		if foldPrefix(text[at:], s.query) {
			return true
		}
	}
	return false
}

func foldPrefix(text, query string) bool {
	for query != "" {
		if text == "" {
			return false
		}
		got, gotSize := utf8.DecodeRuneInString(text)
		want, wantSize := utf8.DecodeRuneInString(query)
		if unicode.ToLower(got) != unicode.ToLower(want) {
			return false
		}
		text, query = text[gotSize:], query[wantSize:]
	}
	return true
}

// Move steps the cursor by delta through count matching lines, wrapping at either end.
func (s *Search) Move(delta, count int) bool {
	if count <= 0 || s.query == "" {
		return false
	}
	s.cursor = (s.cursor + delta%count + count) % count
	return true
}

// Highlight paints every case-insensitive occurrence of the query in text with mark.
func (s Search) Highlight(text string, mark lipgloss.Style) string {
	ranges := s.matchRanges(text)
	if len(ranges) == 0 {
		return text
	}
	runes := []rune(text)
	var out strings.Builder
	at := 0
	for _, match := range ranges {
		out.WriteString(string(runes[at:match.start]))
		out.WriteString(mark.Render(string(runes[match.start:match.end])))
		at = match.end
	}
	out.WriteString(string(runes[at:]))
	return out.String()
}

type searchRange struct{ start, end int }

func (s Search) matchRanges(text string) []searchRange {
	if s.query == "" {
		return nil
	}
	textRunes, queryRunes := []rune(text), []rune(s.query)
	if len(queryRunes) == 0 || len(queryRunes) > len(textRunes) {
		return nil
	}
	var out []searchRange
	for at := 0; at+len(queryRunes) <= len(textRunes); {
		matched := true
		for i, want := range queryRunes {
			if unicode.ToLower(textRunes[at+i]) != unicode.ToLower(want) {
				matched = false
				break
			}
		}
		if !matched {
			at++
			continue
		}
		out = append(out, searchRange{start: at, end: at + len(queryRunes)})
		at += len(queryRunes)
	}
	return out
}
