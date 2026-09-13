package comp_test

import (
	"image/color"
	"testing"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
)

func TestPRStateReadsTheLifecycleBeforeTheDraftFlag(t *testing.T) {
	tests := []struct {
		name  string
		state gh.PRState
		draft bool
		want  string
	}{
		{"open", gh.PRStateOpen, false, "Open"},
		{"open draft", gh.PRStateOpen, true, "Draft"},
		{"closed", gh.PRStateClosed, false, "Closed"},
		{"closed draft", gh.PRStateClosed, true, "Closed"},
		{"merged", gh.PRStateMerged, false, "Merged"},
		{"merged draft", gh.PRStateMerged, true, "Merged"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := gh.PullRequest{State: tt.state, IsDraft: tt.draft}

			got, gotColor := comp.PRStateLabel(testTheme, pr)
			if got != tt.want {
				t.Errorf("PRStateLabel = %q, want %q", got, tt.want)
			}

			_, iconColor := comp.PRStateIcon(testTheme, pr)
			if iconColor != gotColor {
				t.Errorf("the icon and the label disagree: %v against %v", iconColor, gotColor)
			}
		})
	}
}

func TestPRStateLabelPassesAStateItDoesNotKnowThrough(t *testing.T) {
	got, _ := comp.PRStateLabel(testTheme, gh.PullRequest{State: "LOCKED"})
	if got != "LOCKED" {
		t.Errorf("PRStateLabel = %q, want the state it was given", got)
	}
}

func TestPRStateLabelFallsBackToTheDraftFlag(t *testing.T) {
	got, _ := comp.PRStateLabel(testTheme, gh.PullRequest{IsDraft: true})
	if got != "Draft" {
		t.Errorf("PRStateLabel = %q, want %q", got, "Draft")
	}
}

func TestReviewerColorSaysWhichWayTheBallIsGoing(t *testing.T) {
	th := testTheme

	tests := []struct {
		name string
		r    gh.Reviewer
		want color.Color
	}{
		{"never answered", gh.Reviewer{}, th.Subtle},
		{"commented, nothing open", gh.Reviewer{State: gh.ReviewStateCommented, Threads: 2}, th.Subtle},
		{"approved", gh.Reviewer{State: gh.ReviewStateApproved}, th.Success},

		{"a review is requested", gh.Reviewer{Requested: true}, th.Warning},
		{"approved, asked again", gh.Reviewer{State: gh.ReviewStateApproved, Requested: true}, th.Warning},
		{"commented, asked again", gh.Reviewer{State: gh.ReviewStateCommented, Requested: true}, th.Warning},
		{"changes requested, all resolved", gh.Reviewer{State: gh.ReviewStateChangesRequested, Threads: 3}, th.Warning},

		{"open thread", gh.Reviewer{State: gh.ReviewStateCommented, Threads: 2, Unresolved: 1}, th.Error},
		{"changes requested, one left", gh.Reviewer{State: gh.ReviewStateChangesRequested, Threads: 3, Unresolved: 1}, th.Error},
		{"changes requested, no threads", gh.Reviewer{State: gh.ReviewStateChangesRequested}, th.Error},
		{"open thread, asked again", gh.Reviewer{State: gh.ReviewStateCommented, Threads: 1, Unresolved: 1, Requested: true}, th.Error},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := comp.ReviewerColor(th, tt.r); got != tt.want {
				t.Errorf("ReviewerColor = %v, want %v", got, tt.want)
			}
		})
	}
}
