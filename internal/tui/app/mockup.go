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

// mockPullRequests covers what the list has to tell apart: all four states,
// every check rollup, every review decision, three repositories, ages from
// seconds to years, a deleted author, and a title long enough to truncate.
//
// Every row is distinct. Repeating a handful to fill the screen made the groups
// unreadable as a design reference, which is the only thing this data is for.
func mockPullRequests() []gh.PullRequest {
	ago := func(d time.Duration) time.Time { return time.Now().Add(-d) }

	const (
		octo   = "zen-octo/zen-octo"
		term   = "praxis-labs/zen-term"
		linear = "praxis-labs/zen-linear"
	)

	rows := []gh.PullRequest{
		{Title: "Fix the auth retry backoff loop", Repository: octo, Author: gh.Actor{Login: "drucial"},
			State: gh.PRStateOpen, Checks: gh.CheckStateSuccess, ReviewDecision: gh.ReviewDecisionApproved,
			HeadRefName: "fix-auth-retry", Additions: 42, Deletions: 7, ChangedFiles: 3, Comments: 14, UpdatedAt: ago(2 * time.Hour)},

		{Title: "Row grouping: ready, draft, merged, closed", Repository: octo, Author: gh.Actor{Login: "drucial"},
			State: gh.PRStateOpen, Checks: gh.CheckStatePending, ReviewDecision: gh.ReviewDecisionReviewRequired,
			HeadRefName: "row-grouping", Additions: 288, Deletions: 61, ChangedFiles: 7, Comments: 3, UpdatedAt: ago(6 * time.Hour)},

		{Title: "Cache the rate limit between section switches", Repository: octo, Author: gh.Actor{Login: "octobot"},
			State: gh.PRStateOpen, Checks: gh.CheckStateError, ReviewDecision: gh.ReviewDecisionChangesRequested,
			HeadRefName: "cache-rate-limit", Additions: 19, Deletions: 3, ChangedFiles: 2, Comments: 0, UpdatedAt: ago(31 * time.Hour)},

		{Title: "Diff viewer: adopt scroll mode's selection model so the two stop disagreeing about anchors",
			Repository: term, Author: gh.Actor{Login: "drucial"},
			State: gh.PRStateOpen, Checks: gh.CheckStateFailure, ReviewDecision: gh.ReviewDecisionChangesRequested,
			HeadRefName: "diff-scroll-selection", Additions: 318, Deletions: 96, ChangedFiles: 14, Comments: 41, UpdatedAt: ago(3 * 24 * time.Hour)},

		{Title: "Homebrew cask", Repository: term, Author: gh.Actor{Login: "drucial"},
			State: gh.PRStateOpen, Checks: gh.CheckStateSuccess,
			HeadRefName: "homebrew-cask", Additions: 30, ChangedFiles: 1, Comments: 0, UpdatedAt: ago(45 * time.Second)},

		{Title: "Ligature-aware cursor advance", Repository: term, Author: gh.Actor{Login: "nkr"},
			State: gh.PRStateOpen, Checks: gh.CheckStateExpected, ReviewDecision: gh.ReviewDecisionApproved,
			HeadRefName: "ligature-cursor", Additions: 71, Deletions: 24, ChangedFiles: 5, Comments: 7, UpdatedAt: ago(11 * 24 * time.Hour)},

		{Title: "Persist comments on send", Repository: linear, Author: gh.Actor{Login: "drucial"},
			State: gh.PRStateOpen, Checks: gh.CheckStateNone, ReviewDecision: gh.ReviewDecisionReviewRequired,
			HeadRefName: "persist-comments", Additions: 64, Deletions: 18, ChangedFiles: 4, Comments: 22, UpdatedAt: ago(30 * time.Minute)},

		{Title: "Webhook replay on reconnect", Repository: linear, Author: gh.Actor{Login: "octobot"},
			State: gh.PRStateOpen, Checks: gh.CheckStateSuccess, ReviewDecision: gh.ReviewDecisionApproved,
			HeadRefName: "webhook-replay", Additions: 1204, Deletions: 867, ChangedFiles: 39, Comments: 5, UpdatedAt: ago(9 * time.Minute)},

		{Title: "Bump charm deps to v2.0.8", Repository: octo, Author: gh.Actor{Login: "drucial"},
			State: gh.PRStateOpen, IsDraft: true, Checks: gh.CheckStatePending,
			HeadRefName: "bump-charm", Additions: 12, Deletions: 12, ChangedFiles: 2, Comments: 1, UpdatedAt: ago(20 * time.Hour)},

		{Title: "Notifications section type", Repository: octo, Author: gh.Actor{Login: "drucial"},
			State: gh.PRStateOpen, IsDraft: true, Checks: gh.CheckStateNone,
			HeadRefName: "notifications-section", Additions: 5, ChangedFiles: 1, Comments: 0, UpdatedAt: ago(4 * time.Hour)},

		{Title: "Comment anchoring engine", Repository: linear, Author: gh.Actor{Login: "drucial"},
			State: gh.PRStateOpen, IsDraft: true, Checks: gh.CheckStateExpected,
			HeadRefName: "comment-anchors", Additions: 402, Deletions: 31, ChangedFiles: 11, Comments: 63, UpdatedAt: ago(400 * 24 * time.Hour)},

		{Title: "Sixel passthrough behind a flag", Repository: term, Author: gh.Actor{Login: "nkr"},
			State: gh.PRStateOpen, IsDraft: true, Checks: gh.CheckStateFailure,
			HeadRefName: "sixel-passthrough", Additions: 156, Deletions: 8, ChangedFiles: 6, Comments: 9, UpdatedAt: ago(2 * 24 * time.Hour)},

		{Title: "Theme registry and Rosé Pine Moon", Repository: octo, Author: gh.Actor{Login: "drucial"},
			State: gh.PRStateMerged, Checks: gh.CheckStateSuccess, ReviewDecision: gh.ReviewDecisionApproved,
			HeadRefName: "theme-registry", Additions: 210, Deletions: 4, ChangedFiles: 6, Comments: 12, UpdatedAt: ago(4 * 24 * time.Hour)},

		{Title: "Walking skeleton with one live PR section", Repository: octo, Author: gh.Actor{Login: "drucial"},
			State: gh.PRStateMerged, Checks: gh.CheckStateSuccess, ReviewDecision: gh.ReviewDecisionApproved,
			HeadRefName: "walking-skeleton", Additions: 1876, Deletions: 12, ChangedFiles: 24, Comments: 4, UpdatedAt: ago(6 * 24 * time.Hour)},

		{Title: "Drop the vendored ANSI parser", Repository: term, Author: gh.Actor{Login: "nkr"},
			State: gh.PRStateMerged, Checks: gh.CheckStateSuccess,
			HeadRefName: "drop-vendored-ansi", Additions: 8, Deletions: 2140, ChangedFiles: 17, Comments: 2, UpdatedAt: ago(27 * 24 * time.Hour)},

		{Title: "Own the keyboard surface: probe libghostty's binds", Repository: term, Author: gh.Actor{},
			State: gh.PRStateClosed, Checks: gh.CheckStateError,
			HeadRefName: "keyboard-surface", Additions: 88, Deletions: 140, ChangedFiles: 9, Comments: 31, UpdatedAt: ago(9 * 24 * time.Hour)},

		{Title: "Poll the API every second", Repository: linear, Author: gh.Actor{Login: "octobot"},
			State: gh.PRStateClosed, Checks: gh.CheckStateFailure, ReviewDecision: gh.ReviewDecisionChangesRequested,
			HeadRefName: "poll-every-second", Additions: 34, Deletions: 1, ChangedFiles: 1, Comments: 8, UpdatedAt: ago(500 * 24 * time.Hour)},
	}

	for i := range rows {
		rows[i].ID = "PR_" + rows[i].HeadRefName
		rows[i].Number = 412 - i*7
		rows[i].BaseRefName = "main"
	}
	return rows
}
