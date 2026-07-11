package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestAdminDiscoveryShadowRoutesAreRegistered(t *testing.T) {
	router := (&Server{}).Router()
	projectID := "00000000-0000-0000-0000-000000000001"
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/admin/projects/" + projectID + "/discovery-shadow/run"},
		{http.MethodGet, "/api/admin/projects/" + projectID + "/discovery-shadow/report"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code == http.StatusNotFound || res.Code == http.StatusMethodNotAllowed {
			t.Fatalf("%s %s should be registered, got %d", tc.method, tc.path, res.Code)
		}
	}
}

func TestAdminDiscoveryShadowHandlersStayAdminOnlyAndShadowOnly(t *testing.T) {
	serverRaw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	handlerRaw, err := os.ReadFile("handlers_admin_discovery.go")
	if err != nil {
		t.Fatalf("read admin discovery handlers: %v", err)
	}
	server := string(serverRaw)
	handler := string(handlerRaw)
	for _, want := range []string{
		`r.Use(s.requireAdmin)`,
		`/admin/projects/{projectID}/discovery-shadow/run`,
		`/admin/projects/{projectID}/discovery-shadow/report`,
	} {
		if !strings.Contains(server, want) {
			t.Fatalf("admin discovery route contract missing %q", want)
		}
	}
	for _, want := range []string{
		"discovery.NewPostgresRepository",
		"discovery.NewService",
		"RunProject",
		"LatestReport",
	} {
		if !strings.Contains(handler, want) {
			t.Fatalf("admin discovery handler missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"UpdateSEOOpportunityStatus",
		"DismissSEODoctorFinding",
		"UpsertSEOOpportunity",
	} {
		if strings.Contains(handler, forbidden) {
			t.Fatalf("shadow handler must not mutate legacy work via %q", forbidden)
		}
	}
}
