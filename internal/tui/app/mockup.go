package app

import (
	"context"
	"fmt"
	"hash/fnv"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

// Mock is a fixture-backed GitHub client for --mockup.
type Mock struct{}

const mockViewer = "drucial"

func (Mock) Viewer(context.Context) (gh.ViewerResult, error) {
	return gh.ViewerResult{
		Viewer:    gh.Actor{Login: mockViewer},
		RateLimit: gh.RateLimit{Limit: 5000, Cost: 1, Remaining: 4822},
	}, nil
}

func (Mock) AddComment(_ context.Context, _, body string) (gh.CommentResult, error) {
	return gh.CommentResult{
		Comment: gh.Comment{
			Kind:            gh.CommentIssue,
			ID:              "IC_MOCK",
			Author:          gh.Actor{Login: mockViewer},
			CreatedAt:       time.Now(),
			Body:            body,
			ViewerDidAuthor: true,
			CanEdit:         true,
			CanDelete:       true,
			CanReact:        true,
		},
	}, nil
}

var mockLabels = []gh.Label{
	{ID: "LA_MOCK_1", Name: "bug"},
	{ID: "LA_MOCK_2", Name: "needs-design"},
	{ID: "LA_MOCK_3", Name: "M2"},
	{ID: "LA_MOCK_4", Name: "enhancement"},
	{ID: "LA_MOCK_5", Name: "documentation"},
	{ID: "LA_MOCK_6", Name: "good first issue"},
	{ID: "LA_MOCK_7", Name: "duplicate"},
	{ID: "LA_MOCK_8", Name: "wontfix"},
	{ID: "LA_MOCK_9", Name: "question"},
}

var mockUsers = []gh.Actor{
	{ID: "U_MOCK_1", Login: "drucial"},
	{ID: "U_MOCK_2", Login: "nkr"},
	{ID: "U_MOCK_3", Login: "octobot"},
}

var mockMentions = []gh.Mention{
	{Login: "drucial", Name: "Drew White"},
	{Login: "nkr", Name: "Nikita Rushmanov"},
	{Login: "octobot"},
	{Login: "outsider", Name: "Sam Reed"},
}

func (Mock) RepoMeta(context.Context, string) (gh.RepoMetaResult, error) {
	return gh.RepoMetaResult{Meta: gh.RepoMeta{
		Labels:   mockLabels,
		Users:    mockUsers,
		Mentions: mockMentions,
		Methods:  gh.MergeMethods{Merge: true, Squash: true, Rebase: true},
	}}, nil
}

func (Mock) SetLabels(_ context.Context, _ string, labelIDs []string) (gh.LabelsResult, error) {
	out := make([]gh.Label, 0, len(labelIDs))
	for _, l := range mockLabels {
		if slices.Contains(labelIDs, l.ID) {
			out = append(out, l)
		}
	}
	return gh.LabelsResult{Labels: out}, nil
}

// SetState answers from the fixture's open, non-draft start, so a closed draft comes back not-draft.
func (Mock) SetState(_ context.Context, _ string, to gh.PRTransition) (gh.PRStateResult, error) {
	out := gh.PRStateResult{State: gh.PRStateOpen}
	switch to {
	case gh.TransitionDraft:
		out.IsDraft = true
	case gh.TransitionClose:
		out.State = gh.PRStateClosed
	}
	return out, nil
}

var mockBranches = []string{
	"main", "develop", "release/2.0", "release/1.9", "feature/rail-pickers",
	"feature/base-retarget", "fix/scroll-arithmetic", "spike/glamour-width",
}

func (Mock) Branches(_ context.Context, _, query string) (gh.BranchResult, error) {
	out := make([]string, 0, len(mockBranches))
	for _, b := range mockBranches {
		if strings.Contains(strings.ToLower(b), strings.ToLower(query)) {
			out = append(out, b)
		}
	}
	return gh.BranchResult{Query: query, Default: "main", Branches: out}, nil
}

func (Mock) SetBase(_ context.Context, _, base string) (gh.BaseResult, error) {
	return gh.BaseResult{BaseRefName: base}, nil
}

func (Mock) Merge(_ context.Context, _ string, _ gh.MergeOptions) (gh.MergeResult, error) {
	return gh.MergeResult{State: gh.PRStateMerged}, nil
}

func (Mock) DeleteRef(_ context.Context, _ string) error { return nil }

func (Mock) SetAssignees(_ context.Context, _ string, assigneeIDs []string) (gh.AssigneesResult, error) {
	out := make([]gh.Actor, 0, len(assigneeIDs))
	for _, u := range mockUsers {
		if slices.Contains(assigneeIDs, u.ID) {
			out = append(out, u)
		}
	}
	return gh.AssigneesResult{Assignees: out}, nil
}

func (Mock) RequestReviews(context.Context, string, int, []string) error       { return nil }
func (Mock) RemoveReviewRequests(context.Context, string, int, []string) error { return nil }

func (Mock) SetThreadResolved(_ context.Context, threadID string, resolved bool) (gh.ThreadResult, error) {
	return gh.ThreadResult{
		ID:           threadID,
		IsResolved:   resolved,
		CanResolve:   !resolved,
		CanUnresolve: resolved,
	}, nil
}

func (Mock) SetReaction(_ context.Context, _ string,
	content gh.ReactionContent, on bool,
) (gh.ReactionResult, error) {
	if !on {
		return gh.ReactionResult{}, nil
	}
	return gh.ReactionResult{Reactions: []gh.Reaction{
		{Content: content, Count: 1, Viewer: true},
	}}, nil
}

func (Mock) AddReply(_ context.Context, _, body string) (gh.CommentResult, error) {
	n := mockReplies.Add(1)
	return gh.CommentResult{
		Comment: gh.Comment{
			Kind:            gh.CommentThread,
			ID:              "PRRC_MOCK_" + strconv.FormatInt(n, 10),
			Author:          gh.Actor{Login: mockViewer},
			CreatedAt:       time.Now(),
			Body:            body,
			ViewerDidAuthor: true,
			CanEdit:         true,
			CanDelete:       true,
			CanReact:        true,
		},
	}, nil
}

func (Mock) UpdateComment(_ context.Context, kind gh.CommentKind, id, body string) (gh.CommentResult, error) {
	return gh.CommentResult{
		Comment: gh.Comment{
			Kind:            kind,
			ID:              id,
			Author:          gh.Actor{Login: mockViewer},
			CreatedAt:       time.Now(),
			Body:            body,
			ViewerDidAuthor: true,
			CanEdit:         true,
			CanDelete:       true,
			CanReact:        true,
		},
	}, nil
}

// DeleteComment refuses a review's own body, as the real client does.
func (Mock) DeleteComment(_ context.Context, kind gh.CommentKind, _ string) error {
	if kind == gh.CommentReview {
		return fmt.Errorf("deleting a comment: a %q comment cannot be deleted", kind)
	}
	return nil
}

func (Mock) SetBody(_ context.Context, _, body string) (gh.BodyResult, error) {
	return gh.BodyResult{Body: body}, nil
}

// Atomic because writes leave on command goroutines, and a repeated id breaks the focus ring.
var mockReplies atomic.Int64

func (Mock) SearchPullRequests(_ context.Context, query string, _ int) (gh.SearchResult, error) {
	return gh.SearchResult{
		PullRequests: mockSubset(query),
		RateLimit:    gh.RateLimit{Limit: 5000, Cost: 1, Remaining: 4821},
	}, nil
}

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

func (Mock) Pulse(_ context.Context, _ string) (gh.PulseResult, error) {
	d := mockDetail()
	return gh.PulseResult{
		Pulse: gh.Pulse{
			State:          d.State,
			IsDraft:        d.IsDraft,
			ReviewDecision: d.ReviewDecision,
			Merge:          d.Merge,
			Rollup:         d.Rollup,
			UpdatedAt:      d.UpdatedAt,
			HeadRefOid:     d.HeadRefOid,
		},
		RateLimit: gh.RateLimit{Limit: 5000, Cost: 1, Remaining: 4818},
	}, nil
}

func (Mock) PullRequestFiles(_ context.Context, _, _ string, _, _ int) (gh.FilesResult, error) {
	return gh.FilesResult{Files: mockFiles(), MoreFiles: 2}, nil
}

func (Mock) SetFileViewed(_ context.Context, _, _ string, _ bool) error { return nil }

func (Mock) CommitFiles(_ context.Context, _, _ string) (gh.FilesResult, error) {
	return gh.FilesResult{Files: mockFiles()[:1]}, nil
}

func (Mock) Job(_ context.Context, _ string, id int64) (gh.Job, error) {
	started := time.Date(2026, 8, 19, 14, 0, 0, 0, time.UTC)
	names := map[int64]string{1001: "test", 1002: "test", 1003: "build", 1004: "e2e", 1005: "windows"}
	state := gh.CheckStateSuccess
	switch id {
	case 1003:
		state = gh.CheckStateFailure
	case 1004:
		state = gh.CheckStatePending
	case 1005:
		state = gh.CheckStateSkipped
	}
	completed, duration := started.Add(37*time.Second), 37*time.Second
	if state == gh.CheckStatePending {
		completed, duration = time.Time{}, 0
	}
	return gh.Job{
		ID: id, Name: names[id], State: state, StartedAt: started,
		CompletedAt: completed, Duration: duration,
		Steps: []gh.JobStep{
			{Number: 1, Name: "Set up job", State: gh.CheckStateSuccess, StartedAt: started, CompletedAt: started.Add(4 * time.Second), Duration: 4 * time.Second},
			{Number: 2, Name: "Run job", State: state, StartedAt: started.Add(4 * time.Second), CompletedAt: completed, Duration: max(0, duration-5*time.Second)},
			{Number: 3, Name: "Complete job", State: gh.CheckStateSuccess, StartedAt: started.Add(36 * time.Second), CompletedAt: completed, Duration: time.Second},
		},
	}, nil
}

func (Mock) JobLogs(_ context.Context, _ string, id int64) ([]byte, error) {
	result := "ok"
	if id == 1003 {
		result = "##[error]Process completed with exit code 1"
	}
	return []byte("2026-08-19T14:00:00Z ##[group]Set up job\n2026-08-19T14:00:01Z Runner ready\n2026-08-19T14:00:04Z ##[endgroup]\n2026-08-19T14:00:04Z ##[group]Run job\n2026-08-19T14:00:05Z make all\n2026-08-19T14:00:36Z " + result + "\n2026-08-19T14:00:36Z ##[endgroup]\n"), nil
}

func (Mock) RerunJob(context.Context, string, int64) (time.Time, error) {
	return time.Now(), nil
}

func (Mock) RerunFailedJobs(context.Context, string, int64) error { return nil }
func (Mock) RerunAllJobs(context.Context, string, int64) error    { return nil }

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

func commitItem(c gh.Commit) gh.TimelineItem {
	return gh.TimelineItem{
		Kind:      gh.TimelineCommit,
		Actor:     c.Author,
		CreatedAt: c.CommittedAt,
		Commit:    &c,
	}
}

func ptr[T any](v T) *T { return &v }

func mockDetail() gh.PullRequestDetail {
	ago := func(d time.Duration) time.Time { return time.Now().Add(-d) }

	perms := func(kind gh.CommentKind, id, who string, at time.Time, body string) gh.Comment {
		return gh.Comment{
			Kind: kind, ID: id, Author: gh.Actor{Login: who}, CreatedAt: at, Body: body,
			ViewerDidAuthor: who == mockViewer,
			CanEdit:         true,
			CanDelete:       true,
			CanReact:        true,
		}
	}
	mine := func(kind gh.CommentKind, id string, at time.Time, body string) gh.Comment {
		return perms(kind, id, mockViewer, at, body)
	}
	theirs := func(kind gh.CommentKind, id, who string, at time.Time, body string) gh.Comment {
		return perms(kind, id, who, at, body)
	}

	reacted := func(c gh.Comment, rs ...gh.Reaction) gh.Comment {
		c.Reactions = rs
		return c
	}

	return gh.PullRequestDetail{
		Body: mockBody,
		Reactions: []gh.Reaction{
			{Content: gh.ReactionRocket, Count: 3},
		},

		Labels:    mockLabels[:3:3],
		Assignees: []gh.Actor{mockUsers[0]},
		Reviewers: []gh.Reviewer{
			{Actor: gh.Actor{Login: "nkr"}, State: gh.ReviewStateChangesRequested},
			{Actor: gh.Actor{Login: "copilot-pull-request-reviewer"}, State: gh.ReviewStateCommented},
			{Actor: gh.Actor{Login: "praxis-labs/maintainers"}, Requested: true, Team: true},
		},

		Rollup: gh.CheckRollup{
			State: gh.CheckStateFailure,
			Checks: []gh.Check{
				{Name: "test", Workflow: "Rails Unit Tests", State: gh.CheckStateSuccess, JobID: 1001},
				{Name: "test", Workflow: "Rails Lint", State: gh.CheckStateSuccess, JobID: 1002},
				{Name: "build", Workflow: "Build", State: gh.CheckStateFailure, JobID: 1003},
				{Name: "e2e", Workflow: "E2E Tests", State: gh.CheckStatePending, JobID: 1004},
				{Name: "windows", Workflow: "Build", State: gh.CheckStateSkipped, JobID: 1005},
				{Name: "codecov", State: gh.CheckStateSuccess},
			},
			Passed: 3, Failed: 1, Pending: 1, Skipped: 1,
		},
		Merge:    gh.MergeBlocked,
		BehindBy: 4,

		Viewer: gh.ViewerActions{CanUpdate: true, CanClose: true, CanAssign: true, CanReact: true},

		Commits: mockCommits(),

		Timeline: []gh.TimelineItem{
			{Kind: gh.TimelineComment, Actor: gh.Actor{Login: "octobot"}, CreatedAt: ago(20 * time.Hour),
				Comment: ptr(reacted(theirs(gh.CommentIssue, "IC_1", "octobot", ago(20*time.Hour),
					"Coverage held at 84.2%. No new uncovered branches in `internal/gh`."),
					gh.Reaction{Content: gh.ReactionThumbsUp, Count: 4, Viewer: true},
					gh.Reaction{Content: gh.ReactionEyes, Count: 1}))},

			{Kind: gh.TimelineReview, Actor: gh.Actor{Login: "nkr"},
				CreatedAt: ago(6 * time.Hour), Review: gh.ReviewStateChangesRequested,
				Comment: ptr(theirs(gh.CommentReview, "REV_1", "nkr", ago(6*time.Hour),
					"Close. Two things on the retry path, then this is good to go."))},

			commitItem(mockCommits()[0]),
			commitItem(mockCommits()[1]),
			commitItem(mockCommits()[2]),

			{Kind: gh.TimelineForcePushed, Actor: gh.Actor{Login: "drucial"}, CreatedAt: ago(3 * time.Hour)},

			commitItem(mockCommits()[3]),

			{Kind: gh.TimelineComment, Actor: gh.Actor{}, CreatedAt: ago(2 * time.Hour),
				Comment: ptr(perms(gh.CommentIssue, "IC_2", "", ago(2*time.Hour),
					"Rebased onto main. The ceiling is a constant now."))},

			{Kind: gh.TimelineReview, Actor: gh.Actor{Login: "nkr"},
				CreatedAt: ago(90 * time.Minute), Review: gh.ReviewStateApproved,
				Comment: ptr(theirs(gh.CommentReview, "REV_2", "nkr", ago(90*time.Minute), ""))},

			{Kind: gh.TimelineReadyForReview, Actor: gh.Actor{Login: "drucial"}, CreatedAt: ago(time.Hour)},
		},

		Threads: []gh.ReviewThread{
			{ID: "RT_1", ReviewID: "REV_1", Path: "internal/gh/client.go", Line: 42, Side: gh.SideRight,
				CanReply: true, CanResolve: true,
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
					theirs(gh.CommentThread, "RC_1", "nkr", ago(6*time.Hour),
						"This backs off forever. Needs a ceiling."),
					mine(gh.CommentThread, "RC_2", ago(5*time.Hour),
						"Capped at 30s, matching `fetchTimeout`."),
				}},

			{ID: "RT_2", ReviewID: "REV_1", Path: "internal/gh/search.go", Line: 118, Side: gh.SideRight,
				IsOutdated: true,
				CanReply:   true, CanResolve: true,
				Hunk: &gh.Hunk{
					Header: "@@ -116,3 +116,3 @@ func total(res searchResponse) int {",
					Lines: []gh.DiffLine{
						{Kind: gh.DiffContext, Old: 116, New: 116, Content: "func total(res searchResponse) int {"},
						{Kind: gh.DiffContext, Old: 117, New: 117, Content: "\treturn res.Search.IssueCount + res.Search.More"},
						{Kind: gh.DiffContext, Old: 118, New: 118, Content: "}"},
					},
				},
				Comments: []gh.Comment{
					theirs(gh.CommentThread, "RC_3", "nkr", ago(6*time.Hour),
						"Worth pulling this sum out into a named helper."),
				}},

			{ID: "RT_3", ReviewID: "REV_1", Path: "internal/store/store.go", Line: 88, Side: gh.SideLeft,
				IsResolved: true,
				CanReply:   true, CanUnresolve: true,
				Hunk: &gh.Hunk{
					Header: "@@ -86,4 +86,3 @@ func (s *Store) Begin(i int) bool {",
					Lines: []gh.DiffLine{
						{Kind: gh.DiffContext, Old: 86, New: 86, Content: "// Begin marks one section in flight."},
						{Kind: gh.DiffContext, Old: 87, New: 87, Content: "// It refuses a section that already has"},
						{Kind: gh.DiffRemoved, Old: 88, Content: "// a request out, which is what refuces the"},
					},
				},
				Comments: []gh.Comment{
					theirs(gh.CommentThread, "RC_4", "nkr", ago(6*time.Hour), "Typo: refuces."),
					mine(gh.CommentThread, "RC_5", ago(5*time.Hour), "Fixed."),
					theirs(gh.CommentThread, "RC_6", "nkr", ago(4*time.Hour), "Thanks."),
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

func mockSubset(query string) []gh.PullRequest {
	const least = 4

	rows := mockPullRequests()
	if settled(query) {
		return closedRows(rows)
	}
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

// A plain substring check would also match "Involved", which filters on -author:@me.
func authored(query string) bool {
	return strings.Contains(query, "author:@me") && !strings.Contains(query, "-author:@me")
}

func settled(query string) bool { return strings.Contains(query, "is:closed") }

func closedRows(rows []gh.PullRequest) []gh.PullRequest {
	out := make([]gh.PullRequest, 0, len(rows))
	for _, pr := range rows {
		if pr.State == gh.PRStateMerged || pr.State == gh.PRStateClosed {
			out = append(out, pr)
		}
	}
	return out
}

func mockPullRequests() []gh.PullRequest {
	ago := func(d time.Duration) time.Time { return time.Now().Add(-d) }

	const (
		octo   = "praxis-labs/zen-octo"
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
