package store

import "github.com/zen-octo/zen-octo/internal/gh"

// BeginPulse marks a cheap recheck in flight and reports whether it started. It
// refuses a detail never fetched: a pulse refreshes a page, it never builds one.
func (s *Store) BeginPulse(id string) bool {
	held, ok := s.details[id]
	if id == "" || !ok || !held.Loaded {
		return false
	}
	// A full fetch answers everything this would, so a pulse under one asks for
	// nothing and lands after the better answer.
	if held.Status == StatusLoading || s.pulsing[id] {
		return false
	}

	if s.pulsing == nil {
		s.pulsing = make(map[string]bool)
	}
	delete(s.stalePulse, id)
	s.pulsing[id] = true
	return true
}

// PulseApplied writes the volatile fields over the held detail, field by field
// where DetailApplied replaces the struct, and folds the response into the budget.
func (s *Store) PulseApplied(id string, res gh.PulseResult) {
	delete(s.pulsing, id)
	s.adopt(res.RateLimit)

	held, ok := s.details[id]
	if id == "" || !ok || s.stalePulse[id] {
		delete(s.stalePulse, id)
		return
	}

	// Somebody pushed, so the held diff is measured against a commit that is no
	// longer the tip. Read before the overwrite puts the new one in.
	if res.Pulse.HeadRefOid != held.Detail.HeadRefOid {
		s.markFilesStale(id)
	}

	// Everything else is left alone. The timeline, threads, reviewers and
	// permissions are not on the wire, and a zero over any of them empties a page.
	d := held.Detail
	d.State = res.Pulse.State
	d.IsDraft = res.Pulse.IsDraft
	d.ReviewDecision = res.Pulse.ReviewDecision
	d.UpdatedAt = res.Pulse.UpdatedAt
	d.HeadRefOid = res.Pulse.HeadRefOid
	d.Merge = res.Pulse.Merge
	d.Rollup = res.Pulse.Rollup
	// The row carries the rollup's summary, which is the one field the row build
	// takes from the rollup rather than from the node beside it.
	d.Checks = res.Pulse.Rollup.State

	held.Detail = d
	s.put(id, held)
	s.syncRow(d.PullRequest)
}

// PulseFailed clears the flight and keeps everything held. It takes no error:
// one painted over content the reader is reading fine is the loudest thing up.
func (s *Store) PulseFailed(id string) {
	delete(s.pulsing, id)
	delete(s.stalePulse, id)
}

// markPulseStale records that the pulse in flight predates something that knows
// better. Only while one is out, the way markStale reads its own flight.
func (s *Store) markPulseStale(id string) {
	if !s.pulsing[id] {
		return
	}
	if s.stalePulse == nil {
		s.stalePulse = make(map[string]bool)
	}
	s.stalePulse[id] = true
}
