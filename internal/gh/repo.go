package gh

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// repoMetaQuery is what the detail rail's controls draw their choices from:
// labels, the people who can be assigned, and what a merge here may be made of.
//
// Branches are not here and belong nowhere near it. They are a search keyed by
// what somebody typed rather than a set fetched once, which is Branches in
// branch.go.
//
// The first hundred of each list, which is GitHub's own page cap. The detail
// asks for the same number, so the two sides of a picker are truncated at the
// same point and a pull request's own labels are all present to be checked. Past
// that the union in the screen is the backstop: a choice the picker cannot list
// is one it must not delete.
const repoMetaQuery = `
query RepoMeta($owner: String!, $name: String!) {
  rateLimit { limit cost remaining resetAt }
  repository(owner: $owner, name: $name) {
    labels(first: 100) { nodes { id name } }
    assignableUsers(first: 100) { nodes { id login } }
    mergeCommitAllowed
    squashMergeAllowed
    rebaseMergeAllowed
    deleteBranchOnMerge
  }
}`

type repoMetaResponse struct {
	RateLimit struct {
		Limit     int
		Cost      int
		Remaining int
		ResetAt   time.Time
	}

	Repository *struct {
		Labels struct {
			Nodes []struct{ ID, Name string }
		}
		AssignableUsers struct {
			Nodes []struct{ ID, Login string }
		}

		MergeCommitAllowed  bool
		SquashMergeAllowed  bool
		RebaseMergeAllowed  bool
		DeleteBranchOnMerge bool
	}
}

// RepoMeta fetches the choices every picker on the detail rail draws from.
// repo is "owner/name", which is what PullRequest.Repository carries.
//
// A repository the token cannot see comes back as a null node rather than an
// error, so an empty name is a failure here and not an empty picker: a picker
// offering nothing reads as a repository with no labels.
func (c *Client) RepoMeta(ctx context.Context, repo string) (RepoMetaResult, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" {
		return RepoMetaResult{}, fmt.Errorf("fetching repository metadata: %q is not owner/name", repo)
	}

	var resp repoMetaResponse
	vars := map[string]any{"owner": owner, "name": name}

	if err := c.gql.DoWithContext(ctx, repoMetaQuery, vars, &resp); err != nil {
		return RepoMetaResult{}, fmt.Errorf("fetching repository metadata: %w", classify(err))
	}
	if resp.Repository == nil {
		return RepoMetaResult{}, fmt.Errorf("fetching repository metadata: GitHub returned no repository %q", repo)
	}

	labels, users := resp.Repository.Labels.Nodes, resp.Repository.AssignableUsers.Nodes
	meta := RepoMeta{
		Labels: make([]Label, 0, len(labels)),
		Users:  make([]Actor, 0, len(users)),
		Methods: MergeMethods{
			Merge:         resp.Repository.MergeCommitAllowed,
			Squash:        resp.Repository.SquashMergeAllowed,
			Rebase:        resp.Repository.RebaseMergeAllowed,
			DeleteOnMerge: resp.Repository.DeleteBranchOnMerge,
		},
	}
	for _, n := range labels {
		meta.Labels = append(meta.Labels, Label{ID: n.ID, Name: n.Name})
	}
	for _, n := range users {
		meta.Users = append(meta.Users, Actor{ID: n.ID, Login: n.Login})
	}

	return RepoMetaResult{
		Meta: meta,
		RateLimit: RateLimit{
			Limit:     resp.RateLimit.Limit,
			Cost:      resp.RateLimit.Cost,
			Remaining: resp.RateLimit.Remaining,
			ResetAt:   resp.RateLimit.ResetAt,
		},
	}, nil
}
