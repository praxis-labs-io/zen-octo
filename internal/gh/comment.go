package gh

import (
	"context"
	"fmt"
	"time"
)

// addCommentMutation writes a comment on anything GitHub calls commentable, a
// pull request among them. It asks the new comment back rather than just its
// id: the caller is holding a placeholder it has to replace, and the fields
// below are what the read path already renders one from.
const addCommentMutation = `
mutation AddComment($subjectId: ID!, $body: String!) {
  rateLimit { limit cost remaining resetAt }
  addComment(input: {subjectId: $subjectId, body: $body}) {
    commentEdge {
      node {
        id
        createdAt
        body
        author { login }
        viewerDidAuthor
        viewerCanUpdate
        viewerCanDelete
        viewerCanReact
      }
    }
  }
}`

type addCommentResponse struct {
	RateLimit struct {
		Limit     int
		Cost      int
		Remaining int
		ResetAt   time.Time
	}

	AddComment struct {
		CommentEdge struct {
			Node struct {
				commentNode
				CreatedAt time.Time
			}
		}
	}
}

// AddComment posts a comment on a pull request and returns it as GitHub
// recorded it. subjectID is the pull request's node id.
//
// It maps through the same commentNode the detail query reads, so a comment
// just written and a comment fetched an hour later are the same shape. The
// permission fields come back true for the author, and taking GitHub's answer
// rather than assuming is what keeps a later edit honest.
func (c *Client) AddComment(ctx context.Context, subjectID, body string) (CommentResult, error) {
	var resp addCommentResponse
	vars := map[string]any{"subjectId": subjectID, "body": body}

	if err := c.gql.DoWithContext(ctx, addCommentMutation, vars, &resp); err != nil {
		return CommentResult{}, fmt.Errorf("posting a comment: %w", classify(err))
	}

	node := resp.AddComment.CommentEdge.Node
	if node.ID == "" {
		return CommentResult{}, fmt.Errorf("posting a comment: GitHub returned no comment")
	}

	return CommentResult{
		Comment: node.comment(CommentIssue, node.CreatedAt),
		RateLimit: RateLimit{
			Limit:     resp.RateLimit.Limit,
			Cost:      resp.RateLimit.Cost,
			Remaining: resp.RateLimit.Remaining,
			ResetAt:   resp.RateLimit.ResetAt,
		},
	}, nil
}
