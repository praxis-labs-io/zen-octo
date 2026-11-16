package gh

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
)

// PRRef names a pull request by its repository and number.
type PRRef struct {
	Owner  string
	Name   string
	Number int
}

// Repo is "owner/name", the form PullRequest.Repository carries.
func (r PRRef) Repo() string { return r.Owner + "/" + r.Name }

func (r PRRef) String() string { return r.Repo() + "#" + strconv.Itoa(r.Number) }

// ParsePRRef reads owner/repo#123 or a github.com pull request URL, trailing tab or fragment included.
func ParsePRRef(s string) (PRRef, error) {
	ref, ok := parseShortRef(s)
	if !ok {
		ref, ok = parseURLRef(s)
	}
	if !ok {
		return PRRef{}, fmt.Errorf("no pull request in %q: want owner/repo#123 or https://github.com/owner/repo/pull/123", s)
	}
	return ref, nil
}

func parseShortRef(s string) (PRRef, bool) {
	repo, num, ok := strings.Cut(s, "#")
	if !ok {
		return PRRef{}, false
	}
	owner, name, ok := strings.Cut(repo, "/")
	if !ok {
		return PRRef{}, false
	}
	return newRef(owner, name, num)
}

func parseURLRef(s string) (PRRef, bool) {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return PRRef{}, false
	}
	if host := strings.ToLower(u.Host); host != "github.com" && host != "www.github.com" {
		return PRRef{}, false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 || parts[2] != "pull" {
		return PRRef{}, false
	}
	return newRef(parts[0], parts[1], parts[3])
}

func newRef(owner, name, num string) (PRRef, bool) {
	if !validRepoPart(owner) || !validRepoPart(name) {
		return PRRef{}, false
	}
	n, err := strconv.ParseUint(num, 10, 32)
	if err != nil || n == 0 || n > math.MaxInt32 {
		return PRRef{}, false
	}
	return PRRef{Owner: owner, Name: name, Number: int(n)}, true
}

func validRepoPart(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.'
		if !ok {
			return false
		}
	}
	return true
}

const findPullRequestQuery = `
query FindPullRequest($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {` + pullRequestRowFields + `
    }
  }
}`

type findPullRequestResponse struct {
	Repository *struct {
		PullRequest *pullRequestRow
	}
}

// FindPullRequest looks a pull request up by repository and number, the shape a list row carries.
func (c *Client) FindPullRequest(ctx context.Context, ref PRRef) (PullRequest, error) {
	vars := map[string]any{"owner": ref.Owner, "name": ref.Name, "number": ref.Number}

	var resp findPullRequestResponse
	err := c.gql.DoWithContext(ctx, findPullRequestQuery, vars, &resp)
	if notFound(err) || err == nil && (resp.Repository == nil || resp.Repository.PullRequest == nil) {
		return PullRequest{}, fmt.Errorf("no pull request at %s, or no access to it", ref)
	}
	if err != nil {
		return PullRequest{}, fmt.Errorf("fetching %s: %w", ref, classify(err))
	}
	return resp.Repository.PullRequest.pullRequest(), nil
}

func notFound(err error) bool {
	var gqlErr *api.GraphQLError
	return errors.As(err, &gqlErr) && gqlErr.Match("NOT_FOUND", "repository.")
}
