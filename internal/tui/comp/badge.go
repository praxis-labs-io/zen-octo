package comp

import (
	"image/color"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

// Nerd Font octicon and codicon glyphs, the vocabulary gh-dash uses, so state reads by shape and not color alone.
const (
	glyphPROpen   = ""
	glyphPRDraft  = ""
	glyphPRMerged = ""
	glyphPRClosed = ""
)

type prStateKind int

const (
	prKindOpen prStateKind = iota
	prKindDraft
	prKindClosed
	prKindMerged

	// A state GitHub added since, deliberately not read as open.
	prKindUnknown
)

// State before the draft flag: GitHub leaves a closed pull request's draft flag set.
func prStateOf(pr gh.PullRequest) prStateKind {
	switch pr.State {
	case gh.PRStateMerged:
		return prKindMerged
	case gh.PRStateClosed:
		return prKindClosed
	}
	if pr.IsDraft {
		return prKindDraft
	}
	if pr.State == gh.PRStateOpen {
		return prKindOpen
	}
	return prKindUnknown
}

func PRStateIcon(th theme.Theme, pr gh.PullRequest) (string, color.Color) {
	switch prStateOf(pr) {
	case prKindMerged:
		return glyphPRMerged, th.Accent
	case prKindClosed:
		return glyphPRClosed, th.Error
	case prKindDraft:
		return glyphPRDraft, th.Subtle
	case prKindOpen:
		return glyphPROpen, th.Success
	}
	return glyphPROpen, th.Subtle
}

func PRStateLabel(th theme.Theme, pr gh.PullRequest) (string, color.Color) {
	switch prStateOf(pr) {
	case prKindMerged:
		return "Merged", th.Accent
	case prKindClosed:
		return "Closed", th.Error
	case prKindDraft:
		return "Draft", th.Subtle
	case prKindOpen:
		return "Open", th.Success
	}
	return string(pr.State), th.Subtle
}

// CheckStateIcon is the glyph for the head commit's check rollup. Nothing reported reads as a pass.
func CheckStateIcon(th theme.Theme, s gh.CheckState) (string, color.Color) {
	switch s {
	case gh.CheckStateFailure, gh.CheckStateError:
		return "✗", th.Error
	case gh.CheckStatePending, gh.CheckStateExpected:
		return "●", th.Warning
	case gh.CheckStateSkipped:
		return "○", th.Subtle
	case gh.CheckStateSuccess, gh.CheckStateNone:
		return "✓", th.Success
	}
	return "●", th.Subtle
}

// CheckStateLabel names the check rollup, or returns empty when nothing reported.
func CheckStateLabel(th theme.Theme, s gh.CheckState) (string, color.Color) {
	switch s {
	case gh.CheckStateSuccess:
		return "passing", th.Success
	case gh.CheckStateFailure:
		return "failing", th.Error
	case gh.CheckStateError:
		return "errored", th.Error
	case gh.CheckStatePending:
		return "running", th.Warning
	case gh.CheckStateExpected:
		return "queued", th.Warning
	case gh.CheckStateSkipped:
		return "skipped", th.Subtle
	case gh.CheckStateNone:
		return "", th.Subtle
	}
	return "", th.Subtle
}

// ReviewerColor is Error while a reviewer blocks (open threads, or changes requested with no threads),
// Warning while their review is requested or their resolved threads await a second look,
// Success on approval, and Subtle otherwise.
func ReviewerColor(th theme.Theme, r gh.Reviewer) color.Color {
	blocked := r.Unresolved > 0 ||
		(r.State == gh.ReviewStateChangesRequested && r.Threads == 0)

	addressed := r.State == gh.ReviewStateChangesRequested &&
		r.Threads > 0 && r.Unresolved == 0

	switch {
	case blocked:
		return th.Error
	case r.Requested, addressed:
		return th.Warning
	case r.State == gh.ReviewStateApproved:
		return th.Success
	}
	return th.Subtle
}

// ReviewStateLabel names one reviewer's verdict; ReviewLabel summarises the pull request.
func ReviewStateLabel(th theme.Theme, s gh.ReviewState) (string, color.Color) {
	switch s {
	case gh.ReviewStateApproved:
		return "approved", th.Success
	case gh.ReviewStateChangesRequested:
		return "requested changes", th.Error
	case gh.ReviewStateDismissed:
		return "had a review dismissed", th.Subtle
	}
	return "reviewed", th.Accent
}

// MergeStateLabel names whether the pull request can merge or GitHub's topmost reason it cannot.
// checks distinguishes the kinds of UNSTABLE.
func MergeStateLabel(th theme.Theme, s gh.MergeState, checks gh.CheckState) (string, color.Color) {
	switch s {
	case gh.MergeClean:
		return "Ready to merge", th.Success
	case gh.MergeBlocked:
		return "Blocked", th.Error
	case gh.MergeConflicting:
		return "Conflicts", th.Error
	case gh.MergeBehind:
		return "Behind the base", th.Warning
	case gh.MergeUnstable:
		switch checks {
		case gh.CheckStatePending:
			return "Checks running", th.Warning
		case gh.CheckStateExpected:
			return "Checks queued", th.Warning
		}
		return "Checks failing", th.Warning
	case gh.MergeDraft:
		return "Draft", th.Subtle
	}
	return "Checking", th.Subtle
}

// ReviewColor is the review decision as a color. No review required reads as approved.
func ReviewColor(th theme.Theme, d gh.ReviewDecision) color.Color {
	switch d {
	case gh.ReviewDecisionChangesRequested:
		return th.Error
	case gh.ReviewDecisionReviewRequired:
		return th.Warning
	case gh.ReviewDecisionApproved, gh.ReviewDecisionNone:
		return th.Success
	}
	return th.Success
}

// ReviewLabel names the review decision, or returns empty when no review is required.
func ReviewLabel(th theme.Theme, d gh.ReviewDecision) (string, color.Color) {
	switch d {
	case gh.ReviewDecisionApproved:
		return "approved", th.Success
	case gh.ReviewDecisionChangesRequested:
		return "changes requested", th.Error
	case gh.ReviewDecisionReviewRequired:
		return "review required", th.Warning
	case gh.ReviewDecisionNone:
		return "", th.Subtle
	}
	return "", th.Subtle
}
