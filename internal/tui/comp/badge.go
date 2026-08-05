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
func PRStateIcon(th theme.Theme, pr gh.PullRequest) (string, color.Color) {
	if pr.IsDraft {
		return glyphPRDraft, th.Faint
	}
	switch pr.State {
	case gh.PRStateMerged:
		return glyphPRMerged, th.Secondary
	case gh.PRStateClosed:
		return glyphPRClosed, th.Error
	case gh.PRStateOpen:
		return glyphPROpen, th.Success
	}
	return glyphPROpen, th.Faint
}

// PRStateLabel names the same thing in words, for places with room for them.
func PRStateLabel(th theme.Theme, pr gh.PullRequest) (string, color.Color) {
	if pr.IsDraft {
		return "Draft", th.Faint
	}
	switch pr.State {
	case gh.PRStateMerged:
		return "Merged", th.Secondary
	case gh.PRStateClosed:
		return "Closed", th.Error
	case gh.PRStateOpen:
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
	case gh.CheckStateSuccess, gh.CheckStateNone:
		return "✓", th.Success
	}
	return "✓", th.Success
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
	case gh.CheckStateNone:
		return "", th.Faint
	}
	return "", th.Faint
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
