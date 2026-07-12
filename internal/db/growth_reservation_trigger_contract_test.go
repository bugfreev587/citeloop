package db

import (
	"os"
	"strings"
	"testing"
)

func TestCanonicalGrowthReservationTriggerUsesTableSpecificRecordFields(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0074b_fix_canonical_growth_reservation_trigger.sql")
	if err != nil {
		t.Fatalf("read canonical Growth trigger fix: %v", err)
	}
	sql := strings.ToLower(string(raw))
	for _, want := range []string{
		"if tg_table_name = 'seo_opportunities' then",
		"opportunity_id := new.id",
		"elsif tg_table_name = 'content_actions' then",
		"opportunity_id := new.opportunity_id",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("canonical Growth trigger fix missing %q", want)
		}
	}
	if strings.Contains(sql, "case when tg_table_name") {
		t.Fatal("polymorphic trigger must not resolve fields from the wrong NEW record type")
	}
}
