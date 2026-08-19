package gh

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
)

func TestJobFetchesTheStepsBesideTheLog(t *testing.T) {
	rest := &fakeREST{body: `{
		"id": 9001,
		"name": "test",
		"status": "completed",
		"conclusion": "failure",
		"started_at": "2026-08-19T14:00:00Z",
		"completed_at": "2026-08-19T14:02:03Z",
		"steps": [
			{"number": 1, "name": "Set up job", "status": "completed", "conclusion": "success", "started_at": "2026-08-19T14:00:00Z", "completed_at": "2026-08-19T14:00:04Z"},
			{"number": 2, "name": "Run tests", "status": "completed", "conclusion": "failure", "started_at": "2026-08-19T14:00:04Z", "completed_at": "2026-08-19T14:02:03Z"},
			{"number": 3, "name": "Publish", "status": "completed", "conclusion": "skipped", "started_at": null, "completed_at": null}
		]
	}`}

	got, err := newWithDoer(nil, rest).Job(context.Background(), "zen-octo/zen-octo", 9001)
	if err != nil {
		t.Fatalf("Job: %v", err)
	}
	if rest.gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", rest.gotMethod)
	}
	if want := "repos/zen-octo/zen-octo/actions/jobs/9001"; rest.gotPath != want {
		t.Errorf("path = %q, want %q", rest.gotPath, want)
	}
	if got.ID != 9001 || got.Name != "test" || got.State != CheckStateFailure {
		t.Errorf("job = %+v, want the failed test job", got)
	}
	if got.Duration.String() != "2m3s" {
		t.Errorf("duration = %s, want 2m3s", got.Duration)
	}
	if len(got.Steps) != 3 {
		t.Fatalf("steps = %+v, want three", got.Steps)
	}
	if got.Steps[0].State != CheckStateSuccess || got.Steps[0].Duration.String() != "4s" {
		t.Errorf("setup = %+v, want a four-second pass", got.Steps[0])
	}
	if got.Steps[1].State != CheckStateFailure || got.Steps[1].Duration.String() != "1m59s" {
		t.Errorf("tests = %+v, want a 1m59s failure", got.Steps[1])
	}
	if got.Steps[2].State != CheckStateSkipped || got.Steps[2].Duration != 0 {
		t.Errorf("publish = %+v, want a skipped step with no duration", got.Steps[2])
	}
}

func TestJobForARepoWithoutAnOwnerIsRefusedBeforeTheRequest(t *testing.T) {
	rest := &fakeREST{body: `{}`}
	if _, err := newWithDoer(nil, rest).Job(context.Background(), "zen-octo", 9001); err == nil {
		t.Fatal("want an error for a repo with no owner")
	}
	if rest.gotPath != "" {
		t.Errorf("requested %q, want no request at all", rest.gotPath)
	}
}

func TestJobLogsAsksTheJobsLogEndpoint(t *testing.T) {
	rest := &fakeREST{body: "line one\nline two\n"}

	got, err := newWithDoer(nil, rest).JobLogs(context.Background(), "zen-octo/zen-octo", 9001)
	if err != nil {
		t.Fatalf("JobLogs: %v", err)
	}
	if rest.gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", rest.gotMethod)
	}
	if want := "repos/zen-octo/zen-octo/actions/jobs/9001/logs"; rest.gotPath != want {
		t.Errorf("path = %q, want %q", rest.gotPath, want)
	}
	// A log is plain text, not JSON: this proves JobLogs never tries to decode
	// it the way every other REST call in this package does.
	if string(got) != "line one\nline two\n" {
		t.Errorf("body = %q, want the log verbatim", got)
	}
}

func TestJobLogsForARepoWithoutAnOwnerIsRefusedBeforeTheRequest(t *testing.T) {
	rest := &fakeREST{body: "irrelevant"}
	if _, err := newWithDoer(nil, rest).JobLogs(context.Background(), "zen-octo", 9001); err == nil {
		t.Fatal("want an error for a repo with no owner")
	}
	if rest.gotPath != "" {
		t.Errorf("requested %q, want no request at all", rest.gotPath)
	}
}

func TestRerunJobPostsToTheSelectedJobsEndpoint(t *testing.T) {
	rest := &fakeREST{}

	if err := newWithDoer(nil, rest).RerunJob(context.Background(), "zen-octo/zen-octo", 9001); err != nil {
		t.Fatalf("RerunJob: %v", err)
	}
	if rest.gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", rest.gotMethod)
	}
	if want := "repos/zen-octo/zen-octo/actions/jobs/9001/rerun"; rest.gotPath != want {
		t.Errorf("path = %q, want %q", rest.gotPath, want)
	}
	if rest.gotBody != "" {
		t.Errorf("body = %q, want no body sent", rest.gotBody)
	}
}

func TestRerunFailedJobsPostsToTheRerunFailedEndpoint(t *testing.T) {
	rest := &fakeREST{}

	if err := newWithDoer(nil, rest).RerunFailedJobs(context.Background(), "zen-octo/zen-octo", 555200001); err != nil {
		t.Fatalf("RerunFailedJobs: %v", err)
	}
	if rest.gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", rest.gotMethod)
	}
	if want := "repos/zen-octo/zen-octo/actions/runs/555200001/rerun-failed-jobs"; rest.gotPath != want {
		t.Errorf("path = %q, want %q", rest.gotPath, want)
	}
	if rest.gotBody != "" {
		t.Errorf("body = %q, want no body sent", rest.gotBody)
	}
}

func TestRerunAllJobsPostsToTheRerunEndpoint(t *testing.T) {
	rest := &fakeREST{}

	if err := newWithDoer(nil, rest).RerunAllJobs(context.Background(), "zen-octo/zen-octo", 555200001); err != nil {
		t.Fatalf("RerunAllJobs: %v", err)
	}
	if rest.gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", rest.gotMethod)
	}
	if want := "repos/zen-octo/zen-octo/actions/runs/555200001/rerun"; rest.gotPath != want {
		t.Errorf("path = %q, want %q", rest.gotPath, want)
	}
}

func TestRerunAllJobsForARepoWithoutAnOwnerIsRefusedBeforeTheRequest(t *testing.T) {
	rest := &fakeREST{}
	if err := newWithDoer(nil, rest).RerunAllJobs(context.Background(), "zen-octo", 1); err == nil {
		t.Fatal("want an error for a repo with no owner")
	}
	if rest.gotPath != "" {
		t.Errorf("requested %q, want no request at all", rest.gotPath)
	}
}

func TestAForbiddenJobsCallNamesTheScopeToAdd(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Oauth-Scopes", "gist, read:org")
	headers.Set("X-Accepted-Oauth-Scopes", "repo")

	tests := []struct {
		name string
		call func(rest *fakeREST) error
	}{
		{"Job", func(rest *fakeREST) error {
			_, err := newWithDoer(nil, rest).Job(context.Background(), "zen-octo/zen-octo", 1)
			return err
		}},
		{"JobLogs", func(rest *fakeREST) error {
			_, err := newWithDoer(nil, rest).JobLogs(context.Background(), "zen-octo/zen-octo", 1)
			return err
		}},
		{"RerunJob", func(rest *fakeREST) error {
			return newWithDoer(nil, rest).RerunJob(context.Background(), "zen-octo/zen-octo", 1)
		}},
		{"RerunFailedJobs", func(rest *fakeREST) error {
			return newWithDoer(nil, rest).RerunFailedJobs(context.Background(), "zen-octo/zen-octo", 1)
		}},
		{"RerunAllJobs", func(rest *fakeREST) error {
			return newWithDoer(nil, rest).RerunAllJobs(context.Background(), "zen-octo/zen-octo", 1)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rest := &fakeREST{err: &api.HTTPError{StatusCode: 403, Headers: headers}}
			err := tt.call(rest)

			var scope *ScopeError
			if !errors.As(err, &scope) {
				t.Fatalf("err = %v, want a ScopeError", err)
			}
			if !strings.Contains(err.Error(), "gh auth refresh -s repo") {
				t.Errorf("err = %q, want the refresh command", err)
			}
		})
	}
}

func TestARerunTransportErrorIsWrappedNotSwallowed(t *testing.T) {
	wantErr := errors.New("connection reset")
	rest := &fakeREST{err: wantErr}

	err := newWithDoer(nil, rest).RerunAllJobs(context.Background(), "zen-octo/zen-octo", 1)
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want it to wrap %v", err, wantErr)
	}
}
