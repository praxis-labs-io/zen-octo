package app_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/app"
	"github.com/praxis-labs-io/zen-octo/internal/tui/keys"
)

// Pressed through the binding, so a rebind of the submit chord moves these tests with it.
var post = keys.Detail.Post.Keys()[0]

type reviewCall struct {
	call  string
	id    string
	event gh.ReviewEvent
	body  string
}

func (f *fakeSearcher) StartReview(_ context.Context, prID string, event gh.ReviewEvent, body string) (gh.ReviewResult, error) {
	return f.review(reviewCall{call: "start", id: prID, event: event, body: body}, prID, "PRR_NEW")
}

func (f *fakeSearcher) SubmitReview(_ context.Context, reviewID string, event gh.ReviewEvent, body string) (gh.ReviewResult, error) {
	f.mu.Lock()
	prID := ""
	for id, d := range f.details {
		if d.DraftReview.ID == reviewID {
			prID = id
		}
	}
	f.mu.Unlock()
	return f.review(reviewCall{call: "submit", id: reviewID, event: event, body: body}, prID, reviewID)
}

// Lands the review on the served detail, so the refetch after it finds what GitHub would.
func (f *fakeSearcher) review(c reviewCall, prID, reviewID string) (gh.ReviewResult, error) {
	f.mu.Lock()
	f.reviewCalls = append(f.reviewCalls, c)
	err, hold := f.postErr, f.postHold
	f.mu.Unlock()

	time.Sleep(hold)
	if err != nil {
		return gh.ReviewResult{}, err
	}

	review := gh.Review{ID: reviewID, State: c.event.Submitted(), Body: c.body}

	f.mu.Lock()
	defer f.mu.Unlock()
	if d, ok := f.details[prID]; ok {
		said := gh.Comment{Kind: gh.CommentReview, ID: reviewID, Author: f.viewer.Viewer, Body: c.body}
		d.Timeline = append(slices.Clone(d.Timeline), gh.TimelineItem{
			Kind: gh.TimelineReview, Actor: f.viewer.Viewer, CreatedAt: time.Now(), Comment: &said,
			Review: review.State,
		})
		d.DraftReview = gh.Review{}
		d.Threads = slices.Clone(d.Threads)
		for i := range d.Threads {
			d.Threads[i].Draft = false
			d.Threads[i].Comments = slices.Clone(d.Threads[i].Comments)
			for j := range d.Threads[i].Comments {
				d.Threads[i].Comments[j].Draft = false
			}
		}
		f.details[prID] = d
	}
	return gh.ReviewResult{Review: review}, nil
}

func (f *fakeSearcher) DiscardReview(_ context.Context, reviewID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.discards = append(f.discards, reviewID)
	return f.postErr
}

func (f *fakeSearcher) reviewsSent() []reviewCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.reviewCalls)
}

func (f *fakeSearcher) discarded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.discards)
}

func (f *fakeSearcher) servePendingReview(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	held := f.details[id]
	held.DraftReview = gh.Review{ID: "PRR_1", State: gh.ReviewStatePending, Body: "Two things."}
	held.Threads = append(slices.Clone(held.Threads), gh.ReviewThread{
		ID: "RT_9", ReviewID: "PRR_1", Path: "internal/store/store.go", Line: 88,
		Side: gh.SideRight, Draft: true,
		Comments: []gh.Comment{{
			Kind: gh.CommentThread, ID: "RC_9", Author: gh.Actor{Login: "octocat"},
			CreatedAt: time.Now(), Body: "Needs a ceiling.", ViewerDidAuthor: true, Draft: true,
		}},
	})
	f.details[id] = held
}

func reviewer() *fakeSearcher {
	return &fakeSearcher{prs: samplePRs(), viewer: gh.ViewerResult{Viewer: gh.Actor{Login: "octocat"}}}
}

func openReviewForm(t *testing.T, client *fakeSearcher, open ...string) tea.Model {
	t.Helper()

	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	m := press(loaded(t, client, 160, 44), append([]string{"enter"}, open...)...)
	m = press(m, "S")
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Review #412") {
		t.Fatalf("S opened no form:\n%s", out)
	}
	return m
}

func onPendingCard(t *testing.T, m tea.Model) tea.Model {
	t.Helper()

	m = press(m, "2")
	for range 12 {
		if strings.Contains(stripANSI(render(t, m)), "D discard") {
			return m
		}
		m = press(m, "}")
	}
	t.Fatalf("no stop on the pending review:\n%s", stripANSI(render(t, m)))
	return m
}

func TestSOpensTheReviewFormOnEveryTab(t *testing.T) {
	for _, tt := range []struct {
		name string
		open []string
	}{
		{"conversation", nil},
		{"commits", []string{"]"}},
		{"checks", []string{"]", "]"}},
		{"files", []string{"]", "]", "]"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out := stripANSI(render(t, openReviewForm(t, reviewer(), tt.open...)))
			for _, want := range []string{"Verdict", "Comment", "Approve", "Request changes", "Summary", "Submit"} {
				if !strings.Contains(out, want) {
					t.Errorf("the form is missing %q:\n%s", want, out)
				}
			}
		})
	}
}

func TestEachVerdictIsSubmittedInOneCall(t *testing.T) {
	for _, tt := range []struct {
		name  string
		moves []string
		want  gh.ReviewEvent
		toast string
	}{
		{"comment", nil, gh.ReviewEventComment, "Reviewed"},
		{"approve", []string{"j"}, gh.ReviewEventApprove, "Approved"},
		{"request changes", []string{"j", "j"}, gh.ReviewEventRequestChanges, "Requested changes"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := reviewer()

			m := press(openReviewForm(t, client), tt.moves...)
			m = press(write(press(m, "tab"), "Two things."), post)

			got := client.reviewsSent()
			want := []reviewCall{{call: "start", id: "PR_412", event: tt.want, body: "Two things."}}
			if !slices.Equal(got, want) {
				t.Fatalf("sent %+v, want %+v", got, want)
			}
			if bar := lastLine(render(t, m)); !strings.Contains(bar, tt.toast) {
				t.Errorf("status bar = %q, want %q", strings.TrimSpace(bar), tt.toast)
			}
		})
	}
}

func TestAReviewWithNothingToSayWaitsForASummary(t *testing.T) {
	for _, tt := range []struct {
		name  string
		moves []string
		hint  string
	}{
		{"comment", nil, "a comment needs a summary"},
		{"request changes", []string{"j", "j"}, "changes need a summary"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := reviewer()

			m := press(openReviewForm(t, client), tt.moves...)
			if out := stripANSI(render(t, m)); !strings.Contains(out, tt.hint) {
				t.Errorf("the form does not say why it cannot submit:\n%s", out)
			}

			press(m, post, "tab", "tab", "enter")
			if got := client.reviewsSent(); len(got) != 0 {
				t.Errorf("sent %+v with nothing to say", got)
			}
		})
	}
}

func TestABareApprovalNeedsNoSummary(t *testing.T) {
	client := reviewer()

	press(openReviewForm(t, client), "j", post)

	got := client.reviewsSent()
	if len(got) != 1 || got[0].event != gh.ReviewEventApprove || got[0].body != "" {
		t.Errorf("sent %+v, want one approval with no summary", got)
	}
}

func TestAnAuthorIsOfferedCommentOnly(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs(), viewer: gh.ViewerResult{Viewer: gh.Actor{Login: "drucial"}}}

	m := openReviewForm(t, client)
	out := stripANSI(render(t, m))
	for _, gone := range []string{"Verdict", "Approve", "Request changes"} {
		if strings.Contains(out, gone) {
			t.Errorf("the author's form offers %q:\n%s", gone, out)
		}
	}

	press(write(m, "Answered below."), post)
	if got := client.reviewsSent(); len(got) != 1 || got[0].event != gh.ReviewEventComment {
		t.Errorf("sent %+v, want one comment", got)
	}
}

func TestASettledPullRequestTakesACommentOnly(t *testing.T) {
	client := reviewer()
	for i := range client.prs {
		client.prs[i].State = gh.PRStateMerged
		client.serveDetail(client.prs[i].ID, "Merged.")
	}

	out := stripANSI(render(t, press(loaded(t, client, 160, 44), "enter", "S")))
	if !strings.Contains(out, "Summary") {
		t.Fatalf("S opened no form on a merged pull request:\n%s", out)
	}
	if strings.Contains(out, "Approve") {
		t.Errorf("a merged pull request offers an approval:\n%s", out)
	}
}

func TestThePendingReviewIsACardAtTheEndOfTheConversation(t *testing.T) {
	client := reviewer()
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveThread("PR_412")
	client.servePendingReview("PR_412")

	out := stripANSI(render(t, press(loaded(t, client, 160, 60), "enter")))
	lines := strings.Split(out, "\n")

	at := func(want string) int {
		for i, line := range lines {
			if strings.Contains(line, want) {
				return i
			}
		}
		t.Fatalf("no %q on the page:\n%s", want, out)
		return -1
	}

	head, note, thread, compose := at("pending review"), at("Only you can see this"),
		at("internal/store/store.go:88"), at("write a comment")
	if head >= note || note >= thread || thread >= compose {
		t.Errorf("rows %d, %d, %d, %d: want the card, its note, its thread, then the compose card:\n%s",
			head, note, thread, compose, out)
	}
	if !strings.Contains(strings.Join(lines[note:thread+2], "\n"), "╰─") {
		t.Errorf("the draft thread does not hang off the pending card:\n%s", out)
	}
	if !strings.Contains(lines[0]+lines[1], "1 pending comment") {
		t.Errorf("the header does not count the pending comment:\n%s", out)
	}
	if strings.Contains(strings.Join(lines[:head], "\n"), "store.go:88") {
		t.Errorf("the draft thread also renders loose above the card:\n%s", out)
	}
}

func TestSubmittingThePendingReviewPublishesItBeforeItLands(t *testing.T) {
	client := reviewer()
	client.holdPosts()
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.servePendingReview("PR_412")

	m := press(loaded(t, client, 160, 60), "enter", "S")
	out := stripANSI(render(t, m))
	for _, want := range []string{"Two things.", "Publishes 1 pending comment"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the form is missing %q:\n%s", want, out)
		}
	}

	m = press(m, "j", post)
	out = stripANSI(render(t, m))
	if strings.Contains(out, "Only you can see this") || strings.Contains(out, "pending comment") {
		t.Errorf("the pending review is still on the page:\n%s", out)
	}
	if !strings.Contains(out, "approved · saving") {
		t.Errorf("the review does not read as an approval being saved:\n%s", out)
	}

	got := client.reviewsSent()
	want := []reviewCall{{call: "submit", id: "PRR_1", event: gh.ReviewEventApprove, body: "Two things."}}
	if !slices.Equal(got, want) {
		t.Errorf("sent %+v, want %+v", got, want)
	}
}

func TestASubmittedReviewTakesTheViewersRowOnTheRail(t *testing.T) {
	client := reviewer()
	client.holdPosts()
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveReviewers("PR_412", []gh.Reviewer{{Actor: gh.Actor{Login: "octocat"}, Requested: true}})

	m := press(loaded(t, client, 160, 60), "enter")
	before := stripANSI(render(t, m))

	m = press(m, "S", "j", post)
	after := stripANSI(render(t, m))
	if before == after || !strings.Contains(after, "octocat") {
		t.Fatalf("the rail did not change:\n%s", after)
	}
	if !strings.Contains(after, "approved") {
		t.Errorf("the viewer's row does not read approved:\n%s", after)
	}
}

func TestARefusedSubmitRevertsAndKeepsTheForm(t *testing.T) {
	client := reviewer()
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.servePendingReview("PR_412")
	client.postErr = errors.New("502 Bad Gateway")

	m := press(loaded(t, client, 160, 60), "enter")
	before := len(client.opened())

	m = press(write(press(m, "S", "j", "j", "tab"), " And a third."), post)

	out := stripANSI(render(t, m))
	if !strings.Contains(out, "Only you can see this") {
		t.Errorf("the pending review did not come back:\n%s", out)
	}
	if bar := lastLine(render(t, m)); !strings.Contains(bar, "Could not submit the review: 502 Bad Gateway") {
		t.Errorf("status bar = %q, want the failure", strings.TrimSpace(bar))
	}
	if got := len(client.opened()); got <= before {
		t.Error("a refused pending review was not refetched")
	}

	out = stripANSI(render(t, press(m, "S")))
	if !strings.Contains(out, "Two things. And a third.") {
		t.Errorf("the reopened form lost the summary:\n%s", out)
	}
	if !strings.Contains(out, "✓ Request changes") {
		t.Errorf("the reopened form lost the verdict:\n%s", out)
	}
}

func TestARefusedNewReviewOnlyReverts(t *testing.T) {
	client := reviewer()
	client.postErr = errors.New("422 Unprocessable")

	m := openReviewForm(t, client)
	before := len(client.opened())
	press(write(press(m, "tab"), "Looks fine."), post)

	if got := len(client.opened()); got != before {
		t.Errorf("fetched %d more times, want a plain revert", got-before)
	}
}

func TestEscKeepsTheFormForTheNextOpen(t *testing.T) {
	client := reviewer()

	m := press(write(press(openReviewForm(t, client), "j", "tab"), "Ship it."), "esc")
	if out := stripANSI(render(t, m)); strings.Contains(out, "Review #412") {
		t.Fatalf("esc left the form up:\n%s", out)
	}

	out := stripANSI(render(t, press(m, "S")))
	if !strings.Contains(out, "Ship it.") || !strings.Contains(out, "✓ Approve") {
		t.Errorf("the form did not come back as it was left:\n%s", out)
	}
}

func TestEnterOnThePendingCardOpensTheForm(t *testing.T) {
	client := reviewer()
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.servePendingReview("PR_412")

	m := onPendingCard(t, press(loaded(t, client, 160, 60), "enter"))
	if out := stripANSI(render(t, press(m, "enter"))); !strings.Contains(out, "Review #412") {
		t.Errorf("enter on the pending card opened no form:\n%s", out)
	}
}

func TestDiscardingThePendingReviewAsksFirst(t *testing.T) {
	client := reviewer()
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.servePendingReview("PR_412")

	m := onPendingCard(t, press(loaded(t, client, 160, 60), "enter"))

	m = press(m, "D")
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Discard your review and its 1 comment?") {
		t.Fatalf("D opened no confirm:\n%s", out)
	}
	m = press(m, "enter")
	if got := client.discarded(); len(got) != 0 {
		t.Fatalf("enter on Keep it discarded %v", got)
	}

	m = press(onPendingCard(t, m), "D", "j", "enter")
	if got := client.discarded(); !slices.Equal(got, []string{"PRR_1"}) {
		t.Errorf("discarded %v, want the pending review", got)
	}
	out := stripANSI(render(t, m))
	if strings.Contains(out, "Only you can see this") || strings.Contains(out, "Needs a ceiling.") {
		t.Errorf("the review or its comment is still on the page:\n%s", out)
	}
	if bar := lastLine(render(t, m)); !strings.Contains(bar, "Discarded") {
		t.Errorf("status bar = %q, want the discard reported", strings.TrimSpace(bar))
	}
}

func TestARefusedDiscardPutsTheReviewBack(t *testing.T) {
	client := reviewer()
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.servePendingReview("PR_412")
	client.postErr = errors.New("502 Bad Gateway")

	m := press(onPendingCard(t, press(loaded(t, client, 160, 60), "enter")), "D", "j", "enter")

	if out := stripANSI(render(t, m)); !strings.Contains(out, "Only you can see this") {
		t.Errorf("the pending review did not come back:\n%s", out)
	}
	if bar := lastLine(render(t, m)); !strings.Contains(bar, "Could not discard the review") {
		t.Errorf("status bar = %q, want the failure", strings.TrimSpace(bar))
	}
}

func TestTheBarNamesTheReviewKey(t *testing.T) {
	client := reviewer()
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	if bar := lastLine(render(t, press(loaded(t, client, 160, 40), "enter"))); !strings.Contains(bar, "S review") {
		t.Errorf("status bar = %q, want the review key", strings.TrimSpace(bar))
	}

	client.servePendingReview("PR_412")
	m := press(loaded(t, client, 160, 40), "enter")
	if bar := lastLine(render(t, m)); !strings.Contains(bar, "S submit review") {
		t.Errorf("status bar = %q, want the submit named while a review is pending", strings.TrimSpace(bar))
	}
}

func TestTheMergeFormWarnsOfAPendingReview(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveMergeable("PR_412")
	client.servePendingReview("PR_412")
	client.serveRepoMeta(gh.RepoMeta{Methods: gh.MergeMethods{Squash: true}})

	out := stripANSI(render(t, press(loaded(t, client, 160, 44), "enter", "1", "j", "j", "j", "j", "j", "enter")))
	if !strings.Contains(out, "Your 1 pending comment won't be published") {
		t.Errorf("the form does not warn of the pending review:\n%s", out)
	}
}

func TestTheTallestReviewFormFitsTheNarrowestFrame(t *testing.T) {
	client := reviewer()
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.servePendingReview("PR_412")

	m := press(atTheFloor(t, client), "enter", "S")

	out := stripANSI(render(t, m))
	for _, want := range []string{"Request changes", "Publishes 1 pending comment", "esc cancel", "Submit"} {
		if !strings.Contains(out, want) {
			t.Errorf("the form is missing %q at %dx%d:\n%s", want, app.MinWidth, app.MinHeight, out)
		}
	}

	lines := strings.Split(out, "\n")
	foot := strings.Join(lines[min(len(lines), footerRow(t, lines)+1):], "\n")
	if !strings.Contains(foot, "╯") {
		t.Errorf("the form has no bottom border at %dx%d:\n%s", app.MinWidth, app.MinHeight, out)
	}
}
