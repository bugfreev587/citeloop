package db

import (
	"strings"
	"testing"
)

func TestUpsertSEOOpportunityCastsEvidenceForJSONOperators(t *testing.T) {
	query := strings.ToLower(upsertSEOOpportunity)
	if strings.Contains(query, "$11->>") {
		t.Fatal("UpsertSEOOpportunity must cast the evidence parameter before using json operators")
	}
	for _, field := range []string{"intent_type", "engine", "evidence_window", "reason"} {
		want := "coalesce(($11::jsonb)->>'" + field + "', '')"
		if !strings.Contains(query, want) {
			t.Fatalf("UpsertSEOOpportunity must use %q in opportunity_key hash", want)
		}
	}
}
