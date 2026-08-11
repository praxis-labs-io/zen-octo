package store

import (
	"slices"
	"strconv"

	"github.com/zen-octo/zen-octo/internal/gh"
)

// Edit is one metadata write in flight: a field of the pull request claimed
// here and not yet answered for.
//
// Held beside the fetched detail for the reason Pending is, and folded at read
// time for the same one. Where a comment appends to a timeline, an edit
// replaces a whole value, so the fold is an assignment and the order they were
// held in decides what the reader sees: two out on one field settle last-held
// wins, which is also the order they were pressed.
//
// One interface rather than a map per field. Every metadata write differs only
// in which value it carries, and five near-identical fold blocks in Detail
// would be five places to forget the clone.
type Edit interface {
	// Key is what the response reconciles against. Minted by the store and
	// belonging to this session, never a node id.
	Key() string

	// Apply folds this write over a fetched detail. It must not write into any
	// slice the detail arrived holding: the caller may already have handed that
	// slice to a rendered page.
	Apply(gh.PullRequestDetail) gh.PullRequestDetail
}

// LabelEdit is a whole label set claimed for a pull request. The picker applies
// a set rather than a delta, and so does the mutation behind it.
type LabelEdit struct {
	key    string
	labels []gh.Label
}

func (e LabelEdit) Key() string { return e.key }

// Apply replaces the label set. Cloned, so a caller that appends to what it is
// handed cannot write into the slice this edit is still holding.
func (e LabelEdit) Apply(d gh.PullRequestDetail) gh.PullRequestDetail {
	d.Labels = slices.Clone(e.labels)
	return d
}

// PendingLabels holds a label set applied here and not yet acknowledged, and
// returns the key the response reconciles against.
func (s *Store) PendingLabels(id string, labels []gh.Label) string {
	return s.holdEdit(id, func(key string) Edit {
		return LabelEdit{key: key, labels: slices.Clone(labels)}
	})
}

// holdEdit mints the key and files the edit. The counter is the one comments
// and resolutions draw from, so no two writes in flight can take one key
// between them.
func (s *Store) holdEdit(id string, build func(key string) Edit) string {
	s.writes++
	key := "pending-" + strconv.Itoa(s.writes)

	if s.edits == nil {
		s.edits = make(map[string][]Edit)
	}
	s.edits[id] = append(s.edits[id], build(key))
	return key
}

// LabelsApplied takes GitHub's answer for a label set and drops the write it
// settles. The answer is written into the held detail rather than left for the
// next fetch: dropping the edit alone would put the fetched labels back on the
// screen until something refetched, which reads as the write undoing itself.
//
// No budget to fold: a mutation cannot report the rate limit.
func (s *Store) LabelsApplied(id, key string, res gh.LabelsResult) {
	if !s.dropEdit(id, key) {
		return
	}

	held, ok := s.details[id]
	if !ok {
		return
	}

	held.Detail.Labels = res.Labels
	s.put(id, held)
}

// EditReverted takes a metadata write back off the screen. The caller owns
// saying why: the store cannot tell a rejected write from a lost one.
func (s *Store) EditReverted(id, key string) { s.dropEdit(id, key) }

// dropEdit removes one write and reports whether it was there. A response for a
// key already gone is one that already settled.
//
// It gives back a bare yes, unlike dropPending: an edit settles by assignment
// and the response carries everything the assignment needs, so there is nothing
// on the held write the caller has to read back.
func (s *Store) dropEdit(id, key string) bool {
	held := s.edits[id]
	at := slices.IndexFunc(held, func(e Edit) bool { return e.Key() == key })
	if at < 0 {
		return false
	}

	s.edits[id] = slices.Delete(held, at, at+1)
	if len(s.edits[id]) == 0 {
		delete(s.edits, id)
	}
	return true
}
