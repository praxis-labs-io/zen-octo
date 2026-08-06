package comp

import (
	"hash/fnv"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// Token is one run of code sharing a color. Color is nil where the style has
// nothing to say, which is most of the punctuation and whitespace in a file.
type Token struct {
	Text  string
	Color color.Color
}

// Syntax colors source code. It returns tokens rather than rendered text
// because the caller owns the rest of the line's style: a diff paints a
// background per cell, and a token that rendered itself would end in a reset
// and tear a hole in it.
//
// Chroma's own terminal formatter is unusable here for that reason.
//
// Lines mutates the cache, so it belongs on an Update path.
type Syntax struct {
	style *chroma.Style
	cache map[uint64][][]Token
}

// NewSyntax builds a colorizer over a Chroma style, reporting whether the name
// was one Chroma knows. An unknown name still yields a working colorizer, so a
// typo in config degrades to different colors rather than no diff.
func NewSyntax(name string) (Syntax, bool) {
	_, ok := styles.Registry[name]
	return Syntax{style: styles.Get(name), cache: make(map[uint64][][]Token)}, ok || name == ""
}

// SyntaxNames lists the styles Chroma ships, for the message that follows a
// name it did not recognise.
func SyntaxNames() []string { return styles.Names() }

// Lines splits code into lines of colored tokens. The lexer is chosen from the
// path, and the whole body is tokenised at once: a lexer carries state across
// lines, so a multi-line string highlighted line by line comes apart.
func (s *Syntax) Lines(path, code string) [][]Token {
	h := fnv.New64a()
	_, _ = h.Write([]byte(path))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(code))
	key := h.Sum64()

	if out, ok := s.cache[key]; ok {
		return out
	}

	out := s.tokenise(path, code)
	s.cache[key] = out
	return out
}

func (s *Syntax) tokenise(path, code string) [][]Token {
	lexer := lexers.Match(path)
	if lexer == nil {
		lexer = lexers.Fallback
	}

	iter, err := chroma.Coalesce(lexer).Tokenise(nil, code)
	if err != nil {
		return plain(code)
	}

	var out [][]Token
	for _, line := range chroma.SplitTokensIntoLines(iter.Tokens()) {
		row := make([]Token, 0, len(line))
		for _, t := range line {
			// Only the foreground is read. A Chroma style carries a background
			// of its own, and taking it would paint over the terminal's, which
			// is what keeps a transparent one transparent.
			text := strings.TrimSuffix(t.Value, "\n")
			if text == "" {
				continue
			}
			row = append(row, Token{Text: text, Color: colorOf(s.style.Get(t.Type).Colour)})
		}
		out = append(out, row)
	}
	return out
}

// plain is the fallback when a lexer fails outright: uncolored code beats no
// code.
func plain(code string) [][]Token {
	lines := strings.Split(code, "\n")
	out := make([][]Token, len(lines))
	for i, line := range lines {
		out[i] = []Token{{Text: line}}
	}
	return out
}

func colorOf(c chroma.Colour) color.Color {
	if !c.IsSet() {
		return nil
	}
	return lipgloss.Color(c.String())
}
