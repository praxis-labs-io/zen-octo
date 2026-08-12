package prview_test

import (
	"strings"
	"testing"
	"time"

	"github.com/zen-octo/zen-octo/internal/gh"
)

// conversationWith renders a page whose timeline is exactly these items.
//
// The threads go with it. They hang off reviews that are not on this page, so
// they would render at the end and push the line under test off the frame.
func conversationWith(t *testing.T, items ...gh.TimelineItem) string {
	t.Helper()

	d := sampleDetail()
	d.Timeline = items
	d.Threads = nil
	return stripANSI(detailed(held(d), 200, 60).View())
}

// happening is one event by drucial, an hour ago.
func happening(kind gh.TimelineKind, subject string) gh.TimelineItem {
	return gh.TimelineItem{
		Kind:      kind,
		Actor:     gh.Actor{Login: "drucial"},
		CreatedAt: time.Now().Add(-time.Hour),
		Subject:   subject,
	}
}

// Every kind reads as something somebody did, in the past tense the rest of the
// conversation is written in. A kind with no words renders to nothing and takes
// its row with it, which is the failure this catches: the line is missing, and
// nothing else on the page changes to say so.
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

// One picker apply writes a label set as one event per label. Three rows for
// one keystroke is what buries the discussion between them.
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

// The actor is part of the run, where it is not for a push. Two people
// labelling in a row is two things happening, and folding them credits one of
// them with the other's work.
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

// A run is one keystroke's worth of events, not everything of a kind that
// happens to sit together. Two requests for the same person minutes apart are
// two things somebody did, and PR #20 carries exactly that pair: folded, they
// read as "requested reviews from Copilot and Copilot".
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

// A swap writes an add and a remove at the same instant. Folding them into one
// line would mean saying which way each name went, and the sentence stops being
// a sentence.
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

// Copilot is named the way the reviewer picker names it. Its login is what the
// rail shows and not a word anybody would think to look for, and a request for
// it is the one metadata write with no other evidence on the page.
func TestARequestNamesCopilotAndATeamTheWayTheRailDoes(t *testing.T) {
	out := conversationWith(t,
		happening(gh.TimelineReviewRequested, gh.CopilotLogin),
		happening(gh.TimelineReviewRequested, "zen-octo/maintainers"),
	)

	if want := "requested reviews from Copilot and @zen-octo/maintainers"; !strings.Contains(out, want) {
		t.Errorf("the conversation is missing %q:\n%s", want, out)
	}
	if strings.Contains(out, "@"+gh.CopilotLogin) {
		t.Errorf("Copilot is named by its login:\n%s", out)
	}
}

// A merge falling off a heavily labelled pull request would leave the
// conversation reading as one nobody ever merged. The comments and the threads
// each say what their page did not reach, and the events now do too.
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

// GitHub answers with a login in the account's own case rather than the case it
// was asked in, so the bot's name cannot hang on an exact match.
func TestCopilotIsNamedWhateverCaseItArrivesIn(t *testing.T) {
	out := conversationWith(t, happening(gh.TimelineReviewRequested, "Copilot-Pull-Request-Reviewer"))

	if want := "requested a review from Copilot"; !strings.Contains(out, want) {
		t.Errorf("the conversation is missing %q:\n%s", want, out)
	}
}

// An event is something to read, not something to act on, so tab walks past it
// the way it walks past a push. A run landing between two cards must not become
// a stop between them.
func TestTabWalksPastAMetadataEvent(t *testing.T) {
	d := sampleDetail()
	d.Timeline = append([]gh.TimelineItem{
		happening(gh.TimelineLabeled, "bug"),
		happening(gh.TimelineReviewRequested, "nkr"),
	}, d.Timeline...)

	m := detailed(held(d), 200, 60)
	for i, card := range []string{cardDescription, cardComment, cardReview, cardThread} {
		if got := focusedCard(t, tabbed(m, i+1).View()); !strings.HasPrefix(got, card) {
			t.Errorf("tab %d focused %q, want %q", i+1, got, card)
		}
	}
}
