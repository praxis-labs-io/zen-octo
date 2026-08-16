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
                    name
                    status
                    conclusion
                    startedAt
                    checkSuite { workflowRun { workflow { name } } }
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
						Typename   string `json:"__typename"`
						Name       string
						Context    string
						Status     string
						Conclusion string
						State      string
						StartedAt  time.Time

						CheckSuite struct {
							WorkflowRun *struct {
								Workflow struct{ Name string }
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
	started := make([]time.Time, 0, len(src.Contexts.Nodes))

	for _, c := range src.Contexts.Nodes {
		check := Check{
			Name:  cmp.Or(c.Name, c.Context),
			State: checkState(c.Typename, c.Status, c.Conclusion, c.State),
		}
		// A job is named for what it does, so half a repository's checks are
		// called "test". The workflow it ran under is what tells them apart.
		if run := c.CheckSuite.WorkflowRun; run != nil {
			check.Workflow = run.Workflow.Name
		}

		key := check.Key()
		i, seen := at[key]
		switch {
		case !seen:
			at[key] = len(out.Checks)
			out.Checks = append(out.Checks, check)
			started = append(started, c.StartedAt)
		case c.StartedAt.After(started[i]):
			out.Checks[i], started[i] = check, c.StartedAt
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
