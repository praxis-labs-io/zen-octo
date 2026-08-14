package gh

import (
	"context"
	"fmt"
	"time"
)

// addReplyMutation answers a line-anchored review thread. addComment cannot: its
// subjectId is a Commentable, and a PullRequestReviewThread is not one. This is
// GitHub's own call for the reply box under a review comment, and it posts
// immediately rather than holding the comment in a pending review.
//
// It asks the comment back for the same reason addComment does, and selects no
// rateLimit for the same reason: that field is on Query alone, and a mutation
// naming it is rejected whole.
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

// AddReply posts a reply to a review thread and returns it as GitHub recorded
// it. threadID is the thread's node id, which is what ReviewThread.ID carries.
//
// The comment comes back as CommentThread, the same kind the detail query gives
// the comments already in the thread, so a reply just written and one fetched an
// hour later render the same and edit through the same mutation.
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
