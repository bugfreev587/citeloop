package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorSiteFixRevisionUsesFindingScopedPredecessor(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "migrations", "0053_site_fix_revision_finding_relationship.sql"))
	if err != nil {
		t.Fatalf("read revision relationship migration: %v", err)
	}
	sql := strings.ToLower(string(raw))
	for _, want := range []string{
		"set local lock_timeout = '5s'",
		"set local statement_timeout = '30s'",
		"constraint site_fixes_project_finding_id_key unique (project_id, doctor_finding_id, id)",
		"constraint site_fixes_supersedes_finding_project_fk",
		"foreign key (project_id, doctor_finding_id, supersedes_site_fix_id)",
		"references site_fixes(project_id, doctor_finding_id, id)",
		"deferrable initially deferred not valid",
		"validate constraint site_fixes_supersedes_finding_project_fk",
		"drop constraint if exists site_fixes_supersedes_project_fk",
		"reset statement_timeout",
		"reset lock_timeout",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("revision relationship migration missing %q", want)
		}
	}
	if strings.Contains(sql, "foreign key (project_id, candidate_id, supersedes_site_fix_id)") {
		t.Fatal("revision relationship must allow a new immutable candidate snapshot")
	}
	validateAt := strings.Index(sql, "validate constraint site_fixes_supersedes_finding_project_fk")
	dropAt := strings.Index(sql, "drop constraint if exists site_fixes_supersedes_project_fk")
	if validateAt < 0 || dropAt < 0 || validateAt > dropAt {
		t.Fatal("new finding-scoped FK must validate before the old candidate-scoped FK is dropped")
	}
}
