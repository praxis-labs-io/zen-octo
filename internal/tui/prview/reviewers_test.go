package prview_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

// openReviewers opens the Reviewers picker from the add row under the section.
//
// The fixture panel is three: nkr has asked for changes, octobot has approved,
// and a team is still being waited on. Nobody in it is a person with an
// outstanding request, which is what makes the checked set worth asserting.
func openReviewers(t *testing.T) prview.Model {
	t.Helper()
	return openPicker(t, "+ Add reviewer")
}

// The section is the picker, so a reviewer row and the add row both open it.
func TestEnterOnAReviewerOpensTheSamePickerAsTheAddRow(t *testing.T) {
	for _, row := range []string{"@nkr", "+ Add reviewer"} {
		t.Run(row, func(t *testing.T) {
			m := openPicker(t, row)
			if got := menuBox(t, m, "Reviewers"); !strings.Contains(got, "Copilot") {
				t.Errorf("the picker did not open over the repository's people:\n%s", got)
			}
		})
	}
}

// Copilot cannot be discovered: GitHub publishes no connection that reports it
// as a requestable reviewer, and suggestedActors answers with the coding agent
// instead. So it is offered always, and first.
func TestTheReviewerPickerOffersCopilotFirst(t *testing.T) {
	box := menuBox(t, openReviewers(t), "Reviewers")

	rows := strings.Split(box, "\n")
	first := -1
	for i, r := range rows {
		if strings.Contains(r, "Copilot") {
			first = i
			break
		}
	}
	if first < 0 {
		t.Fatalf("Copilot is not offered at all:\n%s", box)
	}
	for i, r := range rows {
		if i < first && strings.Contains(r, "@") {
			t.Errorf("somebody is listed above Copilot:\n%s", box)
		}
	}

	// Named for itself. Its login is the one the rail shows and not a word
	// anybody would think to look for.
	if strings.Contains(box, "@"+gh.CopilotLogin) {
		t.Errorf("Copilot is listed by its login rather than its name:\n%s", box)
	}
}

// GitHub refuses a review requested of the pull request's own author, and a row
// that can only fail is worse than no row. The author is in the repository's
// assignable list, so this is a filter rather than an accident.
func TestTheReviewerPickerLeavesOutTheAuthor(t *testing.T) {
	box := menuBox(t, openReviewers(t), "Reviewers")

	if strings.Contains(box, "@drucial") {
		t.Errorf("the author is offered as a reviewer:\n%s", box)
	}
	if !strings.Contains(box, "@nkr") {
		t.Errorf("everyone else came out with them:\n%s", box)
	}
}

// A tick means a review is requested, not that somebody is on the pull request.
// Everyone in the fixture has answered or is a team, so nothing opens checked.
func TestTheReviewerPickerChecksWhoIsStillBeingWaitedOn(t *testing.T) {
	// One person still being waited on and one who has already approved, which
	// is the whole of the question.
	d := sampleDetail()
	d.Reviewers = []gh.Reviewer{
		{Actor: gh.Actor{Login: "octobot"}, State: gh.ReviewStateApproved},
		{Actor: gh.Actor{Login: "nkr"}, Requested: true},
	}

	m := onRailRow(t, detailed(held(d), 200, 60), "+ Add reviewer")
	m, _ = key(m, "enter")
	m.SetRepo(loadedRepo())

	for _, line := range strings.Split(menuBox(t, m, "Reviewers"), "\n") {
		if strings.Contains(line, "@nkr") && !strings.Contains(line, "✓") {
			t.Errorf("an outstanding request is not checked:\n%s", line)
		}
		if strings.Contains(line, "@octobot") && strings.Contains(line, "✓") {
			t.Errorf("somebody who has already reviewed opened checked:\n%s", line)
		}
	}
}

func TestCheckingAReviewerAsksForTheReview(t *testing.T) {
	m := press(openReviewers(t), "space") // the cursor opens on Copilot

	got, ok := asked(t, m, "enter").(prview.SetReviewersMsg)
	if !ok {
		t.Fatalf("enter sent %T, want a SetReviewersMsg", asked(t, m, "enter"))
	}

	if got.ID != "PR_412" || got.Repo != "zen-octo/zen-octo" || got.Number != 412 {
		t.Errorf("addressed %s %s#%d, want the pull request on screen", got.ID, got.Repo, got.Number)
	}
	if want := []string{gh.CopilotLogin}; !slices.Equal(got.Add, want) {
		t.Errorf("Add = %q, want %q", got.Add, want)
	}
	if len(got.Remove) != 0 {
		t.Errorf("Remove = %q, want nothing", got.Remove)
	}
}

// Unchecking an outstanding request cancels it, and nothing else moves.
func TestUncheckingAReviewerCancelsTheRequest(t *testing.T) {
	d := sampleDetail()
	d.Reviewers = []gh.Reviewer{{Actor: gh.Actor{Login: "nkr"}, Requested: true}}

	m := onRailRow(t, detailed(held(d), 200, 60), "+ Add reviewer")
	m, _ = key(m, "enter")
	m.SetRepo(loadedRepo())

	m = press(m, "down", "space") // Copilot is first, nkr next

	got, ok := asked(t, m, "enter").(prview.SetReviewersMsg)
	if !ok {
		t.Fatalf("enter sent %T, want a SetReviewersMsg", asked(t, m, "enter"))
	}
	if want := []string{"nkr"}; !slices.Equal(got.Remove, want) {
		t.Errorf("Remove = %q, want %q", got.Remove, want)
	}
	if len(got.Add) != 0 {
		t.Errorf("Add = %q, want nothing", got.Add)
	}
}

// Applying an untouched picker is how a reader backs out of one they opened by
// mistake, and it should cost neither a request nor a toast.
func TestApplyingAnUnchangedReviewerPickerWritesNothing(t *testing.T) {
	if got := asked(t, openReviewers(t), "enter"); got != nil {
		t.Errorf("an untouched picker sent %T, want nothing", got)
	}
}

// The picker offers users alone, so a team could never be ticked. Counting one
// as an outstanding request would put it in the remove set and cancel a request
// nothing on screen offered to cancel.
func TestATeamRequestSurvivesAReviewerWrite(t *testing.T) {
	m := press(openReviewers(t), "space") // request Copilot, touch nothing else

	got, ok := asked(t, m, "enter").(prview.SetReviewersMsg)
	if !ok {
		t.Fatalf("enter sent %T, want a SetReviewersMsg", asked(t, m, "enter"))
	}

	if slices.Contains(got.Remove, "zen-octo/maintainers") {
		t.Errorf("Remove = %q, want the team left alone", got.Remove)
	}
	if !slices.ContainsFunc(got.Panel, func(r gh.Reviewer) bool {
		return r.Actor.Login == "zen-octo/maintainers"
	}) {
		t.Error("the team came off the panel the rail is about to show")
	}
}

// The panel the rail shows while the write is out keeps everyone who has
// answered, because cancelling reaches an outstanding request and nothing else.
func TestTheOptimisticPanelKeepsWhoHasAlreadyReviewed(t *testing.T) {
	m := press(openReviewers(t), "space")

	got, ok := asked(t, m, "enter").(prview.SetReviewersMsg)
	if !ok {
		t.Fatalf("enter sent %T, want a SetReviewersMsg", asked(t, m, "enter"))
	}

	logins := make([]string, 0, len(got.Panel))
	for _, r := range got.Panel {
		logins = append(logins, r.Actor.Login)
	}
	for _, want := range []string{"nkr", "octobot", gh.CopilotLogin} {
		if !slices.Contains(logins, want) {
			t.Errorf("panel = %q, want %q on it", logins, want)
		}
	}
}

// Ticking somebody who has already reviewed is a fresh request, which is what
// GitHub's own re-request button does.
func TestRequestingAReviewAgainFromSomebodyWhoAnswered(t *testing.T) {
	d := sampleDetail()
	d.Reviewers = []gh.Reviewer{{Actor: gh.Actor{Login: "nkr"}, State: gh.ReviewStateApproved}}

	m := onRailRow(t, detailed(held(d), 200, 60), "+ Add reviewer")
	m, _ = key(m, "enter")
	m.SetRepo(loadedRepo())

	m = press(m, "down", "space") // nkr, who opened unchecked

	got, ok := asked(t, m, "enter").(prview.SetReviewersMsg)
	if !ok {
		t.Fatalf("enter sent %T, want a SetReviewersMsg", asked(t, m, "enter"))
	}
	if want := []string{"nkr"}; !slices.Equal(got.Add, want) {
		t.Errorf("Add = %q, want %q", got.Add, want)
	}
	// The verdict stays on the panel. GitHub keeps showing it, so a row
	// flipping to "waiting" would be a state the refetch is about to
	// contradict.
	if len(got.Panel) != 1 || got.Panel[0].State != gh.ReviewStateApproved {
		t.Errorf("panel = %+v, want the approval kept", got.Panel)
	}
}

// The picker applies a delta, and Chosen reports only ids it was handed items
// for, so a checked login with no item silently becomes a cancellation. The
// repository's page is a first hundred and a review can be requested of anyone
// with read access, so the two lists genuinely differ.
func TestThePickerListsAPendingReviewerTheRepositoryPageMissed(t *testing.T) {
	d := sampleDetail()
	d.Reviewers = []gh.Reviewer{{Actor: gh.Actor{Login: "ghost"}, Requested: true}}

	m := onRailRow(t, detailed(held(d), 200, 60), "+ Add reviewer")
	m, _ = key(m, "enter")
	m.SetRepo(loadedRepo())

	box := menuBox(t, m, "Reviewers")
	if !strings.Contains(box, "@ghost") {
		t.Fatalf("a pending reviewer the repository's page did not reach is missing:\n%s", box)
	}
	for _, line := range strings.Split(box, "\n") {
		if strings.Contains(line, "@ghost") && !strings.Contains(line, "✓") {
			t.Errorf("the extra reviewer is listed but not checked:\n%s", box)
		}
	}

	// Ticking somebody else must not cancel them on the way past.
	m = press(m, "space") // Copilot, the first row
	got, ok := asked(t, m, "enter").(prview.SetReviewersMsg)
	if !ok {
		t.Fatalf("enter sent %T, want a SetReviewersMsg", asked(t, m, "enter"))
	}
	if slices.Contains(got.Remove, "ghost") {
		t.Errorf("Remove = %q, want the unlisted reviewer left alone", got.Remove)
	}
}

// A verdict and an open request are both true at once after a re-request, and
// the panel carries them on one row. Reading "no state means waiting" hides the
// request: nothing can cancel it, and ticking asks again for a review already
// pending.
func TestAReRequestedReviewerOpensCheckedAndCanBeCancelled(t *testing.T) {
	d := sampleDetail()
	d.Reviewers = []gh.Reviewer{
		{Actor: gh.Actor{Login: "nkr"}, State: gh.ReviewStateApproved, Requested: true},
	}

	m := onRailRow(t, detailed(held(d), 200, 60), "+ Add reviewer")
	m, _ = key(m, "enter")
	m.SetRepo(loadedRepo())

	for _, line := range strings.Split(menuBox(t, m, "Reviewers"), "\n") {
		if strings.Contains(line, "@nkr") && !strings.Contains(line, "✓") {
			t.Fatalf("a reviewer with an outstanding re-request opened unchecked:\n%s", line)
		}
	}

	m = press(m, "down", "space") // untick @nkr

	got, ok := asked(t, m, "enter").(prview.SetReviewersMsg)
	if !ok {
		t.Fatalf("enter sent %T, want a SetReviewersMsg", asked(t, m, "enter"))
	}
	if want := []string{"nkr"}; !slices.Equal(got.Remove, want) {
		t.Errorf("Remove = %q, want %q", got.Remove, want)
	}
	// The verdict survives the cancellation. Only the request was theirs to take.
	if len(got.Panel) != 1 || got.Panel[0].State != gh.ReviewStateApproved || got.Panel[0].Requested {
		t.Errorf("panel = %+v, want the approval kept and the request cleared", got.Panel)
	}
}

// Assigning is CanAssign's to permit, but the mutation behind it is
// updatePullRequest, which GitHub governs with CanUpdate. A triage collaborator
// is answered true for the first and false for the second.
func TestTheAssigneeSectionIsInertWithoutTheUpdatePermission(t *testing.T) {
	d := sampleDetail()
	d.Viewer.CanAssign = true
	d.Viewer.CanUpdate = false

	if strings.Contains(stripANSI(detailed(held(d), 200, 60).View()), "+ Add assignee") {
		t.Error("the add row is offered where updatePullRequest would be refused")
	}
}
