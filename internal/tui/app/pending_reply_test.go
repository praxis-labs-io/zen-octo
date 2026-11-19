package app_test

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func answeringIntoReview(t *testing.T, client *fakeSearcher, body string) tea.Model {
	t.Helper()

	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveThread("PR_412")
	client.servePendingReview("PR_412")

	m := press(loaded(t, client, 160, 60), "enter", "2", "}", "r")
	return write(m, body)
}

func header(t *testing.T, m tea.Model) string {
	t.Helper()
	lines := strings.SplitN(stripANSI(render(t, m)), "\n", 3)
	return lines[0] + lines[1]
}

func TestAReplyBoxSaysItAddsToAPendingReview(t *testing.T) {
	client := reviewer()

	out := stripANSI(render(t, answeringIntoReview(t, client, "capped it")))
	for _, want := range []string{"add to review", "Add to review"} {
		if !strings.Contains(out, want) {
			t.Errorf("the reply box does not say %q:\n%s", want, out)
		}
	}
}

func TestAReplyBoxWithNoReviewPendingPosts(t *testing.T) {
	client := reviewer()

	out := stripANSI(render(t, answering(t, client, "capped it")))
	if strings.Contains(out, "Add to review") {
		t.Errorf("the reply box offers a review nobody started:\n%s", out)
	}
	if !strings.Contains(out, "Post") {
		t.Errorf("the reply box lost its button:\n%s", out)
	}
}

func TestAReplyIntoAPendingReviewIsCountedBeforeItLands(t *testing.T) {
	client := reviewer()
	client.holdPosts()

	m := press(answeringIntoReview(t, client, "capped it"), post)

	out := stripANSI(render(t, m))
	if !strings.Contains(out, "adding") {
		t.Errorf("the reply does not say it is being added to the review:\n%s", out)
	}
	if got := header(t, m); !strings.Contains(got, "2 pending comments") {
		t.Errorf("header = %q, want the reply counted", got)
	}
	if got := client.answered(); len(got) != 1 || got[0] != "RT_1 in PRR_1: capped it" {
		t.Errorf("sent %v, want the reply addressed to the pending review", got)
	}
}

func TestAReplyIntoAPendingReviewLandsPending(t *testing.T) {
	client := reviewer()

	m := press(answeringIntoReview(t, client, "capped it"), post)

	out := stripANSI(render(t, m))
	reply := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "drucial · said") {
			reply = line
		}
	}
	if !strings.Contains(reply, "pending") {
		t.Errorf("the landed reply reads published:\n%s", out)
	}
	if got := header(t, m); !strings.Contains(got, "2 pending comments") {
		t.Errorf("header = %q, want the reply counted", got)
	}
	if bar := lastLine(render(t, m)); !strings.Contains(bar, "Added to your review") {
		t.Errorf("status bar = %q, want the reply reported as added to the review", strings.TrimSpace(bar))
	}

	out = stripANSI(render(t, press(m, "S")))
	if !strings.Contains(out, "Publishes 2 pending comments") {
		t.Errorf("the review form does not count the reply:\n%s", out)
	}
}

func TestARefusedReplyIntoAPendingReviewRevertsAndRefetches(t *testing.T) {
	client := reviewer()
	client.postErr = errors.New("422 Unprocessable")

	m := answeringIntoReview(t, client, "capped it")
	before := len(client.opened())
	m = press(m, post)

	out := stripANSI(render(t, m))
	if strings.Count(out, "capped it") != 1 || !strings.Contains(out, "write a reply") {
		t.Errorf("the words did not go back into the box:\n%s", out)
	}
	if got := header(t, m); !strings.Contains(got, "1 pending comment") || strings.Contains(got, "2 pending") {
		t.Errorf("header = %q, want the count back where it was", got)
	}
	if bar := lastLine(render(t, m)); !strings.Contains(bar, "Could not add the reply to your review: 422 Unprocessable") {
		t.Errorf("status bar = %q, want the failure", strings.TrimSpace(bar))
	}
	if got := len(client.opened()); got <= before {
		t.Error("a refused reply into a review was not refetched")
	}
}

func TestAnOpenReplyBoxFollowsAReviewThatEndedElsewhere(t *testing.T) {
	client := reviewer()
	client.postErr = errors.New("422 Unprocessable")

	m := answeringIntoReview(t, client, "capped it")
	client.mu.Lock()
	gone := client.details["PR_412"]
	gone.DraftReview.ID = ""
	client.details["PR_412"] = gone
	client.mu.Unlock()

	out := stripANSI(render(t, press(m, post)))
	if strings.Contains(out, "Add to review") {
		t.Errorf("the box still adds to a review that is gone:\n%s", out)
	}
	if !strings.Contains(out, "capped it") || !strings.Contains(out, "post") {
		t.Errorf("the box did not come back to post directly:\n%s", out)
	}
}

func TestTheReviewWaitsForAReplyStillBeingAdded(t *testing.T) {
	tests := []struct {
		name string
		keys func(t *testing.T, m tea.Model) tea.Model
		gone string
	}{
		{"S", func(_ *testing.T, m tea.Model) tea.Model { return press(m, "S") }, "Review #412"},
		{"D", func(t *testing.T, m tea.Model) tea.Model { return press(onPendingCard(t, m), "D") }, "Discard your review"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := reviewer()
			client.holdPosts()

			m := tt.keys(t, press(answeringIntoReview(t, client, "capped it"), post))

			if out := stripANSI(render(t, m)); strings.Contains(out, tt.gone) {
				t.Errorf("%s went ahead with a reply still on its way:\n%s", tt.name, out)
			}
			if bar := lastLine(render(t, m)); !strings.Contains(bar, "A comment is still being added to your review") {
				t.Errorf("status bar = %q, want it to say why", strings.TrimSpace(bar))
			}
		})
	}
}
