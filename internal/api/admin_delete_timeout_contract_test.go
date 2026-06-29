package api

import (
	"os"
	"strings"
	"testing"
)

func TestAdminDeleteHandlersUseBoundedDatabaseContexts(t *testing.T) {
	for _, file := range []string{"handlers_admin_projects.go", "handlers_admin_users.go"} {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		text := string(source)
		if !strings.Contains(text, "adminDeleteContext(r.Context())") {
			t.Fatalf("%s should wrap destructive admin deletes in adminDeleteContext", file)
		}
		if strings.Contains(text, "DeleteProject(r.Context()") || strings.Contains(text, "DeleteProjectsByOwner(r.Context()") {
			t.Fatalf("%s should not pass the request context directly to destructive admin deletes", file)
		}
		if !strings.Contains(text, "writeAdminDeleteError") {
			t.Fatalf("%s should map delete timeouts to an explicit admin delete response", file)
		}
	}
}

func TestAdminDeleteTimeoutContract(t *testing.T) {
	source, err := os.ReadFile("admin_delete.go")
	if err != nil {
		t.Fatalf("admin delete helper missing: %v", err)
	}
	text := string(source)
	for _, expected := range []string{
		"adminDeleteTimeout",
		"context.WithTimeout(parent, adminDeleteTimeout)",
		"http.StatusGatewayTimeout",
		"admin delete timed out",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("admin_delete.go should contain %q", expected)
		}
	}
}

func TestAdminDeletesCoordinateWithSchedulerAdvisoryLock(t *testing.T) {
	source, err := os.ReadFile("admin_delete.go")
	if err != nil {
		t.Fatalf("admin delete helper missing: %v", err)
	}
	text := string(source)
	for _, expected := range []string{
		"pg_advisory_xact_lock($1)",
		"scheduler.LockKey(projectID)",
		"deleteAdminProjectRecord",
		"deleteAdminOwnerProjects",
		"ListProjectsByOwner",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("admin_delete.go should contain %q", expected)
		}
	}
}
