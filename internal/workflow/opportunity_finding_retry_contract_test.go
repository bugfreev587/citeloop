package workflow

import (
	"os"
	"strings"
	"testing"
)

func TestStuckOpportunityFindingIsReclaimedForCheckpointedRetry(t *testing.T) {
	raw, err := os.ReadFile("../db/queries/workflow.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	start := strings.Index(sql, "-- name: reclaimstuckworkflowevents")
	end := strings.Index(sql[start:], "-- name: markworkfloweventsucceeded")
	if start < 0 || end < 0 {
		t.Fatal("reclaim query not found")
	}
	body := sql[start : start+end]
	if strings.Contains(body, "opportunity_finding.requested") || strings.Contains(body, "explicit retry required") {
		t.Fatal("checkpointed Opportunity Finding must no longer be killed during worker reclaim")
	}
	for _, want := range []string{"status = 'pending'", "run_after = now()", "reclaimed after worker timeout"} {
		if !strings.Contains(body, want) {
			t.Fatalf("retryable reclaim query missing %q", want)
		}
	}
}
