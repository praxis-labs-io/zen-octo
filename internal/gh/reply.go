package gh

import (
	"context"
	"fmt"
	"time"
)

// addComment cannot reach a thread: a PullRequestReviewThread is not Commentable.
const addReplyMutation = `
mutation AddReply($threadId: ID!, $reviewId: ID, $body: String!) {
  addPullRequestReviewThreadReply(input: {pullRequestReviewThreadId: $threadId, pullRequestReviewId: $reviewId, body: $body}) {
    comment {
      id
      createdAt
      body
      state
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
			State     string
		}
	}
}

// AddReply answers threadID. A reviewID adds the reply to that pending review, and the result comes back Draft.
func (c *Client) AddReply(ctx context.Context, threadID, reviewID, body string) (CommentResult, error) {
	var resp addReplyResponse
	vars := map[string]any{"threadId": threadID, "reviewId": nullable(reviewID), "body": body}

	if err := c.gql.DoWithContext(ctx, addReplyMutation, vars, &resp); err != nil {
		return CommentResult{}, fmt.Errorf("posting a reply: %w", classify(err))
	}

	node := resp.AddPullRequestReviewThreadReply.Comment
	if node.ID == "" {
		return CommentResult{}, fmt.Errorf("posting a reply: GitHub returned no comment")
	}

	comment := node.comment(CommentThread, node.CreatedAt)
	comment.Draft = ReviewState(node.State) == ReviewStatePending
	return CommentResult{Comment: comment}, nil
}
