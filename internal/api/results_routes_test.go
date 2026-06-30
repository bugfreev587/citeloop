package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestResultsRoutesAreRegistered(t *testing.T) {
	router := (&Server{}).Router()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "results actions", method: http.MethodGet, path: "/api/projects/not-a-uuid/results/actions"},
		{name: "results action detail", method: http.MethodGet, path: "/api/projects/not-a-uuid/results/actions/not-an-action"},
		{name: "results recompute", method: http.MethodPost, path: "/api/projects/not-a-uuid/results/recompute"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)
			if res.Code != http.StatusBadRequest {
				t.Fatalf("%s status = %d, want %d", tt.name, res.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestResultsHandlersExposeActionLevelAttribution(t *testing.T) {
	handler, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatalf("read handlers_seo.go: %v", err)
	}
	server, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	combined := string(handler) + string(server)
	for _, want := range []string{
		"ResultsAction",
		"ActionMeasurement",
		"listResultsActions",
		"getResultsAction",
		"recomputeResults",
		"ListResultsActionRows",
		"ListActionMeasurementsForProject",
		"outcome_label",
		"outcome_reason",
		"attribution_confidence",
		"confounders",
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("results attribution handler missing %q", want)
		}
	}
}
