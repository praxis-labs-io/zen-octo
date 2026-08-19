package prview

import (
	"testing"
	"time"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

func TestLogLinesSkipAStatusOnlyStepAndReachTheOneAfterIt(t *testing.T) {
	at := time.Date(2026, 8, 19, 14, 0, 0, 0, time.UTC)
	job := gh.Job{Steps: []gh.JobStep{
		{Number: 1, Name: "one", StartedAt: at},
		{Number: 2, Name: "skipped", State: gh.CheckStateSkipped},
		{Number: 3, Name: "three", StartedAt: at.Add(2 * time.Second)},
	}}
	sections := splitJobLog(job,
		"2026-08-19T14:00:01Z first\n2026-08-19T14:00:03Z third\n")
	if len(sections[0].lines) != 1 || sections[0].lines[0] != "first" {
		t.Errorf("first = %q", sections[0].lines)
	}
	if len(sections[1].lines) != 0 {
		t.Errorf("skipped = %q, want no log", sections[1].lines)
	}
	if len(sections[2].lines) != 1 || sections[2].lines[0] != "third" {
		t.Errorf("third = %q", sections[2].lines)
	}
}
