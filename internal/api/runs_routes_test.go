package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

func TestGenerationRunCursorParamOmitsZeroTime(t *testing.T) {
	if got := generationRunCursorParam(time.Time{}); got.Valid {
		t.Fatal("zero cursor must be encoded as SQL null")
	}
	nonZero := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	if got := generationRunCursorParam(nonZero); !got.Valid || !got.Time.Equal(nonZero) {
		t.Fatalf("non-zero cursor = %+v, want valid %s", got, nonZero)
	}
}
