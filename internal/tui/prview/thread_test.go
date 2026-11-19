package prview_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

func posted(t *testing.T, m prview.Model, body string) prview.PostThreadMsg {
	t.Helper()

	_, cmd := post(typed(m, body))
	msg, ok := runCmd(cmd).(prview.PostThreadMsg)
	if !ok {
		t.Fatalf("posting produced %T, want a PostThreadMsg", runCmd(cmd))
	}
	return msg
}

func TestCOnFilesOpensABoxOnWhatTheCursorNames(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		head string
		want gh.ReviewThreadInput
	}{
		{
			name: "the file, with nothing under the cursor",
			head: "comment on this file",
			want: gh.ReviewThreadInput{Path: "internal/gh/client.go", Subject: gh.SubjectFile},
		},
		{
			name: "the hunk, from its head",
			keys: []string{"}"},
			head: "comment on lines 40–43",
			want: gh.ReviewThreadInput{
				Path: "internal/gh/client.go", Subject: gh.SubjectLine,
				StartLine: 40, StartSide: gh.SideRight, Line: 43, Side: gh.SideRight,
			},
		},
		{
			name: "the row under the cursor",
			keys: []string{"}", "j", "j"},
			head: "comment on line 41",
			want: gh.ReviewThreadInput{
				Path: "internal/gh/client.go", Subject: gh.SubjectLine,
				StartLine: 41, StartSide: gh.SideLeft, Line: 41, Side: gh.SideLeft,
			},
		},
		{
			name: "the selection",
			keys: []string{"}", "j", "j", "v", "j", "j"},
			head: "comment on lines 41–42",
			want: gh.ReviewThreadInput{
				Path: "internal/gh/client.go", Subject: gh.SubjectLine,
				StartLine: 41, StartSide: gh.SideLeft, Line: 42, Side: gh.SideRight,
			},
		},
		{
			name: "a selection from a context line down to a removed one",
			keys: []string{"}", "j", "v", "j"},
			head: "comment on lines 40–41",
			want: gh.ReviewThreadInput{
				Path: "internal/gh/client.go", Subject: gh.SubjectLine,
				StartLine: 40, StartSide: gh.SideRight, Line: 41, Side: gh.SideLeft,
			},
		},
		{
			name: "a row in the left column side by side",
			keys: []string{"|", "}", "j", "h"},
			head: "comment on line 40",
			want: gh.ReviewThreadInput{
				Path: "internal/gh/client.go", Subject: gh.SubjectLine,
				StartLine: 40, StartSide: gh.SideLeft, Line: 40, Side: gh.SideLeft,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := press(press(onFiles(200, 50), tt.keys...), "c")

			if !m.Composing() {
				t.Fatal("c on Files opened no box")
			}
			if out := stripANSI(m.View()); !strings.Contains(out, tt.head) {
				t.Errorf("the box does not say %q:\n%s", tt.head, out)
			}

			want := tt.want
			want.Body, want.PullRequestID = "name this", "PR_412"
			if got := posted(t, m, "name this").Thread; got != want {
				t.Errorf("sent %+v, want %+v", got, want)
			}
		})
	}
}

func TestCOnAFoldedHunkOpensItAroundTheBox(t *testing.T) {
	m := press(onFiles(200, 50), "}", "space")
	if strings.Contains(stripANSI(m.View()), "time.Sleep(delay)") {
		t.Fatal("setup: the hunk did not fold")
	}

	out := stripANSI(press(m, "c").View())
	if !strings.Contains(out, "comment on lines 40–43") || !strings.Contains(out, "time.Sleep(delay)") {
		t.Errorf("the box opened under a hunk whose code is hidden:\n%s", out)
	}
}

func TestASelectionStartedInAFoldedHunkHoldsNothing(t *testing.T) {
	m := press(onFiles(200, 50), "}", "j", "space", "v", "space", "c")
	if !m.Composing() {
		t.Fatal("c opened no box")
	}

	want := gh.ReviewThreadInput{
		Path: "internal/gh/client.go", Subject: gh.SubjectLine, Body: "name this", PullRequestID: "PR_412",
		StartLine: 40, StartSide: gh.SideRight, Line: 40, Side: gh.SideRight,
	}
	if got := posted(t, m, "name this").Thread; got != want {
		t.Errorf("sent %+v, want the row under the cursor alone", got)
	}
}

func TestAThreadJoinsThePendingReviewWhenThereIsOne(t *testing.T) {
	d := sampleDetail()
	d.DraftReview = gh.Review{ID: "PRR_9", State: gh.ReviewStatePending}

	m := detailed(held(d), 200, 50)
	m.SetFiles(loadedFiles(sampleFiles(), 0))
	m = press(m, "]", "]", "]", "}", "c")

	got := posted(t, m, "name this").Thread
	if got.ReviewID != "PRR_9" || got.PullRequestID != "" {
		t.Errorf("sent review %q and pull request %q, want the pending review alone", got.ReviewID, got.PullRequestID)
	}
}

func TestTheBoxHangsUnderTheLineItAnswers(t *testing.T) {
	m := press(onFiles(200, 50), "}", "j", "j", "j", "c")
	lines := strings.Split(stripANSI(m.View()), "\n")

	code, box, next := -1, -1, -1
	for i, line := range lines {
		switch {
		case strings.Contains(line, "min(delay*2, fetchTimeout)"):
			code = i
		case strings.Contains(line, "comment on line 41"):
			box = i
		case box >= 0 && next < 0 && strings.Contains(line, "time.Sleep(delay)"):
			next = i
		}
	}
	if code < 0 || box < 0 || next < 0 {
		t.Fatalf("the line, the box or the line after them is missing:\n%s", strings.Join(lines, "\n"))
	}
	if code > box || box > next {
		t.Errorf("the box is on row %d, want it between the line it answers on %d and the next on %d",
			box, code, next)
	}
}

func TestAFileCommentBoxHeadsTheFile(t *testing.T) {
	out := stripANSI(press(onFiles(200, 50), "c").View())

	box, hunk := strings.Index(out, "comment on this file"), strings.Index(out, "@@ -40,4 +40,5 @@")
	if box < 0 || hunk < 0 || box > hunk {
		t.Errorf("the box does not open above the file's first hunk:\n%s", out)
	}
}

func TestEscKeepsTheWordsForTheSameSpot(t *testing.T) {
	m := typed(press(onFiles(200, 50), "}", "j", "j", "c"), "name this")
	m = press(m, "esc")
	if m.Composing() {
		t.Fatal("esc left the box open")
	}

	if out := stripANSI(press(m, "c").View()); !strings.Contains(out, "name this") {
		t.Errorf("c on the same line lost the words written there:\n%s", out)
	}
}

func TestPostingLeavesTheCursorWhereItWas(t *testing.T) {
	m, _ := post(typed(press(onFiles(200, 50), "}", "j", "j", "c"), "name this"))

	if m.Composing() {
		t.Fatal("posting left the box open")
	}
	if got := barredCode(m.View()); !strings.Contains(got, "time.Sleep(delay)") {
		t.Errorf("the cursor is on %q after posting, want the line the comment was written on", got)
	}
}

func TestAFailedThreadReopensItsBoxWithTheWords(t *testing.T) {
	m, cmd := post(typed(press(onFiles(200, 50), "}", "j", "j", "c"), "name this"))
	msg := runCmd(cmd).(prview.PostThreadMsg)

	m.RestoreThread(msg.Thread, msg.Thread.Body)

	out := stripANSI(m.View())
	if !m.Composing() || !strings.Contains(out, "comment on line 41") {
		t.Fatalf("the box did not reopen on its line:\n%s", out)
	}
	if !strings.Contains(out, "name this") {
		t.Errorf("the words did not come back:\n%s", out)
	}
}

func TestTypingInTheDiffBoxRendersWhatARebuildWould(t *testing.T) {
	for _, tt := range []struct {
		name string
		keys []string
	}{
		{"a new thread", []string{"}", "j", "j", "c"}},
		{"a reply", []string{"}", "}", "r"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			joined := typed(press(onFiles(200, 50), tt.keys...), "name this")
			if !joined.Composing() {
				t.Fatal("setup: no box has the keyboard")
			}

			rebuilt := joined
			rebuilt.SetSize(200, 50)

			if joined.View() != rebuilt.View() {
				t.Errorf("the cached page and a rebuilt one differ:\n%s\nwant\n%s",
					stripANSI(joined.View()), stripANSI(rebuilt.View()))
			}
		})
	}
}

func TestTheDiffBoxFitsTheFloor(t *testing.T) {
	const width, height = 56, 21

	for _, tt := range []struct {
		name string
		text string
		last string
	}{
		{"empty", "", ""},
		{"longer than the pane", strings.Repeat("more\n", 30) + "the caret is here", "the caret is here"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := sampleDetail()
			m := detailed(held(d), width, height)
			m.SetFiles(loadedFiles([]gh.ChangedFile{longFile("main.go", 80)}, 0))
			m = typed(press(m, "]", "]", "]", "}", "j", "j", "j", "j", "c"), tt.text)

			frame := m.View()
			lines := strings.Split(frame, "\n")
			if len(lines) != height {
				t.Errorf("the frame is %d lines, want %d", len(lines), height)
			}
			for i, line := range lines {
				if w := lipgloss.Width(line); w != width {
					t.Errorf("line %d is %d columns wide, want %d", i, w, width)
					break
				}
			}

			out := stripANSI(frame)
			if !strings.Contains(out, "Start a review") {
				t.Errorf("the box's button is off the screen:\n%s", out)
			}
			if !strings.Contains(out, tt.last) {
				t.Errorf("the line being written is off the screen:\n%s", out)
			}
		})
	}
}

func TestOpeningTheBoxKeepsTheLineItAnswersOnScreen(t *testing.T) {
	m := detailed(held(sampleDetail()), 120, 30)
	m.SetFiles(loadedFiles([]gh.ChangedFile{longFile("main.go", 80)}, 0))
	m = press(m, "]", "]", "]", "}")
	for range 40 {
		m = press(m, "j")
	}
	m = press(m, "c")

	out := stripANSI(m.View())
	if !strings.Contains(out, "case 39:") {
		t.Errorf("the line the box answers left the screen:\n%s", out)
	}
	if !strings.Contains(out, "Start a review") {
		t.Errorf("the box's button is off the screen:\n%s", out)
	}
}

func TestADraftCommentReadsPending(t *testing.T) {
	d := sampleDetail()
	d.Threads[0].Draft = true
	d.Threads[0].Comments[0].Draft = true

	for _, tt := range []struct {
		name string
		m    prview.Model
	}{
		{"in the conversation", detailed(held(d), 200, 60)},
		{"in the diff", func() prview.Model {
			m := detailed(held(d), 200, 60)
			m.SetFiles(loadedFiles(sampleFiles(), 0))
			return press(m, "]", "]", "]")
		}()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out := stripANSI(tt.m.View())
			if !strings.Contains(out, "nkr · said · pending") {
				t.Errorf("the draft comment does not read pending:\n%s", out)
			}
			if strings.Contains(out, "octobot · said · pending") {
				t.Errorf("a published reply reads pending:\n%s", out)
			}
		})
	}
}

func TestTheButtonSaysWhatPostingDoes(t *testing.T) {
	for _, tt := range []struct {
		name   string
		review string
		button string
		hint   string
	}{
		{"with no pending review", "", "Start a review", "start a review"},
		{"with one held", "PRR_9", "Add to review", "add to review"},
	} {
		for _, width := range []int{200, 56} {
			t.Run(tt.name+" at "+strconv.Itoa(width), func(t *testing.T) {
				d := sampleDetail()
				if tt.review != "" {
					d.DraftReview = gh.Review{ID: tt.review, State: gh.ReviewStatePending}
				}
				m := detailed(held(d), width, 30)
				m.SetFiles(loadedFiles(sampleFiles(), 0))
				m = press(m, "]", "]", "]", "}", "j", "c")

				lines := strings.Split(stripANSI(m.View()), "\n")
				row := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, tt.button) })
				if row < 0 {
					t.Fatalf("no row carries %q:\n%s", tt.button, strings.Join(lines, "\n"))
				}
				if width == 200 && !strings.Contains(lines[row], tt.hint+" ·") {
					t.Errorf("the hint beside the button does not say %q: %q", tt.hint, lines[row])
				}
				if width == 56 && strings.Contains(lines[row], "editor") {
					t.Errorf("the hint held its place over the button at the floor: %q", lines[row])
				}
			})
		}
	}
}

// A thread carries no start side, so this holds for a range across both sides as much as one on one side.
func TestARangeThreadIsHeadedByItsLastLine(t *testing.T) {
	d := sampleDetail()
	d.Threads = append(d.Threads, gh.ReviewThread{
		ID: "RT_9", Path: "internal/gh/client.go", Subject: gh.SubjectLine,
		StartLine: 41, Line: 42, Side: gh.SideRight,
		Comments: []gh.Comment{{ID: "RC_9", Author: gh.Actor{Login: "nkr"}, Body: "Both of these."}},
	})
	m := detailed(held(d), 200, 80)
	m.SetFiles(loadedFiles(sampleFiles(), 0))
	out := stripANSI(press(m, "]", "]", "]").View())

	said := strings.Index(out, "Both of these.")
	if said < 0 {
		t.Fatalf("the range thread is missing:\n%s", out)
	}
	head := strings.LastIndex(out[:said], "internal/gh/client.go:")
	if head < 0 || !strings.HasPrefix(out[head:], "internal/gh/client.go:42 ") {
		t.Errorf("the range thread is not headed by its last line alone:\n%s", out)
	}
}
