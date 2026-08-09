package gh

import (
	"context"
	"fmt"
	"time"
)

// viewerQuery asks who the token belongs to. GitHub answers per-comment
// permissions on the detail query already; the login is for everything it does
// not flag, starting with writing "you" where a name goes.
const viewerQuery = `
query Viewer {
  rateLimit { limit cost remaining resetAt }
  viewer { login }
}`

type viewerResponse struct {
	RateLimit struct {
		Limit     int
		Cost      int
		Remaining int
		ResetAt   time.Time
	}

	Viewer struct{ Login string }
}

// Viewer is the account behind the ambient gh token. It is asked once a session:
// a login does not change under a running program, and nothing else the app
// does can answer it.
func (c *Client) Viewer(ctx context.Context) (ViewerResult, error) {
	var resp viewerResponse

	if err := c.gql.DoWithContext(ctx, viewerQuery, nil, &resp); err != nil {
		return ViewerResult{}, fmt.Errorf("fetching the viewer: %w", classify(err))
	}
	if resp.Viewer.Login == "" {
		return ViewerResult{}, fmt.Errorf("fetching the viewer: no account behind this token")
	}

	return ViewerResult{
		Viewer: Actor{Login: resp.Viewer.Login},
		RateLimit: RateLimit{
			Limit:     resp.RateLimit.Limit,
			Cost:      resp.RateLimit.Cost,
			Remaining: resp.RateLimit.Remaining,
			ResetAt:   resp.RateLimit.ResetAt,
		},
	}, nil
}
