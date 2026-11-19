package store

import (
	"cmp"
	"slices"
	"time"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

// ReviewEdit submits the viewer's review: the pending one when there is one, or a new one opened to be submitted.
type ReviewEdit struct {
	key   string
	id    string
	state gh.ReviewState
	body  string
	by    gh.Actor
	at    time.Time

	saving bool
}

func (e ReviewEdit) Key() string      { return e.key }
func (e ReviewEdit) Field() editField { return fieldReview }

// Apply publishes every draft comment along with the review: GitHub holds one pending review per viewer.
func (e ReviewEdit) Apply(d gh.PullRequestDetail) gh.PullRequestDetail {
	id := cmp.Or(e.id, d.DraftReview.ID, e.key)
	if d.DraftReview.ID != "" {
		d.Threads = publishDrafts(d.Threads)
	}
	d.DraftReview = gh.Review{}

	if !hasReview(d.Timeline, id) {
		said := gh.Comment{
			Kind:            gh.CommentReview,
			ID:              id,
			Author:          e.by,
			CreatedAt:       e.at,
			Body:            e.body,
			ViewerDidAuthor: true,
			Editing:         e.saving,
		}
		d.Timeline = append(slices.Clone(d.Timeline), gh.TimelineItem{
			Kind:      gh.TimelineReview,
			Actor:     e.by,
			CreatedAt: e.at,
			Comment:   &said,
			Review:    e.state,
		})
	}

	d.Reviewers = reviewedBy(d.Reviewers, e.by.Login, e.state)
	gh.RecountThreads(&d)
	return d
}

func (e ReviewEdit) landed(r gh.Review) ReviewEdit {
	e.id, e.saving = r.ID, false
	e.state = cmp.Or(r.State, e.state)
	e.body = r.Body
	return e
}

func hasReview(timeline []gh.TimelineItem, id string) bool {
	return slices.ContainsFunc(timeline, func(item gh.TimelineItem) bool {
		return item.Kind == gh.TimelineReview && item.Said().ID == id
	})
}

func publishDrafts(threads []gh.ReviewThread) []gh.ReviewThread {
	out := slices.Clone(threads)
	for i := range out {
		out[i].Draft = false
		out[i].Comments = slices.Clone(out[i].Comments)
		for j := range out[i].Comments {
			out[i].Comments[j].Draft = false
		}
	}
	return out
}

// A submitted review takes its author off the requests, as GitHub does.
func reviewedBy(panel []gh.Reviewer, login string, state gh.ReviewState) []gh.Reviewer {
	out := slices.Clone(panel)
	if login == "" {
		return out
	}
	at := slices.IndexFunc(out, func(r gh.Reviewer) bool { return r.Actor.Login == login })
	if at < 0 {
		return append(out, gh.Reviewer{Actor: gh.Actor{Login: login}, State: state})
	}
	out[at].State, out[at].Requested = state, false
	return out
}

// DiscardEdit throws away the viewer's pending review and every draft comment in it.
type DiscardEdit struct {
	key string
	id  string
}

func (e DiscardEdit) Key() string      { return e.key }
func (e DiscardEdit) Field() editField { return fieldReview }

func (e DiscardEdit) Apply(d gh.PullRequestDetail) gh.PullRequestDetail {
	if d.DraftReview.ID != e.id {
		return d
	}
	d.DraftReview = gh.Review{}

	threads := make([]gh.ReviewThread, 0, len(d.Threads))
	for _, t := range d.Threads {
		if t.Draft {
			continue
		}
		t.Comments = slices.DeleteFunc(slices.Clone(t.Comments), func(c gh.Comment) bool { return c.Draft })
		threads = append(threads, t)
	}
	d.Threads = threads

	d.Reviewers = slices.Clone(d.Reviewers)
	gh.RecountThreads(&d)
	return d
}

// PendingReview holds a review submitted as by at the given time, which the store has no clock to know.
// reviewID names the pending review being submitted, or is empty when the submit opens one.
func (s *Store) PendingReview(id, reviewID string, event gh.ReviewEvent, body string, by gh.Actor, at time.Time) string {
	return s.holdEdit(id, func(key string) Edit {
		return ReviewEdit{key: key, id: reviewID, state: event.Submitted(), body: body, by: by, at: at, saving: true}
	})
}

// ReviewApplied folds GitHub's review into the held detail and marks it stale: the review decision is GitHub's to say.
func (s *Store) ReviewApplied(id, key string, res gh.ReviewResult) {
	dropped, held, ok := s.settleEdit(id, key, fieldReview)
	if !ok {
		return
	}
	submitted, ok := dropped.(ReviewEdit)
	if !ok {
		return
	}

	held.Detail = submitted.landed(res.Review).Apply(held.Detail)
	s.put(id, held)
	s.markStale(id)
}

func (s *Store) PendingDiscard(id, reviewID string) string {
	return s.holdEdit(id, func(key string) Edit { return DiscardEdit{key: key, id: reviewID} })
}

func (s *Store) DiscardApplied(id, key string) {
	dropped, held, ok := s.settleEdit(id, key, fieldReview)
	if !ok {
		return
	}

	held.Detail = dropped.Apply(held.Detail)
	s.put(id, held)
	s.markStale(id)
}
