package gh

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
)

func TestParsePRRef(t *testing.T) {
	tests := []struct {
		in   string
		want PRRef
	}{
		{in: "acme/rocket#412", want: PRRef{Owner: "acme", Name: "rocket", Number: 412}},
		{in: "my-org/my.repo_2#1", want: PRRef{Owner: "my-org", Name: "my.repo_2", Number: 1}},
		{in: "acme/rocket#2147483647", want: PRRef{Owner: "acme", Name: "rocket", Number: 2147483647}},
		{in: "https://github.com/acme/rocket/pull/412", want: PRRef{Owner: "acme", Name: "rocket", Number: 412}},
		{in: "https://github.com/acme/rocket/pull/412/", want: PRRef{Owner: "acme", Name: "rocket", Number: 412}},
		{in: "https://github.com/acme/rocket/pull/412/files", want: PRRef{Owner: "acme", Name: "rocket", Number: 412}},
		{in: "https://github.com/acme/rocket/pull/412#issuecomment-9", want: PRRef{Owner: "acme", Name: "rocket", Number: 412}},
		{in: "https://github.com/acme/rocket/pull/412?w=1", want: PRRef{Owner: "acme", Name: "rocket", Number: 412}},
		{in: "http://www.github.com/acme/rocket/pull/412", want: PRRef{Owner: "acme", Name: "rocket", Number: 412}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParsePRRef(tt.in)
			if err != nil {
				t.Fatalf("ParsePRRef(%q) error = %v, want nil", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParsePRRef(%q) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParsePRRefRefuses(t *testing.T) {
	for _, in := range []string{
		"",
		"412",
		"acme/rocket",
		"acme/rocket#",
		"acme/rocket#0",
		"acme/rocket#-3",
		"acme/rocket#12a",
		"acme/rocket#+5",
		"acme/rocket# 5",
		"acme/rocket#2147483648",
		"acme/rocket#99999999999999999999",
		"acme#412",
		"acme/rocket/x#412",
		"/rocket#412",
		"acme/..#412",
		"https://github.com/acme/rocket/issues/412",
		"https://github.com/acme/rocket/pull/",
		"https://github.com/acme/rocket",
		"https://gitlab.com/acme/rocket/pull/412",
		"https://github.example.com/acme/rocket/pull/412",
		"ftp://github.com/acme/rocket/pull/412",
	} {
		t.Run(in, func(t *testing.T) {
			_, err := ParsePRRef(in)
			if err == nil {
				t.Fatalf("ParsePRRef(%q) error = nil, want a refusal", in)
			}
			if !strings.Contains(err.Error(), "owner/repo#123") {
				t.Errorf("error = %q, want it to name the accepted forms", err)
			}
		})
	}
}

const foundPRBody = `{
  "repository": {
    "pullRequest": {
      "id": "PR_1", "number": 412, "title": "Fix auth retry",
      "url": "https://github.com/acme/rocket/pull/412",
      "isDraft": false, "state": "OPEN",
      "headRefName": "fix-auth", "baseRefName": "main",
      "changedFiles": 3,
      "author": {"login": "drucial"},
      "repository": {"nameWithOwner": "acme/rocket"},
      "statusCheckRollup": {"nodes": [{"commit": {"statusCheckRollup": {"state": "SUCCESS"}}}]}
    }
  }
}`

func TestFindPullRequestMapsTheRow(t *testing.T) {
	doer := &fakeDoer{body: foundPRBody}

	got, err := newWithDoer(doer, nil).FindPullRequest(context.Background(), PRRef{Owner: "acme", Name: "rocket", Number: 412})
	if err != nil {
		t.Fatalf("FindPullRequest() error = %v, want nil", err)
	}
	if got.ID != "PR_1" || got.Number != 412 || got.Repository != "acme/rocket" || got.HeadRefName != "fix-auth" {
		t.Errorf("got %+v, want PR_1 acme/rocket#412 on fix-auth", got)
	}
	if got.Checks != CheckStateSuccess || got.Author.Login != "drucial" {
		t.Errorf("Checks = %q, Author = %q, want SUCCESS and drucial", got.Checks, got.Author.Login)
	}
	if doer.gotVars["owner"] != "acme" || doer.gotVars["name"] != "rocket" || doer.gotVars["number"] != 412 {
		t.Errorf("vars = %v, want owner acme, name rocket, number 412", doer.gotVars)
	}
}

func TestFindPullRequestReportsAMissingOne(t *testing.T) {
	tests := []struct {
		name string
		doer *fakeDoer
	}{
		{name: "null pull request", doer: &fakeDoer{body: `{"repository": {"pullRequest": null}}`}},
		{name: "null repository", doer: &fakeDoer{body: `{"repository": null}`}},
		{name: "not found on the number", doer: &fakeDoer{
			body: `{"repository": {"pullRequest": null}}`,
			err: &api.GraphQLError{Errors: []api.GraphQLErrorItem{
				{Type: "NOT_FOUND", Path: []any{"repository", "pullRequest"}},
			}},
		}},
		{name: "not found on the repository", doer: &fakeDoer{
			body: `{"repository": null}`,
			err: &api.GraphQLError{Errors: []api.GraphQLErrorItem{
				{Type: "NOT_FOUND", Path: []any{"repository"}},
			}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newWithDoer(tt.doer, nil).FindPullRequest(context.Background(), PRRef{Owner: "acme", Name: "rocket", Number: 9})
			if err == nil || err.Error() != "no pull request at acme/rocket#9, or no access to it" {
				t.Errorf("error = %v, want the not-found message", err)
			}
		})
	}
}

func TestFindPullRequestWrapsOtherFailures(t *testing.T) {
	cause := errors.New("connection reset")
	doer := &fakeDoer{err: cause}

	_, err := newWithDoer(doer, nil).FindPullRequest(context.Background(), PRRef{Owner: "acme", Name: "rocket", Number: 9})
	if !errors.Is(err, cause) {
		t.Fatalf("error = %v, want it to wrap the cause", err)
	}
	if !strings.HasPrefix(err.Error(), "fetching acme/rocket#9: ") {
		t.Errorf("error = %q, want it to name the pull request", err)
	}
}
