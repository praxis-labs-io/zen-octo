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

// Files is one pull request's diff, keyed by the same id as its Detail. It is
// held apart because it costs a second request the detail screen only makes
// when someone opens the Files tab.
type Files struct {
	Files     []gh.ChangedFile
	MoreFiles int
	Status    Status
	Err       error

	// Loaded marks a diff that has answered at least once, so a refetch can be
	// told from a first open.
	Loaded bool
}

// Store holds every configured section, every detail opened this session, and
// the point budget across all of them.
type Store struct {
	sections []Section
	details  map[string]Detail
	files    map[string]Files
	rate     gh.RateLimit
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
	}
}

// Sections is a snapshot for the view.
func (s Store) Sections() []Section { return slices.Clone(s.sections) }

// Rate is the point budget as of the responses seen so far.
func (s Store) Rate() gh.RateLimit { return s.rate }

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

// Detail is what is held for a pull request. The zero value is one never
// opened, which reads as idle and unloaded.
func (s Store) Detail(id string) Detail { return s.details[id] }

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
func (s *Store) BeginFiles(id string) bool {
	held := s.files[id]
	if id == "" || held.Status == StatusLoading {
		return false
	}
	held.Status = StatusLoading
	s.putFiles(id, held)
	return true
}

// FilesApplied stores a pull request's diff. No budget to fold: the REST API
// bills against a separate allowance the GraphQL response knows nothing about.
func (s *Store) FilesApplied(id string, res gh.FilesResult) {
	if id == "" {
		return
	}
	s.putFiles(id, Files{
		Files:     res.Files,
		MoreFiles: res.MoreFiles,
		Status:    StatusReady,
		Loaded:    true,
	})
}

// FilesFailed puts a diff into its error state, keeping whatever it already
// held. A refetch that fails must not empty a diff that was reading fine.
func (s *Store) FilesFailed(id string, err error) {
	held, ok := s.files[id]
	if id == "" || !ok {
		return
	}
	held.Status = StatusFailed
	held.Err = err
	s.putFiles(id, held)
}

func (s *Store) putFiles(id string, f Files) {
	if s.files == nil {
		s.files = make(map[string]Files)
	}
	s.files[id] = f
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
