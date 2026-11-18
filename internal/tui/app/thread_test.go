package app_test

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/keys"
)

func (f *fakeSearcher) serveEmptyReview(id, reviewID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	held := f.details[id]
	held.DraftReview = gh.Review{ID: reviewID, State: gh.ReviewStatePending}
	f.details[id] = held
}

// The detail is the caller's to serve, so a test can hold a pending review in it first.
func drafting(t *testing.T, client *fakeSearcher, body string) tea.Model {
	t.Helper()

	client.serveFiles(412, sampleFiles())

	m := press(loaded(t, client, 160, 40), "enter", "]", "]", "]", "}", "j", "j", "c")
	return write(m, body)
}

func TestAThreadIsInTheDiffBeforeItLands(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(drafting(t, client, "name this"), keys.Detail.Post.Keys()[0])

	out := stripANSI(render(t, m))
	if !strings.Contains(out, "name this") || !strings.Contains(out, "posting") {
		t.Errorf("the thread is not in the diff marked as on its way:\n%s", out)
	}

	got := client.threadsOpened()
	if len(got) != 1 {
		t.Fatalf("opened %d threads, want 1", len(got))
	}
	want := gh.ReviewThreadInput{
		PullRequestID: "PR_412", Path: "internal/gh/client.go", Body: "name this", Subject: gh.SubjectLine,
		Line: 41, Side: gh.SideLeft, StartLine: 41, StartSide: gh.SideLeft,
	}
	if got[0] != want {
		t.Errorf("sent %+v, want %+v", got[0], want)
	}
}

func TestAThreadThatLandsReadsPendingAndSaysWhere(t *testing.T) {
	tests := []struct {
		name   string
		review string
		toast  string
	}{
		{"with no pending review", "", "Started a review"},
		{"with one held", "PRR_9", "Added to your review"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeSearcher{prs: samplePRs()}
			client.serveDetail("PR_412", "Caps the backoff at 30s.")
			if tt.review != "" {
				client.serveEmptyReview("PR_412", tt.review)
			}

			m := press(drafting(t, client, "name this"), keys.Detail.Post.Keys()[0])

			out := stripANSI(render(t, m))
			if !strings.Contains(out, "drucial · said · pending") {
				t.Errorf("the landed thread does not read pending:\n%s", out)
			}
			if strings.Contains(out, "posting") {
				t.Error("the landed thread still says it is on its way")
			}
			if got := lastLine(render(t, m)); !strings.Contains(got, tt.toast) {
				t.Errorf("status bar = %q, want %q", strings.TrimSpace(got), tt.toast)
			}
			if got := client.threadsOpened(); len(got) != 1 || got[0].ReviewID != tt.review {
				t.Errorf("sent %+v, want it addressed to review %q", got, tt.review)
			}
		})
	}
}

func TestASecondThreadJoinsTheReviewTheFirstOpened(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(drafting(t, client, "name this"), keys.Detail.Post.Keys()[0])
	m = press(m, "c")
	press(write(m, "and this"), keys.Detail.Post.Keys()[0])

	got := client.threadsOpened()
	if len(got) != 2 || got[1].ReviewID != "PRR_OPENED" {
		t.Errorf("sent %+v, want the second addressed to the review the first opened", got)
	}
}

func TestAFailedThreadGoesBackToItsBox(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs(), postErr: errors.New("502 Bad Gateway")}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(drafting(t, client, "name this"), keys.Detail.Post.Keys()[0])
	out := stripANSI(render(t, m))

	if strings.Count(out, "name this") != 1 {
		t.Errorf("the words are not in exactly one place:\n%s", out)
	}
	if !strings.Contains(out, "comment on line 41") {
		t.Errorf("the box did not reopen on its line:\n%s", out)
	}
	if strings.Contains(out, "posting") {
		t.Error("the thread that failed is still in the diff")
	}
	if !strings.Contains(lastLine(render(t, m)), "502 Bad Gateway") {
		t.Errorf("status bar = %q, want the reason on it", strings.TrimSpace(lastLine(render(t, m))))
	}
}
