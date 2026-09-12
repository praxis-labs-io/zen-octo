package store

import (
	"slices"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

// CommentWrite is a comment edited or deleted here and not yet answered for.
type CommentWrite struct {
	Key       string
	CommentID string

	// ThreadID is the review thread holding the comment, empty on a top-level comment or a review body.
	ThreadID string

	// Body is ignored on a delete.
	Body string

	Delete bool
}

// PendingCommentEdit holds a rewritten comment and returns the key its response reconciles against.
func (s *Store) PendingCommentEdit(id, commentID, threadID, body string) string {
	return s.holdWrite(id, CommentWrite{
		CommentID: commentID,
		ThreadID:  threadID,
		Body:      body,
	})
}

// PendingCommentDelete holds a deleted comment and returns the key its response reconciles against.
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

		said := *timeline[at].Comment
		said.Body = w.Body
		said.Editing = true
		timeline[at].Comment = &said
	}
	return timeline, threads, freshTimeline, freshThreads
}

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

	if w.Delete && len(threads[at].Comments) == 1 {
		return slices.Delete(threads, at, at+1), fresh
	}

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

// CommentEditApplied writes GitHub's version of the comment into the held detail and drops the write.
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

// CommentDeleteApplied removes the comment from the held detail and drops the write.
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

// CommentWriteReverted drops the write, putting the comment back as fetched.
func (s *Store) CommentWriteReverted(id, key string) { s.dropWrite(id, key) }

func (s *Store) settleWrite(id, key string) (CommentWrite, Detail, bool) {
	w, ok := s.dropWrite(id, key)
	if !ok {
		return CommentWrite{}, Detail{}, false
	}

	held, ok := s.details.look(id)
	if !ok {
		return CommentWrite{}, Detail{}, false
	}
	return w, held, true
}

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

func commentAt(timeline []gh.TimelineItem, id string) int {
	if id == "" {
		return -1
	}
	return slices.IndexFunc(timeline, func(item gh.TimelineItem) bool {
		return item.Comment != nil && item.Comment.ID == id
	})
}
