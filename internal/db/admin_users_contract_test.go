package db

import (
	"strings"
	"testing"
)

func TestAdminUserQueriesExposeOwnerManagementSurface(t *testing.T) {
	for _, expected := range []string{
		"owner_id",
		"count(*)",
		"min(created_at)",
		"max(updated_at)",
		"group by owner_id",
		"order by updated_at desc, created_at desc",
	} {
		if !strings.Contains(listAdminUsers, expected) {
			t.Fatalf("ListAdminUsers should contain %q", expected)
		}
	}
}

func TestAdminUserDeleteRemovesProjectsByOwner(t *testing.T) {
	if !strings.Contains(deleteProjectsByOwner, "delete from projects") {
		t.Fatal("DeleteProjectsByOwner should delete from projects")
	}
	if !strings.Contains(deleteProjectsByOwner, "where owner_id = $1") {
		t.Fatal("DeleteProjectsByOwner should delete every project owned by the selected user")
	}
	if !strings.Contains(deleteProjectsByOwner, "returning") {
		t.Fatal("DeleteProjectsByOwner should return deleted projects for audit/UI feedback")
	}
	whereClause := deleteProjectsByOwner
	if parts := strings.SplitN(whereClause, "returning", 2); len(parts) > 0 {
		whereClause = parts[0]
	}
	if strings.Contains(whereClause, "where id =") {
		t.Fatal("DeleteProjectsByOwner must not delete a single project id")
	}
}
