package store

import (
	"slices"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

// ThreadEdit is a review thread opened here and not yet answered for.
type ThreadEdit struct {
	key    string
	thread gh.ReviewThread
}

func (e ThreadEdit) Key() string      { return e.key }
func (e ThreadEdit) Field() editField { return fieldThreads }

func (e ThreadEdit) Apply(d gh.PullRequestDetail) gh.PullRequestDetail {
	d.Threads = append(slices.Clone(d.Threads), e.thread)
	return d
}

// PendingThread holds t, carrying the one comment that opens it, as the viewer's draft until GitHub
// answers. The thread and its comment take the returned key as their id and read as still posting.
func (s *Store) PendingThread(id string, t gh.ReviewThread) string {
	return s.holdEdit(id, func(key string) Edit {
		t.ID, t.Draft, t.CanReply = key, true, false
		t.Comments = slices.Clone(t.Comments)
		for i := range t.Comments {
			c := &t.Comments[i]
			c.ID, c.Kind, c.Pending, c.Draft = key, gh.CommentThread, true, true
		}
		return ThreadEdit{key: key, thread: t}
	})
}

// ThreadApplied puts GitHub's thread in place of the draft, and takes its review as the viewer's
// pending one when the write is what opened it.
func (s *Store) ThreadApplied(id, key string, res gh.ReviewThreadResult) {
	if _, ok := s.dropEdit(id, key); !ok {
		return
	}
	held, ok := s.details.look(id)
	if !ok {
		return
	}

	if threadAt(held.Detail.Threads, res.Thread.ID) < 0 {
		held.Detail.Threads = append(slices.Clone(held.Detail.Threads), res.Thread)
	}
	if held.Detail.DraftReview.ID == "" && res.Thread.ReviewID != "" {
		held.Detail.DraftReview = gh.Review{ID: res.Thread.ReviewID, State: gh.ReviewStatePending}
	}
	s.put(id, held)
	s.markStale(id)
}
