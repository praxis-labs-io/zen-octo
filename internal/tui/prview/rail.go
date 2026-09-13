package prview

import (
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/paint"
)

const glyphFile = "" // nf-cod-file

const glyphCheck = "●"

const markLead = 2

func (m *Model) railBody(width int) string {
	d := m.railDetail()
	pr := d.PullRequest

	m.railRing.reset()

	at := 0
	var blocks []string

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

	section("State", m.stateRow(d, width))
	section("Author", m.authorRow(pr.Author, width))
	section("Reviewers", m.reviewerRows(d.Reviewers, width))
	section("Assignees", m.actorRows(d, width))
	section("Labels", m.labelRows(d.Labels, width))
	section("Changes", m.changeRow(pr, width))
	section("Checks", m.checkRows(d.Rollup, width))
	section("Base", m.baseRow(d, width))
	section("Merge", m.mergeRow(d, width))

	return strings.Join(blocks, "\n\n")
}

type railEntry struct {
	line string
	key  focusKey
}

func (m Model) railRow(selected bool) (lipgloss.Style, bool) {
	if selected && m.focus == paneRail {
		return lipgloss.NewStyle().Background(m.theme.SelectedBackground), true
	}
	return lipgloss.NewStyle(), false
}

func (m Model) railLine(base lipgloss.Style, lit bool, content string, width int) string {
	var bar color.Color
	if lit {
		bar = m.theme.Accent
	}
	gutter := base.Render(strings.Repeat(" ", max(0, railGutter-1)))
	return m.padTo(paint.Lead(bar, base)+gutter+content, width, base)
}

func (m Model) railFact(text string, c color.Color, width int) []railEntry {
	base, lit := m.railRow(false)
	return []railEntry{{line: m.railLine(base, lit, base.Foreground(c).Render(text), width)}}
}

func (m Model) railControl(kind focusKind, text string, c color.Color, width int) []railEntry {
	key := focusKey{kind: kind}
	base, lit := m.railRow(m.railRing.focused(key))
	return []railEntry{{line: m.railLine(base, lit, base.Foreground(c).Render(text), width), key: key}}
}

// Stays a control while a lifecycle write is out: the optimistic state moves and the permissions do not.
func (m Model) stateRow(d gh.PullRequestDetail, width int) []railEntry {
	icon, _ := comp.PRStateIcon(m.theme, d.PullRequest)
	label, c := comp.PRStateLabel(m.theme, d.PullRequest)
	text := icon + " " + label

	if m.detail.Loaded && !m.detail.StateWriting && len(stateChoices(d)) == 0 {
		return m.railFact(text, c, width)
	}
	return m.railControl(focusState, text, c, width)
}

func (m Model) addRow(kind focusKind, label string, width int) railEntry {
	key := focusKey{kind: kind}
	base, lit := m.railRow(m.railRing.focused(key))
	return railEntry{
		line: m.railLine(base, lit, base.Foreground(m.theme.Subtle).Render("+ "+label), width),
		key:  key,
	}
}

func (m Model) changeRow(pr gh.PullRequest, width int) []railEntry {
	base, lit := m.railRow(false)

	churn := base.Foreground(m.theme.Success).Render("+"+strconv.Itoa(pr.Additions)) +
		base.Render(" ") + base.Foreground(m.theme.Error).Render("−"+strconv.Itoa(pr.Deletions))
	files := base.Foreground(m.theme.Subtle).
		Render("  " + strconv.Itoa(pr.ChangedFiles) + " " + glyphFile)

	return []railEntry{{line: m.railLine(base, lit, churn+files, width)}}
}

func (m Model) checkRows(r gh.CheckRollup, width int) []railEntry {
	if len(r.Checks) == 0 {
		base, lit := m.railRow(false)
		return []railEntry{{
			line: m.railLine(base, lit, base.Foreground(m.theme.Subtle).Render("None yet"), width),
		}}
	}

	checks := newestAttempt(r.Checks)
	out := make([]railEntry, 0, len(checks))
	for _, check := range checks {
		key := focusKey{kind: focusCheck, id: check.Key()}
		base, lit := m.railRow(m.railRing.focused(key))
		glyph, c := comp.CheckStateIcon(m.theme, check.State)

		faint := base.Foreground(m.theme.Subtle)
		out = append(out, railEntry{
			line: m.railLine(base, lit, base.Foreground(c).Render(glyph)+faint.Render(" ")+
				m.fit(faint, checkName(check), railNameRoom(width, markLead)), width),
			key: key,
		})
	}
	return out
}

func checkName(c gh.Check) string {
	if c.Workflow == "" {
		return c.Name
	}
	return c.Workflow + " / " + c.Name
}

func (m Model) reviewerRows(reviewers []gh.Reviewer, width int) []railEntry {
	out := make([]railEntry, 0, len(reviewers)+1)
	for _, r := range reviewers {
		key := focusKey{kind: focusReviewer, id: r.Actor.Login}
		base, lit := m.railRow(m.railRing.focused(key))
		out = append(out, railEntry{
			line: m.railLine(base, lit,
				base.Foreground(comp.ReviewerColor(m.theme, r)).Render(glyphCheck)+
					base.Render(" ")+
					m.fit(base.Foreground(m.theme.Actor), comp.Handle(r.Actor.Login), railNameRoom(width, markLead)), width),
			key: key,
		})
	}
	return append(out, m.addRow(focusAddReviewer, "Add reviewer", width))
}

func railNameRoom(width, lead int) int {
	return max(1, width-railGutter-lead-1)
}

func (m Model) fit(style lipgloss.Style, name string, room int) string {
	return clipTo(style.Render(name), room, style.Foreground(m.theme.Subtle))
}

func (m Model) actorRows(d gh.PullRequestDetail, width int) []railEntry {
	actors := d.Assignees

	assignable := !m.detail.Loaded || (d.Viewer.CanAssign && d.Viewer.CanUpdate)

	out := make([]railEntry, 0, len(actors)+1)
	for _, a := range actors {
		var key focusKey
		if assignable {
			key = focusKey{kind: focusAssignee, id: a.Login}
		}
		base, lit := m.railRow(m.railRing.focused(key))
		out = append(out, railEntry{
			line: m.railLine(base, lit,
				m.fit(base.Foreground(m.theme.Actor), comp.Handle(a.Login), railNameRoom(width, 0)), width),
			key: key,
		})
	}

	if !assignable {
		if len(out) == 0 {
			return m.railFact("None", m.theme.Subtle, width)
		}
		return out
	}
	return append(out, m.addRow(focusAddAssignee, "Add assignee", width))
}

func (m Model) labelRows(labels []gh.Label, width int) []railEntry {
	out := make([]railEntry, 0, len(labels)+1)
	for _, l := range labels {
		key := focusKey{kind: focusLabel, id: l.Name}
		base, lit := m.railRow(m.railRing.focused(key))
		out = append(out, railEntry{
			line: m.railLine(base, lit,
				m.fit(base.Foreground(m.theme.Accent), l.Name, railNameRoom(width, 0)), width),
			key: key,
		})
	}
	return append(out, m.addRow(focusAddLabel, "Add label", width))
}

func (m Model) authorRow(a gh.Actor, width int) []railEntry {
	if a.Login == "" {
		return m.railFact("Unknown", m.theme.Subtle, width)
	}
	return m.railFact(comp.Handle(a.Login), m.theme.Actor, width)
}

// Keeps the lifecycle write guard, or an optimistic merge drops this ring stop from under the reader.
func (m Model) baseRow(d gh.PullRequestDetail, width int) []railEntry {
	base := d.BaseRefName

	text, c := "Up to date with "+base, m.theme.Success
	switch {
	case m.detail.BaseWriting:
		text, c = "Retargeting to "+base, m.theme.Subtle

	case d.BehindBy == gh.BehindUnknown:
		text, c = "Merging into "+base, m.theme.Subtle

	case d.BehindBy == gh.BehindNoHead:
		text, c = "Based on "+base, m.theme.Subtle

	case d.BehindBy > 0:
		text, c = comp.Plural(d.BehindBy, "commit")+" behind "+base, m.theme.Warning
	}

	if m.detail.Loaded && !m.detail.StateWriting && (!d.Viewer.CanUpdate || d.State == gh.PRStateMerged) {
		return m.railFact(text, c, width)
	}
	return m.railControl(focusBase, text, c, width)
}

func (m Model) mergeRow(d gh.PullRequestDetail, width int) []railEntry {
	text, c := comp.MergeStateLabel(m.theme, d.Merge, d.Checks)
	if d.State == gh.PRStateMerged {
		text, c = "Merged into "+d.BaseRefName, m.theme.Accent
	}

	if m.detail.Loaded && !m.detail.StateWriting && !mergeable(d) {
		return m.railFact(text, c, width)
	}
	return m.railControl(focusMerge, text, c, width)
}

// Lifecycle first: GitHub answers CLEAN on a closed pull request too.
func mergeable(d gh.PullRequestDetail) bool {
	if d.State != gh.PRStateOpen {
		return false
	}
	switch d.Merge {
	case gh.MergeClean, gh.MergeUnstable:
		return true
	case gh.MergeBlocked, gh.MergeBehind:
		return d.Viewer.CanMergeAsAdmin
	}
	return false
}
