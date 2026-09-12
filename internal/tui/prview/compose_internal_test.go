package prview

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/syntax"
)

func editorFixture(t *testing.T) Model {
	t.Helper()

	now := time.Now()
	rev := gh.Comment{Kind: gh.CommentReview, ID: "REV_1", Author: gh.Actor{Login: "nkr"}, CreatedAt: now}
	d := gh.PullRequestDetail{
		PullRequest: gh.PullRequest{ID: "PR_1", Number: 1, Title: "t", Repository: "o/r"},
		Body:        "desc",
		Timeline: []gh.TimelineItem{
			{Kind: gh.TimelineReview, Actor: gh.Actor{Login: "nkr"}, CreatedAt: now, Comment: &rev},
		},
		Threads: []gh.ReviewThread{{
			ID: "RT_1", ReviewID: "REV_1", Path: "a.go", Line: 1, Side: gh.SideRight, CanReply: true,
			Comments: []gh.Comment{
				{Kind: gh.CommentThread, ID: "RC_1", Author: gh.Actor{Login: "nkr"}, CreatedAt: now, Body: "one"},
			},
		}},
	}

	syn, _ := syntax.New(testTheme.Syntax)
	m := New(testTheme, d.PullRequest, RailPreference{}, syn)
	m.SetDetail(store.Detail{Detail: d, Status: store.StatusReady, Loaded: true})
	m.SetSize(200, 60)
	m = pressKeys(m, "2")
	return m
}

func pressKeys(m Model, keys ...string) Model {
	for _, k := range keys {
		m, _ = m.Update(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
	}
	return m
}

var seqs = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripSeqs(s string) string { return seqs.ReplaceAllString(s, "") }

func TestTheEditorIsTheOneTheReaderNamed(t *testing.T) {
	tests := []struct {
		name     string
		visual   string
		editor   string
		wantName string
		wantArgs []string
	}{
		{name: "neither set", wantName: "vi"},
		{name: "EDITOR", editor: "nvim", wantName: "nvim"},
		{name: "VISUAL wins", visual: "hx", editor: "nvim", wantName: "hx"},
		{name: "arguments come with it", editor: "code -w", wantName: "code", wantArgs: []string{"-w"}},
		{name: "blank is unset", editor: "   ", wantName: "vi"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("VISUAL", tt.visual)
			t.Setenv("EDITOR", tt.editor)

			name, args := editorCommand()
			if name != tt.wantName {
				t.Errorf("editor = %q, want %q", name, tt.wantName)
			}
			if !slices.Equal(args, tt.wantArgs) {
				t.Errorf("args = %q, want %q", args, tt.wantArgs)
			}
		})
	}
}

// In the package because editorDoneMsg is unexported.
func TestTheEditorWritesBackToTheBoxThatOpenedIt(t *testing.T) {
	tests := []struct {
		name  string
		open  []string
		want  string
		other string
	}{
		{
			name:  "a reply box",
			open:  []string{"}", "}", "r"},
			want:  "write a reply",
			other: "write a comment",
		},
		{
			name:  "the compose card",
			open:  []string{"c"},
			want:  "write a comment",
			other: "write a reply",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := pressKeys(editorFixture(t), tt.open...)
			if !m.Composing() {
				t.Fatal("no box has the keyboard")
			}

			m, _ = m.Update(editorDoneMsg{body: "written elsewhere\n"})
			frame := stripSeqs(m.View())

			if !strings.Contains(frame, "written elsewhere") {
				t.Fatalf("the editor's text is nowhere on the page:\n%s", frame)
			}

			at := strings.Index(frame, "written elsewhere")
			mine := strings.LastIndex(frame[:at], tt.want)
			theirs := strings.LastIndex(frame[:at], tt.other)
			if mine < 0 || theirs > mine {
				t.Errorf("the text landed under %q rather than %q:\n%s", tt.other, tt.want, frame)
			}
		})
	}
}

func TestTheEditorOpensOnTheBoxsOwnWords(t *testing.T) {
	path, err := draftFile("half an answer")
	if err != nil {
		t.Fatalf("draftFile: %v", err)
	}
	defer func() { _ = os.Remove(path) }()

	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the draft back: %v", err)
	}
	if string(out) != "half an answer" {
		t.Errorf("the editor would open on %q, want the words already in the box", out)
	}
}

func TestWrappedRowsFoldsWhereTheTextareaFolds(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		width int
		want  int
	}{
		{"words too long to pair", "aaaaaa bbbbbb cccccc", 10, 3},
		{"a word longer than the width", "aaaaaaaaaaaaaaa", 10, 2},
		{"a line that fills the width", "aaaaaaaaaa", 10, 1},
		{"nothing", "", 10, 1},
		{"a blank line is still a row", "one\n\ntwo", 10, 3},
		{"no width", "one\ntwo", 0, 2},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := wrappedRows(c.text, c.width); got != c.want {
				t.Errorf("wrappedRows(%q, %d) = %d, want %d", c.text, c.width, got, c.want)
			}
		})
	}
}
