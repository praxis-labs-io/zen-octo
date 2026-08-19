package gh

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Job fetches the step metadata beside a log. The downloadable text names
// groups, but only this endpoint says which steps failed, which were skipped,
// and how long each one ran.
func (c *Client) Job(ctx context.Context, repo string, jobID int64) (Job, error) {
	if !strings.Contains(repo, "/") {
		return Job{}, fmt.Errorf("fetching job (%s job %d): %q is not owner/name", repo, jobID, repo)
	}

	var resp struct {
		ID          int64
		Name        string
		Status      string
		Conclusion  string
		StartedAt   time.Time `json:"started_at"`
		CompletedAt time.Time `json:"completed_at"`
		Steps       []struct {
			Number      int
			Name        string
			Status      string
			Conclusion  string
			StartedAt   time.Time `json:"started_at"`
			CompletedAt time.Time `json:"completed_at"`
		}
	}
	path := fmt.Sprintf("repos/%s/actions/jobs/%d", repo, jobID)
	if err := c.rest.DoWithContext(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return Job{}, fmt.Errorf("fetching job (%s job %d): %w", repo, jobID, classify(err))
	}

	job := Job{
		ID:          resp.ID,
		Name:        resp.Name,
		State:       checkState("CheckRun", strings.ToUpper(resp.Status), strings.ToUpper(resp.Conclusion), ""),
		StartedAt:   resp.StartedAt,
		CompletedAt: resp.CompletedAt,
	}
	if !job.StartedAt.IsZero() && !job.CompletedAt.IsZero() {
		job.Duration = job.CompletedAt.Sub(job.StartedAt)
	}
	job.Steps = make([]JobStep, 0, len(resp.Steps))
	for _, s := range resp.Steps {
		step := JobStep{
			Number:      s.Number,
			Name:        s.Name,
			State:       checkState("CheckRun", strings.ToUpper(s.Status), strings.ToUpper(s.Conclusion), ""),
			StartedAt:   s.StartedAt,
			CompletedAt: s.CompletedAt,
		}
		if !step.StartedAt.IsZero() && !step.CompletedAt.IsZero() {
			step.Duration = step.CompletedAt.Sub(step.StartedAt)
		}
		job.Steps = append(job.Steps, step)
	}
	return job, nil
}

// JobLogs fetches one job's raw log text via the undecoded request path: the
// endpoint redirects to a blob-storage URL, not a JSON body.
func (c *Client) JobLogs(ctx context.Context, repo string, jobID int64) ([]byte, error) {
	if !strings.Contains(repo, "/") {
		return nil, fmt.Errorf("fetching job logs (%s job %d): %q is not owner/name", repo, jobID, repo)
	}

	path := fmt.Sprintf("repos/%s/actions/jobs/%d/logs", repo, jobID)
	resp, err := c.rest.RequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching job logs (%s job %d): %w", repo, jobID, classify(err))
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fetching job logs (%s job %d): %w", repo, jobID, err)
	}
	return body, nil
}

// RerunFailedJobs re-runs only the failed jobs of a workflow run.
func (c *Client) RerunFailedJobs(ctx context.Context, repo string, runID int64) error {
	return c.postRerun(ctx, repo, runID, "rerun-failed-jobs", "rerunning failed jobs")
}

// RerunAllJobs re-runs every job of a workflow run.
func (c *Client) RerunAllJobs(ctx context.Context, repo string, runID int64) error {
	return c.postRerun(ctx, repo, runID, "rerun", "rerunning all jobs")
}

// postRerun is what RerunFailedJobs and RerunAllJobs share: same shape, same
// undocumented 201 body DoWithContext cannot safely decode, different path.
func (c *Client) postRerun(ctx context.Context, repo string, runID int64, action, verb string) error {
	if !strings.Contains(repo, "/") {
		return fmt.Errorf("%s (%s run %d): %q is not owner/name", verb, repo, runID, repo)
	}

	path := fmt.Sprintf("repos/%s/actions/runs/%d/%s", repo, runID, action)
	resp, err := c.rest.RequestWithContext(ctx, http.MethodPost, path, nil)
	if err != nil {
		return fmt.Errorf("%s (%s run %d): %w", verb, repo, runID, classify(err))
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}
