package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoveryWorkIdentitySchemaContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "migrations", "0046_discovery_work_identity.sql"))
	if err != nil {
		t.Fatalf("read discovery work identity migration: %v", err)
	}
	migration := strings.ToLower(string(raw))
	for _, want := range []string{
		"create table if not exists discovery_shadow_runs",
		"create table if not exists discovery_candidates",
		"create table if not exists work_conflict_buckets",
		"create table if not exists work_signature_registry",
		"create table if not exists discovery_review_items",
		"mode text not null default 'shadow'",
		"where mode = 'enforced' and active = true",
		"unique (project_id, bucket_key)",
		"unique (candidate_id, mode)",
	} {
		if !strings.Contains(migration, want) {
			t.Fatalf("discovery migration missing %q", want)
		}
	}
}

func TestDiscoveryQueriesExposeShadowFoundation(t *testing.T) {
	queries := map[string]string{
		"CreateDiscoveryShadowRun":                     createDiscoveryShadowRun,
		"CompleteDiscoveryShadowRun":                   completeDiscoveryShadowRun,
		"FailDiscoveryShadowRun":                       failDiscoveryShadowRun,
		"UpsertDiscoveryCandidate":                     upsertDiscoveryCandidate,
		"UpsertShadowWorkSignature":                    upsertShadowWorkSignature,
		"DeleteShadowWorkSignatureForCandidate":        deleteShadowWorkSignatureForCandidate,
		"EnsureWorkConflictBucket":                     ensureWorkConflictBucket,
		"ListActiveSEOOpportunitiesForDiscoveryShadow": listActiveSEOOpportunitiesForDiscoveryShadow,
		"ListActiveDoctorFindingsForDiscoveryShadow":   listActiveDoctorFindingsForDiscoveryShadow,
		"GetLatestDiscoveryShadowRun":                  getLatestDiscoveryShadowRun,
		"ListDiscoveryShadowSignaturesForRun":          listDiscoveryShadowSignaturesForRun,
	}
	for name, query := range queries {
		if strings.TrimSpace(query) == "" {
			t.Fatalf("%s query should exist", name)
		}
	}
	for name, query := range map[string]string{
		"opportunities": listActiveSEOOpportunitiesForDiscoveryShadow,
		"doctor":        listActiveDoctorFindingsForDiscoveryShadow,
	} {
		lower := strings.ToLower(query)
		if !strings.Contains(lower, "project_id = $1") {
			t.Fatalf("%s shadow inventory must be project scoped: %s", name, query)
		}
		if !strings.Contains(lower, "status in") {
			t.Fatalf("%s shadow inventory must be status bounded: %s", name, query)
		}
	}
	if strings.Contains(strings.ToLower(upsertDiscoveryCandidate), "update seo_opportunities") ||
		strings.Contains(strings.ToLower(upsertDiscoveryCandidate), "update seo_doctor_findings") {
		t.Fatal("shadow candidate upsert must not mutate legacy work rows")
	}
	candidateQuery := strings.ToLower(upsertDiscoveryCandidate)
	if !strings.Contains(candidateQuery, "shadow_run_id, project_id, source_kind, source_object_type, source_object_id, candidate_schema_version") {
		t.Fatal("candidate snapshots must be idempotent within a run without overwriting prior-run provenance")
	}
	signatureQuery := strings.ToLower(upsertShadowWorkSignature)
	if !strings.Contains(signatureQuery, "on conflict (candidate_id) where mode in ('shadow')") {
		t.Fatal("shadow signature upsert must target only the partial shadow uniqueness index")
	}
	updateClause := strings.SplitN(signatureQuery, "do update set", 2)
	if len(updateClause) != 2 {
		t.Fatal("shadow signature query must expose an idempotent update clause")
	}
	if strings.Contains(updateClause[1], "mode =") || strings.Contains(updateClause[1], "active =") {
		t.Fatal("shadow signature retry must never rewrite enforcement mode or active state")
	}
}
