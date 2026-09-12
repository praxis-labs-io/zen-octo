// Package store owns fetched state and the writes in flight over it.
package store

import (
	"slices"

	"github.com/praxis-labs-io/zen-octo/internal/config"
	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

type Status int

const (
	StatusIdle Status = iota
	StatusLoading
	StatusReady
	StatusFailed
)

type Section struct {
	config.Section

	PRs    []gh.PullRequest
	Status Status
	Err    error

	// Loaded is true once the section has answered, so a reload can be told from a first fetch.
	Loaded bool
}

type Detail struct {
	Detail gh.PullRequestDetail
	Status Status
	Err    error

	// Loaded is true once the detail has answered, so a refetch can be told from a first open.
	Loaded bool

	// StateWriting is true while a lifecycle write is out, when the permissions still describe the old state.
	StateWriting bool

	// BaseWriting is true while a retarget is out, which BehindUnknown alone cannot tell from one that landed.
	BaseWriting bool
}

type Files struct {
	Files     []gh.ChangedFile
	MoreFiles int
	Truncated bool
	Status    Status
	Err       error

	Loaded bool
}

type Job struct {
	Job    gh.Job
	Log    string
	Status Status
	Err    error
	Loaded bool
}

// Pending is a comment applied here and not yet answered for. Key is minted by the store, never a node id.
type Pending struct {
	Key     string
	Comment gh.Comment

	// ThreadID is the review thread a reply hangs off, empty on a top-level comment.
	ThreadID string
}

// Resolution is a review thread resolved or unresolved here and not yet answered for.
type Resolution struct {
	Key      string
	ThreadID string
	Resolved bool
}

// FileView is a file marked viewed or unviewed here and not yet answered for.
type FileView struct {
	Key    string
	Path   string
	Viewed bool
}

type Store struct {
	sections []Section

	details cache[Detail]
	files   cache[Files]
	commits cache[Files]
	jobs    cache[Job]

	repos    map[string]Repo
	branches map[string]Branches
	rate     gh.RateLimit
	viewer   gh.Actor

	pending   map[string][]Pending
	resolving map[string][]Resolution
	edits     map[string][]Edit
	rewrites  map[string][]CommentWrite
	reacting  map[string][]ReactionWrite
	viewing   map[string][]FileView
	writes    int

	staleFetch map[string]bool
	staleFiles map[string]bool

	pulsing    map[string]bool
	stalePulse map[string]bool

	staleTimeline map[string]bool

	seq        int
	sectionSeq []int
	rowSeq     map[string]int
}

func New(sections []config.Section) Store {
	held := make([]Section, len(sections))
	for i, s := range sections {
		held[i] = Section{Section: s}
	}
	return Store{
		sections: held,
		repos:    make(map[string]Repo),
		branches: make(map[string]Branches),

		details:       newCache[Detail](detailCap),
		files:         newCache[Files](filesCap),
		commits:       newCache[Files](commitCap),
		jobs:          newCache[Job](jobCap),
		pulsing:       make(map[string]bool),
		stalePulse:    make(map[string]bool),
		staleTimeline: make(map[string]bool),
	}
}

// Sections is a snapshot for the view.
func (s Store) Sections() []Section { return slices.Clone(s.sections) }

func (s Store) Rate() gh.RateLimit { return s.rate }

func (s Store) Viewer() gh.Actor { return s.viewer }

func (s *Store) ViewerApplied(res gh.ViewerResult) {
	s.viewer = res.Viewer
	s.adopt(res.RateLimit)
}

func (s Store) Loading() bool { return Loading(s.sections) }

func Loading(sections []Section) bool {
	return slices.ContainsFunc(sections, func(sec Section) bool {
		return sec.Status == StatusLoading
	})
}

func (s *Store) BeginAll() {
	for i := range s.sections {
		s.sections[i].Status = StatusLoading
		s.stampSection(i)
	}
}

// nextSeq is the one write here held in an int, so a caller on a copy of the model loses it.
func (s *Store) nextSeq() int {
	s.seq++
	return s.seq
}

func (s *Store) stampSection(i int) {
	if len(s.sectionSeq) != len(s.sections) {
		s.sectionSeq = make([]int, len(s.sections))
	}
	s.sectionSeq[i] = s.nextSeq()
}

// Begin marks one section in flight and reports whether it started. It refuses one already in flight.
func (s *Store) Begin(i int) bool {
	if i < 0 || i >= len(s.sections) || s.sections[i].Status == StatusLoading {
		return false
	}
	s.sections[i].Status = StatusLoading
	s.stampSection(i)
	return true
}

// Applied stores a section's rows, keeping any row written since its fetch went out, and folds the budget.
func (s *Store) Applied(i int, res gh.SearchResult) {
	if i < 0 || i >= len(s.sections) {
		return
	}
	s.sections[i].PRs = res.PullRequests
	s.sections[i].Status = StatusReady
	s.sections[i].Err = nil
	s.sections[i].Loaded = true
	s.restoreRows(i)
	s.adopt(res.RateLimit)
}

func (s *Store) restoreRows(i int) {
	var began int
	if i < len(s.sectionSeq) {
		began = s.sectionSeq[i]
	}

	rows := s.sections[i].PRs
	for n, row := range rows {
		if s.rowSeq[row.ID] <= began {
			continue
		}
		if held := s.Detail(row.ID).Detail.PullRequest; held.ID != "" {
			rows[n] = held
		}
	}
}

// Failed puts a section into its error state, keeping its rows.
func (s *Store) Failed(i int, err error) {
	if i < 0 || i >= len(s.sections) {
		return
	}
	s.sections[i].Status = StatusFailed
	s.sections[i].Err = err
}

// PollFailed ends a background poll's flight, restoring the section's last status without recording the error.
func (s *Store) PollFailed(i int) {
	if i < 0 || i >= len(s.sections) || s.sections[i].Status != StatusLoading {
		return
	}
	if s.sections[i].Err != nil {
		s.sections[i].Status = StatusFailed
		return
	}
	s.sections[i].Status = StatusReady
}

// Detail is what is held for a pull request with every write in flight folded in, or the zero value if never opened.
func (s Store) Detail(id string) Detail {
	held := s.details.get(id)
	waiting, settling, editing := s.pending[id], s.resolving[id], s.edits[id]
	rewriting, reacting := s.rewrites[id], s.reacting[id]
	if len(waiting) == 0 && len(settling) == 0 && len(editing) == 0 &&
		len(rewriting) == 0 && len(reacting) == 0 {
		return held
	}

	for _, e := range editing {
		held.Detail = e.Apply(held.Detail)
		held.StateWriting = held.StateWriting || e.Field() == fieldState
		held.BaseWriting = held.BaseWriting || e.Field() == fieldBase
	}

	timeline, threads := held.Detail.Timeline, held.Detail.Threads
	var freshTimeline, freshThreads bool

	timeline, threads, freshTimeline, freshThreads = foldWrites(
		rewriting, timeline, threads, freshTimeline, freshThreads)

	held.Detail.Reactions, timeline, threads, freshTimeline, freshThreads = foldReactions(
		reacting, held.Detail.Reactions, timeline, threads, freshTimeline, freshThreads)

	for _, p := range waiting {
		if p.ThreadID == "" {
			if !freshTimeline {
				out := make([]gh.TimelineItem, len(timeline), len(timeline)+len(waiting))
				copy(out, timeline)
				timeline, freshTimeline = out, true
			}
			timeline = append(timeline, timelineComment(p.Comment))
			continue
		}

		at := threadAt(threads, p.ThreadID)

		if at < 0 {
			continue
		}

		if !freshThreads {
			threads, freshThreads = slices.Clone(threads), true
		}

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

		threads[at].IsResolved = r.Resolved
		threads[at].Pending = true
	}

	held.Detail.Timeline = timeline
	held.Detail.Threads = threads

	if freshThreads {
		held.Detail.Reviewers = slices.Clone(held.Detail.Reviewers)
		gh.RecountThreads(&held.Detail)
	}
	return held
}

func (s *Store) PendingComment(id string, c gh.Comment) string {
	return s.hold(id, "", c)
}

// PendingReply is PendingComment for a reply to threadID, and sets the comment's Kind to CommentThread.
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

func (s *Store) PendingResolve(id, threadID string, resolved bool) string {
	key := s.nextKey()

	if s.resolving == nil {
		s.resolving = make(map[string][]Resolution)
	}
	s.resolving[id] = append(s.resolving[id], Resolution{Key: key, ThreadID: threadID, Resolved: resolved})
	return key
}

func (s *Store) ResolveApplied(id, key string, res gh.ThreadResult) {
	r, dropped := s.dropResolve(id, key)
	if !dropped {
		return
	}

	held, ok := s.details.look(id)
	if !ok {
		return
	}

	at := threadAt(held.Detail.Threads, r.ThreadID)

	if at < 0 {
		return
	}

	threads := slices.Clone(held.Detail.Threads)
	threads[at].IsResolved = res.IsResolved
	threads[at].CanResolve = res.CanResolve
	threads[at].CanUnresolve = res.CanUnresolve
	held.Detail.Threads = threads

	held.Detail.Reviewers = slices.Clone(held.Detail.Reviewers)
	gh.RecountThreads(&held.Detail)
	s.put(id, held)
}

func (s *Store) ResolveReverted(id, key string) { s.dropResolve(id, key) }

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

func threadAt(threads []gh.ReviewThread, id string) int {
	return slices.IndexFunc(threads, func(t gh.ReviewThread) bool { return t.ID == id })
}

func (s *Store) PendingApplied(id, key string, res gh.CommentResult) {
	p, dropped := s.dropPending(id, key)
	if !dropped {
		return
	}

	held, ok := s.details.look(id)
	if !ok {
		return
	}

	if p.ThreadID != "" {
		s.replyApplied(id, held, p.ThreadID, res.Comment)
		return
	}

	if hasComment(held.Detail.Timeline, res.Comment.ID) {
		return
	}

	held.Detail.Timeline = append(held.Detail.Timeline, timelineComment(res.Comment))
	s.put(id, held)
}

func (s *Store) replyApplied(id string, held Detail, threadID string, c gh.Comment) {
	at := threadAt(held.Detail.Threads, threadID)

	if at < 0 || hasThreadComment(held.Detail.Threads[at].Comments, c.ID) {
		return
	}

	threads := slices.Clone(held.Detail.Threads)
	threads[at].Comments = append(slices.Clone(threads[at].Comments), c)
	held.Detail.Threads = threads
	s.put(id, held)
}

func hasComment(timeline []gh.TimelineItem, id string) bool {
	if id == "" {
		return false
	}
	return slices.ContainsFunc(timeline, func(item gh.TimelineItem) bool {
		return item.Kind == gh.TimelineComment && item.Said().ID == id
	})
}

func hasThreadComment(comments []gh.Comment, id string) bool {
	if id == "" {
		return false
	}
	return slices.ContainsFunc(comments, func(c gh.Comment) bool { return c.ID == id })
}

func (s *Store) PendingReverted(id, key string) { s.dropPending(id, key) }

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

func timelineComment(c gh.Comment) gh.TimelineItem {
	return gh.TimelineItem{
		Kind:      gh.TimelineComment,
		Actor:     c.Author,
		CreatedAt: c.CreatedAt,
		Comment:   &c,
	}
}

// BeginDetail marks a pull request in flight and reports whether it started. It refuses one already in flight.
func (s *Store) BeginDetail(id string) bool {
	held := s.details.get(id)
	if id == "" || held.Status == StatusLoading {
		return false
	}
	delete(s.staleFetch, id)
	s.markPulseStale(id)

	held.Status = StatusLoading
	s.put(id, held)
	return true
}

// DetailApplied stores a pull request and folds the budget, dropping a response asked for before a write settled.
func (s *Store) DetailApplied(id string, res gh.DetailResult) {
	if id == "" {
		return
	}
	s.adopt(res.RateLimit)

	if s.staleFetch[id] {
		held := s.details.get(id)
		held.Status = StatusReady
		s.put(id, held)
		return
	}
	delete(s.staleTimeline, id)
	s.put(id, Detail{Detail: res.Detail, Status: StatusReady, Loaded: true})
	s.syncRow(id)
}

func (s *Store) syncRow(id string) {
	pr := s.Detail(id).Detail.PullRequest
	if pr.ID == "" {
		return
	}
	if s.rowSeq == nil {
		s.rowSeq = make(map[string]int)
	}
	s.rowSeq[pr.ID] = s.nextSeq()

	for i := range s.sections {
		at := slices.IndexFunc(s.sections[i].PRs, func(held gh.PullRequest) bool {
			return held.ID == pr.ID
		})
		if at < 0 {
			continue
		}
		rows := slices.Clone(s.sections[i].PRs)
		rows[at] = pr
		s.sections[i].PRs = rows
	}
}

// StaleDetail reports whether the latest detail fetch was asked for before a write settled.
func (s Store) StaleDetail(id string) bool { return s.staleFetch[id] }

func (s *Store) markStale(id string) {
	s.markPulseStale(id)

	if s.details.get(id).Status != StatusLoading {
		return
	}
	if s.staleFetch == nil {
		s.staleFetch = make(map[string]bool)
	}
	s.staleFetch[id] = true
}

// StaleFiles reports whether the held diff predates a push or a retarget, so another fetch is owed.
func (s Store) StaleFiles(id string) bool { return s.staleFiles[id] }

func (s *Store) markFilesStale(id string) {
	if id == "" {
		return
	}
	if s.staleFiles == nil {
		s.staleFiles = make(map[string]bool)
	}
	s.staleFiles[id] = true
}

// StaleTimeline reports whether the pull request changed in ways a pulse cannot carry, so a full fetch is owed.
func (s Store) StaleTimeline(id string) bool { return s.staleTimeline[id] }

func (s *Store) markTimelineStale(id string) {
	if id == "" {
		return
	}
	s.staleTimeline[id] = true
}

// DetailFailed puts a held pull request into its error state, keeping what it held.
func (s *Store) DetailFailed(id string, err error) {
	held, ok := s.details.look(id)
	if id == "" || !ok {
		return
	}
	delete(s.staleFetch, id)

	held.Status = StatusFailed
	held.Err = err
	s.put(id, held)
}

func (s *Store) put(id string, d Detail) {
	s.details.put(id, d)
	for _, gone := range s.details.evict(id, s.detailPinned) {
		delete(s.staleFetch, gone)
		delete(s.staleTimeline, gone)
		delete(s.stalePulse, gone)
		delete(s.rowSeq, gone)
	}
}

func (s Store) detailPinned(id string) bool {
	if s.details.get(id).Status == StatusLoading || s.pulsing[id] {
		return true
	}
	return len(s.pending[id]) > 0 || len(s.resolving[id]) > 0 || len(s.edits[id]) > 0 ||
		len(s.rewrites[id]) > 0 || len(s.reacting[id]) > 0
}

func diffPinned(held *cache[Files]) func(string) bool {
	return func(key string) bool { return held.get(key).Status == StatusLoading }
}

func (s Store) filesPinned(id string) bool {
	return s.files.get(id).Status == StatusLoading || len(s.viewing[id]) > 0
}

func fileViewedState(viewed bool) gh.FileViewedState {
	if viewed {
		return gh.FileViewed
	}
	return gh.FileUnviewed
}

// Files is the diff held for a pull request with viewed-state writes in flight folded in.
func (s Store) Files(id string) Files {
	held := s.files.get(id)
	if len(s.viewing[id]) == 0 {
		return held
	}
	held.Files = slices.Clone(held.Files)
	for _, write := range s.viewing[id] {
		at := slices.IndexFunc(held.Files, func(f gh.ChangedFile) bool { return f.Path == write.Path })
		if at < 0 {
			continue
		}
		held.Files[at].Viewed = fileViewedState(write.Viewed)
		held.Files[at].Viewing = true
	}
	return held
}

// BeginFiles marks a diff in flight and reports whether it started. Starting clears StaleFiles.
func (s *Store) BeginFiles(id string) bool {
	dropped, ok := beginDiff(&s.files, id, s.filesPinned)
	if !ok {
		return false
	}
	s.dropFileDebts(dropped)
	delete(s.staleFiles, id)
	return true
}

func (s *Store) dropFileDebts(dropped []string) {
	for _, gone := range dropped {
		delete(s.staleFiles, gone)
		delete(s.viewing, gone)
	}
}

func (s *Store) FilesApplied(id string, res gh.FilesResult) {
	s.adopt(res.RateLimit)
	s.dropFileDebts(diffApplied(&s.files, id, res, s.filesPinned))
}

// FilesFailed puts a diff into its error state, keeping what it held.
func (s *Store) FilesFailed(id string, err error) {
	s.dropFileDebts(diffFailed(&s.files, id, err, s.filesPinned))
}

// UseFiles marks a diff read from the cache as recently used.
func (s *Store) UseFiles(id string) { s.files.touch(id) }

func (s Store) CommitFiles(sha string) Files { return s.commits.get(sha) }

func (s *Store) BeginCommitFiles(sha string) bool {
	_, ok := beginDiff(&s.commits, sha, diffPinned(&s.commits))
	return ok
}

func (s *Store) UseCommitFiles(sha string) { s.commits.touch(sha) }

func (s *Store) CommitFilesApplied(sha string, res gh.FilesResult) {
	diffApplied(&s.commits, sha, res, diffPinned(&s.commits))
}

// CommitFilesFailed puts a commit's diff into its error state, keeping what it held.
func (s *Store) CommitFilesFailed(sha string, err error) {
	diffFailed(&s.commits, sha, err, diffPinned(&s.commits))
}

func beginDiff(held *cache[Files], key string, pinned func(string) bool) ([]string, bool) {
	at := held.get(key)
	if key == "" || at.Status == StatusLoading {
		return nil, false
	}
	at.Status = StatusLoading
	return putDiff(held, key, at, pinned), true
}

func diffApplied(held *cache[Files], key string, res gh.FilesResult, pinned func(string) bool) []string {
	if key == "" {
		return nil
	}
	return putDiff(held, key, Files{
		Files:     res.Files,
		MoreFiles: res.MoreFiles,
		Truncated: res.Truncated,
		Status:    StatusReady,
		Loaded:    true,
	}, pinned)
}

func diffFailed(held *cache[Files], key string, err error, pinned func(string) bool) []string {
	at, ok := held.look(key)
	if key == "" || !ok {
		return nil
	}
	at.Status = StatusFailed
	at.Err = err
	return putDiff(held, key, at, pinned)
}

func putDiff(held *cache[Files], key string, f Files, pinned func(string) bool) []string {
	held.put(key, f)
	return held.evict(key, pinned)
}

func (s *Store) PendingFileView(id, path string, viewed bool) string {
	key := s.nextKey()
	if s.viewing == nil {
		s.viewing = make(map[string][]FileView)
	}
	s.viewing[id] = append(s.viewing[id], FileView{Key: key, Path: path, Viewed: viewed})
	return key
}

func (s *Store) FileViewApplied(id, key string) {
	write, ok := s.dropFileView(id, key)
	if !ok {
		return
	}
	defer func() { s.dropFileDebts(s.files.evict(id, s.filesPinned)) }()
	held, ok := s.files.look(id)
	if !ok {
		return
	}
	at := slices.IndexFunc(held.Files, func(f gh.ChangedFile) bool { return f.Path == write.Path })
	if at < 0 {
		return
	}
	held.Files = slices.Clone(held.Files)
	held.Files[at].Viewed = fileViewedState(write.Viewed)
	s.files.put(id, held)
}

func (s *Store) FileViewReverted(id, key string) {
	if _, ok := s.dropFileView(id, key); !ok {
		return
	}
	s.dropFileDebts(s.files.evict("", s.filesPinned))
}

func (s *Store) dropFileView(id, key string) (FileView, bool) {
	writes := s.viewing[id]
	at := slices.IndexFunc(writes, func(w FileView) bool { return w.Key == key })
	if at < 0 {
		return FileView{}, false
	}
	write := writes[at]
	s.viewing[id] = slices.Delete(writes, at, at+1)
	if len(s.viewing[id]) == 0 {
		delete(s.viewing, id)
	}
	return write, true
}

// adopt keeps the lowest remaining within one reset window, because responses arrive out of order.
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
