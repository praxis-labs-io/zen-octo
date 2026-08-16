package store

import (
	"slices"

	"github.com/zen-octo/zen-octo/internal/gh"
)

// ReactionWrite is one reaction toggled here and not yet answered for.
//
// A fourth kind of write beside Pending, Edit and CommentWrite, because it does
// a fourth thing. It is not a CommentWrite carrying a reaction: that type's
// delete branch takes a thread away with its last comment and its edit branch
// marks the comment Editing, and neither is anything a reaction does.
//
// Held beside the fetched detail for the reason Pending is: a refetch replaces
// a timeline wholesale, and one fetched before the mutation answered is not
// evidence the mutation failed.
//
// CommentID empty is the description, which is a field of the pull request and
// has no comment to name. ThreadID is the review thread a comment sits in,
// empty on a top-level comment or a review's own body.
type ReactionWrite struct {
	Key       string
	CommentID string
	ThreadID  string

	Content gh.ReactionContent

	// On is the direction the key asked for, not the state it lands on. Two out
	// on one reaction then compose in the order they were pressed.
	On bool
}

// PendingReaction holds a reaction toggled here and returns the key the
// response reconciles against. The pill moves from now on, which is the
// acknowledgement.
func (s *Store) PendingReaction(id, commentID, threadID string,
	content gh.ReactionContent, on bool,
) string {
	w := ReactionWrite{
		Key:       s.nextKey(),
		CommentID: commentID,
		ThreadID:  threadID,
		Content:   content,
		On:        on,
	}
	if s.reacting == nil {
		s.reacting = make(map[string][]ReactionWrite)
	}
	s.reacting[id] = append(s.reacting[id], w)
	return w.Key
}

// putReaction writes one group over a subject's set and rebuilds it in GitHub's
// own order.
//
// The order rather than an append, so a reaction given here lands where the
// answer is going to put it: appending would move the pill sideways the moment
// the mutation settled.
//
// A group at zero comes off, unless it is still being written. That one is the
// only thing a second press has to read, and the key goes inert on a marked
// one: two toggles on one reaction settle in the order the responses arrive,
// which is not the order they were pressed.
func putReaction(held []gh.Reaction, r gh.Reaction) []gh.Reaction {
	by := make(map[gh.ReactionContent]gh.Reaction, len(held)+1)
	for _, h := range held {
		by[h.Content] = h
	}
	by[r.Content] = r

	var out []gh.Reaction
	for _, c := range gh.ReactionOrder {
		if got, ok := by[c]; ok && (got.Count > 0 || got.Pending) {
			out = append(out, got)
		}
	}
	return out
}

// reactionIn is a subject's group for one content, or a zero one where nobody
// has given it.
func reactionIn(held []gh.Reaction, content gh.ReactionContent) gh.Reaction {
	for _, r := range held {
		if r.Content == content {
			return r
		}
	}
	return gh.Reaction{Content: content}
}

// applyReaction is one toggle over a subject's reactions.
func applyReaction(held []gh.Reaction, content gh.ReactionContent, on bool) []gh.Reaction {
	r := reactionIn(held, content)
	switch {
	case on && !r.Viewer:
		r.Count++
		r.Viewer = true
	case !on && r.Viewer:
		r.Count--
		r.Viewer = false
	}
	r.Pending = true
	return putReaction(held, r)
}

// foldReactions applies every reaction in flight over a fetched description,
// timeline and threads, and reports which of the two slices it cloned.
//
// Cloning is lazy and shared with the folds around it, for the reason foldWrites
// gives. The description needs no flag: applyReaction returns a fresh slice
// every time, so there is nothing held for it to write into.
//
// A comment the fold cannot find is skipped. A refetch that landed while the
// write was out may no longer carry it, and there is nothing honest to invent.
func foldReactions(writes []ReactionWrite, body []gh.Reaction,
	timeline []gh.TimelineItem, threads []gh.ReviewThread,
	freshTimeline, freshThreads bool,
) ([]gh.Reaction, []gh.TimelineItem, []gh.ReviewThread, bool, bool) {
	for _, w := range writes {
		switch {
		case w.CommentID == "":
			body = applyReaction(body, w.Content, w.On)

		case w.ThreadID != "":
			threads, freshThreads = reactInThread(w, threads, freshThreads)

		default:
			at := commentAt(timeline, w.CommentID)
			if at < 0 {
				continue
			}
			if !freshTimeline {
				timeline, freshTimeline = slices.Clone(timeline), true
			}

			// The item holds a pointer to the comment, and the comment is the
			// held one until this copies it. Writing through the pointer would
			// move a pill inside a detail this call was supposed to leave alone.
			said := *timeline[at].Comment
			said.Reactions = applyReaction(said.Reactions, w.Content, w.On)
			timeline[at].Comment = &said
		}
	}
	return body, timeline, threads, freshTimeline, freshThreads
}

// reactInThread is the fold above, one level down. Nothing here can take a
// thread away, so it is shorter than the delete's equivalent.
func reactInThread(w ReactionWrite, threads []gh.ReviewThread, fresh bool) ([]gh.ReviewThread, bool) {
	at := threadAt(threads, w.ThreadID)
	if at < 0 {
		return threads, fresh
	}

	in := slices.IndexFunc(threads[at].Comments, func(c gh.Comment) bool { return c.ID == w.CommentID })
	if in < 0 {
		return threads, fresh
	}

	if !fresh {
		threads, fresh = slices.Clone(threads), true
	}

	// The outer clone is not enough: a thread's comments are their own slice,
	// still the held one, and writing into it reaches the detail this call was
	// supposed to leave alone.
	comments := slices.Clone(threads[at].Comments)
	comments[in].Reactions = applyReaction(comments[in].Reactions, w.Content, w.On)
	threads[at].Comments = comments
	return threads, fresh
}

// ReactionApplied writes GitHub's answer into the held detail and drops the
// write it settles.
//
// The one group the write moved, never the whole set the payload carries. Two
// toggles on one subject answer in whatever order the network gives them, and
// each response is a snapshot of the subject as it stood when GitHub handled
// that call: taking either one whole lets the older snapshot land last and
// delete a reaction the other one added. Writing the group this write is
// answering for is order-independent, because no two writes on a subject share
// a content while the first is still out.
func (s *Store) ReactionApplied(id, key string, res gh.ReactionResult) {
	w, held, ok := s.settleReaction(id, key)
	if !ok {
		return
	}

	// The answer's own group, or a zero one where the removal took the last of
	// them: putReaction drops a group at zero that nothing is still writing.
	settled := reactionIn(res.Reactions, w.Content)
	settled.Pending = false

	switch {
	case w.CommentID == "":
		held.Detail.Reactions = putReaction(held.Detail.Reactions, settled)

	case w.ThreadID != "":
		at := threadAt(held.Detail.Threads, w.ThreadID)
		if at < 0 {
			return
		}
		in := slices.IndexFunc(held.Detail.Threads[at].Comments, func(c gh.Comment) bool {
			return c.ID == w.CommentID
		})
		if in < 0 {
			return
		}
		threads := slices.Clone(held.Detail.Threads)
		comments := slices.Clone(threads[at].Comments)
		comments[in].Reactions = putReaction(comments[in].Reactions, settled)
		threads[at].Comments = comments
		held.Detail.Threads = threads

	default:
		at := commentAt(held.Detail.Timeline, w.CommentID)
		if at < 0 {
			return
		}
		timeline := slices.Clone(held.Detail.Timeline)
		said := *timeline[at].Comment
		said.Reactions = putReaction(said.Reactions, settled)
		timeline[at].Comment = &said
		held.Detail.Timeline = timeline
	}

	s.put(id, held)
	s.markStale(id)
}

// ReactionReverted puts the pill back the way it was fetched. The caller owns
// saying why: the store cannot tell a rejected write from a lost one.
func (s *Store) ReactionReverted(id, key string) { s.dropReaction(id, key) }

// settleReaction drops the write a response answers for and hands it back with
// the held detail, or false when there is nothing to write into.
func (s *Store) settleReaction(id, key string) (ReactionWrite, Detail, bool) {
	w, ok := s.dropReaction(id, key)
	if !ok {
		return ReactionWrite{}, Detail{}, false
	}

	held, ok := s.details.look(id)
	if !ok {
		return ReactionWrite{}, Detail{}, false
	}
	return w, held, true
}

// dropReaction removes one write and gives it back, with whether it was there.
// A response for a key already gone is one that already settled.
func (s *Store) dropReaction(id, key string) (ReactionWrite, bool) {
	held := s.reacting[id]
	at := slices.IndexFunc(held, func(w ReactionWrite) bool { return w.Key == key })
	if at < 0 {
		return ReactionWrite{}, false
	}

	w := held[at]
	s.reacting[id] = slices.Delete(held, at, at+1)
	if len(s.reacting[id]) == 0 {
		delete(s.reacting, id)
	}
	return w, true
}
