package gh

import (
	"strconv"
	"time"
)

type PRState string

const (
	PRStateOpen   PRState = "OPEN"
	PRStateClosed PRState = "CLOSED"
	PRStateMerged PRState = "MERGED"
)

// PRTransition is a move rather than a destination: draft and closed are independent fields, and each
// move is its own mutation.
type PRTransition string

const (
	TransitionReady  PRTransition = "READY"
	TransitionDraft  PRTransition = "DRAFT"
	TransitionClose  PRTransition = "CLOSE"
	TransitionReopen PRTransition = "REOPEN"
)

// CheckState is the rollup of a commit's checks. Empty means none reported, not all passing.
type CheckState string

const (
	CheckStateNone     CheckState = ""
	CheckStateExpected CheckState = "EXPECTED"
	CheckStateError    CheckState = "ERROR"
	CheckStateFailure  CheckState = "FAILURE"
	CheckStatePending  CheckState = "PENDING"
	CheckStateSuccess  CheckState = "SUCCESS"

	// CheckStateSkipped is a per-check conclusion; GitHub never returns it for a whole commit.
	CheckStateSkipped CheckState = "SKIPPED"
)

// ReviewDecision summarises review on a pull request. Empty means no review is required.
type ReviewDecision string

const (
	ReviewDecisionNone             ReviewDecision = ""
	ReviewDecisionApproved         ReviewDecision = "APPROVED"
	ReviewDecisionChangesRequested ReviewDecision = "CHANGES_REQUESTED"
	ReviewDecisionReviewRequired   ReviewDecision = "REVIEW_REQUIRED"
)

// ReviewState is one reviewer's verdict.
type ReviewState string

const (
	ReviewStateNone             ReviewState = ""
	ReviewStateCommented        ReviewState = "COMMENTED"
	ReviewStateApproved         ReviewState = "APPROVED"
	ReviewStateChangesRequested ReviewState = "CHANGES_REQUESTED"
	ReviewStateDismissed        ReviewState = "DISMISSED"
	ReviewStatePending          ReviewState = "PENDING"
)

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

	TimelineLabeled         TimelineKind = "LABELED"
	TimelineUnlabeled       TimelineKind = "UNLABELED"
	TimelineAssigned        TimelineKind = "ASSIGNED"
	TimelineUnassigned      TimelineKind = "UNASSIGNED"
	TimelineReviewRequested TimelineKind = "REVIEW_REQUESTED"
	TimelineReviewCancelled TimelineKind = "REVIEW_REQUEST_REMOVED"
	TimelineBaseChanged     TimelineKind = "BASE_REF_CHANGED"
)

// Actor is a user, organization, or bot. Login is empty for a deleted account, and ID is set
// only on the lists a picker writes back, because updatePullRequest takes node ids.
type Actor struct {
	ID    string
	Login string
}

// Mention is someone nameable in a comment. Name is empty where the account set none. No node id, so
// it is not an Actor.
type Mention struct {
	Login string
	Name  string
}

// Label omits GitHub's color on purpose: a hex chosen against a white page vanishes on a dark terminal.
type Label struct {
	ID   string
	Name string
}

// CommentKind picks the mutation: GitHub edits each kind through a different call.
type CommentKind string

const (
	// CommentIssue stands alone in the conversation; GitHub calls it an issue comment on a pull request too.
	CommentIssue CommentKind = "ISSUE"

	// CommentReview is a review's own body, above its threads.
	CommentReview CommentKind = "REVIEW"

	CommentThread CommentKind = "THREAD"
)

// ReactionContent values are GitHub's enum words, sent to the mutation as they are.
type ReactionContent string

const (
	ReactionThumbsUp   ReactionContent = "THUMBS_UP"
	ReactionThumbsDown ReactionContent = "THUMBS_DOWN"
	ReactionLaugh      ReactionContent = "LAUGH"
	ReactionHooray     ReactionContent = "HOORAY"
	ReactionConfused   ReactionContent = "CONFUSED"
	ReactionHeart      ReactionContent = "HEART"
	ReactionRocket     ReactionContent = "ROCKET"
	ReactionEyes       ReactionContent = "EYES"
)

// ReactionOrder is GitHub's own order, the one its page offers.
var ReactionOrder = []ReactionContent{
	ReactionThumbsUp,
	ReactionThumbsDown,
	ReactionLaugh,
	ReactionHooray,
	ReactionConfused,
	ReactionHeart,
	ReactionRocket,
	ReactionEyes,
}

// Reaction is one kind of reaction on a subject. Viewer is whether the token's account gave it.
// GitHub answers with all eight groups on every subject; this package drops the ones nobody gave.
type Reaction struct {
	Content ReactionContent
	Count   int
	Viewer  bool

	// Pending is set by the store on an optimistic copy, never by this package.
	Pending bool
}

// Comment is a standalone comment, a review body, or a thread comment. ViewerDidAuthor is not CanEdit:
// a maintainer can edit anyone's.
type Comment struct {
	Kind      CommentKind
	ID        string
	Author    Actor
	CreatedAt time.Time
	Body      string

	ViewerDidAuthor bool
	CanEdit         bool
	CanDelete       bool
	CanReact        bool

	Reactions []Reaction

	// Pending is a comment GitHub has not acknowledged yet. Set by the store, never by this package.
	Pending bool

	// Editing is a comment on GitHub showing a rewrite it has not confirmed. Set by the store, never
	// by this package.
	Editing bool
}

// DiffSide tells apart a deleted and an added line carrying the same number.
type DiffSide string

const (
	SideRight DiffSide = "RIGHT"
	SideLeft  DiffSide = "LEFT"
)

// ReviewThread is a line-anchored discussion. ReviewID is the review its first comment was submitted with.
// StartLine is zero on a single-line thread, and Hunk is nil when GitHub returned none.
// CanResolve and CanUnresolve are separate: a viewer may close a thread and not reopen it.
type ReviewThread struct {
	ID         string
	ReviewID   string
	Path       string
	Line       int
	StartLine  int
	Side       DiffSide
	IsResolved bool
	IsOutdated bool

	CanReply     bool
	CanResolve   bool
	CanUnresolve bool

	// Pending is set by the store while a resolve is out, never by this package.
	Pending bool

	Hunk     *Hunk
	Comments []Comment
}

// TimelineItem is one conversation entry. Comment is nil on an event; Review is set only on
// TimelineReview, Commit only on TimelineCommit.
type TimelineItem struct {
	Kind      TimelineKind
	Actor     Actor
	CreatedAt time.Time
	Comment   *Comment
	Review    ReviewState
	Commit    *Commit

	// Subject is the label, handle or branch an event acted on, and empty on kinds acting on the whole
	// pull request.
	Subject string

	// Was is the value Subject replaced. Only TimelineBaseChanged has one.
	Was string
}

// Said returns the comment, or the zero Comment on an event.
func (i TimelineItem) Said() Comment {
	if i.Comment == nil {
		return Comment{}
	}
	return *i.Comment
}

// Commit is one commit on the pull request. Author is empty when the commit email links to no account;
// AuthorName is what git recorded. Checks is this commit's own rollup, current only on the head
// commit.
type Commit struct {
	SHA         string
	Short       string
	Headline    string
	Body        string
	Author      Actor
	AuthorName  string
	CommittedAt time.Time
	Checks      CheckState
}

// MergeState folds GitHub's mergeable and mergeStateStatus into one answer.
type MergeState string

const (
	MergeUnknown     MergeState = "UNKNOWN"
	MergeClean       MergeState = "CLEAN"
	MergeBlocked     MergeState = "BLOCKED"
	MergeBehind      MergeState = "BEHIND"
	MergeConflicting MergeState = "DIRTY"
	MergeUnstable    MergeState = "UNSTABLE"
	MergeDraft       MergeState = "DRAFT"

	// MergeHasHooks never reaches a caller; it folds into MergeClean.
	MergeHasHooks MergeState = "HAS_HOOKS"
)

type MergeMethod string

const (
	MergeMethodMerge  MergeMethod = "MERGE"
	MergeMethodSquash MergeMethod = "SQUASH"
	MergeMethodRebase MergeMethod = "REBASE"
)

// MergeMessage is the commit message GitHub would write for one method, empty for a rebase.
// Fetched rather than built, because repository settings decide the squash title and body.
type MergeMessage struct {
	Headline string
	Body     string
}

// Reviewer is someone on the reviewers panel. State is empty until they submit, and Requested can
// hold beside a State. Threads counts the review threads they opened, Unresolved the open ones.
// Team marks a team request, whose Login is a synthetic "org/slug" no write accepts.
type Reviewer struct {
	Actor      Actor
	State      ReviewState
	Unresolved int
	Threads    int
	Requested  bool
	Team       bool
}

// Check is one entry behind the rollup. JobID and RunID are zero on a status context.
type Check struct {
	Name     string
	Workflow string
	State    CheckState

	JobID       int64
	RunID       int64
	DistinctID  int64
	StartedAt   time.Time
	CompletedAt time.Time
	Duration    time.Duration
	DetailsURL  string
}

// LogicalKey is stable across a rerun, which changes the job id but keeps the run id.
func (c Check) LogicalKey() string {
	return strconv.FormatInt(c.RunID, 10) + "\x00" + c.Workflow + "\x00" + c.Name
}

// Key is unique within one rollup. It equals LogicalKey unless GitHub returned two checks with the
// same identity.
func (c Check) Key() string {
	key := c.LogicalKey()
	if c.DistinctID != 0 {
		key += "\x00" + strconv.FormatInt(c.DistinctID, 10)
	}
	return key
}

// Job is one Actions job. Step state and timing come from the job endpoint only, not the rollup.
type Job struct {
	ID          int64
	Name        string
	State       CheckState
	StartedAt   time.Time
	CompletedAt time.Time
	Duration    time.Duration
	Steps       []JobStep
}

// JobStep is one section of a job log. Number is GitHub's order within the job.
type JobStep struct {
	Number      int
	Name        string
	State       CheckState
	StartedAt   time.Time
	CompletedAt time.Time
	Duration    time.Duration
}

// CheckRollup is the head commit's checks. State is GitHub's summary; Checks and the counts are derived here.
type CheckRollup struct {
	State  CheckState
	Checks []Check

	Passed  int
	Failed  int
	Pending int
	Skipped int
}

// ViewerActions is what GitHub says the viewer may do. CanUpdate also gates the draft toggle, which
// has no field of its own. GitHub has no flag for review requests or a plain merge, and
// viewerCanDeleteHeadRef is false on every open pull request.
type ViewerActions struct {
	CanUpdate bool
	CanClose  bool
	CanReopen bool
	CanAssign bool

	CanMergeAsAdmin bool

	// CanReact covers the description alone; each comment carries its own.
	CanReact bool
}

type PullRequestDetail struct {
	PullRequest

	Body      string
	Labels    []Label
	Assignees []Actor
	Reviewers []Reviewer

	// Reactions are the description's.
	Reactions []Reaction

	Timeline []TimelineItem
	Threads  []ReviewThread
	Commits  []Commit
	Rollup   CheckRollup

	Merge MergeState

	// HeadRefOid is the head tip when fetched, which a merge sends as the commit it means. HeadRefID
	// is empty once the branch is gone.
	HeadRefOid string
	HeadRefID  string

	// CrossRepository is a head branch living in a fork.
	CrossRepository bool

	MergeCommit  MergeMessage
	SquashCommit MergeMessage

	Viewer ViewerActions

	// BehindBy is how many commits the base has that the head lacks, or BehindUnknown or BehindNoHead.
	BehindBy int

	// MoreComments, MoreThreads, MoreCommits and MoreEvents count what the first page did not reach.
	// MoreEvents counts only the event types asked for, never the whole timeline.
	MoreComments int
	MoreThreads  int
	MoreCommits  int
	MoreEvents   int
}

// MergeMessage is what GitHub would commit for m, and the zero value for a rebase.
func (d PullRequestDetail) MergeMessage(m MergeMethod) MergeMessage {
	switch m {
	case MergeMethodMerge:
		return d.MergeCommit
	case MergeMethodSquash:
		return d.SquashCommit
	}
	return MergeMessage{}
}

// BehindUnknown is BehindBy after an optimistic retarget, before GitHub has counted.
const BehindUnknown = -1

// BehindNoHead is BehindBy with no head branch left to compare, as on a merged pull request.
const BehindNoHead = -2

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

type DiffKind int

const (
	DiffContext DiffKind = iota
	DiffAdded
	DiffRemoved
)

// DiffLine is one line of a hunk. Old is zero on an added line and New is zero on a removed one.
type DiffLine struct {
	Kind    DiffKind
	Old     int
	New     int
	Content string
}

// Hunk is one @@ block. Header is GitHub's own line, section heading included.
type Hunk struct {
	Header string
	Lines  []DiffLine
}

// FileViewedState is the viewer's mark on a file. Dismissed is a viewed file changed since.
type FileViewedState string

const (
	FileUnviewed  FileViewedState = "UNVIEWED"
	FileViewed    FileViewedState = "VIEWED"
	FileDismissed FileViewedState = "DISMISSED"
)

// ChangedFile is one file's diff. Omitted says why Hunks is empty, as for a binary or oversized file.
type ChangedFile struct {
	Path         string
	PreviousPath string
	Status       FileStatus
	Additions    int
	Deletions    int
	Hunks        []Hunk
	Omitted      string
	Viewed       FileViewedState
	Viewing      bool
}

// FilesResult is one files response. RateLimit is empty for commit files, which need no GraphQL request.
type FilesResult struct {
	Files     []ChangedFile
	RateLimit RateLimit

	MoreFiles int

	// Truncated is a full page with no total to measure it against, as on a commit.
	Truncated bool
}

type DetailResult struct {
	Detail    PullRequestDetail
	RateLimit RateLimit
}

// Pulse is the small subset of a detail: lifecycle, review, mergeability and checks.
type Pulse struct {
	State          PRState
	IsDraft        bool
	ReviewDecision ReviewDecision
	Merge          MergeState
	Rollup         CheckRollup
	UpdatedAt      time.Time

	// HeadRefOid moving is the one sign here that the held diff is stale.
	HeadRefOid string
}

type PulseResult struct {
	Pulse     Pulse
	RateLimit RateLimit
}

// RateLimit is the GraphQL point budget as of the last response.
type RateLimit struct {
	Limit     int
	Cost      int
	Remaining int
	ResetAt   time.Time
}

type ViewerResult struct {
	Viewer    Actor
	RateLimit RateLimit
}

// CommentResult carries no RateLimit, like every mutation result: rateLimit is a Query field a
// mutation cannot select.
type CommentResult struct {
	Comment Comment
}

// ThreadResult carries the permissions back because resolving flips them.
type ThreadResult struct {
	ID           string
	IsResolved   bool
	CanResolve   bool
	CanUnresolve bool
}

// ReactionResult is a subject's whole reaction set after a toggle.
type ReactionResult struct {
	Reactions []Reaction
}

type SearchResult struct {
	PullRequests []PullRequest
	RateLimit    RateLimit
}

// RepoMeta is a repository's picker choices. Users is who may be assigned, and is offered for review
// too: GitHub has no requestable-reviewers connection. Mentions is the wider set of everyone who has
// taken part, and has no ids to write back.
type RepoMeta struct {
	Labels   []Label
	Users    []Actor
	Mentions []Mention
	Methods  MergeMethods
}

type MergeMethods struct {
	Merge, Squash, Rebase bool

	// DeleteOnMerge is GitHub deleting the head branch itself; a client also deleting it races that
	// and fails.
	DeleteOnMerge bool
}

func (m MergeMethods) Allows(method MergeMethod) bool {
	switch method {
	case MergeMethodMerge:
		return m.Merge
	case MergeMethodSquash:
		return m.Squash
	case MergeMethodRebase:
		return m.Rebase
	}
	return false
}

type RepoMetaResult struct {
	Meta      RepoMeta
	RateLimit RateLimit
}

type LabelsResult struct {
	Labels []Label
}

type AssigneesResult struct {
	Assignees []Actor
}

// PRStateResult carries both fields because closing a draft leaves it a draft.
type PRStateResult struct {
	State   PRState
	IsDraft bool
}

// BaseResult carries no behind-by count: the mutation runs no comparison.
type BaseResult struct {
	BaseRefName string
}

type BodyResult struct {
	Body string
}

// MergeOptions is one merge. Headline and Body are empty for a rebase, and may be empty otherwise for
// GitHub's default.
type MergeOptions struct {
	Method   MergeMethod
	Headline string
	Body     string

	// ExpectedHeadOid makes GitHub refuse the merge if the branch has moved since.
	ExpectedHeadOid string
}

type MergeResult struct {
	State PRState
}

// BranchResult is one branch search. Query is the search it answers, so a caller can drop a stale one.
type BranchResult struct {
	Query    string
	Default  string
	Branches []string

	// More is how many matched past the page.
	More int

	RateLimit RateLimit
}

type PullRequest struct {
	ID     string
	Number int
	Title  string
	URL    string
	// Repository is "owner/name".
	Repository  string
	Author      Actor
	State       PRState
	IsDraft     bool
	HeadRefName string
	BaseRefName string

	Additions    int
	Deletions    int
	ChangedFiles int

	// Comments counts the conversation plus its review threads.
	Comments int

	Checks         CheckState
	ReviewDecision ReviewDecision

	CreatedAt time.Time
	UpdatedAt time.Time
}
