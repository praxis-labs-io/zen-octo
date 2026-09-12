package prview_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

func openReviewers(t *testing.T) prview.Model {
	t.Helper()
	return openPicker(t, "+ Add reviewer")
}

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

	if strings.Contains(box, "@"+gh.CopilotLogin) {
		t.Errorf("Copilot is listed by its login rather than its name:\n%s", box)
	}
}

func TestTheReviewerPickerLeavesOutTheAuthor(t *testing.T) {
	box := menuBox(t, openReviewers(t), "Reviewers")

	if strings.Contains(box, "@drucial") {
		t.Errorf("the author is offered as a reviewer:\n%s", box)
	}
	if !strings.Contains(box, "@nkr") {
		t.Errorf("everyone else came out with them:\n%s", box)
	}
}

func TestTheReviewerPickerChecksWhoIsStillBeingWaitedOn(t *testing.T) {
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
	m := press(openReviewers(t), "space")

	got, ok := asked(t, m, "enter").(prview.SetReviewersMsg)
	if !ok {
		t.Fatalf("enter sent %T, want a SetReviewersMsg", asked(t, m, "enter"))
	}

	if got.ID != "PR_412" || got.Repo != "acme/rocket" || got.Number != 412 {
		t.Errorf("addressed %s %s#%d, want the pull request on screen", got.ID, got.Repo, got.Number)
	}
	if want := []string{gh.CopilotLogin}; !slices.Equal(got.Add, want) {
		t.Errorf("Add = %q, want %q", got.Add, want)
	}
	if len(got.Remove) != 0 {
		t.Errorf("Remove = %q, want nothing", got.Remove)
	}
}

func TestUncheckingAReviewerCancelsTheRequest(t *testing.T) {
	d := sampleDetail()
	d.Reviewers = []gh.Reviewer{{Actor: gh.Actor{Login: "nkr"}, Requested: true}}

	m := onRailRow(t, detailed(held(d), 200, 60), "+ Add reviewer")
	m, _ = key(m, "enter")
	m.SetRepo(loadedRepo())

	m = press(m, "down", "space")

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

func TestApplyingAnUnchangedReviewerPickerWritesNothing(t *testing.T) {
	if got := asked(t, openReviewers(t), "enter"); got != nil {
		t.Errorf("an untouched picker sent %T, want nothing", got)
	}
}

func TestATeamRequestSurvivesAReviewerWrite(t *testing.T) {
	m := press(openReviewers(t), "space")

	got, ok := asked(t, m, "enter").(prview.SetReviewersMsg)
	if !ok {
		t.Fatalf("enter sent %T, want a SetReviewersMsg", asked(t, m, "enter"))
	}

	if slices.Contains(got.Remove, "acme/maintainers") {
		t.Errorf("Remove = %q, want the team left alone", got.Remove)
	}
	if !slices.ContainsFunc(got.Panel, func(r gh.Reviewer) bool {
		return r.Actor.Login == "acme/maintainers"
	}) {
		t.Error("the team came off the panel the rail is about to show")
	}
}

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

func TestRequestingAReviewAgainFromSomebodyWhoAnswered(t *testing.T) {
	d := sampleDetail()
	d.Reviewers = []gh.Reviewer{{Actor: gh.Actor{Login: "nkr"}, State: gh.ReviewStateApproved}}

	m := onRailRow(t, detailed(held(d), 200, 60), "+ Add reviewer")
	m, _ = key(m, "enter")
	m.SetRepo(loadedRepo())

	m = press(m, "down", "space")

	got, ok := asked(t, m, "enter").(prview.SetReviewersMsg)
	if !ok {
		t.Fatalf("enter sent %T, want a SetReviewersMsg", asked(t, m, "enter"))
	}
	if want := []string{"nkr"}; !slices.Equal(got.Add, want) {
		t.Errorf("Add = %q, want %q", got.Add, want)
	}
	if len(got.Panel) != 1 || got.Panel[0].State != gh.ReviewStateApproved {
		t.Errorf("panel = %+v, want the approval kept", got.Panel)
	}
}

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

	m = press(m, "space")
	got, ok := asked(t, m, "enter").(prview.SetReviewersMsg)
	if !ok {
		t.Fatalf("enter sent %T, want a SetReviewersMsg", asked(t, m, "enter"))
	}
	if slices.Contains(got.Remove, "ghost") {
		t.Errorf("Remove = %q, want the unlisted reviewer left alone", got.Remove)
	}
}

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

	m = press(m, "down", "space")

	got, ok := asked(t, m, "enter").(prview.SetReviewersMsg)
	if !ok {
		t.Fatalf("enter sent %T, want a SetReviewersMsg", asked(t, m, "enter"))
	}
	if want := []string{"nkr"}; !slices.Equal(got.Remove, want) {
		t.Errorf("Remove = %q, want %q", got.Remove, want)
	}
	if len(got.Panel) != 1 || got.Panel[0].State != gh.ReviewStateApproved || got.Panel[0].Requested {
		t.Errorf("panel = %+v, want the approval kept and the request cleared", got.Panel)
	}
}

func TestTheAssigneeSectionIsInertWithoutTheUpdatePermission(t *testing.T) {
	d := sampleDetail()
	d.Viewer.CanAssign = true
	d.Viewer.CanUpdate = false

	if strings.Contains(stripANSI(detailed(held(d), 200, 60).View()), "+ Add assignee") {
		t.Error("the add row is offered where updatePullRequest would be refused")
	}
}
