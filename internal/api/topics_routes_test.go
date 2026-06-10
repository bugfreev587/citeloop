package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestTopicMutationRoutesAreRegistered(t *testing.T) {
	router := (&Server{}).Router()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "update topic",
			method: http.MethodPut,
			path:   "/api/projects/not-a-uuid/topics/not-a-topic",
		},
		{
			name:   "schedule topic",
			method: http.MethodPost,
			path:   "/api/projects/not-a-uuid/topics/not-a-topic/schedule",
		},
		{
			name:   "archive topic",
			method: http.MethodPost,
			path:   "/api/projects/not-a-uuid/topics/not-a-topic/archive",
		},
		{
			name:   "restore topic",
			method: http.MethodPost,
			path:   "/api/projects/not-a-uuid/topics/not-a-topic/restore",
		},
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

func TestGenerateTopicRouteStartsBackgroundGeneration(t *testing.T) {
	source, err := os.ReadFile("handlers_agents.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)

	if !strings.Contains(body, "http.StatusAccepted") {
		t.Fatal("generate topic should accept background generation without holding the HTTP request open")
	}
	if !strings.Contains(body, "startTopicGeneration") {
		t.Fatal("generate topic should dispatch writer and QA work outside the request path")
	}
	if strings.Contains(body, "arts, err := ag.Generate(r.Context(), id, topic)") {
		t.Fatal("generate topic must not run Writer+QA synchronously on the request context")
	}
}
