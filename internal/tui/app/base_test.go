package app_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func openBasePicker(t *testing.T, client *fakeSearcher) tea.Model {
	t.Helper()

	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveBranches("develop", "main", "release/2.0", "fix-auth")

	return press(toBaseRow(t, client), "enter")
}

func openSearchablePicker(t *testing.T, client *fakeSearcher) tea.Model {
	t.Helper()

	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveBranches("develop", "main", "release/2.0", "release/1.9",
		"feature/rail", "feature/base", "spike/glamour", "fix/scroll")

	m := press(toBaseRow(t, client), "enter")
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Type to filter") {
		t.Fatalf("the picker opened with no filter row, so nothing can be typed:\n%s", out)
	}
	return m
}

func toBaseRow(t *testing.T, client *fakeSearcher) tea.Model {
	t.Helper()

	m := press(loaded(t, client, 160, 44), "enter", "1", "j", "j", "j", "j")
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Up to date with main") {
		t.Fatalf("the rail has no Base row to stand on:\n%s", out)
	}
	return m
}

func TestARetargetReadsOnTheRailBeforeItLands(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()

	m := press(openBasePicker(t, client), "j", "enter")

	if out := stripANSI(render(t, m)); !strings.Contains(out, "Retargeting to") {
		t.Errorf("the rail does not read as retargeting before the write landed:\n%s", out)
	}
	if got := client.retargets(); len(got) != 1 || !strings.HasPrefix(got[0], "PR_412: ") {
		t.Errorf("sent %v, want one retarget addressed to the pull request", got)
	}
}

func TestEnterOnTheBranchAlreadySetWritesNothing(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := press(openBasePicker(t, client), "enter")

	if got := client.retargets(); len(got) != 0 {
		t.Errorf("sent %v, want nothing for a branch that was already set", got)
	}
	if out := stripANSI(render(t, m)); strings.Contains(out, "Merge into") {
		t.Errorf("the picker stayed up after enter:\n%s", out)
	}
}

func TestARetargetThatLandsNamesTheBranch(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := press(openBasePicker(t, client), "j", "enter")

	if bar := lastLine(render(t, m)); !strings.Contains(bar, "Now merging into") {
		t.Errorf("status bar = %q, want the write reported", strings.TrimSpace(bar))
	}
}

func TestARetargetRefetchesTheDetail(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := openBasePicker(t, client)
	before := len(client.opened())
	press(m, "j", "enter")

	if got := len(client.opened()) - before; got != 1 {
		t.Errorf("the detail was fetched %d more times, want 1 after the write settled", got)
	}
}

func TestARetargetRefetchesADiffAlreadyOpened(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveBranches("develop", "main", "release/2.0")
	client.serveFiles(412, sampleFiles())

	m := press(loaded(t, client, 160, 44), "enter", "]", "]", "]")
	if got := client.fetched(); len(got) != 1 {
		t.Fatalf("setup: fetched %v, want the Files tab to have asked for one diff", got)
	}

	m = press(m, "[", "[", "[", "1", "j", "j", "j", "j", "enter")
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Merge into") {
		t.Fatalf("the branch picker did not open:\n%s", out)
	}

	before := len(client.fetched())
	press(m, "j", "enter")

	if got := len(client.fetched()) - before; got != 1 {
		t.Errorf("the diff was fetched %d more times, want 1 after the retarget", got)
	}
}

func TestARetargetDoesNotFetchADiffNobodyAskedFor(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := openBasePicker(t, client)
	before := len(client.fetched())
	press(m, "j", "enter")

	if got := len(client.fetched()) - before; got != 0 {
		t.Errorf("the diff was fetched %d times for a tab nobody opened, want 0", got)
	}
}

func TestARetargetRaisesOneToast(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := press(openBasePicker(t, client), "j", "enter")

	if bar := lastLine(render(t, m)); strings.Contains(bar, "Refreshed") {
		t.Errorf("status bar = %q, want no refresh summary behind the write's own toast",
			strings.TrimSpace(bar))
	}
}

func TestAFailedRetargetPutsTheFetchedBranchBack(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs(), postErr: errors.New("403 Forbidden")}

	m := press(openBasePicker(t, client), "j", "enter")

	out := stripANSI(render(t, m))
	if strings.Contains(out, "Retargeting to") {
		t.Errorf("the rail stayed mid-retarget after the write failed:\n%s", out)
	}
	if !strings.Contains(out, "Up to date with main") {
		t.Errorf("the rail did not go back to the branch it was on:\n%s", out)
	}

	bar := lastLine(render(t, m))
	if !strings.Contains(bar, "403 Forbidden") {
		t.Errorf("status bar = %q, want the reason on it", strings.TrimSpace(bar))
	}
	if !strings.Contains(bar, "Could not merge into") {
		t.Errorf("status bar = %q, want what failed named", strings.TrimSpace(bar))
	}
}

func TestASyncDoesNotUndoARetargetStillInFlight(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()

	m := press(press(openBasePicker(t, client), "j", "enter"), "s")

	if out := stripANSI(render(t, m)); !strings.Contains(out, "Retargeting to") {
		t.Errorf("the sync dropped a retarget still on its way:\n%s", out)
	}
}

func TestTypingIntoTheBranchPickerSearchesOverTheWire(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := press(openSearchablePicker(t, client), "r", "e", "l")

	m = settleSearch(m, "r", "re", "rel")

	got := client.searches()
	if !slices.Contains(got, "rel") {
		t.Fatalf("searches = %v, want one for the whole word typed", got)
	}
	if len(got) != 2 {
		t.Errorf("searches = %v, want the keystrokes folded into one", got)
	}

	if out := stripANSI(render(t, m)); !strings.Contains(out, "release/2.0") {
		t.Errorf("the picker did not take the search's answer:\n%s", out)
	}
}

func TestAFailedBranchSearchSaysSoAndKeepsTheList(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := openSearchablePicker(t, client)
	client.failBranches(errors.New("bad gateway"))
	m = settleSearch(press(m, "r", "e", "l"), "rel")

	out := stripANSI(render(t, m))
	if !strings.Contains(out, "Merge into") {
		t.Errorf("the picker went away when the search failed:\n%s", out)
	}
	if bar := lastLine(render(t, m)); !strings.Contains(bar, "bad gateway") {
		t.Errorf("status bar = %q, want the reason on it", strings.TrimSpace(bar))
	}
}

func TestASecondPullRequestOpensTheBranchPickerToo(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveDetail("PR_408", "Bumps the deps.")
	client.serveBranches("develop", "main", "release/2.0")

	m := press(toBaseRow(t, client), "enter")
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Merge into") {
		t.Fatalf("setup: the first picker did not open:\n%s", out)
	}

	m = press(m, "esc", "esc", "esc", "j", "enter")
	if out := stripANSI(render(t, m)); !strings.Contains(out, "#408") {
		t.Fatalf("setup: did not land on the second pull request:\n%s", out)
	}

	m = press(m, "1", "j", "j", "j", "j", "enter")

	if out := stripANSI(render(t, m)); !strings.Contains(out, "Merge into") {
		t.Errorf("the Base key is dead on the second pull request:\n%s", out)
	}
}

func TestTheSyncKeyReachesTheBranchList(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveBranches("develop", "main")

	m := press(press(toBaseRow(t, client), "enter"), "esc")
	client.serveBranches("develop", "main", "hotfix/urgent")

	m = press(m, "s")
	m = press(m, "enter")

	if out := stripANSI(render(t, m)); !strings.Contains(out, "hotfix/urgent") {
		t.Errorf("the sync key did not reach the branch list:\n%s", out)
	}
}

func TestTheDiffRefetchUsesThePostRetargetFileCount(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveBranches("develop", "main", "release/2.0")
	client.serveFiles(412, sampleFiles())
	client.retargetedFiles = 9

	m := press(loaded(t, client, 160, 44), "enter", "]", "]", "]")
	if got := client.diffAsks(); len(got) != 1 || got[0] != 3 {
		t.Fatalf("setup: diff asks = %v, want one for the staged count of 3", got)
	}

	m = press(m, "[", "[", "[", "1", "j", "j", "j", "j", "enter")
	press(m, "j", "enter")

	got := client.diffAsks()
	if len(got) != 2 {
		t.Fatalf("diff asks = %v, want the retarget to have refetched once", got)
	}
	if got[1] != 9 {
		t.Errorf("the refetch measured against %d files, want the post-retarget 9", got[1])
	}
}
