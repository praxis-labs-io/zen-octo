package prview

// focusKind is what a focusable thing is. An action key reads it to know what
// it has been handed: a reply belongs on a thread and nowhere else.
type focusKind int

const (
	// focusNone is the zero value, so an untouched ring is an unfocused one.
	// Every screen opens that way: the reader came to read, not to act.
	focusNone focusKind = iota
	focusDescription
	focusComment
	focusReview
	focusThread
	focusReviewer
	focusAssignee
	focusLabel
	focusCheck
)

// prose is whether this kind renders a markdown body, which is the only thing
// a fold key has to work on.
func (k focusKind) prose() bool {
	switch k {
	case focusDescription, focusComment, focusReview, focusThread:
		return true
	}
	return false
}

// focusKey names one focusable thing: what it is and which one. Focus is held
// by key rather than by position, so a refetch that adds a comment above the
// focused one leaves the focus where the reader put it.
type focusKey struct {
	kind  focusKind
	index int
}

// focusItem is a key with where it landed. start and lines are in the body the
// pane was handed, before the blank line every pane opens with.
type focusItem struct {
	focusKey
	start int
	lines int
}

// covers reports whether the item shows any part of itself in a window.
func (it focusItem) covers(top, height int) bool {
	return it.start < top+height && it.start+it.lines > top
}

// ring is the focus order of one pane. The items are rebuilt on every render;
// on survives it, because it names a thing rather than a slot.
type ring struct {
	items []focusItem

	// lead is what the tab put above the first item. The conversation opens
	// with its header block, and without this every item is that many lines out.
	lead int

	on focusKey
}

// reset empties the items for a fresh render, keeping the focus and the lead.
func (r *ring) reset() { r.items = r.items[:0] }

// add records one focusable block at the line it was written to.
func (r *ring) add(key focusKey, start, lines int) {
	r.items = append(r.items, focusItem{focusKey: key, start: start, lines: lines})
}

// focused reports whether a key is the one holding focus. A zero key is never
// focused, so a caller with nothing to name does not light the whole pane up.
func (r ring) focused(key focusKey) bool {
	return key.kind != focusNone && key == r.on
}

// index is where the focus sits in the current items, or minus one when the
// thing it names is no longer on the screen.
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

// clear drops the focus, and reports whether there was one to drop. The screen
// reads that answer to decide whether esc backs out or only lets go.
func (r *ring) clear() bool {
	had := r.on.kind != focusNone
	r.on = focusKey{}
	return had
}

// step moves the focus one item and reports whether the ring took the key. top
// and height are the window the pane is showing, in the same lines the items
// were recorded in.
//
// Focus does not survive being scrolled out of the window. A reader who scrolled
// away has moved on, and the one thing the ring must not do is haul them back to
// a card they left behind. So it re-anchors to what is on the screen now.
func (r *ring) step(delta, top, height int) bool {
	if len(r.items) == 0 {
		r.on = focusKey{}
		return false
	}

	at := r.index()
	if at < 0 || !r.items[at].covers(top, height) {
		r.on = r.items[r.anchor(delta, top, height)].focusKey
		return true
	}

	r.on = r.items[(at+delta+len(r.items))%len(r.items)].focusKey
	return true
}

// anchor is where focus lands when there is none to move: the first item that
// begins inside the window going forward, the last that ends inside it going
// back. A window between two items falls to whichever end it is nearer.
func (r ring) anchor(delta, top, height int) int {
	if delta < 0 {
		for i := len(r.items) - 1; i >= 0; i-- {
			if r.items[i].start+r.items[i].lines <= top+height {
				return i
			}
		}
		return 0
	}
	for i, it := range r.items {
		if it.start >= top {
			return i
		}
	}
	return len(r.items) - 1
}

// show is the offset that brings the focused item onto a window of the given
// height, moving no further than it has to. An item taller than the window pins
// to its top: the alternative opens on its last line with its heading above.
func (r ring) show(top, height int) int {
	at := r.index()
	if at < 0 || height <= 0 {
		return top
	}

	it := r.items[at]
	switch {
	case it.start < top:
		return it.start
	case it.start+it.lines > top+height:
		return max(it.start, it.start+it.lines-height)
	}
	return top
}
