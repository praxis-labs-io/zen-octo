package prview

import (
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// railBody renders the details column. Sections with nothing behind them are
// left out rather than shown empty, so the rail never pads itself with fields
// this build does not fetch.
func railBody(th theme.Theme, pr gh.PullRequest) string {
	var blocks []string

	if label, c := comp.PRStateLabel(th, pr); label != "" {
		blocks = append(blocks, railSection(th, "State", entry(label, c)))
	}

	if label, c := comp.CheckStateLabel(th, pr.Checks); label != "" {
		glyph, _ := comp.CheckStateIcon(th, pr.Checks)
		blocks = append(blocks, railSection(th, "Checks", entry(glyph+" "+label, c)))
	}

	if label, c := comp.ReviewLabel(th, pr.ReviewDecision); label != "" {
		blocks = append(blocks, railSection(th, "Review", entry(label, c)))
	}

	if pr.Author.Login != "" {
		blocks = append(blocks, railSection(th, "Author", entry(pr.Author.Login, th.Actor)))
	}

	if pr.BaseRefName != "" {
		blocks = append(blocks, railSection(th, "Branch",
			entry(pr.BaseRefName, th.Primary),
			entry("← "+pr.HeadRefName, th.Faint),
		))
	}

	if pr.ChangedFiles > 0 {
		blocks = append(blocks, railSection(th, "Changes",
			entry("+"+strconv.Itoa(pr.Additions)+" −"+strconv.Itoa(pr.Deletions), th.Success),
			entry(plural(pr.ChangedFiles, "file"), th.Faint),
		))
	}

	return strings.Join(blocks, "\n\n")
}

// railSection is a faint heading over its indented entries.
func railSection(th theme.Theme, title string, entries ...string) string {
	head := lipgloss.NewStyle().Foreground(th.Faint).Render(title)
	return head + "\n" + strings.Join(entries, "\n")
}

func entry(text string, c color.Color) string {
	return lipgloss.NewStyle().Foreground(c).Render(" " + text)
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}
