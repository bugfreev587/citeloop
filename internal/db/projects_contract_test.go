package db

import (
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
