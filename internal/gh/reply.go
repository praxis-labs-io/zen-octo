package gh

import (
	"context"
	"fmt"
	"time"
)

// addComment cannot reach a thread: a PullRequestReviewThread is not Commentable.
const addReplyMutation = `
mutation AddReply($threadId: ID!, $body: String!) {
  addPullRequestReviewThreadReply(input: {pullRequestReviewThreadId: $threadId, body: $body}) {
    comment {
      id
      createdAt
      body
      author { login }
      viewerDidAuthor
      viewerCanUpdate
      viewerCanDelete
      viewerCanReact
      reactionGroups { content viewerHasReacted reactors { totalCount } }
    }
  }
}`

type addReplyResponse struct {
	AddPullRequestReviewThreadReply struct {
		Comment struct {
			commentNode
			CreatedAt time.Time
		}
	}
}

// AddReply posts a reply to the review thread with node id threadID and returns it as GitHub recorded it.
func (c *Client) AddReply(ctx context.Context, threadID, body string) (CommentResult, error) {
	var resp addReplyResponse
	vars := map[string]any{"threadId": threadID, "body": body}

	if err := c.gql.DoWithContext(ctx, addReplyMutation, vars, &resp); err != nil {
		return CommentResult{}, fmt.Errorf("posting a reply: %w", classify(err))
	}

	node := resp.AddPullRequestReviewThreadReply.Comment
	if node.ID == "" {
		return CommentResult{}, fmt.Errorf("posting a reply: GitHub returned no comment")
	}

	return CommentResult{Comment: node.comment(CommentThread, node.CreatedAt)}, nil
}
