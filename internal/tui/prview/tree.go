package prview

import (
	"path"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/paint"
	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

const treeIndent = 2

type node struct {
	name     string
	key      string
	children []*node
	file     *gh.ChangedFile
}

type row struct {
	key    string
	label  string
	depth  int
	dir    bool
	file   *gh.ChangedFile
	folded bool
}

func buildTree(files []gh.ChangedFile) *node {
	root := &node{}

	sorted := make([]gh.ChangedFile, len(files))
	copy(sorted, files)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })

	for i := range sorted {
		f := &sorted[i]
		at := root
		var walked string

		segments := strings.Split(f.Path, "/")
		for _, dir := range segments[:len(segments)-1] {
			walked = path.Join(walked, dir)
			at = at.child(dir, walked)
		}
		at.children = append(at.children, &node{name: segments[len(segments)-1], key: f.Path, file: f})
	}
	return root
}

func (n *node) child(name, key string) *node {
	for _, c := range n.children {
		if c.file == nil && c.name == name {
			return c
		}
	}
	made := &node{name: name, key: key}
	at := len(n.children)
	for i, c := range n.children {
		if c.file != nil {
			at = i
			break
		}
	}
	n.children = append(n.children, nil)
	copy(n.children[at+1:], n.children[at:])
	n.children[at] = made
	return made
}

func flatten(n *node, collapsed map[string]bool, depth int, out []row) []row {
	for _, c := range n.children {
		if c.file != nil {
			out = append(out, row{key: c.key, label: c.name, depth: depth, file: c.file,
				folded: collapsed[c.key]})
			continue
		}

		label, end := c.name, c
		for len(end.children) == 1 && end.children[0].file == nil {
			end = end.children[0]
			label += "/" + end.name
		}

		folded := collapsed[end.key]
		out = append(out, row{key: end.key, label: label + "/", depth: depth, dir: true, folded: folded})
		if !folded {
			out = flatten(end, collapsed, depth+1, out)
		}
	}
	return out
}

func renderRow(th theme.Theme, r row, width int, selected bool) string {
	base := lipgloss.NewStyle()
	if selected {
		base = base.Background(th.SelectedBackground)
	}

	lead := base.Render(strings.Repeat(" ", r.depth*treeIndent))

	marker := "  "
	markerColor := th.Subtle
	switch {
	case r.dir && r.folded:
		marker = "▸ "
	case r.dir:
		marker = "▾ "
	case r.file != nil && r.file.Viewed == gh.FileUnviewed:
		marker = "○ "
	case r.file != nil && r.file.Viewed == gh.FileDismissed:
		marker = "⊙ "
		markerColor = th.Warning
	case r.file != nil && r.file.Viewed == gh.FileViewed:
		marker = "● "
		markerColor = th.Accent
	}
	lead += base.Foreground(markerColor).Render(marker)

	name := base.Foreground(th.Text)
	if r.dir {
		name = base.Foreground(th.Accent)
	}

	room := max(0, width-lipgloss.Width(lead))
	label := name.Render(r.label)
	if lipgloss.Width(label) > room {
		label = paint.Clip(label, room, base.Foreground(th.Subtle))
	}

	line := lead + label
	if lipgloss.Width(line) > width {
		return paint.Clip(line, width, base.Foreground(th.Subtle))
	}
	return line + base.Render(strings.Repeat(" ", width-lipgloss.Width(line)))
}
