package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorSiteFixRevisionSignatureSchemaContract(t *testing.T) {
	indexRaw, err := os.ReadFile(filepath.Join("..", "migrations", "0052_01_work_signature_shadow_candidate.sql"))
	if err != nil {
		t.Fatalf("read concurrent shadow index migration: %v", err)
	}
	dropRaw, err := os.ReadFile(filepath.Join("..", "migrations", "0052_02_work_signature_revision_support.sql"))
	if err != nil {
		t.Fatalf("read revision support migration: %v", err)
	}
	indexSQL := strings.ToLower(string(indexRaw))
	dropSQL := strings.ToLower(string(dropRaw))
	for _, want := range []string{
		"citeloop:migration-mode=nontransactional",
		"create unique index concurrently",
		"on work_signature_registry (candidate_id)",
		"where mode in ('shadow')",
	} {
		if !strings.Contains(indexSQL, want) {
			t.Fatalf("shadow index migration missing %q", want)
		}
	}
	if !strings.Contains(dropSQL, "drop constraint if exists work_signature_registry_candidate_id_mode_key") {
		t.Fatal("revision migration must remove the constraint that blocks a second enforced signature")
	}
	if strings.Contains(dropSQL, "drop index") && strings.Contains(dropSQL, "uniq_enforced_active_work_signature") {
		t.Fatal("revision migration must preserve active exact-signature uniqueness")
	}
}

func TestTerminalSiteFixCanReserveRevisionSignatureContract(t *testing.T) {
	raw, err := os.ReadFile("queries/discovery.sql")
	if err != nil {
		t.Fatal(err)
	}
	insert := queryContractSection(t, string(raw), "InsertEnforcedWorkSignature")
	if strings.Contains(strings.ToLower(insert), "on conflict (candidate_id") {
		t.Fatal("enforced revisions must not conflict on candidate_id after a terminal predecessor")
	}
	shadow := queryContractSection(t, string(raw), "UpsertShadowWorkSignature")
	lower := strings.ToLower(shadow)
	if !strings.Contains(lower, "on conflict (candidate_id) where mode in ('shadow')") {
		t.Fatal("shadow upsert must infer the replacement partial unique index")
	}
}
