package store

import (
	"slices"
	"strconv"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

// Edit is a metadata write on a pull request, applied here and not yet answered for.
type Edit interface {
	// Key is what the response reconciles against.
	Key() string

	// Field is the part of the pull request the edit replaces.
	Field() editField

	// Apply folds the edit over a detail. It must not write into any slice the detail holds.
	Apply(gh.PullRequestDetail) gh.PullRequestDetail
}

type editField int

const (
	fieldLabels editField = iota
	fieldState
	fieldAssignees
	fieldReviewers
	fieldBase
	fieldBody
)

// LabelEdit replaces a pull request's whole label set.
type LabelEdit struct {
	key    string
	labels []gh.Label
}

func (e LabelEdit) Key() string      { return e.key }
func (e LabelEdit) Field() editField { return fieldLabels }

func (e LabelEdit) Apply(d gh.PullRequestDetail) gh.PullRequestDetail {
	d.Labels = slices.Clone(e.labels)
	return d
}

// AssigneeEdit replaces a pull request's whole assignee set.
type AssigneeEdit struct {
	key       string
	assignees []gh.Actor
}

func (e AssigneeEdit) Key() string      { return e.key }
func (e AssigneeEdit) Field() editField { return fieldAssignees }

func (e AssigneeEdit) Apply(d gh.PullRequestDetail) gh.PullRequestDetail {
	d.Assignees = slices.Clone(e.assignees)
	return d
}

// ReviewerEdit replaces a pull request's whole reviewer panel.
type ReviewerEdit struct {
	key       string
	reviewers []gh.Reviewer
}

func (e ReviewerEdit) Key() string      { return e.key }
func (e ReviewerEdit) Field() editField { return fieldReviewers }

func (e ReviewerEdit) Apply(d gh.PullRequestDetail) gh.PullRequestDetail {
	d.Reviewers = slices.Clone(e.reviewers)
	return d
}

// StateEdit carries a lifecycle transition rather than a state, so two in flight compose in press order.
type StateEdit struct {
	key string
	to  gh.PRTransition
}

func (e StateEdit) Key() string      { return e.key }
func (e StateEdit) Field() editField { return fieldState }

// Apply leaves the draft flag alone on close and reopen, as GitHub does.
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

// BodyEdit replaces a pull request's description.
type BodyEdit struct {
	key  string
	body string
}

func (e BodyEdit) Key() string      { return e.key }
func (e BodyEdit) Field() editField { return fieldBody }

func (e BodyEdit) Apply(d gh.PullRequestDetail) gh.PullRequestDetail {
	d.Body = e.body
	return d
}

// PendingBody holds a rewritten description and returns the key its response reconciles against.
func (s *Store) PendingBody(id, body string) string {
	return s.holdEdit(id, func(key string) Edit { return BodyEdit{key: key, body: body} })
}

// BodyApplied settles a description write with GitHub's answer.
func (s *Store) BodyApplied(id, key string, res gh.BodyResult) {
	_, held, ok := s.settleEdit(id, key, fieldBody)
	if !ok {
		return
	}

	held.Detail.Body = res.Body
	s.put(id, held)
	s.markStale(id)
}

// PendingState holds a lifecycle change and returns the key its response reconciles against.
func (s *Store) PendingState(id string, to gh.PRTransition) string {
	return s.holdEdit(id, func(key string) Edit { return StateEdit{key: key, to: to} })
}

// StateApplied settles a lifecycle write with GitHub's state and draft flag, leaving permissions to the refetch.
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

// MergeEdit marks a pull request merged.
type MergeEdit struct {
	key string
}

func (e MergeEdit) Key() string { return e.key }

// Field is fieldState, so a merge and a close in flight settle last-held-wins and set StateWriting.
func (e MergeEdit) Field() editField { return fieldState }

func (e MergeEdit) Apply(d gh.PullRequestDetail) gh.PullRequestDetail {
	d.State = gh.PRStateMerged
	return d
}

// PendingMerge holds a merge and returns the key its response reconciles against.
func (s *Store) PendingMerge(id string) string {
	return s.holdEdit(id, func(key string) Edit { return MergeEdit{key: key} })
}

// MergeApplied settles a merge with GitHub's state. It marks the detail stale and not the diff.
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

// BaseEdit retargets a pull request onto another base branch.
type BaseEdit struct {
	key  string
	base string
}

func (e BaseEdit) Key() string      { return e.key }
func (e BaseEdit) Field() editField { return fieldBase }

// Apply sets BehindBy to BehindUnknown, because zero means up to date and only GitHub can compare.
func (e BaseEdit) Apply(d gh.PullRequestDetail) gh.PullRequestDetail {
	d.BaseRefName = e.base
	d.BehindBy = gh.BehindUnknown
	return d
}

// PendingBase holds a retarget and returns the key its response reconciles against.
func (s *Store) PendingBase(id, base string) string {
	return s.holdEdit(id, func(key string) Edit { return BaseEdit{key: key, base: base} })
}

// BaseApplied settles a retarget with GitHub's base and marks both the detail and the diff stale.
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

// PendingAssignees holds an assignee set and returns the key its response reconciles against.
func (s *Store) PendingAssignees(id string, assignees []gh.Actor) string {
	return s.holdEdit(id, func(key string) Edit {
		return AssigneeEdit{key: key, assignees: slices.Clone(assignees)}
	})
}

// AssigneesApplied settles an assignee write with GitHub's set.
func (s *Store) AssigneesApplied(id, key string, res gh.AssigneesResult) {
	_, held, ok := s.settleEdit(id, key, fieldAssignees)
	if !ok {
		return
	}

	held.Detail.Assignees = res.Assignees
	s.put(id, held)
	s.markStale(id)
}

// PendingReviewers holds a reviewer panel and returns the key its response reconciles against.
func (s *Store) PendingReviewers(id string, reviewers []gh.Reviewer) string {
	return s.holdEdit(id, func(key string) Edit {
		return ReviewerEdit{key: key, reviewers: slices.Clone(reviewers)}
	})
}

// ReviewersApplied settles a reviewer write by promoting its own panel into the held detail.
// The endpoint reports only outstanding requests, so there is no answer to take.
func (s *Store) ReviewersApplied(id, key string) {
	dropped, held, ok := s.settleEdit(id, key, fieldReviewers)
	if !ok {
		return
	}

	held.Detail = dropped.Apply(held.Detail)
	s.put(id, held)
	s.markStale(id)
}

// PendingLabels holds a label set and returns the key its response reconciles against.
func (s *Store) PendingLabels(id string, labels []gh.Label) string {
	return s.holdEdit(id, func(key string) Edit {
		return LabelEdit{key: key, labels: slices.Clone(labels)}
	})
}

func (s *Store) holdEdit(id string, mint func(key string) Edit) string {
	key := s.nextKey()
	if s.edits == nil {
		s.edits = make(map[string][]Edit)
	}
	s.edits[id] = append(s.edits[id], mint(key))
	return key
}

// settleEdit gates an answer only on later writes to its own field: another field's write says nothing about it.
func (s *Store) settleEdit(id, key string, f editField) (Edit, Detail, bool) {
	dropped, ok := s.dropEdit(id, key)
	if !ok {
		return nil, Detail{}, false
	}

	held, ok := s.details.look(id)
	if !ok || s.laterEdit(id, f) {
		return nil, Detail{}, false
	}
	return dropped, held, true
}

// LabelsApplied settles a label write with GitHub's set.
func (s *Store) LabelsApplied(id, key string, res gh.LabelsResult) {
	_, held, ok := s.settleEdit(id, key, fieldLabels)
	if !ok {
		return
	}

	held.Detail.Labels = res.Labels
	s.put(id, held)
	s.markStale(id)
}

// EditReverted drops a metadata write, putting the fetched value back.
func (s *Store) EditReverted(id, key string) { s.dropEdit(id, key) }

// EditRevertedStale is EditReverted that also marks the fetch in flight stale,
// for failures that mean the held detail is behind GitHub.
func (s *Store) EditRevertedStale(id, key string) {
	if _, ok := s.dropEdit(id, key); !ok {
		return
	}
	s.markStale(id)
}

// nextKey mints keys from a sequence rather than a clock, so the same keystrokes mint the same keys.
func (s *Store) nextKey() string {
	s.writes++
	return "pending-" + strconv.Itoa(s.writes)
}

func (s *Store) laterEdit(id string, f editField) bool {
	return slices.ContainsFunc(s.edits[id], func(e Edit) bool { return e.Field() == f })
}

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
