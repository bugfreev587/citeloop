package api

import (
	"net/http"
	"net/http/httptest"
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
