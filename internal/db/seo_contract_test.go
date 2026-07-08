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
	for _, field := range []string{"intent_type", "engine"} {
		want := "coalesce(($11::jsonb)->>'" + field + "', '')"
		if !strings.Contains(query, want) {
			t.Fatalf("UpsertSEOOpportunity stable identity must use %q", want)
		}
	}
	for _, field := range []string{"evidence_window", "reason"} {
		if strings.Contains(query, "opportunity_identity_key") && strings.Contains(query, "opportunity_identity_key") {
			continue
		}
		want := "coalesce(($11::jsonb)->>'" + field + "', '')"
		if strings.Contains(query, want) && strings.Contains(query, "opportunity_key") {
			t.Fatalf("UpsertSEOOpportunity stable identity must not include volatile evidence field %q", field)
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

func TestUpsertSEOOpportunitySeparatesStableIdentityFromEvidenceFingerprint(t *testing.T) {
	query := strings.ToLower(upsertSEOOpportunity)
	for _, want := range []string{
		"opportunity_identity_key",
		"evidence_fingerprint",
		"status in ('open','accepted','converted','dismissed','snoozed','watching')",
		"seo_opportunities.evidence_fingerprint",
		"previously_reviewed_status",
		"reopened_reason",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("UpsertSEOOpportunity must separate stable identity and reviewed-state evidence; missing %q in %s", want, query)
		}
	}
	identityEnd := strings.Index(query, "as opportunity_identity_key")
	fingerprintEnd := strings.Index(query, "as evidence_fingerprint")
	if identityEnd < 0 || fingerprintEnd < 0 || fingerprintEnd <= identityEnd {
		t.Fatalf("UpsertSEOOpportunity should compute identity before fingerprint: %s", query)
	}
	identityStart := strings.LastIndex(query[:identityEnd], "encode(digest(")
	fingerprintStart := strings.LastIndex(query[identityEnd:fingerprintEnd], "encode(digest(")
	if identityStart < 0 || fingerprintStart < 0 {
		t.Fatalf("UpsertSEOOpportunity should hash both identity and fingerprint: %s", query)
	}
	fingerprintStart += identityEnd
	identityExpr := query[identityStart:identityEnd]
	for _, forbidden := range []string{"evidence_window", "reason", "priority_score", "confidence"} {
		if strings.Contains(identityExpr, forbidden) {
			t.Fatalf("stable opportunity identity must not include %q: %s", forbidden, identityExpr)
		}
	}
	fingerprintExpr := query[fingerprintStart:fingerprintEnd]
	for _, want := range []string{"evidence_window", "reason", "$4::numeric::text", "$5::numeric::text"} {
		if !strings.Contains(fingerprintExpr, want) {
			t.Fatalf("evidence fingerprint must include %q: %s", want, fingerprintExpr)
		}
	}
}

func TestOpportunityReconsiderationSchemaAddsReturnedAndReviewStates(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "migrations", "0042_opportunity_reconsideration_review_states.sql"))
	if err != nil {
		t.Fatalf("read opportunity reconsideration migration: %v", err)
	}
	migration := strings.ToLower(string(raw))
	for _, want := range []string{
		"content_actions_status_check",
		"'returned'",
		"add column if not exists status_reason",
		"add column if not exists opportunity_identity_key",
		"add column if not exists evidence_fingerprint",
		"create table if not exists seo_opportunity_review_states",
		"unique (project_id, opportunity_identity_key)",
		"review_status text not null",
		"'dismissed','snoozed','watching'",
		"drop index if exists uniq_open_seo_opportunity_key",
		"where status in ('open','accepted','converted','dismissed','snoozed','watching')",
	} {
		if !strings.Contains(migration, want) {
			t.Fatalf("opportunity reconsideration migration missing %q in %s", want, migration)
		}
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
