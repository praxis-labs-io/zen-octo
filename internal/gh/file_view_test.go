package gh

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSetFileViewedUsesTheMatchingMutation(t *testing.T) {
	tests := []struct {
		name   string
		viewed bool
		body   string
		query  string
	}{
		{"viewed", true, `{"markFileAsViewed":{"pullRequest":{"id":"PR_17"}}}`, "markFileAsViewed"},
		{"unviewed", false, `{"unmarkFileAsViewed":{"pullRequest":{"id":"PR_17"}}}`, "unmarkFileAsViewed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doer := &fakeDoer{body: tt.body}
			if err := newWithDoer(doer, nil).SetFileViewed(
				context.Background(), "PR_17", "internal/gh/files.go", tt.viewed,
			); err != nil {
				t.Fatalf("SetFileViewed: %v", err)
			}
			if !strings.Contains(doer.gotQuery, tt.query) {
				t.Errorf("query does not contain %q", tt.query)
			}
			if doer.gotVars["pullRequestId"] != "PR_17" || doer.gotVars["path"] != "internal/gh/files.go" {
				t.Errorf("vars = %#v", doer.gotVars)
			}
		})
	}
}

func TestSetFileViewedRejectsAnEmptyPayload(t *testing.T) {
	err := newWithDoer(&fakeDoer{body: `{}`}, nil).
		SetFileViewed(context.Background(), "PR_17", "a.go", true)
	if err == nil || !strings.Contains(err.Error(), "no pull request") {
		t.Fatalf("err = %v, want the empty payload named", err)
	}
}

func TestSetFileViewedClassifiesGraphQLErrors(t *testing.T) {
	err := errors.New("refused")
	got := newWithDoer(&fakeDoer{err: err}, nil).
		SetFileViewed(context.Background(), "PR_17", "a.go", false)
	if !errors.Is(got, err) {
		t.Fatalf("err = %v, want wrapped refusal", got)
	}
}
