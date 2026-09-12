package comp

import (
	"fmt"
	"hash/fnv"
	"image/color"
	"strconv"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

// Markdown renders GitHub markdown in the theme and caches the output.
type Markdown struct {
	style ansi.StyleConfig

	// Dropped whenever the width changes: glamour output belongs to the width it was rendered at.
	cache map[uint64]string
	width int
}

func NewMarkdown(th theme.Theme) Markdown {
	return Markdown{style: markdownStyle(th), cache: make(map[uint64]string)}
}

// Render returns body wrapped to width in theme colors, caching it, so it belongs on an Update path.
// A body glamour cannot render comes back unchanged.
func (m *Markdown) Render(body string, width int) string {
	if width <= 0 || strings.TrimSpace(body) == "" {
		return ""
	}
	if width != m.width {
		m.cache, m.width = make(map[uint64]string), width
	}

	h := fnv.New64a()
	_, _ = h.Write([]byte(body))
	key := h.Sum64()

	if out, ok := m.cache[key]; ok {
		return out
	}

	out := trimBlank(render(m.style, body, width))
	m.cache[key] = out
	return out
}

// Glamour pads blank lines to the wrap width, so a block rendering to nothing still costs rows.
func trimBlank(s string) string {
	lines := strings.Split(s, "\n")
	blank := func(line string) bool { return strings.TrimSpace(xansi.Strip(line)) == "" }

	for len(lines) > 0 && blank(lines[0]) {
		lines = lines[1:]
	}
	for len(lines) > 0 && blank(lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// Glamour pads every line to width, so width must be the viewport's own or soft wrap doubles each line.
func render(style ansi.StyleConfig, body string, width int) string {
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),

		glamour.WithPreservedNewLines(),
	)
	if err != nil {
		return body
	}
	out, err := r.Render(body)
	if err != nil {
		return body
	}
	return out
}

// Built on the ASCII config, the only stock glamour style carrying no colors of its own.
func markdownStyle(th theme.Theme) ansi.StyleConfig {
	s := styles.ASCIIStyleConfig

	s.Document.Margin = uintPtr(0)
	s.Document.BlockPrefix = ""
	s.Document.BlockSuffix = ""

	s.Document.Color = hex(th.Text)
	s.Text.Color = hex(th.Text)
	s.Paragraph.Color = hex(th.Text)
	s.Item.Color = hex(th.Text)
	s.Enumeration.Color = hex(th.Accent)

	s.Heading.Color = hex(th.Accent)
	s.Heading.Bold = boolPtr(true)
	for _, h := range []*ansi.StyleBlock{&s.H1, &s.H2, &s.H3, &s.H4, &s.H5, &s.H6} {
		h.Color = hex(th.Accent)
		h.Bold = boolPtr(true)
		h.BackgroundColor = nil
	}

	s.Strong.BlockPrefix, s.Strong.BlockSuffix = "", ""
	s.Emph.BlockPrefix, s.Emph.BlockSuffix = "", ""

	s.Strong.Color = hex(th.Text)
	s.Strong.Bold = boolPtr(true)
	s.Emph.Color = hex(th.Text)
	s.Emph.Italic = boolPtr(true)
	s.Strikethrough.Color = hex(th.Subtle)
	s.Strikethrough.CrossedOut = boolPtr(true)

	s.Link.Color = hex(th.Accent)
	s.Link.Underline = boolPtr(true)
	s.LinkText.Color = hex(th.Accent)
	s.Image.Color = hex(th.Accent)
	s.ImageText.Color = hex(th.Subtle)

	s.BlockQuote.Color = hex(th.Subtle)
	s.BlockQuote.Italic = boolPtr(true)
	s.BlockQuote.IndentToken = stringPtr("│ ")
	s.HorizontalRule.Color = hex(th.BorderMutedOrSubtle())
	s.Task.Ticked, s.Task.Unticked = "[✓] ", "[ ] "

	s.Code.Color = hex(th.Warning)
	s.CodeBlock.Color = hex(th.Warning)
	s.CodeBlock.Chroma = nil

	s.Table.Color = hex(th.Text)
	s.DefinitionTerm.Color = hex(th.Accent)
	s.DefinitionDescription.Color = hex(th.Text)
	s.HTMLBlock.Color = hex(th.Subtle)
	s.HTMLSpan.Color = hex(th.Subtle)

	return s
}

// A slot goes as its bare index so lipgloss.Color keeps it a slot; NoColor has no spelling, as its RGBA is black.
func hex(c color.Color) *string {
	switch c := c.(type) {
	case nil:
		return nil
	case lipgloss.NoColor:
		return nil
	case xansi.BasicColor:
		s := strconv.Itoa(int(c))
		return &s
	case xansi.IndexedColor:
		s := strconv.Itoa(int(c))
		return &s
	}
	r, g, b, _ := c.RGBA()
	s := fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
	return &s
}

func boolPtr(v bool) *bool       { return &v }
func uintPtr(v uint) *uint       { return &v }
func stringPtr(v string) *string { return &v }
