package prview_test

import (
	"strconv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

const (
	narrowFrame = 65
	wideFrame   = 100

	railCells = 37
)

func TestANarrowRailLandsOverTheConversation(t *testing.T) {
	m := detailed(held(sampleDetail()), narrowFrame, 30)
	if strings.Contains(stripANSI(m.View()), "Reviewers") {
		t.Fatalf("setup: the rail is already up at %d columns", narrowFrame)
	}

	before := strings.Split(stripANSI(m.View()), "\n")
	shown := press(m, "d")
	if !strings.Contains(stripANSI(shown.View()), "Reviewers") {
		t.Fatalf("d left the rail off at %d columns, where the client is read-only without it", narrowFrame)
	}

	if _, got := paneEdges(t, shown.View()); got != narrowFrame-1 {
		t.Errorf("the conversation's right border is at %d, want %d: it gave up width to the rail",
			got, narrowFrame-1)
	}

	after := strings.Split(stripANSI(press(shown, "l").View()), "\n")
	if len(before) != len(after) {
		t.Fatalf("the frame is %d lines with the rail up and %d without", len(after), len(before))
	}
	for i := paneRow(t, before) + 1; i < len(before)-1; i++ {
		l, r := rightOf(before[i], railCells), rightOf(after[i], railCells)
		if l != r {
			t.Fatalf("line %d moved under the rail:\n%q\n%q", i, l, r)
		}
	}
}

func paneRow(t *testing.T, frame []string) int {
	t.Helper()

	for i, line := range frame {
		if strings.HasPrefix(line, "╭") {
			return i
		}
	}
	t.Fatal("no pane on the frame")
	return 0
}

func TestTheNarrowRailCarriesEveryControl(t *testing.T) {
	out := stripANSI(press(detailed(held(sampleDetail()), narrowFrame, 45), "d").View())

	for _, want := range []string{"State", "Reviewers", "Assignees", "Labels", "Base"} {
		if !strings.Contains(out, want) {
			t.Errorf("the rail has no %s row at %d columns:\n%s", want, narrowFrame, out)
		}
	}
}

func TestARailWithRoomForAColumnTakesOne(t *testing.T) {
	shown := press(detailed(held(sampleDetail()), wideFrame, 30), "d")
	if !strings.Contains(stripANSI(shown.View()), "Reviewers") {
		t.Fatalf("setup: d left the rail off at %d columns", wideFrame)
	}

	if got := paneEnd(t, shown.View()); got != railCells {
		t.Errorf("the rail took %d columns, want %d", got, railCells)
	}
	if _, got := paneEdges(t, shown.View()); got != wideFrame-1 {
		t.Errorf("the conversation ends at %d, want it to fill out to %d", got, wideFrame-1)
	}
}

func TestAnOverlaidRailStepsAsideForABox(t *testing.T) {
	m := press(detailed(held(sampleDetail()), narrowFrame, 30), "d", "c")
	if out := stripANSI(m.View()); strings.Contains(out, "Reviewers") {
		t.Errorf("the rail is still over the box being written in:\n%s", out)
	}
	if out := stripANSI(m.View()); !strings.Contains(out, "post") {
		t.Errorf("the box has no footer, so the rail is not what was covering it:\n%s", out)
	}

	if out := stripANSI(press(m, "esc").View()); !strings.Contains(out, "Reviewers") {
		t.Errorf("the rail did not come back when the box closed:\n%s", out)
	}
}

func TestAColumnRailStaysUnderABox(t *testing.T) {
	m := press(detailed(held(sampleDetail()), wideFrame, 30), "d", "c")
	if out := stripANSI(m.View()); !strings.Contains(out, "Reviewers") {
		t.Errorf("the rail gave up its column for a box that fits beside it:\n%s", out)
	}
}

func TestOpeningTheRailFocusesIt(t *testing.T) {
	focused := fgSeq(testTheme.Accent)

	for _, width := range []int{narrowFrame, wideFrame} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			m := press(detailed(held(sampleDetail()), width, 30), "d")
			if got := conversationBorder(t, m.View()); got == focused {
				t.Error("the rail opened and the conversation kept the keys")
			}
			if got := conversationBorder(t, press(m, "d").View()); got != focused {
				t.Error("the rail closed and took the keys with it")
			}
		})
	}
}

func rightOf(line string, n int) string {
	at := 0
	for i, r := range line {
		if at >= n {
			return line[i:]
		}
		at += lipgloss.Width(string(r))
	}
	return ""
}
