package prview_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

const jumpHeight = 24

func jumping(t *testing.T, n int) prview.Model {
	t.Helper()

	m := walked(detailed(held(sampleDetail()), 200, jumpHeight), n)
	m.SetFiles(loadedFiles(sampleFiles(), 0))
	return m
}

func onTab(t *testing.T, frame, name string) bool {
	t.Helper()
	return currentTab(t, frame) == name
}

func landed(t *testing.T, frame string) {
	t.Helper()

	top := paneTopAt(frame)
	code := lineOf(t, frame, "min(delay*2, fetchTimeout)") - top
	card := lineOf(t, frame, cardThread) - top

	if code >= card {
		t.Errorf("the code is on row %d and the card answering it on row %d, want the code above it:\n%s",
			code, card, stripANSI(frame))
	}
	if card > 8 {
		t.Errorf("the card opens on row %d, too far down the pane to be where the jump landed:\n%s",
			card, stripANSI(frame))
	}
	if strings.Contains(stripANSI(frame), "@@ -40,4 +40,5 @@") {
		t.Errorf("the diff opened on the file's own top rather than on the thread:\n%s", stripANSI(frame))
	}
}

func TestVOpensOnTheCodeTheThreadWasWrittenAgainst(t *testing.T) {
	m := press(jumping(t, tabThread), "v")

	if !onTab(t, m.View(), "Files") {
		t.Fatalf("v did not reach the Files tab:\n%s", stripANSI(m.View()))
	}
	landed(t, m.View())
}

func TestVFetchesTheDiffAndJumpsWhenItLands(t *testing.T) {
	m := walked(detailed(held(sampleDetail()), 200, jumpHeight), tabThread)

	next, cmd := key(m, "v")
	if cmd == nil {
		t.Fatal("v asked for nothing with no diff on the screen")
	}
	if got := cmd(); got != (prview.NeedFilesMsg{ID: "PR_412"}) {
		t.Fatalf("v produced %+v, want a request for the diff", got)
	}
	if out := stripANSI(next.View()); !strings.Contains(out, "Loading the diff") {
		t.Fatalf("the tab is not waiting on the diff:\n%s", out)
	}

	next.SetFiles(loadedFiles(sampleFiles(), 0))

	landed(t, next.View())
}

func TestVUnfoldsTheDirectoryAboveTheFile(t *testing.T) {
	folded := press(jumping(t, tabThread), "]", "]", "]", "1", "j", "j", "space")
	if strings.Contains(cursorFile(folded.View()), "client.go") {
		t.Fatal("setup: the cursor is on the file rather than the directory above it")
	}
	if strings.Contains(stripANSI(folded.View()), "This backs off forever.") {
		t.Fatal("setup: the folded directory is still showing the thread")
	}

	landed(t, press(folded, "[", "[", "[", "v").View())
}

func TestVMovesTheTreeCursorToTheFile(t *testing.T) {
	m := jumping(t, tabThread)
	if got := cursorFile(m.View()); got == "client.go" {
		t.Fatal("setup: the cursor is already on the file the thread is in")
	}

	if got := cursorFile(press(m, "v").View()); got != "client.go" {
		t.Errorf("the tree cursor is on %q, want the file the thread is in", got)
	}
}

func TestVOnAFileTheDiffDoesNotCarrySaysSoAndStaysPut(t *testing.T) {
	next, cmd := key(jumping(t, tabLocked), "v")
	if cmd == nil {
		t.Fatal("v said nothing about a file that is not in the diff")
	}

	want := prview.ThreadNotInDiffMsg{Path: "internal/tui/app/app.go"}
	if got := cmd(); got != want {
		t.Fatalf("v produced %+v, want %+v", got, want)
	}
	if !onTab(t, next.View(), "Conversation") {
		t.Error("v left the conversation for a tab with nothing on it to show")
	}
}

func TestVOnAThreadWithNoLineDoesNothing(t *testing.T) {
	d := sampleDetail()
	d.Threads[3].Line = 0

	m := walked(detailed(held(d), 200, jumpHeight), tabOther)
	m.SetFiles(loadedFiles(sampleFiles(), 0))

	if got := asked(t, m, "v"); got != nil {
		t.Errorf("v asked for %+v on a thread with no line", got)
	}
}

func TestVIsInertWithNothingFocused(t *testing.T) {
	m := detailed(held(sampleDetail()), 200, jumpHeight)
	m.SetFiles(loadedFiles(sampleFiles(), 0))

	if got := asked(t, m, "v"); got != nil {
		t.Errorf("v asked for %+v with no card focused", got)
	}
}

func TestAJumpTheReaderTabbedAwayFromIsDropped(t *testing.T) {
	m := press(walked(detailed(held(sampleDetail()), 200, jumpHeight), tabThread), "v")
	m = press(m, "[", "[", "[")

	m.SetFiles(loadedFiles(sampleFiles(), 0))

	if !onTab(t, m.View(), "Conversation") {
		t.Fatalf("the arriving diff pulled the reader back to the Files tab:\n%s", stripANSI(m.View()))
	}
}

func TestAJumpWaitingOnADiffThatFailedIsDropped(t *testing.T) {
	m := press(walked(detailed(held(sampleDetail()), 200, jumpHeight), tabThread), "v")
	m.SetFiles(store.Files{Status: store.StatusFailed, Err: errors.New("network is down")})

	m.SetFiles(loadedFiles(sampleFiles(), 0))

	out := stripANSI(m.View())
	if !strings.Contains(out, "internal/gh/client.go") {
		t.Errorf("the diff did not open where it normally does, so a dead jump landed late:\n%s", out)
	}
}

func TestAJumpIntoTheLastFileLandsWithTheThreadOnScreen(t *testing.T) {
	d := sampleDetail()
	d.Threads[3].Path = "internal/tui/prview/files.go"
	d.Threads[3].Line = 2

	m := walked(detailed(held(d), 200, jumpHeight), tabOther)
	m.SetFiles(loadedFiles(sampleFiles(), 0))

	out := stripANSI(press(m, "v").View())
	if !strings.Contains(out, "Is r free after the move?") {
		t.Errorf("the thread in the last file is nowhere on the frame:\n%s", out)
	}
}

func TestFoldingADirectoryTakesItsThreadOffTheDiffAndTheJumpPutsItBack(t *testing.T) {
	m := press(jumping(t, tabThread), "v")
	landed(t, m.View())

	folded := press(m, "1", "k", "space")
	if strings.Contains(stripANSI(folded.View()), "This backs off forever.") {
		t.Fatal("setup: the folded directory is still showing the thread")
	}

	landed(t, press(folded, "[", "[", "[", "v").View())
}

func TestVAsksAgainForADiffThatFailed(t *testing.T) {
	m := walked(detailed(held(sampleDetail()), 200, jumpHeight), tabThread)

	m = press(m, "]", "]", "]")
	m.SetFiles(store.Files{Status: store.StatusFailed, Err: errors.New("502 Bad Gateway")})
	m = press(m, "[", "[", "[")

	next, cmd := key(m, "v")
	if cmd == nil {
		t.Fatal("v asked for nothing against a diff that failed")
	}
	if got := cmd(); got != (prview.NeedFilesMsg{ID: "PR_412"}) {
		t.Fatalf("v produced %+v, want another request for the diff", got)
	}

	next.SetFiles(loadedFiles(sampleFiles(), 0))
	landed(t, next.View())
}

func tallFiles() []gh.ChangedFile {
	files := make([]gh.ChangedFile, 0, 31)
	for i := range 30 {
		files = append(files, gh.ChangedFile{
			Path: fmt.Sprintf("internal/gh/a%02d.go", i), Status: gh.FileModified, Additions: 1,
			Hunks: []gh.Hunk{{
				Header: "@@ -1,1 +1,2 @@",
				Lines: []gh.DiffLine{
					{Kind: gh.DiffContext, Old: 1, New: 1, Content: "package gh"},
				},
			}},
		})
	}
	return append(files, sampleFiles()[0])
}

func TestVLeavesTheTreeCursorOnScreenAfterUnfolding(t *testing.T) {
	m := walked(detailed(held(sampleDetail()), 200, jumpHeight), tabThread)
	m.SetFiles(loadedFiles(tallFiles(), 0))

	folded := press(m, "]", "]", "]", "1", "k", "space")
	if got := selectedRow(folded.View()); !strings.Contains(got, "internal/gh/") {
		t.Fatalf("setup: the fold landed on %q rather than the directory", strings.TrimSpace(got))
	}
	if strings.Contains(stripANSI(folded.View()), "a00.go") {
		t.Fatal("setup: the directory did not fold")
	}

	back := press(folded, "[", "[", "[", "v")
	if got := cursorFile(back.View()); got != "client.go" {
		t.Errorf("the tree cursor reads %q, want it on the file and on the screen:\n%s",
			got, stripANSI(back.View()))
	}
}

func railTo(t *testing.T, m prview.Model, want string) prview.Model {
	t.Helper()

	for range 24 {
		for l := range strings.SplitSeq(stripANSI(m.View()), "\n") {
			if head, _, ok := strings.Cut(l, "││"); ok {
				l = head
			}
			if strings.Contains(l, "▌") && strings.Contains(l, want) {
				return m
			}
		}
		m = press(m, "j")
	}
	t.Fatalf("the rail's cursor never reached %q", want)
	return m
}

func TestEnterOnARailCheckOpensItOnTheChecksTab(t *testing.T) {
	d := sampleDetail()
	d.Rollup = checkRollup()
	m := railTo(t, press(detailed(held(d), 160, 44), "1"), "Build / test")

	if strings.Contains(stripANSI(m.View()), "─Log─") {
		t.Fatal("setup: the Checks tab is already open")
	}

	m = press(m, "enter")

	out := stripANSI(m.View())
	if !strings.Contains(out, "─Log─") {
		t.Fatalf("enter on a rail check did not open the Checks tab:\n%s", out)
	}

	_, cmd := key(m, "r")
	if cmd == nil {
		t.Fatal("the jump landed somewhere with no failed job under the cursor")
	}
	msg, ok := cmd().(prview.RerunCheckMsg)
	if !ok {
		t.Fatalf("r sent %T after the jump", cmd())
	}
	if msg.JobID != 103 {
		t.Errorf("the jump landed on job %d, want the check the rail was on", msg.JobID)
	}
}

func TestAJumpIntoAFoldedWorkflowOpensIt(t *testing.T) {
	d := sampleDetail()
	d.Rollup = checkRollup()

	m := press(detailed(held(d), 160, 44), "]", "]", "j", "space")
	if strings.Contains(stripANSI(m.View()), "  test") {
		t.Fatal("setup: the workflow did not fold")
	}

	m = railTo(t, press(m, "[", "[", "1"), "Build / test")
	m = press(m, "enter")

	out := stripANSI(m.View())
	if !strings.Contains(out, "▾ ✗ Build") {
		t.Errorf("the workflow the jump landed in is still folded:\n%s", out)
	}
	if !strings.Contains(out, "  ✗ test") {
		t.Errorf("the row the jump landed on is not drawn:\n%s", out)
	}
}
