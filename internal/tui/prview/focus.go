package prview

import "github.com/zen-octo/zen-octo/internal/gh"

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

	// focusThread is the thread's own card: the anchor, the code, and the comment
	// that opened it. focusThreadComment is one of the replies hanging off it,
	// each of which is a card of its own.
	//
	// Both are ring stops. A card the motion key walks past is a card the reader
	// can see and cannot reach, and crossing a heavily reviewed page is what the
	// scroll keys are for.
	focusThread
	focusThreadComment

	focusState
	focusReviewer
	focusAssignee
	focusLabel
	focusCheck
	focusBase
	focusMerge

	// The add rows sit under what they add to rather than among it. Each opens
	// a picker, so an action key reads the kind and knows it has a control
	// rather than a reviewer, an assignee or a label.
	focusAddReviewer
	focusAddAssignee
	focusAddLabel

	// The comment box is the last card in the conversation, so tab reaches it
	// the same way it reaches everything above it. Focus on it is not the same
	// as typing in it: enter starts that, and esc stops it.
	focusCompose

	// focusReply is the box r opens inside a thread. Unlike the compose card it
	// exists only while it is open, and tab never walks onto it: the box has the
	// keyboard from the moment it appears, and tab means the post button then.
	// It is in the ring so the page can be scrolled to keep it in sight.
	focusReply
)

// prose is whether this kind renders a markdown body, which is the only thing
// a fold key has to work on.
func (k focusKind) prose() bool {
	switch k {
	case focusDescription, focusComment, focusReview, focusThread, focusThreadComment:
		return true
	}
	return false
}

// focusKey names one focusable thing: what it is and which one.
//
// Which one is the thing's own identity, never its place in a slice. A refetch
// re-sorts the timeline whenever a rebase rewrites a commit's date, and a place
// would then name whichever card took it: focus and everything the reader
// unfolded would move to a card they never pointed at.
//
// id is the node id on a comment, a review or a thread; a login on a reviewer
// or an assignee; a name on a label; and Key on a check, which the rollup keeps
// one of per workflow and name. It is empty on the rows there is only ever one
// of, which is the description and every rail control.
//
// Two focusable things sharing an id would wedge tab, because the ring steps
// from the first match and lands back on the same key. Nothing above produces
// one: GitHub's node ids are unique, reviewers are deduplicated by login, and
// no pull request carries a label or an assignee twice.
type focusKey struct {
	kind focusKind
	id   string
}

// threadKey is what a review thread answers to. The conversation and the Files
// tab both render the same threads, and an unfold on one is an unfold on the
// other, so they have to build this the same way.
func threadKey(t gh.ReviewThread) focusKey {
	return focusKey{kind: focusThread, id: t.ID}
}

// threadCommentKey is what one comment inside a thread answers to. Built from
// the comment's own node id and nothing else, for the same reason as above: both
// tabs render the same comments and a fold has to carry between them.
func threadCommentKey(c gh.Comment) focusKey {
	return focusKey{kind: focusThreadComment, id: c.ID}
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
// on survives it, because it names what it points at rather than where that
// landed on the screen.
type ring struct {
	items []focusItem
	on    focusKey
}

// reset empties the items for a fresh render, keeping the focus.
func (r *ring) reset() { r.items = r.items[:0] }

// stops is how many things there are to land on, as of the last render.
func (r ring) stops() int { return len(r.items) }

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

// live is whether the focus is on the screen. Scrolled out of the window it is
// nothing the reader can see, so it is nothing for a key to act on either. This
// is the rule step re-anchors by, and every key that reads the focus holds to
// it: one that acted on a card off screen would move the page under a reader
// who had already left it.
func (r ring) live(top, height int) bool {
	at := r.index()
	return at >= 0 && r.items[at].covers(top, height)
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
//
// wrap is the caller's, because the two rings end differently. A page of cards
// comes back round: the ring is the whole of it and there is nothing past the
// last one. The rail is a list of controls inside a pane that is taller than
// them, so its end is a boundary rather than a seam, and reporting the key
// untaken is what lets the pane scroll to the facts underneath.
func (r *ring) step(delta, top, height int, wrap bool) bool {
	if len(r.items) == 0 {
		r.on = focusKey{}
		return false
	}

	at := r.index()
	if at < 0 || !r.items[at].covers(top, height) {
		r.on = r.items[r.anchor(delta, top, height)].focusKey
		return true
	}

	next := at + delta
	if !wrap && (next < 0 || next >= len(r.items)) {
		return false
	}

	r.on = r.items[(next+len(r.items))%len(r.items)].focusKey
	return true
}

// anchor is where focus lands when there is none to move: the first item on the
// screen going forward, the last going back. A window between two items falls to
// whichever end it is nearer.
//
// On the screen means any part of it, not all of it. A card taller than the
// window is the one the reader is looking at, and a scan for the first item to
// begin below the top skips straight past it to the next one.
func (r ring) anchor(delta, top, height int) int {
	if delta < 0 {
		for i := len(r.items) - 1; i >= 0; i-- {
			if r.items[i].covers(top, height) {
				return i
			}
		}
		return 0
	}
	for i, it := range r.items {
		if it.covers(top, height) {
			return i
		}
	}
	return len(r.items) - 1
}

// show is the offset that brings the focused item onto a window of the given
// height. An item already on screen whole leaves the page where it is, because
// the highlight is signal enough and scrolling under a reader who can already
// see the thing is worse than not scrolling.
//
// Anything else goes to the top row rather than the shortest distance onto the
// screen. The shortest distance lands a card at the foot of the window, and
// what a card is worth reading for is the replies under it.
func (r ring) show(top, height int) int {
	at := r.index()
	if at < 0 || height <= 0 {
		return top
	}

	it := r.items[at]
	if it.start >= top && it.start+it.lines <= top+height {
		return top
	}
	return it.start
}
