package scheduler

import (
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
)

func TestOpportunityBatchKeyUsesUnplannedActionIdentity(t *testing.T) {
	projectID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	firstBatch := []db.ContentAction{{
		ID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
	}}
	secondBatch := []db.ContentAction{{
		ID: uuid.MustParse("33333333-3333-3333-3333-333333333333"),
	}}

	firstKey := opportunityBatchKey(projectID, firstBatch)
	secondKey := opportunityBatchKey(projectID, secondBatch)

	if firstKey == secondKey {
		t.Fatalf("batch key must change when a later reviewed opportunity batch has different unplanned actions: %q", firstKey)
	}
	if firstKey != firstBatch[0].ID.String() {
		t.Fatalf("first batch key = %q, want first unplanned action id %q", firstKey, firstBatch[0].ID)
	}
}

func TestOpportunityBatchKeyFallsBackToProjectWhenEmpty(t *testing.T) {
	projectID := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	if got := opportunityBatchKey(projectID, nil); got != projectID.String() {
		t.Fatalf("empty batch key = %q, want project id %q", got, projectID)
	}
}
