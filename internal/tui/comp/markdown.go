package comp

import (
	"fmt"
	"hash/fnv"
	"image/color"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// Markdown renders GitHub markdown in the active theme. Rendering a long
// comment thread costs real time, so output is kept.
//
// Render mutates the cache, which means it belongs on an Update path.
// syncContent is where the screens call it; View never does.
type Markdown struct {
	style ansi.StyleConfig

	// cache is keyed by body alone and dropped when the width changes. Glamour
	// wraps at a width, so an entry from another width is wrong rather than
	// stale, and a drag-resize would otherwise leave one dead entry per column
	// it passed through.
	cache map[uint64]string
	width int
}

// NewMarkdown builds a renderer over one theme.
func NewMarkdown(th theme.Theme) Markdown {
	return Markdown{style: markdownStyle(th), cache: make(map[uint64]string)}
}

// Render returns body wrapped to width, in theme colors. A body that fails to
// parse comes back as itself: unstyled text beats an empty pane.
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

// trimBlank drops leading and trailing lines with nothing visible on them.
// Glamour pads every line out to the wrap width, so a block that renders to
// nothing still arrives as a row of spaces and still costs a line. A body
// opening with an HTML comment, which bots write, is the common case.
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

// render wraps to exactly width. Glamour pads every line out to that width, so
// the caller has to hand it the viewport's own width: one column narrower and
// soft wrap puts every line onto two.
func render(style ansi.StyleConfig, body string, width int) string {
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),

		// GitHub renders a single newline in a comment as a line break. CommonMark
		// calls it a soft break and folds it into the paragraph, which puts two
		// lines somebody typed onto one and makes a comment read differently here
		// from the way it reads in the browser it was written in.
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

// markdownStyle dresses glamour in the theme. It builds on the ASCII config
// rather than the dark one because that is the only stock config carrying no
// colors of its own: patching the dark style leaves every element we did not
// think of rendering in a palette nobody chose.
//
// Code blocks get no Chroma. A syntax theme is a second palette that is not the
// theme, and the diff viewer needs a colorizer of its own anyway.
func markdownStyle(th theme.Theme) ansi.StyleConfig {
	s := styles.ASCIIStyleConfig

	// The pane already indents and the viewport already wraps to its width. A
	// margin here spends columns twice and pushes the wrap point off.
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

	// The ASCII config brackets emphasis in the markers themselves, for
	// terminals that cannot show weight. Ours can, so they are just noise.
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

// hex renders a theme color the way glamour wants it. Its style config is JSON
// shaped, so colors arrive as strings rather than as color.Color.
func hex(c color.Color) *string {
	if c == nil {
		return nil
	}
	r, g, b, _ := c.RGBA()
	s := fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
	return &s
}

func boolPtr(v bool) *bool       { return &v }
func uintPtr(v uint) *uint       { return &v }
func stringPtr(v string) *string { return &v }
