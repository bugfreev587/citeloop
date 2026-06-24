package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/googledata"
	"github.com/google/uuid"
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
		{name: "gsc connection", method: http.MethodGet, path: "/api/projects/not-a-uuid/seo/gsc/connection"},
		{name: "gsc oauth start", method: http.MethodPost, path: "/api/projects/not-a-uuid/seo/gsc/oauth/start"},
		{name: "gsc oauth complete", method: http.MethodPost, path: "/api/projects/not-a-uuid/seo/gsc/oauth/complete"},
		{name: "gsc property select", method: http.MethodPost, path: "/api/projects/not-a-uuid/seo/gsc/property"},
		{name: "gsc revoke", method: http.MethodPost, path: "/api/projects/not-a-uuid/seo/gsc/revoke"},
		{name: "autopilot objectives", method: http.MethodGet, path: "/api/projects/not-a-uuid/seo/autopilot/objectives"},
		{name: "autopilot create objective", method: http.MethodPost, path: "/api/projects/not-a-uuid/seo/autopilot/objectives"},
		{name: "autopilot policy", method: http.MethodGet, path: "/api/projects/not-a-uuid/seo/autopilot/policy"},
		{name: "autopilot update policy", method: http.MethodPut, path: "/api/projects/not-a-uuid/seo/autopilot/policy"},
		{name: "autopilot generate plan", method: http.MethodPost, path: "/api/projects/not-a-uuid/seo/autopilot/plans/generate"},
		{name: "autopilot plans", method: http.MethodGet, path: "/api/projects/not-a-uuid/seo/autopilot/plans"},
		{name: "safe mode list", method: http.MethodGet, path: "/api/projects/not-a-uuid/seo/autopilot/safe-mode"},
		{name: "safe mode enter", method: http.MethodPost, path: "/api/projects/not-a-uuid/seo/autopilot/safe-mode"},
		{name: "safe mode exit", method: http.MethodPost, path: "/api/projects/not-a-uuid/seo/autopilot/safe-mode/not-an-id/exit"},
		{name: "geo crawler audit", method: http.MethodPost, path: "/api/projects/not-a-uuid/geo/crawler-audit"},
		{name: "geo crawler latest", method: http.MethodGet, path: "/api/projects/not-a-uuid/geo/crawler-audit/latest"},
		{name: "geo observe provider", method: http.MethodPost, path: "/api/projects/not-a-uuid/geo/runs/observe-provider"},
		{name: "geo runs", method: http.MethodGet, path: "/api/projects/not-a-uuid/geo/runs"},
		{name: "geo surface monitor", method: http.MethodPost, path: "/api/projects/not-a-uuid/geo/external-surfaces/monitor"},
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

func TestStartGSCOAuthReturnsGoogleAuthorizationURL(t *testing.T) {
	projectID := uuid.New()
	srv := &Server{Env: config.Env{
		GoogleOAuthClientID:     "google-client-id",
		GoogleOAuthClientSecret: "google-client-secret",
		PublicAppURL:            "https://app.citeloop.test",
		NotificationSecretKey:   "state-secret",
	}}
	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/seo/gsc/oauth/start", nil)
	res := httptest.NewRecorder()

	srv.Router().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var out struct {
		AuthorizationURL string `json:"authorization_url"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.AuthorizationURL, "accounts.google.com") {
		t.Fatalf("authorization_url = %q", out.AuthorizationURL)
	}
	parsed, err := url.Parse(out.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("scope") != googledata.ScopeSearchConsoleReadonly {
		t.Fatalf("scope = %q, want %q", parsed.Query().Get("scope"), googledata.ScopeSearchConsoleReadonly)
	}
	if parsed.Query().Get("redirect_uri") != "https://app.citeloop.test/integrations/google/search-console/callback" {
		t.Fatalf("redirect_uri = %q", parsed.Query().Get("redirect_uri"))
	}
	if parsed.Query().Get("state") == "" {
		t.Fatalf("authorization_url missing state: %q", out.AuthorizationURL)
	}
	if strings.Contains(out.AuthorizationURL, "client_secret") {
		t.Fatalf("authorization_url leaked client_secret: %q", out.AuthorizationURL)
	}
}

func TestGEORoutesAreRegisteredForValidProject(t *testing.T) {
	router := (&Server{}).Router()
	projectID := uuid.New().String()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "crawler audit", method: http.MethodPost, path: "/api/projects/" + projectID + "/geo/crawler-audit"},
		{name: "crawler latest", method: http.MethodGet, path: "/api/projects/" + projectID + "/geo/crawler-audit/latest"},
		{name: "geo overview", method: http.MethodGet, path: "/api/projects/" + projectID + "/geo/overview"},
		{name: "generate prompt set", method: http.MethodPost, path: "/api/projects/" + projectID + "/geo/prompt-sets/generate"},
		{name: "prompt sets", method: http.MethodGet, path: "/api/projects/" + projectID + "/geo/prompt-sets"},
		{name: "update prompt set", method: http.MethodPut, path: "/api/projects/" + projectID + "/geo/prompt-sets/" + uuid.New().String()},
		{name: "update geo prompt", method: http.MethodPut, path: "/api/projects/" + projectID + "/geo/prompts/" + uuid.New().String()},
		{name: "update geo competitor", method: http.MethodPut, path: "/api/projects/" + projectID + "/geo/competitors/" + uuid.New().String()},
		{name: "observe fixtures", method: http.MethodPost, path: "/api/projects/" + projectID + "/geo/runs/observe"},
		{name: "observe provider", method: http.MethodPost, path: "/api/projects/" + projectID + "/geo/runs/observe-provider"},
		{name: "geo runs", method: http.MethodGet, path: "/api/projects/" + projectID + "/geo/runs"},
		{name: "observations", method: http.MethodGet, path: "/api/projects/" + projectID + "/geo/observations"},
		{name: "external surfaces", method: http.MethodGet, path: "/api/projects/" + projectID + "/geo/external-surfaces"},
		{name: "create external surface", method: http.MethodPost, path: "/api/projects/" + projectID + "/geo/external-surfaces"},
		{name: "monitor external surfaces", method: http.MethodPost, path: "/api/projects/" + projectID + "/geo/external-surfaces/monitor"},
		{name: "analyze geo opportunities", method: http.MethodPost, path: "/api/projects/" + projectID + "/geo/opportunities/analyze"},
		{name: "asset briefs", method: http.MethodGet, path: "/api/projects/" + projectID + "/geo/asset-briefs"},
		{name: "accept asset brief", method: http.MethodPost, path: "/api/projects/" + projectID + "/geo/asset-briefs/" + uuid.New().String() + "/accept"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)
			if res.Code == http.StatusNotFound {
				t.Fatalf("%s status = %d, want route registered", tt.name, res.Code)
			}
		})
	}
}
