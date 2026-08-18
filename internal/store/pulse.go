package store

import (
	"slices"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

// BeginPulse marks a cheap recheck in flight and reports whether it started. It
// refuses a detail never fetched: a pulse refreshes a page, it never builds one.
func (s *Store) BeginPulse(id string) bool {
	held, ok := s.details.look(id)
	if id == "" || !ok || !held.Loaded {
		return false
	}
	// A full fetch answers everything this would, so a pulse under one asks for
	// nothing and lands after the better answer.
	if held.Status == StatusLoading || s.pulsing[id] {
		return false
	}

	delete(s.stalePulse, id)
	s.pulsing[id] = true
	return true
}

// PulseApplied writes the volatile fields over the held detail, folds the budget,
// and reports whether any of them moved: an unchanged recheck must cost no render.
func (s *Store) PulseApplied(id string, res gh.PulseResult) bool {
	delete(s.pulsing, id)
	s.adopt(res.RateLimit)

	// The mark is left standing, the way DetailApplied leaves staleFetch: it is
	// what tells the caller it owes another. BeginPulse clears it.
	held, ok := s.details.look(id)
	if id == "" || !ok || s.stalePulse[id] {
		return false
	}

	// Somebody pushed, so the held diff is measured against a commit that is no
	// longer the tip. Read before the overwrite puts the new one in.
	if res.Pulse.HeadRefOid != held.Detail.HeadRefOid {
		s.markFilesStale(id)
	}
	// A later instant means something this query cannot carry has changed: a
	// comment, a review, a label. Only a full fetch holds any of it.
	if res.Pulse.UpdatedAt.After(held.Detail.UpdatedAt) {
		s.markTimelineStale(id)
	}
	moved := pulseMoved(held.Detail, res.Pulse)

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
	s.syncRow(id)
	return moved
}

// pulseMoved is whether the answer differs from what is held, over the same
// fields the fold writes. It sits here so a ninth cannot be written uncompared.
func pulseMoved(held gh.PullRequestDetail, p gh.Pulse) bool {
	return held.State != p.State ||
		held.IsDraft != p.IsDraft ||
		held.ReviewDecision != p.ReviewDecision ||
		!held.UpdatedAt.Equal(p.UpdatedAt) ||
		held.HeadRefOid != p.HeadRefOid ||
		held.Merge != p.Merge ||
		held.Checks != p.Rollup.State ||
		rollupMoved(held.Rollup, p.Rollup)
}

// rollupMoved compares the checks behind the summary. gh.Check is three strings,
// so the slice compares by value and a re-run that changed nothing reads as such.
func rollupMoved(held, next gh.CheckRollup) bool {
	return held.State != next.State ||
		held.Passed != next.Passed || held.Failed != next.Failed ||
		held.Pending != next.Pending || held.Skipped != next.Skipped ||
		!slices.Equal(held.Checks, next.Checks)
}

// PulseFailed clears the flight and keeps everything held. It takes no error:
// one painted over content the reader is reading fine is the loudest thing up.
func (s *Store) PulseFailed(id string) {
	delete(s.pulsing, id)
	delete(s.stalePulse, id)
}

// StalePulse reports whether the pulse that just answered was overtaken. A
// caller that started one and finds this true owes another.
func (s Store) StalePulse(id string) bool { return s.stalePulse[id] }

// markPulseStale records that the pulse in flight predates something that knows
// better. Only while one is out, the way markStale reads its own flight.
func (s *Store) markPulseStale(id string) {
	if !s.pulsing[id] {
		return
	}
	s.stalePulse[id] = true
}
