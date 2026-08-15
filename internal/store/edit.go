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
	fieldAssignees
	fieldReviewers
	fieldBase
	fieldBody
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

// AssigneeEdit is a whole assignee set claimed for a pull request. The picker
// applies a set rather than a delta, and so does the mutation behind it.
type AssigneeEdit struct {
	key       string
	assignees []gh.Actor
}

func (e AssigneeEdit) Key() string      { return e.key }
func (e AssigneeEdit) Field() editField { return fieldAssignees }

// Apply replaces the assignee set. Cloned, for the reason LabelEdit gives.
func (e AssigneeEdit) Apply(d gh.PullRequestDetail) gh.PullRequestDetail {
	d.Assignees = slices.Clone(e.assignees)
	return d
}

// ReviewerEdit is a whole reviewer panel claimed for a pull request.
//
// The whole panel rather than the requests it changes. The rail lists everyone
// who has reviewed alongside everyone still being waited on, the write only
// moves the second group, and the two are one slice here: carrying the delta
// would mean rebuilding the first group at fold time from a detail that may
// have been refetched since.
type ReviewerEdit struct {
	key       string
	reviewers []gh.Reviewer
}

func (e ReviewerEdit) Key() string      { return e.key }
func (e ReviewerEdit) Field() editField { return fieldReviewers }

// Apply replaces the reviewer panel. Cloned, for the reason LabelEdit gives.
func (e ReviewerEdit) Apply(d gh.PullRequestDetail) gh.PullRequestDetail {
	d.Reviewers = slices.Clone(e.reviewers)
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

// BodyEdit is a rewritten description claimed for a pull request.
//
// The description reads as a comment on the screen and is a field of the pull
// request to GitHub, which is why it is here rather than among the comment
// writes: one mutation replaces it, and nothing has to be found in a timeline
// to apply it.
type BodyEdit struct {
	key  string
	body string
}

func (e BodyEdit) Key() string      { return e.key }
func (e BodyEdit) Field() editField { return fieldBody }

// Apply replaces the description. Nothing to clone: it writes a string.
func (e BodyEdit) Apply(d gh.PullRequestDetail) gh.PullRequestDetail {
	d.Body = e.body
	return d
}

// PendingBody holds a rewritten description applied here and not yet
// acknowledged, and returns the key the response reconciles against.
func (s *Store) PendingBody(id, body string) string {
	return s.holdEdit(id, func(key string) Edit { return BodyEdit{key: key, body: body} })
}

// BodyApplied takes GitHub's answer for a description, on the terms settleEdit
// sets out. A fetch already in flight was answered from the text before the
// write, so it is marked stale the way every other settled edit marks one.
func (s *Store) BodyApplied(id, key string, res gh.BodyResult) {
	_, held, ok := s.settleEdit(id, key, fieldBody)
	if !ok {
		return
	}

	held.Detail.Body = res.Body
	s.put(id, held)
	s.markStale(id)
}

// PendingState holds a lifecycle change applied here and not yet acknowledged,
// and returns the key the response reconciles against.
func (s *Store) PendingState(id string, to gh.PRTransition) string {
	return s.holdEdit(id, func(key string) Edit { return StateEdit{key: key, to: to} })
}

// StateApplied takes GitHub's answer for a lifecycle change and drops the write
// it settles, on the terms settleEdit sets out.
//
// It writes no permissions. What the viewer may do next is GitHub's to say, and
// a locally guessed CanReopen would offer a menu item that opens a write GitHub
// rejects. The refetch the caller fires is what brings those back.
func (s *Store) StateApplied(id, key string, res gh.PRStateResult) {
	_, held, ok := s.settleEdit(id, key, fieldState)
	if !ok {
		return
	}

	held.Detail.State = res.State
	held.Detail.IsDraft = res.IsDraft
	s.put(id, held)
	s.syncRow(id)
	s.markStale(id)
}

// MergeEdit is a merge claimed for a pull request, before GitHub has said so.
//
// It carries no method, unlike every other edit here carrying its value. A
// merge lands a pull request in one place however it was made, so the method is
// the caller's business on the way out and nothing this has to fold.
type MergeEdit struct {
	key string
}

func (e MergeEdit) Key() string { return e.key }

// Field is the state, not a field of its own. A merge is a lifecycle move, so a
// close and a merge in flight together settle last-held-wins the way two
// lifecycle writes do, and the fold marks the detail StateWriting for free:
// during the round trip the state says merged while the permissions still say
// closable, and the State row has to wait that out rather than believe them.
func (e MergeEdit) Field() editField { return fieldState }

// Apply moves the lifecycle and nothing else. A scalar, so nothing to clone.
//
// It leaves the merge state alone. mergeStateStatus is what stands in the way
// of merging and it says nothing once the merge has happened; the rail reads
// the lifecycle first, so the row says "Merged" whatever GitHub last answered
// there. That is also why there is no MergeWriting flag beside StateWriting:
// the optimistic state is what the row is gated on.
func (e MergeEdit) Apply(d gh.PullRequestDetail) gh.PullRequestDetail {
	d.State = gh.PRStateMerged
	return d
}

// PendingMerge holds a merge applied here and not yet acknowledged, and returns
// the key the response reconciles against.
func (s *Store) PendingMerge(id string) string {
	return s.holdEdit(id, func(key string) Edit { return MergeEdit{key: key} })
}

// MergeApplied takes GitHub's answer for a merge and drops the write it
// settles, on the terms settleEdit sets out.
//
// It marks the detail stale and not the diff. A merge writes a commit onto the
// base and changes nothing about the difference between the two branches, which
// is what the Files tab is showing.
func (s *Store) MergeApplied(id, key string, res gh.MergeResult) {
	_, held, ok := s.settleEdit(id, key, fieldState)
	if !ok {
		return
	}

	held.Detail.State = res.State
	s.put(id, held)
	s.syncRow(id)
	s.markStale(id)
}

// BaseEdit is a retarget claimed for a pull request: the branch it now merges
// into, before GitHub has said so.
type BaseEdit struct {
	key  string
	base string
}

func (e BaseEdit) Key() string      { return e.key }
func (e BaseEdit) Field() editField { return fieldBase }

// Apply moves the base and takes the behind-by count with it. Scalars, so
// nothing to clone.
//
// The count goes to BehindUnknown rather than to zero. It is a comparison of
// two branches that only GitHub can run, the old number was counted against a
// branch this pull request no longer targets, and zero already means up to
// date: writing it here would render "Up to date with develop" over a branch
// forty commits ahead.
func (e BaseEdit) Apply(d gh.PullRequestDetail) gh.PullRequestDetail {
	d.BaseRefName = e.base
	d.BehindBy = gh.BehindUnknown
	return d
}

// PendingBase holds a retarget applied here and not yet acknowledged, and
// returns the key the response reconciles against.
func (s *Store) PendingBase(id, base string) string {
	return s.holdEdit(id, func(key string) Edit { return BaseEdit{key: key, base: base} })
}

// BaseApplied takes GitHub's answer for a retarget and drops the write it
// settles, on the terms settleEdit sets out.
//
// The unknown count survives the settle, and that is the point of writing it
// again here. The write has landed and the comparison has not been run: letting
// the fetched number back would put a count against the old branch under the
// name of the new one for as long as the refetch takes.
//
// The diff is marked stale beside it. A base change rewrites every file in one,
// and the Files tab asks for a diff once per open, so nothing else would ask
// again.
func (s *Store) BaseApplied(id, key string, res gh.BaseResult) {
	_, held, ok := s.settleEdit(id, key, fieldBase)
	if !ok {
		return
	}

	held.Detail.BaseRefName = res.BaseRefName
	held.Detail.BehindBy = gh.BehindUnknown
	s.put(id, held)
	s.syncRow(id)
	s.markStale(id)
	s.markFilesStale(id)
}

// PendingAssignees holds an assignee set applied here and not yet acknowledged,
// and returns the key the response reconciles against.
func (s *Store) PendingAssignees(id string, assignees []gh.Actor) string {
	return s.holdEdit(id, func(key string) Edit {
		return AssigneeEdit{key: key, assignees: slices.Clone(assignees)}
	})
}

// AssigneesApplied takes GitHub's answer for an assignee set and drops the
// write it settles, on the terms settleEdit sets out.
func (s *Store) AssigneesApplied(id, key string, res gh.AssigneesResult) {
	_, held, ok := s.settleEdit(id, key, fieldAssignees)
	if !ok {
		return
	}

	held.Detail.Assignees = res.Assignees
	s.put(id, held)
	s.markStale(id)
}

// PendingReviewers holds a reviewer panel applied here and not yet
// acknowledged, and returns the key the response reconciles against.
func (s *Store) PendingReviewers(id string, reviewers []gh.Reviewer) string {
	return s.holdEdit(id, func(key string) Edit {
		return ReviewerEdit{key: key, reviewers: slices.Clone(reviewers)}
	})
}

// ReviewersApplied settles a reviewer write by promoting the panel it put up
// into the held detail, on the terms settleEdit sets out.
//
// It is the one Applied that takes no answer from GitHub, because there is none
// worth taking: the endpoint reports the requests the pull request now holds and
// says nothing about who has already reviewed, so the panel cannot be rebuilt
// from it. Promoting the write's own value is what keeps the rail still. Just
// dropping the edit would put the fetched panel back for as long as the refetch
// takes, which reads as the write undoing itself.
//
// The refetch is what reconciles, and marking the fetch stale is what makes
// sure the one that lands was asked for after the write rather than before it.
func (s *Store) ReviewersApplied(id, key string) {
	dropped, held, ok := s.settleEdit(id, key, fieldReviewers)
	if !ok {
		return
	}

	held.Detail = dropped.Apply(held.Detail)
	s.put(id, held)
	s.markStale(id)
}

// PendingLabels holds a label set applied here and not yet acknowledged, and
// returns the key the response reconciles against.
func (s *Store) PendingLabels(id string, labels []gh.Label) string {
	return s.holdEdit(id, func(key string) Edit {
		return LabelEdit{key: key, labels: slices.Clone(labels)}
	})
}

// holdEdit mints a key, puts the write on the queue and hands the key back. The
// edit is built from the key rather than handed in, so no caller can append one
// naming a key the store did not issue.
func (s *Store) holdEdit(id string, mint func(key string) Edit) string {
	key := s.nextKey()
	if s.edits == nil {
		s.edits = make(map[string][]Edit)
	}
	s.edits[id] = append(s.edits[id], mint(key))
	return key
}

// settleEdit drops the write a response answers for and hands back both it and
// the held detail to write the answer into, or false when there is nothing to
// write.
//
// Two guards, and both are load-bearing. A key already gone is a write that
// settled already. A later write still out on the same field means the earlier
// one is answering last, carrying a value the reader has moved on from; the
// fold renders the later edit until it answers for itself.
//
// On the same field, never on the pull request. A state change in flight says
// nothing about what the labels are, and gating one on the other drops an
// answer nobody is going to send again: the fold would keep rendering the
// fetched set, and if the state write then failed there would be no edit left
// to explain it.
//
// The caller writes the answer into the held detail rather than leaving it for
// the next fetch. Dropping the edit alone would put the fetched value back on
// the screen until something refetched, which reads as the write undoing
// itself.
func (s *Store) settleEdit(id, key string, f editField) (Edit, Detail, bool) {
	dropped, ok := s.dropEdit(id, key)
	if !ok {
		return nil, Detail{}, false
	}

	held, ok := s.details[id]
	if !ok || s.laterEdit(id, f) {
		return nil, Detail{}, false
	}
	return dropped, held, true
}

// LabelsApplied takes GitHub's answer for a label set and drops the write it
// settles, on the terms settleEdit sets out.
//
// No budget to fold: a mutation cannot report the rate limit.
func (s *Store) LabelsApplied(id, key string, res gh.LabelsResult) {
	_, held, ok := s.settleEdit(id, key, fieldLabels)
	if !ok {
		return
	}

	held.Detail.Labels = res.Labels
	s.put(id, held)
	s.markStale(id)
}

// EditReverted takes a metadata write back off the screen. The caller owns
// saying why: the store cannot tell a rejected write from a lost one.
func (s *Store) EditReverted(id, key string) { s.dropEdit(id, key) }

// EditRevertedStale takes a failed write off the screen and marks the fetch in
// flight stale, for the writes whose failure is itself evidence that what is
// held no longer matches GitHub.
//
// Two of them. A reviewer write can fail with part of it already applied,
// because requesting and cancelling are separate calls and the second can fail
// over the first. A merge is refused for reasons that are all about the screen
// being out of date, the head having moved since it was fetched most of all, so
// putting the fetched row back and saying nothing leaves the reader looking at
// the same stale answer that just lost them the merge.
//
// Everything else is all or nothing, and reverting one of those says the pull
// request never moved, which is true and needs no refetch.
//
// The caller owes the refetch either way. This only makes sure the answer it
// takes was asked for after the failure rather than before it.
func (s *Store) EditRevertedStale(id, key string) {
	if _, ok := s.dropEdit(id, key); !ok {
		return
	}
	s.markStale(id)
}

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

// dropEdit removes one write and gives it back, or false when it was not
// there. A response for a key already gone is one that already settled.
//
// The write itself comes back because most responses carry a replacement for
// what it claimed and one does not: a reviewer write is answered by an endpoint
// that reports the requests it now holds and nothing about who has already
// reviewed. That one promotes its own optimistic value into the held detail
// instead, which needs the value read back off the write.
func (s *Store) dropEdit(id, key string) (Edit, bool) {
	held := s.edits[id]
	at := slices.IndexFunc(held, func(e Edit) bool { return e.Key() == key })
	if at < 0 {
		return nil, false
	}

	dropped := held[at]
	s.edits[id] = slices.Delete(held, at, at+1)
	if len(s.edits[id]) == 0 {
		delete(s.edits, id)
	}
	return dropped, true
}
