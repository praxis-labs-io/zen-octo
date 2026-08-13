package store

import (
	"slices"

	"github.com/zen-octo/zen-octo/internal/gh"
)

// CommentWrite is a comment rewritten or removed here and not yet answered for.
//
// It is a third kind of write beside Pending and Edit because it does a third
// thing. Pending appends something new to the page, an Edit replaces a whole
// field of the pull request, and this one reaches into a timeline or a thread
// and changes a comment that is already there.
//
// Held beside the fetched detail for the reason Pending is: a refetch replaces
// a timeline wholesale, and one fetched before the mutation answered is not
// evidence the mutation failed.
//
// Key is minted here and belongs to this session. CommentID is GitHub's, since
// this write always names something GitHub already has.
type CommentWrite struct {
	Key       string
	CommentID string

	// ThreadID is the review thread the comment sits in, empty on a top-level
	// comment or a review's own body. It is what tells the fold which of the two
	// places to look, and a thread comment cannot be found without it: the
	// timeline carries no id for one.
	ThreadID string

	// Body is what the comment is being rewritten to, ignored on a delete.
	Body string

	Delete bool
}

// PendingCommentEdit holds a rewritten comment and returns the key the response
// reconciles against. The new body renders from now on.
func (s *Store) PendingCommentEdit(id, commentID, threadID, body string) string {
	return s.holdWrite(id, CommentWrite{
		CommentID: commentID,
		ThreadID:  threadID,
		Body:      body,
	})
}

// PendingCommentDelete holds a comment taken off the page and returns the key
// the response reconciles against. The card is gone from now on, which is the
// acknowledgement.
func (s *Store) PendingCommentDelete(id, commentID, threadID string) string {
	return s.holdWrite(id, CommentWrite{
		CommentID: commentID,
		ThreadID:  threadID,
		Delete:    true,
	})
}

func (s *Store) holdWrite(id string, w CommentWrite) string {
	w.Key = s.nextKey()
	if s.rewrites == nil {
		s.rewrites = make(map[string][]CommentWrite)
	}
	s.rewrites[id] = append(s.rewrites[id], w)
	return w.Key
}

// foldWrites applies every comment write in flight over a fetched timeline and
// its threads, and reports which of the two it cloned.
//
// Cloning is lazy and shared with the folds around it, because most calls need
// neither: a detail with only a comment out must not pay for cloning every
// thread's comments.
//
// A comment the fold cannot find is skipped. A refetch that landed while the
// write was out may no longer carry it, and there is nothing honest to invent.
func foldWrites(writes []CommentWrite, timeline []gh.TimelineItem, threads []gh.ReviewThread,
	freshTimeline, freshThreads bool,
) ([]gh.TimelineItem, []gh.ReviewThread, bool, bool) {
	for _, w := range writes {
		if w.ThreadID != "" {
			threads, freshThreads = foldIntoThread(w, threads, freshThreads)
			continue
		}

		at := commentAt(timeline, w.CommentID)
		if at < 0 {
			continue
		}

		if !freshTimeline {
			timeline, freshTimeline = slices.Clone(timeline), true
		}

		if w.Delete {
			timeline = slices.Delete(timeline, at, at+1)
			continue
		}

		// The item holds a pointer to the comment, and the comment is the held
		// one until this copies it. Writing through the pointer would rewrite the
		// body inside a detail this call was supposed to leave alone.
		said := *timeline[at].Comment
		said.Body = w.Body
		said.Editing = true
		timeline[at].Comment = &said
	}
	return timeline, threads, freshTimeline, freshThreads
}

// foldIntoThread is the fold above, one level down, where a delete can take the
// thread with it.
func foldIntoThread(w CommentWrite, threads []gh.ReviewThread, fresh bool) ([]gh.ReviewThread, bool) {
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

	// A thread is its comments. GitHub drops one whose last comment goes, and
	// leaving an empty card on the page would say a discussion is still there
	// with nothing in it.
	if w.Delete && len(threads[at].Comments) == 1 {
		return slices.Delete(threads, at, at+1), fresh
	}

	// The outer clone is not enough: a thread's comments are their own slice,
	// still the held one, and writing into it reaches the detail this call was
	// supposed to leave alone.
	comments := slices.Clone(threads[at].Comments)
	if w.Delete {
		comments = slices.Delete(comments, in, in+1)
	} else {
		comments[in].Body = w.Body
		comments[in].Editing = true
	}
	threads[at].Comments = comments
	return threads, fresh
}

// CommentEditApplied writes GitHub's answer into the held detail and drops the
// write it settles.
//
// The answer rather than what was sent, the way every settled edit takes
// GitHub's version: the mutation is the cheapest place to learn what the server
// made of the text, and the permissions come back with it.
func (s *Store) CommentEditApplied(id, key string, res gh.CommentResult) {
	w, held, ok := s.settleWrite(id, key)
	if !ok {
		return
	}

	if w.ThreadID != "" {
		s.threadCommentApplied(id, held, w, res.Comment)
		return
	}

	at := commentAt(held.Detail.Timeline, w.CommentID)
	if at < 0 {
		return
	}

	timeline := slices.Clone(held.Detail.Timeline)
	said := res.Comment
	timeline[at].Comment = &said
	held.Detail.Timeline = timeline
	s.put(id, held)
	s.markStale(id)
}

// threadCommentApplied is the settle above, inside the thread the comment sits
// in.
func (s *Store) threadCommentApplied(id string, held Detail, w CommentWrite, c gh.Comment) {
	at := threadAt(held.Detail.Threads, w.ThreadID)
	if at < 0 {
		return
	}
	in := slices.IndexFunc(held.Detail.Threads[at].Comments, func(h gh.Comment) bool {
		return h.ID == w.CommentID
	})
	if in < 0 {
		return
	}

	threads := slices.Clone(held.Detail.Threads)
	comments := slices.Clone(threads[at].Comments)
	comments[in] = c
	threads[at].Comments = comments
	held.Detail.Threads = threads
	s.put(id, held)
	s.markStale(id)
}

// CommentDeleteApplied takes the comment out of the held detail and drops the
// write it settles.
//
// Written into the detail rather than left to the fold, because the fold goes
// with the write. Dropping the write alone would put the deleted card back on
// the page until something refetched.
func (s *Store) CommentDeleteApplied(id, key string) {
	w, held, ok := s.settleWrite(id, key)
	if !ok {
		return
	}

	timeline, threads, fresh, freshThreads := foldWrites(
		[]CommentWrite{{CommentID: w.CommentID, ThreadID: w.ThreadID, Delete: true}},
		held.Detail.Timeline, held.Detail.Threads, false, false,
	)
	if !fresh && !freshThreads {
		return
	}

	held.Detail.Timeline = timeline
	held.Detail.Threads = threads
	s.put(id, held)
	s.markStale(id)
}

// CommentWriteReverted puts the comment back the way it was fetched. The caller
// owns saying why: the store cannot tell a rejected write from a lost one.
func (s *Store) CommentWriteReverted(id, key string) { s.dropWrite(id, key) }

// settleWrite drops the write a response answers for and hands it back with the
// held detail, or false when there is nothing to write into.
func (s *Store) settleWrite(id, key string) (CommentWrite, Detail, bool) {
	w, ok := s.dropWrite(id, key)
	if !ok {
		return CommentWrite{}, Detail{}, false
	}

	held, ok := s.details[id]
	if !ok {
		return CommentWrite{}, Detail{}, false
	}
	return w, held, true
}

// dropWrite removes one write and gives it back, with whether it was there. A
// response for a key already gone is one that already settled.
func (s *Store) dropWrite(id, key string) (CommentWrite, bool) {
	held := s.rewrites[id]
	at := slices.IndexFunc(held, func(w CommentWrite) bool { return w.Key == key })
	if at < 0 {
		return CommentWrite{}, false
	}

	w := held[at]
	s.rewrites[id] = slices.Delete(held, at, at+1)
	if len(s.rewrites[id]) == 0 {
		delete(s.rewrites, id)
	}
	return w, true
}

// commentAt is where a comment sits in a timeline, or -1. It matches the
// comment's own id rather than the item's, because an event carries no comment.
func commentAt(timeline []gh.TimelineItem, id string) int {
	if id == "" {
		return -1
	}
	return slices.IndexFunc(timeline, func(item gh.TimelineItem) bool {
		return item.Comment != nil && item.Comment.ID == id
	})
}
