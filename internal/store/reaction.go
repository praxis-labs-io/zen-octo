package store

import (
	"slices"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

// ReactionWrite is a reaction toggled here and not yet answered for. An empty CommentID is the
// description; ThreadID is empty outside a review thread.
type ReactionWrite struct {
	Key       string
	CommentID string
	ThreadID  string

	Content gh.ReactionContent

	// On is the direction pressed rather than the resulting state, so two writes compose in press order.
	On bool
}

// PendingReaction holds a toggled reaction and returns the key its response reconciles against.
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

// putReaction rebuilds in GitHub's order so a pill does not move when the answer lands.
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

func reactionIn(held []gh.Reaction, content gh.ReactionContent) gh.Reaction {
	for _, r := range held {
		if r.Content == content {
			return r
		}
	}
	return gh.Reaction{Content: content}
}

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

			said := *timeline[at].Comment
			said.Reactions = applyReaction(said.Reactions, w.Content, w.On)
			timeline[at].Comment = &said
		}
	}
	return body, timeline, threads, freshTimeline, freshThreads
}

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

	comments := slices.Clone(threads[at].Comments)
	comments[in].Reactions = applyReaction(comments[in].Reactions, w.Content, w.On)
	threads[at].Comments = comments
	return threads, fresh
}

// ReactionApplied writes GitHub's count for the one reaction the write moved and drops the write.
// Only that group, because answers to two toggles on one subject can arrive in either order.
func (s *Store) ReactionApplied(id, key string, res gh.ReactionResult) {
	w, held, ok := s.settleReaction(id, key)
	if !ok {
		return
	}

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

// ReactionReverted drops the write, putting the reaction back as fetched.
func (s *Store) ReactionReverted(id, key string) { s.dropReaction(id, key) }

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
