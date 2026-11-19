package prview

import (
	"cmp"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

// PostThreadMsg asks the root to open a review thread. Thread names the viewer's pending review when
// there is one and the pull request when there is not, so GitHub opens one to hold it.
type PostThreadMsg struct {
	ID     string
	Thread gh.ReviewThreadInput
}

func (m Model) threadWords() words {
	if m.detail.Detail.DraftReview.ID != "" {
		return words{placeholder: "Leave a comment", button: "Add to review", send: "add to review", back: "done"}
	}
	return words{placeholder: "Leave a comment", button: "Start a review", send: "start a review", back: "done"}
}

type spot struct {
	path string
	file bool
	from anchor
	to   anchor
}

func (s spot) key() focusKey {
	if s.file {
		return focusKey{kind: focusNewThread, id: s.path}
	}
	return focusKey{kind: focusNewThread, id: strings.Join([]string{
		s.path,
		string(s.from.side), strconv.Itoa(s.from.line),
		string(s.to.side), strconv.Itoa(s.to.line),
	}, ":")}
}

func (s spot) input() gh.ReviewThreadInput {
	if s.file {
		return gh.ReviewThreadInput{Path: s.path, Subject: gh.SubjectFile}
	}
	return gh.ReviewThreadInput{
		Path: s.path, Subject: gh.SubjectLine,
		Line: s.to.line, Side: s.to.side,
		StartLine: s.from.line, StartSide: s.from.side,
	}
}

func spotOf(in gh.ReviewThreadInput) spot {
	if in.Subject == gh.SubjectFile {
		return spot{path: in.Path, file: true}
	}
	to := anchor{side: cmp.Or(in.Side, gh.SideRight), line: in.Line}
	from := anchor{side: cmp.Or(in.StartSide, to.side), line: cmp.Or(in.StartLine, in.Line)}
	return spot{path: in.Path, from: from, to: to}
}

func (s spot) name() string {
	switch {
	case s.file:
		return "this file"
	case s.from.line == s.to.line:
		return "line " + strconv.Itoa(s.to.line)
	}
	return "lines " + strconv.Itoa(s.from.line) + "–" + strconv.Itoa(s.to.line)
}

func lineAnchor(l gh.DiffLine) anchor {
	if l.Kind == gh.DiffRemoved {
		return anchor{side: gh.SideLeft, line: l.Old}
	}
	return anchor{side: gh.SideRight, line: l.New}
}

func (m Model) canOpenThread() bool {
	f := m.shownFile()
	return m.tab == tabFiles && m.detail.Loaded && m.files.Loaded && f != nil && f.Omitted == ""
}

func (m Model) spotted() spot {
	f := m.shownFile()
	whole := spot{path: f.Path, file: true}
	if !m.diffDriving() {
		return whole
	}

	on := m.pageRing.on
	if rows := m.diff.anchors[on]; m.walkedInto(on) && len(rows) > 0 {
		at := min(m.diffCursor, len(rows))
		from, to := at, at
		if m.selecting() {
			held := min(m.selection.anchor, len(rows))
			from, to = min(at, held), max(at, held)
		}
		return spot{path: f.Path, from: rows[from-1], to: rows[to-1]}
	}

	if on.kind == focusHunk {
		for _, h := range f.Hunks {
			if hunkKey(f.Path, h) == on && len(h.Lines) > 0 {
				return spot{path: f.Path, from: lineAnchor(h.Lines[0]), to: lineAnchor(h.Lines[len(h.Lines)-1])}
			}
		}
	}
	return whole
}

func (m Model) openingIn(path string) (spot, bool) {
	if m.inline.at.kind != focusNewThread || m.opening.path != path {
		return spot{}, false
	}
	return m.opening, true
}

func (m Model) openThread() (Model, tea.Cmd) {
	at := m.spotted()
	if on := m.pageRing.on; on.kind == focusHunk && !at.file {
		m.open[on] = true
	}

	m.compose.stop()
	m.clearMention()

	m.opening = at
	cmd := m.inline.open(at.key(), m.pageRing.on, "", m.threadWords())
	m.pageRing.on = at.key()
	m.focus = paneMain
	m.page.ok = false

	m.showOpenedBox()
	return m, cmd
}

func (m *Model) openingCard(width int) rendered {
	inner := width - threadIndent
	key := m.inline.at

	head := m.said(m.who, "comment on "+m.opening.name(), m.theme.Subtle, gh.TimelineItem{})
	block := m.card(head, m.inlineBox(m.cardWidth(inner), replyRows, boxChrome), inner, m.lit(key), "")

	return rendered{
		block:  indent(block, threadIndent),
		stops:  []focusItem{{focusKey: key, lines: strings.Count(block, "\n") + 1}},
		boxAt:  m.cardLead(inner, replyRows+1),
		boxCol: cardIndent + threadIndent,
	}
}

func (m *Model) diffBoxBlock(width int) rendered {
	if m.page.box.kind == focusNewThread {
		return m.openingCard(width)
	}
	for i, t := range m.detail.Detail.Threads {
		if m.boxIn(t) {
			return m.diffThread(i, width)
		}
	}
	return rendered{}
}

// Cut by line rather than by block: a diff is rows, and the box's card is the one block among them.
func (m *Model) holdDiffPage(body string) {
	from, to := m.diff.boxFrom, m.diff.boxFrom+m.diff.boxLines
	lines := strings.Split(body, "\n")
	if m.writing() == nil || m.diff.boxLines == 0 || to > len(lines) {
		return
	}

	var head, tail []focusItem
	for _, it := range m.diff.stops {
		switch {
		case it.start < from:
			head = append(head, it)
		case it.start >= to:
			tail = append(tail, it)
		}
	}

	m.page = pageCache{
		head:      strings.Join(lines[:from], "\n"),
		tail:      strings.Join(lines[to:], "\n"),
		items:     head,
		tailItems: shifted(tail, to),
		at:        from,
		gap:       "\n",
		box:       m.inline.at,
		ok:        true,
	}
}

func (m Model) postThread() (Model, tea.Cmd) {
	body := m.inline.body()
	if body == "" {
		return m, nil
	}

	in := m.opening.input()
	in.Body = body
	if review := m.detail.Detail.DraftReview.ID; review != "" {
		in.ReviewID = review
	} else {
		in.PullRequestID = m.pr.ID
	}

	id, from := m.pr.ID, m.inline.from
	m.inline.area.Reset()
	m.inline.close()
	m.selection = span{}

	m.pageRing.on = from
	m.page.ok = false
	m.syncContent()
	return m, func() tea.Msg { return PostThreadMsg{ID: id, Thread: in} }
}

// RestoreThread files a failed thread as its spot's draft and reopens the box only if its file is on
// screen and nothing else has the keyboard.
func (m *Model) RestoreThread(in gh.ReviewThreadInput, body string) tea.Cmd {
	at := spotOf(in)
	if m.writing() == nil {
		m.opening = at
	}
	return m.restore(at.key(), body, m.threadWords(),
		func() bool { return m.canOpenThread() && m.shownPath == at.path },
		func() focusKey { return m.pageRing.on })
}
