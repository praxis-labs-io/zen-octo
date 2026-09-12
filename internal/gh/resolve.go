package gh

import (
	"context"
	"fmt"
)

const resolveThreadMutation = `
mutation ResolveThread($threadId: ID!) {
  resolveReviewThread(input: {threadId: $threadId}) {
    thread {
      id
      isResolved
      viewerCanResolve
      viewerCanUnresolve
    }
  }
}`

const unresolveThreadMutation = `
mutation UnresolveThread($threadId: ID!) {
  unresolveReviewThread(input: {threadId: $threadId}) {
    thread {
      id
      isResolved
      viewerCanResolve
      viewerCanUnresolve
    }
  }
}`

type threadNode struct {
	ID                 string
	IsResolved         bool
	ViewerCanResolve   bool
	ViewerCanUnresolve bool
}

type threadResolveResponse struct {
	ResolveReviewThread   struct{ Thread threadNode }
	UnresolveReviewThread struct{ Thread threadNode }
}

func (c *Client) SetThreadResolved(ctx context.Context, threadID string, resolved bool) (ThreadResult, error) {
	doc, doing := unresolveThreadMutation, "unresolving a review thread"
	if resolved {
		doc, doing = resolveThreadMutation, "resolving a review thread"
	}

	var resp threadResolveResponse
	vars := map[string]any{"threadId": threadID}

	if err := c.gql.DoWithContext(ctx, doc, vars, &resp); err != nil {
		return ThreadResult{}, fmt.Errorf("%s: %w", doing, classify(err))
	}

	node := resp.UnresolveReviewThread.Thread
	if resolved {
		node = resp.ResolveReviewThread.Thread
	}
	if node.ID == "" {
		return ThreadResult{}, fmt.Errorf("%s: GitHub returned no thread", doing)
	}

	return ThreadResult{
		ID:           node.ID,
		IsResolved:   node.IsResolved,
		CanResolve:   node.ViewerCanResolve,
		CanUnresolve: node.ViewerCanUnresolve,
	}, nil
}
