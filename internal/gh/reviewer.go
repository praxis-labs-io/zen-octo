package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const (
	// CopilotLogin is the review bot's login as GraphQL reports it, the spelling everything above this
	// package uses.
	CopilotLogin = "copilot-pull-request-reviewer"

	// POST takes only this form; given "Copilot" it answers 200 and writes nothing.
	copilotPostLogin = "copilot-pull-request-reviewer[bot]"
	// DELETE takes only this form; the [bot] form resolves to a Bot and 422s for not being a User.
	copilotDeleteLogin = "Copilot"
)

func postLogin(login string) string {
	if login == CopilotLogin {
		return copilotPostLogin
	}
	return login
}

func deleteLogin(login string) string {
	if login == CopilotLogin {
		return copilotDeleteLogin
	}
	return login
}

// GitHub reports a login in the account's own case, not the case it was asked in.
func loginKey(login string) string {
	for _, alias := range []string{CopilotLogin, copilotPostLogin, copilotDeleteLogin} {
		if strings.EqualFold(login, alias) {
			return CopilotLogin
		}
	}
	return strings.ToLower(login)
}

// No teams array: the picker offers users alone, so a team request is never cancelled either.
type reviewersRequest struct {
	Reviewers []string `json:"reviewers"`
}

// The POST response never lists the bot, landed or not, so this read is the only confirmation.
const reviewRequestsQuery = `
query ReviewRequests($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      reviewRequests(first: 100) {
        nodes {
          requestedReviewer {
            ... on User { login }
            ... on Bot { login }
          }
        }
      }
    }
  }
}`

type reviewRequestsResponse struct {
	Repository *struct {
		PullRequest *struct {
			ReviewRequests struct {
				Nodes []struct {
					RequestedReviewer *struct{ Login string }
				}
			}
		}
	}
}

func (c *Client) awaitingReview(ctx context.Context, owner, name string, number int) (map[string]bool, error) {
	var resp reviewRequestsResponse
	vars := map[string]any{"owner": owner, "name": name, "number": number}

	if err := c.gql.DoWithContext(ctx, reviewRequestsQuery, vars, &resp); err != nil {
		return nil, classify(err)
	}
	if resp.Repository == nil || resp.Repository.PullRequest == nil {
		return nil, fmt.Errorf("GitHub returned no pull request %s/%s#%d", owner, name, number)
	}

	out := make(map[string]bool)
	for _, n := range resp.Repository.PullRequest.ReviewRequests.Nodes {
		if n.RequestedReviewer != nil && n.RequestedReviewer.Login != "" {
			out[loginKey(n.RequestedReviewer.Login)] = true
		}
	}
	return out, nil
}

// RequestReviews asks for a review from each login and errors unless GitHub then lists every one as
// requested. REST because GraphQL requestReviews accepts a bot id, reports success, and requests
// nothing. An empty slice makes no call.
func (c *Client) RequestReviews(ctx context.Context, repo string, number int, logins []string) error {
	if len(logins) == 0 {
		return nil
	}
	owner, name, path, err := reviewersPath(repo, number, "requesting reviews")
	if err != nil {
		return err
	}

	want := make([]string, 0, len(logins))
	for _, l := range logins {
		want = append(want, postLogin(l))
	}

	body, err := json.Marshal(reviewersRequest{Reviewers: want})
	if err != nil {
		return fmt.Errorf("requesting reviews: %w", err)
	}

	if err := c.rest.DoWithContext(ctx, http.MethodPost, path, bytes.NewReader(body), nil); err != nil {
		return fmt.Errorf("requesting reviews: %w", classify(err))
	}

	waiting, err := c.awaitingReview(ctx, owner, name, number)
	if err != nil {
		return fmt.Errorf("requesting reviews: confirming the request: %w", err)
	}
	for _, l := range logins {
		if !waiting[loginKey(l)] {
			return fmt.Errorf("requesting reviews: GitHub did not record a request for %s", l)
		}
	}
	return nil
}

// RemoveReviewRequests cancels the review requested of each login. It confirms nothing: a login already gone
// looks the same as one removed. An empty slice makes no call.
func (c *Client) RemoveReviewRequests(ctx context.Context, repo string, number int, logins []string) error {
	if len(logins) == 0 {
		return nil
	}
	_, _, path, err := reviewersPath(repo, number, "removing review requests")
	if err != nil {
		return err
	}

	drop := make([]string, 0, len(logins))
	for _, l := range logins {
		drop = append(drop, deleteLogin(l))
	}

	body, err := json.Marshal(reviewersRequest{Reviewers: drop})
	if err != nil {
		return fmt.Errorf("removing review requests: %w", err)
	}

	if err := c.rest.DoWithContext(ctx, http.MethodDelete, path, bytes.NewReader(body), nil); err != nil {
		return fmt.Errorf("removing review requests: %w", classify(err))
	}
	return nil
}

func reviewersPath(repo string, number int, doing string) (owner, name, path string, err error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" {
		return "", "", "", fmt.Errorf("%s: %q is not owner/name", doing, repo)
	}
	return owner, name, fmt.Sprintf("repos/%s/%s/pulls/%d/requested_reviewers", owner, name, number), nil
}
