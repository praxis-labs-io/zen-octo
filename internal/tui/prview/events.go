package prview

import (
	"strings"
	"time"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
)

var eventLabels = map[gh.TimelineKind]string{
	gh.TimelineMerged:         "merged this",
	gh.TimelineClosed:         "closed this",
	gh.TimelineReopened:       "reopened this",
	gh.TimelineReadyForReview: "marked this ready for review",
	gh.TimelineDraft:          "converted this to a draft",
	gh.TimelineForcePushed:    "force-pushed",
}

var runKinds = map[gh.TimelineKind]bool{
	gh.TimelineLabeled:         true,
	gh.TimelineUnlabeled:       true,
	gh.TimelineAssigned:        true,
	gh.TimelineUnassigned:      true,
	gh.TimelineReviewRequested: true,
	gh.TimelineReviewCancelled: true,
}

const metaWindow = time.Minute

// The window runs from the head of the run, so a slow drip of writes cannot chain into one line.
func metaRun(items []gh.TimelineItem) []gh.TimelineItem {
	head := items[0]
	if !runKinds[head.Kind] {
		return items[:1]
	}
	for i, item := range items {
		if item.Kind != head.Kind || item.Actor != head.Actor ||
			item.CreatedAt.Sub(head.CreatedAt) > metaWindow {
			return items[:i]
		}
	}
	return items
}

// Returns empty for a kind with no wording, so an event GitHub adds later costs no blank row.
func (m *Model) happened(run []gh.TimelineItem) string {
	last := run[len(run)-1]

	on := make([]string, 0, len(run))
	for _, item := range run {
		on = append(on, named(item.Kind, item.Subject))
	}

	verb := eventVerb(last, on)
	if verb == "" {
		return ""
	}
	return wrap(m.faint().Render("● ")+m.said(last.Actor, verb, m.theme.Subtle, last), m.bodyWidth())
}

func eventVerb(item gh.TimelineItem, on []string) string {
	switch item.Kind {
	case gh.TimelineLabeled:
		return pick(len(on), "added the label ", "added the labels ") + list(on)
	case gh.TimelineUnlabeled:
		return pick(len(on), "removed the label ", "removed the labels ") + list(on)
	case gh.TimelineAssigned:
		return "assigned " + list(on)
	case gh.TimelineUnassigned:
		return "unassigned " + list(on)
	case gh.TimelineReviewRequested:
		return pick(len(on), "requested a review from ", "requested reviews from ") + list(on)
	case gh.TimelineReviewCancelled:
		return pick(len(on),
			"cancelled a review request for ", "cancelled review requests for ") + list(on)
	case gh.TimelineBaseChanged:
		return "changed the base from " + item.Was + " to " + item.Subject
	}
	return eventLabels[item.Kind]
}

func named(kind gh.TimelineKind, subject string) string {
	switch kind {
	case gh.TimelineAssigned, gh.TimelineUnassigned,
		gh.TimelineReviewRequested, gh.TimelineReviewCancelled:
		return actorName(subject)
	}
	return subject
}

func list(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

func pick(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// Matched without case: GitHub answers with a login in the account's own case.
func actorName(login string) string {
	if strings.EqualFold(login, gh.CopilotLogin) {
		return "Copilot"
	}
	return comp.Handle(login)
}
