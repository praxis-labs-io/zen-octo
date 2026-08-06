package gh

import "time"

// PRState is where a pull request sits in its lifecycle.
type PRState string

const (
	PRStateOpen   PRState = "OPEN"
	PRStateClosed PRState = "CLOSED"
	PRStateMerged PRState = "MERGED"
)

// CheckState is the rollup of every check on a commit. An empty value means
// no checks reported, which is different from all of them passing.
type CheckState string

const (
	CheckStateNone     CheckState = ""
	CheckStateExpected CheckState = "EXPECTED"
	CheckStateError    CheckState = "ERROR"
	CheckStateFailure  CheckState = "FAILURE"
	CheckStatePending  CheckState = "PENDING"
	CheckStateSuccess  CheckState = "SUCCESS"

	// CheckStateSkipped is a conclusion rather than a rollup state. GitHub
	// never returns it for the whole commit, only for one check inside it.
	CheckStateSkipped CheckState = "SKIPPED"
)

// ReviewDecision is GitHub's summary of where review stands. An empty value
// means no review is required.
type ReviewDecision string

const (
	ReviewDecisionNone             ReviewDecision = ""
	ReviewDecisionApproved         ReviewDecision = "APPROVED"
	ReviewDecisionChangesRequested ReviewDecision = "CHANGES_REQUESTED"
	ReviewDecisionReviewRequired   ReviewDecision = "REVIEW_REQUIRED"
)

// ReviewState is one review's verdict. It is not ReviewDecision: a decision
// summarises the pull request, a state is what one reviewer said.
type ReviewState string

const (
	ReviewStateNone             ReviewState = ""
	ReviewStateCommented        ReviewState = "COMMENTED"
	ReviewStateApproved         ReviewState = "APPROVED"
	ReviewStateChangesRequested ReviewState = "CHANGES_REQUESTED"
	ReviewStateDismissed        ReviewState = "DISMISSED"
	ReviewStatePending          ReviewState = "PENDING"
)

// TimelineKind is what happened. The conversation renders comments and reviews
// in full and everything else as a single line.
type TimelineKind string

const (
	TimelineComment        TimelineKind = "COMMENT"
	TimelineReview         TimelineKind = "REVIEW"
	TimelineCommit         TimelineKind = "COMMIT"
	TimelineMerged         TimelineKind = "MERGED"
	TimelineClosed         TimelineKind = "CLOSED"
	TimelineReopened       TimelineKind = "REOPENED"
	TimelineReadyForReview TimelineKind = "READY_FOR_REVIEW"
	TimelineDraft          TimelineKind = "CONVERT_TO_DRAFT"
	TimelineForcePushed    TimelineKind = "FORCE_PUSHED"
)

// Actor is a user, organization, or bot. Author fields are nil on GitHub when
// the account is deleted, so Login can legitimately be empty.
type Actor struct {
	Login string
}

// Label is one label. Color is GitHub's own six hex digits without a leading
// hash, because a label's color is its identity across every client.
type Label struct {
	Name  string
	Color string
}

// Comment is one piece of writing in the conversation, whether it stands alone
// or sits inside a review thread.
type Comment struct {
	Author    Actor
	CreatedAt time.Time
	Body      string
}

// DiffSide is which half of the diff a line belongs to. A comment on a deleted
// line and one on an added line can carry the same number, so the side is what
// tells them apart.
type DiffSide string

const (
	SideRight DiffSide = "RIGHT"
	SideLeft  DiffSide = "LEFT"
)

// ReviewThread is a line-anchored discussion. ReviewID names the review its
// first comment was submitted with, which is how the conversation puts a thread
// under the review that opened it.
//
// StartLine is zero on a single-line thread. Line is the last line either way,
// which is where GitHub itself hangs the thread.
//
// Hunk is the few lines of diff the thread was written against, nil when GitHub
// returned none. Without it a comment on the conversation reads as an assertion
// about code that is nowhere on the screen.
type ReviewThread struct {
	ReviewID   string
	Path       string
	Line       int
	StartLine  int
	Side       DiffSide
	IsResolved bool
	IsOutdated bool
	Hunk       *Hunk
	Comments   []Comment
}

// TimelineItem is one entry in the conversation. Body is empty on an event,
// Review is set only when Kind is TimelineReview, and Commit only when Kind is
// TimelineCommit.
type TimelineItem struct {
	Kind      TimelineKind
	ID        string
	Actor     Actor
	CreatedAt time.Time
	Body      string
	Review    ReviewState
	Commit    *Commit
}

// Commit is one commit behind the pull request. Author is the GitHub account
// behind it, empty when the commit email is linked to none; AuthorName is what
// git recorded, which is then all there is to name them by.
//
// Checks is the rollup on this commit alone, not the pull request's. Only the
// head commit's is current, and the ones under it are what the branch looked
// like at the time.
type Commit struct {
	SHA         string
	Short       string
	Headline    string
	Author      Actor
	AuthorName  string
	CommittedAt time.Time
	Checks      CheckState
}

// MergeState is whether the pull request can be merged, and what is in the way
// if it cannot. It folds GitHub's mergeable and mergeStateStatus, which answer
// the same question from two directions.
type MergeState string

const (
	MergeUnknown     MergeState = "UNKNOWN"
	MergeClean       MergeState = "CLEAN"
	MergeBlocked     MergeState = "BLOCKED"
	MergeBehind      MergeState = "BEHIND"
	MergeConflicting MergeState = "DIRTY"
	MergeUnstable    MergeState = "UNSTABLE"
	MergeDraft       MergeState = "DRAFT"

	// MergeHasHooks is clean with a pre-receive hook waiting. It never reaches
	// a caller; mergeState folds it into MergeClean.
	MergeHasHooks MergeState = "HAS_HOOKS"
)

// Reviewer is someone GitHub lists on the pull request's reviewers panel:
// either they have reviewed, or a review has been requested of them.
//
// State is empty when the request is still outstanding. It is not a review's
// own state until they submit one.
//
// Unresolved is how many of their review threads are still open. A reviewer who
// only commented is still waiting on something if any of them are.
type Reviewer struct {
	Actor      Actor
	State      ReviewState
	Unresolved int
}

// Check is one entry behind the rollup, whether GitHub calls it a check run or
// a status context.
//
// Name is the job, and Workflow the run it belongs to, empty on a status
// context. Neither is unique on its own: a repository with five suites has five
// jobs called "test".
type Check struct {
	Name     string
	Workflow string
	State    CheckState
}

// CheckRollup is the head commit's checks as a whole. State is GitHub's own
// summary; the list and the counts are this package's, from the contexts
// behind it.
type CheckRollup struct {
	State  CheckState
	Checks []Check

	Passed  int
	Failed  int
	Pending int
	Skipped int
}

// PullRequestDetail embeds the row, so a detail response refreshes the header
// and the rail rather than leaving them on what search returned.
type PullRequestDetail struct {
	PullRequest

	Body      string
	Labels    []Label
	Assignees []Actor
	Reviewers []Reviewer

	Timeline []TimelineItem
	Threads  []ReviewThread
	Commits  []Commit
	Rollup   CheckRollup

	Merge MergeState

	// BehindBy is how many commits the base has that the head does not. Zero is
	// up to date.
	BehindBy int

	// MoreComments, MoreThreads and MoreCommits are what the first page did not
	// reach. A dropped comment that reads as no comment is the failure worth a
	// field.
	MoreComments int
	MoreThreads  int
	MoreCommits  int
}

// FileStatus is what happened to a file in the pull request.
type FileStatus string

const (
	FileAdded     FileStatus = "added"
	FileModified  FileStatus = "modified"
	FileRemoved   FileStatus = "removed"
	FileRenamed   FileStatus = "renamed"
	FileCopied    FileStatus = "copied"
	FileChanged   FileStatus = "changed"
	FileUnchanged FileStatus = "unchanged"
)

// DiffKind is what a line does in the diff.
type DiffKind int

const (
	DiffContext DiffKind = iota
	DiffAdded
	DiffRemoved
)

// DiffLine is one line of a hunk. Old is zero on an added line and New is zero
// on a removed one, which is also how the gutter decides what to print.
type DiffLine struct {
	Kind    DiffKind
	Old     int
	New     int
	Content string
}

// Hunk is one @@ block. Header is GitHub's own, section heading and all, since
// the heading names the function the change sits in.
type Hunk struct {
	Header string
	Lines  []DiffLine
}

// ChangedFile is one file's diff. Omitted says why there are no hunks, empty
// when the hunks are the whole story: GitHub returns no patch for a binary file
// or for a diff it considers too large, and a file that reads as unchanged is
// worse than one that says why.
type ChangedFile struct {
	Path         string
	PreviousPath string
	Status       FileStatus
	Additions    int
	Deletions    int
	Hunks        []Hunk
	Omitted      string
}

// FilesResult is one files response. It carries no rate limit: the REST API
// bills by request against a separate budget the GraphQL one knows nothing
// about.
type FilesResult struct {
	Files []ChangedFile

	// MoreFiles is what the first page did not reach, the same way MoreComments
	// and MoreThreads report their own overflow.
	MoreFiles int

	// Truncated says the page came back full with no total to measure it
	// against. A commit response carries no changed-file count, so this is all
	// there is to say that GitHub is holding more.
	Truncated bool
}

// DetailResult is one detail response: what it returned and what it cost.
type DetailResult struct {
	Detail    PullRequestDetail
	RateLimit RateLimit
}

// RateLimit is the GraphQL point budget as of the last response. GitHub bills
// by query complexity rather than request count, so this is the ceiling worth
// watching.
type RateLimit struct {
	Limit     int
	Cost      int
	Remaining int
	ResetAt   time.Time
}

// SearchResult is one search response: what it matched and what it cost.
type SearchResult struct {
	PullRequests []PullRequest
	RateLimit    RateLimit
}

// PullRequest is the shape the rest of the app sees. It is deliberately not
// the GraphQL response: everything above this package depends on this type.
type PullRequest struct {
	ID          string
	Number      int
	Title       string
	URL         string
	Repository  string // owner/name
	Author      Actor
	State       PRState
	IsDraft     bool
	HeadRefName string
	BaseRefName string

	Additions    int
	Deletions    int
	ChangedFiles int

	// Comments is the conversation plus its review threads, which is what the
	// list means by "how much discussion is on this".
	Comments int

	Checks         CheckState
	ReviewDecision ReviewDecision

	CreatedAt time.Time
	UpdatedAt time.Time
}
