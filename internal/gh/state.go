package gh

import (
	"context"
	"fmt"
)

// Each aliases its payload to result, so one response struct decodes all four.
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

// SetState applies a lifecycle transition and returns the state GitHub recorded, which may not be the
// one asked for. An unknown transition is an error, made without calling GitHub.
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
