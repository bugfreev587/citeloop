package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSEORoutesAreRegistered(t *testing.T) {
	router := (&Server{}).Router()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "overview", method: http.MethodGet, path: "/api/projects/not-a-uuid/seo/overview"},
		{name: "sync", method: http.MethodPost, path: "/api/projects/not-a-uuid/seo/sync"},
		{name: "analyze", method: http.MethodPost, path: "/api/projects/not-a-uuid/seo/analyze"},
		{name: "runs", method: http.MethodGet, path: "/api/projects/not-a-uuid/seo/runs"},
		{name: "opportunities", method: http.MethodGet, path: "/api/projects/not-a-uuid/seo/opportunities"},
		{name: "opportunity detail", method: http.MethodGet, path: "/api/projects/not-a-uuid/seo/opportunities/not-an-id"},
		{name: "accept", method: http.MethodPost, path: "/api/projects/not-a-uuid/seo/opportunities/not-an-id/accept"},
		{name: "dismiss", method: http.MethodPost, path: "/api/projects/not-a-uuid/seo/opportunities/not-an-id/dismiss"},
		{name: "create action", method: http.MethodPost, path: "/api/projects/not-a-uuid/seo/opportunities/not-an-id/actions"},
		{name: "actions", method: http.MethodGet, path: "/api/projects/not-a-uuid/seo/actions"},
		{name: "action detail", method: http.MethodGet, path: "/api/projects/not-a-uuid/seo/actions/not-an-id"},
		{name: "brief", method: http.MethodGet, path: "/api/projects/not-a-uuid/seo/briefs/latest"},
		{name: "settings", method: http.MethodGet, path: "/api/projects/not-a-uuid/seo/settings"},
		{name: "update settings", method: http.MethodPut, path: "/api/projects/not-a-uuid/seo/settings"},
		{name: "autopilot objectives", method: http.MethodGet, path: "/api/projects/not-a-uuid/seo/autopilot/objectives"},
		{name: "autopilot create objective", method: http.MethodPost, path: "/api/projects/not-a-uuid/seo/autopilot/objectives"},
		{name: "autopilot policy", method: http.MethodGet, path: "/api/projects/not-a-uuid/seo/autopilot/policy"},
		{name: "autopilot update policy", method: http.MethodPut, path: "/api/projects/not-a-uuid/seo/autopilot/policy"},
		{name: "autopilot generate plan", method: http.MethodPost, path: "/api/projects/not-a-uuid/seo/autopilot/plans/generate"},
		{name: "autopilot plans", method: http.MethodGet, path: "/api/projects/not-a-uuid/seo/autopilot/plans"},
		{name: "safe mode list", method: http.MethodGet, path: "/api/projects/not-a-uuid/seo/autopilot/safe-mode"},
		{name: "safe mode enter", method: http.MethodPost, path: "/api/projects/not-a-uuid/seo/autopilot/safe-mode"},
		{name: "safe mode exit", method: http.MethodPost, path: "/api/projects/not-a-uuid/seo/autopilot/safe-mode/not-an-id/exit"},
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
