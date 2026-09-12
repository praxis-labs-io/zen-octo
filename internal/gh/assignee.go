package gh

import (
	"context"
	"fmt"
)

// updatePullRequest rather than the add and remove mutations: one call applies a whole set.
const setAssigneesMutation = `
mutation SetAssignees($pullRequestId: ID!, $assigneeIds: [ID!]!) {
  updatePullRequest(input: {pullRequestId: $pullRequestId, assigneeIds: $assigneeIds}) {
    pullRequest {
      id
      assignees(first: 100) { nodes { id login } }
    }
  }
}`

type setAssigneesResponse struct {
	UpdatePullRequest struct {
		PullRequest *struct {
			ID        string
			Assignees struct {
				Nodes []struct{ ID, Login string }
			}
		}
	}
}

// SetAssignees replaces a pull request's assignees with assigneeIDs, user node ids, and returns the set
// GitHub recorded. An empty slice clears them all.
func (c *Client) SetAssignees(ctx context.Context, prID string, assigneeIDs []string) (AssigneesResult, error) {
	if assigneeIDs == nil {
		assigneeIDs = []string{}
	}

	var resp setAssigneesResponse
	vars := map[string]any{"pullRequestId": prID, "assigneeIds": assigneeIDs}

	if err := c.gql.DoWithContext(ctx, setAssigneesMutation, vars, &resp); err != nil {
		return AssigneesResult{}, fmt.Errorf("setting assignees: %w", classify(err))
	}

	pr := resp.UpdatePullRequest.PullRequest
	if pr == nil || pr.ID == "" {
		return AssigneesResult{}, fmt.Errorf("setting assignees: GitHub returned no pull request")
	}

	out := make([]Actor, 0, len(pr.Assignees.Nodes))
	for _, n := range pr.Assignees.Nodes {
		out = append(out, Actor{ID: n.ID, Login: n.Login})
	}
	return AssigneesResult{Assignees: out}, nil
}
