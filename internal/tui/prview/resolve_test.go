package prview_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

const tabResolved = 6

func asked(t *testing.T, m prview.Model, k string) tea.Msg {
	t.Helper()

	_, cmd := key(m, k)
	if cmd == nil {
		return nil
	}
	return cmd()
}

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

func TestXOnAResolvedThreadAsksToUnresolve(t *testing.T) {
	m := onThread(t, tabResolved)
	if got := focusedCard(t, m.View()); !strings.Contains(got, "resolved") {
		t.Fatalf("the ring landed on %q, want the resolved thread", got)
	}

	got := resolveAsked(t, m, "x")
	want := prview.ResolveThreadMsg{ID: "PR_412", ThreadID: "RT_2"}
	if got != want {
		t.Errorf("x asked for %+v, want %+v", got, want)
	}
}

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

func TestXIsInertOnTheFilesTab(t *testing.T) {
	m := onThread(t, tabThread)
	m.SetFiles(loadedFiles(sampleFiles(), 0))

	if got := asked(t, press(m, "]", "]", "]"), "x"); got != nil {
		t.Errorf("x asked for %+v from a tab with no ring", got)
	}
}

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

func TestAThreadPushedBackOpenComesBackIntoView(t *testing.T) {
	long := strings.Repeat("A line of the nit.\n\n", 3) + "The last line of it."

	d := sampleDetail()
	d.Threads[1].CanUnresolve = true
	d.Threads[1].Comments[0].Body = long

	m := walked(detailed(held(d), 160, 20), tabResolved)
	if !strings.Contains(stripANSI(m.View()), "✓ internal/store/store.go:88") {
		t.Fatal("the resolved thread is not on screen to begin with")
	}

	open := sampleDetail()
	open.Threads[1].IsResolved = false
	open.Threads[1].Comments[0].Body = long
	m.SetDetail(held(open))

	if out := stripANSI(m.View()); !strings.Contains(out, "The last line of it.") {
		t.Errorf("the thread grew off the bottom of the window:\n%s", out)
	}
}

func TestXIsInertOnAThreadWithAWriteStillOut(t *testing.T) {
	d := sampleDetail()
	d.Threads[0].Pending = true
	d.Threads[0].CanUnresolve = true

	m := walked(detailed(held(d), 200, 60), tabThread)

	if got := asked(t, m, "x"); got != nil {
		t.Errorf("x asked for %+v on a thread already answering for a write", got)
	}
	if out := stripANSI(m.View()); strings.Contains(out, "x resolve") || strings.Contains(out, "x unresolve") {
		t.Error("the card names a key that is inert while the write is out")
	}
}

func TestAThreadReopenedAndResolvedAgainCollapses(t *testing.T) {
	m := onThread(t, tabResolved)

	m = press(m, "space")
	if !strings.Contains(stripANSI(m.View()), "Typo.") {
		t.Fatal("setup: o did not open the resolved thread")
	}

	open := sampleDetail()
	open.Threads[1].IsResolved = false
	m.SetDetail(held(open))

	closed := sampleDetail()
	m.SetDetail(held(closed))

	if out := stripANSI(m.View()); strings.Contains(out, "Typo.") {
		t.Errorf("the thread resolved again is still showing its comments:\n%s", out)
	}
}

func TestAThreadCardGivesUpWholeHintsRatherThanClippingOne(t *testing.T) {
	whole := map[string]bool{
		"r reply": true, "R quote": true, "x resolve": true, "⏎ in diff": true,
	}

	tests := []struct {
		width int
		want  []string
	}{
		{200, []string{"r reply", "R quote", "x resolve", "⏎ in diff"}},
		{44, []string{"r reply", "R quote", "x resolve"}},
		{34, []string{"r reply", "R quote"}},
	}

	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.width), func(t *testing.T) {
			got := cardHints(t, walked(detailed(held(sampleDetail()), tt.width, 60), tabThread).View())

			if !slices.Equal(got, tt.want) {
				t.Errorf("hints = %q, want %q", got, tt.want)
			}
			for _, hint := range got {
				if !whole[hint] {
					t.Errorf("%q is not a whole hint, so the line was cut", hint)
				}
			}
		})
	}
}

func cardHints(t *testing.T, frame string) []string {
	t.Helper()

	for _, line := range strings.Split(stripANSI(frame), "\n") {
		at := strings.Index(line, "╰")
		if at < 0 || !strings.Contains(line, "·") {
			continue
		}

		end := strings.Index(line[at:], "╯")
		if end < 0 {
			continue
		}

		text := strings.Trim(line[at:at+end], "╰╯─ ")
		if text == "" {
			continue
		}
		return strings.Split(text, " · ")
	}
	t.Fatalf("no card on the frame carries hints:\n%s", stripANSI(frame))
	return nil
}
