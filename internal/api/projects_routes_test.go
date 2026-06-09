package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProjectLifecycleRoutesAreRegistered(t *testing.T) {
	router := (&Server{}).Router()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "archive", method: http.MethodPost, path: "/api/projects/not-a-uuid/archive"},
		{name: "restore", method: http.MethodPost, path: "/api/projects/not-a-uuid/restore"},
		{name: "delete", method: http.MethodDelete, path: "/api/projects/not-a-uuid/"},
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

func TestProjectStatusFilterRejectsUnknownStatus(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/projects?status=deleted", nil)
	res := httptest.NewRecorder()

	status, ok := projectStatusFilter(res, req)
	if ok || status != "" {
		t.Fatalf("status filter = (%q, %v), want rejected", status, ok)
	}
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusBadRequest)
	}
}
