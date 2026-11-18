package prview

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

const (
	formMinWidth = 52
	// Matches the picker's, so the modals open at the same width.
	formMaxWidth = 72

	formButtonPad = 2
	formMark      = "✓ "
	formGap       = "  "

	formFieldLead = 2
)

// formWidth fits the longest line between the form's floor and ceiling, and inside the frame.
func formWidth(longest, frameWidth int) int {
	want := min(max(longest, formMinWidth), formMaxWidth)
	if room := frameWidth - 4; room > 0 {
		want = min(want, room)
	}
	return max(want, 1)
}

func fieldText(width int) int { return max(1, width-4) }

func choiceRows(th theme.Theme, heading string, names []string, at int, focused bool, width int) []string {
	plain := lipgloss.NewStyle()

	out := make([]string, 0, len(names)+1)
	out = append(out, formPad(plain.Foreground(th.Text).Bold(true).Render(heading), width, plain))

	for i, name := range names {
		chosen := i == at

		base := plain
		if chosen && focused {
			base = base.Background(th.SelectedBackground)
		}

		mark, c := base.Foreground(th.Subtle).Render(formGap), th.Subtle
		if chosen {
			mark, c = base.Foreground(th.Success).Render(formMark), th.Text
		}

		out = append(out, formPad(mark+base.Foreground(c).Render(name), width, base))
	}
	return out
}

func formField(th theme.Theme, title, content string, lines, width int, focused bool) []string {
	pane := comp.NewPane(th).Title(title).Focus(focused).Size(width, lines+2)
	padded := lipgloss.NewStyle().Padding(0, 1).Render(content)
	return strings.Split(pane.Render(padded), "\n")
}

func formNote(th theme.Theme, text string, c lipgloss.Style, width int) string {
	plain := lipgloss.NewStyle()
	return formPad(clipTo(c.Render(text), width, plain.Foreground(th.Subtle)), width, plain)
}

func formWarning(th theme.Theme, text string, width int) string {
	return formNote(th, text, lipgloss.NewStyle().Foreground(th.Warning), width)
}

// The hint gives way before the button, the only way to send where the terminal cannot send the chord.
func formFooter(th theme.Theme, label, hint string, width int, ready, focused bool) string {
	style := lipgloss.NewStyle().
		Padding(0, formButtonPad).
		Foreground(th.Text).
		Background(th.SelectedBackground)

	switch {
	case !ready:
		style = style.Foreground(th.Subtle)
	case focused:
		style = style.Foreground(th.Inverted).Background(th.Accent)
	}
	button := style.Render(label)

	faint := lipgloss.NewStyle().Foreground(th.Subtle).Render(hint)
	gap := width - lipgloss.Width(hint) - lipgloss.Width(button)
	if gap < 1 {
		faint, gap = "", max(0, width-lipgloss.Width(button))
	}
	return faint + strings.Repeat(" ", gap) + button
}

func formPad(content string, width int, style lipgloss.Style) string {
	if gap := width - lipgloss.Width(content); gap > 0 {
		return content + style.Render(strings.Repeat(" ", gap))
	}
	return content
}
