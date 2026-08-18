package comp_test

import (
	"testing"

	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
)

func TestSplitDetailsSeparatesProseFromFolds(t *testing.T) {
	body := "Before it.\n\n<details>\n<summary>File summaries</summary>\n\n| a | b |\n| 1 | 2 |\n\n</details>\n\nBetween them.\n\n<details><summary>Review details</summary>\none line\n</details>\n\nAfter it.\n"

	got := comp.SplitDetails(body)
	if len(got) != 5 {
		t.Fatalf("got %d segments, want 5: %+v", len(got), got)
	}

	want := []struct {
		summary string
		lines   int
	}{
		{}, {summary: "File summaries", lines: 2}, {}, {summary: "Review details", lines: 1}, {},
	}
	for i, w := range want {
		if got[i].Summary != w.summary || got[i].Lines != w.lines {
			t.Errorf("segment %d = %q/%d lines, want %q/%d",
				i, got[i].Summary, got[i].Lines, w.summary, w.lines)
		}
	}

	if got[0].Text == "" || got[4].Text == "" {
		t.Error("the prose either side of the folds went missing")
	}
}

func TestSplitDetailsHandlesTheAwkwardShapes(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    int
		summary string
	}{
		{
			name: "nothing to fold is one piece of prose",
			body: "Just a description.", want: 1,
		},
		{
			name: "a summary wrapped in markup keeps only its words",
			body: "<details><summary><b>Files not reviewed (1)</b></summary>\nx\n</details>",
			want: 1, summary: "Files not reviewed (1)",
		},
		{
			name: "a block with no summary still folds",
			body: "<details>\nx\n</details>",
			want: 1, summary: "Details",
		},
		{
			// GitHub writes <DETAILS> on occasion and the tag is not the point.
			name: "case does not matter",
			body: "<DETAILS><SUMMARY>Overview</SUMMARY>\nx\n</DETAILS>",
			want: 1, summary: "Overview",
		},
		{
			name: "an empty body is no segments at all",
			body: "   \n\n  ", want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := comp.SplitDetails(tt.body)
			if len(got) != tt.want {
				t.Fatalf("got %d segments, want %d: %+v", len(got), tt.want, got)
			}
			if tt.want > 0 && got[0].Summary != tt.summary {
				t.Errorf("summary = %q, want %q", got[0].Summary, tt.summary)
			}
		})
	}
}

// Folded is what tells a screen whether the expand key has anything to do.
func TestFoldedReportsWhetherThereIsAnythingToOpen(t *testing.T) {
	if comp.Folded(comp.SplitDetails("Just prose.")) {
		t.Error("prose alone reads as having a fold in it")
	}
	if !comp.Folded(comp.SplitDetails("<details><summary>x</summary>y</details>")) {
		t.Error("a fold reads as having none")
	}
}
