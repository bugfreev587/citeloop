package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/citeloop/citeloop/internal/config"
)

func TestClerkAuthProtectsAPIRoutesWhenConfigured(t *testing.T) {
	srv := &Server{Env: config.Env{ClerkSecretKey: "sk_test_fake"}}
	router := srv.Router()

	apiReq := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	apiRes := httptest.NewRecorder()
	router.ServeHTTP(apiRes, apiReq)
	if apiRes.Code != http.StatusForbidden {
		t.Fatalf("/api/projects status = %d, want %d", apiRes.Code, http.StatusForbidden)
	}

	healthReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthRes := httptest.NewRecorder()
	router.ServeHTTP(healthRes, healthReq)
	if healthRes.Code != http.StatusOK {
		t.Fatalf("/healthz status = %d, want %d", healthRes.Code, http.StatusOK)
	}
}
