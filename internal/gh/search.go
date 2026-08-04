package gh

import (
	"context"
	"fmt"
	"time"
)

const searchPullRequestsQuery = `
query SearchPullRequests($q: String!, $limit: Int!) {
  search(query: $q, type: ISSUE, first: $limit) {
    nodes {
      ... on PullRequest {
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
        author { login }
        repository { nameWithOwner }
        statusCheckRollup: commits(last: 1) {
          nodes { commit { statusCheckRollup { state } } }
        }
      }
    }
  }
}`

// searchPullRequestsResponse mirrors the query above. It stays unexported:
// callers get []PullRequest.
type searchPullRequestsResponse struct {
	Search struct {
		Nodes []struct {
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
	}
}

// SearchPullRequests runs a raw GitHub search query and returns the pull
// requests it matched. The query is whatever the user put in their config;
// this package does not interpret it.
func (c *Client) SearchPullRequests(ctx context.Context, query string, limit int) ([]PullRequest, error) {
	var resp searchPullRequestsResponse
	vars := map[string]any{"q": query, "limit": limit}

	if err := c.gql.DoWithContext(ctx, searchPullRequestsQuery, vars, &resp); err != nil {
		return nil, fmt.Errorf("searching pull requests (%s): %w", query, classify(err))
	}

	prs := make([]PullRequest, 0, len(resp.Search.Nodes))
	for _, n := range resp.Search.Nodes {
		// Search returns issues and pull requests in one connection. Anything
		// that isn't a PR comes back as an empty node from the inline fragment.
		if n.ID == "" {
			continue
		}

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
		prs = append(prs, pr)
	}
	return prs, nil
}
