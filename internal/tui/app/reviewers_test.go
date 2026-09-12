package app_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

func reviewing(t *testing.T, client *fakeSearcher) tea.Model {
	t.Helper()

	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveRepoMeta(gh.RepoMeta{Users: repoUserSet()})

	return press(loaded(t, client, 160, 40), "enter", "1", "j")
}

func openReviewerPicker(t *testing.T, client *fakeSearcher) tea.Model {
	t.Helper()

	m := press(reviewing(t, client), "enter")
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Reviewers") {
		t.Fatalf("the reviewer picker did not open:\n%s", out)
	}
	return m
}

func TestAReviewerReadsOnTheRailBeforeItLands(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()

	m := press(openReviewerPicker(t, client), "down", "space", "enter")

	if out := stripANSI(render(t, m)); !strings.Contains(out, "@nkr") {
		t.Errorf("the new reviewer is not on the rail before the write landed:\n%s", out)
	}
	if got, want := client.reviewerWrites(), []string{"+acme/rocket#412: nkr"}; !slices.Equal(got, want) {
		t.Errorf("sent %v, want the request addressed by repository and number", got)
	}
}

func TestRequestingCopilotSendsItAsAReviewer(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := openReviewerPicker(t, client)
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Copilot") {
		t.Fatalf("Copilot is not on the picker:\n%s", out)
	}

	press(m, "space", "enter")

	want := []string{"+acme/rocket#412: " + gh.CopilotLogin}
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

func TestCancellingAReviewRequestSaysSo(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveRepoMeta(gh.RepoMeta{Users: repoUserSet()})
	client.serveReviewers("PR_412", []gh.Reviewer{{Actor: gh.Actor{Login: "nkr"}, Requested: true}})

	m := press(loaded(t, client, 160, 40), "enter", "1", "j", "enter")
	m = press(m, "down", "space", "enter")

	if !strings.Contains(lastLine(render(t, m)), "Cancelled 1 review request") {
		t.Errorf("status bar = %q, want the cancellation reported", strings.TrimSpace(lastLine(render(t, m))))
	}
	if got, want := client.reviewerWrites(), []string{"-acme/rocket#412: nkr"}; !slices.Equal(got, want) {
		t.Errorf("sent %v, want the removal alone", got)
	}
}

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

func TestAReviewerWriteRefetchesTheDetail(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := openReviewerPicker(t, client)
	before := len(client.opened())

	press(m, "down", "space", "enter")

	if got := len(client.opened()); got <= before {
		t.Errorf("the detail was fetched %d times, want another after the write", got-before)
	}
}

func TestAReviewerWriteRaisesOneToast(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := press(openReviewerPicker(t, client), "down", "space", "enter")

	if got := lastLine(render(t, m)); strings.Contains(got, "Refreshed") {
		t.Errorf("status bar = %q, want the write's own toast rather than the sync's", strings.TrimSpace(got))
	}
}

func TestASyncDoesNotUndoAReviewerWriteStillInFlight(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()

	m := press(press(openReviewerPicker(t, client), "down", "space", "enter"), "s")

	if out := stripANSI(render(t, m)); !strings.Contains(out, "@nkr") {
		t.Errorf("the sync dropped a reviewer whose write is still on its way:\n%s", out)
	}
}

func TestSwappingReviewersCancelsBeforeItAsks(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveRepoMeta(gh.RepoMeta{Users: repoUserSet()})
	client.serveReviewers("PR_412", []gh.Reviewer{{Actor: gh.Actor{Login: "nkr"}, Requested: true}})

	m := press(loaded(t, client, 160, 40), "enter", "1", "j", "enter")
	m = press(m, "space", "down", "space", "enter")

	want := []string{
		"-acme/rocket#412: nkr",
		"+acme/rocket#412: " + gh.CopilotLogin,
	}
	if got := client.reviewerWrites(); !slices.Equal(got, want) {
		t.Errorf("sent %v, want %v", got, want)
	}
	if got := lastLine(render(t, m)); !strings.Contains(got, "Reviewers updated") {
		t.Errorf("status bar = %q, want one toast covering both directions", strings.TrimSpace(got))
	}
}

func TestAReviewerWriteThatFailsHalfwayCorrectsTheRail(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs(), requestErr: errors.New("422 Unprocessable Entity")}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveRepoMeta(gh.RepoMeta{Users: repoUserSet()})
	client.serveReviewers("PR_412", []gh.Reviewer{{Actor: gh.Actor{Login: "nkr"}, Requested: true}})

	m := press(loaded(t, client, 160, 40), "enter", "1", "j", "enter")
	before := len(client.opened())
	m = press(m, "space", "down", "space", "enter")

	if !strings.Contains(lastLine(render(t, m)), "422 Unprocessable Entity") {
		t.Errorf("status bar = %q, want the reason still on it", strings.TrimSpace(lastLine(render(t, m))))
	}
	if got := len(client.opened()); got <= before {
		t.Errorf("the detail was fetched %d more times, want the failure to correct the rail", got-before)
	}
	if out := stripANSI(render(t, m)); strings.Contains(out, "@nkr") {
		t.Errorf("the rail still claims a review request the failed write cancelled:\n%s", out)
	}
}
