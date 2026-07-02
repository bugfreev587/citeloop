package db

import (
	"os"
	"path/filepath"
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

func TestUpsertSEOOpportunityUsesConsistentProjectIDParameterType(t *testing.T) {
	query := strings.ToLower(upsertSEOOpportunity)
	if strings.Contains(query, "$1::text") {
		t.Fatal("UpsertSEOOpportunity must not type project_id as text when the insert target is uuid")
	}
	if !strings.Contains(query, "$1::uuid::text") {
		t.Fatal("UpsertSEOOpportunity must derive the opportunity hash from project_id as uuid text")
	}
}

func TestLatestTechnicalChecksQuerySupportsAnalyzerExpansion(t *testing.T) {
	query := strings.ToLower(listLatestTechnicalChecks)
	for _, want := range []string{
		"from technical_checks tc",
		"join seo_runs sr on sr.id = tc.run_id",
		"agent = 'seo_sync'",
		"max(started_at)",
		"limit $2",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("ListLatestTechnicalChecks query missing %q in %s", want, query)
		}
	}
}

func TestSEODoctorSchemaDefinesRunsAndFindings(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "migrations", "0034_seo_doctor.sql"))
	if err != nil {
		t.Fatalf("read seo doctor migration: %v", err)
	}
	migration := strings.ToLower(string(raw))
	for _, want := range []string{
		"create table if not exists seo_doctor_runs",
		"create table if not exists seo_doctor_findings",
		"trigger text not null",
		"progress_percent int not null",
		"block_reason text",
		"finding_key text not null",
		"developer_instructions text not null",
		"acceptance_tests jsonb not null default '[]'",
		"where status in ('queued','running')",
		"where status = 'active'",
	} {
		if !strings.Contains(migration, want) {
			t.Fatalf("seo doctor migration missing %q in %s", want, migration)
		}
	}
}

func TestSEODoctorQueriesExposeRunFindingAndFreshnessContracts(t *testing.T) {
	queries := map[string]string{
		"CreateSEODoctorRun":              createSEODoctorRun,
		"GetSEODoctorRun":                 getSEODoctorRun,
		"GetActiveSEODoctorRun":           getActiveSEODoctorRun,
		"UpdateSEODoctorRunProgress":      updateSEODoctorRunProgress,
		"CompleteSEODoctorRun":            completeSEODoctorRun,
		"FailSEODoctorRun":                failSEODoctorRun,
		"LatestSEODoctorRun":              latestSEODoctorRun,
		"LatestCompletedSEODoctorRun":     latestCompletedSEODoctorRun,
		"CountManualSEODoctorRunsSince":   countManualSEODoctorRunsSince,
		"ListSEODoctorRunsDueWeekly":      listSEODoctorRunsDueWeekly,
		"UpsertSEODoctorFinding":          upsertSEODoctorFinding,
		"ResolveMissingSEODoctorFindings": resolveMissingSEODoctorFindings,
		"ListSEODoctorFindingsForRun":     listSEODoctorFindingsForRun,
		"GetSEODoctorFinding":             getSEODoctorFinding,
		"DismissSEODoctorFinding":         dismissSEODoctorFinding,
		"LinkSEODoctorFindingToAction":    linkSEODoctorFindingToAction,
	}
	for name, query := range queries {
		if strings.TrimSpace(query) == "" {
			t.Fatalf("%s query should exist", name)
		}
	}
	if !strings.Contains(strings.ToLower(getActiveSEODoctorRun), "status in ('queued','running')") {
		t.Fatalf("GetActiveSEODoctorRun must dedupe queued/running runs: %s", getActiveSEODoctorRun)
	}
	if !strings.Contains(strings.ToLower(countManualSEODoctorRunsSince), "trigger = 'manual'") {
		t.Fatalf("CountManualSEODoctorRunsSince must count manual runs only: %s", countManualSEODoctorRunsSince)
	}
	if !strings.Contains(strings.ToLower(listSEODoctorRunsDueWeekly), "interval '6 days'") {
		t.Fatalf("ListSEODoctorRunsDueWeekly must respect manual/onboarding/weekly freshness window: %s", listSEODoctorRunsDueWeekly)
	}
}
