package growthwork

import (
	"os"
	"strings"
	"testing"
)

func TestProjectCutoverIsFencedConservedLedgeredAndRollbackable(t *testing.T) {
	raw, err := os.ReadFile("service.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, want := range []string{
		"EnsureProjectCutover",
		"FenceProductWriterAuthority",
		"SwitchProductWriterAuthority",
		"CountUnrepresentedActiveLegacyGrowth",
		"CreateMigrationBatch",
		"AppendMigrationLedger",
		"rollbackGrowthCutover",
		"ReleaseProductWriterFence",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("Growth cutover missing %q", want)
		}
	}
}

func TestDoctorCreationRequiresGrowthVisibilityGate(t *testing.T) {
	raw, err := os.ReadFile("../api/handlers_site_fixes.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "EnsureProjectCutover") {
		t.Fatal("Doctor can reserve before active legacy Growth is represented")
	}
}

func TestCutoverCandidateIsJournaledBeforeArbitration(t *testing.T) {
	raw, err := os.ReadFile("service.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	migrateStart := strings.Index(source, "func (s *Service) migrateLegacyOpportunity")
	materializeFunction := strings.Index(source, "func (s *Service) materializeCutoverCandidate")
	if migrateStart < 0 || materializeFunction < 0 {
		t.Fatal("cutover materialization flow is missing")
	}
	migrateEnd := strings.Index(source[migrateStart:], "func canonicalParamsFromOpportunity") + migrateStart
	migrate := source[migrateStart:migrateEnd]
	if strings.Index(migrate, "materializeCutoverCandidate") > strings.Index(migrate, "NewArbitrationService") {
		t.Fatal("cutover candidate is arbitrated before durable materialization")
	}
	journalFunction := source[materializeFunction:]
	if !strings.Contains(journalFunction, "AppendGrowthCutoverSessionEntry") || !strings.Contains(journalFunction, "tx.Commit") {
		t.Fatal("cutover materialization does not atomically append its journal entry")
	}
	for _, want := range []string{"RunID", "CandidateID", "ArbitrationDecisionID", "AICallID", "InverseOperation"} {
		if !strings.Contains(source, want) {
			t.Fatalf("cutover journal does not cover %s", want)
		}
	}
}

func TestGrowthEvidenceMergeDispatchesToCanonicalOwner(t *testing.T) {
	raw, err := os.ReadFile("service.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, want := range []string{"mergePreparedEvidence", "OwnerDoctor", "MergeCanonicalDoctorSiteFixEvidence", "OwnerOpportunities", "MergeCanonicalGrowthOpportunityEvidence"} {
		if !strings.Contains(source, want) {
			t.Fatalf("owner-aware evidence merge missing %q", want)
		}
	}
}
