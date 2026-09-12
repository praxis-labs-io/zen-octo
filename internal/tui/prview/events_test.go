package prview_test

import (
	"strings"
	"testing"
	"time"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

func conversationWith(t *testing.T, items ...gh.TimelineItem) string {
	t.Helper()

	d := sampleDetail()
	d.Timeline = items
	d.Threads = nil
	return stripANSI(detailed(held(d), 200, 60).View())
}

func happening(kind gh.TimelineKind, subject string) gh.TimelineItem {
	return gh.TimelineItem{
		Kind:      kind,
		Actor:     gh.Actor{Login: "drucial"},
		CreatedAt: time.Now().Add(-time.Hour),
		Subject:   subject,
	}
}

func TestEveryEventReadsAsASentence(t *testing.T) {
	baseChanged := happening(gh.TimelineBaseChanged, "develop")
	baseChanged.Was = "main"

	cases := []struct {
		item gh.TimelineItem
		want string
	}{
		{happening(gh.TimelineMerged, ""), "drucial · merged this"},
		{happening(gh.TimelineClosed, ""), "drucial · closed this"},
		{happening(gh.TimelineReopened, ""), "drucial · reopened this"},
		{happening(gh.TimelineReadyForReview, ""), "drucial · marked this ready for review"},
		{happening(gh.TimelineDraft, ""), "drucial · converted this to a draft"},
		{happening(gh.TimelineForcePushed, ""), "drucial · force-pushed"},
		{happening(gh.TimelineLabeled, "bug"), "drucial · added the label bug"},
		{happening(gh.TimelineUnlabeled, "wip"), "drucial · removed the label wip"},
		{happening(gh.TimelineAssigned, "nkr"), "drucial · assigned @nkr"},
		{happening(gh.TimelineUnassigned, "nkr"), "drucial · unassigned @nkr"},
		{happening(gh.TimelineReviewRequested, "nkr"), "drucial · requested a review from @nkr"},
		{happening(gh.TimelineReviewCancelled, "nkr"), "drucial · cancelled a review request for @nkr"},
		{baseChanged, "drucial · changed the base from main to develop"},
	}

	for _, c := range cases {
		t.Run(string(c.item.Kind), func(t *testing.T) {
			if out := conversationWith(t, c.item); !strings.Contains(out, c.want) {
				t.Errorf("the conversation is missing %q:\n%s", c.want, out)
			}
		})
	}
}

func TestARunOfLabelsIsOneLineNamingEveryOne(t *testing.T) {
	out := conversationWith(t,
		happening(gh.TimelineLabeled, "bug"),
		happening(gh.TimelineLabeled, "needs-docs"),
		happening(gh.TimelineLabeled, "ready"),
	)

	if want := "drucial · added the labels bug, needs-docs and ready"; !strings.Contains(out, want) {
		t.Errorf("the run did not fold into %q:\n%s", want, out)
	}
	if n := strings.Count(out, "added the label"); n != 1 {
		t.Errorf("the run rendered %d lines, want one", n)
	}
}

func TestTwoPeopleLabellingInARowAreTwoLines(t *testing.T) {
	mine := happening(gh.TimelineLabeled, "bug")
	theirs := happening(gh.TimelineLabeled, "needs-docs")
	theirs.Actor = gh.Actor{Login: "nkr"}

	out := conversationWith(t, mine, theirs)

	for _, want := range []string{
		"drucial · added the label bug",
		"nkr · added the label needs-docs",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the conversation is missing %q:\n%s", want, out)
		}
	}
}

func TestTwoRequestsMinutesApartAreTwoLines(t *testing.T) {
	first := happening(gh.TimelineReviewRequested, gh.CopilotLogin)
	again := happening(gh.TimelineReviewRequested, gh.CopilotLogin)
	again.CreatedAt = first.CreatedAt.Add(9 * time.Minute)

	out := conversationWith(t, first, again)

	if strings.Contains(out, "Copilot and Copilot") {
		t.Errorf("the two requests folded into one line naming Copilot twice:\n%s", out)
	}
	if n := strings.Count(out, "requested a review from Copilot"); n != 2 {
		t.Errorf("the two requests rendered %d lines, want two:\n%s", n, out)
	}
}

func TestAnAddAndARemoveAreTwoLines(t *testing.T) {
	out := conversationWith(t,
		happening(gh.TimelineUnlabeled, "wip"),
		happening(gh.TimelineLabeled, "ready"),
	)

	for _, want := range []string{
		"drucial · removed the label wip",
		"drucial · added the label ready",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the conversation is missing %q:\n%s", want, out)
		}
	}
}

func TestARequestNamesCopilotAndATeamTheWayTheRailDoes(t *testing.T) {
	out := conversationWith(t,
		happening(gh.TimelineReviewRequested, gh.CopilotLogin),
		happening(gh.TimelineReviewRequested, "acme/maintainers"),
	)

	if want := "requested reviews from Copilot and @acme/maintainers"; !strings.Contains(out, want) {
		t.Errorf("the conversation is missing %q:\n%s", want, out)
	}
	if strings.Contains(out, "@"+gh.CopilotLogin) {
		t.Errorf("Copilot is named by its login:\n%s", out)
	}
}

func TestTheEventsTheWindowCutOffAreSaidToBeThere(t *testing.T) {
	d := sampleDetail()
	d.Timeline = []gh.TimelineItem{happening(gh.TimelineLabeled, "bug")}
	d.Threads = nil
	d.MoreEvents = 4

	out := stripANSI(detailed(held(d), 200, 60).View())
	if want := "4 earlier events on GitHub"; !strings.Contains(out, want) {
		t.Errorf("the conversation is missing %q:\n%s", want, out)
	}
}

func TestCopilotIsNamedWhateverCaseItArrivesIn(t *testing.T) {
	out := conversationWith(t, happening(gh.TimelineReviewRequested, "Copilot-Pull-Request-Reviewer"))

	if want := "requested a review from Copilot"; !strings.Contains(out, want) {
		t.Errorf("the conversation is missing %q:\n%s", want, out)
	}
}

func TestTabWalksPastAMetadataEvent(t *testing.T) {
	d := sampleDetail()
	d.Timeline = append([]gh.TimelineItem{
		happening(gh.TimelineLabeled, "bug"),
		happening(gh.TimelineReviewRequested, "nkr"),
	}, d.Timeline...)

	m := detailed(held(d), 200, 60)
	for i, card := range []string{cardDescription, cardComment, cardReview, cardThread} {
		if got := focusedCard(t, walked(m, i+1).View()); !strings.HasPrefix(got, card) {
			t.Errorf("tab %d focused %q, want %q", i+1, got, card)
		}
	}
}
