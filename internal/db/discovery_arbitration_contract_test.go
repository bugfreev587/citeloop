package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverySemanticArbitrationSchemaContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "migrations", "0047_discovery_semantic_arbitration.sql"))
	if err != nil {
		t.Fatalf("read discovery semantic arbitration migration: %v", err)
	}
	migration := strings.ToLower(string(raw))
	for _, want := range []string{
		"add column if not exists candidate_version bigint not null default 1",
		"create table if not exists ai_call_records",
		"create table if not exists discovery_arbitration_decisions",
		"create table if not exists work_review_memory",
		"create table if not exists work_signature_aliases",
		"create table if not exists discovery_semantic_gold_cases",
		"create table if not exists discovery_semantic_evaluations",
		"create table if not exists discovery_arbitration_configs",
		"expected_bucket_versions jsonb not null",
		"compared_work_ids jsonb not null",
		"automatic_suppression_enabled boolean not null default false",
		"check (not automatic_suppression_enabled or launch_ready)",
		"add column if not exists arbitration_decision_id uuid",
		"add column if not exists reserved_work_type text",
		"add column if not exists reserved_work_id uuid",
		"add column if not exists evidence_fingerprint text not null default ''",
		"where active = true",
	} {
		if !strings.Contains(migration, want) {
			t.Fatalf("discovery semantic arbitration migration missing %q", want)
		}
	}
	for _, want := range []string{
		"stage text not null check",
		"status text not null check",
		"confidence numeric(5,4) not null",
		"jsonb_typeof(expected_bucket_versions) = 'object'",
		"jsonb_typeof(compared_work_ids) = 'array'",
		"unique (project_id, alias_exact_signature_hash, alias_signature_version)",
	} {
		if !strings.Contains(migration, want) {
			t.Fatalf("arbitration schema invariant missing %q", want)
		}
	}
	memorySection := strings.SplitN(migration, "create table if not exists work_review_memory", 2)
	if len(memorySection) != 2 || !strings.Contains(strings.SplitN(memorySection[1], ");", 2)[0], "signature_payload jsonb not null") {
		t.Fatal("review memory must retain canonical signature material for semantic alias comparison")
	}
}

func TestDiscoveryArbitrationQueries(t *testing.T) {
	queries := map[string]string{
		"CreateAICallRecord":             createAICallRecord,
		"FinishAICallRecord":             finishAICallRecord,
		"CreateArbitrationDecision":      createArbitrationDecision,
		"MaterializeConflictBuckets":     materializeConflictBuckets,
		"GetConflictBucketSnapshot":      getConflictBucketSnapshot,
		"ListSnapshotActiveSignatures":   listSnapshotActiveSignatures,
		"ListSnapshotReviewMemory":       listSnapshotReviewMemory,
		"ListSnapshotReviewAliases":      listSnapshotReviewAliases,
		"ListDiscoveryReviewItems":       listDiscoveryReviewItems,
		"GetDiscoveryReviewItem":         getDiscoveryReviewItem,
		"UpsertWorkReviewMemory":         upsertWorkReviewMemory,
		"UpsertWorkSignatureAlias":       upsertWorkSignatureAlias,
		"GetDiscoveryCandidateForReview": getDiscoveryCandidateForReview,
		"ListDiscoverySemanticGoldCases": listDiscoverySemanticGoldCases,
		"CreateDiscoverySemanticEvaluation": createDiscoverySemanticEvaluation,
		"UpsertDiscoveryArbitrationEvaluationConfig": upsertDiscoveryArbitrationEvaluationConfig,
		"GetLatestDiscoverySemanticEvaluation": getLatestDiscoverySemanticEvaluation,
	}
	config := strings.ToLower(upsertDiscoveryArbitrationEvaluationConfig)
	if !strings.Contains(config, "automatic_suppression_enabled") || !strings.Contains(config, "launch_ready") {
		t.Fatal("evaluation config must persist launch and automatic suppression gates")
	}
	for name, query := range queries {
		if strings.TrimSpace(query) == "" {
			t.Fatalf("%s query should exist", name)
		}
		lower := strings.ToLower(query)
		if strings.Contains(name, "Snapshot") && !strings.Contains(lower, "project_id") {
			t.Fatalf("%s must be project scoped: %s", name, query)
		}
	}
	active := strings.ToLower(listSnapshotActiveSignatures)
	for _, want := range []string{"mode = 'enforced'", "active = true", "conflict_bucket_keys ?|"} {
		if !strings.Contains(active, want) {
			t.Fatalf("active snapshot query missing %q: %s", want, listSnapshotActiveSignatures)
		}
	}
	memory := strings.ToLower(listSnapshotReviewMemory)
	if !strings.Contains(memory, "active = true") || !strings.Contains(memory, "conflict_bucket_keys ?|") {
		t.Fatalf("review-memory snapshot must be active and bucket scoped: %s", listSnapshotReviewMemory)
	}
	aliases := strings.ToLower(listSnapshotReviewAliases)
	for _, want := range []string{"join work_review_memory", "active = true", "conflict_bucket_keys ?|"} {
		if !strings.Contains(aliases, want) {
			t.Fatalf("review-memory alias snapshot missing %q: %s", want, listSnapshotReviewAliases)
		}
	}
}
