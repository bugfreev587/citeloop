package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestAdminProjectRoutesAreRegistered(t *testing.T) {
	router := (&Server{}).Router()

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/admin/projects"},
		{http.MethodDelete, "/api/admin/projects/00000000-0000-0000-0000-000000000001"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		res := httptest.NewRecorder()

		router.ServeHTTP(res, req)

		if res.Code == http.StatusNotFound || res.Code == http.StatusMethodNotAllowed {
			t.Fatalf("%s %s should be registered, got %d", tc.method, tc.path, res.Code)
		}
	}
}

func TestAdminProjectHandlersUseAdminOnlyQueries(t *testing.T) {
	source, err := os.ReadFile("handlers_admin_projects.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)

	for _, expected := range []string{
		"ListAdminProjects",
		"DeleteProject",
		"userEmail",
		"owner_email",
		"updated_at",
		"UpdatedAt",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("handlers_admin_projects.go should contain %q", expected)
		}
	}
	if strings.Contains(text, "DeleteProjectForOwner") {
		t.Fatal("admin project delete must not be scoped to the current owner")
	}
}
