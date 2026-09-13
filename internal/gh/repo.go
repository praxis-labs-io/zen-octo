package gh

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Pages of 100 to match the detail query, so a pull request's own labels are all present to check.
const repoMetaQuery = `
query RepoMeta($owner: String!, $name: String!) {
  rateLimit { limit cost remaining resetAt }
  repository(owner: $owner, name: $name) {
    labels(first: 100) { nodes { id name } }
    assignableUsers(first: 100) { nodes { id login } }
    mentionableUsers(first: 100) { nodes { login name } }
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
		MentionableUsers struct {
			Nodes []struct{ Login, Name string }
		}

		MergeCommitAllowed  bool
		SquashMergeAllowed  bool
		RebaseMergeAllowed  bool
		DeleteBranchOnMerge bool
	}
}

// RepoMeta fetches the picker choices for repo, "owner/name".
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
	mentions := resp.Repository.MentionableUsers.Nodes
	meta := RepoMeta{
		Labels:   make([]Label, 0, len(labels)),
		Users:    make([]Actor, 0, len(users)),
		Mentions: make([]Mention, 0, len(mentions)),
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

	for _, n := range mentions {
		meta.Mentions = append(meta.Mentions, Mention{Login: n.Login, Name: n.Name})
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
