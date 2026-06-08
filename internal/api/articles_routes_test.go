package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProjectScopedArticleMutationRoutesAreRegistered(t *testing.T) {
	router := (&Server{}).Router()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "get article", method: http.MethodGet, path: "/api/projects/not-a-uuid/articles/not-an-article"},
		{name: "edit article", method: http.MethodPut, path: "/api/projects/not-a-uuid/articles/not-an-article"},
		{name: "ai fix article", method: http.MethodPost, path: "/api/projects/not-a-uuid/articles/not-an-article/ai-fix"},
		{name: "approve article", method: http.MethodPost, path: "/api/projects/not-a-uuid/articles/not-an-article/approve"},
		{name: "reject article", method: http.MethodPost, path: "/api/projects/not-a-uuid/articles/not-an-article/reject"},
		{name: "mark distributed", method: http.MethodPost, path: "/api/projects/not-a-uuid/articles/not-an-article/distributed"},
		{name: "retry publish", method: http.MethodPost, path: "/api/projects/not-a-uuid/articles/not-an-article/retry-publish"},
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

func TestFlatArticleMutationRoutesAreNotRegistered(t *testing.T) {
	router := (&Server{}).Router()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "edit article", method: http.MethodPut, path: "/api/articles/not-an-article"},
		{name: "approve article", method: http.MethodPost, path: "/api/articles/not-an-article/approve"},
		{name: "ai fix article", method: http.MethodPost, path: "/api/articles/not-an-article/ai-fix"},
		{name: "reject article", method: http.MethodPost, path: "/api/articles/not-an-article/reject"},
		{name: "mark distributed", method: http.MethodPost, path: "/api/articles/not-an-article/distributed"},
		{name: "retry publish", method: http.MethodPost, path: "/api/articles/not-an-article/retry-publish"},
		{name: "get article", method: http.MethodGet, path: "/api/articles/not-an-article"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)
			if res.Code != http.StatusNotFound {
				t.Fatalf("%s status = %d, want %d", tt.name, res.Code, http.StatusNotFound)
			}
		})
	}
}

func TestPublishingReconcileRouteIsRegistered(t *testing.T) {
	router := (&Server{}).Router()

	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "reconcile", path: "/api/projects/not-a-uuid/publishing/reconcile"},
		{name: "publish tick", path: "/api/projects/not-a-uuid/tick/publish"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, nil)
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)
			if res.Code != http.StatusBadRequest {
				t.Fatalf("%s status = %d, want %d", tc.name, res.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestProjectActivityRouteIsRegistered(t *testing.T) {
	router := (&Server{}).Router()

	req := httptest.NewRequest(http.MethodGet, "/api/projects/not-a-uuid/activity", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("activity status = %d, want %d", res.Code, http.StatusBadRequest)
	}
}
