package gh

import (
	"context"
	"fmt"
)

// resolveThreadMutation and unresolveThreadMutation close a review thread and
// open it again. Both are addressed to threadId, which is not the spelling the
// reply takes: addPullRequestReviewThreadReply wants pullRequestReviewThreadId,
// and these two take the shorter name.
//
// Both ask the permissions back, because resolving flips which of the two the
// next press needs. Neither selects rateLimit, for the reason addComment gives:
// that field is on Query alone, and a mutation naming it is rejected whole.
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

// threadResolveResponse decodes either payload. The method reads the half its
// own document produced rather than whichever field came back filled, so a
// response landing in the wrong one is a failure instead of a silent pass.
type threadResolveResponse struct {
	ResolveReviewThread   struct{ Thread threadNode }
	UnresolveReviewThread struct{ Thread threadNode }
}

// SetThreadResolved closes a review thread or opens it again, and returns the
// thread as GitHub recorded it. threadID is the thread's node id, which is what
// ReviewThread.ID carries.
//
// One method over two because this is one control with two permissions, which
// is how GitHub models it and how the card names it. The direction picks the
// document and the words the error uses.
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
