package prview_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/tui/prview"
)

// tabResolved is where tab stops on the thread the fixture already carries as
// settled, which is the one the key has to reopen.
const tabResolved = 5

// asked presses a key and reads what it sent the root, or nothing.
func asked(t *testing.T, m prview.Model, k string) tea.Msg {
	t.Helper()

	_, cmd := key(m, k)
	if cmd == nil {
		return nil
	}
	return cmd()
}

// resolveAsked is the same, insisting the key asked for a resolve.
func resolveAsked(t *testing.T, m prview.Model, k string) prview.ResolveThreadMsg {
	t.Helper()

	got := asked(t, m, k)
	msg, ok := got.(prview.ResolveThreadMsg)
	if !ok {
		t.Fatalf("%s produced %T, want a ResolveThreadMsg", k, got)
	}
	return msg
}

func TestXAsksToResolveTheFocusedThread(t *testing.T) {
	got := resolveAsked(t, onThread(t, tabThread), "x")

	want := prview.ResolveThreadMsg{ID: "PR_412", ThreadID: "RT_1", Resolved: true}
	if got != want {
		t.Errorf("x asked for %+v, want %+v", got, want)
	}
}

// A settled thread is collapsed, and closed is the state this key exists to
// change. Reading the open threads alone would leave it with no way back.
func TestXOnAResolvedThreadAsksToUnresolve(t *testing.T) {
	m := onThread(t, tabResolved)
	if got := focusedCard(t, m.View()); !strings.Contains(got, "resolved") {
		t.Fatalf("the fifth tab focused %q, want the resolved thread", got)
	}

	got := resolveAsked(t, m, "x")
	want := prview.ResolveThreadMsg{ID: "PR_412", ThreadID: "RT_2"}
	if got != want {
		t.Errorf("x asked for %+v, want %+v", got, want)
	}
}

// The permissions are separate, and a key that opens a write GitHub rejects is
// worse than one that does nothing.
func TestXIsInertOnAThreadTheViewerMayNotResolve(t *testing.T) {
	m := onThread(t, tabLocked)
	if got := focusedCard(t, m.View()); !strings.HasPrefix(got, cardLocked) {
		t.Fatalf("the sixth tab focused %q, want the locked thread", got)
	}

	if got := asked(t, m, "x"); got != nil {
		t.Errorf("x asked for %+v on a thread nobody may settle", got)
	}
}

func TestXIsInertWithNothingFocused(t *testing.T) {
	if got := asked(t, detailed(held(sampleDetail()), 200, 60), "x"); got != nil {
		t.Errorf("x asked for %+v with no card focused", got)
	}
}

// The Files tab renders the same threads and has no ring to point at one, so
// there is nothing there for the key to act on.
func TestXIsInertOnTheFilesTab(t *testing.T) {
	m := onThread(t, tabThread)
	m.SetFiles(loadedFiles(sampleFiles(), 0))

	if got := asked(t, press(m, "]", "]", "]"), "x"); got != nil {
		t.Errorf("x asked for %+v from a tab with no ring", got)
	}
}

// The box takes every letter while it has the keyboard, x included.
func TestXTypesAnXWhileABoxHasTheKeyboard(t *testing.T) {
	m, cmd := key(replying(t, tabThread, "r"), "x")
	if cmd != nil {
		if got := cmd(); got != nil {
			if _, ok := got.(prview.ResolveThreadMsg); ok {
				t.Fatal("x settled the thread instead of typing into the box")
			}
		}
	}

	if out := stripANSI(m.View()); strings.Contains(out, "Leave a reply") {
		t.Errorf("the box is still empty, so the x went somewhere else:\n%s", out)
	}
}

// The card names the direction the reader can press, and nothing else. Both
// permissions on one control means the wrong word is a key that fails.
func TestTheThreadCardNamesTheResolveKeyItCanUse(t *testing.T) {
	open := stripANSI(onThread(t, tabThread).View())
	if !strings.Contains(open, "x resolve") {
		t.Errorf("the open thread does not name the resolve key:\n%s", open)
	}

	closed := stripANSI(onThread(t, tabResolved).View())
	if !strings.Contains(closed, "x unresolve") {
		t.Errorf("the resolved thread does not name the key that reopens it:\n%s", closed)
	}
	if strings.Contains(closed, "x resolve ") {
		t.Error("the resolved thread offers to resolve what is already settled")
	}

	locked := stripANSI(onThread(t, tabLocked).View())
	if strings.Contains(locked, "x resolve") || strings.Contains(locked, "x unresolve") {
		t.Errorf("a thread nobody may settle still names the key:\n%s", locked)
	}
}

// The write comes back through the store, and the card it lands on is the one
// the reader is standing on. Focus keys by id, so it survives the card
// collapsing under it.
func TestAThreadPushedBackResolvedCollapsesAndKeepsFocus(t *testing.T) {
	m := onThread(t, tabThread)

	d := sampleDetail()
	d.Threads[0].IsResolved = true
	m.SetDetail(held(d))

	out := stripANSI(m.View())
	if !strings.Contains(out, "✓ internal/gh/client.go:42") {
		t.Errorf("the thread does not read as resolved:\n%s", out)
	}
	if strings.Contains(out, "This backs off forever.") {
		t.Error("the resolved thread is still showing its comments")
	}
	if got := focusedCard(t, m.View()); !strings.Contains(got, cardThread) {
		t.Errorf("focus landed on %q, want the thread it was on", got)
	}
}
