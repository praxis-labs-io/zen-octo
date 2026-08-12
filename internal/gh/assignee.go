package gh

import (
	"context"
	"fmt"
)

// setAssigneesMutation replaces a pull request's assignees with the set it is
// handed.
//
// updatePullRequest rather than addAssigneesToAssignable and
// removeAssigneesFromAssignable, which are the two mutations the name suggests,
// for the reason setLabelsMutation gives: the picker applies a whole set, and a
// set is one call here against two there.
//
// It asks the assignees back because the caller is holding an optimistic list
// and GitHub is the only authority on what the pull request actually now
// carries. Ids as well as logins, so the answer can be folded straight back
// into the next picker without a lookup.
//
// A hundred of them, which is GitHub's own page cap. The detail asks for ten,
// which is every assignee any pull request has in practice; the wider page here
// costs nothing and cannot under-report what the write kept.
//
// It asks for no rateLimit. That field is on Query and nowhere else, and a
// mutation selecting it is rejected whole.
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

// SetAssignees replaces the assignees on a pull request and returns the set as
// GitHub recorded it. prID is the pull request's node id; assigneeIDs are user
// node ids, which is the only spelling the mutation takes.
//
// An empty slice clears every assignee, and that is a real call rather than a
// no-op the caller should skip: unchecking the last one in the picker is how a
// reader removes it.
func (c *Client) SetAssignees(ctx context.Context, prID string, assigneeIDs []string) (AssigneesResult, error) {
	// A nil slice marshals to null, which the [ID!]! non-null type rejects. An
	// empty set has to go over the wire as [].
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

	// Not nil: an empty result is the answer when everyone was unchecked, and
	// the caller folds what it is handed over the list it is showing.
	out := make([]Actor, 0, len(pr.Assignees.Nodes))
	for _, n := range pr.Assignees.Nodes {
		out = append(out, Actor{ID: n.ID, Login: n.Login})
	}
	return AssigneesResult{Assignees: out}, nil
}
