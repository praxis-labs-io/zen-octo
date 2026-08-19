package store_test

import (
	"errors"
	"testing"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
)

func TestAJobMovesFromLoadingToReady(t *testing.T) {
	s := store.New(nil)
	if !s.BeginJob(9001) {
		t.Fatal("the first job request was refused")
	}
	if got := s.Job(9001); got.Status != store.StatusLoading || got.Loaded {
		t.Fatalf("begun job = %+v, want loading and not loaded", got)
	}
	if s.BeginJob(9001) {
		t.Error("a duplicate job request started")
	}

	job := gh.Job{ID: 9001, Name: "test", Steps: []gh.JobStep{{Number: 1, Name: "Run tests"}}}
	s.JobApplied(9001, job, []byte("the log\n"))
	got := s.Job(9001)
	if got.Status != store.StatusReady || !got.Loaded || got.Job.Name != "test" || got.Log != "the log\n" {
		t.Errorf("landed job = %+v, want the fetched job and log", got)
	}
}

func TestAJobFailureKeepsTheLogThatWasAlreadyThere(t *testing.T) {
	s := store.New(nil)
	s.BeginJob(9001)
	s.JobApplied(9001, gh.Job{ID: 9001, Name: "test"}, []byte("held\n"))
	s.BeginJob(9001)

	want := errors.New("connection reset")
	s.JobFailed(9001, want)
	got := s.Job(9001)
	if got.Status != store.StatusFailed || !got.Loaded || got.Log != "held\n" || !errors.Is(got.Err, want) {
		t.Errorf("failed refetch = %+v, want the held log and error", got)
	}
}

func TestAStatusContextNeverStartsAJobRequest(t *testing.T) {
	s := store.New(nil)
	if s.BeginJob(0) {
		t.Fatal("job zero started a request")
	}
	if got := s.Job(0); got.Status != store.StatusIdle || got.Loaded || got.Job.ID != 0 || got.Log != "" {
		t.Errorf("job zero = %+v, want the idle zero value", got)
	}
}

func TestJobCacheDropsTheLeastRecentlyReadLog(t *testing.T) {
	s := store.New(nil)
	for id := int64(1); id <= 5; id++ {
		s.BeginJob(id)
		s.JobApplied(id, gh.Job{ID: id}, []byte("log"))
	}
	s.UseJob(1)
	s.BeginJob(6)
	s.JobApplied(6, gh.Job{ID: 6}, []byte("new"))

	if !s.Job(1).Loaded {
		t.Error("the job read again was evicted")
	}
	if s.Job(2).Loaded {
		t.Error("the least recently read job remained cached")
	}
}

func TestJobLogsAreKeyedByConcreteAttempt(t *testing.T) {
	s := store.New(nil)
	for id, log := range map[int64]string{9001: "first", 9002: "rerun"} {
		s.BeginJob(id)
		s.JobApplied(id, gh.Job{ID: id, Name: "test"}, []byte(log))
	}
	if got := s.Job(9001).Log; got != "first" {
		t.Errorf("first attempt = %q, want first", got)
	}
	if got := s.Job(9002).Log; got != "rerun" {
		t.Errorf("rerun = %q, want rerun", got)
	}
}
