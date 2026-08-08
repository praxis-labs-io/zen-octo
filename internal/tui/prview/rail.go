package prview

import (
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
)

// glyphFile marks the changed-file count. It comes from the codicons range, the
// same set the draft and closed markers come from.
const glyphFile = "" // nf-cod-file

// glyphCheck marks every row in the Checks section. Color carries which state
// it is in: a column of one shape reads down faster than a column of four.
const glyphCheck = "●"

// railNameRoom is what a marked row leaves a name: the gutter, the mark and the
// space after it on the left, and one column short of the border on the right.
const railNameRoom = 4

// railBody renders the details column: what is happening to the pull request,
// then who is on it, then what it touches.
//
// Every section renders, empty or not. No reviewer and no label are both facts
// worth reading, and a section that disappears when it has nothing behind it
// reads as one that was never fetched.
//
// The branch is not here. It is the second line of the header, and the rail has
// no room to spend saying it twice.
//
// It records the ring as it goes. The four sections a later milestone acts on
// are walkable; the rest state a fact and there is nothing to do to them.
//
// Rows are handed over already the full width, gutter and all, the same way the
// file column hands over its own. The selected row is a background, and one
// stops at the last word unless the row runs to the border.
func (m *Model) railBody(width int) string {
	d := m.railDetail()
	pr := d.PullRequest

	m.railRing.reset()

	at := 0
	var blocks []string

	// section lays a heading over its rows and records the ones there is
	// something to do to. Rows carry their own key rather than taking one from
	// the section, because a section can hold two kinds: the reviewers on it
	// and the row that adds another.
	section := func(title string, rows []railEntry) {
		lines := make([]string, len(rows))
		for i, r := range rows {
			if r.key.kind != focusNone {
				m.railRing.add(r.key, at+1+i, 1)
			}
			lines[i] = r.line
		}
		head := m.faint().Render(strings.Repeat(" ", railGutter) + title)
		blocks = append(blocks, head+"\n"+strings.Join(lines, "\n"))
		at += len(rows) + 2
	}

	icon, _ := comp.PRStateIcon(m.theme, pr)
	label, c := comp.PRStateLabel(m.theme, pr)

	section("State", m.railControl(focusState, icon+" "+label, c, width))
	section("Author", m.authorRow(pr.Author, width))
	section("Reviewers", m.reviewerRows(d.Reviewers, width))
	section("Assignees", m.actorRows(d.Assignees, width))
	section("Labels", m.labelRows(d.Labels, width))
	section("Changes", m.changeRow(pr, width))
	// Checks runs to any length, so it goes below everything of a fixed size.
	// The two rows under it are what you read just before merging, which is the
	// other reason they sit at the bottom.
	section("Checks", m.checkRows(d.Rollup, width))
	section("Base", m.baseRow(pr.BaseRefName, d.BehindBy, width))
	section("Merge", m.mergeRow(d.Merge, width))

	return strings.Join(blocks, "\n\n")
}

// railEntry is one rendered row and what the ring calls it. A key of no kind is
// a row tab walks past.
type railEntry struct {
	line string
	key  focusKey
}

// railBase is the style every cell in a row is rendered against. The selected
// row carries a background, and it has to be set on each cell: every styled run
// ends in a reset that clears the background with it, so a joined row wrapped in
// the background afterwards paints only its first cell.
func (m Model) railBase(selected bool) lipgloss.Style {
	if selected {
		return lipgloss.NewStyle().Background(m.theme.SelectedBackground)
	}
	return lipgloss.NewStyle()
}

// railLine is one row: its gutter, its content, and the padding that runs the
// cursor line out to the border rather than stopping at the last word.
func (m Model) railLine(base lipgloss.Style, content string, width int) string {
	return m.padTo(base.Render(strings.Repeat(" ", railGutter))+content, width, base)
}

// railFact is a one-row section stating something about the pull request. There
// is nothing to do to it, so the ring walks past.
func (m Model) railFact(text string, c color.Color, width int) []railEntry {
	base := m.railBase(false)
	return []railEntry{{line: m.railLine(base, base.Foreground(c).Render(text), width)}}
}

// railControl is a one-row section that is also something to act on. State is
// one: draft and ready, closed and reopened are all changes to what it says.
// Merge is the other.
func (m Model) railControl(kind focusKind, text string, c color.Color, width int) []railEntry {
	key := focusKey{kind: kind}
	base := m.railBase(m.railRing.focused(key))
	return []railEntry{{line: m.railLine(base, base.Foreground(c).Render(text), width), key: key}}
}

// addRow opens the picker for a section. It sits under whatever is already
// there and stands in for the empty note, because a section with none of
// something and no way to add one has nothing to say.
func (m Model) addRow(kind focusKind, label string, width int) railEntry {
	key := focusKey{kind: kind}
	base := m.railBase(m.railRing.focused(key))
	return railEntry{
		line: m.railLine(base, base.Foreground(m.theme.Faint).Render("+ "+label), width),
		key:  key,
	}
}

// changeRow is the churn and the file count on one row, the count marked with a
// glyph rather than the word: the rail has thirty-odd columns and "files" earns
// none of them.
func (m Model) changeRow(pr gh.PullRequest, width int) []railEntry {
	base := m.railBase(false)

	churn := base.Foreground(m.theme.Success).Render("+"+strconv.Itoa(pr.Additions)) +
		base.Render(" ") + base.Foreground(m.theme.Error).Render("−"+strconv.Itoa(pr.Deletions))
	files := base.Foreground(m.theme.Faint).
		Render("  " + strconv.Itoa(pr.ChangedFiles) + " " + glyphFile)

	return []railEntry{{line: m.railLine(base, churn+files, width)}}
}

// checkRows is every check on the head commit, each marked with its own state.
// They all take the same dot: a column of four different shapes is harder to
// read down than a column of one.
func (m Model) checkRows(r gh.CheckRollup, width int) []railEntry {
	// The one section with no add row. A check is something a workflow runs,
	// not something to put on the pull request, so the note stands alone.
	if len(r.Checks) == 0 {
		key := focusKey{kind: focusCheck}
		base := m.railBase(m.railRing.focused(key))
		return []railEntry{{
			line: m.railLine(base, base.Foreground(m.theme.Faint).Render("None yet"), width),
			key:  key,
		}}
	}

	out := make([]railEntry, 0, len(r.Checks))
	for i, check := range r.Checks {
		key := focusKey{kind: focusCheck, index: i}
		base := m.railBase(m.railRing.focused(key))
		_, c := comp.CheckStateIcon(m.theme, check.State)

		faint := base.Foreground(m.theme.Faint)
		out = append(out, railEntry{
			line: m.railLine(base, base.Foreground(c).Render(glyphCheck)+faint.Render(" ")+
				m.fit(faint, checkName(check), width), width),
			key: key,
		})
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

// reviewerRows marks each reviewer with where they stand. The rail has no room
// for the words, and the same dot the checks use carries it in one cell.
func (m Model) reviewerRows(reviewers []gh.Reviewer, width int) []railEntry {
	out := make([]railEntry, 0, len(reviewers)+1)
	for i, r := range reviewers {
		key := focusKey{kind: focusReviewer, index: i}
		base := m.railBase(m.railRing.focused(key))
		out = append(out, railEntry{
			line: m.railLine(base,
				base.Foreground(comp.ReviewerColor(m.theme, r)).Render(glyphCheck)+
					base.Render(" ")+
					m.fit(base.Foreground(m.theme.Actor), comp.Handle(r.Actor.Login), width), width),
			key: key,
		})
	}
	return append(out, m.addRow(focusAddReviewer, "Add reviewer", width))
}

// fit renders a name and clips it to what is left of a marked row. A bot login
// runs past the rail as readily as a workflow name does, and the rail is a
// column: wrapping turns one row into two that read as two.
//
// It renders before it clips, the way the file column does. Clipping the plain
// text and styling the result puts the ellipsis inside the run, and the cursor
// line stops at it.
func (m Model) fit(style lipgloss.Style, name string, width int) string {
	room := max(1, width-railNameRoom)

	out := style.Render(name)
	if lipgloss.Width(out) > room {
		return comp.Clip(out, room, style.Foreground(m.theme.Faint))
	}
	return out
}

func (m Model) actorRows(actors []gh.Actor, width int) []railEntry {
	out := make([]railEntry, 0, len(actors)+1)
	for i, a := range actors {
		key := focusKey{kind: focusAssignee, index: i}
		base := m.railBase(m.railRing.focused(key))
		out = append(out, railEntry{
			line: m.railLine(base,
				m.fit(base.Foreground(m.theme.Actor), comp.Handle(a.Login), width), width),
			key: key,
		})
	}
	return append(out, m.addRow(focusAddAssignee, "Add assignee", width))
}

// labelRows colors each label the color GitHub gave it. This is the one place
// the theme does not decide: a label's color is its identity, and the same label
// has to read the same here as in the browser.
//
// It is a foreground, not a filled chip. A chip buys nothing a colored word does
// not already say, and the row already spends its background on the cursor.
func (m Model) labelRows(labels []gh.Label, width int) []railEntry {
	out := make([]railEntry, 0, len(labels)+1)
	for i, l := range labels {
		key := focusKey{kind: focusLabel, index: i}
		base := m.railBase(m.railRing.focused(key))
		out = append(out, railEntry{
			line: m.railLine(base,
				m.fit(base.Foreground(lipgloss.Color("#"+l.Color)), l.Name, width), width),
			key: key,
		})
	}
	return append(out, m.addRow(focusAddLabel, "Add label", width))
}

// authorRow names who raised it. The login is empty once the account is
// deleted, which is a fact rather than a section to drop.
func (m Model) authorRow(a gh.Actor, width int) []railEntry {
	if a.Login == "" {
		return m.railFact("Unknown", m.theme.Faint, width)
	}
	return m.railFact(comp.Handle(a.Login), m.theme.Actor, width)
}

// baseRow is how far the branch has fallen behind what it is merging into.
// GitHub says "out-of-date"; the number is the same answer with the size of the
// problem attached.
func (m Model) baseRow(base string, behindBy, width int) []railEntry {
	if behindBy == 0 {
		return m.railFact("Up to date with "+base, m.theme.Success, width)
	}
	return m.railFact(comp.Plural(behindBy, "commit")+" behind "+base, m.theme.Warning, width)
}

// mergeRow is whether it can be merged, and what is in the way if it cannot.
// The ring stops here for the same reason it stops on the state: merging is a
// change to what this row says, and this is the row that says it.
func (m Model) mergeRow(s gh.MergeState, width int) []railEntry {
	label, c := comp.MergeStateLabel(m.theme, s)
	return m.railControl(focusMerge, label, c, width)
}
