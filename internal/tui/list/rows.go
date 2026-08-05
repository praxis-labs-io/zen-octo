package list

import (
	"slices"
	"strings"

	"github.com/zen-octo/zen-octo/internal/gh"
)

// Rendered heights. A header is one line and a pull request is two, which is
// why neither the cursor nor the scroll offset can be a row index.
const (
	headerLines = 1
	rowLines    = 2

	// groupGap is the blank space above a group. Rows inside one already have a
	// line between them, so a group needs more than that to read as a break.
	// The first group takes less: there is nothing above it to break from.
	groupGap = 2
	topGap   = 1
)

// group is which block a pull request sorts into. The order of the constants is
// the order they render in.
type group int

const (
	groupReady group = iota
	groupDraft
	groupMerged
	groupClosed
)

var groupLabels = [...]string{"Ready", "Draft", "Merged", "Closed"}

// groupOf reads state before draft, because a draft that was closed is closed.
func groupOf(pr gh.PullRequest) group {
	switch pr.State {
	case gh.PRStateMerged:
		return groupMerged
	case gh.PRStateClosed:
		return groupClosed
	}
	if pr.IsDraft {
		return groupDraft
	}
	return groupReady
}

// item is one entry in the rendered order: a group header or a pull request.
type item struct {
	header     string // group label, empty on a pull request
	count      int    // pull requests in the group, on a header
	gapAbove   int    // blank lines above, on a header
	blankBelow bool   // one line below, on every row but a group's last
	pr         gh.PullRequest
}

// lines is how tall the item renders, blank line and all.
func (i item) lines() int {
	n := headerLines
	if i.isPR() {
		n = rowLines
	}
	n += i.gapAbove
	if i.blankBelow {
		n++
	}
	return n
}

func (i item) isPR() bool { return i.header == "" }

// rows is the rendered order together with the map from it to viewport lines.
// Headers are drawn but never selected, and rows are taller than headers, so
// every scroll and clamp path goes through here rather than through an index.
type rows struct {
	items      []item
	selectable []int // indices into items that are pull requests
	lineOf     []int // first viewport line of each item
	total      int
}

// newRows groups the pull requests, orders each group, and measures the result.
func newRows(prs []gh.PullRequest) rows {
	items := arrange(prs)

	r := rows{items: items, lineOf: make([]int, len(items))}
	for i, it := range items {
		r.lineOf[i] = r.total
		if it.isPR() {
			r.selectable = append(r.selectable, i)
		}
		r.total += it.lines()
	}
	return r
}

// arrange sorts the pull requests into groups and returns the rendered order: a
// header for every group that has anything in it, then its rows.
func arrange(prs []gh.PullRequest) []item {
	buckets := make([][]gh.PullRequest, len(groupLabels))
	for _, pr := range prs {
		g := groupOf(pr)
		buckets[g] = append(buckets[g], pr)
	}

	items := make([]item, 0, len(prs)+len(buckets))
	for g, bucket := range buckets {
		if len(bucket) == 0 {
			continue
		}
		slices.SortStableFunc(bucket, byRepoThenRecency)
		// A group after the first opens with its gap, and every row but the last
		// closes with a single line. The last skips its own so the gap before
		// the next header is the group gap and not a line more.
		gap := groupGap
		if len(items) == 0 {
			gap = topGap
		}
		items = append(items, item{header: groupLabels[g], count: len(bucket), gapAbove: gap})
		for i, pr := range bucket {
			items = append(items, item{pr: pr, blankBelow: i < len(bucket)-1})
		}
	}
	return items
}

// byRepoThenRecency keeps a repository's pull requests together, newest first
// within it.
func byRepoThenRecency(a, b gh.PullRequest) int {
	if c := strings.Compare(a.Repository, b.Repository); c != 0 {
		return c
	}
	return b.UpdatedAt.Compare(a.UpdatedAt)
}

func (r rows) len() int { return len(r.selectable) }

// pr is the pull request at a selectable index.
func (r rows) pr(n int) (gh.PullRequest, bool) {
	if n < 0 || n >= len(r.selectable) {
		return gh.PullRequest{}, false
	}
	return r.items[r.selectable[n]].pr, true
}

// item is the index into the rendered order of the nth selectable row, so the
// renderer can tell the selected row from the rest in one pass.
func (r rows) item(n int) int {
	if n < 0 || n >= len(r.selectable) {
		return -1
	}
	return r.selectable[n]
}

// align rounds a viewport offset up to the start of an item, so the top of the
// window is never a row's second line with its title scrolled off above it.
func (r rows) align(line int) int {
	for _, at := range r.lineOf {
		if at >= line {
			return at
		}
	}
	return line
}

// span is the first and last viewport line of the nth selectable row.
func (r rows) span(n int) (first, last int) {
	i := r.item(n)
	if i < 0 {
		return 0, 0
	}
	return r.lineOf[i], r.lineOf[i] + rowLines - 1
}
