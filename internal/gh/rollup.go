package gh

import (
	"cmp"
	"time"
)

// Shared by the detail and pulse documents so their shapes cannot drift apart.
const rollupSelection = `
      statusCheckRollup: commits(last: 1) {
        nodes {
          commit {
            statusCheckRollup {
              state
              contexts(first: 100) {
                nodes {
                  __typename
                  ... on CheckRun {
                    databaseId
                    name
                    status
                    conclusion
                    startedAt
                    completedAt
                    detailsUrl
                    checkSuite { workflowRun { databaseId workflow { name } } }
                  }
                  ... on StatusContext { context state }
                }
              }
            }
          }
        }
      }`

// StatusCheckRollup is null on a commit nothing has run against, which is not all checks passing.
type rollupNode struct {
	Nodes []struct {
		Commit struct {
			StatusCheckRollup *struct {
				State    string
				Contexts struct {
					Nodes []struct {
						Typename    string `json:"__typename"`
						Name        string
						Context     string
						Status      string
						Conclusion  string
						State       string
						DatabaseID  int64
						StartedAt   time.Time
						CompletedAt time.Time
						DetailsURL  string

						CheckSuite struct {
							WorkflowRun *struct {
								DatabaseID int64
								Workflow   struct{ Name string }
							}
						}
					}
				}
			}
		}
	}
}

// Keeps every context GitHub returns: two jobs may share a name, and CheckRun exposes nothing that
// proves a rerun.
func rollup(r rollupNode) CheckRollup {
	if len(r.Nodes) == 0 || r.Nodes[0].Commit.StatusCheckRollup == nil {
		return CheckRollup{}
	}

	src := r.Nodes[0].Commit.StatusCheckRollup
	out := CheckRollup{State: CheckState(src.State)}

	ids := make([]int64, 0, len(src.Contexts.Nodes))
	for _, c := range src.Contexts.Nodes {
		check := Check{
			Name:        cmp.Or(c.Name, c.Context),
			State:       checkState(c.Typename, c.Status, c.Conclusion, c.State),
			StartedAt:   c.StartedAt,
			CompletedAt: c.CompletedAt,
			DetailsURL:  c.DetailsURL,
		}
		if run := c.CheckSuite.WorkflowRun; run != nil {
			check.Workflow = run.Workflow.Name
			check.RunID = run.DatabaseID
			check.JobID = c.DatabaseID
		}
		if !check.StartedAt.IsZero() && !check.CompletedAt.IsZero() {
			check.Duration = check.CompletedAt.Sub(check.StartedAt)
		}

		out.Checks = append(out.Checks, check)
		ids = append(ids, c.DatabaseID)
	}

	counts := make(map[string]int, len(out.Checks))
	for _, check := range out.Checks {
		counts[check.LogicalKey()]++
	}
	for i := range out.Checks {
		if counts[out.Checks[i].LogicalKey()] < 2 {
			continue
		}
		out.Checks[i].DistinctID = ids[i]
		if out.Checks[i].DistinctID == 0 {
			out.Checks[i].DistinctID = -int64(i + 1)
		}
	}

	for _, check := range out.Checks {
		switch check.State {
		case CheckStateSuccess:
			out.Passed++
		case CheckStatePending, CheckStateExpected:
			out.Pending++
		case CheckStateSkipped:
			out.Skipped++
		default:
			out.Failed++
		}
	}
	return out
}

func checkState(typename, status, conclusion, state string) CheckState {
	if typename != "CheckRun" {
		switch state {
		case "SUCCESS":
			return CheckStateSuccess
		case "PENDING":
			return CheckStatePending
		case "EXPECTED":
			return CheckStateExpected
		}
		return CheckStateFailure
	}

	if status != "COMPLETED" {
		return CheckStatePending
	}
	switch conclusion {
	case "SUCCESS", "NEUTRAL":
		return CheckStateSuccess
	case "SKIPPED":
		return CheckStateSkipped
	}
	return CheckStateFailure
}
