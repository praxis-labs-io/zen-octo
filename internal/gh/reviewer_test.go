package gh

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
)

func requested(logins ...string) string {
	quoted := make([]string, 0, len(logins))
	for _, l := range logins {
		quoted = append(quoted, `{"login": "`+l+`"}`)
	}
	return `{"requested_reviewers": [` + strings.Join(quoted, ",") + `]}`
}

func TestRequestReviewsPostsTheLoginsToThePullRequest(t *testing.T) {
	rest := &fakeREST{body: requested("nkr")}

	if err := newWithDoer(nil, rest).RequestReviews(context.Background(), "zen-octo/zen-octo", 17, []string{"nkr"}); err != nil {
		t.Fatalf("RequestReviews: %v", err)
	}

	if rest.gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", rest.gotMethod)
	}
	if want := "repos/zen-octo/zen-octo/pulls/17/requested_reviewers"; rest.gotPath != want {
		t.Errorf("path = %q, want %q", rest.gotPath, want)
	}
	if want := `{"reviewers":["nkr"]}`; rest.gotBody != want {
		t.Errorf("body = %q, want %q", rest.gotBody, want)
	}
}

func TestRemoveReviewRequestsDeletesTheLogins(t *testing.T) {
	rest := &fakeREST{}

	if err := newWithDoer(nil, rest).RemoveReviewRequests(context.Background(), "zen-octo/zen-octo", 17, []string{"nkr"}); err != nil {
		t.Fatalf("RemoveReviewRequests: %v", err)
	}

	if rest.gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", rest.gotMethod)
	}
	if want := "repos/zen-octo/zen-octo/pulls/17/requested_reviewers"; rest.gotPath != want {
		t.Errorf("path = %q, want %q", rest.gotPath, want)
	}
	if want := `{"reviewers":["nkr"]}`; rest.gotBody != want {
		t.Errorf("body = %q, want %q", rest.gotBody, want)
	}
}

// The bot answers to a different name in each direction. The request has to
// carry the [bot] suffix, which is the spelling the endpoint accepts, and the
// answer comes back as "Copilot", which is neither of the other two.
func TestRequestingCopilotSendsTheBotLoginAndAcceptsTheAnswer(t *testing.T) {
	rest := &fakeREST{body: requested("Copilot")}

	err := newWithDoer(nil, rest).RequestReviews(context.Background(), "zen-octo/zen-octo", 17, []string{CopilotLogin})
	if err != nil {
		t.Fatalf("RequestReviews: %v", err)
	}

	if want := `{"reviewers":["copilot-pull-request-reviewer[bot]"]}`; rest.gotBody != want {
		t.Errorf("body = %q, want %q", rest.gotBody, want)
	}
}

func TestRemovingCopilotSendsTheBotLogin(t *testing.T) {
	rest := &fakeREST{}

	err := newWithDoer(nil, rest).RemoveReviewRequests(context.Background(), "zen-octo/zen-octo", 17, []string{CopilotLogin})
	if err != nil {
		t.Fatalf("RemoveReviewRequests: %v", err)
	}

	if want := `{"reviewers":["copilot-pull-request-reviewer[bot]"]}`; rest.gotBody != want {
		t.Errorf("body = %q, want %q", rest.gotBody, want)
	}
}

// The failure that matters comes back as a success. Asking for a login GitHub
// does not recognise as a reviewer answers 200 with the pull request unchanged,
// and a caller reading the status code alone would paint a reviewer on the rail
// that nobody added.
func TestRequestReviewsRejectsASilentNoOp(t *testing.T) {
	rest := &fakeREST{body: `{"requested_reviewers": []}`}

	err := newWithDoer(nil, rest).RequestReviews(context.Background(), "zen-octo/zen-octo", 17, []string{"Copilot"})
	if err == nil {
		t.Fatal("a 200 that recorded nothing came back as a success")
	}
	if !strings.Contains(err.Error(), "did not record a request") {
		t.Errorf("error = %q, want it to say the request was not recorded", err)
	}
}

// Two asked for and one recorded is the same failure. The whole ask has to come
// back, not merely something.
func TestRequestReviewsRejectsAPartialAnswer(t *testing.T) {
	rest := &fakeREST{body: requested("nkr")}

	err := newWithDoer(nil, rest).RequestReviews(context.Background(), "zen-octo/zen-octo", 17, []string{"nkr", "octobot"})
	if err == nil {
		t.Fatal("a response missing one of the two reviewers came back as a success")
	}
	if !strings.Contains(err.Error(), "octobot") {
		t.Errorf("error = %q, want it to name who was not recorded", err)
	}
}

// GitHub echoes a login in whatever case it holds rather than the case it was
// asked in, and that is not a write that failed.
func TestRequestReviewsAcceptsTheAnswerInAnotherCase(t *testing.T) {
	rest := &fakeREST{body: requested("NKR")}

	if err := newWithDoer(nil, rest).RequestReviews(context.Background(), "zen-octo/zen-octo", 17, []string{"nkr"}); err != nil {
		t.Errorf("RequestReviews = %v, want the differently-cased login accepted", err)
	}
}

// Applying a picker that changed nothing reaches here with an empty side. The
// endpoint answers 422 to one, so it is not a call to make.
func TestNeitherReviewerCallSendsAnEmptySet(t *testing.T) {
	rest := &fakeREST{}
	client := newWithDoer(nil, rest)

	if err := client.RequestReviews(context.Background(), "zen-octo/zen-octo", 17, nil); err != nil {
		t.Errorf("RequestReviews(nil) = %v, want nil", err)
	}
	if err := client.RemoveReviewRequests(context.Background(), "zen-octo/zen-octo", 17, nil); err != nil {
		t.Errorf("RemoveReviewRequests(nil) = %v, want nil", err)
	}
	if rest.gotMethod != "" {
		t.Errorf("called GitHub anyway, with %s %s", rest.gotMethod, rest.gotPath)
	}
}

func TestTheReviewerCallsRejectMalformedRepositoryNames(t *testing.T) {
	for _, repo := range []string{"", "zen-octo", "/zen-octo", "zen-octo/"} {
		rest := &fakeREST{}
		client := newWithDoer(nil, rest)

		if err := client.RequestReviews(context.Background(), repo, 17, []string{"nkr"}); err == nil {
			t.Errorf("RequestReviews(%q) = nil error, want one", repo)
		}
		if err := client.RemoveReviewRequests(context.Background(), repo, 17, []string{"nkr"}); err == nil {
			t.Errorf("RemoveReviewRequests(%q) = nil error, want one", repo)
		}
		if rest.gotMethod != "" {
			t.Errorf("RequestReviews(%q) called GitHub anyway", repo)
		}
	}
}

func TestAForbiddenReviewerCallNamesTheScopeToAdd(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Oauth-Scopes", "gist, read:org")
	headers.Set("X-Accepted-Oauth-Scopes", "repo")

	rest := &fakeREST{err: &api.HTTPError{StatusCode: 403, Headers: headers}}
	err := newWithDoer(nil, rest).RequestReviews(context.Background(), "zen-octo/zen-octo", 17, []string{"nkr"})

	var scope *ScopeError
	if !errors.As(err, &scope) {
		t.Fatalf("err = %v, want a ScopeError", err)
	}
	if !strings.Contains(err.Error(), "gh auth refresh -s repo") {
		t.Errorf("err = %q, want the refresh command", err)
	}
}

func TestTheReviewerCallsWrapTransportErrors(t *testing.T) {
	boom := errors.New("boom")

	for _, tt := range []struct {
		name string
		call func(*Client) error
		want string
	}{
		{"request", func(c *Client) error {
			return c.RequestReviews(context.Background(), "zen-octo/zen-octo", 17, []string{"nkr"})
		}, "requesting reviews"},
		{"remove", func(c *Client) error {
			return c.RemoveReviewRequests(context.Background(), "zen-octo/zen-octo", 17, []string{"nkr"})
		}, "removing review requests"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(newWithDoer(nil, &fakeREST{err: boom}))
			if !errors.Is(err, boom) {
				t.Fatalf("error = %v, want it to wrap %v", err, boom)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to name what failed", err)
			}
		})
	}
}

// Teams are requested through a separate array, and this client never fills it.
// The picker offers users alone, so a team it cannot offer is one it must not
// cancel either.
func TestTheReviewerBodyCarriesNoTeams(t *testing.T) {
	rest := &fakeREST{body: requested("nkr")}

	if err := newWithDoer(nil, rest).RequestReviews(context.Background(), "zen-octo/zen-octo", 17, []string{"nkr"}); err != nil {
		t.Fatalf("RequestReviews: %v", err)
	}
	if strings.Contains(rest.gotBody, "team_reviewers") {
		t.Errorf("body = %q, want no teams in it", rest.gotBody)
	}
}
