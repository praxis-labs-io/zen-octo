package comp

import (
	"image/color"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// The badge helpers return a glyph and its color rather than a styled string,
// because the list bakes a selection background into every cell and can only do
// that if it owns the final style.

// State glyphs come from the Nerd Fonts octicon and codicon ranges, the same
// vocabulary gh-dash uses. Shape carries the meaning here: four circles told
// apart by color alone is what the first pass got wrong.
const (
	glyphPROpen   = "" // nf-oct-git_pull_request
	glyphPRDraft  = "" // nf-cod-git_pull_request_draft
	glyphPRMerged = "" // nf-oct-git_merge
	glyphPRClosed = "" // nf-cod-git_pull_request_closed
)

// PRStateIcon is the lifecycle marker: open, draft, merged, or closed.
//
// The state is read before the draft flag, never the other way around. They are
// independent fields and a closed draft carries both, but draft is a stage of
// being open: GitHub keeps the flag set on one it closed, so reading it first
// leaves a closed pull request marked as a draft somebody could still pick up.
// Reopening it gives the draft back, which is where the flag earns its keep.
func PRStateIcon(th theme.Theme, pr gh.PullRequest) (string, color.Color) {
	switch pr.State {
	case gh.PRStateMerged:
		return glyphPRMerged, th.Secondary
	case gh.PRStateClosed:
		return glyphPRClosed, th.Error
	}
	if pr.IsDraft {
		return glyphPRDraft, th.Faint
	}
	if pr.State == gh.PRStateOpen {
		return glyphPROpen, th.Success
	}
	return glyphPROpen, th.Faint
}

// PRStateLabel names the same thing in words, for places with room for them. It
// reads the two fields in the order PRStateIcon gives its reasons for.
func PRStateLabel(th theme.Theme, pr gh.PullRequest) (string, color.Color) {
	switch pr.State {
	case gh.PRStateMerged:
		return "Merged", th.Secondary
	case gh.PRStateClosed:
		return "Closed", th.Error
	}
	if pr.IsDraft {
		return "Draft", th.Faint
	}
	if pr.State == gh.PRStateOpen {
		return "Open", th.Success
	}
	return string(pr.State), th.Faint
}

// CheckStateIcon is the rollup of every check on the head commit. Nothing
// reported reads as a pass: there is no failure either way, and a blank where
// an icon goes reads as a rendering fault rather than as the absence of news.
func CheckStateIcon(th theme.Theme, s gh.CheckState) (string, color.Color) {
	switch s {
	case gh.CheckStateFailure, gh.CheckStateError:
		return "✗", th.Error
	case gh.CheckStatePending, gh.CheckStateExpected:
		return "●", th.Warning
	case gh.CheckStateSkipped:
		return "○", th.Faint
	case gh.CheckStateSuccess, gh.CheckStateNone:
		return "✓", th.Success
	}
	// The rollup state comes off the wire unvalidated, so a state GitHub adds
	// later arrives here. That is not news either way, and a pass is the one
	// reading of it that could be wrong.
	return "●", th.Faint
}

// CheckStateLabel names the rollup. It returns empty when nothing reported, so
// a caller can drop the row rather than print a blank.
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
		return "skipped", th.Faint
	case gh.CheckStateNone:
		return "", th.Faint
	}
	return "", th.Faint
}

// ReviewerColor is where one reviewer stands, for a caller with room for a mark
// but not for the words. Three answers, because a rail row has one cell to say
// them in: green is done with it, red is waiting on a change, muted has not
// weighed in.
//
// An open thread reads as red whatever the verdict was. Someone who left three
// unanswered questions and called it a comment is waiting on the same thing as
// someone who asked for changes.
func ReviewerColor(th theme.Theme, r gh.Reviewer) color.Color {
	switch {
	case r.State == gh.ReviewStateChangesRequested, r.Unresolved > 0:
		return th.Error
	case r.State == gh.ReviewStateApproved:
		return th.Success
	}
	return th.Faint
}

// ReviewStateLabel names one reviewer's verdict, in the past tense the
// conversation reads it in. It is not ReviewLabel: that one summarises the pull
// request, this one is what a person said.
func ReviewStateLabel(th theme.Theme, s gh.ReviewState) (string, color.Color) {
	switch s {
	case gh.ReviewStateApproved:
		return "approved", th.Success
	case gh.ReviewStateChangesRequested:
		return "requested changes", th.Error
	case gh.ReviewStateDismissed:
		return "had a review dismissed", th.Faint
	}
	return "reviewed", th.Secondary
}

// MergeStateLabel names whether the pull request can be merged, and what is in
// the way if it cannot. GitHub reports only the topmost reason, so this says
// one thing rather than listing them.
func MergeStateLabel(th theme.Theme, s gh.MergeState) (string, color.Color) {
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
		return "Checks failing", th.Warning
	case gh.MergeDraft:
		return "Draft", th.Faint
	}
	// GitHub computes mergeability lazily and answers UNKNOWN until it has. It
	// is a wait rather than an answer.
	return "Checking", th.Faint
}

// ReviewColor is where review stands, as a color for a caller drawing its own
// mark. Nothing blocking reads the same as an approval, because it is the same
// news.
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

// ReviewLabel names where review stands. It returns empty when no review is
// required.
func ReviewLabel(th theme.Theme, d gh.ReviewDecision) (string, color.Color) {
	switch d {
	case gh.ReviewDecisionApproved:
		return "approved", th.Success
	case gh.ReviewDecisionChangesRequested:
		return "changes requested", th.Error
	case gh.ReviewDecisionReviewRequired:
		return "review required", th.Warning
	case gh.ReviewDecisionNone:
		return "", th.Faint
	}
	return "", th.Faint
}
