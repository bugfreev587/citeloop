package seo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrowthGeneratorsCannotCallLegacyOpportunityUpsert(t *testing.T) {
	for _, name := range []string{"service.go", "doctor.go"} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), ".UpsertSEOOpportunity(") {
			t.Fatalf("%s bypasses canonical Growth reservation", name)
		}
	}
	raw, err := os.ReadFile(filepath.Join("..", "growthwork", "service.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, want := range []string{
		"migrateLegacyGrowth", "NewArbitrationService", "ReservePrepared",
		"mergeExactEvidence", "ErrSnapshotStale", "CreateCanonicalGrowthOpportunity",
		"EnsureOpportunityReserved", "CreateDuplicateGrowthOpportunityAlias",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("canonical Growth writer missing %q", want)
		}
	}
}
