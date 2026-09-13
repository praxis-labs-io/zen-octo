package store

import (
	"slices"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

// BeginPulse marks a recheck in flight, refusing a detail never loaded, being fully fetched, or already rechecking.
func (s *Store) BeginPulse(id string) bool {
	held, ok := s.details.look(id)
	if id == "" || !ok || !held.Loaded {
		return false
	}
	if held.Status == StatusLoading || s.pulsing[id] {
		return false
	}

	delete(s.stalePulse, id)
	s.pulsing[id] = true
	return true
}

// PulseApplied writes the volatile fields over the held detail, folds the budget, and reports
// whether any of them moved. A moved head marks the diff stale; a later updatedAt marks the timeline stale.
func (s *Store) PulseApplied(id string, res gh.PulseResult) bool {
	delete(s.pulsing, id)
	s.adopt(res.RateLimit)

	held, ok := s.details.look(id)
	if id == "" || !ok || s.stalePulse[id] {
		return false
	}

	if res.Pulse.HeadRefOid != held.Detail.HeadRefOid {
		s.markFilesStale(id)
	}
	if res.Pulse.UpdatedAt.After(held.Detail.UpdatedAt) {
		s.markTimelineStale(id)
	}
	moved := pulseMoved(held.Detail, res.Pulse)

	d := held.Detail
	d.State = res.Pulse.State
	d.IsDraft = res.Pulse.IsDraft
	d.ReviewDecision = res.Pulse.ReviewDecision
	d.UpdatedAt = res.Pulse.UpdatedAt
	d.HeadRefOid = res.Pulse.HeadRefOid
	d.Merge = res.Pulse.Merge
	d.Rollup = res.Pulse.Rollup
	d.Checks = res.Pulse.Rollup.State

	held.Detail = d
	s.put(id, held)
	s.syncRow(id)
	return moved
}

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

func rollupMoved(held, next gh.CheckRollup) bool {
	return held.State != next.State ||
		held.Passed != next.Passed || held.Failed != next.Failed ||
		held.Pending != next.Pending || held.Skipped != next.Skipped ||
		!slices.Equal(held.Checks, next.Checks)
}

// PulseFailed ends the flight and keeps everything held.
func (s *Store) PulseFailed(id string) {
	delete(s.pulsing, id)
	delete(s.stalePulse, id)
}

// StalePulse reports whether the recheck that just answered was overtaken, so another is owed.
func (s Store) StalePulse(id string) bool { return s.stalePulse[id] }

func (s *Store) markPulseStale(id string) {
	if !s.pulsing[id] {
		return
	}
	s.stalePulse[id] = true
}
