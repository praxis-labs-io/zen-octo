package prview

import (
	"strings"
	"time"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
)

// The one-line entries in the conversation: what happened to the pull request,
// as against what somebody wrote about it. Comments, reviews and commits render
// as blocks in conversation.go; everything here is a single faint row that tab
// walks past.

// eventLabels is the verb for a kind that acts on the pull request as a whole,
// and so names nothing it was done to.
var eventLabels = map[gh.TimelineKind]string{
	gh.TimelineMerged:         "merged this",
	gh.TimelineClosed:         "closed this",
	gh.TimelineReopened:       "reopened this",
	gh.TimelineReadyForReview: "marked this ready for review",
	gh.TimelineDraft:          "converted this to a draft",
	gh.TimelineForcePushed:    "force-pushed",
}

// runKinds is the kinds that fold into one line when they repeat. Everything
// else stands alone: a base change carries its own pair of branches, and
// nothing can be merged or force-pushed twice in a row.
var runKinds = map[gh.TimelineKind]bool{
	gh.TimelineLabeled:         true,
	gh.TimelineUnlabeled:       true,
	gh.TimelineAssigned:        true,
	gh.TimelineUnassigned:      true,
	gh.TimelineReviewRequested: true,
	gh.TimelineReviewCancelled: true,
}

// metaWindow is how long one keystroke's events can take to arrive. A picker
// apply is a single mutation and GitHub stamps its events within the same
// second, so this is slack rather than a threshold.
const metaWindow = time.Minute

// metaRun is the events that fold together, from the head of a timeline: the
// same kind by the same person, inside one window of time. One picker apply
// writes a label set as one event per label, and three rows for one keystroke
// is what buries the discussion between them.
//
// The actor is part of the run where it is not for a push. Two people labelling
// in a row is two things happening, and a mixed push is one thing with two
// authors.
//
// So is the clock, and it is what a run of pushes has no use for. A rebase
// written over a week is still one push, but two review requests for the same
// person an hour apart are two things somebody did, and folding them reads as
// "requested reviews from Copilot and Copilot". The window runs from the head
// rather than the item before, so a slow drip of writes cannot chain into one
// line covering an afternoon.
//
// A kind that does not fold comes back as a run of one, so a caller walks the
// timeline the same way whatever it is looking at.
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

// happened is a run of events as one line: who, what they did to what, and
// when. It returns empty for a kind this build has no words for, which is what
// keeps an event GitHub adds later from costing a blank row.
//
// The run is dated by its last event, the way a push is. Nobody read the first
// label of three going on as a separate moment.
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
	return wrap(m.faint().Render("● ")+m.said(last.Actor, verb, m.theme.Faint, last), m.bodyWidth())
}

// eventVerb is what a run did, in the past tense the conversation reads in. on
// is every subject in the run, already named for the reader.
//
// The count stays out of the words. "added 2 labels ready and wip" reads as
// arithmetic, and the names are already there to be counted.
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

// named is a subject as the reader should see it. A label and a branch are
// words; everyone else is a handle, so a label called after somebody cannot be
// read as them.
func named(kind gh.TimelineKind, subject string) string {
	switch kind {
	case gh.TimelineAssigned, gh.TimelineUnassigned,
		gh.TimelineReviewRequested, gh.TimelineReviewCancelled:
		return actorName(subject)
	}
	return subject
}

// list is names in a sentence: one alone, two joined by and, more with commas
// and an and before the last.
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

// pick is the wording for one against the wording for more. A count of none
// takes the plural, which is the only reading that is not a claim about a
// subject nobody sent.
func pick(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// actorName is how a person, a bot or a team reads. Copilot is named for itself
// rather than by its login, which is the one the rail shows and not a word
// anybody would think to look for.
//
// Matched without case, because GitHub answers with the login in the account's
// own case rather than the case it was asked in. A reviewer panel and a
// timeline event report the same bot, and one of them arriving capitalised
// would put its raw login on screen next to its name.
func actorName(login string) string {
	if strings.EqualFold(login, gh.CopilotLogin) {
		return "Copilot"
	}
	return comp.Handle(login)
}
