package prview_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

func openAssignees(t *testing.T) prview.Model {
	t.Helper()
	return openPicker(t, "+ Add assignee")
}

func assigneeLogins(msg prview.SetAssigneesMsg) []string {
	out := make([]string, 0, len(msg.Assignees))
	for _, a := range msg.Assignees {
		out = append(out, a.Login)
	}
	return out
}

func TestEnterOnAnAssigneeOpensTheSamePickerAsTheAddRow(t *testing.T) {
	for _, row := range []string{"@drucial", "+ Add assignee"} {
		t.Run(row, func(t *testing.T) {
			m := openPicker(t, row)
			if got := menuBox(t, m, "Assignees"); !strings.Contains(got, "@nkr") {
				t.Errorf("the picker did not open over the repository's people:\n%s", got)
			}
		})
	}
}

func TestTheAssigneePickerOpensOnWhoIsAlreadyAssigned(t *testing.T) {
	got := menuBox(t, openAssignees(t), "Assignees")

	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "@drucial") && !strings.Contains(line, "✓") {
			t.Errorf("the assignee already on the pull request is not checked:\n%s", got)
		}
		if strings.Contains(line, "@nkr") && strings.Contains(line, "✓") {
			t.Errorf("somebody who is not assigned opened checked:\n%s", got)
		}
	}
}

func TestCheckingAnAssigneeAndApplyingAsksForTheWholeSet(t *testing.T) {
	m := openAssignees(t)

	m = press(m, "down")
	m = press(m, " ")

	got, ok := asked(t, m, "enter").(prview.SetAssigneesMsg)
	if !ok {
		t.Fatalf("enter sent %T, want a SetAssigneesMsg", asked(t, m, "enter"))
	}

	if got.ID != "PR_412" {
		t.Errorf("ID = %q, want PR_412", got.ID)
	}
	if want := []string{"drucial", "nkr"}; !slices.Equal(assigneeLogins(got), want) {
		t.Errorf("assignees = %q, want %q", assigneeLogins(got), want)
	}
	if got.Assignees[1].ID != "U_2" {
		t.Errorf("assignees[1].ID = %q, want the node id", got.Assignees[1].ID)
	}
}

func TestUncheckingEveryAssigneeAsksForAnEmptySet(t *testing.T) {
	m := press(openAssignees(t), " ")

	got, ok := asked(t, m, "enter").(prview.SetAssigneesMsg)
	if !ok {
		t.Fatalf("enter sent %T, want a SetAssigneesMsg", asked(t, m, "enter"))
	}
	if len(got.Assignees) != 0 {
		t.Errorf("assignees = %q, want none", assigneeLogins(got))
	}
}

func TestApplyingAnUnchangedAssigneePickerWritesNothing(t *testing.T) {
	if got := asked(t, openAssignees(t), "enter"); got != nil {
		t.Errorf("an untouched picker sent %T, want nothing", got)
	}
}

func TestTheAssigneePickerListsSomebodyTheRepositoryPageMissed(t *testing.T) {
	d := sampleDetail()
	d.Assignees = append(d.Assignees, gh.Actor{ID: "U_9", Login: "ghost"})

	m := onRailRow(t, detailed(held(d), 200, 60), "+ Add assignee")
	m, _ = key(m, "enter")
	m.SetRepo(loadedRepo())

	box := menuBox(t, m, "Assignees")
	if !strings.Contains(box, "@ghost") {
		t.Fatalf("an assignee the repository's page did not reach is missing:\n%s", box)
	}
	for _, line := range strings.Split(box, "\n") {
		if strings.Contains(line, "@ghost") && !strings.Contains(line, "✓") {
			t.Errorf("the extra assignee is listed but not checked, so applying would drop them:\n%s", box)
		}
	}
}

func TestTheAssigneeSectionIsInertWithoutPermission(t *testing.T) {
	d := sampleDetail()
	d.Viewer.CanAssign = false

	frame := stripANSI(detailed(held(d), 200, 60).View())
	if strings.Contains(frame, "+ Add assignee") {
		t.Error("the add row is offered to a viewer who cannot assign")
	}
	if !strings.Contains(frame, "@drucial") {
		t.Error("the assignees themselves came off the rail with the add row")
	}
}

func TestTheRingWalksPastTheAssigneesWithoutPermission(t *testing.T) {
	d := sampleDetail()
	d.Viewer.CanAssign = false

	m := press(detailed(held(d), 200, 60), "1")
	for range 20 {
		m = press(m, "j")
		if got := markedRailRow(t, m.View()); got == "@drucial" || got == "+ Add assignee" {
			t.Fatalf("the ring stopped on %q, which the viewer cannot change", got)
		}
	}
}

func TestTheAssigneeRowsKeepTheirKeysBeforeTheDetailLands(t *testing.T) {
	loading := store.Detail{Status: store.StatusLoading}

	if !strings.Contains(stripANSI(detailed(loading, 200, 60).View()), "+ Add assignee") {
		t.Error("the add row went before the detail said anything about permissions")
	}
}
