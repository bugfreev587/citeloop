package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyDoctorHealthSentinelMigrationIsPreciseAndIdempotent(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "migrations", "0073_legacy_doctor_health_sentinel.sql"))
	if err != nil {
		t.Fatalf("read legacy Doctor health migration: %v", err)
	}
	sql := strings.ToLower(string(raw))
	for _, required := range []string{
		"issue_type = 'no_active_technical_blockers'",
		"set local lock_timeout = '5s'",
		"set local statement_timeout = '30s'",
		"category = 'coverage'",
		"severity = 'info'",
		"lower(btrim(fix_intent)) = 'no repair needed.'",
		"evidence->>'source' = 'technical_checks'",
		"'check', 'legacy_report_health'",
		"run.status = 'completed'",
		"run.healthy_coverage = '[]'::jsonb",
		"jsonb_array_elements(healthy_coverage)",
		"else 'healthy'",
		"status = 'resolved'",
		"legacy_health_reclassification_version",
		"legacy_health_reclassification_version' is distinct from 'v1'",
		"legacy_health_previous_finding_kind",
		"legacy_health_previous_status",
		"legacy_health_kind_preserved_for_dependencies",
		"from site_fixes fix",
		"from site_fix_evidence_merges merge",
		"historical doctor health sentinel is not repairable.",
		"update work_signature_registry signature",
		"status = 'failed_terminal', active = false",
		"autofix_eligible = false",
		"review_required = false",
		"developer_instructions = ''",
		"likely_files_or_surfaces = '[]'::jsonb",
		"acceptance_tests = '[]'::jsonb",
		"resolved_at = coalesce(resolved_at, now())",
	} {
		if !strings.Contains(sql, required) {
			t.Errorf("migration missing %q", required)
		}
	}
}

func TestDoctorReportQueriesHideLegacyHealthSentinel(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("queries", "seo.sql"))
	if err != nil {
		t.Fatalf("read Doctor queries: %v", err)
	}
	sql := strings.ToLower(string(raw))
	for _, query := range []string{"-- name: listseodoctorfindingsforrun :many", "-- name: listcurrentseodoctorfindings :many"} {
		start := strings.Index(sql, query)
		if start < 0 {
			t.Fatalf("missing query %s", query)
		}
		end := strings.Index(sql[start:], ";")
		if end < 0 {
			t.Fatalf("unterminated query %s", query)
		}
		body := sql[start : start+end]
		if !strings.Contains(body, "issue_type <> 'no_active_technical_blockers'") {
			t.Errorf("%s does not hide legacy health sentinel", query)
		}
	}
}
