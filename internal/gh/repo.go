package gh

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// repoMetaQuery is what a picker on the detail rail draws its choices from.
//
// Labels alone. The assignable users, the branches and the merge flags belong
// to pickers that do not exist yet, and asking for them now spends rate-limit
// points on every open for lists nothing renders. Each lands with the ticket
// that reads it.
//
// The first hundred, which is GitHub's own page cap. The detail asks for the
// same number, so the two sides of the picker are truncated at the same point
// and a pull request's own labels are all present to be checked. Past that the
// union in the screen is the backstop: a label the picker cannot list is one it
// must not delete.
const repoMetaQuery = `
query RepoMeta($owner: String!, $name: String!) {
  rateLimit { limit cost remaining resetAt }
  repository(owner: $owner, name: $name) {
    labels(first: 100) { nodes { id name } }
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

	nodes := resp.Repository.Labels.Nodes
	meta := RepoMeta{Labels: make([]Label, 0, len(nodes))}
	for _, n := range nodes {
		meta.Labels = append(meta.Labels, Label{ID: n.ID, Name: n.Name})
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
