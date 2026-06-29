package db

import (
	"strings"
	"testing"
)

func TestAdminProjectQueriesExposeGlobalManagementSurface(t *testing.T) {
	if !strings.Contains(listAdminProjects, "from projects") {
		t.Fatal("ListAdminProjects should read from projects")
	}
	if !strings.Contains(listAdminProjects, "order by created_at desc") {
		t.Fatal("ListAdminProjects should keep newest projects first")
	}
	if !strings.Contains(deleteProject, "delete from projects") {
		t.Fatal("DeleteProject should delete from projects")
	}
	whereClause := deleteProject
	if parts := strings.SplitN(whereClause, "returning", 2); len(parts) > 0 {
		whereClause = parts[0]
	}
	if strings.Contains(whereClause, "owner_id") {
		t.Fatal("DeleteProject should be admin-scoped by route auth, not owner_id in the where clause")
	}
}
