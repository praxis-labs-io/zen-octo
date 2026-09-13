package prview

import "github.com/praxis-labs-io/zen-octo/internal/gh"

type focusKind int

const (
	focusNone focusKind = iota
	focusDescription
	focusComment
	focusReview

	focusThread
	focusThreadComment

	focusHunk

	focusState
	focusReviewer
	focusAssignee
	focusLabel
	focusCheck
	focusBase
	focusMerge

	focusAddReviewer
	focusAddAssignee
	focusAddLabel

	focusCompose

	focusReply
)

func (k focusKind) prose() bool {
	switch k {
	case focusDescription, focusComment, focusReview, focusThread, focusThreadComment:
		return true
	}
	return false
}

// Identity, never slice position: a refetch re-sorts the timeline and a position names another card.
type focusKey struct {
	kind focusKind
	id   string
}

func threadKey(t gh.ReviewThread) focusKey {
	return focusKey{kind: focusThread, id: t.ID}
}

func hunkKey(path string, h gh.Hunk) focusKey {
	return focusKey{kind: focusHunk, id: path + "@" + h.Header}
}

func threadCommentKey(c gh.Comment) focusKey {
	return focusKey{kind: focusThreadComment, id: c.ID}
}

// start and lines count within the pane's body, before the blank line every pane opens with.
type focusItem struct {
	focusKey
	start int
	lines int
}

func (it focusItem) covers(top, height int) bool {
	return it.start < top+height && it.start+it.lines > top
}

// Deliberately not covers: the block a key may act on is not always the block the reader is at.
func (it focusItem) headOn(top, height int) bool {
	return it.start >= top && it.start < top+height
}

func (it focusItem) whole(top, height int) bool {
	return it.start >= top && it.start+it.lines <= top+height
}

type ring struct {
	items []focusItem
	on    focusKey
}

// Drops the slice rather than reusing it: View runs on a copy, and two tabs share this ring.
func (r *ring) reset() { r.items = nil }

func (r ring) stops() int { return len(r.items) }

func (r *ring) add(key focusKey, start, lines int) {
	r.items = append(r.items, focusItem{focusKey: key, start: start, lines: lines})
}

func (r ring) focused(key focusKey) bool {
	return key.kind != focusNone && key == r.on
}

func (r ring) index() int {
	if r.on.kind == focusNone {
		return -1
	}
	for i, it := range r.items {
		if it.focusKey == r.on {
			return i
		}
	}
	return -1
}

func (r ring) live(top, height int) bool {
	at := r.index()
	return at >= 0 && r.items[at].covers(top, height)
}

func (r *ring) clear() bool {
	had := r.on.kind != focusNone
	r.on = focusKey{}
	return had
}

// Both ends stop rather than wrap: on a page-deep pull request a wrap is the longest throw there is.
func (r *ring) step(delta, top, height int) bool {
	if len(r.items) == 0 {
		r.on = focusKey{}
		return false
	}

	if at := r.index(); at >= 0 && r.items[at].headOn(top, height) {
		next := at + delta
		if next < 0 || next >= len(r.items) {
			return false
		}
		r.on = r.items[next].focusKey
		return true
	}

	at, ok := r.seek(delta, top, height)
	if !ok {
		return false
	}
	r.on = r.items[at].focusKey
	return true
}

func (r *ring) advance(delta int) bool {
	at := r.index()
	if at < 0 {
		return false
	}

	next := at + delta
	if next < 0 || next >= len(r.items) {
		return false
	}
	r.on = r.items[next].focusKey
	return true
}

// Back stops at the last card whole on screen, not the block under the top row a screen away.
func (r ring) seek(delta, top, height int) (int, bool) {
	if delta < 0 {
		for i := len(r.items) - 1; i >= 0; i-- {
			if r.items[i].whole(top, height) {
				return i, true
			}
		}
		for i := len(r.items) - 1; i >= 0; i-- {
			if r.items[i].start < top {
				return i, true
			}
		}
		return 0, false
	}
	for i, it := range r.items {
		if it.start >= top {
			return i, true
		}
	}
	return 0, false
}

// Tops the item: the shortest scroll leaves a card's replies below the fold.
func (r ring) show(top, height int) int {
	at := r.index()
	if at < 0 || height <= 0 {
		return top
	}

	it := r.items[at]
	if it.whole(top, height) {
		return top
	}
	return it.start
}
