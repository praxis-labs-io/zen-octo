package comp_test

import (
	"testing"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
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
