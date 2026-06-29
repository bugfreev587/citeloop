package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestAdminUserRoutesAreRegistered(t *testing.T) {
	router := (&Server{}).Router()

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/admin/users"},
		{http.MethodDelete, "/api/admin/users/user_123"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		res := httptest.NewRecorder()

		router.ServeHTTP(res, req)

		if res.Code == http.StatusNotFound || res.Code == http.StatusMethodNotAllowed {
			t.Fatalf("%s %s should be registered, got %d", tc.method, tc.path, res.Code)
		}
	}
}

func TestAdminUserHandlersUseOwnerWideDelete(t *testing.T) {
	source, err := os.ReadFile("handlers_admin_users.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)

	for _, expected := range []string{
		"ListAdminUsers",
		"DeleteProjectsByOwner",
		"userEmail",
		"owner_email",
		"deleted_projects",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("handlers_admin_users.go should contain %q", expected)
		}
	}
	if strings.Contains(text, "DeleteProjectForOwner") || strings.Contains(text, "DeleteProject(") {
		t.Fatal("admin user delete must delete every project for the owner, not a single project")
	}
}
