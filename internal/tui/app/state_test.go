package app_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

func moving(t *testing.T, client *fakeSearcher) tea.Model {
	t.Helper()

	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	return press(loaded(t, client, 160, 40), "enter", "1")
}

func openStateMenu(t *testing.T, client *fakeSearcher) tea.Model {
	t.Helper()

	m := press(moving(t, client), "enter")
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Convert to draft") {
		t.Fatalf("the state menu did not open:\n%s", out)
	}
	return m
}

func TestAStateChangeReadsOnTheRailBeforeItLands(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()

	m := press(openStateMenu(t, client), "enter")

	if out := stripANSI(render(t, m)); !strings.Contains(out, "Draft") {
		t.Errorf("the rail does not read as a draft before the write landed:\n%s", out)
	}
	if got, want := client.stateWrites(), []string{"PR_412: DRAFT"}; !slices.Equal(got, want) {
		t.Errorf("sent %v, want the transition addressed to the pull request", got)
	}
}

func TestClosingSendsTheCloseTransition(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()

	m := press(openStateMenu(t, client), "j", "enter")

	if got, want := client.stateWrites(), []string{"PR_412: CLOSE"}; !slices.Equal(got, want) {
		t.Errorf("sent %v, want a close", got)
	}
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Closed") {
		t.Errorf("the rail does not read as closed before the write landed:\n%s", out)
	}
}

func TestAStateWriteThatLandsSaysSo(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := press(openStateMenu(t, client), "enter")

	if !strings.Contains(lastLine(render(t, m)), "Converted to draft") {
		t.Errorf("status bar = %q, want the write reported", strings.TrimSpace(lastLine(render(t, m))))
	}
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Draft") {
		t.Errorf("the rail went back after landing:\n%s", out)
	}
}

func TestAStateWriteRefetchesTheDetail(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := openStateMenu(t, client)
	before := len(client.opened())
	press(m, "enter")

	if got := len(client.opened()) - before; got != 1 {
		t.Errorf("the detail was fetched %d more times, want 1 after the write settled", got)
	}
}

func TestClosingAPullRequestCorrectsTheRowBehindIt(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	closed := press(openStateMenu(t, client), "j", "enter")

	out := stripANSI(render(t, press(closed, "esc")))
	group, ok := groupOf(t, out, "Fix auth retry")
	if !ok {
		t.Fatalf("the pull request left the list entirely:\n%s", out)
	}
	if group != "Closed" {
		t.Errorf("the row sits under %q, want it under Closed once the write landed:\n%s", group, out)
	}
}

func groupOf(t *testing.T, frame, row string) (string, bool) {
	t.Helper()

	var group string
	for _, line := range strings.Split(frame, "\n") {
		for _, name := range []string{"Ready", "Draft", "Merged", "Closed"} {
			if strings.Contains(line, name) {
				group = name
			}
		}
		if strings.Contains(line, row) {
			return group, true
		}
	}
	return "", false
}

func TestAStateWriteRaisesOneToast(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := press(openStateMenu(t, client), "enter")

	bar := lastLine(render(t, m))
	if strings.Contains(bar, "Refreshed") {
		t.Errorf("status bar = %q, want no refresh summary behind the write's own toast", strings.TrimSpace(bar))
	}
	if !strings.Contains(bar, "Converted to draft") {
		t.Errorf("status bar = %q, want the write reported", strings.TrimSpace(bar))
	}
}

func TestAFailedStateWritePutsTheFetchedStateBack(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs(), postErr: errors.New("403 Forbidden")}

	m := press(openStateMenu(t, client), "enter")

	if out := stripANSI(render(t, m)); strings.Contains(out, "Draft") {
		t.Errorf("the rail stayed a draft after the write failed:\n%s", out)
	}
	bar := lastLine(render(t, m))
	if !strings.Contains(bar, "403 Forbidden") {
		t.Errorf("status bar = %q, want the reason on it", strings.TrimSpace(bar))
	}
	if !strings.Contains(bar, "convert it to a draft") {
		t.Errorf("status bar = %q, want the move that failed named", strings.TrimSpace(bar))
	}
}

func TestASyncDoesNotUndoAStateWriteStillInFlight(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()

	m := press(press(openStateMenu(t, client), "enter"), "s")

	if out := stripANSI(render(t, m)); !strings.Contains(out, "Draft") {
		t.Errorf("the sync dropped a state change still on its way:\n%s", out)
	}
}

func TestTheRailTakesGitHubsAnswerForTheState(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveState("PR_412", gh.PRStateClosed, false)

	m := press(openStateMenu(t, client), "enter")

	if out := stripANSI(render(t, m)); !strings.Contains(out, "Closed") {
		t.Errorf("the rail kept the ask rather than GitHub's answer:\n%s", out)
	}
}

func TestQDoesNotQuitWhileTheStateMenuIsUp(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := press(openStateMenu(t, client), "q")

	if out := stripANSI(render(t, m)); !strings.Contains(out, "Convert to draft") {
		t.Errorf("q reached the root and closed the menu:\n%s", out)
	}
}

func TestASyncInFlightDoesNotUndoALandedStateWrite(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := moving(t, client)

	m, stale := holdBack(m, keyMsg("s"), "detailFetchedMsg")
	if len(stale) == 0 {
		t.Fatal("the sync key started no detail fetch")
	}

	m = press(m, "enter", "j", "enter")
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Closed") {
		t.Fatalf("the close never reached the rail:\n%s", out)
	}

	m = settle(m, stale...)

	if out := stripANSI(render(t, m)); !strings.Contains(out, "Closed") {
		t.Errorf("the stale response put the close back undone:\n%s", out)
	}
}

func TestTheToastNamesWhereItLandedNotWhatWasAsked(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveState("PR_412", gh.PRStateClosed, false)

	m := press(openStateMenu(t, client), "enter")

	bar := lastLine(render(t, m))
	if strings.Contains(bar, "Converted to draft") {
		t.Errorf("status bar = %q, want it not to claim a move that did not happen", strings.TrimSpace(bar))
	}
	if !strings.Contains(strings.ToLower(bar), "closed") {
		t.Errorf("status bar = %q, want the state it landed in", strings.TrimSpace(bar))
	}
}

func TestSyncWaitsOnTheRefetchAWriteStarted(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m, refetch := holdBack(openStateMenu(t, client), keyMsg("enter"), "detailFetchedMsg")
	if len(refetch) == 0 {
		t.Fatal("the write started no refetch")
	}

	m = press(m, "s")
	m = settle(m, refetch...)

	if got := lastLine(render(t, m)); !strings.Contains(got, "Refreshed") {
		t.Errorf("status bar = %q, want the sync reported when the refetch landed", strings.TrimSpace(got))
	}
}
