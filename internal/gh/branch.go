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
// The page is alphabetical, because that is the only order GitHub will apply,
// and a bigger one does not fix that: taking a hundred of vscode's four
// thousand still takes them from the front of the alphabet. What reaches a
// branch outside the page is narrowing the search, never scrolling, so the page
// is sized to be scanned rather than paged through. The picker shows ten rows
// at a time.
const branchPage = 30

// branchQuery is the repository's branches matching a substring of their name.
//
// query is what somebody typed into the picker. GitHub matches it as a
// case-insensitive substring anywhere in the name rather than as a prefix, so
// "notebook" finds "aamunger/notebookSmokeTests"; that is the same rule
// comp.Picker filters by, which is what lets the two compose instead of
// disagreeing. An empty string matches every branch.
//
// The commit date comes back per ref because the sort happens here, and it is
// worth knowing exactly how much that sort buys. refs takes an orderBy and
// ignores it on refs/heads: asking for TAG_COMMIT_DATE answers 200 with the
// names in neither commit order nor a stable one, so ALPHABETICAL is the only
// order the connection will apply. GitHub then pages before this client sees
// anything, which means the sort below orders the page and cannot choose it.
//
// So on a repository whose matches fit in one page the list really is newest
// first; past that it is the front of the alphabet, newest first among itself.
// Ordering the whole of vscode by date would cost a request per hundred
// branches, forty-eight of them, to fill a modal showing ten rows. Narrowing
// the search is what reaches the rest, and the note beside the title is what
// says so.
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

// Branches searches a repository's branches. repo is "owner/name", which is
// what PullRequest.Repository carries; query is a substring of the branch name,
// empty for all of them.
//
// One page, newest commit first within it. GitHub pages alphabetically before
// this sees anything, so the order is the whole truth only when the match fits
// in a page; More is what says it did not.
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
