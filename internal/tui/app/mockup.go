package app

import (
	"context"
	"hash/fnv"
	"strings"
	"time"

	"github.com/zen-octo/zen-octo/internal/gh"
)

// Mock stands in for the GitHub client behind --mockup. It serves fixtures, so
// the production render path runs unchanged over data we control and the layout
// can be judged at a real terminal width without a network or an account.
type Mock struct{}

// SearchPullRequests answers from the fixtures, keyed by the query so the tabs
// carry counts that differ.
func (Mock) SearchPullRequests(_ context.Context, query string, _ int) (gh.SearchResult, error) {
	return gh.SearchResult{
		PullRequests: mockSubset(query),
		RateLimit:    gh.RateLimit{Limit: 5000, Cost: 1, Remaining: 4821},
	}, nil
}

// PullRequest answers with one conversation whatever is asked for, wrapped
// round the row that was opened. The fixture exists to judge the detail layout,
// and every row leading to the same discussion is what makes it reachable.
func (Mock) PullRequest(_ context.Context, id, _ string) (gh.DetailResult, error) {
	var row gh.PullRequest
	for _, pr := range mockPullRequests() {
		if pr.ID == id {
			row = pr
			break
		}
	}

	detail := mockDetail()
	detail.PullRequest = row
	detail.Checks = detail.Rollup.State

	return gh.DetailResult{
		Detail:    detail,
		RateLimit: gh.RateLimit{Limit: 5000, Cost: 1, Remaining: 4818},
	}, nil
}

// PullRequestFiles answers with one diff whatever is asked for. It covers what
// the Files tab has to tell apart: nesting deep enough to fold, a rename, a
// file with no patch, and lines two of the fixture's review threads anchor to.
func (Mock) PullRequestFiles(_ context.Context, _ string, _, _ int) (gh.FilesResult, error) {
	return gh.FilesResult{Files: mockFiles(), MoreFiles: 2}, nil
}

// CommitFiles answers with the first file of the same diff whatever commit is
// asked for. One file is enough to judge the Commits tab's two panes against
// each other, which is what the fixture is for.
func (Mock) CommitFiles(_ context.Context, _, _ string) (gh.FilesResult, error) {
	return gh.FilesResult{Files: mockFiles()[:1]}, nil
}

func mockFiles() []gh.ChangedFile {
	return []gh.ChangedFile{
		{
			Path: "internal/gh/client.go", Status: gh.FileModified, Additions: 4, Deletions: 1,
			Hunks: []gh.Hunk{{
				Header: "@@ -38,6 +38,9 @@ func New() (*Client, error) {",
				Lines: []gh.DiffLine{
					{Kind: gh.DiffContext, Old: 38, New: 38, Content: "\t\tresp, err := c.do(req)"},
					{Kind: gh.DiffContext, Old: 39, New: 39, Content: "\t\tif err == nil {"},
					{Kind: gh.DiffContext, Old: 40, New: 40, Content: "\t\t\treturn resp, nil"},
					{Kind: gh.DiffRemoved, Old: 41, Content: "\t\ttime.Sleep(delay)"},
					{Kind: gh.DiffAdded, New: 41, Content: "\t\t// A retry that never gives up is a hang with a progress bar."},
					{Kind: gh.DiffAdded, New: 42, Content: "\t\tdelay = min(delay*2, fetchTimeout)"},
					{Kind: gh.DiffAdded, New: 43, Content: "\t\ttime.Sleep(delay)"},
					{Kind: gh.DiffContext, Old: 42, New: 44, Content: "\t}"},
				},
			}},
		},
		{
			Path: "internal/gh/search.go", Status: gh.FileModified, Additions: 1, Deletions: 1,
			Hunks: []gh.Hunk{{
				Header: "@@ -116,3 +116,3 @@ func total(res searchResponse) int {",
				Lines: []gh.DiffLine{
					{Kind: gh.DiffContext, Old: 116, New: 116, Content: "func total(res searchResponse) int {"},
					{Kind: gh.DiffRemoved, Old: 117, Content: "\treturn res.Search.IssueCount + res.Search.More"},
					{Kind: gh.DiffAdded, New: 117, Content: "\treturn res.Search.IssueCount"},
					{Kind: gh.DiffContext, Old: 118, New: 118, Content: "}"},
				},
			}},
		},
		{
			Path: "internal/store/store.go", Status: gh.FileModified, Additions: 0, Deletions: 1,
			Hunks: []gh.Hunk{{
				Header: "@@ -86,4 +86,3 @@ func (s *Store) Begin(i int) bool {",
				Lines: []gh.DiffLine{
					{Kind: gh.DiffContext, Old: 86, New: 86, Content: "// Begin marks one section in flight."},
					{Kind: gh.DiffContext, Old: 87, New: 87, Content: "// It refuses a section that already has"},
					{Kind: gh.DiffRemoved, Old: 88, Content: "// a request out, which is what refuces the"},
					{Kind: gh.DiffContext, Old: 89, New: 88, Content: "// duplicate."},
				},
			}},
		},
		{
			Path: "internal/tui/prview/files.go", PreviousPath: "internal/tui/prview/diff.go",
			Status: gh.FileRenamed, Additions: 2, Deletions: 0,
			Hunks: []gh.Hunk{{
				Header: "@@ -1,3 +1,5 @@",
				Lines: []gh.DiffLine{
					{Kind: gh.DiffContext, Old: 1, New: 1, Content: "package prview"},
					{Kind: gh.DiffContext, Old: 2, New: 2, Content: ""},
					{Kind: gh.DiffAdded, New: 3, Content: "// tabWidth is what a tab expands to."},
					{Kind: gh.DiffAdded, New: 4, Content: "const tabWidth = 4"},
				},
			}},
		},
		{
			Path: "docs/screenshot.png", Status: gh.FileModified, Additions: 0, Deletions: 0,
			Omitted: "binary, or too large for GitHub to return a diff",
		},
	}
}

// mockDetail covers what the conversation has to tell apart: a description with
// every markdown element the renderer styles, comments, both review verdicts, a
// resolved thread beside two open ones, and a rollup that is neither all green
// nor all red.
// mockCommits is the branch behind the fixture, oldest first. It covers what
// the column has to tell apart: the three check states, and an author GitHub
// has no account for.
func mockCommits() []gh.Commit {
	ago := func(d time.Duration) time.Time { return time.Now().Add(-d) }

	return []gh.Commit{
		{SHA: "a3f91c2d5e8b4770c1e2f6a9d3045bb812e7c440", Short: "a3f91c2",
			Headline: "Cap the retry backoff at the fetch timeout",
			Body: "The loop doubled the delay with no ceiling, so a dead endpoint\n" +
				"backed off past the point anything was waiting for it.",
			Author: gh.Actor{Login: "drucial"}, CommittedAt: ago(19 * time.Hour), Checks: gh.CheckStateSuccess},
		{SHA: "7b20ef4a11", Short: "7b20ef4", Headline: "Drop the phantom count from the search total",
			Author: gh.Actor{Login: "drucial"}, CommittedAt: ago(18 * time.Hour), Checks: gh.CheckStateSuccess},
		{SHA: "c1d8a04bb9", Short: "c1d8a04", Headline: "Fix the typo in the Begin comment",
			AuthorName: "Drew White", CommittedAt: ago(17 * time.Hour), Checks: gh.CheckStatePending},
		{SHA: "9e4c77f320", Short: "9e4c77f", Headline: "Rebase onto main and hold the ceiling as a constant",
			Author: gh.Actor{Login: "drucial"}, CommittedAt: ago(150 * time.Minute), Checks: gh.CheckStateFailure},
	}
}

// commitItem is how a commit reads on the timeline.
func commitItem(c gh.Commit) gh.TimelineItem {
	return gh.TimelineItem{
		Kind:      gh.TimelineCommit,
		Actor:     c.Author,
		CreatedAt: c.CommittedAt,
		Commit:    &c,
	}
}

func mockDetail() gh.PullRequestDetail {
	ago := func(d time.Duration) time.Time { return time.Now().Add(-d) }

	return gh.PullRequestDetail{
		Body: mockBody,

		Labels: []gh.Label{
			{Name: "bug", Color: "d73a4a"},
			{Name: "needs-design", Color: "c5def5"},
			{Name: "M2", Color: "0e8a16"},
		},
		Assignees: []gh.Actor{{Login: "drucial"}},
		Reviewers: []gh.Reviewer{
			{Actor: gh.Actor{Login: "nkr"}, State: gh.ReviewStateChangesRequested},
			{Actor: gh.Actor{Login: "copilot-pull-request-reviewer"}, State: gh.ReviewStateCommented},
			{Actor: gh.Actor{Login: "zen-octo/maintainers"}},
		},

		Rollup: gh.CheckRollup{
			State: gh.CheckStateFailure,
			Checks: []gh.Check{
				{Name: "test", Workflow: "Rails Unit Tests", State: gh.CheckStateSuccess},
				{Name: "test", Workflow: "Rails Lint", State: gh.CheckStateSuccess},
				{Name: "build", Workflow: "Build", State: gh.CheckStateFailure},
				{Name: "e2e", Workflow: "E2E Tests", State: gh.CheckStatePending},
				{Name: "windows", Workflow: "Build", State: gh.CheckStateSkipped},
				{Name: "codecov", State: gh.CheckStateSuccess},
			},
			Passed: 3, Failed: 1, Pending: 1, Skipped: 1,
		},
		Merge:    gh.MergeBlocked,
		BehindBy: 4,

		Commits: mockCommits(),

		Timeline: []gh.TimelineItem{
			{Kind: gh.TimelineComment, Actor: gh.Actor{Login: "octobot"}, CreatedAt: ago(20 * time.Hour),
				Body: "Coverage held at 84.2%. No new uncovered branches in `internal/gh`."},

			{Kind: gh.TimelineReview, ID: "REV_1", Actor: gh.Actor{Login: "nkr"},
				CreatedAt: ago(6 * time.Hour), Review: gh.ReviewStateChangesRequested,
				Body: "Close. Two things on the retry path, then this is good to go."},

			// A run of three and a lone one, so the fold and the single line
			// both show at a glance.
			commitItem(mockCommits()[0]),
			commitItem(mockCommits()[1]),
			commitItem(mockCommits()[2]),

			{Kind: gh.TimelineForcePushed, Actor: gh.Actor{Login: "drucial"}, CreatedAt: ago(3 * time.Hour)},

			commitItem(mockCommits()[3]),

			{Kind: gh.TimelineComment, Actor: gh.Actor{}, CreatedAt: ago(2 * time.Hour),
				Body: "Rebased onto main. The ceiling is a constant now."},

			{Kind: gh.TimelineReview, ID: "REV_2", Actor: gh.Actor{Login: "nkr"},
				CreatedAt: ago(90 * time.Minute), Review: gh.ReviewStateApproved},

			{Kind: gh.TimelineReadyForReview, Actor: gh.Actor{Login: "drucial"}, CreatedAt: ago(time.Hour)},
		},

		Threads: []gh.ReviewThread{
			{ReviewID: "REV_1", Path: "internal/gh/client.go", Line: 42, Side: gh.SideRight,
				Hunk: &gh.Hunk{
					Header: "@@ -38,4 +38,5 @@ func New() (*Client, error) {",
					Lines: []gh.DiffLine{
						{Kind: gh.DiffContext, Old: 40, New: 40, Content: "\t\t\treturn resp, nil"},
						{Kind: gh.DiffRemoved, Old: 41, Content: "\t\ttime.Sleep(delay)"},
						{Kind: gh.DiffAdded, New: 41, Content: "\t\t// A retry that never gives up is a hang with a progress bar."},
						{Kind: gh.DiffAdded, New: 42, Content: "\t\tdelay = min(delay*2, fetchTimeout)"},
					},
				},
				Comments: []gh.Comment{
					{Author: gh.Actor{Login: "nkr"}, CreatedAt: ago(6 * time.Hour),
						Body: "This backs off forever. Needs a ceiling."},
					{Author: gh.Actor{Login: "drucial"}, CreatedAt: ago(5 * time.Hour),
						Body: "Capped at 30s, matching `fetchTimeout`."},
				}},

			{ReviewID: "REV_1", Path: "internal/gh/search.go", Line: 118, Side: gh.SideRight,
				IsOutdated: true,
				Comments: []gh.Comment{
					{Author: gh.Actor{Login: "nkr"}, CreatedAt: ago(6 * time.Hour),
						Body: "Worth pulling this sum out into a named helper."},
				}},

			{ReviewID: "REV_1", Path: "internal/store/store.go", Line: 88, Side: gh.SideLeft,
				IsResolved: true,
				Comments: []gh.Comment{
					{Author: gh.Actor{Login: "nkr"}, CreatedAt: ago(6 * time.Hour), Body: "Typo: refuces."},
					{Author: gh.Actor{Login: "drucial"}, CreatedAt: ago(5 * time.Hour), Body: "Fixed."},
					{Author: gh.Actor{Login: "nkr"}, CreatedAt: ago(4 * time.Hour), Body: "Thanks."},
				}},
		},

		MoreComments: 12,
	}
}

const mockBody = `Retries currently back off without a ceiling, so a GitHub
outage leaves the client sleeping for minutes at a time with no way out but
quitting. This caps the backoff and reports what it is waiting on.

## What changed

- ` + "`retryDelay`" + ` caps at 30s, matching ` + "`fetchTimeout`" + `
- The sleep is interruptible, so **ctrl+c** still works mid-wait
- Failures name the attempt they are on

` + "```go" + `
func backoff(attempt int) time.Duration {
	return min(base<<attempt, ceiling)
}
` + "```" + `

> The ceiling is the timeout on purpose. A retry that outlives the request it
> is retrying is not a retry.

Closes [ZNO-9](https://linear.app/praxis-labs/issue/ZNO-9).
`

// mockSubset is the fixtures a section gets. The one asking for the user's own
// pull requests gets all of them: it is the default first tab and the one the
// layout is judged on. The rest take a rotated slice, which still spans states
// because the list buckets by state before it renders.
func mockSubset(query string) []gh.PullRequest {
	const least = 4

	rows := mockPullRequests()
	// The floor is also what keeps the modulus below from dividing by zero if
	// the fixtures are ever cut back.
	if authored(query) || len(rows) <= least {
		return rows
	}

	h := fnv.New32a()
	_, _ = h.Write([]byte(query))
	sum := h.Sum32()

	start := int(sum % uint32(len(rows)))
	n := least + int(sum%uint32(len(rows)-least))

	out := make([]gh.PullRequest, 0, n)
	for i := range n {
		out = append(out, rows[(start+i)%len(rows)])
	}
	return out
}

// authored reports the section asking for the user's own pull requests. The
// exclusion is the point: "Involved" filters on -author:@me, and a plain
// substring check hands it the full set too.
func authored(query string) bool {
	return strings.Contains(query, "author:@me") && !strings.Contains(query, "-author:@me")
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
