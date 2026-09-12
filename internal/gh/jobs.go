package gh

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	maxJobLogBytes         = 8 << 20
	maxJobLogTransferBytes = 32 << 20
)

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

// JobLogs fetches a job's raw log, keeping the last 8 MiB and stopping after 32 MiB, with a marker line at each cut.
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

	body, truncated, stopped, err := readJobLogDownload(
		resp.Body, maxJobLogBytes, maxJobLogTransferBytes,
	)
	if err != nil {
		return nil, fmt.Errorf("fetching job logs (%s job %d): %w", repo, jobID, err)
	}
	if truncated {
		body = append([]byte("[zen-octo: earlier log output truncated]\n"), body...)
	}
	if stopped {
		body = append(body, []byte("\n[zen-octo: log download stopped after 32 MiB; later output unavailable]\n")...)
	}
	return body, nil
}

func readJobLogDownload(r io.Reader, keep, transfer int) ([]byte, bool, bool, error) {
	limited := &io.LimitedReader{R: r, N: int64(transfer)}
	body, truncated, err := readJobLog(limited, keep)
	if err != nil || limited.N > 0 {
		return body, truncated, false, err
	}
	var extra [1]byte
	n, probeErr := io.ReadFull(r, extra[:])
	if probeErr != nil && probeErr != io.EOF {
		return nil, false, false, probeErr
	}
	return body, truncated, n > 0, nil
}

func readJobLog(r io.Reader, limit int) ([]byte, bool, error) {
	if limit <= 0 {
		return nil, false, nil
	}
	writer := tailWriter{limit: limit}
	if _, err := io.Copy(&writer, r); err != nil {
		return nil, false, err
	}
	window := writer.bytes()
	truncated := writer.total > int64(limit)
	body := window
	if truncated {
		partial := window[0] != '\n'
		body = window[1:]
		if partial {
			if newline := bytes.IndexByte(body, '\n'); newline >= 0 {
				body = body[newline+1:]
			}
		}
	}
	return bytes.Clone(body), truncated, nil
}

type tailWriter struct {
	buf   []byte
	start int
	count int
	limit int
	total int64
}

func (w *tailWriter) Write(p []byte) (int, error) {
	n := len(p)
	w.total += int64(n)
	capacity := w.limit + 1
	if w.buf == nil {
		w.buf = make([]byte, capacity)
	}
	if len(p) >= capacity {
		copy(w.buf, p[len(p)-capacity:])
		w.start, w.count = 0, capacity
		return n, nil
	}

	if w.count < capacity {
		fill := min(len(p), capacity-w.count)
		w.copyAt((w.start+w.count)%capacity, p[:fill])
		w.count += fill
		p = p[fill:]
	}
	if len(p) > 0 {
		w.copyAt(w.start, p)
		w.start = (w.start + len(p)) % capacity
	}
	return n, nil
}

func (w *tailWriter) copyAt(at int, p []byte) {
	n := copy(w.buf[at:], p)
	copy(w.buf, p[n:])
}

func (w *tailWriter) bytes() []byte {
	if w.count == 0 {
		return nil
	}
	out := make([]byte, w.count)
	n := copy(out, w.buf[w.start:min(len(w.buf), w.start+w.count)])
	copy(out[n:], w.buf[:w.count-n])
	return out
}

// RerunJob re-runs a job and the jobs depending on it, returning when GitHub accepted it, or the zero time.
func (c *Client) RerunJob(ctx context.Context, repo string, jobID int64) (time.Time, error) {
	if !strings.Contains(repo, "/") {
		return time.Time{}, fmt.Errorf("rerunning job (%s job %d): %q is not owner/name", repo, jobID, repo)
	}

	path := fmt.Sprintf("repos/%s/actions/jobs/%d/rerun", repo, jobID)
	resp, err := c.rest.RequestWithContext(ctx, http.MethodPost, path, nil)
	if err != nil {
		return time.Time{}, fmt.Errorf("rerunning job (%s job %d): %w", repo, jobID, classify(err))
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	acceptedAt, _ := http.ParseTime(resp.Header.Get("Date"))
	return acceptedAt, nil
}

func (c *Client) RerunFailedJobs(ctx context.Context, repo string, runID int64) error {
	return c.postRerun(ctx, repo, runID, "rerun-failed-jobs", "rerunning failed jobs")
}

func (c *Client) RerunAllJobs(ctx context.Context, repo string, runID int64) error {
	return c.postRerun(ctx, repo, runID, "rerun", "rerunning all jobs")
}

// Undecoded because the 201 body is undocumented.
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
