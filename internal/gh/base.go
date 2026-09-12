package gh

import (
	"context"
	"fmt"
)

const setBaseMutation = `
mutation SetBase($pullRequestId: ID!, $baseRefName: String!) {
  updatePullRequest(input: {pullRequestId: $pullRequestId, baseRefName: $baseRefName}) {
    pullRequest {
      id
      baseRefName
    }
  }
}`

type setBaseResponse struct {
	UpdatePullRequest struct {
		PullRequest *struct {
			ID          string
			BaseRefName string
		}
	}
}

// The description is a pull request field, not a comment node, so updateIssueComment cannot reach it.
const setBodyMutation = `
mutation SetBody($pullRequestId: ID!, $body: String!) {
  updatePullRequest(input: {pullRequestId: $pullRequestId, body: $body}) {
    pullRequest {
      id
      body
    }
  }
}`

type setBodyResponse struct {
	UpdatePullRequest struct {
		PullRequest *struct {
			ID   string
			Body string
		}
	}
}

// SetBody rewrites a pull request's description and returns it as GitHub recorded it. An empty body
// clears it.
func (c *Client) SetBody(ctx context.Context, prID, body string) (BodyResult, error) {
	var resp setBodyResponse
	vars := map[string]any{"pullRequestId": prID, "body": body}

	if err := c.gql.DoWithContext(ctx, setBodyMutation, vars, &resp); err != nil {
		return BodyResult{}, fmt.Errorf("editing the description: %w", classify(err))
	}

	pr := resp.UpdatePullRequest.PullRequest
	if pr == nil || pr.ID == "" {
		return BodyResult{}, fmt.Errorf("editing the description: GitHub returned no pull request")
	}
	return BodyResult{Body: pr.Body}, nil
}

// SetBase retargets a pull request onto base, a branch name without refs/heads/, and returns the base
// GitHub recorded. GitHub refuses a merged pull request and a base equal to the head.
func (c *Client) SetBase(ctx context.Context, prID, base string) (BaseResult, error) {
	var resp setBaseResponse
	vars := map[string]any{"pullRequestId": prID, "baseRefName": base}

	if err := c.gql.DoWithContext(ctx, setBaseMutation, vars, &resp); err != nil {
		return BaseResult{}, fmt.Errorf("setting base branch: %w", classify(err))
	}

	pr := resp.UpdatePullRequest.PullRequest
	if pr == nil || pr.BaseRefName == "" {
		return BaseResult{}, fmt.Errorf("setting base branch: GitHub returned no pull request")
	}
	return BaseResult{BaseRefName: pr.BaseRefName}, nil
}
