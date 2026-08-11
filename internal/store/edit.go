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

	// Field is which part of the pull request this edit replaces. Two writes
	// settle in whatever order the network gives them, and only one on the same
	// field can overwrite the other's answer.
	Field() editField

	// Apply folds this write over a fetched detail. It must not write into any
	// slice the detail arrived holding: the caller may already have handed that
	// slice to a rendered page.
	Apply(gh.PullRequestDetail) gh.PullRequestDetail
}

// editField names the part of the pull request an edit replaces.
//
// It exists because the queue holds more than one kind. An answer is only ever
// stale against a later write on the same field: a label set landing while a
// state change is still out says nothing about the state, and gating one on the
// other drops an answer nobody is going to send again.
type editField int

const (
	fieldLabels editField = iota
	fieldState
)

// LabelEdit is a whole label set claimed for a pull request. The picker applies
// a set rather than a delta, and so does the mutation behind it.
type LabelEdit struct {
	key    string
	labels []gh.Label
}

func (e LabelEdit) Key() string      { return e.key }
func (e LabelEdit) Field() editField { return fieldLabels }

// Apply replaces the label set. Cloned, so a caller that appends to what it is
// handed cannot write into the slice this edit is still holding.
func (e LabelEdit) Apply(d gh.PullRequestDetail) gh.PullRequestDetail {
	d.Labels = slices.Clone(e.labels)
	return d
}

// StateEdit is a lifecycle change claimed for a pull request: ready, draft,
// closed or reopened.
//
// It carries the transition rather than the state it lands on. Two out at once
// then compose in the order they were pressed, and neither one captures a draft
// flag that was true when it was held and false by the time it applies.
type StateEdit struct {
	key string
	to  gh.PRTransition
}

func (e StateEdit) Key() string      { return e.key }
func (e StateEdit) Field() editField { return fieldState }

// Apply moves the two fields the transition touches. Nothing to clone: it
// writes scalars, so there is no slice here for a caller to be handed and then
// written into.
//
// Draft and closed are independent. Closing keeps the draft flag, because
// GitHub does, and reopening gives back whatever was closed.
func (e StateEdit) Apply(d gh.PullRequestDetail) gh.PullRequestDetail {
	switch e.to {
	case gh.TransitionReady:
		d.IsDraft = false
	case gh.TransitionDraft:
		d.IsDraft = true
	case gh.TransitionClose:
		d.State = gh.PRStateClosed
	case gh.TransitionReopen:
		d.State = gh.PRStateOpen
	}
	return d
}

// PendingState holds a lifecycle change applied here and not yet acknowledged,
// and returns the key the response reconciles against.
func (s *Store) PendingState(id string, to gh.PRTransition) string {
	key := s.nextKey()
	if s.edits == nil {
		s.edits = make(map[string][]Edit)
	}
	s.edits[id] = append(s.edits[id], StateEdit{key: key, to: to})
	return key
}

// StateApplied takes GitHub's answer for a lifecycle change and drops the write
// it settles, on the same terms as LabelsApplied: written into the held detail
// so the row does not fall back to the fetched state, and skipped while a later
// write on the state is still out.
//
// It writes no permissions. What the viewer may do next is GitHub's to say, and
// a locally guessed CanReopen would offer a menu item that opens a write GitHub
// rejects. The refetch the caller fires is what brings those back.
func (s *Store) StateApplied(id, key string, res gh.PRStateResult) {
	if !s.dropEdit(id, key) {
		return
	}

	held, ok := s.details[id]
	if !ok || s.laterEdit(id, fieldState) {
		return
	}

	held.Detail.State = res.State
	held.Detail.IsDraft = res.IsDraft
	s.put(id, held)
	s.markStale(id)
}

// PendingLabels holds a label set applied here and not yet acknowledged, and
// returns the key the response reconciles against.
func (s *Store) PendingLabels(id string, labels []gh.Label) string {
	key := s.nextKey()
	if s.edits == nil {
		s.edits = make(map[string][]Edit)
	}
	s.edits[id] = append(s.edits[id], LabelEdit{key: key, labels: slices.Clone(labels)})
	return key
}

// LabelsApplied takes GitHub's answer for a label set and drops the write it
// settles. The answer is written into the held detail rather than left for the
// next fetch: dropping the edit alone would put the fetched labels back on the
// screen until something refetched, which reads as the write undoing itself.
//
// It writes nothing while a later write on the labels is still out. Two settle
// in whatever order the network gives them, and the earlier one answering last
// would otherwise overwrite the reader's newer ask with a set they have already
// moved on from. The fold renders the later edit until it answers for itself.
//
// On the labels, not on the pull request. A state change in flight says nothing
// about what the labels are, and gating on it drops an answer nobody is going
// to send again: the fold would keep rendering the fetched set, and if the
// state write then failed there would be no edit left to explain it.
//
// No budget to fold: a mutation cannot report the rate limit.
func (s *Store) LabelsApplied(id, key string, res gh.LabelsResult) {
	if !s.dropEdit(id, key) {
		return
	}

	held, ok := s.details[id]
	if !ok || s.laterEdit(id, fieldLabels) {
		return
	}

	held.Detail.Labels = res.Labels
	s.put(id, held)
	s.markStale(id)
}

// EditReverted takes a metadata write back off the screen. The caller owns
// saying why: the store cannot tell a rejected write from a lost one.
func (s *Store) EditReverted(id, key string) { s.dropEdit(id, key) }

// nextKey mints the name a response reconciles against: a sequence rather than
// a clock, so the same run of keystrokes produces the same keys every time.
//
// One counter across comments, resolutions and edits, so no two writes in
// flight can take one key between them.
func (s *Store) nextKey() string {
	s.writes++
	return "pending-" + strconv.Itoa(s.writes)
}

// laterEdit reports whether another write on the same field is still out. Every
// edit left in the queue when one settles was held after it, so its answer is
// the one the reader is waiting on.
func (s *Store) laterEdit(id string, f editField) bool {
	return slices.ContainsFunc(s.edits[id], func(e Edit) bool { return e.Field() == f })
}

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
