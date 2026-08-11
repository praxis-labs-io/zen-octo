package app_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
)

// reviewing opens the staged pull request with the rail focused and its cursor
// on the row that adds a reviewer.
//
// The tab count is the rail's own order: the state row, then this one. A change
// to that order fails the picker assertion in every test below rather than
// passing quietly.
func reviewing(t *testing.T, client *fakeSearcher) tea.Model {
	t.Helper()

	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveRepoMeta(gh.RepoMeta{Users: repoUserSet()})

	return press(loaded(t, client, 160, 40), "enter", "2", "tab", "tab")
}

func openReviewerPicker(t *testing.T, client *fakeSearcher) tea.Model {
	t.Helper()

	m := press(reviewing(t, client), "enter")
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Reviewers") {
		t.Fatalf("the reviewer picker did not open:\n%s", out)
	}
	return m
}

// The rail changing is the acknowledgement, the same way the optimistic comment
// is one for a comment.
func TestAReviewerReadsOnTheRailBeforeItLands(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()

	m := press(openReviewerPicker(t, client), "down", "space", "enter") // @nkr

	if out := stripANSI(render(t, m)); !strings.Contains(out, "@nkr") {
		t.Errorf("the new reviewer is not on the rail before the write landed:\n%s", out)
	}
	if got, want := client.reviewerWrites(), []string{"+zen-octo/zen-octo#412: nkr"}; !slices.Equal(got, want) {
		t.Errorf("sent %v, want the request addressed by repository and number", got)
	}
}

// Copilot is the reason this write goes over REST at all. It has to leave as
// the bot login and its own row has to name it something a reader recognises.
func TestRequestingCopilotSendsItAsAReviewer(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := openReviewerPicker(t, client)
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Copilot") {
		t.Fatalf("Copilot is not on the picker:\n%s", out)
	}

	press(m, "space", "enter") // Copilot is the first row

	want := []string{"+zen-octo/zen-octo#412: " + gh.CopilotLogin}
	if got := client.reviewerWrites(); !slices.Equal(got, want) {
		t.Errorf("sent %v, want %v", got, want)
	}
}

func TestAReviewerWriteThatLandsSaysWhatItDid(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := press(openReviewerPicker(t, client), "down", "space", "enter")

	if !strings.Contains(lastLine(render(t, m)), "Requested 1 review") {
		t.Errorf("status bar = %q, want the write reported", strings.TrimSpace(lastLine(render(t, m))))
	}
	if out := stripANSI(render(t, m)); !strings.Contains(out, "@nkr") {
		t.Errorf("the reviewer came off the rail after landing:\n%s", out)
	}
}

// Cancelling is its own sentence. "Reviewers updated" over a request that was
// taken away reads as one that was added.
func TestCancellingAReviewRequestSaysSo(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveRepoMeta(gh.RepoMeta{Users: repoUserSet()})
	client.serveReviewers("PR_412", []gh.Reviewer{{Actor: gh.Actor{Login: "nkr"}}})

	m := press(loaded(t, client, 160, 40), "enter", "2", "tab", "tab", "enter")
	m = press(m, "down", "space", "enter") // uncheck @nkr

	if !strings.Contains(lastLine(render(t, m)), "Cancelled 1 review request") {
		t.Errorf("status bar = %q, want the cancellation reported", strings.TrimSpace(lastLine(render(t, m))))
	}
	if got, want := client.reviewerWrites(), []string{"-zen-octo/zen-octo#412: nkr"}; !slices.Equal(got, want) {
		t.Errorf("sent %v, want the removal alone", got)
	}
}

// The revert branch. Nothing was typed, so the fetched panel going back on the
// rail is the whole of it, and the toast carries the reason.
func TestAFailedReviewerWritePutsTheFetchedPanelBack(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs(), postErr: errors.New("502 Bad Gateway")}

	m := press(openReviewerPicker(t, client), "down", "space", "enter")

	if out := stripANSI(render(t, m)); strings.Contains(out, "@nkr") {
		t.Errorf("the reviewer stayed on the rail after the write failed:\n%s", out)
	}
	if !strings.Contains(lastLine(render(t, m)), "502 Bad Gateway") {
		t.Errorf("status bar = %q, want the reason on it", strings.TrimSpace(lastLine(render(t, m))))
	}
}

// The endpoint says nothing about who has already reviewed, and asking for a
// review rewrites the decision the header renders. Neither is something the
// store can compute, so the write asks for the whole detail again.
func TestAReviewerWriteRefetchesTheDetail(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := openReviewerPicker(t, client)
	before := len(client.opened())

	press(m, "down", "space", "enter")

	if got := len(client.opened()); got <= before {
		t.Errorf("the detail was fetched %d times, want another after the write", got-before)
	}
}

// The refetch borrows no refresh leg, so nothing raises "Refreshed" behind the
// toast that already said what happened.
func TestAReviewerWriteRaisesOneToast(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := press(openReviewerPicker(t, client), "down", "space", "enter")

	if got := lastLine(render(t, m)); strings.Contains(got, "Refreshed") {
		t.Errorf("status bar = %q, want the write's own toast rather than the sync's", strings.TrimSpace(got))
	}
}

// A sync landing while a write is out must not put the old panel back. The
// store holds the edit beside the fetched detail for exactly this.
func TestASyncDoesNotUndoAReviewerWriteStillInFlight(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()

	m := press(press(openReviewerPicker(t, client), "down", "space", "enter"), "s")

	if out := stripANSI(render(t, m)); !strings.Contains(out, "@nkr") {
		t.Errorf("the sync dropped a reviewer whose write is still on its way:\n%s", out)
	}
}
