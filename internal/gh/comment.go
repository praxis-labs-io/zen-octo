package gh

import (
	"context"
	"fmt"
	"time"
)

const addCommentMutation = `
mutation AddComment($subjectId: ID!, $body: String!) {
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
        reactionGroups { content viewerHasReacted reactors { totalCount } }
      }
    }
  }
}`

type addCommentResponse struct {
	AddComment struct {
		CommentEdge struct {
			Node struct {
				commentNode
				CreatedAt time.Time
			}
		}
	}
}

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

	return CommentResult{Comment: node.comment(CommentIssue, node.CreatedAt)}, nil
}
