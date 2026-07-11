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
		"unique (candidate_id)",
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
}
