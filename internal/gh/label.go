package gh

import (
	"context"
	"fmt"
)

// setLabelsMutation replaces a pull request's labels with the set it is handed.
//
// updatePullRequest rather than addLabelsToLabelable and
// removeLabelsFromLabelable, which are the two mutations the name suggests. The
// picker applies a whole set, and a set is one call here against two there: two
// would leave the pull request holding half the change when the second fails,
// and there is no order to run them in that makes that safe.
//
// It asks the labels back for the same reason addComment asks for the comment.
// The caller is holding an optimistic list and GitHub is the only authority on
// what it actually now carries.
//
// It asks for no rateLimit. That field is on Query and nowhere else, and a
// mutation selecting it is rejected whole.
const setLabelsMutation = `
mutation SetLabels($pullRequestId: ID!, $labelIds: [ID!]!) {
  updatePullRequest(input: {pullRequestId: $pullRequestId, labelIds: $labelIds}) {
    pullRequest {
      id
      labels(first: 20) { nodes { id name } }
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

// SetLabels replaces the labels on a pull request and returns the set as GitHub
// recorded it. prID is the pull request's node id; labelIDs are label node ids,
// which is the only spelling the mutation takes.
//
// An empty slice clears every label, and that is a real call rather than a
// no-op the caller should skip: unchecking the last label in the picker is how
// a reader removes it.
func (c *Client) SetLabels(ctx context.Context, prID string, labelIDs []string) (LabelsResult, error) {
	// A nil slice marshals to null, which the [ID!]! non-null type rejects. An
	// empty set has to go over the wire as [].
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

	// Not nil: an empty result is the answer when every label was unchecked,
	// and the caller folds what it is handed over the list it is showing.
	out := make([]Label, 0, len(pr.Labels.Nodes))
	for _, n := range pr.Labels.Nodes {
		out = append(out, Label{ID: n.ID, Name: n.Name})
	}
	return LabelsResult{Labels: out}, nil
}
