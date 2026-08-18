package gh

import (
	"encoding/json"
	"testing"
)

// startedAt and completedAt are independently nullable on the wire. A
// completed timestamp with no start must not compute against the zero time.
func TestDurationNeedsBothTimestamps(t *testing.T) {
	const body = `{"nodes": [{"commit": {"statusCheckRollup": {
	  "state": "SUCCESS",
	  "contexts": {"nodes": [
	    {"__typename": "CheckRun", "name": "deploy", "status": "COMPLETED",
	     "conclusion": "SUCCESS", "completedAt": "2026-08-05T09:05:00Z"}
	  ]}
	}}}]}`

	var node rollupNode
	if err := json.Unmarshal([]byte(body), &node); err != nil {
		t.Fatalf("decoding fixture: %v", err)
	}

	r := rollup(node)
	if len(r.Checks) != 1 {
		t.Fatalf("checks = %d, want 1", len(r.Checks))
	}
	if got := r.Checks[0].Duration; got != 0 {
		t.Errorf("Duration = %v, want zero: no startedAt to measure from", got)
	}
}
