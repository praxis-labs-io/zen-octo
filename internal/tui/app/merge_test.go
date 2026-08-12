package app_test

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/app"
)

// serveMergeable stages a pull request GitHub would merge: clean, with the head
// commit and the branch it sits on, and with GitHub's own commit message for
// each of the two methods that write one.
func (f *fakeSearcher) serveMergeable(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	held := f.details[id]
	held.Merge = gh.MergeClean
	held.HeadRefOid = "9f1c2b7"
	held.HeadRefID = "REF_88"
	held.MergeCommit = gh.MergeMessage{Headline: "Merge pull request #412 from zen-octo/fix-auth"}
	held.SquashCommit = gh.MergeMessage{Headline: "Fix auth retry (#412)"}
	f.details[id] = held
}

// toMergeRow opens the staged pull request with the rail focused and its cursor
// on the Merge row, which is the last one.
//
// The tab count is the rail's own order: the state row, the three add rows, the
// base, then this. A change to that order fails the assertion under it in every
// test below rather than passing quietly.
func toMergeRow(t *testing.T, client *fakeSearcher) tea.Model {
	t.Helper()

	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveMergeable("PR_412")
	client.serveRepoMeta(gh.RepoMeta{
		Methods: gh.MergeMethods{Merge: true, Squash: true, Rebase: true},
	})

	m := press(loaded(t, client, 160, 44), "enter", "2",
		"tab", "tab", "tab", "tab", "tab", "tab")
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Ready to merge") {
		t.Fatalf("the rail has no Merge row to stand on:\n%s", out)
	}
	return m
}

// openMergeForm walks to the Merge row and opens the form over the repository's
// merge methods.
func openMergeForm(t *testing.T, client *fakeSearcher) tea.Model {
	t.Helper()

	m := press(toMergeRow(t, client), "enter")
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Squash and merge") {
		t.Fatalf("enter on the Merge row opened no form:\n%s", out)
	}
	return m
}

// pressMerge steps from the method rows to the button and presses it.
func pressMerge(m tea.Model) tea.Model {
	return press(m, "tab", "tab", "tab", "tab", "enter")
}

// The rail changing is the acknowledgement, the way it is for every other write
// the rail makes.
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

// A merge writes the merged event onto the timeline, settles the checks and
// moves what the viewer may do next, and the store can compute none of it.
func TestAMergeRefetchesTheDetail(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := openMergeForm(t, client)
	before := len(client.opened())
	pressMerge(m)

	if got := len(client.opened()); got <= before {
		t.Errorf("the detail was fetched %d times, want another after the merge", got-before)
	}
}

// The branch goes with the merge, because the form opened with the box ticked.
func TestAMergeDeletesTheHeadBranch(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	pressMerge(openMergeForm(t, client))

	if got := client.deletes(); len(got) != 1 || got[0] != "REF_88" {
		t.Errorf("deleted %v, want the head branch's node id", got)
	}
}

// Unticking has to reach the write, or the checkbox is decoration.
func TestAnUntickedFormLeavesTheBranchAlone(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	// Method, headline, message, delete: untick, then on to the button.
	press(openMergeForm(t, client), "tab", "tab", "tab", "space", "tab", "enter")

	if got := client.deletes(); len(got) != 0 {
		t.Errorf("deleted %v, want the branch kept", got)
	}
	if len(client.merges()) != 1 {
		t.Errorf("sent %d merges, want the merge made anyway", len(client.merges()))
	}
}

// Two calls, and the second cannot undo the first. A merge that landed stays
// landed, and the only thing left to do about the branch is say so.
func TestAFailedBranchDeleteLeavesTheMergeStanding(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.deleteErr = errors.New("Reference does not exist")

	m := pressMerge(openMergeForm(t, client))

	if out := stripANSI(render(t, m)); !strings.Contains(out, "Merged into main") {
		t.Errorf("a failed delete took the merge off the rail:\n%s", out)
	}
	if bar := lastLine(render(t, m)); !strings.Contains(bar, "still there") {
		t.Errorf("status bar = %q, want the branch reported as left behind", strings.TrimSpace(bar))
	}
}

// The revert branch. Nothing was typed and no branch was touched, so the
// fetched state going back on the rail is the whole of it.
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

	// GitHub's own sentence, which is what tells the reader to sync.
	if bar := lastLine(render(t, m)); !strings.Contains(bar, "Head branch was modified") {
		t.Errorf("status bar = %q, want GitHub's own refusal", strings.TrimSpace(bar))
	}
	if got := client.deletes(); len(got) != 0 {
		t.Errorf("deleted %v after a merge that never happened", got)
	}
}

// GitHub computes mergeability lazily and the first query is what starts it, so
// a pull request nothing has looked at recently answers UNKNOWN and has a real
// answer a moment later.
func TestADetailThatCannotSayWhetherItMergesIsAskedAgain(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(loaded(t, client, 160, 44), "enter")
	before := len(client.opened())

	// GitHub has worked it out by the time the wait runs out.
	client.serveMergeable("PR_412")
	m = settle(m, app.MergeProbe("PR_412"))

	if got := len(client.opened()); got != before+1 {
		t.Fatalf("the detail was fetched %d more times, want exactly one probe", got-before)
	}
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Ready to merge") {
		t.Errorf("the probe's answer is not on the rail:\n%s", out)
	}
}

// A wait that runs out on a question already answered asks nothing. The answer
// can arrive from the sync key or from a write's own refetch while the wait is
// still out, and a request for something already on the screen is one the
// reader never made.
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
}
