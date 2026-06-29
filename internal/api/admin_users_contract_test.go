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
	handler, err := os.ReadFile("handlers_admin_users.go")
	if err != nil {
		t.Fatal(err)
	}
	helper, err := os.ReadFile("admin_delete.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(handler) + "\n" + string(helper)

	for _, expected := range []string{
		"ListAdminUsers",
		"DeleteProjectsByOwner",
		"deleteAdminOwnerProjects",
		"userEmail",
		"owner_email",
		"deleted_projects",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("admin user delete source should contain %q", expected)
		}
	}
	handlerText := string(handler)
	if strings.Contains(handlerText, "DeleteProjectForOwner") || strings.Contains(handlerText, "DeleteProject(") {
		t.Fatal("admin user delete must delete every project for the owner, not a single project")
	}
}
