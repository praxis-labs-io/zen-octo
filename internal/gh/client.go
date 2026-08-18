// Package gh talks to GitHub. It is the only package in zen-octo that touches
// the network, and it returns domain types rather than API response shapes, so
// everything above it can be tested against a fake.
package gh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
)

// graphQLDoer is the slice of go-gh's GraphQL client this package uses.
// Tests substitute a fake; nothing else in zen-octo sees it.
type graphQLDoer interface {
	DoWithContext(ctx context.Context, query string, variables map[string]any, response any) error
}

// restDoer is the same seam over REST. GraphQL has no field carrying a patch,
// so the diff is the one thing this package cannot ask for in a query.
type restDoer interface {
	DoWithContext(ctx context.Context, method, path string, body io.Reader, response any) error
	// RequestWithContext is the undecoded half, for a body that is not JSON.
	RequestWithContext(ctx context.Context, method, path string, body io.Reader) (*http.Response, error)
}

// Client is a GitHub API client riding the token from the user's gh login.
type Client struct {
	gql  graphQLDoer
	rest restDoer
}

// New builds a client from the ambient gh authentication.
func New() (*Client, error) {
	gql, err := api.NewGraphQLClient(api.ClientOptions{})
	if err != nil {
		return nil, fmt.Errorf("building GitHub client: %w", err)
	}
	rest, err := api.NewRESTClient(api.ClientOptions{})
	if err != nil {
		return nil, fmt.Errorf("building GitHub client: %w", err)
	}
	return &Client{gql: gql, rest: rest}, nil
}

// newWithDoer builds a client around substitute transports, for tests.
func newWithDoer(gql graphQLDoer, rest restDoer) *Client {
	return &Client{gql: gql, rest: rest}
}

// ScopeError is a 403 the token can't satisfy. It carries the command that
// fixes it, because "HTTP 403" on its own sends people to the wrong place.
type ScopeError struct {
	Missing []string
	err     error
}

func (e *ScopeError) Error() string {
	if len(e.Missing) == 0 {
		return fmt.Sprintf("%v\nYour gh token is missing a scope this call needs.", e.err)
	}
	noun := "scope"
	if len(e.Missing) > 1 {
		noun = "scopes"
	}
	return fmt.Sprintf(
		"%v\nYour gh token is missing the %s %s. Run:\n  gh auth refresh -s %s",
		e.err,
		strings.Join(e.Missing, ", "),
		noun,
		strings.Join(e.Missing, ","),
	)
}

func (e *ScopeError) Unwrap() error { return e.err }

// classify turns a raw API error into a ScopeError when the token is the
// problem, so callers can show the fix instead of the status code.
func classify(err error) error {
	if err == nil {
		return nil
	}

	var httpErr *api.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 403 {
		return err
	}

	have := scopeSet(httpErr.Headers.Get("X-Oauth-Scopes"))
	var missing []string
	for _, want := range splitScopes(httpErr.Headers.Get("X-Accepted-Oauth-Scopes")) {
		if _, ok := have[want]; !ok {
			missing = append(missing, want)
		}
	}
	return &ScopeError{Missing: missing, err: err}
}

func splitScopes(header string) []string {
	var out []string
	for _, s := range strings.Split(header, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func scopeSet(header string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, s := range splitScopes(header) {
		out[s] = struct{}{}
	}
	return out
}
