package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdminGEOCredentialRoutesAreRegistered(t *testing.T) {
	router := (&Server{}).Router()

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/admin/geo-credentials"},
		{http.MethodPut, "/api/admin/geo-credentials/perplexity"},
		{http.MethodPost, "/api/admin/geo-credentials/perplexity/test"},
		{http.MethodDelete, "/api/admin/geo-credentials/perplexity"},
		{http.MethodPut, "/api/admin/geo-credentials/openai"},
		{http.MethodPut, "/api/admin/geo-credentials/anthropic"},
		{http.MethodPut, "/api/admin/geo-credentials/gemini"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		res := httptest.NewRecorder()

		router.ServeHTTP(res, req)

		if res.Code == http.StatusNotFound || res.Code == http.StatusMethodNotAllowed {
			t.Fatalf("%s %s should be registered, got %d", tc.method, tc.path, res.Code)
		}
	}
}
