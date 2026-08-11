package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// GitHub's review bot answers to a different name in each direction, and all
// three are live. The client speaks the GraphQL spelling everywhere above this
// file, because that is the one the detail query returns and the rail renders,
// and translates at the REST boundary.
//
//	request as  copilot-pull-request-reviewer[bot]
//	answers as  Copilot
//	reads as    copilot-pull-request-reviewer
//
// The two ways to get this wrong both look like success. A bare "Copilot" in
// the request returns 200 with an empty requested_reviewers and writes nothing;
// the login without the [bot] suffix returns 422. Neither is recoverable from
// the status code alone, which is why RequestReviews reads the response.
const (
	// CopilotLogin is the bot as everything above this package names it.
	CopilotLogin = "copilot-pull-request-reviewer"

	copilotRequestLogin = "copilot-pull-request-reviewer[bot]"
	copilotAnswerLogin  = "Copilot"
)

// requestLogin is a login as the REST endpoint takes it.
func requestLogin(login string) string {
	if login == CopilotLogin {
		return copilotRequestLogin
	}
	return login
}

// loginKey is what two spellings of one account compare as: the bot's three
// names folded to one, and everything else folded to lower case.
//
// Case, because GitHub echoes a login in whatever case the account holds rather
// than the case it was asked in, and a login is unique without it. Comparing
// the two verbatim would read an answer as a write that never happened.
func loginKey(login string) string {
	if strings.EqualFold(login, copilotAnswerLogin) || strings.EqualFold(login, copilotRequestLogin) {
		return CopilotLogin
	}
	return strings.ToLower(login)
}

// reviewersRequest is the body both verbs take. Teams go in a separate array
// this client never fills: the picker offers users alone, and a request it
// cannot offer is one it must not cancel either.
type reviewersRequest struct {
	Reviewers []string `json:"reviewers"`
}

// reviewersResponse is the pull request the endpoint answers with, cut down to
// the one field worth reading.
type reviewersResponse struct {
	RequestedReviewers []struct {
		Login string `json:"login"`
	} `json:"requested_reviewers"`
}

// RequestReviews asks for a review from each login, and reports an error unless
// every one of them is waiting for a review afterwards.
//
// REST rather than GraphQL, which is the whole reason this file exists.
// requestReviews accepts a bot node id, reports success and requests nothing,
// and the only id resolvable for Copilot is the coding agent rather than the
// reviewer. That was settled by hand against a real pull request before any of
// this was built.
//
// The response is read rather than trusted, because the failure that matters
// here comes back as a 200. Asking for a login GitHub does not recognise as a
// reviewer answers with the pull request unchanged, and a caller that took the
// status code would paint a reviewer on the rail that nobody added.
//
// An empty slice is not a call. The endpoint takes it and answers 422, and the
// caller reaches this only by applying a picker that changed nothing.
func (c *Client) RequestReviews(ctx context.Context, repo string, number int, logins []string) error {
	if len(logins) == 0 {
		return nil
	}
	path, err := reviewersPath(repo, number, "requesting reviews")
	if err != nil {
		return err
	}

	want := make([]string, 0, len(logins))
	for _, l := range logins {
		want = append(want, requestLogin(l))
	}

	body, err := json.Marshal(reviewersRequest{Reviewers: want})
	if err != nil {
		return fmt.Errorf("requesting reviews: %w", err)
	}

	var resp reviewersResponse
	if err := c.rest.DoWithContext(ctx, http.MethodPost, path, bytes.NewReader(body), &resp); err != nil {
		return fmt.Errorf("requesting reviews: %w", classify(err))
	}

	got := make(map[string]bool, len(resp.RequestedReviewers))
	for _, r := range resp.RequestedReviewers {
		got[loginKey(r.Login)] = true
	}
	for _, l := range logins {
		if !got[loginKey(l)] {
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
	path, err := reviewersPath(repo, number, "removing review requests")
	if err != nil {
		return err
	}

	drop := make([]string, 0, len(logins))
	for _, l := range logins {
		drop = append(drop, requestLogin(l))
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

// reviewersPath is the endpoint both verbs share, refusing a malformed
// repository before anything reaches the network the way RepoMeta does.
func reviewersPath(repo string, number int, doing string) (string, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" {
		return "", fmt.Errorf("%s: %q is not owner/name", doing, repo)
	}
	return fmt.Sprintf("repos/%s/%s/pulls/%d/requested_reviewers", owner, name, number), nil
}
