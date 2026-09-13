package list

import (
	"slices"
	"strings"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

const (
	headerLines = 1
	rowLines    = 2

	groupGap = 2
	topGap   = 1
)

type group int

const (
	groupReady group = iota
	groupDraft
	groupMerged
	groupClosed
)

var groupLabels = [...]string{"Ready", "Draft", "Merged", "Closed"}

// State before draft: GitHub keeps the draft flag on a closed pull request.
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

type item struct {
	header     string
	count      int
	gapAbove   int
	blankBelow bool
	pr         gh.PullRequest
}

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

type rows struct {
	items      []item
	selectable []int
	lineOf     []int
	total      int
}

func (r rows) same(other rows) bool {
	return slices.Equal(r.items, other.items)
}

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

func byRepoThenRecency(a, b gh.PullRequest) int {
	if c := strings.Compare(a.Repository, b.Repository); c != 0 {
		return c
	}
	return b.UpdatedAt.Compare(a.UpdatedAt)
}

func (r rows) len() int { return len(r.selectable) }

func (r rows) pr(n int) (gh.PullRequest, bool) {
	if n < 0 || n >= len(r.selectable) {
		return gh.PullRequest{}, false
	}
	return r.items[r.selectable[n]].pr, true
}

func (r rows) item(n int) int {
	if n < 0 || n >= len(r.selectable) {
		return -1
	}
	return r.selectable[n]
}

func (r rows) align(line int) int {
	for _, at := range r.lineOf {
		if at >= line {
			return at
		}
	}
	return line
}

func (r rows) top(n, height int) int {
	row := r.item(n)
	if row < 0 {
		return 0
	}
	last := r.lineOf[row] + rowLines - 1

	head := row
	for head > 0 && !r.items[head-1].isPR() {
		head--
	}

	for _, at := range [...]int{r.lineOf[head], r.lineOf[head] + r.items[head].gapAbove, r.lineOf[row]} {
		if last-at+1 <= height {
			return at
		}
	}
	return r.lineOf[row]
}

// A viewport clamps its offset by lines, not items, so without padding a full scroll opens mid-row.
func (r rows) pad(height int) int {
	if height <= 0 || r.total <= height {
		return 0
	}
	return r.align(r.total-height) - (r.total - height)
}

func (r rows) span(n int) (first, last int) {
	i := r.item(n)
	if i < 0 {
		return 0, 0
	}
	return r.lineOf[i], r.lineOf[i] + rowLines - 1
}
