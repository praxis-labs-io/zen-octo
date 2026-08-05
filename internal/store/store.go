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
}

// Store holds every configured section and the point budget across them.
type Store struct {
	sections []Section
	rate     gh.RateLimit
}

// New builds a store over the configured sections, none of them fetched.
func New(sections []config.Section) Store {
	held := make([]Section, len(sections))
	for i, s := range sections {
		held[i] = Section{Section: s}
	}
	return Store{sections: held}
}

// Len is the number of sections.
func (s Store) Len() int { return len(s.sections) }

// Sections is a snapshot for the view.
func (s Store) Sections() []Section { return slices.Clone(s.sections) }

// Rate is the point budget as of the responses seen so far.
func (s Store) Rate() gh.RateLimit { return s.rate }

// Loading reports whether any section has a fetch in flight.
func (s Store) Loading() bool {
	return slices.ContainsFunc(s.sections, func(sec Section) bool {
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

// adopt keeps the budget falling through a burst. Sections answer in whatever
// order they finish, so the newest arrival is not the truest one: the lowest
// remaining inside a window is. A later reset means a new window, and there the
// number legitimately goes back up.
func (s *Store) adopt(r gh.RateLimit) {
	if r.Limit == 0 {
		return
	}
	if s.rate.Limit == 0 || r.ResetAt.After(s.rate.ResetAt) || r.Remaining < s.rate.Remaining {
		s.rate = r
	}
}
