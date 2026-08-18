package gh

import (
	"cmp"
	"time"
)

// rollupSelection is the head commit's checks, as both the detail query and the
// pulse ask for them. One document changing the shape has to change the other.
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

// rollupNode decodes rollupSelection. The inner pointer is null on a commit
// nothing has run against, which is not the same as one whose checks all passed.
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

// rollup counts the head commit's checks. GitHub gives the summary state; the
// breakdown behind it is what makes the rail worth reading.
func rollup(r rollupNode) CheckRollup {
	if len(r.Nodes) == 0 || r.Nodes[0].Commit.StatusCheckRollup == nil {
		return CheckRollup{}
	}

	src := r.Nodes[0].Commit.StatusCheckRollup
	out := CheckRollup{State: CheckState(src.State)}

	// A re-run leaves the previous attempt in the connection, so the same job
	// arrives twice with two different answers. Only the latest one is true.
	at := make(map[string]int, len(src.Contexts.Nodes))

	for _, c := range src.Contexts.Nodes {
		check := Check{
			Name:        cmp.Or(c.Name, c.Context),
			State:       checkState(c.Typename, c.Status, c.Conclusion, c.State),
			JobID:       c.DatabaseID,
			StartedAt:   c.StartedAt,
			CompletedAt: c.CompletedAt,
			DetailsURL:  c.DetailsURL,
		}
		// A job is named for what it does, so half a repository's checks are
		// called "test". The workflow it ran under is what tells them apart.
		if run := c.CheckSuite.WorkflowRun; run != nil {
			check.Workflow = run.Workflow.Name
			check.RunID = run.DatabaseID
		}
		if !check.CompletedAt.IsZero() {
			check.Duration = check.CompletedAt.Sub(check.StartedAt)
		}

		key := check.Key()
		i, seen := at[key]
		switch {
		case !seen:
			at[key] = len(out.Checks)
			out.Checks = append(out.Checks, check)
		case c.StartedAt.After(out.Checks[i].StartedAt):
			out.Checks[i] = check
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

// checkState folds a check run and a status context into the one vocabulary. A
// check run has a status and a conclusion, a status context only a state.
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
