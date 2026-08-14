package app_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
)

// mentionSet is a repository's mentionable users, one of them somebody who has
// never touched the sample pull request.
func mentionSet() []gh.Mention {
	return []gh.Mention{
		{Login: "nkr", Name: "Nikita Rushmanov"},
		{Login: "sam", Name: "Sam Reed"},
	}
}

// typeInto sends one printable key at a time, the way a reader writes.
func typeInto(m tea.Model, text string) tea.Model {
	for _, r := range text {
		m = settle(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// mentioning opens the sample pull request, opens the compose box, and types an
// @word into it.
func mentioning(t *testing.T, client *fakeSearcher, token string) tea.Model {
	t.Helper()

	m := press(loaded(t, client, 160, 40), "enter", "c")
	return typeInto(m, token)
}

// Nothing at startup and nothing on open. The list costs a request, and a
// reader who never writes a comment should never pay for it.
func TestThePeopleAreNotFetchedUntilSomebodyNeedsThem(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveRepoMeta(gh.RepoMeta{Mentions: mentionSet()})

	m := press(loaded(t, client, 160, 40), "enter")
	render(t, m)

	if got := client.metaCalls(); len(got) != 0 {
		t.Errorf("the repository was asked about %v before anything needed it", got)
	}
}

func TestTheFirstAtFetchesTheRepositorysPeople(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveRepoMeta(gh.RepoMeta{Mentions: mentionSet()})

	m := mentioning(t, client, "@")

	if got, want := client.metaCalls(), []string{"zen-octo/zen-octo"}; !slices.Equal(got, want) {
		t.Errorf("metaCalls = %v, want %v", got, want)
	}
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Sam Reed") {
		t.Errorf("the fetched people are not on the frame:\n%s", out)
	}
}

// The list belongs to the repository and the cache is the root's, so a picker
// opened earlier has already paid for it.
func TestTheMentionListCostsOneRequestForTheWholeSession(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveRepoMeta(gh.RepoMeta{
		Labels:   []gh.Label{{ID: "L_bug", Name: "bug"}},
		Mentions: mentionSet(),
	})

	// Open the label picker from the rail first, then leave it.
	m := press(loaded(t, client, 160, 40), "enter", "2", "j", "j", "j", "enter")
	if out := stripANSI(render(t, m)); !strings.Contains(out, "space toggle") {
		t.Fatalf("setup: the label picker did not open:\n%s", out)
	}
	m = press(m, "esc", "1", "c")
	m = typeInto(m, "@")

	if got, want := client.metaCalls(), []string{"zen-octo/zen-octo"}; !slices.Equal(got, want) {
		t.Errorf("metaCalls = %v, want %v", got, want)
	}
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Sam Reed") {
		t.Errorf("the held people never reached the popup:\n%s", out)
	}
}

// The toast is over the pane and gone in seconds. The popup is under the caret
// and has to say for itself that the list is not coming, or a short list of
// participants reads as everybody there is.
func TestAFailedPeopleFetchReachesThePopupAndNotJustTheToast(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs(), metaErr: errors.New("boom")}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	out := stripANSI(render(t, mentioning(t, client, "@")))

	if !strings.Contains(out, "Could not read the repository") {
		t.Errorf("neither the popup nor the toast reports the failure:\n%s", out)
	}
	if strings.Contains(out, "Loading people") {
		t.Errorf("a failed fetch still reads as one on its way:\n%s", out)
	}
}

// One esc closes the popup and the next gives the keyboard back. A leaked esc
// would close the box on the first press, and on an edit it throws the draft
// away: the reader dismissing a list of names would lose the comment.
func TestTheFirstEscapeClosesTheListAndTheSecondClosesTheBox(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveRepoMeta(gh.RepoMeta{Mentions: mentionSet()})

	m := press(mentioning(t, client, "@nk"), "esc")
	out := stripANSI(render(t, m))
	if strings.Contains(out, "Nikita Rushmanov") {
		t.Errorf("the first escape left the popup up:\n%s", out)
	}
	if !strings.Contains(out, "esc done") {
		t.Errorf("the first escape took the keyboard out of the box:\n%s", out)
	}

	if out := stripANSI(render(t, press(m, "esc"))); strings.Contains(out, "esc done") {
		t.Errorf("the second escape left the box holding the keyboard:\n%s", out)
	}
}

// The sync key drops the held choices, and the next @ has to pay for them
// again: what comes back is the point of the key.
func TestRefreshingDropsThePeopleAndTheNextAtFetchesThemAgain(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveRepoMeta(gh.RepoMeta{Mentions: mentionSet()})

	m := mentioning(t, client, "@")
	if got := len(client.metaCalls()); got != 1 {
		t.Fatalf("setup: metaCalls = %d, want 1", got)
	}

	// Two escapes: the first closes the popup and the second gives the keyboard
	// back, which is the whole of the difference between them. Then a space,
	// because the box keeps its words and the @ already in it would run into
	// the next one.
	m = press(m, "esc", "esc", "s", "c")
	typeInto(m, " @")

	if got := len(client.metaCalls()); got != 2 {
		t.Errorf("metaCalls = %d after a sync and a second @, want 2", got)
	}
}
