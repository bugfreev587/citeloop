package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRunsRoutesAreRegistered(t *testing.T) {
	router := (&Server{}).Router()

	req := httptest.NewRequest(http.MethodGet, "/api/projects/not-a-uuid/runs", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("list runs status = %d, want %d", res.Code, http.StatusBadRequest)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/projects/not-a-uuid/runs/not-a-run", nil)
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("get run status = %d, want %d", res.Code, http.StatusBadRequest)
	}
}
