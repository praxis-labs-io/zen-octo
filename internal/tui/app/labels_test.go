package app_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
)

func repoLabelSet() []gh.Label {
	return []gh.Label{
		{ID: "LA_1", Name: "bug"},
		{ID: "LA_2", Name: "enhancement"},
	}
}

// labelling opens the staged pull request with the rail focused and its cursor
// on the row that already carries a label.
//
// The tab count is the rail's own order: the state row, the two add rows above
// the labels section, then the label itself. A change to that order fails the
// picker assertion in every test below rather than passing quietly.
func labelling(t *testing.T, client *fakeSearcher) tea.Model {
	t.Helper()

	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveLabels("PR_412", repoLabelSet()[:1])
	client.serveRepoMeta(gh.RepoMeta{Labels: repoLabelSet()})

	return press(loaded(t, client, 160, 40), "enter", "2", "tab", "tab", "tab", "tab")
}

func openLabelPicker(t *testing.T, client *fakeSearcher) tea.Model {
	t.Helper()

	m := press(labelling(t, client), "enter")
	if out := stripANSI(render(t, m)); !strings.Contains(out, "space toggle") {
		t.Fatalf("the label picker did not open:\n%s", out)
	}
	return m
}

func TestThePickerAsksTheRepositoryOnceForItsChoices(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := openLabelPicker(t, client)
	m = press(m, "esc", "enter") // open it a second time

	if out := stripANSI(render(t, m)); !strings.Contains(out, "space toggle") {
		t.Fatalf("the picker did not open a second time:\n%s", out)
	}
	if got, want := client.metaCalls(), []string{"zen-octo/zen-octo"}; !slices.Equal(got, want) {
		t.Errorf("asked %v, want the repository read once and cached", got)
	}
}

// The rail changing is the acknowledgement, the same way the optimistic comment
// is one for a comment.
func TestALabelReadsOnTheRailBeforeItLands(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()

	m := press(openLabelPicker(t, client), "down", "space", "enter")

	if out := stripANSI(render(t, m)); !strings.Contains(out, "enhancement") {
		t.Errorf("the new label is not on the rail before the write landed:\n%s", out)
	}
	if got, want := client.labelWrites(), []string{"PR_412: LA_1,LA_2"}; !slices.Equal(got, want) {
		t.Errorf("sent %v, want the whole set addressed to the pull request", got)
	}
}

func TestALabelWriteThatLandsSaysSo(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := press(openLabelPicker(t, client), "down", "space", "enter")

	if !strings.Contains(lastLine(render(t, m)), "Labels updated") {
		t.Errorf("status bar = %q, want the write reported", strings.TrimSpace(lastLine(render(t, m))))
	}
	if out := stripANSI(render(t, m)); !strings.Contains(out, "enhancement") {
		t.Errorf("the label came off the rail after landing:\n%s", out)
	}
}

// The revert branch. Nothing was typed, so the fetched set going back on the
// rail is the whole of it, and the toast carries the reason.
func TestAFailedLabelWritePutsTheFetchedSetBack(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs(), postErr: errors.New("502 Bad Gateway")}

	m := press(openLabelPicker(t, client), "down", "space", "enter")

	if out := stripANSI(render(t, m)); strings.Contains(out, "enhancement") {
		t.Errorf("the label stayed on the rail after the write failed:\n%s", out)
	}
	if !strings.Contains(lastLine(render(t, m)), "502 Bad Gateway") {
		t.Errorf("status bar = %q, want the reason on it", strings.TrimSpace(lastLine(render(t, m))))
	}
}

// A sync landing while a write is out must not put the old set back. The store
// holds the edit beside the fetched detail for exactly this.
func TestASyncDoesNotUndoALabelWriteStillInFlight(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()

	m := press(press(openLabelPicker(t, client), "down", "space", "enter"), "s")

	if out := stripANSI(render(t, m)); !strings.Contains(out, "enhancement") {
		t.Errorf("the sync dropped a label whose write is still on its way:\n%s", out)
	}
}

// GitHub is the authority on what the pull request ended up carrying. A label
// deleted from the repository since the picker was filled comes back absent.
func TestTheRailTakesGitHubsAnswerRatherThanTheAsk(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	// The repository no longer carries the label the picker offered, so the
	// write comes back without it.
	client.serveRepoMeta(gh.RepoMeta{Labels: repoLabelSet()})

	m := openLabelPicker(t, client)
	client.serveRepoMeta(gh.RepoMeta{Labels: repoLabelSet()[:1]})
	m = press(m, "down", "space", "enter")

	if out := stripANSI(render(t, m)); strings.Contains(out, "enhancement") {
		t.Errorf("the rail kept a label GitHub did not confirm:\n%s", out)
	}
}

func TestAFailedRepositoryReadSaysSoAndOpensNoPicker(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs(), metaErr: errors.New("403 Forbidden")}

	m := press(labelling(t, client), "enter")

	out := stripANSI(render(t, m))
	if strings.Contains(out, "space toggle") {
		t.Errorf("a picker opened over choices that never arrived:\n%s", out)
	}
	if !strings.Contains(lastLine(render(t, m)), "403 Forbidden") {
		t.Errorf("status bar = %q, want the reason on it", strings.TrimSpace(lastLine(render(t, m))))
	}
}

// The root stands aside while a picker is up, the same way it does for a
// comment box. q is a letter in a filter.
func TestQDoesNotQuitWhileAPickerIsUp(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := press(openLabelPicker(t, client), "q")

	if out := stripANSI(render(t, m)); !strings.Contains(out, "space toggle") {
		t.Errorf("q reached the root and closed the picker:\n%s", out)
	}
}
