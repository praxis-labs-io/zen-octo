package comp_test

import (
	"image/color"
	"testing"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

// State and IsDraft are independent fields, and a pull request closed while it
// was a draft carries both. The lifecycle is what decides which one shows: a
// row reading "Draft" over a closed pull request says it is still waiting to be
// picked up, and no key on the rail will move it.
//
// The icon follows the same order. A pair that disagree read as a rendering
// fault, and one of them being right is worse than neither: the eye trusts
// whichever it saw first.
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
		// GitHub does not produce this one, but the flag is never cleared on
		// the way through and nothing here should depend on that.
		{"merged draft", gh.PRStateMerged, true, "Merged"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := gh.PullRequest{State: tt.state, IsDraft: tt.draft}

			got, gotColor := comp.PRStateLabel(theme.RosePineMoon, pr)
			if got != tt.want {
				t.Errorf("PRStateLabel = %q, want %q", got, tt.want)
			}

			// The icon carries the same answer, so its color has to match the
			// label's rather than the two disagreeing about the same pair.
			_, iconColor := comp.PRStateIcon(theme.RosePineMoon, pr)
			if iconColor != gotColor {
				t.Errorf("the icon and the label disagree: %v against %v", iconColor, gotColor)
			}
		})
	}
}

// A state GitHub adds later arrives here unvalidated. It says what it was given
// rather than claiming the pull request is open.
func TestPRStateLabelPassesAStateItDoesNotKnowThrough(t *testing.T) {
	got, _ := comp.PRStateLabel(theme.RosePineMoon, gh.PullRequest{State: "LOCKED"})
	if got != "LOCKED" {
		t.Errorf("PRStateLabel = %q, want the state it was given", got)
	}
}

// An unknown state on a draft is still a draft: the flag is the only thing
// either field knows about it.
func TestPRStateLabelFallsBackToTheDraftFlag(t *testing.T) {
	got, _ := comp.PRStateLabel(theme.RosePineMoon, gh.PullRequest{IsDraft: true})
	if got != "Draft" {
		t.Errorf("PRStateLabel = %q, want %q", got, "Draft")
	}
}

// The rail has one cell to say where a reviewer stands, so the color is the
// whole of the meaning. The pairs that share a color are the ones worth reading
// twice: an outstanding request and a set of resolved asks are both in flight,
// and a changes-requested review with nothing to resolve is as blocking on its
// last day as its first.
func TestReviewerColorSaysWhichWayTheBallIsGoing(t *testing.T) {
	th := theme.RosePineMoon

	tests := []struct {
		name string
		r    gh.Reviewer
		want color.Color
	}{
		{"never answered", gh.Reviewer{}, th.Subtle},
		{"commented, nothing open", gh.Reviewer{State: gh.ReviewStateCommented, Threads: 2}, th.Subtle},
		{"approved", gh.Reviewer{State: gh.ReviewStateApproved}, th.Success},

		{"a review is requested", gh.Reviewer{Requested: true}, th.Warning},
		// The re-request has to show, or pressing the key looks like nothing.
		{"approved, asked again", gh.Reviewer{State: gh.ReviewStateApproved, Requested: true}, th.Warning},
		{"commented, asked again", gh.Reviewer{State: gh.ReviewStateCommented, Requested: true}, th.Warning},
		// Every point met, no verdict since. Not blocking, not agreed.
		{"changes requested, all resolved", gh.Reviewer{State: gh.ReviewStateChangesRequested, Threads: 3}, th.Warning},

		{"open thread", gh.Reviewer{State: gh.ReviewStateCommented, Threads: 2, Unresolved: 1}, th.Error},
		{"changes requested, one left", gh.Reviewer{State: gh.ReviewStateChangesRequested, Threads: 3, Unresolved: 1}, th.Error},
		// Prose with nothing to resolve. Nothing can record that it was dealt
		// with, so going quiet on it would claim it had been.
		{"changes requested, no threads", gh.Reviewer{State: gh.ReviewStateChangesRequested}, th.Error},
		// Blocking outranks in flight: asking again does not clear what is open.
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
