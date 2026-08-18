package gh

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

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
