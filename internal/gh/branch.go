package gh

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Sized to scan rather than page: GitHub pages alphabetically, so narrowing the search is what reaches
// the rest.
const branchPage = 30

// refs ignores orderBy on refs/heads, so the page comes back alphabetical and the date sort can only
// order it.
const branchQuery = `
query Branches($owner: String!, $name: String!, $query: String!, $first: Int!) {
  rateLimit { limit cost remaining resetAt }
  repository(owner: $owner, name: $name) {
    defaultBranchRef { name }
    refs(refPrefix: "refs/heads/", query: $query, first: $first) {
      totalCount
      nodes {
        name
        target { ... on Commit { committedDate } }
      }
    }
  }
}`

type branchResponse struct {
	RateLimit struct {
		Limit     int
		Cost      int
		Remaining int
		ResetAt   time.Time
	}

	Repository *struct {
		DefaultBranchRef *struct{ Name string }

		Refs struct {
			TotalCount int
			Nodes      []branchNode
		}
	}
}

type branchNode struct {
	Name   string
	Target *struct{ CommittedDate time.Time }
}

func (n branchNode) committed() time.Time {
	if n.Target == nil {
		return time.Time{}
	}
	return n.Target.CommittedDate
}

// Branches returns one page of repo's branches whose name contains query, newest commit first, or all
// of them for an empty query. repo is "owner/name"; More counts matches past the page. A repository
// the token cannot see is an error.
func (c *Client) Branches(ctx context.Context, repo, query string) (BranchResult, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" {
		return BranchResult{}, fmt.Errorf("fetching branches: %q is not owner/name", repo)
	}

	var resp branchResponse
	vars := map[string]any{"owner": owner, "name": name, "query": query, "first": branchPage}

	if err := c.gql.DoWithContext(ctx, branchQuery, vars, &resp); err != nil {
		return BranchResult{}, fmt.Errorf("fetching branches: %w", classify(err))
	}
	if resp.Repository == nil {
		return BranchResult{}, fmt.Errorf("fetching branches: GitHub returned no repository %q", repo)
	}

	nodes := resp.Repository.Refs.Nodes
	out := BranchResult{
		Query:    query,
		Branches: make([]string, 0, len(nodes)),
		More:     max(0, resp.Repository.Refs.TotalCount-len(nodes)),
		RateLimit: RateLimit{
			Limit:     resp.RateLimit.Limit,
			Cost:      resp.RateLimit.Cost,
			Remaining: resp.RateLimit.Remaining,
			ResetAt:   resp.RateLimit.ResetAt,
		},
	}
	if ref := resp.Repository.DefaultBranchRef; ref != nil {
		out.Default = ref.Name
	}

	slices.SortStableFunc(nodes, func(a, b branchNode) int {
		return b.committed().Compare(a.committed())
	})
	for _, n := range nodes {
		out.Branches = append(out.Branches, n.Name)
	}
	return out, nil
}
