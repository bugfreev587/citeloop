package db

import (
	"os"
	"strings"
	"testing"
)

func TestSiteFixEvidenceMergeRelationshipReadsHaveAConcurrentCoveringIndex(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0101_site_fix_evidence_merge_read_index.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration := strings.ToLower(string(raw))
	for _, required := range []string{
		"-- citeloop:migration-mode=nontransactional",
		"-- citeloop:index=idx_site_fix_evidence_merges_project_fix_finding",
		"create index concurrently if not exists idx_site_fix_evidence_merges_project_fix_finding",
		"on site_fix_evidence_merges (project_id, site_fix_id, doctor_finding_id)",
	} {
		if !strings.Contains(migration, required) {
			t.Errorf("relationship-read index migration missing %q", required)
		}
	}
	if strings.Count(migration, "create index") != 1 {
		t.Fatalf("nontransactional migration must create exactly one index")
	}
}
