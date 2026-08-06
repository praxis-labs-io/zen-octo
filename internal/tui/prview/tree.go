package prview

import (
	"path"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// treeIndent is what one level of nesting costs. Two columns is enough to read
// as a level in a column this narrow, and a deep repository runs out of room
// fast at any more.
const treeIndent = 2

// node is one entry in the file tree. A directory has children and no file; a
// file has a file and none.
type node struct {
	name     string
	key      string
	children []*node
	file     *gh.ChangedFile
}

// row is one printed line of the tree, after folding has decided what is on
// screen. Key is what the collapse map and the diff pane are keyed by: a path
// for a file, a directory path for a directory.
type row struct {
	key    string
	label  string
	depth  int
	dir    bool
	file   *gh.ChangedFile
	folded bool
}

// buildTree nests the changed files by directory. Files arrive in whatever
// order GitHub returned them, and a tree in that order is not a tree.
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

// child finds or adds a directory under this node, keeping directories ahead of
// files so a folder's contents do not read as siblings of the folder next to it.
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

// flatten walks the tree into the lines that are on screen. A directory with
// one directory inside it prints as one row: internal/tui/prview/ is a path,
// and spending three lines and six columns to say so is what makes a narrow
// tree unreadable.
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

// renderRow is one line of the tree: the fold marker and the name. No churn:
// every file's own heading in the diff carries it, and repeating it here costs
// the column the cells a nested path needs.
//
// Selection is painted cell by cell. A joined row wrapped in the background
// style afterwards paints only its first cell, because every styled run ends in
// a reset that clears the background with it.
func renderRow(th theme.Theme, r row, width int, selected bool) string {
	base := lipgloss.NewStyle()
	if selected {
		base = base.Background(th.SelectedBackground)
	}

	lead := base.Render(strings.Repeat(" ", r.depth*treeIndent))

	marker := "  "
	switch {
	case r.dir && r.folded:
		marker = "▸ "
	case r.dir:
		marker = "▾ "
	}
	lead += base.Foreground(th.Faint).Render(marker)

	name := base.Foreground(th.Primary)
	if r.dir {
		name = base.Foreground(th.Secondary)
	}

	room := max(0, width-lipgloss.Width(lead))
	label := name.Render(r.label)
	if lipgloss.Width(label) > room {
		label = comp.Clip(label, room, base.Foreground(th.Faint))
	}

	line := lead + label
	// A column too narrow for the indent alone overflows everything above it,
	// and the pane would clip it mid-cell with nothing to say it had.
	if lipgloss.Width(line) > width {
		return comp.Clip(line, width, base.Foreground(th.Faint))
	}
	return line + base.Render(strings.Repeat(" ", width-lipgloss.Width(line)))
}
