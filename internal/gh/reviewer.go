package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// GitHub's review bot answers to a different name in each direction, and the
// two REST verbs do not agree with each other. All of this was established by
// hand against a live pull request; none of it is documented.
//
//	POST    copilot-pull-request-reviewer[bot]   Copilot → 200, writes nothing
//	DELETE  Copilot                              the [bot] form → 422
//	GraphQL copilot-pull-request-reviewer        what reviewRequests reports
//
// The POST response never lists the bot at all. requested_reviewers comes back
// empty from a request that worked, so reading it cannot tell a success from
// the silent no-op above, and treating an empty array as failure rejects every
// Copilot request there is.
//
// Which is why the confirmation is a GraphQL read rather than the response
// body. reviewRequests is the only place a bot request is visible, and being
// able to tell those two 200s apart is the whole reason this write checks
// anything.
//
// This package speaks the GraphQL spelling everywhere, because that is the one
// the detail query returns and the rail renders, and translates per verb below.
const (
	// CopilotLogin is the bot as everything above this package names it.
	CopilotLogin = "copilot-pull-request-reviewer"

	copilotPostLogin   = "copilot-pull-request-reviewer[bot]"
	copilotDeleteLogin = "Copilot"
)

// postLogin is a login as the request endpoint takes it.
func postLogin(login string) string {
	if login == CopilotLogin {
		return copilotPostLogin
	}
	return login
}

// deleteLogin is a login as the cancel endpoint takes it, which for the bot is
// not the one that requested it. DELETE resolves the [bot] form to a Bot node
// and then rejects it for not being a User.
func deleteLogin(login string) string {
	if login == CopilotLogin {
		return copilotDeleteLogin
	}
	return login
}

// loginKey is what two spellings of one account compare as: the bot's names
// folded to one, and everything else folded to lower case.
//
// Case, because GitHub reports a login in whatever case the account holds
// rather than the case it was asked in, and a login is unique without it.
// Comparing the two verbatim would read a landed write as one that never
// happened.
func loginKey(login string) string {
	for _, alias := range []string{CopilotLogin, copilotPostLogin, copilotDeleteLogin} {
		if strings.EqualFold(login, alias) {
			return CopilotLogin
		}
	}
	return strings.ToLower(login)
}

// reviewersRequest is the body both verbs take. Teams go in a separate array
// this client never fills: the picker offers users alone, and a request it
// cannot offer is one it must not cancel either.
type reviewersRequest struct {
	Reviewers []string `json:"reviewers"`
}

// reviewRequestsQuery is who a pull request is currently waiting on. It is the
// confirmation a review request gets, because the REST response cannot give
// one: it omits the bot whether or not the write landed.
//
// It asks for no rateLimit. This runs on the back of a write, and the budget it
// would report is one the next fetch reports anyway.
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

// awaitingReview is who the pull request is waiting on, as GraphQL reports it.
// Teams come back with no login and are dropped: nothing here asks about one.
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

// RequestReviews asks for a review from each login, and reports an error unless
// the pull request is waiting on every one of them afterwards.
//
// REST rather than GraphQL, which is the whole reason this file exists.
// requestReviews accepts a bot node id, reports success and requests nothing,
// and the only id resolvable for Copilot is the coding agent rather than the
// reviewer.
//
// The confirmation is a second call rather than the response body. GitHub
// answers 200 both when the request landed and when it silently did nothing,
// and the response omits a bot request either way, so the body cannot tell the
// two apart. It costs a read on a write nobody makes often.
//
// A failed confirmation fails the write, though the request may well have
// landed. That is the safe way round: the caller reverts, and its refetch puts
// back whatever GitHub actually holds.
//
// An empty slice is not a call. The endpoint takes it and answers 422, and the
// caller reaches this only by applying a picker that changed nothing.
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

// RemoveReviewRequests cancels the review requested of each login.
//
// It reads nothing back, unlike RequestReviews. Cancelling is idempotent and
// the endpoint answers with whatever requests are left, so a login already gone
// is indistinguishable from one this call removed, and treating that as a
// failure would report an error for the state the caller asked for. The refetch
// the caller fires is what settles who is actually waiting.
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

// reviewersPath splits the repository and builds the endpoint both verbs share,
// refusing a malformed name before anything reaches the network the way
// RepoMeta does. The halves come back because the confirmation query addresses
// a repository by them rather than by the joined path.
func reviewersPath(repo string, number int, doing string) (owner, name, path string, err error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" {
		return "", "", "", fmt.Errorf("%s: %q is not owner/name", doing, repo)
	}
	return owner, name, fmt.Sprintf("repos/%s/%s/pulls/%d/requested_reviewers", owner, name, number), nil
}
