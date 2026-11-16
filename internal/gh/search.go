package gh

import (
	"context"
	"fmt"
	"time"
)

// The fields a list row reads, shared by search and the lookup by number so the two cannot drift.
const pullRequestRowFields = `
        id
        number
        title
        url
        isDraft
        state
        createdAt
        updatedAt
        additions
        deletions
        changedFiles
        headRefName
        baseRefName
        reviewDecision
        comments { totalCount }
        reviewThreads { totalCount }
        author { login }
        repository { nameWithOwner }
        statusCheckRollup: commits(last: 1) {
          nodes { commit { statusCheckRollup { state } } }
        }`

const searchPullRequestsQuery = `
query SearchPullRequests($q: String!, $limit: Int!) {
  rateLimit { limit cost remaining resetAt }
  search(query: $q, type: ISSUE, first: $limit) {
    nodes {
      ... on PullRequest {` + pullRequestRowFields + `
      }
    }
  }
}`

type pullRequestRow struct {
	ID           string
	Number       int
	Title        string
	URL          string
	IsDraft      bool
	State        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Additions    int
	Deletions    int
	ChangedFiles int
	HeadRefName  string
	BaseRefName  string

	ReviewDecision string
	Comments       struct{ TotalCount int }
	ReviewThreads  struct{ TotalCount int }
	Author         *struct{ Login string }
	Repository     struct{ NameWithOwner string }

	StatusCheckRollup struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct{ State string }
			}
		}
	}
}

func (n pullRequestRow) pullRequest() PullRequest {
	pr := PullRequest{
		ID:             n.ID,
		Number:         n.Number,
		Title:          n.Title,
		URL:            n.URL,
		Repository:     n.Repository.NameWithOwner,
		State:          PRState(n.State),
		IsDraft:        n.IsDraft,
		HeadRefName:    n.HeadRefName,
		BaseRefName:    n.BaseRefName,
		Additions:      n.Additions,
		Deletions:      n.Deletions,
		ChangedFiles:   n.ChangedFiles,
		Comments:       n.Comments.TotalCount + n.ReviewThreads.TotalCount,
		ReviewDecision: ReviewDecision(n.ReviewDecision),
		CreatedAt:      n.CreatedAt,
		UpdatedAt:      n.UpdatedAt,
	}
	if n.Author != nil {
		pr.Author = Actor{Login: n.Author.Login}
	}
	if nodes := n.StatusCheckRollup.Nodes; len(nodes) > 0 {
		if rollup := nodes[0].Commit.StatusCheckRollup; rollup != nil {
			pr.Checks = CheckState(rollup.State)
		}
	}
	return pr
}

type searchPullRequestsResponse struct {
	RateLimit struct {
		Limit     int
		Cost      int
		Remaining int
		ResetAt   time.Time
	}

	Search struct {
		Nodes []pullRequestRow
	}
}

func (c *Client) SearchPullRequests(ctx context.Context, query string, limit int) (SearchResult, error) {
	var resp searchPullRequestsResponse
	vars := map[string]any{"q": query, "limit": limit}

	if err := c.gql.DoWithContext(ctx, searchPullRequestsQuery, vars, &resp); err != nil {
		return SearchResult{}, fmt.Errorf("searching pull requests (%s): %w", query, classify(err))
	}

	prs := make([]PullRequest, 0, len(resp.Search.Nodes))
	for _, n := range resp.Search.Nodes {
		if n.ID == "" {
			continue
		}
		prs = append(prs, n.pullRequest())
	}

	return SearchResult{
		PullRequests: prs,
		RateLimit: RateLimit{
			Limit:     resp.RateLimit.Limit,
			Cost:      resp.RateLimit.Cost,
			Remaining: resp.RateLimit.Remaining,
			ResetAt:   resp.RateLimit.ResetAt,
		},
	}, nil
}
