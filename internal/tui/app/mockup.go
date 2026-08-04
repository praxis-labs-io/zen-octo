package app

import (
	"context"
	"time"

	"github.com/zen-octo/zen-octo/internal/gh"
)

// Mock stands in for the GitHub client behind --mockup. It serves fixtures, so
// the production render path runs unchanged over data we control and the layout
// can be judged at a real terminal width without a network or an account.
type Mock struct{}

// SearchPullRequests ignores the query and returns the fixtures.
func (Mock) SearchPullRequests(context.Context, string, int) (gh.SearchResult, error) {
	return gh.SearchResult{
		PullRequests: mockPullRequests(),
		RateLimit:    gh.RateLimit{Limit: 5000, Cost: 1, Remaining: 4821},
	}, nil
}

// mockPullRequests covers the states the row renderer has to tell apart: open,
// draft, merged, closed, every check rollup, a title long enough to truncate,
// and a deleted author.
func mockPullRequests() []gh.PullRequest {
	ago := func(d time.Duration) time.Time { return time.Now().Add(-d) }

	rows := []gh.PullRequest{
		{Title: "Fix the auth retry backoff loop", Repository: "zen-octo/zen-octo", Author: gh.Actor{Login: "drucial"},
			State: gh.PRStateOpen, Checks: gh.CheckStateSuccess, ReviewDecision: gh.ReviewDecisionApproved,
			BaseRefName: "main", HeadRefName: "fix-auth-retry", Additions: 42, Deletions: 7, ChangedFiles: 3, UpdatedAt: ago(2 * time.Hour)},

		{Title: "Bump charm deps to v2.0.8", Repository: "zen-octo/zen-octo", Author: gh.Actor{Login: "drucial"},
			State: gh.PRStateOpen, IsDraft: true, Checks: gh.CheckStatePending,
			BaseRefName: "main", HeadRefName: "bump-charm", Additions: 12, Deletions: 12, ChangedFiles: 2, UpdatedAt: ago(20 * time.Hour)},

		{Title: "Diff viewer: adopt scroll mode's selection model so the two stop disagreeing about anchors",
			Repository: "praxis-labs/zen-term", Author: gh.Actor{Login: "drucial"},
			State: gh.PRStateOpen, Checks: gh.CheckStateFailure, ReviewDecision: gh.ReviewDecisionChangesRequested,
			BaseRefName: "main", HeadRefName: "diff-scroll-selection", Additions: 318, Deletions: 96, ChangedFiles: 14, UpdatedAt: ago(3 * 24 * time.Hour)},

		{Title: "Theme registry and Rosé Pine Moon", Repository: "zen-octo/zen-octo", Author: gh.Actor{Login: "drucial"},
			State: gh.PRStateMerged, Checks: gh.CheckStateSuccess, ReviewDecision: gh.ReviewDecisionApproved,
			BaseRefName: "main", HeadRefName: "theme-registry", Additions: 210, Deletions: 4, ChangedFiles: 6, UpdatedAt: ago(4 * 24 * time.Hour)},

		{Title: "Own the keyboard surface: probe libghostty's binds", Repository: "praxis-labs/zen-term", Author: gh.Actor{},
			State: gh.PRStateClosed, Checks: gh.CheckStateError,
			BaseRefName: "main", HeadRefName: "keyboard-surface", Additions: 88, Deletions: 140, ChangedFiles: 9, UpdatedAt: ago(9 * 24 * time.Hour)},

		{Title: "Persist comments on send", Repository: "praxis-labs/zen-linear", Author: gh.Actor{Login: "drucial"},
			State: gh.PRStateOpen, Checks: gh.CheckStateNone, ReviewDecision: gh.ReviewDecisionReviewRequired,
			BaseRefName: "main", HeadRefName: "persist-comments", Additions: 64, Deletions: 18, ChangedFiles: 4, UpdatedAt: ago(30 * time.Minute)},

		{Title: "Comment anchoring engine", Repository: "praxis-labs/zen-linear", Author: gh.Actor{Login: "drucial"},
			State: gh.PRStateOpen, IsDraft: true, Checks: gh.CheckStateExpected,
			BaseRefName: "main", HeadRefName: "comment-anchors", Additions: 402, Deletions: 31, ChangedFiles: 11, UpdatedAt: ago(400 * 24 * time.Hour)},

		{Title: "Homebrew cask", Repository: "praxis-labs/zen-term", Author: gh.Actor{Login: "drucial"},
			State: gh.PRStateOpen, Checks: gh.CheckStateSuccess,
			BaseRefName: "main", HeadRefName: "homebrew-cask", Additions: 30, Deletions: 0, ChangedFiles: 1, UpdatedAt: ago(45 * time.Second)},
	}

	// Enough rows to overflow any terminal, so scrolling is visible in a mockup
	// run rather than only under test.
	for i := range rows {
		rows[i].ID = "PR_" + rows[i].HeadRefName
		rows[i].Number = 412 - i*7
	}
	repeated := make([]gh.PullRequest, 0, len(rows)*4)
	for round := range 4 {
		for _, pr := range rows {
			pr.ID += "_" + string(rune('a'+round))
			pr.Number -= round * 61
			repeated = append(repeated, pr)
		}
	}
	return repeated
}
