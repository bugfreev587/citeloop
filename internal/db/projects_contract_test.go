package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectQueriesAreOwnerScoped(t *testing.T) {
	if !strings.Contains(listProjectsByOwner, "where owner_id = $1") {
		t.Fatal("ListProjectsByOwner must filter by owner_id")
	}
	if !strings.Contains(getProjectForOwner, "where id = $1") || !strings.Contains(getProjectForOwner, "and owner_id = $2") {
		t.Fatal("GetProjectForOwner must be scoped by project id and owner id")
	}
	if !strings.Contains(updateProjectConfigForOwner, "where id = $1") || !strings.Contains(updateProjectConfigForOwner, "and owner_id = $3") {
		t.Fatal("UpdateProjectConfigForOwner must be scoped by project id and owner id")
	}
}

func TestProjectQueriesTouchUpdatedAt(t *testing.T) {
	if !strings.Contains(createProject, "updated_at") {
		t.Fatal("CreateProject should return updated_at")
	}
	if !strings.Contains(updateProjectConfig, "updated_at = now()") {
		t.Fatal("UpdateProjectConfig should refresh project updated_at")
	}
	if !strings.Contains(updateProjectConfigForOwner, "updated_at = now()") {
		t.Fatal("UpdateProjectConfigForOwner should refresh project updated_at")
	}

	files, err := os.ReadDir(filepath.Join("..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	var migration string
	for _, file := range files {
		if strings.Contains(file.Name(), "project_updated_at") {
			body, err := os.ReadFile(filepath.Join("..", "migrations", file.Name()))
			if err != nil {
				t.Fatal(err)
			}
			migration += "\n" + strings.ToLower(string(body))
		}
	}
	if migration == "" {
		t.Fatal("project updated_at migration is required")
	}
	for _, expected := range []string{
		"alter table projects",
		"add column if not exists updated_at",
		"update projects",
		"set updated_at = created_at",
		"set not null",
	} {
		if !strings.Contains(migration, expected) {
			t.Fatalf("project updated_at migration should contain %q", expected)
		}
	}
}

func TestProjectHardDeleteIsOwnerScopedAndCascading(t *testing.T) {
	if !strings.Contains(deleteProjectForOwner, "delete from projects") {
		t.Fatal("DeleteProjectForOwner must delete from projects")
	}
	if !strings.Contains(deleteProjectForOwner, "where id = $1") || !strings.Contains(deleteProjectForOwner, "and owner_id = $2") {
		t.Fatal("DeleteProjectForOwner must be scoped by project id and owner id")
	}
	if !strings.Contains(deleteProjectForOwner, "returning") {
		t.Fatal("DeleteProjectForOwner should return the deleted project for audit/UI feedback")
	}

	files, err := os.ReadDir(filepath.Join("..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	var migration string
	for _, file := range files {
		if strings.Contains(file.Name(), "project_hard_delete_cascade") {
			body, err := os.ReadFile(filepath.Join("..", "migrations", file.Name()))
			if err != nil {
				t.Fatal(err)
			}
			migration += "\n" + strings.ToLower(string(body))
		}
	}
	if migration == "" {
		t.Fatal("project hard delete cascade migration is required")
	}

	for _, table := range []string{
		"product_profiles",
		"content_inventory",
		"topics",
		"articles",
		"generation_runs",
		"notification_channels",
		"notification_subscriptions",
		"notification_deliveries",
	} {
		if !strings.Contains(migration, "alter table public."+table) {
			t.Fatalf("cascade migration must alter %s", table)
		}
	}
	if got := strings.Count(migration, "on delete cascade"); got < 8 {
		t.Fatalf("cascade migration should add on delete cascade for every project-scoped legacy table, got %d", got)
	}
}

func TestContentPlanAutoMigrationDisablesLegacyProjects(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "migrations", "0035_pause_content_plan_auto.sql"))
	if err != nil {
		t.Fatal(err)
	}
	migration := strings.ToLower(string(raw))

	for _, want := range []string{
		"update projects",
		"jsonb_set",
		"auto_advance_enabled",
		"false",
	} {
		if !strings.Contains(migration, want) {
			t.Fatalf("content plan auto migration missing %q", want)
		}
	}
}
