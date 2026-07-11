package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestAdminDiscoveryReviewRoutesAreRegistered(t *testing.T) {
	router := (&Server{}).Router()
	projectID := "00000000-0000-0000-0000-000000000001"
	candidateID := "00000000-0000-0000-0000-000000000002"
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/admin/projects/" + projectID + "/discovery-arbitration/" + candidateID + "/prepare"},
		{http.MethodGet, "/api/admin/projects/" + projectID + "/discovery-review"},
		{http.MethodGet, "/api/admin/projects/" + projectID + "/discovery-review/" + candidateID},
		{http.MethodPost, "/api/admin/projects/" + projectID + "/discovery-review/" + candidateID + "/resolve"},
		{http.MethodGet, "/api/admin/projects/" + projectID + "/discovery-semantic-evaluation"},
		{http.MethodPost, "/api/admin/projects/" + projectID + "/discovery-semantic-evaluation/run"},
	} {
		request := httptest.NewRequest(tc.method, tc.path, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code == http.StatusNotFound || response.Code == http.StatusMethodNotAllowed {
			t.Fatalf("%s %s should be registered, got %d", tc.method, tc.path, response.Code)
		}
	}
}

func TestAdminDiscoveryReviewRoutesStayInternalAndExposeNoReserve(t *testing.T) {
	serverRaw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	server := string(serverRaw)
	for _, want := range []string{
		`r.Use(s.requireAdmin)`,
		`/admin/projects/{projectID}/discovery-arbitration/{candidateID}/prepare`,
		`/admin/projects/{projectID}/discovery-review`,
		`/admin/projects/{projectID}/discovery-review/{candidateID}`,
		`/admin/projects/{projectID}/discovery-review/{candidateID}/resolve`,
		`/admin/projects/{projectID}/discovery-semantic-evaluation`,
		`/admin/projects/{projectID}/discovery-semantic-evaluation/run`,
	} {
		if !strings.Contains(server, want) {
			t.Fatalf("admin discovery review contract missing %q", want)
		}
	}
	if strings.Contains(server, "discovery-arbitration/{candidateID}/reserve") {
		t.Fatal("Phase 1B must not expose reserve before a canonical Phase 2/3 WorkCreator exists")
	}
	handlerRaw, err := os.ReadFile("handlers_admin_discovery_review.go")
	if err != nil {
		t.Fatal(err)
	}
	handler := string(handlerRaw)
	for _, want := range []string{
		"discovery.NewArbitrationService", "discovery.NewReviewService", "discovery.NewSemanticEvaluationService",
		"discovery.ErrSnapshotStale", "http.StatusConflict", "http.StatusUnprocessableEntity", "http.StatusServiceUnavailable",
	} {
		if !strings.Contains(handler, want) {
			t.Fatalf("admin discovery review handler missing %q", want)
		}
	}
	for _, forbidden := range []string{"ReservePrepared", "WorkCreator", "InsertEnforcedWorkSignature"} {
		if strings.Contains(handler, forbidden) {
			t.Fatalf("Phase 1B admin API exposed work reservation via %q", forbidden)
		}
	}
}
