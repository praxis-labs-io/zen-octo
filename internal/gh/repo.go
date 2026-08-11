package gh

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// repoMetaQuery is everything a picker needs to offer a choice, in one round
// trip. Four lists that change on the scale of days, asked for together because
// a reader opening the labels picker will open the assignees one next and a
// second wait buys nothing.
//
// The first hundred of each. GitHub caps a connection page at a hundred and a
// repository past that in any of these is one where filtering by hand has
// already stopped working; paging it would spend three more round trips to
// serve a case the picker's filter row serves better.
const repoMetaQuery = `
query RepoMeta($owner: String!, $name: String!) {
  rateLimit { limit cost remaining resetAt }
  repository(owner: $owner, name: $name) {
    mergeCommitAllowed
    squashMergeAllowed
    rebaseMergeAllowed
    deleteBranchOnMerge
    assignableUsers(first: 100) { nodes { login } }
    labels(first: 100) { nodes { id name } }
    refs(refPrefix: "refs/heads/", first: 100, orderBy: {field: ALPHABETICAL, direction: ASC}) {
      nodes { name }
    }
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
		MergeCommitAllowed  bool
		SquashMergeAllowed  bool
		RebaseMergeAllowed  bool
		DeleteBranchOnMerge bool

		AssignableUsers struct {
			Nodes []struct{ Login string }
		}
		Labels struct {
			Nodes []struct{ ID, Name string }
		}
		Refs struct {
			Nodes []struct{ Name string }
		}
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

	r := resp.Repository
	meta := RepoMeta{
		Merge: MergeMethods{
			Merge:        r.MergeCommitAllowed,
			Squash:       r.SquashMergeAllowed,
			Rebase:       r.RebaseMergeAllowed,
			DeleteBranch: r.DeleteBranchOnMerge,
		},
		Assignable: make([]Actor, 0, len(r.AssignableUsers.Nodes)),
		Labels:     make([]Label, 0, len(r.Labels.Nodes)),
		Branches:   make([]string, 0, len(r.Refs.Nodes)),
	}
	for _, n := range r.AssignableUsers.Nodes {
		meta.Assignable = append(meta.Assignable, Actor{Login: n.Login})
	}
	for _, n := range r.Labels.Nodes {
		meta.Labels = append(meta.Labels, Label{ID: n.ID, Name: n.Name})
	}
	for _, n := range r.Refs.Nodes {
		meta.Branches = append(meta.Branches, n.Name)
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
