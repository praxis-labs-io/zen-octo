package gh

import (
	"context"
	"fmt"
)

// The four documents behind PRTransition. GitHub gives each move its own
// mutation rather than a field to set, so there is no one document with the
// destination as a variable.
//
// Each aliases its payload to `result`. The four payload types differ only in
// name and carry the same pullRequest, so the alias is what lets one response
// struct decode all four instead of four structs told apart by a switch.
//
// They ask the state back for the same reason addComment asks for the comment:
// the caller is holding an optimistic row and GitHub is the only authority on
// where the pull request actually now sits. Both fields, because closing a
// draft leaves it a draft.
//
// None of them asks for rateLimit. That field is on Query and nowhere else, and
// a mutation selecting it is rejected whole.
const (
	markReadyMutation = `
mutation MarkReady($pullRequestId: ID!) {
  result: markPullRequestReadyForReview(input: {pullRequestId: $pullRequestId}) {
    pullRequest { id state isDraft }
  }
}`

	convertDraftMutation = `
mutation ConvertToDraft($pullRequestId: ID!) {
  result: convertPullRequestToDraft(input: {pullRequestId: $pullRequestId}) {
    pullRequest { id state isDraft }
  }
}`

	closePRMutation = `
mutation ClosePR($pullRequestId: ID!) {
  result: closePullRequest(input: {pullRequestId: $pullRequestId}) {
    pullRequest { id state isDraft }
  }
}`

	reopenPRMutation = `
mutation ReopenPR($pullRequestId: ID!) {
  result: reopenPullRequest(input: {pullRequestId: $pullRequestId}) {
    pullRequest { id state isDraft }
  }
}`
)

type prStateResponse struct {
	Result struct {
		PullRequest *struct {
			ID      string
			State   string
			IsDraft bool
		}
	}
}

// stateMutation is the document for a transition, and false for a transition
// this package does not know. Kept apart from SetState so the refusal happens
// before anything reaches the network.
func stateMutation(to PRTransition) (string, bool) {
	switch to {
	case TransitionReady:
		return markReadyMutation, true
	case TransitionDraft:
		return convertDraftMutation, true
	case TransitionClose:
		return closePRMutation, true
	case TransitionReopen:
		return reopenPRMutation, true
	}
	return "", false
}

// SetState moves a pull request through its lifecycle and returns where it
// landed. prID is the pull request's node id.
//
// It refuses a transition it has no document for without calling GitHub, the
// way RepoMeta refuses a malformed repository name: a request that could only
// come back rejected is not worth a round trip.
//
// It does not check that the answer is the state that was asked for. GitHub is
// the authority on where the pull request sits, and a caller folding what it is
// handed is right even when that is not what it wanted.
func (c *Client) SetState(ctx context.Context, prID string, to PRTransition) (PRStateResult, error) {
	doc, ok := stateMutation(to)
	if !ok {
		return PRStateResult{}, fmt.Errorf("changing the state: no such transition (%s)", to)
	}

	var resp prStateResponse
	vars := map[string]any{"pullRequestId": prID}

	if err := c.gql.DoWithContext(ctx, doc, vars, &resp); err != nil {
		return PRStateResult{}, fmt.Errorf("changing the state: %w", classify(err))
	}

	pr := resp.Result.PullRequest
	if pr == nil || pr.ID == "" {
		return PRStateResult{}, fmt.Errorf("changing the state: GitHub returned no pull request")
	}
	return PRStateResult{State: PRState(pr.State), IsDraft: pr.IsDraft}, nil
}
