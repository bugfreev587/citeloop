package db

import (
	"os"
	"strings"
	"testing"
)

func TestGrowthAliasSignatureForeignKeyIsDeferredUntilReservationCommit(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0074a_growth_alias_signature_deferred.sql")
	if err != nil {
		t.Fatalf("read deferred Growth alias migration: %v", err)
	}
	sql := strings.ToLower(string(raw))
	for _, want := range []string{
		"alter table growth_opportunity_work_aliases",
		"alter constraint growth_opportunity_work_alias_project_id_work_signature_id_fkey",
		"deferrable initially deferred",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("deferred Growth alias migration missing %q", want)
		}
	}
}
