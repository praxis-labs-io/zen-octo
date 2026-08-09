// Package store owns fetched state and the lifecycle of a refresh. Views read
// from it; they never fetch.
//
// It holds no goroutines and imports nothing from the UI. Concurrency lives in
// the commands the root model batches, and every mutation here happens in
// Update, which is what keeps -race quiet.
package store

import (
	"slices"
	"strconv"

	"github.com/zen-octo/zen-octo/internal/config"
	"github.com/zen-octo/zen-octo/internal/gh"
)

// Status is where a section's last fetch got to.
type Status int

const (
	StatusIdle Status = iota
	StatusLoading
	StatusReady
	StatusFailed
)

// Section is one tab's worth of state: what to ask GitHub for, what came back,
// and where the fetch got to.
type Section struct {
	config.Section

	PRs    []gh.PullRequest
	Status Status
	Err    error

	// Loaded marks a section that has answered at least once. A reload puts it
	// back into StatusLoading, and without this the tab could not tell "no
	// count yet" from "a count, being checked".
	Loaded bool
}

// Detail is one pull request's full state, keyed by its id. Same lifecycle as a
// section: begun, then either applied or failed.
type Detail struct {
	Detail gh.PullRequestDetail
	Status Status
	Err    error

	// Loaded marks a detail that has answered at least once, so a background
	// refetch can be told from a first open.
	Loaded bool
}

// Files is one diff: a pull request's, keyed by the same id as its Detail, or
// one commit's, keyed by its sha. Both are held apart from the detail because
// each costs a request of its own, made only when someone asks to see it.
type Files struct {
	Files     []gh.ChangedFile
	MoreFiles int
	Truncated bool
	Status    Status
	Err       error

	// Loaded marks a diff that has answered at least once, so a refetch can be
	// told from a first open.
	Loaded bool
}

// Pending is a write applied here and not yet answered for. It renders as
// though it had landed, which is what optimistic means.
//
// It is held beside the fetched detail rather than inside it, and that is the
// whole point of the type. A refetch replaces a timeline wholesale, and a
// timeline fetched before the mutation answered is not evidence the mutation
// failed. Written into the detail, an optimistic comment would vanish on the
// next refresh with nothing to say why.
//
// Key is minted here and belongs to this session. It is not a node id, and the
// comment carrying it is marked Pending so nothing mistakes it for one.
type Pending struct {
	Key     string
	Comment gh.Comment
}

// Store holds every configured section, every detail opened this session, and
// the point budget across all of them.
type Store struct {
	sections []Section
	details  map[string]Detail
	files    map[string]Files
	commits  map[string]Files
	rate     gh.RateLimit
	viewer   gh.Actor

	// pending is the writes in flight, by pull request. The counter names them:
	// a sequence rather than a clock, so the same run of keystrokes produces the
	// same keys every time.
	pending map[string][]Pending
	writes  int
}

// New builds a store over the configured sections, none of them fetched.
func New(sections []config.Section) Store {
	held := make([]Section, len(sections))
	for i, s := range sections {
		held[i] = Section{Section: s}
	}
	return Store{
		sections: held,
		details:  make(map[string]Detail),
		files:    make(map[string]Files),
		commits:  make(map[string]Files),
	}
}

// Sections is a snapshot for the view.
func (s Store) Sections() []Section { return slices.Clone(s.sections) }

// Rate is the point budget as of the responses seen so far.
func (s Store) Rate() gh.RateLimit { return s.rate }

// Viewer is the account the token belongs to. The zero Actor is one not yet
// answered for, which reads the same as an account with no login.
func (s Store) Viewer() gh.Actor { return s.viewer }

// ViewerApplied stores the login and folds the response into the budget. There
// is no Begin or Failed beside it: the login is asked for once at startup, and
// nothing on the screen waits on it.
func (s *Store) ViewerApplied(res gh.ViewerResult) {
	s.viewer = res.Viewer
	s.adopt(res.RateLimit)
}

// Loading reports whether any section has a fetch in flight.
func (s Store) Loading() bool { return Loading(s.sections) }

// Loading is the same question asked of a snapshot, for a view holding one.
func Loading(sections []Section) bool {
	return slices.ContainsFunc(sections, func(sec Section) bool {
		return sec.Status == StatusLoading
	})
}

// BeginAll marks every section in flight.
func (s *Store) BeginAll() {
	for i := range s.sections {
		s.sections[i].Status = StatusLoading
	}
}

// Begin marks one section in flight and reports whether it started. It refuses
// a section that already has a request out, which is what holds the invariant
// the rest of this package rests on: one response per slot, so a late arrival
// never lands on rows that replaced the ones it was fetched for.
func (s *Store) Begin(i int) bool {
	if i < 0 || i >= len(s.sections) || s.sections[i].Status == StatusLoading {
		return false
	}
	s.sections[i].Status = StatusLoading
	return true
}

// Applied stores a section's rows and folds the response into the budget.
func (s *Store) Applied(i int, res gh.SearchResult) {
	if i < 0 || i >= len(s.sections) {
		return
	}
	s.sections[i].PRs = res.PullRequests
	s.sections[i].Status = StatusReady
	s.sections[i].Err = nil
	s.sections[i].Loaded = true
	s.adopt(res.RateLimit)
}

// Failed puts a section into its error state, keeping whatever rows it had. The
// view shows the error rather than the rows, and holding them means a retry
// that fails again has not also emptied the tab.
func (s *Store) Failed(i int, err error) {
	if i < 0 || i >= len(s.sections) {
		return
	}
	s.sections[i].Status = StatusFailed
	s.sections[i].Err = err
}

// Detail is what is held for a pull request, with whatever is still in flight
// folded into its timeline. The zero value is one never opened, which reads as
// idle and unloaded.
//
// The fold happens here rather than at write time so a refetch cannot drop a
// pending comment. Callers ask for a detail at settle points, not per frame.
func (s Store) Detail(id string) Detail {
	held := s.details[id]
	waiting := s.pending[id]
	if len(waiting) == 0 {
		return held
	}

	// A fresh slice, because the held timeline is shared with the map and
	// appending to it would leave the pending items behind on the next call.
	timeline := make([]gh.TimelineItem, len(held.Detail.Timeline), len(held.Detail.Timeline)+len(waiting))
	copy(timeline, held.Detail.Timeline)
	for _, p := range waiting {
		timeline = append(timeline, timelineComment(p.Comment))
	}

	held.Detail.Timeline = timeline
	return held
}

// PendingComment holds a comment written but not yet acknowledged, and returns
// the key the response reconciles against. The comment renders from now on; the
// caller is expected to post it and answer with one of the two calls below.
func (s *Store) PendingComment(id string, c gh.Comment) string {
	s.writes++
	key := "pending-" + strconv.Itoa(s.writes)

	c.Pending = true
	c.ID = key
	if s.pending == nil {
		s.pending = make(map[string][]Pending)
	}
	s.pending[id] = append(s.pending[id], Pending{Key: key, Comment: c})
	return key
}

// PendingApplied swaps the placeholder for what GitHub recorded. The real
// comment goes onto the held timeline, so it survives everything the
// placeholder was standing in for.
//
// No budget to fold: a mutation cannot report the rate limit, so the write's
// cost shows up on the next fetch.
func (s *Store) PendingApplied(id, key string, res gh.CommentResult) {
	if !s.dropPending(id, key) {
		return
	}

	held, ok := s.details[id]
	if !ok {
		return
	}

	// A refetch that landed while the write was out already carries it. Adding
	// it again puts the same comment on the page twice, and the two cards share
	// a node id, which is the one thing the focus ring cannot survive.
	if hasComment(held.Detail.Timeline, res.Comment.ID) {
		return
	}

	held.Detail.Timeline = append(held.Detail.Timeline, timelineComment(res.Comment))
	s.put(id, held)
}

// hasComment reports whether a timeline already holds a comment with this id.
func hasComment(timeline []gh.TimelineItem, id string) bool {
	if id == "" {
		return false
	}
	return slices.ContainsFunc(timeline, func(item gh.TimelineItem) bool {
		return item.Kind == gh.TimelineComment && item.Said().ID == id
	})
}

// PendingReverted takes the placeholder back off the screen. The caller owns
// saying why: the store has no way to tell a rejected write from a lost one.
func (s *Store) PendingReverted(id, key string) { s.dropPending(id, key) }

// dropPending removes one write and reports whether it was there. A response
// for a key already gone is one that already settled, and applying it twice
// would put the comment in the conversation a second time.
func (s *Store) dropPending(id, key string) bool {
	waiting := s.pending[id]
	at := slices.IndexFunc(waiting, func(p Pending) bool { return p.Key == key })
	if at < 0 {
		return false
	}

	s.pending[id] = slices.Delete(waiting, at, at+1)
	if len(s.pending[id]) == 0 {
		delete(s.pending, id)
	}
	return true
}

// timelineComment is a comment as the conversation reads one. A top-level
// comment is a timeline item whichever direction it arrived from.
func timelineComment(c gh.Comment) gh.TimelineItem {
	return gh.TimelineItem{
		Kind:      gh.TimelineComment,
		Actor:     c.Author,
		CreatedAt: c.CreatedAt,
		Comment:   &c,
	}
}

// BeginDetail marks a pull request in flight and reports whether it started. It
// refuses one already on its way, so opening a screen twice in quick succession
// costs one request rather than two.
func (s *Store) BeginDetail(id string) bool {
	held := s.details[id]
	if id == "" || held.Status == StatusLoading {
		return false
	}
	held.Status = StatusLoading
	s.put(id, held)
	return true
}

// DetailApplied stores a pull request and folds the response into the budget.
func (s *Store) DetailApplied(id string, res gh.DetailResult) {
	if id == "" {
		return
	}
	s.put(id, Detail{Detail: res.Detail, Status: StatusReady, Loaded: true})
	s.adopt(res.RateLimit)
}

// DetailFailed puts a pull request into its error state, keeping whatever it
// already held. A background refetch that fails must not empty a screen that
// was reading fine a moment ago.
func (s *Store) DetailFailed(id string, err error) {
	held, ok := s.details[id]
	if id == "" || !ok {
		return
	}
	held.Status = StatusFailed
	held.Err = err
	s.put(id, held)
}

// put writes a detail, building the map if this Store was made without New. A
// nil map panics on write, and it would do it inside Update.
func (s *Store) put(id string, d Detail) {
	if s.details == nil {
		s.details = make(map[string]Detail)
	}
	s.details[id] = d
}

// Files is the diff held for a pull request. The zero value is one never
// fetched, which reads as idle and unloaded.
func (s Store) Files(id string) Files { return s.files[id] }

// BeginFiles marks a diff in flight and reports whether it started. It refuses
// one already on its way, so tabbing in and out of Files costs one request.
func (s *Store) BeginFiles(id string) bool { return beginDiff(&s.files, id) }

// FilesApplied stores a pull request's diff. No budget to fold: the REST API
// bills against a separate allowance the GraphQL response knows nothing about.
func (s *Store) FilesApplied(id string, res gh.FilesResult) { diffApplied(&s.files, id, res) }

// FilesFailed puts a diff into its error state, keeping whatever it already
// held. A refetch that fails must not empty a diff that was reading fine.
func (s *Store) FilesFailed(id string, err error) { diffFailed(&s.files, id, err) }

// CommitFiles is the diff held for one commit, keyed by its sha rather than by
// the pull request. A commit belongs to whichever pull requests carry it, and
// its diff is the same either way.
func (s Store) CommitFiles(sha string) Files { return s.commits[sha] }

// BeginCommitFiles marks a commit's diff in flight and reports whether it
// started.
func (s *Store) BeginCommitFiles(sha string) bool { return beginDiff(&s.commits, sha) }

// CommitFilesApplied stores a commit's diff.
func (s *Store) CommitFilesApplied(sha string, res gh.FilesResult) {
	diffApplied(&s.commits, sha, res)
}

// CommitFilesFailed puts a commit's diff into its error state, keeping whatever
// it already held.
func (s *Store) CommitFilesFailed(sha string, err error) { diffFailed(&s.commits, sha, err) }

// The three below are the diff lifecycle, shared by the pull request's own and
// by each commit's. They take the map by pointer so a Store built without New
// can still be written to: a nil map panics on write, and it would do it inside
// Update.

func beginDiff(held *map[string]Files, key string) bool {
	at := (*held)[key]
	if key == "" || at.Status == StatusLoading {
		return false
	}
	at.Status = StatusLoading
	putDiff(held, key, at)
	return true
}

func diffApplied(held *map[string]Files, key string, res gh.FilesResult) {
	if key == "" {
		return
	}
	putDiff(held, key, Files{
		Files:     res.Files,
		MoreFiles: res.MoreFiles,
		Truncated: res.Truncated,
		Status:    StatusReady,
		Loaded:    true,
	})
}

func diffFailed(held *map[string]Files, key string, err error) {
	at, ok := (*held)[key]
	if key == "" || !ok {
		return
	}
	at.Status = StatusFailed
	at.Err = err
	putDiff(held, key, at)
}

func putDiff(held *map[string]Files, key string, f Files) {
	if *held == nil {
		*held = make(map[string]Files)
	}
	(*held)[key] = f
}

// adopt keeps the budget falling through a burst. Sections answer in whatever
// order they finish, so the newest arrival is not the truest one: the lowest
// remaining inside a window is. A later reset means a new window, and there the
// number legitimately goes back up.
//
// The lower-remaining clause is scoped to the held window on purpose. A
// straggler issued before a reset carries the old window's exhausted number,
// and taking it would read as an empty budget seconds after it refilled.
func (s *Store) adopt(r gh.RateLimit) {
	if r.Limit == 0 {
		return
	}

	switch {
	case s.rate.Limit == 0, r.ResetAt.After(s.rate.ResetAt):
		s.rate = r
	case r.ResetAt.Equal(s.rate.ResetAt) && r.Remaining < s.rate.Remaining:
		s.rate = r
	}
}
