package gh

import (
	"context"
	"fmt"
)

// updatePullRequest rather than the add and remove mutations: two calls leave half a set applied if one fails.
const setLabelsMutation = `
mutation SetLabels($pullRequestId: ID!, $labelIds: [ID!]!) {
  updatePullRequest(input: {pullRequestId: $pullRequestId, labelIds: $labelIds}) {
    pullRequest {
      id
      labels(first: 100) { nodes { id name } }
    }
  }
}`

type setLabelsResponse struct {
	UpdatePullRequest struct {
		PullRequest *struct {
			ID     string
			Labels struct {
				Nodes []struct{ ID, Name string }
			}
		}
	}
}

// SetLabels replaces a pull request's labels with the label node ids in labelIDs; an empty slice clears them.
func (c *Client) SetLabels(ctx context.Context, prID string, labelIDs []string) (LabelsResult, error) {
	if labelIDs == nil {
		labelIDs = []string{}
	}

	var resp setLabelsResponse
	vars := map[string]any{"pullRequestId": prID, "labelIds": labelIDs}

	if err := c.gql.DoWithContext(ctx, setLabelsMutation, vars, &resp); err != nil {
		return LabelsResult{}, fmt.Errorf("setting labels: %w", classify(err))
	}

	pr := resp.UpdatePullRequest.PullRequest
	if pr == nil || pr.ID == "" {
		return LabelsResult{}, fmt.Errorf("setting labels: GitHub returned no pull request")
	}

	out := make([]Label, 0, len(pr.Labels.Nodes))
	for _, n := range pr.Labels.Nodes {
		out = append(out, Label{ID: n.ID, Name: n.Name})
	}
	return LabelsResult{Labels: out}, nil
}
