package db

import (
	"os"
	"strings"
	"testing"
)

func TestAdminDeletePerformanceIndexesMigration(t *testing.T) {
	data, err := os.ReadFile("../migrations/0026_admin_delete_indexes.sql")
	if err != nil {
		t.Fatalf("admin delete index migration missing: %v", err)
	}
	sql := strings.ToLower(string(data))

	for _, want := range []string{
		"create index if not exists idx_projects_owner_id",
		"on projects (owner_id)",
		"create index if not exists idx_articles_project_id",
		"on articles (project_id)",
		"create index if not exists idx_generation_runs_project_id",
		"on generation_runs (project_id)",
		"create index if not exists idx_notification_deliveries_channel_id",
		"on notification_deliveries (channel_id)",
		"create index if not exists idx_workflow_events_project_id",
		"on workflow_events (project_id)",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration should contain %q", want)
		}
	}
}
