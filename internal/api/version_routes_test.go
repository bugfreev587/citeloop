package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/citeloop/citeloop/internal/config"
)

func TestVersionRoutesArePublicAndMachineReadable(t *testing.T) {
	srv := &Server{Env: config.Env{ClerkSecretKey: "sk_test_fake"}}
	router := srv.Router()

	for _, path := range []string{"/healthz", "/api/meta/version"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d", path, res.Code, http.StatusOK)
		}

		var body struct {
			Status string `json:"status"`
			Build  struct {
				Service string `json:"service"`
			} `json:"build"`
			Database struct {
				MigrationStatus string `json:"migration_status"`
			} `json:"database"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s did not return JSON: %v", path, err)
		}
		if body.Status != "ok" || body.Build.Service != "citeloop-api" {
			t.Fatalf("%s body = %+v, want service metadata", path, body)
		}
		if body.Database.MigrationStatus == "" {
			t.Fatalf("%s missing database migration status: %+v", path, body)
		}
	}
}
