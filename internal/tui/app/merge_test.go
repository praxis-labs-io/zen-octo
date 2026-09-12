package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/app"
)

func (f *fakeSearcher) serveMergeable(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	held := f.details[id]
	held.Merge = gh.MergeClean
	held.HeadRefOid = "9f1c2b7"
	held.HeadRefID = "REF_88"
	held.MergeCommit = gh.MergeMessage{Headline: "Merge pull request #412 from acme/fix-auth"}
	held.SquashCommit = gh.MergeMessage{Headline: "Fix auth retry (#412)"}
	f.details[id] = held
}

func toMergeRow(t *testing.T, client *fakeSearcher) tea.Model {
	t.Helper()

	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveMergeable("PR_412")
	client.serveRepoMeta(gh.RepoMeta{
		Methods: gh.MergeMethods{Merge: true, Squash: true, Rebase: true},
	})

	m := press(loaded(t, client, 160, 44), "enter", "1",
		"j", "j", "j", "j", "j")
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Ready to merge") {
		t.Fatalf("the rail has no Merge row to stand on:\n%s", out)
	}
	return m
}

func openMergeForm(t *testing.T, client *fakeSearcher) tea.Model {
	t.Helper()

	m := press(toMergeRow(t, client), "enter")
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Squash and merge") {
		t.Fatalf("enter on the Merge row opened no form:\n%s", out)
	}
	return m
}

func pressMerge(m tea.Model) tea.Model {
	return press(m, "tab", "tab", "tab", "tab", "enter")
}

func TestAMergeReadsOnTheRailBeforeItLands(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()

	m := pressMerge(openMergeForm(t, client))

	if out := stripANSI(render(t, m)); !strings.Contains(out, "Merged into main") {
		t.Errorf("the rail does not read as merged before the write landed:\n%s", out)
	}
	if got := client.merges(); len(got) != 1 {
		t.Fatalf("sent %d merges, want one", len(got))
	}
	if got := client.merges()[0]; got.Method != gh.MergeMethodSquash || got.ExpectedHeadOid != "9f1c2b7" {
		t.Errorf("merged %+v, want a squash of the head commit", got)
	}
}

func TestAMergeThatLandsNamesTheBranch(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := pressMerge(openMergeForm(t, client))

	if bar := lastLine(render(t, m)); !strings.Contains(bar, "Merged into main") {
		t.Errorf("status bar = %q, want the write reported", strings.TrimSpace(bar))
	}
}

func TestAMergeRefetchesTheDetail(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := openMergeForm(t, client)
	before := len(client.opened())
	pressMerge(m)

	if got := len(client.opened()); got <= before {
		t.Errorf("the detail was fetched %d times, want another after the merge", got-before)
	}
}

func TestAMergeDeletesTheHeadBranch(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	pressMerge(openMergeForm(t, client))

	if got := client.deletes(); len(got) != 1 || got[0] != "REF_88" {
		t.Errorf("deleted %v, want the head branch's node id", got)
	}
}

func TestAnUntickedFormLeavesTheBranchAlone(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	press(openMergeForm(t, client), "tab", "tab", "tab", "space", "tab", "enter")

	if got := client.deletes(); len(got) != 0 {
		t.Errorf("deleted %v, want the branch kept", got)
	}
	if len(client.merges()) != 1 {
		t.Errorf("sent %d merges, want the merge made anyway", len(client.merges()))
	}
}

func TestAFailedBranchDeleteLeavesTheMergeStanding(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.deleteErr = errors.New("Reference does not exist")

	m := pressMerge(openMergeForm(t, client))

	if out := stripANSI(render(t, m)); !strings.Contains(out, "Merged into main") {
		t.Errorf("a failed delete took the merge off the rail:\n%s", out)
	}
	if bar := lastLine(render(t, m)); !strings.Contains(bar, "Could not confirm") {
		t.Errorf("status bar = %q, want the delete reported as unconfirmed", strings.TrimSpace(bar))
	}
}

func TestAFailedMergePutsTheStateBack(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.postErr = errors.New("Head branch was modified. Review and try the merge again.")

	m := pressMerge(openMergeForm(t, client))

	out := stripANSI(render(t, m))
	if strings.Contains(out, "Merged into main") {
		t.Errorf("a refused merge is still on the rail:\n%s", out)
	}
	if !strings.Contains(out, "Ready to merge") {
		t.Errorf("the rail did not go back to what GitHub last said:\n%s", out)
	}

	if bar := lastLine(render(t, m)); !strings.Contains(bar, "Head branch was modified") {
		t.Errorf("status bar = %q, want GitHub's own refusal", strings.TrimSpace(bar))
	}
	if got := client.deletes(); len(got) != 0 {
		t.Errorf("deleted %v after a merge that never happened", got)
	}
}

func TestADetailThatCannotSayWhetherItMergesIsAskedAgain(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(loaded(t, client, 160, 44), "enter")
	before := len(client.opened())

	client.serveMergeable("PR_412")
	m = settle(m, app.MergeProbe("PR_412"))

	if got := len(client.pulsed()); got != 1 {
		t.Fatalf("the probe made %d pulses, want exactly one", got)
	}
	if got := len(client.opened()); got != before {
		t.Errorf("the detail was fetched %d more times, want the probe to cost no page", got-before)
	}
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Ready to merge") {
		t.Errorf("the probe's answer is not on the rail:\n%s", out)
	}
}

func TestTheProbeAsksNothingOnceTheAnswerIsIn(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveMergeable("PR_412")

	m := press(loaded(t, client, 160, 44), "enter")
	before := len(client.opened())

	settle(m, app.MergeProbe("PR_412"))

	if got := len(client.opened()); got != before {
		t.Errorf("the detail was fetched %d more times, want none", got-before)
	}
	if got := client.pulsed(); len(got) != 0 {
		t.Errorf("the probe rechecked %v, want nothing asked once the answer is in", got)
	}
}

func TestAFailedMergeAsksForTheDetailAgain(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.postErr = errors.New("Head branch was modified. Review and try the merge again.")

	m := openMergeForm(t, client)
	before := len(client.opened())
	pressMerge(m)

	if got := len(client.opened()); got <= before {
		t.Errorf("the detail was fetched %d more times, want another after the refusal", got-before)
	}
}

func TestAnAnswerThatIsNotMergedDeletesNothing(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.mergeState = gh.PRStateOpen

	m := pressMerge(openMergeForm(t, client))

	if got := client.deletes(); len(got) != 0 {
		t.Errorf("deleted %v off the back of an answer that was not a merge", got)
	}
	if bar := lastLine(render(t, m)); !strings.Contains(bar, "rather than merged") {
		t.Errorf("status bar = %q, want the answer reported as not a merge", strings.TrimSpace(bar))
	}
}

func TestAProbeSwallowedByAFetchInFlightIsArmedAgain(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(loaded(t, client, 160, 44), "enter")

	m, held := holdBack(m, keyMsg("s"), "detailFetched")
	if len(held) == 0 {
		t.Fatal("setup: no detail response was held, so nothing is in flight")
	}

	_, cmd := m.Update(app.MergeProbe("PR_412"))
	if cmd == nil {
		t.Fatal("the probe was swallowed by the fetch in flight and nothing was armed to ask again")
	}
	if got := client.pulsed(); len(got) != 0 {
		t.Errorf("the probe rechecked %v under a fetch already in flight", got)
	}
}

func TestTheMockupOffersEveryMergeMethod(t *testing.T) {
	res, err := app.Mock{}.RepoMeta(context.Background(), "praxis-labs/zen-octo")
	if err != nil {
		t.Fatalf("RepoMeta: %v", err)
	}

	for _, method := range []gh.MergeMethod{
		gh.MergeMethodMerge, gh.MergeMethodSquash, gh.MergeMethodRebase,
	} {
		if !res.Meta.Methods.Allows(method) {
			t.Errorf("the mockup forbids %s, so its merge form cannot open", method)
		}
	}
}
