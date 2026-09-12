package app_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

func (f *fakeSearcher) serveComment(id, commentID, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.details == nil {
		f.details = make(map[string]gh.PullRequestDetail)
	}

	held := f.details[id]
	c := gh.Comment{
		Kind: gh.CommentIssue, ID: commentID, Author: gh.Actor{Login: "drucial"},
		CreatedAt: time.Now().Add(-time.Hour), Body: body,
		ViewerDidAuthor: true, CanEdit: true, CanDelete: true,
	}
	held.Timeline = append(held.Timeline, gh.TimelineItem{
		Kind: gh.TimelineComment, Actor: c.Author, CreatedAt: c.CreatedAt, Comment: &c,
	})
	f.details[id] = held
}

func (f *fakeSearcher) serveOwnDescription(id, login string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	held := f.details[id]
	held.Author = gh.Actor{Login: login}
	f.details[id] = held
}

func onComment(t *testing.T, client *fakeSearcher) tea.Model {
	t.Helper()

	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveComment("PR_412", "IC_1", "Covrage held at 84.2%.")
	return press(loaded(t, client, 160, 40), "enter", "2", "}")
}

func TestAnEditedCommentIsOnTheScreenBeforeItLands(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()

	m := press(write(press(onComment(t, client), "e"), " Fixed."), "ctrl+enter")

	out := stripANSI(render(t, m))
	if !strings.Contains(out, "Fixed.") {
		t.Errorf("the new words are not on the card:\n%s", out)
	}
	if !strings.Contains(out, "saving") {
		t.Error("the card does not say the edit is still on its way")
	}
	if strings.Contains(out, "posting") {
		t.Error("an edit reads as a comment being posted")
	}

	if got := client.edits(); len(got) != 1 ||
		!strings.HasPrefix(got[0], "ISSUE IC_1: Covrage held at 84.2%. Fixed.") {
		t.Errorf("wrote %v, want the whole new body against the comment's id", got)
	}
}

func TestAnEditThatLandsLosesItsMarkerAndSaysSo(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := press(write(press(onComment(t, client), "e"), " Fixed."), "ctrl+enter")

	out := stripANSI(render(t, m))
	if !strings.Contains(out, "Fixed.") {
		t.Fatalf("the edit left the card when it landed:\n%s", out)
	}
	if strings.Contains(out, "saving") {
		t.Error("the card still says the edit is on its way after GitHub confirmed it")
	}
	if !strings.Contains(out, "Saved") {
		t.Error("nothing on the bar says the edit landed")
	}
}

func TestAFailedEditPutsTheCommentBackAndKeepsTheWords(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs(), postErr: errors.New("502 Bad Gateway")}

	m := press(write(press(onComment(t, client), "e"), " Fixed."), "ctrl+enter")
	out := stripANSI(render(t, m))

	if !strings.Contains(out, "Fixed.") {
		t.Errorf("the words did not come back anywhere:\n%s", out)
	}
	if !strings.Contains(out, "ctrl+e editor") {
		t.Error("the box did not take the keyboard back with the failed edit")
	}
	if !strings.Contains(out, "502 Bad Gateway") {
		t.Error("the bar does not say why the edit did not save")
	}
}

func TestAConfirmedDeleteTakesTheCommentOffBeforeItLands(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()

	m := press(onComment(t, client), "D", "j", "enter")

	out := stripANSI(render(t, m))
	if strings.Contains(out, "Covrage held at 84.2%") {
		t.Errorf("the comment is still on the page:\n%s", out)
	}
	if got := client.deletedComments(); len(got) != 1 || got[0] != "ISSUE IC_1" {
		t.Errorf("deleted %v, want the comment by kind and id", got)
	}
}

func TestADeleteThatLandsSaysSo(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := press(onComment(t, client), "D", "j", "enter")

	out := stripANSI(render(t, m))
	if strings.Contains(out, "Covrage held at 84.2%") {
		t.Errorf("the comment came back after a delete that worked:\n%s", out)
	}
	if !strings.Contains(out, "Deleted") {
		t.Error("nothing on the bar says the comment went")
	}
}

func TestAFailedDeletePutsTheCommentBack(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs(), postErr: errors.New("403 Forbidden")}

	m := press(onComment(t, client), "D", "j", "enter")

	out := stripANSI(render(t, m))
	if !strings.Contains(out, "Covrage held at 84.2%") {
		t.Errorf("the comment did not come back:\n%s", out)
	}
	if !strings.Contains(out, "403 Forbidden") {
		t.Error("the bar does not say why the comment is still there")
	}
}

func TestCancellingTheConfirmWritesNothing(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := press(onComment(t, client), "D", "enter")

	if got := client.deletedComments(); len(got) != 0 {
		t.Errorf("deleted %v, want nothing", got)
	}
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Covrage held at 84.2%") {
		t.Errorf("the comment went on a cancelled delete:\n%s", out)
	}
}

func TestEditingTheDescriptionWritesThePullRequest(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs(), viewer: gh.ViewerResult{Viewer: gh.Actor{Login: "drucial"}}}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveOwnDescription("PR_412", "drucial")

	m := press(loaded(t, client, 160, 40), "enter", "2")
	m = press(write(press(m, "e"), " And the retry count."), "ctrl+enter")

	if got := client.describes(); len(got) != 1 ||
		!strings.HasSuffix(got[0], "Caps the backoff at 30s. And the retry count.") {
		t.Errorf("wrote %v, want the whole description against the pull request", got)
	}

	out := stripANSI(render(t, m))
	if !strings.Contains(out, "And the retry count.") {
		t.Errorf("the new description is not on the card:\n%s", out)
	}
	if !strings.Contains(out, "Saved") {
		t.Error("nothing on the bar says the description landed")
	}
}

func TestAFailedDescriptionEditKeepsTheWords(t *testing.T) {
	client := &fakeSearcher{
		prs:     samplePRs(),
		postErr: errors.New("502 Bad Gateway"),
		viewer:  gh.ViewerResult{Viewer: gh.Actor{Login: "drucial"}},
	}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveOwnDescription("PR_412", "drucial")

	m := press(loaded(t, client, 160, 40), "enter", "2")
	m = press(write(press(m, "e"), " And the retry count."), "ctrl+enter")

	out := stripANSI(render(t, m))
	if !strings.Contains(out, "And the retry count.") {
		t.Errorf("the words did not come back:\n%s", out)
	}
	if !strings.Contains(out, "502 Bad Gateway") {
		t.Error("the bar does not say why the description did not save")
	}
}

func TestARefreshDoesNotUndoAnEditStillInFlight(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()

	m := press(write(press(onComment(t, client), "e"), " Fixed."), "ctrl+enter")
	m = press(m, "s")

	if out := stripANSI(render(t, m)); !strings.Contains(out, "Fixed.") {
		t.Errorf("the refresh undid an edit still on its way:\n%s", out)
	}
}
