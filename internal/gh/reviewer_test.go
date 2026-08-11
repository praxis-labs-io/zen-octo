package gh

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

// waitingOn is a reviewRequests answer naming these logins. It is the
// confirmation a request gets: the REST response omits a bot whether or not the
// write landed, so GraphQL is the only place the proof lives.
func waitingOn(logins ...string) string {
	nodes := make([]string, 0, len(logins))
	for _, l := range logins {
		nodes = append(nodes, `{"requestedReviewer": {"login": "`+l+`"}}`)
	}
	return `{"repository": {"pullRequest": {"reviewRequests": {"nodes": [` +
		strings.Join(nodes, ",") + `]}}}}`
}

// requesting is a client whose POST succeeds and whose confirmation reports
// these logins as being waited on.
func requesting(logins ...string) (*Client, *fakeDoer, *fakeREST) {
	gql, rest := &fakeDoer{body: waitingOn(logins...)}, &fakeREST{}
	return newWithDoer(gql, rest), gql, rest
}

func TestRequestReviewsPostsTheLoginsToThePullRequest(t *testing.T) {
	client, _, rest := requesting("nkr")

	if err := client.RequestReviews(context.Background(), "zen-octo/zen-octo", 17, []string{"nkr"}); err != nil {
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
	gql, rest := &fakeDoer{}, &fakeREST{}

	if err := newWithDoer(gql, rest).RemoveReviewRequests(context.Background(), "zen-octo/zen-octo", 17, []string{"nkr"}); err != nil {
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
	// Cancelling confirms nothing, unlike requesting. It is idempotent, so a
	// login already gone reads the same as one this call removed, and a read
	// after it could only report the state the caller asked for.
	if gql.gotQuery != "" {
		t.Error("a cancellation went looking for a confirmation it cannot use")
	}
}

// The two verbs take different spellings of the same bot, which is the trap
// that cost a working write. POST resolves the [bot] form; DELETE resolves it
// to a Bot node and then rejects it for not being a User.
func TestTheTwoReviewerVerbsSpellCopilotDifferently(t *testing.T) {
	client, _, rest := requesting(CopilotLogin)

	if err := client.RequestReviews(context.Background(), "zen-octo/zen-octo", 17, []string{CopilotLogin}); err != nil {
		t.Fatalf("RequestReviews: %v", err)
	}
	if want := `{"reviewers":["copilot-pull-request-reviewer[bot]"]}`; rest.gotBody != want {
		t.Errorf("POST body = %q, want %q", rest.gotBody, want)
	}

	drop := &fakeREST{}
	if err := newWithDoer(nil, drop).RemoveReviewRequests(context.Background(), "zen-octo/zen-octo", 17, []string{CopilotLogin}); err != nil {
		t.Fatalf("RemoveReviewRequests: %v", err)
	}
	if want := `{"reviewers":["Copilot"]}`; drop.gotBody != want {
		t.Errorf("DELETE body = %q, want %q", drop.gotBody, want)
	}
}

// The regression that shipped. GitHub's POST response never lists the bot,
// whether or not the request landed, so reading requested_reviewers rejects
// every Copilot request there is. The confirmation has to be the GraphQL read.
func TestAnEmptyRESTResponseIsNotAFailedCopilotRequest(t *testing.T) {
	gql := &fakeDoer{body: waitingOn(CopilotLogin)}
	// What GitHub actually answers a successful Copilot request with.
	rest := &fakeREST{body: `{"requested_reviewers": [], "requested_teams": []}`}

	err := newWithDoer(gql, rest).RequestReviews(context.Background(), "zen-octo/zen-octo", 17, []string{CopilotLogin})
	if err != nil {
		t.Fatalf("RequestReviews = %v, want the landed request accepted", err)
	}
	if !strings.Contains(gql.gotQuery, "reviewRequests") {
		t.Error("the request was never confirmed against reviewRequests")
	}
	if got, want := gql.gotVars["number"], 17; got != want {
		t.Errorf("confirmed number = %v, want %v", got, want)
	}
}

// The failure the confirmation exists for. Asking under a name GitHub does not
// recognise answers 200 and writes nothing, and the pull request is waiting on
// nobody afterwards.
func TestRequestReviewsRejectsASilentNoOp(t *testing.T) {
	client, _, _ := requesting()

	err := client.RequestReviews(context.Background(), "zen-octo/zen-octo", 17, []string{CopilotLogin})
	if err == nil {
		t.Fatal("a 200 that recorded nothing came back as a success")
	}
	if !strings.Contains(err.Error(), "did not record a request") {
		t.Errorf("error = %q, want it to say the request was not recorded", err)
	}
}

// Two asked for and one recorded is the same failure. The whole ask has to
// land, not merely something.
func TestRequestReviewsRejectsAPartialAnswer(t *testing.T) {
	client, _, _ := requesting("nkr")

	err := client.RequestReviews(context.Background(), "zen-octo/zen-octo", 17, []string{"nkr", "octobot"})
	if err == nil {
		t.Fatal("a confirmation missing one of the two reviewers came back as a success")
	}
	if !strings.Contains(err.Error(), "octobot") {
		t.Errorf("error = %q, want it to name who was not recorded", err)
	}
}

// GitHub reports a login in whatever case the account holds rather than the
// case it was asked in, and that is not a write that failed.
func TestRequestReviewsAcceptsTheAnswerInAnotherCase(t *testing.T) {
	client, _, _ := requesting("NKR")

	if err := client.RequestReviews(context.Background(), "zen-octo/zen-octo", 17, []string{"nkr"}); err != nil {
		t.Errorf("RequestReviews = %v, want the differently-cased login accepted", err)
	}
}

// A confirmation that cannot be read fails the write, though the request may
// well have landed. That is the safe way round: the caller reverts and its
// refetch puts back whatever GitHub actually holds.
func TestAConfirmationThatFailsFailsTheWrite(t *testing.T) {
	boom := errors.New("boom")
	client := newWithDoer(&fakeDoer{err: boom}, &fakeREST{})

	err := client.RequestReviews(context.Background(), "zen-octo/zen-octo", 17, []string{"nkr"})
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want it to wrap %v", err, boom)
	}
	if !strings.Contains(err.Error(), "confirming the request") {
		t.Errorf("error = %q, want it to name the step that failed", err)
	}
}

func TestAConfirmationWithNoPullRequestIsAnError(t *testing.T) {
	client := newWithDoer(&fakeDoer{body: `{"repository": null}`}, &fakeREST{})

	err := client.RequestReviews(context.Background(), "zen-octo/zen-octo", 17, []string{"nkr"})
	if err == nil {
		t.Fatal("a confirmation over a null repository came back as a success")
	}
	if !strings.Contains(err.Error(), "no pull request") {
		t.Errorf("error = %q, want it to say nothing came back", err)
	}
}

// Applying a picker that changed nothing reaches here with an empty side. The
// endpoint answers 422 to one, so it is not a call to make, and there is
// nothing to confirm either.
func TestNeitherReviewerCallSendsAnEmptySet(t *testing.T) {
	gql, rest := &fakeDoer{}, &fakeREST{}
	client := newWithDoer(gql, rest)

	if err := client.RequestReviews(context.Background(), "zen-octo/zen-octo", 17, nil); err != nil {
		t.Errorf("RequestReviews(nil) = %v, want nil", err)
	}
	if err := client.RemoveReviewRequests(context.Background(), "zen-octo/zen-octo", 17, nil); err != nil {
		t.Errorf("RemoveReviewRequests(nil) = %v, want nil", err)
	}
	if rest.gotMethod != "" {
		t.Errorf("called GitHub anyway, with %s %s", rest.gotMethod, rest.gotPath)
	}
	if gql.gotQuery != "" {
		t.Error("confirmed a request that was never made")
	}
}

func TestTheReviewerCallsRejectMalformedRepositoryNames(t *testing.T) {
	for _, repo := range []string{"", "zen-octo", "/zen-octo", "zen-octo/"} {
		gql, rest := &fakeDoer{}, &fakeREST{}
		client := newWithDoer(gql, rest)

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
	client, _, rest := requesting("nkr")

	if err := client.RequestReviews(context.Background(), "zen-octo/zen-octo", 17, []string{"nkr"}); err != nil {
		t.Fatalf("RequestReviews: %v", err)
	}
	if strings.Contains(rest.gotBody, "team_reviewers") {
		t.Errorf("body = %q, want no teams in it", rest.gotBody)
	}
}

// A team being waited on has no login, and nothing here asks about one. It must
// not decode as an empty-login entry that a caller could match against.
func TestTheConfirmationIgnoresTeams(t *testing.T) {
	gql := &fakeDoer{body: `{"repository": {"pullRequest": {"reviewRequests": {"nodes": [
	  {"requestedReviewer": {}},
	  {"requestedReviewer": {"login": "nkr"}}
	]}}}}`}

	if err := newWithDoer(gql, &fakeREST{}).RequestReviews(
		context.Background(), "zen-octo/zen-octo", 17, []string{"nkr"},
	); err != nil {
		t.Errorf("RequestReviews = %v, want the team beside the reviewer ignored", err)
	}
}

// TestLiveTheReviewRequestsQueryMatchesTheSchema reads rather than writes, so
// it runs against a real pull request and proves every field resolves.
//
// It earns a live test where the two REST calls cannot get one. This query is
// the whole confirmation a review request receives: the REST response omits a
// bot request whether or not it landed, so a typo here fails every request with
// "confirming the request" and the write that actually worked reads as broken.
// That is exactly the shape of the bug this replaced.
//
// It lives in the internal test package because awaitingReview is unexported,
// and there is no exported way to reach it that does not write.
//
//	ZEN_OCTO_LIVE=1 go test ./internal/gh/ -run TestLive -v
func TestLiveTheReviewRequestsQueryMatchesTheSchema(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Pull request 1, which is merged and will not move again. Who it was
	// waiting on is not asserted; that it resolves at all is the point.
	if _, err := client.awaitingReview(ctx, "zen-octo", "zen-octo", 1); err != nil {
		t.Fatalf("awaitingReview: %v", err)
	}
}

// A repository the token cannot see comes back as a null node rather than an
// error, and reading that as "waiting on nobody" would fail every request made
// against it with a message blaming the reviewer.
func TestLiveAMissingPullRequestIsAnError(t *testing.T) {
	if os.Getenv("ZEN_OCTO_LIVE") == "" {
		t.Skip("set ZEN_OCTO_LIVE=1 to run against the real GitHub API")
	}

	client, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := client.awaitingReview(ctx, "zen-octo", "zen-octo", 999999); err == nil {
		t.Fatal("a pull request that does not exist came back as a success")
	}
}
