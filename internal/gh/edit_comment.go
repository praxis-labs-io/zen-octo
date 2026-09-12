package gh

import (
	"context"
	"fmt"
	"time"
)

const updateIssueCommentMutation = `
mutation UpdateIssueComment($id: ID!, $body: String!) {
  updateIssueComment(input: {id: $id, body: $body}) {
    issueComment {
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

const updateReviewCommentMutation = `
mutation UpdateReviewComment($id: ID!, $body: String!) {
  updatePullRequestReviewComment(input: {pullRequestReviewCommentId: $id, body: $body}) {
    pullRequestReviewComment {
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

const updateReviewMutation = `
mutation UpdateReview($id: ID!, $body: String!) {
  updatePullRequestReview(input: {pullRequestReviewId: $id, body: $body}) {
    pullRequestReview {
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

type editedNode struct {
	commentNode
	CreatedAt time.Time
}

// Read by kind rather than by whichever field came back filled, so a mismatched response fails.
type updateCommentResponse struct {
	UpdateIssueComment             struct{ IssueComment editedNode }
	UpdatePullRequestReviewComment struct {
		PullRequestReviewComment editedNode
	}
	UpdatePullRequestReview struct{ PullRequestReview editedNode }
}

// UpdateComment rewrites the body of the comment with node id id and returns it as GitHub recorded it.
// An unknown kind is an error.
func (c *Client) UpdateComment(ctx context.Context, kind CommentKind, id, body string) (CommentResult, error) {
	doc, doing := "", "editing a comment"
	switch kind {
	case CommentIssue:
		doc = updateIssueCommentMutation
	case CommentThread:
		doc, doing = updateReviewCommentMutation, "editing a review comment"
	case CommentReview:
		doc, doing = updateReviewMutation, "editing a review"
	default:
		return CommentResult{}, fmt.Errorf("editing a comment: no mutation for a %q comment", kind)
	}

	var resp updateCommentResponse
	vars := map[string]any{"id": id, "body": body}

	if err := c.gql.DoWithContext(ctx, doc, vars, &resp); err != nil {
		return CommentResult{}, fmt.Errorf("%s: %w", doing, classify(err))
	}

	var node editedNode
	switch kind {
	case CommentIssue:
		node = resp.UpdateIssueComment.IssueComment
	case CommentThread:
		node = resp.UpdatePullRequestReviewComment.PullRequestReviewComment
	case CommentReview:
		node = resp.UpdatePullRequestReview.PullRequestReview
	}
	if node.ID == "" {
		return CommentResult{}, fmt.Errorf("%s: GitHub returned no comment", doing)
	}

	return CommentResult{Comment: node.comment(kind, node.CreatedAt)}, nil
}

// clientMutationId is all the payload has left once the comment is gone.
const deleteIssueCommentMutation = `
mutation DeleteIssueComment($id: ID!) {
  deleteIssueComment(input: {id: $id}) {
    clientMutationId
  }
}`

const deleteReviewCommentMutation = `
mutation DeleteReviewComment($id: ID!) {
  deletePullRequestReviewComment(input: {id: $id}) {
    clientMutationId
  }
}`

// DeleteComment removes an issue or thread comment by node id. A CommentReview is refused: GitHub cannot
// delete a submitted review, though its viewerCanDelete says otherwise.
func (c *Client) DeleteComment(ctx context.Context, kind CommentKind, id string) error {
	doc, doing := "", "deleting a comment"
	switch kind {
	case CommentIssue:
		doc = deleteIssueCommentMutation
	case CommentThread:
		doc, doing = deleteReviewCommentMutation, "deleting a review comment"
	default:
		return fmt.Errorf("deleting a comment: a %q comment cannot be deleted", kind)
	}

	var resp struct{}
	if err := c.gql.DoWithContext(ctx, doc, map[string]any{"id": id}, &resp); err != nil {
		return fmt.Errorf("%s: %w", doing, classify(err))
	}
	return nil
}
