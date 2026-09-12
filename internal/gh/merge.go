package gh

import (
	"context"
	"fmt"
)

// Headline and body are null only for a rebase; null on another method puts GitHub's default over the caller's text.
const mergeMutation = `
mutation MergePR(
  $pullRequestId: ID!,
  $mergeMethod: PullRequestMergeMethod!,
  $expectedHeadOid: GitObjectID,
  $commitHeadline: String,
  $commitBody: String
) {
  mergePullRequest(input: {
    pullRequestId: $pullRequestId,
    mergeMethod: $mergeMethod,
    expectedHeadOid: $expectedHeadOid,
    commitHeadline: $commitHeadline,
    commitBody: $commitBody
  }) {
    pullRequest {
      id
      state
    }
  }
}`

type mergeResponse struct {
	MergePullRequest struct {
		PullRequest *struct {
			ID    string
			State string
		}
	}
}

func (c *Client) Merge(ctx context.Context, prID string, opts MergeOptions) (MergeResult, error) {
	vars := map[string]any{
		"pullRequestId":   prID,
		"mergeMethod":     string(opts.Method),
		"expectedHeadOid": nil,
		"commitHeadline":  nil,
		"commitBody":      nil,
	}
	if opts.ExpectedHeadOid != "" {
		vars["expectedHeadOid"] = opts.ExpectedHeadOid
	}

	if opts.Method != MergeMethodRebase {
		vars["commitHeadline"] = opts.Headline
		vars["commitBody"] = opts.Body
	}

	var resp mergeResponse
	if err := c.gql.DoWithContext(ctx, mergeMutation, vars, &resp); err != nil {
		return MergeResult{}, fmt.Errorf("merging: %w", classify(err))
	}

	pr := resp.MergePullRequest.PullRequest
	if pr == nil || pr.ID == "" {
		return MergeResult{}, fmt.Errorf("merging: GitHub returned no pull request")
	}
	return MergeResult{State: PRState(pr.State)}, nil
}

const deleteRefMutation = `
mutation DeleteRef($refId: ID!) {
  deleteRef(input: {refId: $refId}) {
    clientMutationId
  }
}`

func (c *Client) DeleteRef(ctx context.Context, refID string) error {
	var resp struct {
		DeleteRef struct{ ClientMutationID string }
	}

	vars := map[string]any{"refId": refID}
	if err := c.gql.DoWithContext(ctx, deleteRefMutation, vars, &resp); err != nil {
		return fmt.Errorf("deleting the branch: %w", classify(err))
	}
	return nil
}
