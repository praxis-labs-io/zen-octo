package gh

import (
	"cmp"
	"context"
	"fmt"
)

const startReviewMutation = `
mutation StartReview($prId: ID!, $body: String!) {
  addPullRequestReview(input: {pullRequestId: $prId, body: $body}) {
    pullRequestReview {
      id
      state
      body
    }
  }
}`

// A thread with no review of its own still opens one: GitHub publishes no line comment standing alone.
const addReviewThreadMutation = `
mutation AddReviewThread(
  $prId: ID,
  $reviewId: ID,
  $path: String!,
  $body: String!,
  $subjectType: PullRequestReviewThreadSubjectType!,
  $line: Int,
  $side: DiffSide,
  $startLine: Int,
  $startSide: DiffSide
) {
  addPullRequestReviewThread(input: {
    pullRequestId: $prId
    pullRequestReviewId: $reviewId
    path: $path
    body: $body
    subjectType: $subjectType
    line: $line
    side: $side
    startLine: $startLine
    startSide: $startSide
  }) {
    thread {
      id
      isResolved
      isOutdated
      viewerCanReply
      viewerCanResolve
      viewerCanUnresolve
      path
      line
      startLine
      originalLine
      originalStartLine
      diffSide
      subjectType
      comments(first: 50) {
        totalCount
        nodes {
          id
          author { login }
          createdAt
          body
          state
          diffHunk
          viewerDidAuthor
          viewerCanUpdate
          viewerCanDelete
          viewerCanReact
          reactionGroups { content viewerHasReacted reactors { totalCount } }
          pullRequestReview { id }
        }
      }
    }
  }
}`

const submitReviewMutation = `
mutation SubmitReview($reviewId: ID!, $event: PullRequestReviewEvent!, $body: String) {
  submitPullRequestReview(input: {pullRequestReviewId: $reviewId, event: $event, body: $body}) {
    pullRequestReview {
      id
      state
      body
    }
  }
}`

const discardReviewMutation = `
mutation DiscardReview($reviewId: ID!) {
  deletePullRequestReview(input: {pullRequestReviewId: $reviewId}) {
    pullRequestReview {
      id
      state
    }
  }
}`

type reviewNode struct {
	ID    string
	State string
	Body  string
}

func (n reviewNode) review() Review {
	return Review{ID: n.ID, State: ReviewState(n.State), Body: n.Body}
}

type startReviewResponse struct {
	AddPullRequestReview struct{ PullRequestReview reviewNode }
}

type addReviewThreadResponse struct {
	AddPullRequestReviewThread struct{ Thread reviewThreadNode }
}

type submitReviewResponse struct {
	SubmitPullRequestReview struct{ PullRequestReview reviewNode }
}

type discardReviewResponse struct {
	DeletePullRequestReview struct{ PullRequestReview reviewNode }
}

// StartReview opens a pending review holding body as its summary. Nobody but the viewer can see it
// until it is submitted, and GitHub allows only one open at a time per pull request.
func (c *Client) StartReview(ctx context.Context, prID, body string) (ReviewResult, error) {
	var resp startReviewResponse
	vars := map[string]any{"prId": prID, "body": body}

	if err := c.gql.DoWithContext(ctx, startReviewMutation, vars, &resp); err != nil {
		return ReviewResult{}, fmt.Errorf("starting a review: %w", classify(err))
	}

	node := resp.AddPullRequestReview.PullRequestReview
	if node.ID == "" {
		return ReviewResult{}, fmt.Errorf("starting a review: GitHub returned no review")
	}

	return ReviewResult{Review: node.review()}, nil
}

// AddReviewThread opens a thread on a line, a range, or a whole file. With no ReviewID it opens a
// pending review to hold the thread, whose id comes back on the thread.
func (c *Client) AddReviewThread(ctx context.Context, in ReviewThreadInput) (ReviewThreadResult, error) {
	if in.PullRequestID == "" && in.ReviewID == "" {
		return ReviewThreadResult{}, fmt.Errorf("opening a review thread: no pull request or review to open it on")
	}

	var resp addReviewThreadResponse
	if err := c.gql.DoWithContext(ctx, addReviewThreadMutation, in.vars(), &resp); err != nil {
		return ReviewThreadResult{}, fmt.Errorf("opening a review thread: %w", classify(err))
	}

	node := resp.AddPullRequestReviewThread.Thread
	if node.ID == "" {
		return ReviewThreadResult{}, fmt.Errorf("opening a review thread: GitHub returned no thread")
	}

	return ReviewThreadResult{Thread: node.reviewThread()}, nil
}

// A file thread sends no line at all: GitHub refuses one beside subjectType FILE.
func (in ReviewThreadInput) vars() map[string]any {
	subject := cmp.Or(in.Subject, SubjectLine)
	vars := map[string]any{
		"prId":        nullable(in.PullRequestID),
		"reviewId":    nullable(in.ReviewID),
		"path":        in.Path,
		"body":        in.Body,
		"subjectType": string(subject),
		"line":        nil,
		"side":        nil,
		"startLine":   nil,
		"startSide":   nil,
	}
	if subject == SubjectFile {
		return vars
	}

	vars["line"] = in.Line
	vars["side"] = string(cmp.Or(in.Side, SideRight))
	if in.StartLine != 0 && in.StartLine != in.Line {
		vars["startLine"] = in.StartLine
		vars["startSide"] = string(cmp.Or(in.StartSide, in.Side, SideRight))
	}
	return vars
}

// SubmitReview publishes a pending review and every thread held in it, replacing its summary with body.
func (c *Client) SubmitReview(ctx context.Context, reviewID string, event ReviewEvent, body string) (ReviewResult, error) {
	var resp submitReviewResponse
	vars := map[string]any{"reviewId": reviewID, "event": string(event), "body": body}

	if err := c.gql.DoWithContext(ctx, submitReviewMutation, vars, &resp); err != nil {
		return ReviewResult{}, fmt.Errorf("submitting a review: %w", classify(err))
	}

	node := resp.SubmitPullRequestReview.PullRequestReview
	if node.ID == "" {
		return ReviewResult{}, fmt.Errorf("submitting a review: GitHub returned no review")
	}

	return ReviewResult{Review: node.review()}, nil
}

// DiscardReview throws away a pending review and every thread held in it.
func (c *Client) DiscardReview(ctx context.Context, reviewID string) error {
	var resp discardReviewResponse
	vars := map[string]any{"reviewId": reviewID}

	if err := c.gql.DoWithContext(ctx, discardReviewMutation, vars, &resp); err != nil {
		return fmt.Errorf("discarding a review: %w", classify(err))
	}
	if resp.DeletePullRequestReview.PullRequestReview.ID == "" {
		return fmt.Errorf("discarding a review: GitHub returned no review")
	}
	return nil
}

// A zero string has to reach GitHub as null: an empty ID is a node it cannot resolve.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
