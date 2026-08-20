package store

import (
	"strconv"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

func jobKey(id int64) string { return strconv.FormatInt(id, 10) }

// Job returns one Actions job and its log. The zero value is one never asked
// for, which the selected-job pane reads as idle.
func (s Store) Job(id int64) Job {
	if id == 0 {
		return Job{}
	}
	return s.jobs.get(jobKey(id))
}

// BeginJob marks one Actions job in flight. Logs are keyed by concrete job id:
// a rerun keeps the logical check key but has a different log.
func (s *Store) BeginJob(id int64) bool {
	if id == 0 {
		return false
	}
	key := jobKey(id)
	held := s.jobs.get(key)
	if held.Status == StatusLoading {
		return false
	}
	held.Status = StatusLoading
	s.jobs.put(key, held)
	s.jobs.evict(key, func(key string) bool {
		return s.jobs.get(key).Status == StatusLoading
	})
	return true
}

// JobApplied stores the metadata and immutable log text together, since the
// right pane needs both before it can divide lines among steps.
func (s *Store) JobApplied(id int64, job gh.Job, log []byte) {
	if id == 0 {
		return
	}
	key := jobKey(id)
	s.jobs.put(key, Job{
		Job:    job,
		Log:    string(log),
		Status: StatusReady,
		Loaded: true,
	})
	s.jobs.evict(key, func(key string) bool {
		return s.jobs.get(key).Status == StatusLoading
	})
}

// JobLogFailed keeps metadata that landed even when the separate log download
// failed. A completed job can still show its steps and timings after retention
// has removed the blob.
func (s *Store) JobLogFailed(id int64, job gh.Job, err error) {
	if id == 0 {
		return
	}
	key := jobKey(id)
	s.jobs.put(key, Job{Job: job, Status: StatusFailed, Err: err, Loaded: true})
	s.jobs.evict(key, func(key string) bool {
		return s.jobs.get(key).Status == StatusLoading
	})
}

// JobFailed puts a job into its error state while preserving a log that had
// already loaded. A failed refetch must not empty the pane.
func (s *Store) JobFailed(id int64, err error) {
	if id == 0 {
		return
	}
	key := jobKey(id)
	held, ok := s.jobs.look(key)
	if !ok {
		return
	}
	held.Status = StatusFailed
	held.Err = err
	s.jobs.put(key, held)
	s.jobs.evict(key, func(key string) bool {
		return s.jobs.get(key).Status == StatusLoading
	})
}

// UseJob restamps a selected job read without a request.
func (s *Store) UseJob(id int64) {
	if id != 0 {
		s.jobs.touch(jobKey(id))
	}
}
