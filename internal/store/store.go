// Package store owns fetched state and the lifecycle of a refresh. Views read
// from it; they never fetch.
//
// It holds no goroutines and imports nothing from the UI. Concurrency lives in
// the commands the root model batches, and every mutation here happens in
// Update, which is what keeps -race quiet.
package store

import (
	"slices"

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

	// ThreadID is the review thread a reply hangs off, empty on a top-level
	// comment. It is what tells Detail which of the two places to fold this
	// into, and what tells PendingApplied which one GitHub answered for.
	ThreadID string
}

// Resolution is a review thread closed or opened here and not yet answered for.
//
// It is held beside the fetched detail for the reason Pending is, and it folds
// differently: there is nothing to add to the page, only a field to overwrite on
// a thread already on it. Resolved is the state the write is asking for.
type Resolution struct {
	Key      string
	ThreadID string
	Resolved bool
}

// Store holds every configured section, every detail opened this session, and
// the point budget across all of them.
type Store struct {
	sections []Section
	details  map[string]Detail
	files    map[string]Files
	commits  map[string]Files
	repos    map[string]Repo
	rate     gh.RateLimit
	viewer   gh.Actor

	// pending is the writes in flight, by pull request. The counter names them:
	// a sequence rather than a clock, so the same run of keystrokes produces the
	// same keys every time.
	//
	// resolving is the same for the toggle on a review thread, sharing the
	// counter so a comment and a resolve out at once can never take one key
	// between them.
	//
	// edits is the same for the metadata a picker applies, sharing the counter
	// for the same reason. It is a slice rather than one edit per field because
	// two writes on one field can be out at once, and the later one wins only
	// if the order they were held in survives.
	pending   map[string][]Pending
	resolving map[string][]Resolution
	edits     map[string][]Edit
	writes    int
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
		repos:    make(map[string]Repo),
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
// folded into it: a comment onto the timeline, a reply into the thread it
// answers, a resolve over the thread it settles. The zero value is one never
// opened, which reads as idle and unloaded.
//
// The fold happens here rather than at write time so a refetch cannot drop a
// pending comment. Callers ask for a detail at settle points, not per frame.
//
// The clones are lazy because most calls need none. A detail with nothing in
// flight returns above, and one with only a comment out must not pay for cloning
// every thread's comments to append to a timeline.
func (s Store) Detail(id string) Detail {
	held := s.details[id]
	waiting, settling, editing := s.pending[id], s.resolving[id], s.edits[id]
	if len(waiting) == 0 && len(settling) == 0 && len(editing) == 0 {
		return held
	}

	// Edits go first. Each replaces a whole value and none of them touches the
	// timeline or the threads, so the two folds below neither read what this
	// wrote nor write what it read.
	for _, e := range editing {
		held.Detail = e.Apply(held.Detail)
	}

	timeline, threads := held.Detail.Timeline, held.Detail.Threads
	var freshTimeline, freshThreads bool

	for _, p := range waiting {
		if p.ThreadID == "" {
			// A fresh slice, because the held timeline is shared with the map
			// and appending to it would leave the pending items behind on the
			// next call.
			if !freshTimeline {
				out := make([]gh.TimelineItem, len(timeline), len(timeline)+len(waiting))
				copy(out, timeline)
				timeline, freshTimeline = out, true
			}
			timeline = append(timeline, timelineComment(p.Comment))
			continue
		}

		at := threadAt(threads, p.ThreadID)

		// A refetch that landed while the reply was out may no longer carry the
		// thread: it was resolved and hidden, or it fell off the first page.
		// There is nowhere to hang the reply and nowhere honest to invent, so it
		// waits out of sight until the mutation answers.
		if at < 0 {
			continue
		}

		if !freshThreads {
			threads, freshThreads = slices.Clone(threads), true
		}

		// Cloning the outer slice is not enough. Each thread's comments are
		// their own slice, still the held one, and appending to it writes into
		// the detail this call was supposed to leave alone.
		threads[at].Comments = append(slices.Clone(threads[at].Comments), p.Comment)
	}

	for _, r := range settling {
		at := threadAt(threads, r.ThreadID)
		if at < 0 {
			continue
		}

		if !freshThreads {
			threads, freshThreads = slices.Clone(threads), true
		}

		// The outer clone is the whole of it: a resolve writes a field on the
		// thread and touches no comment, so the second level a reply needs is
		// not needed here.
		//
		// The state alone, never the permissions. A viewer allowed to resolve
		// and not to reopen has an inert key on the thread they just closed,
		// and flipping CanUnresolve locally would offer them a write GitHub
		// rejects.
		//
		// Marked pending so the screen keeps a second press off it. Two writes
		// out on one thread answer in whatever order the network gives them,
		// and the one that answers last writes its own state whether or not it
		// was the last one pressed.
		threads[at].IsResolved = r.Resolved
		threads[at].Pending = true
	}

	held.Detail.Timeline = timeline
	held.Detail.Threads = threads
	return held
}

// PendingComment holds a comment written but not yet acknowledged, and returns
// the key the response reconciles against. The comment renders from now on; the
// caller is expected to post it and answer with one of the two calls below.
func (s *Store) PendingComment(id string, c gh.Comment) string {
	return s.hold(id, "", c)
}

// PendingReply is PendingComment for an answer to a review thread. The kind is
// set here rather than taken from the caller: a reply is a thread comment
// whatever the screen thinks it is writing, and the kind picks the mutation that
// edits it later.
func (s *Store) PendingReply(id, threadID string, c gh.Comment) string {
	c.Kind = gh.CommentThread
	return s.hold(id, threadID, c)
}

func (s *Store) hold(id, threadID string, c gh.Comment) string {
	key := s.nextKey()

	c.Pending = true
	c.ID = key
	if s.pending == nil {
		s.pending = make(map[string][]Pending)
	}
	s.pending[id] = append(s.pending[id], Pending{Key: key, Comment: c, ThreadID: threadID})
	return key
}

// PendingResolve holds a thread closed or opened but not yet acknowledged, and
// returns the key the response reconciles against.
//
// One at a time per thread is the screen's rule, not this one's: the fold marks
// the thread pending and the key goes inert while it is. Two held here would
// settle in whatever order the responses arrive, which is not the order they
// were pressed in.
func (s *Store) PendingResolve(id, threadID string, resolved bool) string {
	key := s.nextKey()

	if s.resolving == nil {
		s.resolving = make(map[string][]Resolution)
	}
	s.resolving[id] = append(s.resolving[id], Resolution{Key: key, ThreadID: threadID, Resolved: resolved})
	return key
}

// ResolveApplied takes GitHub's answer for a thread, permissions and all:
// resolving flips which of the two the next press needs, and only GitHub knows
// whether this viewer may.
func (s *Store) ResolveApplied(id, key string, res gh.ThreadResult) {
	r, dropped := s.dropResolve(id, key)
	if !dropped {
		return
	}

	held, ok := s.details[id]
	if !ok {
		return
	}

	at := threadAt(held.Detail.Threads, r.ThreadID)

	// A refetch dropped the thread while the write was out. That refetch is the
	// truer picture, and writing the thread back would be this package
	// inventing state GitHub did not send.
	if at < 0 {
		return
	}

	// Cloned before the write, because the held slice is inside conversations
	// already rendered from a detail handed out earlier.
	threads := slices.Clone(held.Detail.Threads)
	threads[at].IsResolved = res.IsResolved
	threads[at].CanResolve = res.CanResolve
	threads[at].CanUnresolve = res.CanUnresolve
	held.Detail.Threads = threads
	s.put(id, held)
}

// ResolveReverted puts the thread back the way it was fetched. The caller owns
// saying why.
func (s *Store) ResolveReverted(id, key string) { s.dropResolve(id, key) }

// dropResolve is dropPending, one map over.
func (s *Store) dropResolve(id, key string) (Resolution, bool) {
	settling := s.resolving[id]
	at := slices.IndexFunc(settling, func(r Resolution) bool { return r.Key == key })
	if at < 0 {
		return Resolution{}, false
	}

	r := settling[at]
	s.resolving[id] = slices.Delete(settling, at, at+1)
	if len(s.resolving[id]) == 0 {
		delete(s.resolving, id)
	}
	return r, true
}

// threadAt is where a thread sits in a detail, or -1.
func threadAt(threads []gh.ReviewThread, id string) int {
	return slices.IndexFunc(threads, func(t gh.ReviewThread) bool { return t.ID == id })
}

// PendingApplied swaps the placeholder for what GitHub recorded. The real
// comment goes onto the held timeline, so it survives everything the
// placeholder was standing in for.
//
// No budget to fold: a mutation cannot report the rate limit, so the write's
// cost shows up on the next fetch.
func (s *Store) PendingApplied(id, key string, res gh.CommentResult) {
	p, dropped := s.dropPending(id, key)
	if !dropped {
		return
	}

	held, ok := s.details[id]
	if !ok {
		return
	}

	if p.ThreadID != "" {
		s.replyApplied(id, held, p.ThreadID, res.Comment)
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

// replyApplied puts a confirmed reply into the thread it answers.
func (s *Store) replyApplied(id string, held Detail, threadID string, c gh.Comment) {
	at := threadAt(held.Detail.Threads, threadID)

	// A refetch dropped the thread while the reply was out. That refetch is the
	// truer picture, and writing the thread back to hold one comment would be
	// this package inventing state GitHub did not send.
	if at < 0 || hasThreadComment(held.Detail.Threads[at].Comments, c.ID) {
		return
	}

	threads := slices.Clone(held.Detail.Threads)
	threads[at].Comments = append(slices.Clone(threads[at].Comments), c)
	held.Detail.Threads = threads
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

// hasThreadComment is the guard above, one level down. A refetch that landed
// while the reply was out already carries it, and the two copies would share a
// node id, which is the one thing the focus ring cannot survive.
func hasThreadComment(comments []gh.Comment, id string) bool {
	if id == "" {
		return false
	}
	return slices.ContainsFunc(comments, func(c gh.Comment) bool { return c.ID == id })
}

// PendingReverted takes the placeholder back off the screen. The caller owns
// saying why: the store has no way to tell a rejected write from a lost one.
func (s *Store) PendingReverted(id, key string) { s.dropPending(id, key) }

// dropPending removes one write and returns it, with whether it was there. A
// response for a key already gone is one that already settled, and applying it
// twice would put the comment in the conversation a second time.
//
// It gives back the write rather than a bare yes: the answer says nothing about
// which of the two places the comment belongs in, and only the write it settles
// knows.
func (s *Store) dropPending(id, key string) (Pending, bool) {
	waiting := s.pending[id]
	at := slices.IndexFunc(waiting, func(p Pending) bool { return p.Key == key })
	if at < 0 {
		return Pending{}, false
	}

	p := waiting[at]
	s.pending[id] = slices.Delete(waiting, at, at+1)
	if len(s.pending[id]) == 0 {
		delete(s.pending, id)
	}
	return p, true
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
