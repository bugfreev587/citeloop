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
