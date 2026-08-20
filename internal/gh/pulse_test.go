package gh

import (
	"context"
	"strings"
	"testing"
	"time"
)

// pulseBody is one recheck: open, review asked for, blocked on a rule rather
// than a conflict, and a rollup carrying a re-run so the dedupe is exercised.
const pulseBody = `{
  "rateLimit": {"limit": 5000, "cost": 2, "remaining": 4402, "resetAt": "2026-08-15T18:00:00Z"},
  "node": {
    "id": "PR_412",
    "state": "OPEN",
    "isDraft": false,
    "reviewDecision": "REVIEW_REQUIRED",
    "mergeable": "MERGEABLE",
    "mergeStateStatus": "BLOCKED",
    "updatedAt": "2026-08-15T11:00:00Z",
    "headRefOid": "3ac91fe",
    "statusCheckRollup": {
      "nodes": [{"commit": {"statusCheckRollup": {
        "state": "PENDING",
        "contexts": {"nodes": [
          {"__typename": "CheckRun", "name": "test", "status": "COMPLETED",
           "conclusion": "FAILURE", "databaseId": 900001, "startedAt": "2026-08-15T10:00:00Z",
           "checkSuite": {"workflowRun": {"databaseId": 900100, "workflow": {"name": "CI"}}}},
          {"__typename": "CheckRun", "name": "test", "status": "IN_PROGRESS",
           "databaseId": 900002, "startedAt": "2026-08-15T10:30:00Z",
           "checkSuite": {"workflowRun": {"databaseId": 900100, "workflow": {"name": "CI"}}}},
          {"__typename": "StatusContext", "context": "vercel", "state": "SUCCESS"}
        ]}
      }}}]
    }
  }
}`

func TestAPulseReadsEveryFieldItAsksFor(t *testing.T) {
	doer := &fakeDoer{body: pulseBody}

	res, err := newWithDoer(doer, nil).Pulse(context.Background(), "PR_412")
	if err != nil {
		t.Fatalf("Pulse: %v", err)
	}

	p := res.Pulse
	if p.State != PRStateOpen || p.IsDraft {
		t.Errorf("lifecycle = %q draft=%v, want an open pull request", p.State, p.IsDraft)
	}
	if p.ReviewDecision != ReviewDecisionReviewRequired {
		t.Errorf("review decision = %q, want REVIEW_REQUIRED", p.ReviewDecision)
	}
	// BLOCKED and not CONFLICTING: mergeable is the field that knows about
	// conflicts, and a rule standing in the way is the other one's news.
	if p.Merge != MergeBlocked {
		t.Errorf("merge state = %q, want BLOCKED", p.Merge)
	}
	if p.HeadRefOid != "3ac91fe" {
		t.Errorf("head = %q, want the tip the checks ran against", p.HeadRefOid)
	}
	if want := time.Date(2026, 8, 15, 11, 0, 0, 0, time.UTC); !p.UpdatedAt.Equal(want) {
		t.Errorf("updated = %v, want %v", p.UpdatedAt, want)
	}
}

// The rollup is parsed by the same code the detail query's is, which is the
// whole reason the selection and its type are shared.
func TestAPulseCountsTheChecksTheSameWay(t *testing.T) {
	doer := &fakeDoer{body: pulseBody}

	res, err := newWithDoer(doer, nil).Pulse(context.Background(), "PR_412")
	if err != nil {
		t.Fatalf("Pulse: %v", err)
	}

	r := res.Pulse.Rollup
	if r.State != CheckStatePending {
		t.Errorf("rollup state = %q, want PENDING", r.State)
	}
	if len(r.Checks) != 3 {
		t.Fatalf("checks = %v, want every ambiguous same-name check preserved", r.Checks)
	}
	if r.Pending != 1 || r.Passed != 1 || r.Failed != 1 {
		t.Errorf("tally = %d pending %d passed %d failed", r.Pending, r.Passed, r.Failed)
	}

	// Reached through the same rollup() the detail query uses, so the id fields
	// only need proving they arrive here too, not that they decode correctly.
	var keys = make(map[string]bool)
	for _, c := range r.Checks {
		if c.Workflow != "CI" {
			continue
		}
		if c.JobID != 900001 && c.JobID != 900002 {
			t.Errorf("test.JobID = %d", c.JobID)
		}
		if c.RunID != 900100 {
			t.Errorf("test.RunID = %d, want 900100", c.RunID)
		}
		if keys[c.Key()] {
			t.Errorf("duplicate key %q", c.Key())
		}
		keys[c.Key()] = true
	}
}

func TestAPulseFoldsTheBudget(t *testing.T) {
	doer := &fakeDoer{body: pulseBody}

	res, err := newWithDoer(doer, nil).Pulse(context.Background(), "PR_412")
	if err != nil {
		t.Fatalf("Pulse: %v", err)
	}

	if res.RateLimit.Remaining != 4402 || res.RateLimit.Cost != 2 {
		t.Errorf("budget = %+v, want what the response reported", res.RateLimit)
	}
}

// A merged pull request has no head branch, and that is what makes the detail
// query need deletedHeadRef. This one asks for no comparison, so it cannot.
func TestThePulseAsksForNoBranchComparison(t *testing.T) {
	doer := &fakeDoer{body: pulseBody}

	if _, err := newWithDoer(doer, nil).Pulse(context.Background(), "PR_412"); err != nil {
		t.Fatalf("Pulse: %v", err)
	}

	if strings.Contains(doer.gotQuery, "compare") {
		t.Error("the pulse asks for a base comparison, which a merged pull request cannot answer")
	}
	if !strings.Contains(doer.gotQuery, "headRefOid") {
		t.Error("the pulse does not ask for headRefOid, which is what says the diff is stale")
	}
	if got := doer.gotVars; len(got) != 1 || got["id"] != "PR_412" {
		t.Errorf("variables = %v, want the id alone", got)
	}
}

// The inline fragment yields an empty object rather than an error when the id
// belongs to something else, so a bare zero value would read as a real answer.
func TestAPulseOnSomethingThatIsNotAPullRequestFails(t *testing.T) {
	doer := &fakeDoer{body: `{"node": {}}`}

	_, err := newWithDoer(doer, nil).Pulse(context.Background(), "I_1")
	if err == nil {
		t.Fatal("a node that is not a pull request answered without an error")
	}
	if !strings.Contains(err.Error(), "no pull request behind that id") {
		t.Errorf("error = %q, want it to name what was wrong", err)
	}
}
