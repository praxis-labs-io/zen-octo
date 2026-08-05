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

// glyphFile marks the changed-file count. It comes from the codicons range, the
// same set the draft and closed markers come from.
const glyphFile = "\uea7b" // nf-cod-file

// glyphCheck marks every row in the Checks section. Color carries which state
// it is in: a column of one shape reads down faster than a column of four.
const glyphCheck = "●"

// railBody renders the details column: what is happening to the pull request,
// then who is on it, then what it touches.
//
// Every section renders, empty or not. No reviewer and no label are both facts
// worth reading, and a section that disappears when it has nothing behind it
// reads as one that was never fetched.
//
// The branch is not here. It is the second line of the header, and the rail has
// no room to spend saying it twice.
func railBody(th theme.Theme, d gh.PullRequestDetail, width int) string {
	pr := d.PullRequest
	var blocks []string

	icon, _ := comp.PRStateIcon(th, pr)
	label, c := comp.PRStateLabel(th, pr)

	blocks = append(blocks,
		railSection(th, "State", entry(icon+" "+label, c)),
		railSection(th, "Author", authorEntry(th, pr.Author)),
		railSection(th, "Reviewers", orMany(th, reviewerEntries(th, d.Reviewers, width))...),
		railSection(th, "Assignees", orMany(th, actorEntries(th, d.Assignees))...),
		railSection(th, "Labels", orMany(th, labelEntries(d.Labels))...),
		railSection(th, "Changes", changeEntry(th, pr)),
		// Checks runs to any length, so it goes below everything of a fixed
		// size. The two rows under it are what you read just before merging,
		// which is the other reason they sit at the bottom.
		railSection(th, "Checks", orMany(th, checkEntries(th, d.Rollup, width))...),
		railSection(th, "Base", baseEntry(th, pr.BaseRefName, d.BehindBy)),
		railSection(th, "Merge", mergeEntry(th, d.Merge)),
	)

	return strings.Join(blocks, "\n\n")
}

// changeEntry is the churn and the file count on one row, the count marked with
// a glyph rather than the word: the rail has thirty-odd columns and "files"
// earns none of them.
func changeEntry(th theme.Theme, pr gh.PullRequest) string {
	churn := lipgloss.NewStyle().Foreground(th.Success).Render("+"+strconv.Itoa(pr.Additions)) +
		" " + lipgloss.NewStyle().Foreground(th.Error).Render("−"+strconv.Itoa(pr.Deletions))
	files := lipgloss.NewStyle().Foreground(th.Faint).
		Render(strconv.Itoa(pr.ChangedFiles) + " " + glyphFile)

	return " " + churn + "  " + files
}

// checkEntries is every check on the head commit, each marked with its own
// state. They all take the same dot: a column of four different shapes is
// harder to read down than a column of one.
func checkEntries(th theme.Theme, r gh.CheckRollup, width int) []string {
	faint := lipgloss.NewStyle().Foreground(th.Faint)

	out := make([]string, 0, len(r.Checks))
	for _, check := range r.Checks {
		_, c := comp.CheckStateIcon(th, check.State)
		out = append(out, lipgloss.NewStyle().Foreground(c).Render(" "+glyphCheck)+
			faint.Render(" "+fit(th, checkName(check), width)))
	}
	return out
}

// checkName is what GitHub calls the check on its own page. The job name alone
// is not enough: five suites in a repository each have a job called "test", and
// five identical rows say nothing about which one broke.
func checkName(c gh.Check) string {
	if c.Workflow == "" {
		return c.Name
	}
	return c.Workflow + " / " + c.Name
}

// reviewerEntries marks each reviewer with where they stand. The rail has no
// room for the words, and the same dot the checks use carries it in one cell.
func reviewerEntries(th theme.Theme, reviewers []gh.Reviewer, width int) []string {
	out := make([]string, 0, len(reviewers))
	for _, r := range reviewers {
		out = append(out,
			lipgloss.NewStyle().Foreground(comp.ReviewerColor(th, r)).Render(" "+glyphCheck)+
				lipgloss.NewStyle().Foreground(th.Actor).Render(" "+fit(th, comp.Handle(r.Actor.Login), width)))
	}
	return out
}

// fit clips a name to what is left of a marked row. A bot login runs past the
// rail as readily as a workflow name does, and the rail is a column: wrapping
// turns one row into two that read as two.
func fit(th theme.Theme, name string, width int) string {
	room := max(1, width-3)
	if lipgloss.Width(name) <= room {
		return name
	}
	return comp.Clip(name, room, lipgloss.NewStyle().Foreground(th.Faint))
}

func actorEntries(th theme.Theme, actors []gh.Actor) []string {
	out := make([]string, 0, len(actors))
	for _, a := range actors {
		out = append(out, entry(comp.Handle(a.Login), th.Actor))
	}
	return out
}

// labelEntries colors each label the color GitHub gave it. This is the one
// place the theme does not decide: a label's color is its identity, and the
// same label has to read the same here as in the browser.
//
// It is a foreground, not a filled chip. A background has to be set per cell to
// survive the reset every styled run ends with, and a chip buys nothing a
// colored word does not already say.
func labelEntries(labels []gh.Label) []string {
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		out = append(out, entry(l.Name, lipgloss.Color("#"+l.Color)))
	}
	return out
}

// railSection is a faint heading over its indented entries.
func railSection(th theme.Theme, title string, entries ...string) string {
	head := lipgloss.NewStyle().Foreground(th.Faint).Render(title)
	return head + "\n" + strings.Join(entries, "\n")
}

func entry(text string, c color.Color) string {
	return lipgloss.NewStyle().Foreground(c).Render(" " + text)
}

// authorEntry names who raised it. The login is empty once the account is
// deleted, which is a fact rather than a section to drop.
func authorEntry(th theme.Theme, a gh.Actor) string {
	if a.Login == "" {
		return entry("Unknown", th.Faint)
	}
	return entry(comp.Handle(a.Login), th.Actor)
}

// orMany falls back to the empty note when a section has nothing in it.
func orMany(th theme.Theme, entries []string) []string {
	if len(entries) == 0 {
		return []string{entry("None yet", th.Faint)}
	}
	return entries
}

// baseEntry is how far the branch has fallen behind what it is merging into.
// GitHub says "out-of-date"; the number is the same answer with the size of the
// problem attached.
func baseEntry(th theme.Theme, base string, behindBy int) string {
	if behindBy == 0 {
		return entry("Up to date with "+base, th.Success)
	}
	return entry(comp.Plural(behindBy, "commit")+" behind "+base, th.Warning)
}

func mergeEntry(th theme.Theme, s gh.MergeState) string {
	label, c := comp.MergeStateLabel(th, s)
	return entry(label, c)
}
