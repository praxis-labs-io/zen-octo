package gh

import (
	"context"
	"fmt"
	"time"
)

// One comment type up here, three mutations down here. A node id does not name
// the call that rewrites it, so the kind picks the document the way the
// direction picks one in SetThreadResolved.
//
// The three inputs disagree on what the id is called, which is the whole reason
// they cannot share one: updateIssueComment takes id, the review comment takes
// pullRequestReviewCommentId, and the review takes pullRequestReviewId.
//
// Each asks the comment back, the same fields addComment does, so a comment
// just edited and one fetched an hour later are the same shape. None asks for
// rateLimit: that field is on Query alone and a mutation naming it is rejected
// whole.
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

// A review's own words. createdAt is when the review was submitted, which is
// what the conversation already dates the card by.
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

// editedNode is the comment either half of a payload carries.
type editedNode struct {
	commentNode
	CreatedAt time.Time
}

// updateCommentResponse decodes all three payloads. The method reads the one
// its own document produced rather than whichever field came back filled, so a
// response landing in the wrong one is a failure instead of a silent pass.
type updateCommentResponse struct {
	UpdateIssueComment             struct{ IssueComment editedNode }
	UpdatePullRequestReviewComment struct {
		PullRequestReviewComment editedNode
	}
	UpdatePullRequestReview struct{ PullRequestReview editedNode }
}

// UpdateComment rewrites a comment's body and returns it as GitHub recorded it.
// id is the comment's node id, and kind is what it is: an issue comment, a
// review's own body, or a comment inside a review thread.
//
// An unknown kind is an error rather than a guess. Sending the wrong document
// reaches a mutation that will refuse the id, and the refusal would arrive as
// GitHub's words about a call this side chose.
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

// Neither delete has anything worth reading back. A payload has to select
// something, and deleteIssueComment carries nothing but clientMutationId: the
// comment it removed is gone, so there is no node to ask for. The error is the
// whole of the answer either way.
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

// DeleteComment removes a comment. id is its node id, and kind is what it is.
//
// A review's own body has no delete. deletePullRequestReview takes only a
// pending review, GitHub's own page offers no control for a submitted one, and
// viewerCanDelete comes back true on one regardless, which is why this refuses
// here rather than trusting the flag. Nothing above offers the key on a review,
// so reaching this is a bug rather than a reader's press.
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
