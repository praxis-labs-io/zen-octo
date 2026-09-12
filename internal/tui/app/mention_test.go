package app_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

func mentionSet() []gh.Mention {
	return []gh.Mention{
		{Login: "nkr", Name: "Nikita Rushmanov"},
		{Login: "sam", Name: "Sam Reed"},
	}
}

func typeInto(m tea.Model, text string) tea.Model {
	for _, r := range text {
		m = settle(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

func mentioning(t *testing.T, client *fakeSearcher, token string) tea.Model {
	t.Helper()

	m := press(loaded(t, client, 160, 40), "enter", "c")
	return typeInto(m, token)
}

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

	if got, want := client.metaCalls(), []string{"acme/rocket"}; !slices.Equal(got, want) {
		t.Errorf("metaCalls = %v, want %v", got, want)
	}
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Sam Reed") {
		t.Errorf("the fetched people are not on the frame:\n%s", out)
	}
}

func TestTheMentionListCostsOneRequestForTheWholeSession(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveRepoMeta(gh.RepoMeta{
		Labels:   []gh.Label{{ID: "L_bug", Name: "bug"}},
		Mentions: mentionSet(),
	})

	m := press(loaded(t, client, 160, 40), "enter", "1", "j", "j", "j", "enter")
	if out := stripANSI(render(t, m)); !strings.Contains(out, "space toggle") {
		t.Fatalf("setup: the label picker did not open:\n%s", out)
	}
	m = press(m, "esc", "1", "c")
	m = typeInto(m, "@")

	if got, want := client.metaCalls(), []string{"acme/rocket"}; !slices.Equal(got, want) {
		t.Errorf("metaCalls = %v, want %v", got, want)
	}
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Sam Reed") {
		t.Errorf("the held people never reached the popup:\n%s", out)
	}
}

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

func TestRefreshingDropsThePeopleAndTheNextAtFetchesThemAgain(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveRepoMeta(gh.RepoMeta{Mentions: mentionSet()})

	m := mentioning(t, client, "@")
	if got := len(client.metaCalls()); got != 1 {
		t.Fatalf("setup: metaCalls = %d, want 1", got)
	}

	m = press(m, "esc", "esc", "s", "c")
	typeInto(m, " @")

	if got := len(client.metaCalls()); got != 2 {
		t.Errorf("metaCalls = %d after a sync and a second @, want 2", got)
	}
}

func TestAskingForThePeopleRestartsTheSpinner(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveRepoMeta(gh.RepoMeta{Mentions: mentionSet()})

	m := press(loaded(t, client, 160, 40), "enter", "c")

	m, cmd := m.Update(tea.KeyPressMsg{Code: '@', Text: "@"})

	ask := findAsk(cmd)
	if ask == nil {
		t.Fatalf("the first @ asked the root for nothing")
	}
	if _, restarted := m.Update(ask); !hasTick(restarted) {
		t.Error("answering the ask started no spinner tick, so the glyph freezes on its first frame")
	}
}

func findAsk(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case prview.NeedRepoMetaMsg:
		return msg
	case tea.BatchMsg:
		for _, c := range msg {
			if found := findAsk(c); found != nil {
				return found
			}
		}
	}
	return nil
}

func hasTick(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	switch msg := cmd().(type) {
	case spinner.TickMsg:
		return true
	case tea.BatchMsg:
		for _, c := range msg {
			if hasTick(c) {
				return true
			}
		}
	}
	return false
}
