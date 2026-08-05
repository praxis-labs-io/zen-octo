package gh

import (
	"context"
	"strings"
	"testing"
	"time"
)

// detailBody is one pull request with everything the conversation reads: a
// comment from a deleted account, two reviews, a thread under each, an event,
// a truncated comment page, and a rollup that is neither all green nor all red.
const detailBody = `{
  "rateLimit": {"limit": 5000, "cost": 3, "remaining": 4712, "resetAt": "2026-08-05T18:00:00Z"},
  "node": {
    "id": "PR_412", "number": 412, "title": "Fix auth retry",
    "url": "https://github.com/zen-octo/zen-octo/pull/412",
    "isDraft": false, "state": "OPEN",
    "createdAt": "2026-08-01T10:00:00Z", "updatedAt": "2026-08-05T11:00:00Z",
    "additions": 42, "deletions": 7, "changedFiles": 3,
    "headRefName": "fix-auth", "baseRefName": "main",
    "reviewDecision": "CHANGES_REQUESTED",
    "mergeable": "MERGEABLE",
    "mergeStateStatus": "BLOCKED",
    "baseRef": {"compare": {"behindBy": 4}},
    "body": "Caps the backoff.",
    "author": {"login": "drucial"},
    "repository": {"nameWithOwner": "zen-octo/zen-octo"},

    "labels": {"nodes": [{"name": "bug", "color": "d73a4a"}]},
    "assignees": {"nodes": [{"login": "drucial"}]},
    "reviewRequests": {"nodes": [
      {"requestedReviewer": {"login": "nkr"}},
      {"requestedReviewer": {"login": "copilot-pull-request-reviewer"}},
      {"requestedReviewer": {"name": "maintainers"}},
      {"requestedReviewer": null}
    ]},

    "comments": {
      "totalCount": 14,
      "nodes": [
        {"author": null, "createdAt": "2026-08-02T09:00:00Z", "body": "From a deleted account."},
        {"author": {"login": "octobot"}, "createdAt": "2026-08-04T09:00:00Z", "body": "Coverage held."}
      ]
    },

    "reviews": {
      "totalCount": 3,
      "nodes": [
        {"id": "REV_1", "state": "CHANGES_REQUESTED", "body": "Two things.",
         "submittedAt": "2026-08-03T09:00:00Z", "author": {"login": "nkr"}},
        {"id": "REV_2", "state": "APPROVED", "body": "",
         "submittedAt": "2026-08-05T09:00:00Z", "author": {"login": "nkr"}},
        {"id": "REV_3", "state": "PENDING", "body": "not sent yet",
         "submittedAt": null, "author": {"login": "drucial"}}
      ]
    },

    "reviewThreads": {
      "totalCount": 5,
      "nodes": [
        {"isResolved": false, "isOutdated": false, "path": "internal/gh/client.go", "line": 42,
         "originalLine": 40,
         "comments": {"totalCount": 2, "nodes": [
           {"author": {"login": "nkr"}, "createdAt": "2026-08-03T09:00:00Z",
            "body": "Needs a ceiling.", "pullRequestReview": {"id": "REV_1"}},
           {"author": {"login": "drucial"}, "createdAt": "2026-08-03T10:00:00Z",
            "body": "Capped.", "pullRequestReview": {"id": "REV_1"}}
         ]}},
        {"isResolved": true, "isOutdated": true, "path": "internal/store/store.go", "line": null,
         "originalLine": 88,
         "comments": {"totalCount": 1, "nodes": [
           {"author": {"login": "nkr"}, "createdAt": "2026-08-05T09:00:00Z",
            "body": "Typo.", "pullRequestReview": {"id": "REV_2"}}
         ]}}
      ]
    },

    "timelineItems": {"nodes": [
      {"__typename": "HeadRefForcePushedEvent", "createdAt": "2026-08-04T12:00:00Z",
       "actor": {"login": "drucial"}},
      {"__typename": "LabeledEvent", "createdAt": "2026-08-04T13:00:00Z", "actor": null}
    ]},

    "statusCheckRollup": {"nodes": [{"commit": {"statusCheckRollup": {
      "state": "FAILURE",
      "contexts": {"nodes": [
        {"__typename": "CheckRun", "name": "test", "status": "COMPLETED", "conclusion": "SUCCESS",
         "checkSuite": {"workflowRun": {"workflow": {"name": "Rails Unit Tests"}}}},
        {"__typename": "CheckRun", "name": "test", "status": "COMPLETED", "conclusion": "NEUTRAL",
         "checkSuite": {"workflowRun": {"workflow": {"name": "Rails Lint"}}}},
        {"__typename": "CheckRun", "name": "build", "status": "COMPLETED", "conclusion": "FAILURE",
         "checkSuite": {"workflowRun": {"workflow": {"name": "Build"}}}},
        {"__typename": "CheckRun", "name": "windows", "status": "COMPLETED", "conclusion": "SKIPPED",
         "checkSuite": {"workflowRun": null}},
        {"__typename": "CheckRun", "name": "e2e", "status": "IN_PROGRESS", "conclusion": "",
         "startedAt": "2026-08-05T10:00:00Z",
         "checkSuite": {"workflowRun": {"workflow": {"name": "E2E Tests"}}}},
        {"__typename": "CheckRun", "name": "e2e", "status": "COMPLETED", "conclusion": "FAILURE",
         "startedAt": "2026-08-05T09:00:00Z",
         "checkSuite": {"workflowRun": {"workflow": {"name": "E2E Tests"}}}},
        {"__typename": "StatusContext", "context": "codecov", "state": "SUCCESS"},
        {"__typename": "StatusContext", "context": "netlify", "state": "PENDING"},
        {"__typename": "StatusContext", "context": "sonar", "state": "ERROR"}
      ]}
    }}}]}
  }
}`

func fetchDetail(t *testing.T) PullRequestDetail {
	t.Helper()

	res, err := newWithDoer(&fakeDoer{body: detailBody}).PullRequest(context.Background(), "PR_412", "fix-auth")
	if err != nil {
		t.Fatalf("PullRequest() error = %v, want nil", err)
	}
	return res.Detail
}

func TestPullRequestMapsResponseToDomainTypes(t *testing.T) {
	d := fetchDetail(t)

	if d.Number != 412 || d.Title != "Fix auth retry" {
		t.Errorf("row = #%d %q, want #412 \"Fix auth retry\"", d.Number, d.Title)
	}
	if d.Body != "Caps the backoff." {
		t.Errorf("Body = %q, want the description", d.Body)
	}
	if len(d.Labels) != 1 || d.Labels[0].Color != "d73a4a" {
		t.Errorf("Labels = %+v, want one carrying GitHub's own hex", d.Labels)
	}
	if len(d.Assignees) != 1 || d.Assignees[0].Login != "drucial" {
		t.Errorf("Assignees = %+v, want [drucial]", d.Assignees)
	}

}

// A submitted review takes its author off reviewRequests, so the panel is the
// two lists together. Copilot reviews and then vanishes from the requests,
// which is how it went missing.
func TestReviewersAreWhoHasReviewedAndWhoWasAsked(t *testing.T) {
	want := []Reviewer{
		// nkr reviewed twice, and the last word is the one that counts. The
		// open thread on the first review is still theirs.
		{Actor: Actor{Login: "nkr"}, State: ReviewStateApproved, Unresolved: 1},
		// A bot and a team, neither of which has answered yet.
		{Actor: Actor{Login: "copilot-pull-request-reviewer"}},
		{Actor: Actor{Login: "maintainers"}},
	}

	got := fetchDetail(t).Reviewers
	if len(got) != len(want) {
		t.Fatalf("Reviewers = %+v, want %+v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("Reviewers[%d] = %+v, want %+v", i, got[i], w)
		}
	}
}

// Comments, reviews and events arrive in three separate connections. The
// conversation reads top to bottom, so they have to be one list in time order.
func TestTheTimelineIsOneListInTheOrderThingsHappened(t *testing.T) {
	got := fetchDetail(t).Timeline

	want := []struct {
		kind  TimelineKind
		login string
	}{
		{TimelineComment, ""}, // the deleted account, 2nd Aug
		{TimelineReview, "nkr"},
		{TimelineComment, "octobot"},
		{TimelineForcePushed, "drucial"},
		{TimelineReview, "nkr"},
	}

	if len(got) != len(want) {
		t.Fatalf("timeline has %d entries, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Kind != w.kind || got[i].Actor.Login != w.login {
			t.Errorf("timeline[%d] = %s by %q, want %s by %q",
				i, got[i].Kind, got[i].Actor.Login, w.kind, w.login)
		}
	}
}

// A pending review is the viewer's own unsubmitted draft. Nobody else can see
// it, and it has no timestamp to sort by.
func TestAnUnsubmittedReviewStaysOut(t *testing.T) {
	for _, item := range fetchDetail(t).Timeline {
		if item.ID == "REV_3" {
			t.Error("the timeline carries a pending review")
		}
	}
}

func TestAThreadNamesTheReviewThatOpenedIt(t *testing.T) {
	threads := fetchDetail(t).Threads
	if len(threads) != 2 {
		t.Fatalf("got %d threads, want 2", len(threads))
	}

	first := threads[0]
	if first.ReviewID != "REV_1" {
		t.Errorf("ReviewID = %q, want REV_1 from its first comment", first.ReviewID)
	}
	if first.Line != 42 || len(first.Comments) != 2 {
		t.Errorf("thread = line %d with %d comments, want line 42 with 2", first.Line, len(first.Comments))
	}
	if first.Comments[0].Body != "Needs a ceiling." {
		t.Errorf("first comment = %q, want the one that opened the thread", first.Comments[0].Body)
	}

	// An outdated thread has no current line, only the one it was written
	// against. Falling back is what keeps the anchor readable.
	second := threads[1]
	if !second.IsResolved || !second.IsOutdated {
		t.Errorf("second thread = resolved %v outdated %v, want both", second.IsResolved, second.IsOutdated)
	}
	if second.Line != 88 {
		t.Errorf("Line = %d, want 88 from originalLine", second.Line)
	}
}

// A page that stopped short has to say so. A dropped comment that reads as no
// comment is the failure worth a field.
func TestWhatThePageDidNotReachIsReported(t *testing.T) {
	d := fetchDetail(t)

	if d.MoreComments != 12 {
		t.Errorf("MoreComments = %d, want 12 of 14 past the two returned", d.MoreComments)
	}
	if d.MoreThreads != 3 {
		t.Errorf("MoreThreads = %d, want 3 of 5 past the two returned", d.MoreThreads)
	}
}

func TestTheRollupCountsWhatIsBehindIt(t *testing.T) {
	d := fetchDetail(t)

	if d.Rollup.State != CheckStateFailure {
		t.Errorf("State = %q, want %q", d.Rollup.State, CheckStateFailure)
	}
	counts := [4]int{d.Rollup.Passed, d.Rollup.Failed, d.Rollup.Pending, d.Rollup.Skipped}
	if want := [4]int{3, 2, 2, 1}; counts != want {
		t.Errorf("passed/failed/pending/skipped = %v, want %v", counts, want)
	}

	// A rollup that says "failing" does not say which one, so the checks come
	// back too, in the order GitHub listed them. A job is named for what it
	// does, so the workflow it ran under is what tells two "test" jobs apart.
	want := []Check{
		{Name: "test", Workflow: "Rails Unit Tests"},
		{Name: "test", Workflow: "Rails Lint"},
		{Name: "build", Workflow: "Build"},
		{Name: "windows"},                    // a suite with no workflow run behind it
		{Name: "e2e", Workflow: "E2E Tests"}, // twice on the wire, once here
		{Name: "codecov"},                    // status contexts have no workflow at all
		{Name: "netlify"},
		{Name: "sonar"},
	}
	if len(d.Rollup.Checks) != len(want) {
		t.Fatalf("got %d checks, want %d: %+v", len(d.Rollup.Checks), len(want), d.Rollup.Checks)
	}
	for i, w := range want {
		if got := d.Rollup.Checks[i]; got.Name != w.Name || got.Workflow != w.Workflow {
			t.Errorf("check %d = %q under %q, want %q under %q", i, got.Name, got.Workflow, w.Name, w.Workflow)
		}
	}

	// A check run and a status context are different types on the wire saying
	// the same thing, and they fold into one vocabulary.
	states := []CheckState{
		CheckStateSuccess, CheckStateSuccess, CheckStateFailure, CheckStateSkipped,
		CheckStatePending, CheckStateSuccess, CheckStatePending, CheckStateFailure,
	}
	for i, state := range states {
		if got := d.Rollup.Checks[i].State; got != state {
			t.Errorf("check %q = %q, want %q", want[i].Name, got, state)
		}
	}
	// The embedded row carries the same answer, so a screen reading either the
	// search result or the detail sees one state.
	if d.Checks != CheckStateFailure {
		t.Errorf("Checks = %q, want the rollup state", d.Checks)
	}
}

func TestPullRequestReportsWhatTheCallCost(t *testing.T) {
	res, err := newWithDoer(&fakeDoer{body: detailBody}).PullRequest(context.Background(), "PR_412", "fix-auth")
	if err != nil {
		t.Fatalf("PullRequest() error = %v, want nil", err)
	}

	want := RateLimit{Limit: 5000, Cost: 3, Remaining: 4712,
		ResetAt: time.Date(2026, 8, 5, 18, 0, 0, 0, time.UTC)}
	if res.RateLimit != want {
		t.Errorf("RateLimit = %+v, want %+v", res.RateLimit, want)
	}
}

func TestPullRequestPassesTheNodeID(t *testing.T) {
	doer := &fakeDoer{body: detailBody}

	if _, err := newWithDoer(doer).PullRequest(context.Background(), "PR_412", "fix-auth"); err != nil {
		t.Fatalf("PullRequest() error = %v, want nil", err)
	}
	if doer.gotVars["id"] != "PR_412" {
		t.Errorf("id = %v, want the node id unmodified", doer.gotVars["id"])
	}
	// The head branch goes with it: the query asks how far behind the base it
	// has fallen, and GraphQL cannot read it off a sibling field.
	if doer.gotVars["head"] != "fix-auth" {
		t.Errorf("head = %v, want the branch it is merging from", doer.gotVars["head"])
	}
}

// An id that is not a pull request comes back as an empty node rather than an
// error, which would otherwise decode into a blank screen.
func TestAnIDBehindNoPullRequestIsAnError(t *testing.T) {
	doer := &fakeDoer{body: `{"node": {}}`}

	_, err := newWithDoer(doer).PullRequest(context.Background(), "I_1", "topic")
	if err == nil {
		t.Fatal("PullRequest() error = nil, want an error")
	}
	if !strings.Contains(err.Error(), "I_1") {
		t.Errorf("error = %q, want it to name the id", err)
	}
}

// A re-run leaves the previous attempt in the connection, so the same job comes
// back twice saying two different things. Only the latest one is true.
func TestARerunCheckIsReportedOnce(t *testing.T) {
	d := fetchDetail(t)

	seen := 0
	for _, check := range d.Rollup.Checks {
		if check.Workflow == "E2E Tests" && check.Name == "e2e" {
			seen++
			if check.State != CheckStatePending {
				t.Errorf("e2e = %q, want the state of the run that started last", check.State)
			}
		}
	}
	if seen != 1 {
		t.Errorf("e2e appears %d times, want the latest run only", seen)
	}
}

func TestMergeabilityFoldsTheTwoFieldsGitHubAnswersWith(t *testing.T) {
	tests := []struct {
		name      string
		mergeable string
		status    string
		want      MergeState
	}{
		{name: "blocked", mergeable: "MERGEABLE", status: "BLOCKED", want: MergeBlocked},
		{name: "clean", mergeable: "MERGEABLE", status: "CLEAN", want: MergeClean},
		{name: "behind", mergeable: "MERGEABLE", status: "BEHIND", want: MergeBehind},
		{name: "hooks read as clean", mergeable: "MERGEABLE", status: "HAS_HOOKS", want: MergeClean},
		// mergeable is the field that knows about conflicts, and it overrules
		// whatever the status says.
		{name: "conflicts win", mergeable: "CONFLICTING", status: "BLOCKED", want: MergeConflicting},
		// GitHub computes this lazily and says UNKNOWN until it has.
		{name: "not computed yet", mergeable: "UNKNOWN", status: "UNKNOWN", want: MergeUnknown},
		{name: "a state we do not know", mergeable: "MERGEABLE", status: "SOMETHING_NEW", want: MergeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mergeState(tt.mergeable, tt.status); got != tt.want {
				t.Errorf("mergeState(%q, %q) = %q, want %q", tt.mergeable, tt.status, got, tt.want)
			}
		})
	}
}

func TestHowFarBehindTheBaseComesBack(t *testing.T) {
	if got := fetchDetail(t).BehindBy; got != 4 {
		t.Errorf("BehindBy = %d, want 4", got)
	}
}

// A fake doer never executes the query, so nothing else in this file can catch
// a missing fragment: the fixture decodes whatever it is handed. Copilot is
// requested as a Bot, and leaving that fragment out drops it from the list
// with no error anywhere.
func TestTheQueryAsksForEveryShapeOfReviewer(t *testing.T) {
	for _, want := range []string{"... on User { login }", "... on Bot { login }", "... on Team { name }"} {
		if !strings.Contains(pullRequestQuery, want) {
			t.Errorf("the query does not ask for %q", want)
		}
	}
}

// A reviewer who left unanswered questions is waiting on the same thing as one
// who asked for changes, whatever they called the review.
func TestOpenThreadsCountAgainstTheReviewerWhoOpenedThem(t *testing.T) {
	for _, r := range fetchDetail(t).Reviewers {
		switch r.Actor.Login {
		case "nkr":
			// One thread open on REV_1, one resolved on REV_2.
			if r.Unresolved != 1 {
				t.Errorf("nkr has %d open threads, want 1", r.Unresolved)
			}
		default:
			if r.Unresolved != 0 {
				t.Errorf("%s has %d open threads, want none", r.Actor.Login, r.Unresolved)
			}
		}
	}
}
