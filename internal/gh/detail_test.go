package gh

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

const detailBody = `{
  "rateLimit": {"limit": 5000, "cost": 3, "remaining": 4712, "resetAt": "2026-08-05T18:00:00Z"},
  "node": {
    "id": "PR_412", "number": 412, "title": "Fix auth retry",
    "url": "https://github.com/acme/rocket/pull/412",
    "isDraft": false, "state": "OPEN",
    "viewerCanUpdate": true, "viewerCanClose": true, "viewerCanReopen": false,
    "viewerCanAssign": true,
    "viewerCanMergeAsAdmin": false, "isCrossRepository": false,
    "viewerCanReact": true,
    "reactionGroups": [
      {"content": "HEART", "viewerHasReacted": true, "reactors": {"totalCount": 2}},
      {"content": "EYES", "viewerHasReacted": false, "reactors": {"totalCount": 0}}
    ],
    "createdAt": "2026-08-01T10:00:00Z", "updatedAt": "2026-08-05T11:00:00Z",
    "additions": 42, "deletions": 7, "changedFiles": 3,
    "headRefName": "fix-auth", "baseRefName": "main",
    "headRefOid": "9f1c2b7", "headRef": {"id": "REF_88"},
    "reviewDecision": "CHANGES_REQUESTED",
    "mergeable": "MERGEABLE",
    "mergeStateStatus": "BLOCKED",
    "baseRef": {"compare": {"behindBy": 4}},
    "body": "Caps the backoff.",
    "author": {"login": "drucial"},
    "repository": {"nameWithOwner": "acme/rocket"},
    "mergeHeadline": "Merge pull request #412 from acme/fix-auth",
    "mergeBody": "Fix auth retry",
    "squashHeadline": "Fix auth retry (#412)",
    "squashBody": "* Cap the backoff\n\n* Add a test",

    "labels": {"nodes": [{"name": "bug", "color": "d73a4a"}]},
    "assignees": {"nodes": [{"id": "U_1", "login": "drucial"}]},
    "reviewRequests": {"nodes": [
      {"requestedReviewer": {"login": "nkr"}},
      {"requestedReviewer": {"login": "copilot-pull-request-reviewer"}},
      {"requestedReviewer": {"slug": "core-maintainers", "organization": {"login": "acme"}}},
      {"requestedReviewer": null}
    ]},

    "comments": {
      "totalCount": 14,
      "nodes": [
        {"id": "IC_1", "author": null, "createdAt": "2026-08-02T09:00:00Z",
         "body": "From a deleted account.",
         "viewerDidAuthor": false, "viewerCanUpdate": false,
         "viewerCanDelete": true, "viewerCanReact": true},
        {"id": "IC_2", "author": {"login": "octobot"}, "createdAt": "2026-08-04T09:00:00Z",
         "body": "Coverage held.",
         "viewerDidAuthor": false, "viewerCanUpdate": true,
         "viewerCanDelete": true, "viewerCanReact": true,
         "reactionGroups": [
           {"content": "THUMBS_UP", "viewerHasReacted": true, "reactors": {"totalCount": 3}},
           {"content": "LAUGH", "viewerHasReacted": false, "reactors": {"totalCount": 0}},
           {"content": "ROCKET", "viewerHasReacted": false, "reactors": {"totalCount": 1}}
         ]}
      ]
    },

    "reviews": {
      "totalCount": 3,
      "nodes": [
        {"id": "REV_1", "state": "CHANGES_REQUESTED", "body": "Two things.",
         "submittedAt": "2026-08-03T09:00:00Z", "author": {"login": "nkr"},
         "viewerDidAuthor": false, "viewerCanUpdate": false,
         "viewerCanDelete": true, "viewerCanReact": true,
         "reactionGroups": [
           {"content": "CONFUSED", "viewerHasReacted": false, "reactors": {"totalCount": 1}}
         ]},
        {"id": "REV_2", "state": "APPROVED", "body": "",
         "submittedAt": "2026-08-05T09:00:00Z", "author": {"login": "nkr"},
         "viewerDidAuthor": false, "viewerCanUpdate": false,
         "viewerCanDelete": true, "viewerCanReact": true},
        {"id": "REV_3", "state": "PENDING", "body": "not sent yet",
         "submittedAt": null, "author": {"login": "drucial"},
         "viewerDidAuthor": true, "viewerCanUpdate": true,
         "viewerCanDelete": true, "viewerCanReact": false}
      ]
    },

    "reviewThreads": {
      "totalCount": 5,
      "nodes": [
        {"id": "RT_1", "isResolved": false, "isOutdated": false,
         "viewerCanReply": true, "viewerCanResolve": true, "viewerCanUnresolve": false,
         "path": "internal/gh/client.go", "line": 42,
         "startLine": 40, "originalLine": 40, "originalStartLine": 38, "diffSide": "RIGHT",
         "comments": {"totalCount": 2, "nodes": [
           {"id": "RC_1", "author": {"login": "nkr"}, "createdAt": "2026-08-03T09:00:00Z",
            "body": "Needs a ceiling.", "pullRequestReview": {"id": "REV_1"},
            "viewerDidAuthor": false, "viewerCanUpdate": false,
            "viewerCanDelete": true, "viewerCanReact": true,
            "reactionGroups": [
              {"content": "EYES", "viewerHasReacted": true, "reactors": {"totalCount": 4}}
            ],
            "diffHunk": "@@ -39,3 +39,4 @@\n \tfor {\n-\t\ttime.Sleep(delay)\n+\t\tdelay = min(delay*2, fetchTimeout)"},
           {"id": "RC_2", "author": {"login": "drucial"}, "createdAt": "2026-08-03T10:00:00Z",
            "body": "Capped.", "pullRequestReview": {"id": "REV_1"},
            "viewerDidAuthor": true, "viewerCanUpdate": true,
            "viewerCanDelete": true, "viewerCanReact": true}
         ]}},
        {"id": "RT_2", "isResolved": true, "isOutdated": true,
         "viewerCanReply": true, "viewerCanResolve": false, "viewerCanUnresolve": true,
         "path": "internal/store/store.go", "line": null,
         "startLine": null, "originalLine": 88, "originalStartLine": 86, "diffSide": "LEFT",
         "comments": {"totalCount": 1, "nodes": [
           {"id": "RC_3", "author": {"login": "nkr"}, "createdAt": "2026-08-05T09:00:00Z",
            "body": "Typo.", "pullRequestReview": {"id": "REV_2"},
            "viewerDidAuthor": false, "viewerCanUpdate": false,
            "viewerCanDelete": true, "viewerCanReact": true}
         ]}}
      ]
    },

    "commits": {
      "totalCount": 9,
      "nodes": [
        {"commit": {"oid": "a3f91c2d5e", "abbreviatedOid": "a3f91c2",
          "messageHeadline": "Cap the backoff",
          "messageBody": "The retry loop had no ceiling.",
          "committedDate": "2026-08-02T08:00:00Z",
          "author": {"name": "Drew White", "user": {"login": "drucial"}},
          "statusCheckRollup": {"state": "SUCCESS"}}},
        {"commit": {"oid": "7b20ef4a11", "abbreviatedOid": "7b20ef4",
          "messageHeadline": "Drop the count", "committedDate": "2026-08-04T08:00:00Z",
          "author": {"name": "Drew White", "user": null},
          "statusCheckRollup": null}}
      ]
    },

    "timelineItems": {"filteredCount": 14, "nodes": [
      {"__typename": "HeadRefForcePushedEvent", "createdAt": "2026-08-04T12:00:00Z",
       "actor": {"login": "drucial"}},
      {"__typename": "LabeledEvent", "createdAt": "2026-08-04T13:00:00Z",
       "actor": {"login": "drucial"}, "label": {"name": "bug"}},
      {"__typename": "UnlabeledEvent", "createdAt": "2026-08-04T13:30:00Z",
       "actor": {"login": "drucial"}, "label": {"name": "wip"}},
      {"__typename": "AssignedEvent", "createdAt": "2026-08-04T14:00:00Z",
       "actor": {"login": "drucial"}, "assignee": {"login": "drucial"}},
      {"__typename": "UnassignedEvent", "createdAt": "2026-08-04T14:30:00Z",
       "actor": {"login": "drucial"}, "assignee": null},
      {"__typename": "ReviewRequestedEvent", "createdAt": "2026-08-04T15:00:00Z",
       "actor": {"login": "drucial"},
       "requestedReviewer": {"login": "copilot-pull-request-reviewer"}},
      {"__typename": "ReviewRequestedEvent", "createdAt": "2026-08-04T15:30:00Z",
       "actor": {"login": "drucial"},
       "requestedReviewer": {"slug": "core-maintainers", "organization": {"login": "acme"}}},
      {"__typename": "ReviewRequestRemovedEvent", "createdAt": "2026-08-04T16:00:00Z",
       "actor": null, "requestedReviewer": {"login": "nkr"}},
      {"__typename": "BaseRefChangedEvent", "createdAt": "2026-08-04T16:30:00Z",
       "actor": {"login": "drucial"},
       "previousRefName": "develop", "currentRefName": "main"},
      {"__typename": "RenamedTitleEvent", "createdAt": "2026-08-04T17:00:00Z", "actor": null}
    ]},

    "statusCheckRollup": {"nodes": [{"commit": {"statusCheckRollup": {
      "state": "FAILURE",
      "contexts": {"nodes": [
        {"__typename": "CheckRun", "name": "test", "status": "COMPLETED", "conclusion": "SUCCESS",
         "checkSuite": {"workflowRun": {"workflow": {"name": "Rails Unit Tests"}}}},
        {"__typename": "CheckRun", "name": "test", "status": "COMPLETED", "conclusion": "NEUTRAL",
         "checkSuite": {"workflowRun": {"workflow": {"name": "Rails Lint"}}}},
        {"__typename": "CheckRun", "name": "build", "status": "COMPLETED", "conclusion": "FAILURE",
         "databaseId": 8700123456, "startedAt": "2026-08-05T09:00:00Z", "completedAt": "2026-08-05T09:05:00Z",
         "detailsUrl": "https://github.com/acme/rocket/runs/8700123456",
         "checkSuite": {"workflowRun": {"databaseId": 555200001, "workflow": {"name": "Build"}}}},
        {"__typename": "CheckRun", "name": "windows", "status": "COMPLETED", "conclusion": "SKIPPED",
         "checkSuite": {"workflowRun": null}},
        {"__typename": "CheckRun", "name": "e2e", "status": "IN_PROGRESS", "conclusion": "",
         "databaseId": 555111002, "startedAt": "2026-08-05T10:00:00Z",
         "checkSuite": {"workflowRun": {"databaseId": 555200002, "workflow": {"name": "E2E Tests"}}}},
        {"__typename": "CheckRun", "name": "e2e", "status": "COMPLETED", "conclusion": "FAILURE",
         "databaseId": 555111001, "startedAt": "2026-08-05T09:00:00Z",
         "checkSuite": {"workflowRun": {"databaseId": 555200002, "workflow": {"name": "E2E Tests"}}}},
        {"__typename": "StatusContext", "context": "codecov", "state": "SUCCESS"},
        {"__typename": "StatusContext", "context": "netlify", "state": "PENDING"},
        {"__typename": "StatusContext", "context": "sonar", "state": "ERROR"}
      ]}
    }}}]}
  }
}`

func fetchDetail(t *testing.T) PullRequestDetail {
	t.Helper()

	res, err := newWithDoer(&fakeDoer{body: detailBody}, nil).PullRequest(context.Background(), "PR_412", "fix-auth")
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
	if len(d.Labels) != 1 || d.Labels[0].Name != "bug" {
		t.Errorf("Labels = %+v, want [bug]", d.Labels)
	}
	if len(d.Assignees) != 1 || d.Assignees[0] != (Actor{ID: "U_1", Login: "drucial"}) {
		t.Errorf("Assignees = %+v, want [drucial]", d.Assignees)
	}

}

func TestReviewersAreWhoHasReviewedAndWhoWasAsked(t *testing.T) {
	want := []Reviewer{
		{Actor: Actor{Login: "nkr"}, State: ReviewStateApproved, Unresolved: 1, Threads: 2, Requested: true},
		{Actor: Actor{Login: "copilot-pull-request-reviewer"}, Requested: true},
		{Actor: Actor{Login: "acme/core-maintainers"}, Requested: true, Team: true},
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

func TestTheTimelineIsOneListInTheOrderThingsHappened(t *testing.T) {
	got := fetchDetail(t).Timeline

	want := []struct {
		kind  TimelineKind
		login string
	}{
		{TimelineCommit, "drucial"},
		{TimelineComment, ""},
		{TimelineReview, "nkr"},
		{TimelineCommit, ""},
		{TimelineComment, "octobot"},
		{TimelineForcePushed, "drucial"},
		{TimelineLabeled, "drucial"},
		{TimelineUnlabeled, "drucial"},
		{TimelineAssigned, "drucial"},
		{TimelineReviewRequested, "drucial"},
		{TimelineReviewRequested, "drucial"},
		{TimelineReviewCancelled, ""},
		{TimelineBaseChanged, "drucial"},
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

func TestEveryMetadataEventNamesWhatItWasDoneTo(t *testing.T) {
	got := fetchDetail(t).Timeline

	want := map[TimelineKind]struct{ subject, was string }{
		TimelineLabeled:         {subject: "bug"},
		TimelineUnlabeled:       {subject: "wip"},
		TimelineAssigned:        {subject: "drucial"},
		TimelineReviewCancelled: {subject: "nkr"},
		TimelineBaseChanged:     {subject: "main", was: "develop"},
	}

	for _, item := range got {
		w, ok := want[item.Kind]
		if !ok {
			continue
		}
		if item.Subject != w.subject || item.Was != w.was {
			t.Errorf("%s names %q (was %q), want %q (was %q)",
				item.Kind, item.Subject, item.Was, w.subject, w.was)
		}
		delete(want, item.Kind)
	}
	for kind := range want {
		t.Errorf("no %s reached the timeline", kind)
	}
}

func TestTheEventsTheWindowCutOffAreCounted(t *testing.T) {
	if got := fetchDetail(t).MoreEvents; got != 4 {
		t.Errorf("MoreEvents = %d, want 4", got)
	}
}

func TestAReviewRequestEventNamesABotAndATeamTheWayTheRailDoes(t *testing.T) {
	var got []string
	for _, item := range fetchDetail(t).Timeline {
		if item.Kind == TimelineReviewRequested {
			got = append(got, item.Subject)
		}
	}

	want := []string{CopilotLogin, "acme/core-maintainers"}
	if !slices.Equal(got, want) {
		t.Errorf("review requests name %v, want %v", got, want)
	}
}

func TestAnEventThatNamesNobodyStaysOut(t *testing.T) {
	for _, item := range fetchDetail(t).Timeline {
		if item.Kind == TimelineUnassigned {
			t.Errorf("the timeline carries an unassign naming %q", item.Subject)
		}
	}
}

func TestAnUnsubmittedReviewStaysOut(t *testing.T) {
	for _, item := range fetchDetail(t).Timeline {
		if item.Said().ID == "REV_3" {
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

	second := threads[1]
	if !second.IsResolved || !second.IsOutdated {
		t.Errorf("second thread = resolved %v outdated %v, want both", second.IsResolved, second.IsOutdated)
	}
	if second.Line != 88 {
		t.Errorf("Line = %d, want 88 from originalLine", second.Line)
	}
}

func TestAThreadCarriesTheSideAndSpanItAnchorsTo(t *testing.T) {
	threads := fetchDetail(t).Threads

	first := threads[0]
	if first.Side != SideRight {
		t.Errorf("Side = %q, want RIGHT", first.Side)
	}
	if first.StartLine != 40 {
		t.Errorf("StartLine = %d, want 40", first.StartLine)
	}

	second := threads[1]
	if second.Side != SideLeft {
		t.Errorf("Side = %q, want LEFT", second.Side)
	}
	if second.StartLine != 86 {
		t.Errorf("StartLine = %d, want 86 from originalStartLine", second.StartLine)
	}
}

func TestAThreadWithNoSideDefaultsToTheRight(t *testing.T) {
	const body = `{"node": {"id": "PR_1", "reviewThreads": {"totalCount": 1, "nodes": [
	  {"path": "a.go", "line": 3, "comments": {"totalCount": 0, "nodes": []}}
	]}}}`

	res, err := newWithDoer(&fakeDoer{body: body}, nil).PullRequest(context.Background(), "PR_1", "topic")
	if err != nil {
		t.Fatalf("PullRequest: %v", err)
	}
	if got := res.Detail.Threads[0].Side; got != SideRight {
		t.Errorf("Side = %q, want RIGHT", got)
	}
}

func TestWhatThePageDidNotReachIsReported(t *testing.T) {
	d := fetchDetail(t)

	if d.MoreComments != 12 {
		t.Errorf("MoreComments = %d, want 12 of 14 past the two returned", d.MoreComments)
	}
	if d.MoreThreads != 3 {
		t.Errorf("MoreThreads = %d, want 3 of 5 past the two returned", d.MoreThreads)
	}
	if d.MoreCommits != 7 {
		t.Errorf("MoreCommits = %d, want 7 of 9 past the two returned", d.MoreCommits)
	}
}

func TestTheCommitsAreAskedForFromTheNewestEnd(t *testing.T) {
	doer := &fakeDoer{body: detailBody}
	if _, err := newWithDoer(doer, nil).PullRequest(context.Background(), "PR_412", "fix-auth"); err != nil {
		t.Fatalf("PullRequest() error = %v, want nil", err)
	}

	if !strings.Contains(doer.gotQuery, "commits(last: 100)") {
		t.Error("the query does not ask for the commits at the newest end of the branch")
	}
	if strings.Contains(doer.gotQuery, "commits(first:") {
		t.Error("the query asks for the oldest commits on the branch")
	}
}

func TestTheCommitsCarryTheirShaHeadlineAndOwnRollup(t *testing.T) {
	commits := fetchDetail(t).Commits

	if len(commits) != 2 {
		t.Fatalf("%d commits, want the two the page returned", len(commits))
	}

	first := commits[0]
	if first.SHA != "a3f91c2d5e" || first.Short != "a3f91c2" {
		t.Errorf("sha = %q/%q, want the full and abbreviated oid", first.SHA, first.Short)
	}
	if first.Headline != "Cap the backoff" {
		t.Errorf("Headline = %q, want the message headline", first.Headline)
	}
	if first.Body != "The retry loop had no ceiling." {
		t.Errorf("Body = %q, want the rest of the message", first.Body)
	}
	if first.Author.Login != "drucial" {
		t.Errorf("Author = %q, want the linked account", first.Author.Login)
	}
	if first.Checks != CheckStateSuccess {
		t.Errorf("Checks = %q, want this commit's own rollup", first.Checks)
	}

	second := commits[1]
	if second.Author.Login != "" || second.AuthorName != "Drew White" {
		t.Errorf("second author = %q/%q, want the git name alone",
			second.Author.Login, second.AuthorName)
	}
	if second.Checks != CheckStateNone {
		t.Errorf("Checks = %q, want none reported", second.Checks)
	}
}

func TestATimelineCommitCarriesItsCommit(t *testing.T) {
	for _, item := range fetchDetail(t).Timeline {
		if item.Kind != TimelineCommit {
			continue
		}
		if item.Commit == nil {
			t.Fatal("a commit entry carries no commit")
		}
		if item.Commit.Short == "" {
			t.Errorf("the commit behind the entry is empty: %+v", item.Commit)
		}
	}
}

func TestTheRollupCountsWhatIsBehindIt(t *testing.T) {
	d := fetchDetail(t)

	if d.Rollup.State != CheckStateFailure {
		t.Errorf("State = %q, want %q", d.Rollup.State, CheckStateFailure)
	}
	counts := [4]int{d.Rollup.Passed, d.Rollup.Failed, d.Rollup.Pending, d.Rollup.Skipped}
	if want := [4]int{3, 3, 2, 1}; counts != want {
		t.Errorf("passed/failed/pending/skipped = %v, want %v", counts, want)
	}

	want := []Check{
		{Name: "test", Workflow: "Rails Unit Tests"},
		{Name: "test", Workflow: "Rails Lint"},
		{Name: "build", Workflow: "Build"},
		{Name: "windows"},
		{Name: "e2e", Workflow: "E2E Tests"},
		{Name: "e2e", Workflow: "E2E Tests"},
		{Name: "codecov"},
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

	states := []CheckState{
		CheckStateSuccess, CheckStateSuccess, CheckStateFailure, CheckStateSkipped,
		CheckStatePending, CheckStateFailure, CheckStateSuccess, CheckStatePending, CheckStateFailure,
	}
	for i, state := range states {
		if got := d.Rollup.Checks[i].State; got != state {
			t.Errorf("check %q = %q, want %q", want[i].Name, got, state)
		}
	}
	if d.Checks != CheckStateFailure {
		t.Errorf("Checks = %q, want the rollup state", d.Checks)
	}

	build := d.Rollup.Checks[2]
	if build.JobID != 8700123456 {
		t.Errorf("build.JobID = %d, want 8700123456", build.JobID)
	}
	if build.RunID != 555200001 {
		t.Errorf("build.RunID = %d, want 555200001", build.RunID)
	}
	for _, check := range d.Rollup.Checks {
		if check.Workflow == "" && check.JobID != 0 {
			t.Errorf("non-Actions check %q has job id %d", check.Name, check.JobID)
		}
	}
	if build.DetailsURL != "https://github.com/acme/rocket/runs/8700123456" {
		t.Errorf("build.DetailsURL = %q, want the run's own link", build.DetailsURL)
	}
	if want := 5 * time.Minute; build.Duration != want {
		t.Errorf("build.Duration = %v, want %v", build.Duration, want)
	}
}

func TestPullRequestReadsWhatTheViewerMayDo(t *testing.T) {
	res, err := newWithDoer(&fakeDoer{body: detailBody}, nil).PullRequest(context.Background(), "PR_412", "fix-auth")
	if err != nil {
		t.Fatalf("PullRequest() error = %v, want nil", err)
	}

	want := ViewerActions{
		CanUpdate: true, CanClose: true, CanReopen: false, CanAssign: true,
		CanMergeAsAdmin: false, CanReact: true,
	}
	if res.Detail.Viewer != want {
		t.Errorf("Viewer = %+v, want %+v", res.Detail.Viewer, want)
	}
}

func TestReactionsComeBackAtEveryLevel(t *testing.T) {
	res, err := newWithDoer(&fakeDoer{body: detailBody}, nil).PullRequest(context.Background(), "PR_412", "fix-auth")
	if err != nil {
		t.Fatalf("PullRequest() error = %v, want nil", err)
	}
	d := res.Detail

	if want := []Reaction{{Content: ReactionHeart, Count: 2, Viewer: true}}; !slices.Equal(d.Reactions, want) {
		t.Errorf("description reactions = %+v, want %+v", d.Reactions, want)
	}

	want := []Reaction{
		{Content: ReactionThumbsUp, Count: 3, Viewer: true},
		{Content: ReactionRocket, Count: 1},
	}
	if got := commentIn(d.Timeline, "IC_2"); !slices.Equal(got.Reactions, want) {
		t.Errorf("IC_2 reactions = %+v, want %+v", got.Reactions, want)
	}

	if got := commentIn(d.Timeline, "REV_1"); len(got.Reactions) != 1 ||
		got.Reactions[0].Content != ReactionConfused || got.Reactions[0].Viewer {
		t.Errorf("REV_1 reactions = %+v, want one CONFUSED the viewer is not in", got.Reactions)
	}

	rc := d.Threads[0].Comments[0]
	if len(rc.Reactions) != 1 || rc.Reactions[0].Count != 4 || !rc.Reactions[0].Viewer {
		t.Errorf("RC_1 reactions = %+v, want four EYES the viewer is in", rc.Reactions)
	}

	if got := commentIn(d.Timeline, "IC_1"); got.Reactions != nil {
		t.Errorf("IC_1 reactions = %+v, want none", got.Reactions)
	}
}

func commentIn(timeline []TimelineItem, id string) Comment {
	for _, item := range timeline {
		if said := item.Said(); said.ID == id {
			return said
		}
	}
	return Comment{}
}

func TestPullRequestReadsTheHeadCommitAndItsBranch(t *testing.T) {
	d := fetchDetail(t)

	if got, want := d.HeadRefOid, "9f1c2b7"; got != want {
		t.Errorf("HeadRefOid = %q, want %q", got, want)
	}
	if got, want := d.HeadRefID, "REF_88"; got != want {
		t.Errorf("HeadRefID = %q, want %q", got, want)
	}
}

func TestPullRequestSurvivesADeletedHeadBranch(t *testing.T) {
	body := strings.Replace(detailBody, `"headRef": {"id": "REF_88"}`, `"headRef": null`, 1)

	res, err := newWithDoer(&fakeDoer{body: body}, nil).PullRequest(context.Background(), "PR_412", "fix-auth")
	if err != nil {
		t.Fatalf("PullRequest() error = %v, want nil", err)
	}
	if got := res.Detail.HeadRefID; got != "" {
		t.Errorf("HeadRefID = %q, want empty on a deleted branch", got)
	}
}

func compareErr(paths ...[]any) *api.GraphQLError {
	items := make([]api.GraphQLErrorItem, len(paths))
	for i, p := range paths {
		items[i] = api.GraphQLErrorItem{Type: "NOT_FOUND", Message: "Could not resolve to a Ref", Path: p}
	}
	return &api.GraphQLError{Errors: items}
}

func TestPullRequestSurvivesARefusedBaseComparison(t *testing.T) {
	body := strings.Replace(detailBody, `"headRef": {"id": "REF_88"}`, `"headRef": null`, 1)
	doer := &fakeDoer{body: body, err: compareErr([]any{"node", "baseRef", "compare"})}

	res, err := newWithDoer(doer, nil).PullRequest(context.Background(), "PR_412", "fix-auth")
	if err != nil {
		t.Fatalf("PullRequest() error = %v, want the detail GitHub sent with it", err)
	}
	if got := res.Detail.ID; got != "PR_412" {
		t.Errorf("ID = %q, want the pull request the response carried", got)
	}
	if got := res.Detail.BehindBy; got != BehindNoHead {
		t.Errorf("BehindBy = %d, want BehindNoHead (%d)", got, BehindNoHead)
	}
}

func TestARefusedComparisonCostsTheCountAndNotTheScreen(t *testing.T) {
	doer := &fakeDoer{body: detailBody, err: compareErr([]any{"node", "baseRef", "compare"})}

	res, err := newWithDoer(doer, nil).PullRequest(context.Background(), "PR_412", "fix-auth")
	if err != nil {
		t.Fatalf("PullRequest() error = %v, want the detail to open anyway", err)
	}
	if got := res.Detail.HeadRefID; got != "REF_88" {
		t.Errorf("HeadRefID = %q, want the branch the response still carried", got)
	}
	if got := res.Detail.BehindBy; got != BehindNoHead {
		t.Errorf("BehindBy = %d, want the count nobody has (%d)", got, BehindNoHead)
	}
}

func TestPullRequestCountsNothingWhereTheComparisonIsNull(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "no comparison", body: strings.Replace(detailBody, `"compare": {"behindBy": 4}`, `"compare": null`, 1)},
		{name: "no base ref", body: strings.Replace(detailBody, `"baseRef": {"compare": {"behindBy": 4}}`, `"baseRef": null`, 1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.body == detailBody {
				t.Fatal("the fixture did not change, so this asserts nothing")
			}

			res, err := newWithDoer(&fakeDoer{body: tt.body}, nil).PullRequest(context.Background(), "PR_412", "fix-auth")
			if err != nil {
				t.Fatalf("PullRequest() error = %v, want nil", err)
			}
			if got := res.Detail.BehindBy; got != BehindNoHead {
				t.Errorf("BehindBy = %d, want BehindNoHead (%d)", got, BehindNoHead)
			}
		})
	}
}

func TestPullRequestStillFailsOnAnyOtherError(t *testing.T) {
	tests := []struct {
		name string
		err  *api.GraphQLError
	}{
		{name: "another path", err: compareErr([]any{"node", "labels"})},
		{
			name: "the comparison and something else",
			err:  compareErr([]any{"node", "baseRef", "compare"}, []any{"node", "timelineItems"}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doer := &fakeDoer{body: detailBody, err: tt.err}

			if _, err := newWithDoer(doer, nil).PullRequest(context.Background(), "PR_412", "fix-auth"); err == nil {
				t.Fatal("PullRequest() error = nil, want the failure reported")
			}
		})
	}
}

func TestPullRequestReadsTheMergeMessages(t *testing.T) {
	d := fetchDetail(t)

	merge := MergeMessage{
		Headline: "Merge pull request #412 from acme/fix-auth",
		Body:     "Fix auth retry",
	}
	if got := d.MergeMessage(MergeMethodMerge); got != merge {
		t.Errorf("MergeMessage(MERGE) = %+v, want %+v", got, merge)
	}

	squash := MergeMessage{Headline: "Fix auth retry (#412)", Body: "* Cap the backoff\n\n* Add a test"}
	if got := d.MergeMessage(MergeMethodSquash); got != squash {
		t.Errorf("MergeMessage(SQUASH) = %+v, want %+v", got, squash)
	}

	if got := d.MergeMessage(MergeMethodRebase); got != (MergeMessage{}) {
		t.Errorf("MergeMessage(REBASE) = %+v, want empty", got)
	}
}

func TestPullRequestReportsWhatTheCallCost(t *testing.T) {
	res, err := newWithDoer(&fakeDoer{body: detailBody}, nil).PullRequest(context.Background(), "PR_412", "fix-auth")
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

	if _, err := newWithDoer(doer, nil).PullRequest(context.Background(), "PR_412", "fix-auth"); err != nil {
		t.Fatalf("PullRequest() error = %v, want nil", err)
	}
	if doer.gotVars["id"] != "PR_412" {
		t.Errorf("id = %v, want the node id unmodified", doer.gotVars["id"])
	}
	if doer.gotVars["head"] != "fix-auth" {
		t.Errorf("head = %v, want the branch it is merging from", doer.gotVars["head"])
	}
}

func TestAnIDBehindNoPullRequestIsAnError(t *testing.T) {
	doer := &fakeDoer{body: `{"node": {}}`}

	_, err := newWithDoer(doer, nil).PullRequest(context.Background(), "I_1", "topic")
	if err == nil {
		t.Fatal("PullRequest() error = nil, want an error")
	}
	if !strings.Contains(err.Error(), "I_1") {
		t.Errorf("error = %q, want it to name the id", err)
	}
}

func TestSameNamedChecksRemainDistinct(t *testing.T) {
	var got []Check
	for _, check := range fetchDetail(t).Rollup.Checks {
		if check.Workflow == "E2E Tests" && check.Name == "e2e" {
			got = append(got, check)
		}
	}
	if len(got) != 2 {
		t.Fatalf("e2e checks = %d, want both: %+v", len(got), got)
	}
	if got[0].Key() == got[1].Key() {
		t.Errorf("same-name checks share key %q", got[0].Key())
	}
	if ids := []int64{got[0].JobID, got[1].JobID}; !slices.Equal(ids, []int64{555111002, 555111001}) {
		t.Errorf("job ids = %v", ids)
	}
}

func TestNoTwoChecksShareAKey(t *testing.T) {
	seen := make(map[string]Check)
	for _, check := range fetchDetail(t).Rollup.Checks {
		if was, dup := seen[check.Key()]; dup {
			t.Errorf("%q under %q has the key of %q under %q",
				check.Name, check.Workflow, was.Name, was.Workflow)
		}
		seen[check.Key()] = check
	}
}

func TestAKeyTellsTheWorkflowFromTheJob(t *testing.T) {
	a := Check{Workflow: "Lint / Format", Name: "go"}
	b := Check{Workflow: "Lint", Name: "Format / go"}
	if a.Key() == b.Key() {
		t.Errorf("%+v and %+v both key to %q", a, b, a.Key())
	}
}

func TestAKeyTellsIdenticallyNamedWorkflowRunsApart(t *testing.T) {
	a := Check{RunID: 41, Workflow: "CI", Name: "test"}
	b := Check{RunID: 42, Workflow: "CI", Name: "test"}
	if a.Key() == b.Key() {
		t.Errorf("runs %d and %d both key to %q", a.RunID, b.RunID, a.Key())
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
		{name: "conflicts win", mergeable: "CONFLICTING", status: "BLOCKED", want: MergeConflicting},
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

// A fake never runs the query, so a missing Bot fragment would drop Copilot with no error anywhere.
func TestTheQueryAsksForEveryShapeOfReviewer(t *testing.T) {
	for _, want := range []string{"... on User { login }", "... on Bot { login }", "... on Team { slug"} {
		if !strings.Contains(pullRequestQuery, want) {
			t.Errorf("the query does not ask for %q", want)
		}
	}
}

func TestTheQueryAsksForWhatAnchorsAThread(t *testing.T) {
	for _, want := range []string{"startLine", "originalStartLine", "diffSide", "diffHunk"} {
		if !strings.Contains(pullRequestQuery, want) {
			t.Errorf("the query does not ask for %q", want)
		}
	}
}

func TestTheQueryAsksWhatTheViewerMayDo(t *testing.T) {
	for _, want := range []string{
		"viewerDidAuthor", "viewerCanUpdate", "viewerCanDelete", "viewerCanReact",
		"viewerCanReply", "viewerCanResolve", "viewerCanUnresolve", "viewerCanAssign",
		"viewerCanMergeAsAdmin", "isCrossRepository",
	} {
		if !strings.Contains(pullRequestQuery, want) {
			t.Errorf("the query does not ask for %q", want)
		}
	}

	if got := strings.Count(pullRequestQuery, "viewerDidAuthor"); got != 3 {
		t.Errorf("viewerDidAuthor appears %d times, want 3: issue comments, reviews and thread comments", got)
	}
}

func TestTheQueryAsksForReactionsAtEveryLevel(t *testing.T) {
	for _, want := range []string{"reactionGroups", "viewerHasReacted", "reactors", "totalCount"} {
		if !strings.Contains(pullRequestQuery, want) {
			t.Errorf("the query does not ask for %q", want)
		}
	}

	if got := strings.Count(pullRequestQuery, "reactionGroups"); got != 4 {
		t.Errorf("reactionGroups appears %d times, want 4: three comment types and the pull request", got)
	}
}

func TestEveryCommentAndThreadComesBackWithItsID(t *testing.T) {
	d := fetchDetail(t)

	var ids []string
	for _, item := range d.Timeline {
		if item.Comment != nil {
			ids = append(ids, item.Said().ID)
		}
	}
	want := []string{"IC_1", "REV_1", "IC_2", "REV_2"}
	if !slices.Equal(ids, want) {
		t.Errorf("timeline comment ids = %v, want %v", ids, want)
	}

	for i, thread := range d.Threads {
		if thread.ID == "" {
			t.Errorf("Threads[%d] has no id of its own", i)
		}
		for j, c := range thread.Comments {
			if c.ID == "" {
				t.Errorf("Threads[%d].Comments[%d] has no id", i, j)
			}
		}
	}
}

func TestEachCommentSaysWhichKindItIs(t *testing.T) {
	d := fetchDetail(t)

	for _, item := range d.Timeline {
		var want CommentKind
		switch item.Kind {
		case TimelineComment:
			want = CommentIssue
		case TimelineReview:
			want = CommentReview
		default:
			if item.Comment != nil {
				t.Errorf("a %s carries a comment", item.Kind)
			}
			continue
		}
		if got := item.Said().Kind; got != want {
			t.Errorf("a %s carries a %q comment, want %q", item.Kind, got, want)
		}
	}

	for _, c := range d.Threads[0].Comments {
		if c.Kind != CommentThread {
			t.Errorf("a thread comment says it is %q, want %q", c.Kind, CommentThread)
		}
	}
}

func TestAuthorshipAndPermissionAreSeparateAnswers(t *testing.T) {
	byID := make(map[string]Comment)
	for _, item := range fetchDetail(t).Timeline {
		if item.Comment != nil {
			byID[item.Said().ID] = item.Said()
		}
	}

	tests := []struct {
		id string
		authored,
		edit, del, react bool
	}{
		{"IC_1", false, false, true, true},
		{"IC_2", false, true, true, true},
		{"REV_1", false, false, true, true},
	}
	for _, tt := range tests {
		c, ok := byID[tt.id]
		if !ok {
			t.Fatalf("%s is not in the timeline", tt.id)
		}
		if c.ViewerDidAuthor != tt.authored || c.CanEdit != tt.edit ||
			c.CanDelete != tt.del || c.CanReact != tt.react {
			t.Errorf("%s: authored=%v edit=%v delete=%v react=%v, want %v %v %v %v",
				tt.id, c.ViewerDidAuthor, c.CanEdit, c.CanDelete, c.CanReact,
				tt.authored, tt.edit, tt.del, tt.react)
		}
	}

	own := fetchDetail(t).Threads[0].Comments[1]
	if !own.ViewerDidAuthor || !own.CanEdit {
		t.Errorf("RC_2 is the viewer's own: authored=%v edit=%v", own.ViewerDidAuthor, own.CanEdit)
	}
}

func TestAThreadCarriesReplyAndBothDirectionsOfResolve(t *testing.T) {
	threads := fetchDetail(t).Threads

	open := threads[0]
	if !open.CanReply || !open.CanResolve || open.CanUnresolve {
		t.Errorf("open thread: reply=%v resolve=%v unresolve=%v, want true true false",
			open.CanReply, open.CanResolve, open.CanUnresolve)
	}

	closed := threads[1]
	if !closed.CanReply || closed.CanResolve || !closed.CanUnresolve {
		t.Errorf("resolved thread: reply=%v resolve=%v unresolve=%v, want true false true",
			closed.CanReply, closed.CanResolve, closed.CanUnresolve)
	}
}

func TestOpenThreadsCountAgainstTheReviewerWhoOpenedThem(t *testing.T) {
	for _, r := range fetchDetail(t).Reviewers {
		switch r.Actor.Login {
		case "nkr":
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

func TestATeamIsNamedByItsSlugNotItsDisplayName(t *testing.T) {
	tests := []struct {
		name string
		org  string
		slug string
		want string
	}{
		{name: "under its organization", org: "acme", slug: "core-maintainers", want: "acme/core-maintainers"},
		{name: "with no organization", org: "", slug: "core-maintainers", want: "core-maintainers"},
		{name: "with nothing at all", org: "acme", slug: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := teamHandle(tt.org, tt.slug); got != tt.want {
				t.Errorf("teamHandle(%q, %q) = %q, want %q", tt.org, tt.slug, got, tt.want)
			}
		})
	}
}

func TestAThreadCarriesTheDiffItWasWrittenAgainst(t *testing.T) {
	threads := fetchDetail(t).Threads

	hunk := threads[0].Hunk
	if hunk == nil {
		t.Fatal("the thread came back with no hunk")
		return
	}
	if hunk.Header != "@@ -39,3 +39,4 @@" {
		t.Errorf("header = %q", hunk.Header)
	}
	if len(hunk.Lines) != 3 {
		t.Fatalf("lines = %d, want 3", len(hunk.Lines))
	}
	last := hunk.Lines[2]
	if last.Kind != DiffAdded || last.New != 40 {
		t.Errorf("last line = %+v, want the added line at 40", last)
	}

	if threads[1].Hunk != nil {
		t.Errorf("Hunk = %+v, want nil where GitHub sent none", threads[1].Hunk)
	}
}

func TestAReviewersThreadsAreCountedResolvedOrNot(t *testing.T) {
	var nkr Reviewer
	for _, r := range fetchDetail(t).Reviewers {
		if r.Actor.Login == "nkr" {
			nkr = r
		}
	}

	if nkr.Threads != 2 {
		t.Errorf("Threads = %d, want both of them counted", nkr.Threads)
	}
	if nkr.Unresolved != 1 {
		t.Errorf("Unresolved = %d, want the open one alone", nkr.Unresolved)
	}
}

func TestThePendingReviewIsTheViewersDraft(t *testing.T) {
	d := fetchDetail(t)

	want := Review{ID: "REV_3", State: ReviewStatePending, Body: "not sent yet"}
	if d.DraftReview != want {
		t.Errorf("DraftReview = %+v, want %+v", d.DraftReview, want)
	}
}

func TestADraftIsOnlyTheViewersOwnUnsubmittedReview(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "somebody else's",
			body: `{"node": {"id": "PR_1", "reviews": {"totalCount": 1, "nodes": [
			  {"id": "REV_9", "state": "PENDING", "body": "theirs", "viewerDidAuthor": false}
			]}}}`,
		},
		{
			name: "the viewer's, already submitted",
			body: `{"node": {"id": "PR_1", "reviews": {"totalCount": 1, "nodes": [
			  {"id": "REV_9", "state": "APPROVED", "body": "mine", "viewerDidAuthor": true}
			]}}}`,
		},
		{
			name: "none at all",
			body: `{"node": {"id": "PR_1", "reviews": {"totalCount": 0, "nodes": []}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := newWithDoer(&fakeDoer{body: tt.body}, nil).PullRequest(context.Background(), "PR_1", "topic")
			if err != nil {
				t.Fatalf("PullRequest: %v", err)
			}
			if got := res.Detail.DraftReview; got != (Review{}) {
				t.Errorf("DraftReview = %+v, want none", got)
			}
		})
	}
}

func TestADraftReviewIsKeptOutOfTheReviewersPanel(t *testing.T) {
	d := fetchDetail(t)

	for _, r := range d.Reviewers {
		if r.State == ReviewStatePending {
			t.Errorf("%s is in the panel with an unsubmitted review", r.Actor.Login)
		}
	}
}

func TestAnUnsentThreadAndItsCommentsAreMarkedDraft(t *testing.T) {
	const body = `{"node": {"id": "PR_1", "reviewThreads": {"totalCount": 1, "nodes": [
	  {"id": "RT_9", "path": "a.go", "line": 3, "diffSide": "RIGHT", "subjectType": "LINE",
	   "comments": {"totalCount": 1, "nodes": [
	     {"id": "RC_9", "state": "PENDING", "body": "unsent", "pullRequestReview": {"id": "REV_9"}}
	   ]}}
	]}}}`

	res, err := newWithDoer(&fakeDoer{body: body}, nil).PullRequest(context.Background(), "PR_1", "topic")
	if err != nil {
		t.Fatalf("PullRequest: %v", err)
	}

	thread := res.Detail.Threads[0]
	if !thread.Draft {
		t.Error("a thread held in an unsubmitted review did not come back as a draft")
	}
	if !thread.Comments[0].Draft {
		t.Error("an unsent comment did not come back as a draft")
	}
}

func TestASubmittedThreadIsNoDraft(t *testing.T) {
	d := fetchDetail(t)

	for _, thread := range d.Threads {
		if thread.Draft {
			t.Errorf("thread %s came back as a draft", thread.ID)
		}
		for _, c := range thread.Comments {
			if c.Draft {
				t.Errorf("comment %s came back as a draft", c.ID)
			}
		}
	}
}

func TestAnUnsentReplyDoesNotMakeItsThreadADraft(t *testing.T) {
	const body = `{"node": {"id": "PR_1", "reviewThreads": {"totalCount": 1, "nodes": [
	  {"id": "RT_9", "path": "a.go", "line": 3,
	   "comments": {"totalCount": 2, "nodes": [
	     {"id": "RC_9", "state": "SUBMITTED", "body": "sent"},
	     {"id": "RC_10", "state": "PENDING", "body": "unsent"}
	   ]}}
	]}}}`

	res, err := newWithDoer(&fakeDoer{body: body}, nil).PullRequest(context.Background(), "PR_1", "topic")
	if err != nil {
		t.Fatalf("PullRequest: %v", err)
	}

	thread := res.Detail.Threads[0]
	if thread.Draft {
		t.Error("a published thread came back as a draft because a reply to it is unsent")
	}
	if thread.Comments[0].Draft {
		t.Error("a published comment came back as a draft")
	}
	if !thread.Comments[1].Draft {
		t.Error("the unsent reply did not come back as a draft")
	}
}

func TestAFileThreadIsNotAThreadOnLineOne(t *testing.T) {
	const body = `{"node": {"id": "PR_1", "reviewThreads": {"totalCount": 2, "nodes": [
	  {"id": "RT_F", "path": "a.go", "line": 1, "subjectType": "FILE",
	   "comments": {"totalCount": 0, "nodes": []}},
	  {"id": "RT_L", "path": "a.go", "line": 1,
	   "comments": {"totalCount": 0, "nodes": []}}
	]}}}`

	res, err := newWithDoer(&fakeDoer{body: body}, nil).PullRequest(context.Background(), "PR_1", "topic")
	if err != nil {
		t.Fatalf("PullRequest: %v", err)
	}

	if got := res.Detail.Threads[0].Subject; got != SubjectFile {
		t.Errorf("Subject = %q, want FILE", got)
	}
	if got := res.Detail.Threads[1].Subject; got != SubjectLine {
		t.Errorf("Subject = %q, want a thread with no subjectType read as LINE", got)
	}
}

func TestTheDetailAsksForWhatMarksAThreadUnsent(t *testing.T) {
	doer := &fakeDoer{body: detailBody}
	if _, err := newWithDoer(doer, nil).PullRequest(context.Background(), "PR_412", "fix-auth"); err != nil {
		t.Fatalf("PullRequest: %v", err)
	}

	threads := doer.gotQuery
	if at := strings.Index(threads, "reviewThreads("); at >= 0 {
		threads = threads[at:]
	}
	if at := strings.Index(threads, "commits("); at >= 0 {
		threads = threads[:at]
	}

	for _, want := range []string{"subjectType", "state"} {
		if !strings.Contains(threads, want) {
			t.Errorf("the review thread selection does not ask for %q", want)
		}
	}
}
