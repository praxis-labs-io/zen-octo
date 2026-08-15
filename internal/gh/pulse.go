package gh

import (
	"context"
	"fmt"
	"time"
)

// pulseQuery re-asks the fields that move without anyone here touching them.
// A hundred and one nodes against the detail query's five thousand six hundred.
const pulseQuery = `
query PullRequestPulse($id: ID!) {
  rateLimit { limit cost remaining resetAt }
  node(id: $id) {
    ... on PullRequest {
      id
      state
      isDraft
      reviewDecision
      mergeable
      mergeStateStatus
      updatedAt
      headRefOid
` + rollupSelection + `
    }
  }
}`

type pulseResponse struct {
	RateLimit struct {
		Limit     int
		Cost      int
		Remaining int
		ResetAt   time.Time
	}

	Node struct {
		ID               string
		State            string
		IsDraft          bool
		ReviewDecision   string
		Mergeable        string
		MergeStateStatus string
		UpdatedAt        time.Time
		HeadRefOid       string

		StatusCheckRollup rollupNode
	}
}

// Pulse re-reads one pull request's lifecycle, review decision, mergeability
// and checks. It asks for no comparison, so a gone head branch is no error here.
func (c *Client) Pulse(ctx context.Context, id string) (PulseResult, error) {
	var resp pulseResponse
	if err := c.gql.DoWithContext(ctx, pulseQuery, map[string]any{"id": id}, &resp); err != nil {
		return PulseResult{}, fmt.Errorf("checking pull request (%s): %w", id, classify(err))
	}

	n := resp.Node
	if n.ID == "" {
		return PulseResult{}, fmt.Errorf("checking pull request (%s): no pull request behind that id", id)
	}

	return PulseResult{
		Pulse: Pulse{
			State:          PRState(n.State),
			IsDraft:        n.IsDraft,
			ReviewDecision: ReviewDecision(n.ReviewDecision),
			Merge:          mergeState(n.Mergeable, n.MergeStateStatus),
			Rollup:         rollup(n.StatusCheckRollup),
			UpdatedAt:      n.UpdatedAt,
			HeadRefOid:     n.HeadRefOid,
		},
		RateLimit: RateLimit{
			Limit:     resp.RateLimit.Limit,
			Cost:      resp.RateLimit.Cost,
			Remaining: resp.RateLimit.Remaining,
			ResetAt:   resp.RateLimit.ResetAt,
		},
	}, nil
}
