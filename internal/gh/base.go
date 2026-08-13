package gh

import (
	"context"
	"fmt"
)

// setBaseMutation retargets a pull request onto another branch.
//
// updatePullRequest, the same mutation the label and assignee writes go
// through, and so the same viewerCanUpdate permission behind it. The base is a
// branch name rather than a node id, which is the one place this input departs
// from the two beside it.
//
// It asks the name back for the reason setLabelsMutation does: the caller is
// holding an optimistic row and GitHub is the only authority on what the pull
// request now targets.
//
// It asks for no rateLimit, which is on Query and nowhere else.
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

// setBodyMutation rewrites a pull request's description. The same
// updatePullRequest the base, label and assignee writes go through, and so the
// same viewerCanUpdate permission behind it.
//
// The description is a comment everywhere on the screen and is not one to
// GitHub: it is a field of the pull request rather than a Comment node, so
// updateIssueComment cannot reach it and this is the call that can.
//
// It asks the body back for the reason setBaseMutation asks the name, and asks
// for no rateLimit, which is on Query and nowhere else.
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

// SetBody rewrites a pull request's description and returns it as GitHub
// recorded it. prID is the pull request's node id.
//
// An empty body is a description cleared, which is a write like any other, so
// the check below is on the pull request coming back rather than on the text.
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

// SetBase moves a pull request onto another base branch and returns the branch
// as GitHub recorded it. prID is the pull request's node id; base is a branch
// name, without the refs/heads/ prefix.
//
// GitHub refuses a merged pull request and refuses a base equal to the head, so
// the caller offers neither. Everything else it refuses arrives here as an
// error for the revert branch to answer.
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
