package store

import (
	"strconv"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

// Keyed by concrete job id: a rerun keeps the check but has a different log.
func jobKey(id int64) string { return strconv.FormatInt(id, 10) }

// Job is the Actions job held for id, or the zero value where none was asked for.
func (s Store) Job(id int64) Job {
	if id == 0 {
		return Job{}
	}
	return s.jobs.get(jobKey(id))
}

// BeginJob marks a job in flight and reports whether it started.
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

// JobApplied stores a job's metadata and log.
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

// JobLogFailed stores a job's metadata with the error from its log download.
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

// JobFailed puts a held job into its error state, keeping its log.
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

// UseJob marks a job read without a request as recently used.
func (s *Store) UseJob(id int64) {
	if id != 0 {
		s.jobs.touch(jobKey(id))
	}
}
