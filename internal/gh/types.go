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

// Actor is a user, organization, or bot. Author fields are nil on GitHub when
// the account is deleted, so Login can legitimately be empty.
type Actor struct {
	Login string
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
