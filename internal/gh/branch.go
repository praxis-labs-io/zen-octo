package gh

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

// branchPage is how many branches one search returns.
//
// Thirty rather than GitHub's hundred because the list comes back newest first
// and is answered at its top. A branch further down than that is reached by
// typing rather than by scrolling: the picker is thirty columns wide and shows
// ten rows at a time, so a longer page buys a scrollbar over a list the reader
// is already filtering.
const branchPage = 30

// branchQuery is the repository's branches matching a substring of their name.
//
// query is what somebody typed into the picker. GitHub matches it as a
// case-insensitive substring anywhere in the name rather than as a prefix, so
// "notebook" finds "aamunger/notebookSmokeTests"; that is the same rule
// comp.Picker filters by, which is what lets the two compose instead of
// disagreeing. An empty string matches every branch.
//
// The commit date comes back per ref because the sort happens here. refs takes
// an orderBy and ignores it on refs/heads: asking for TAG_COMMIT_DATE answers
// 200 with the names in neither commit order nor a stable one, and ALPHABETICAL
// is the only sort the connection honours. Alphabetical is a list where the
// branch somebody wants is wherever its name happens to fall, which on a
// repository with four thousand branches is a first page of nothing.
//
// totalCount is the whole match, not the page, so it is what the overflow is
// measured against.
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

// branchNode is one ref and the date the sort reads.
//
// Target is nullable and carries a date only on a Commit. A branch always
// points at one, but the field is on the union rather than on Ref, so nothing
// here can assume it arrived.
type branchNode struct {
	Name   string
	Target *struct{ CommittedDate time.Time }
}

// committed is when the branch last moved, and the zero time when GitHub sent
// no date. A ref with no date sorts last rather than being dropped: an
// unreachable branch is worse than an oddly placed one.
func (n branchNode) committed() time.Time {
	if n.Target == nil {
		return time.Time{}
	}
	return n.Target.CommittedDate
}

// Branches searches a repository's branches, newest commit first. repo is
// "owner/name", which is what PullRequest.Repository carries; query is a
// substring of the branch name, empty for all of them.
//
// A repository the token cannot see comes back as a null node rather than an
// error, so that is a failure here and not an empty result: a picker offering
// nothing reads as a repository with no branches.
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

	// Sorted before the names are taken: the date is what the order is for and
	// it does not survive the copy. Stable, so two branches last touched in the
	// same commit keep the alphabetical order GitHub sent them in.
	slices.SortStableFunc(nodes, func(a, b branchNode) int {
		return b.committed().Compare(a.committed())
	})
	for _, n := range nodes {
		out.Branches = append(out.Branches, n.Name)
	}
	return out, nil
}
