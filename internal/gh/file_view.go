package gh

import (
	"context"
	"fmt"
)

const markFileViewedMutation = `
mutation MarkFileViewed($pullRequestId: ID!, $path: String!) {
  markFileAsViewed(input: {pullRequestId: $pullRequestId, path: $path}) {
    pullRequest { id }
  }
}`

const unmarkFileViewedMutation = `
mutation UnmarkFileViewed($pullRequestId: ID!, $path: String!) {
  unmarkFileAsViewed(input: {pullRequestId: $pullRequestId, path: $path}) {
    pullRequest { id }
  }
}`

type fileViewedResponse struct {
	MarkFileAsViewed   struct{ PullRequest struct{ ID string } }
	UnmarkFileAsViewed struct{ PullRequest struct{ ID string } }
}

func (c *Client) SetFileViewed(ctx context.Context, prID, path string, viewed bool) error {
	doc, action := unmarkFileViewedMutation, "marking a file unviewed"
	if viewed {
		doc, action = markFileViewedMutation, "marking a file viewed"
	}

	var resp fileViewedResponse
	vars := map[string]any{"pullRequestId": prID, "path": path}
	if err := c.gql.DoWithContext(ctx, doc, vars, &resp); err != nil {
		return fmt.Errorf("%s: %w", action, classify(err))
	}

	id := resp.UnmarkFileAsViewed.PullRequest.ID
	if viewed {
		id = resp.MarkFileAsViewed.PullRequest.ID
	}
	if id == "" {
		return fmt.Errorf("%s: GitHub returned no pull request", action)
	}
	return nil
}
