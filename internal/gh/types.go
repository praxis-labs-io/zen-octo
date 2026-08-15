package gh

import "time"

// PRState is where a pull request sits in its lifecycle.
type PRState string

const (
	PRStateOpen   PRState = "OPEN"
	PRStateClosed PRState = "CLOSED"
	PRStateMerged PRState = "MERGED"
)

// PRTransition is a change to where a pull request sits in its lifecycle. It is
// the move rather than the destination: GitHub spells each one as its own
// mutation, and draft and closed are two independent fields, so "closed" alone
// does not say what should happen to the other one.
type PRTransition string

const (
	TransitionReady  PRTransition = "READY"
	TransitionDraft  PRTransition = "DRAFT"
	TransitionClose  PRTransition = "CLOSE"
	TransitionReopen PRTransition = "REOPEN"
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

	// The metadata kinds. Each names something the rail can write, and each
	// carries a Subject saying what it was written to.
	TimelineLabeled         TimelineKind = "LABELED"
	TimelineUnlabeled       TimelineKind = "UNLABELED"
	TimelineAssigned        TimelineKind = "ASSIGNED"
	TimelineUnassigned      TimelineKind = "UNASSIGNED"
	TimelineReviewRequested TimelineKind = "REVIEW_REQUESTED"
	TimelineReviewCancelled TimelineKind = "REVIEW_REQUEST_REMOVED"
	TimelineBaseChanged     TimelineKind = "BASE_REF_CHANGED"
)

// Actor is a user, organization, or bot. Author fields are nil on GitHub when
// the account is deleted, so Login can legitimately be empty.
//
// ID is the node id, and it is empty wherever the query had no use for one,
// which is every author field. It is asked for on the two lists a picker
// writes back: updatePullRequest sets assignees by id and has no spelling that
// takes a login.
type Actor struct {
	ID    string
	Login string
}

// Mention is somebody who can be named in a comment on a repository. Login is
// what goes into the buffer, and Name is what tells two similar logins apart. A
// name is empty on an account that has set none, and it is never filled in from
// the login: a name equal to a handle would be indistinguishable from one
// somebody chose.
//
// It is not an Actor, and the missing id is why. An Actor carries one because
// the lists a picker writes back are addressed by node id, and a mention has
// none to carry. Were this an Actor it would compile straight into
// assigneeChoices, where every row would be matched on an id it does not have.
type Mention struct {
	Login string
	Name  string
}

// Label is one label.
//
// ID is the node id, which is the only thing updatePullRequest will take: the
// mutation sets labels by id and has no spelling that accepts a name.
//
// GitHub's own color is deliberately not fetched. The hex is chosen against a
// white browser page, so a pale label vanishes on a dark terminal and no theme
// can reach it, and a terminal speaking only ANSI cannot show it at all. Labels
// are colored from the active theme instead.
type Label struct {
	ID   string
	Name string
}

// CommentKind says which of GitHub's three comment types this is. A node id on
// its own does not name the call that edits it: updateIssueComment,
// updatePullRequestReviewComment and updatePullRequestReview are three
// mutations over one domain type.
type CommentKind string

const (
	// CommentIssue is a comment standing on its own in the conversation.
	// GitHub calls it an issue comment on a pull request as well as an issue.
	CommentIssue CommentKind = "ISSUE"

	// CommentReview is a review's own body, the words above its threads.
	CommentReview CommentKind = "REVIEW"

	// CommentThread is one comment inside a line-anchored review thread.
	CommentThread CommentKind = "THREAD"
)

// ReactionContent is one of the eight reactions GitHub takes. The constant is
// the wire word rather than a name chosen here, because the mutation takes this
// value as an enum and nothing above translates it back.
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

// ReactionOrder is the eight in GitHub's own order, which is the order its page
// offers them in and the order a card renders them in. A screen picking its own
// would put the same set of pills in a different place on every comment.
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

// Reaction is one kind of reaction on one subject, and how many people gave it.
//
// Viewer is whether the account holding the token is one of them, which is what
// makes the key a toggle rather than an add: there is no call that sets a
// reaction, only one that adds and one that removes.
//
// GitHub answers with all eight groups on every subject, nearly all at zero.
// This package returns the ones somebody actually gave, or every card on the
// page grows a row of eight pills saying nothing.
type Reaction struct {
	Content ReactionContent
	Count   int
	Viewer  bool

	// Pending marks a reaction toggled here and not yet answered for. This
	// package never sets it, on the terms Comment.Pending sets out: the store
	// sets it on the optimistic copy and the screen keeps the key off it.
	Pending bool
}

// Comment is one piece of writing in the conversation, whether it stands alone,
// heads a review, or sits inside a review thread. One type for all three is what
// lets one component render them, and what makes issues cheap to add later.
//
// ViewerDidAuthor is not CanEdit. A maintainer can edit and delete anyone's
// comment in their own repository, and only the first of the two says whose
// writing it is.
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

	// Reactions is what people gave this comment, in GitHub's order and with the
	// empty groups dropped.
	Reactions []Reaction

	// Pending marks a comment written here and not yet acknowledged by GitHub.
	// This package never sets it: it has nothing pending, everything it returns
	// has already happened. The store sets it on the optimistic copy it holds
	// until the mutation answers, and the screen reads it to say so.
	Pending bool

	// Editing marks a comment GitHub has, showing words it has not confirmed.
	// Set by the store while a rewrite is out, on the same terms as Pending and
	// apart from it because the two say different things: a pending comment is
	// not on GitHub at all, and this one is, under older text. The screen says
	// so differently for each, and both keep the keys off a comment already
	// answering for a write.
	Editing bool
}

// DiffSide is which half of the diff a line belongs to. A comment on a deleted
// line and one on an added line can carry the same number, so the side is what
// tells them apart.
type DiffSide string

const (
	SideRight DiffSide = "RIGHT"
	SideLeft  DiffSide = "LEFT"
)

// ReviewThread is a line-anchored discussion. ID is the thread's own node,
// which is what a reply or a resolve is addressed to. ReviewID names the review
// its first comment was submitted with, which is how the conversation puts a
// thread under the review that opened it.
//
// StartLine is zero on a single-line thread. Line is the last line either way,
// which is where GitHub itself hangs the thread.
//
// Hunk is the few lines of diff the thread was written against, nil when GitHub
// returned none. Without it a comment on the conversation reads as an assertion
// about code that is nowhere on the screen.
//
// CanResolve and CanUnresolve are separate permissions on one control. Someone
// can be allowed to close a thread and not to reopen it.
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

	// Pending marks a thread whose resolution was changed here and not yet
	// acknowledged. This package never sets it, the same as Comment.Pending: it
	// has nothing pending. The store sets it while the write is out, and the
	// screen reads it to keep a second press off a thread already answering
	// for one.
	Pending bool

	Hunk     *Hunk
	Comments []Comment
}

// TimelineItem is one entry in the conversation. Comment is nil on an event,
// Review is set only when Kind is TimelineReview, and Commit only when Kind is
// TimelineCommit.
//
// Actor and CreatedAt are on the item rather than read off the comment, because
// an event has both and no comment, and the list is sorted by the timestamp
// before anything knows which kind it is looking at.
type TimelineItem struct {
	Kind      TimelineKind
	Actor     Actor
	CreatedAt time.Time
	Comment   *Comment
	Review    ReviewState
	Commit    *Commit

	// Subject is what the event was done to: a label's name, the handle of
	// somebody assigned or asked for a review, or the branch a base moved to.
	// Empty on every kind that acts on the pull request as a whole.
	Subject string

	// Was is the value Subject replaced. Only a base ref change has one.
	Was string
}

// Said is the comment behind a comment or a review, and the zero Comment on
// anything else, so a renderer reads a body without a nil check.
func (i TimelineItem) Said() Comment {
	if i.Comment == nil {
		return Comment{}
	}
	return *i.Comment
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
	Body        string
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

// MergeMethod is how a merge is made. The three GitHub takes, spelled the way
// its enum spells them.
type MergeMethod string

const (
	MergeMethodMerge  MergeMethod = "MERGE"
	MergeMethodSquash MergeMethod = "SQUASH"
	MergeMethodRebase MergeMethod = "REBASE"
)

// MergeMessage is the commit message GitHub would write for one merge method.
//
// It comes from GitHub rather than from anything computed here. The repository
// decides whether a squash title is the pull request's or its single commit's,
// and whether the body is the pull request's or a list of the commits, so no
// combination of fields on this side reconstructs it. Both are empty for a
// rebase, which is GitHub saying a rebase writes no commit of its own.
type MergeMessage struct {
	Headline string
	Body     string
}

// Reviewer is someone GitHub lists on the pull request's reviewers panel:
// either they have reviewed, or a review has been requested of them.
//
// State is empty when the request is still outstanding. It is not a review's
// own state until they submit one.
//
// Unresolved is how many of their review threads are still open, and Threads is
// how many they opened at all. A reviewer who only commented is still waiting on
// something if any of them are open.
//
// The total is there to tell "every point addressed" from "there was nothing to
// address". Both leave Unresolved at zero, and they are opposite answers: one is
// a reviewer whose asks have all been met, the other is one who asked for
// changes in prose with nothing to resolve, and nothing has been done about it.
//
// Requested is whether a review is outstanding from them right now. It is not
// the inverse of State: submitting a review clears the request, but the review
// can then be asked for again, and such a reviewer carries a verdict and an
// open request at once. A caller reading "no state means waiting" gets that
// pair wrong in both directions, which is why this is its own field.
//
// Team marks a review requested of a team rather than a person. Login is then
// the synthetic "org/slug" handle teamHandle builds, which no write accepts
// where a login goes: the REST endpoint takes teams in a separate array. It is
// a field rather than a slash in the login, so a caller that must leave teams
// alone says so instead of sniffing for one.
type Reviewer struct {
	Actor      Actor
	State      ReviewState
	Unresolved int
	Threads    int
	Requested  bool
	Team       bool
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

// Key tells one check from another. A check run has a node id, but the rollup
// folds a re-run onto the attempt it replaces, so the id changes under a check
// that is still the same row. This does not.
//
// The rollup keeps one check per workflow and name, which is what makes it
// unique. The separator is a NUL rather than a slash because a workflow may
// hold one.
func (c Check) Key() string { return c.Workflow + "\x00" + c.Name }

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

// ViewerActions is what the signed-in account may do to a pull request, as
// GitHub answers it rather than as this client guesses it. A control offering a
// write GitHub will refuse is worse than no control.
//
// It sits on the detail rather than on PullRequest because search does not ask
// for these, and a false on every row of the list would read as a refusal
// rather than as a question never put.
//
// CanUpdate governs the draft toggle. GitHub publishes no viewer field for
// markPullRequestReadyForReview or convertPullRequestToDraft; this is the
// nearest one, and it is true for exactly the accounts those two accept, the
// author and anyone with write access.
//
// There is no CanRequestReviews beside CanAssign, because GitHub publishes no
// field for it. The two are not the same permission either: assigning needs
// triage access and requesting a review needs write, so borrowing this one for
// both would hide a control that works.
//
// CanMergeAsAdmin is whether the viewer may merge over branch protection.
// There is no plain viewerCanMerge beside it: an ordinary merge is ungated
// here, and a refusal comes back from the mutation for the revert branch to
// answer, the same way a review request does.
//
// There is no flag for deleting the head branch either. viewerCanDeleteHeadRef
// looks like one and answers a different question: it is false on every open
// pull request, whatever the account holds, and turns true once it closes. Read
// at the only moment a merge form can be open, it says no every time.
type ViewerActions struct {
	CanUpdate bool
	CanClose  bool
	CanReopen bool
	CanAssign bool

	CanMergeAsAdmin bool

	// CanReact governs the description alone. Every comment carries its own
	// answer to the same question, and the description is not a comment: it is
	// a field of the pull request, and the pull request is what a reaction to it
	// is addressed to.
	CanReact bool
}

// PullRequestDetail embeds the row, so a detail response refreshes the header
// and the rail rather than leaving them on what search returned.
type PullRequestDetail struct {
	PullRequest

	Body      string
	Labels    []Label
	Assignees []Actor
	Reviewers []Reviewer

	// Reactions is what people gave the description. It hangs off the detail
	// rather than off a comment because GitHub has no comment here: the
	// description is a field of the pull request, and the pull request is the
	// subject a reaction to it names.
	Reactions []Reaction

	Timeline []TimelineItem
	Threads  []ReviewThread
	Commits  []Commit
	Rollup   CheckRollup

	Merge MergeState

	// HeadRefOid is the commit at the tip of the head branch when this was
	// fetched, which is what a merge sends as the commit it means. HeadRefID is
	// that branch's node id, which is what deleting it takes; it is empty once
	// the branch is gone.
	HeadRefOid string
	HeadRefID  string

	// CrossRepository is a head branch living in a fork rather than here. It
	// stands in for a delete permission GitHub publishes no usable field for:
	// somebody else's branch is the one case worth refusing outright, and the
	// rest is left to the call itself to accept or refuse.
	CrossRepository bool

	// MergeCommit and SquashCommit are what GitHub would write for those two
	// methods. A rebase has neither, so there is no third field.
	MergeCommit  MergeMessage
	SquashCommit MergeMessage

	// Viewer is what the signed-in account may do to this pull request.
	Viewer ViewerActions

	// BehindBy is how many commits the base has that the head does not. Zero is
	// up to date, and BehindUnknown is nobody has counted yet.
	BehindBy int

	// MoreComments, MoreThreads, MoreCommits and MoreEvents are what the first
	// page did not reach. A dropped comment that reads as no comment is the
	// failure worth a field.
	//
	// Events share one window across every type asked for, and the metadata ones
	// are the high-volume ones: a repository that labels on every push can fill
	// the hundred and push a merge or a force push out of it. So this counts the
	// window the item types were filtered to, never the whole timeline, which
	// holds subscriptions and mentions nothing here ever asks for.
	MoreComments int
	MoreThreads  int
	MoreCommits  int
	MoreEvents   int
}

// MergeMessage is what GitHub would commit for one method, and the zero value
// for a rebase, which writes no commit of its own.
func (d PullRequestDetail) MergeMessage(m MergeMethod) MergeMessage {
	switch m {
	case MergeMethodMerge:
		return d.MergeCommit
	case MergeMethodSquash:
		return d.SquashCommit
	}
	return MergeMessage{}
}

// BehindUnknown is BehindBy on a pull request whose base has just moved. The
// count comes from a comparison only GitHub can run, so a retarget applied
// optimistically has no honest number until the refetch answers, and zero is
// already spoken for: it means up to date.
const BehindUnknown = -1

// BehindNoHead is BehindBy with no head branch left to compare, which is the
// ordinary state of a merged pull request. Never counted, not uncounted yet.
const BehindNoHead = -2

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

// ViewerResult is one viewer response: who the token belongs to and what it
// cost. The login is what tells your own writing from everyone else's on a
// screen, before a permission field settles what you can do to it.
type ViewerResult struct {
	Viewer    Actor
	RateLimit RateLimit
}

// CommentResult is one comment written, as GitHub recorded it. The comment
// comes back rather than just its id: a write is the cheapest place to learn
// what the server made of it, and the caller has a placeholder to replace.
//
// It carries no RateLimit, unlike every result beside it. rateLimit is a field
// on Query and a mutation cannot select it, so a write's cost stays invisible
// until the next fetch reports a budget that has already moved.
type CommentResult struct {
	Comment Comment
}

// ThreadResult is one review thread's resolution, as GitHub recorded it. The
// permissions come back with the state because resolving flips them: the same
// key unresolves next, and only GitHub knows whether this viewer may.
//
// It carries no RateLimit, for the reason CommentResult gives.
type ThreadResult struct {
	ID           string
	IsResolved   bool
	CanResolve   bool
	CanUnresolve bool
}

// ReactionResult is a subject's reactions as GitHub recorded them after a
// toggle: the whole set rather than the one that moved, because the payload
// answers with all of it and the card renders all of it.
//
// It carries no RateLimit, for the reason CommentResult gives.
type ReactionResult struct {
	Reactions []Reaction
}

// SearchResult is one search response: what it matched and what it cost.
type SearchResult struct {
	PullRequests []PullRequest
	RateLimit    RateLimit
}

// RepoMeta is the choices a picker on the detail rail draws from. It belongs to
// the repository rather than to any one pull request, changes on the scale of
// days, and is fetched once per repository per session.
//
// Labels and the assignable users. The branches and the merge methods each
// arrive with the picker that reads them, rather than being fetched ahead of a
// caller and billed to every open in the meantime.
//
// Users is who may be assigned, and it is also what the reviewer picker offers.
// GitHub has no "requestable reviewers" connection, and the two sets differ
// only at the margins: anyone with read access can be asked for a review, and
// the assignable list is everyone with triage. The narrower list is the safe
// one to guess with, because a name it leaves out is a write nobody could have
// started rather than one GitHub refuses.
//
// Mentions is who may be named in a comment, which is the wider set: everybody
// who has taken part, rather than everybody with access. The two are separate
// fields because neither answers for the other. Offering the assignable list to
// a mention leaves out most of a thread, and offering the mentionable list to
// the assignee picker offers people GitHub refuses, with no id to write them
// back by.
type RepoMeta struct {
	Labels   []Label
	Users    []Actor
	Mentions []Mention
	Methods  MergeMethods
}

// MergeMethods is what a repository permits a merge to be made of, and what it
// does to the head branch afterwards.
type MergeMethods struct {
	Merge, Squash, Rebase bool

	// DeleteOnMerge is the repository deleting the head branch itself, some
	// moments after the merge lands. A client that also asks races that and
	// fails on a ref already gone, which is an error about a thing that worked.
	DeleteOnMerge bool
}

// Allows reports whether one method is on offer.
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

// RepoMetaResult is one repository-metadata response: the choices and what they
// cost.
type RepoMetaResult struct {
	Meta      RepoMeta
	RateLimit RateLimit
}

// LabelsResult is a pull request's labels as GitHub recorded them after a
// write. The whole set comes back rather than the delta, because the picker
// applies a whole set and the rail renders one: reconciling a delta against a
// list the reader already sees means computing what GitHub just told us.
//
// It carries no RateLimit, for the reason CommentResult gives.
type LabelsResult struct {
	Labels []Label
}

// AssigneesResult is a pull request's assignees as GitHub recorded them after a
// write, for the reason LabelsResult gives: the picker applies a whole set and
// the rail renders one.
//
// It carries no RateLimit, for the reason CommentResult gives.
type AssigneesResult struct {
	Assignees []Actor
}

// PRStateResult is where a pull request sits after a transition, as GitHub
// recorded it. Both fields, because closing a draft leaves it a draft and
// reopening one gives that back: a caller holding an optimistic row needs the
// pair rather than the field it asked to change.
//
// It carries no RateLimit, for the reason CommentResult gives.
type PRStateResult struct {
	State   PRState
	IsDraft bool
}

// BaseResult is the branch a pull request merges into after a retarget, as
// GitHub recorded it. The name comes back rather than being assumed from the
// ask: somebody else retargeting first is reported by the toast rather than
// contradicted by it.
//
// It carries no RateLimit, for the reason CommentResult gives. It carries no
// behind-by count either, because the mutation runs no comparison: that is what
// the refetch behind the write is for.
type BaseResult struct {
	BaseRefName string
}

// BodyResult is a pull request's description as GitHub recorded it after a
// write. The text comes back rather than being assumed from the ask, for the
// reason BaseResult gives: somebody else having edited it first is worth
// showing rather than overwriting on screen.
//
// It carries no RateLimit, for the reason CommentResult gives.
type BodyResult struct {
	Body string
}

// MergeOptions is one merge as the reader set it up.
//
// Headline and Body are empty for a rebase, which takes neither, and may be
// empty for the other two: GitHub writes its own default when they are not
// sent, and that default is the same text the form opened holding.
type MergeOptions struct {
	Method   MergeMethod
	Headline string
	Body     string

	// ExpectedHeadOid is the commit the reader was looking at. GitHub refuses
	// the merge when the branch has moved since, which is the point of sending
	// it: merging a commit nobody has seen is the failure worth a round trip.
	ExpectedHeadOid string
}

// MergeResult is where a pull request sits after a merge, as GitHub recorded
// it.
//
// It carries no RateLimit, for the reason CommentResult gives.
type MergeResult struct {
	State PRState
}

// BranchResult is one branch search: what matched on the page fetched, how many
// matched past it, and what the repository calls its default.
//
// Query is the search this answers. Two searches settle in whatever order the
// network gives them, so a caller painting one has to be able to tell whether
// it is still the one being asked.
type BranchResult struct {
	Query    string
	Default  string
	Branches []string

	// More is what the page did not reach, the same way MoreFiles reports its
	// own overflow. Narrowing the search is what gets at it; there is no paging
	// inside a modal.
	More int

	RateLimit RateLimit
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
