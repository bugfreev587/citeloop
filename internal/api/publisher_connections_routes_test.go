package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/githubapp"
	"github.com/citeloop/citeloop/internal/publisher"
	"github.com/google/uuid"
)

func TestPublisherConnectionRoutesAreRegistered(t *testing.T) {
	router := (&Server{}).Router()

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/projects/not-a-uuid/publisher-connections"},
		{http.MethodPut, "/api/projects/not-a-uuid/publisher-connections/github-nextjs"},
		{http.MethodPut, "/api/projects/not-a-uuid/publisher-connections/dev-to"},
		{http.MethodDelete, "/api/projects/not-a-uuid/publisher-connections/not-a-connection"},
		{http.MethodPut, "/api/projects/not-a-uuid/publisher-connections/not-a-connection/enabled"},
		{http.MethodPost, "/api/projects/not-a-uuid/publisher-connections/not-a-connection/test"},
		{http.MethodPut, "/api/projects/not-a-uuid/publisher-connections/not-a-connection/credential"},
		{http.MethodDelete, "/api/projects/not-a-uuid/publisher-connections/not-a-connection/credential"},
		{http.MethodGet, "/api/projects/not-a-uuid/integrations/github/pr-readiness"},
		{http.MethodPost, "/api/projects/not-a-uuid/integrations/github/pr-readiness/check"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("%s %s status = %d, want %d", tc.method, tc.path, res.Code, http.StatusBadRequest)
		}
	}
}

func TestDevToUpsertUsesSafeDefaults(t *testing.T) {
	cfgRaw, status, err := devToConfigForUpsert(devToPublisherInput{
		Username: " citeloop ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if status != "missing" {
		t.Fatalf("status = %q, want missing until credential is saved", status)
	}
	var cfg map[string]string
	if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["username"] != "citeloop" {
		t.Fatalf("username = %q", cfg["username"])
	}
	if strings.Contains(string(cfgRaw), "api_key") || strings.Contains(string(cfgRaw), "token") {
		t.Fatalf("dev.to config leaked secret-like fields: %s", string(cfgRaw))
	}
}

func TestPublisherCredentialKindMatchesConnectionKind(t *testing.T) {
	for _, tc := range []struct {
		name string
		conn db.PublisherConnection
		in   string
		want string
	}{
		{
			name: "github default",
			conn: db.PublisherConnection{Kind: publisher.ConnectionKindGitHubNextJS},
			in:   "",
			want: publisher.CredentialKindGitHubToken,
		},
		{
			name: "dev.to default",
			conn: db.PublisherConnection{Kind: publisher.ConnectionKindDevTo},
			in:   "",
			want: publisher.CredentialKindDevToAPIKey,
		},
		{
			name: "dev.to explicit",
			conn: db.PublisherConnection{Kind: publisher.ConnectionKindDevTo},
			in:   publisher.CredentialKindDevToAPIKey,
			want: publisher.CredentialKindDevToAPIKey,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := publisherCredentialKindForConnection(tc.conn, tc.in)
			if err != nil {
				t.Fatalf("publisherCredentialKindForConnection returned error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("kind = %q, want %q", got, tc.want)
			}
		})
	}

	if _, err := publisherCredentialKindForConnection(
		db.PublisherConnection{Kind: publisher.ConnectionKindGitHubNextJS},
		publisher.CredentialKindDevToAPIKey,
	); err == nil {
		t.Fatal("expected dev.to API key to be rejected for GitHub connections")
	}
}

func TestDevToAPIKeyVerificationUsesAPIKeyHeader(t *testing.T) {
	var gotAPIKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("api-key")
		if r.URL.Path != "/api/users/me" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"username":"citeloop"}`))
	}))
	defer srv.Close()

	profile, err := verifyDevToAPIKey(context.Background(), srv.Client(), srv.URL, "dev-secret")
	if err != nil {
		t.Fatal(err)
	}
	if gotAPIKey != "dev-secret" {
		t.Fatalf("api-key header = %q", gotAPIKey)
	}
	if profile.Username != "citeloop" {
		t.Fatalf("username = %q", profile.Username)
	}
}

func TestPublisherCredentialRoutesAreRegistered(t *testing.T) {
	router := (&Server{}).Router()
	projectID := uuid.New().String()

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodDelete, "/api/projects/" + projectID + "/publisher-connections/not-a-connection"},
		{http.MethodPut, "/api/projects/" + projectID + "/publisher-connections/not-a-connection/enabled"},
		{http.MethodPut, "/api/projects/" + projectID + "/publisher-connections/not-a-connection/credential"},
		{http.MethodDelete, "/api/projects/" + projectID + "/publisher-connections/not-a-connection/credential"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("%s %s status = %d, want %d", tc.method, tc.path, res.Code, http.StatusBadRequest)
		}
	}
}

func TestRejectPublisherConnectionSecretFields(t *testing.T) {
	for _, raw := range []string{
		`{"repo":"owner/site","base_url":"https://example.com/blog","token":"ghp_secret"}`,
		`{"repo":"owner/site","base_url":"https://example.com/blog","webhook_url":"https://hooks.example"}`,
		`{"repo":"owner/site","base_url":"https://example.com/blog","webhookUrl":"https://hooks.example"}`,
		`{"repo":"owner/site","base_url":"https://example.com/blog","api_key":"secret"}`,
		`{"repo":"owner/site","base_url":"https://example.com/blog","nested":{"password":"secret"}}`,
	} {
		if err := rejectPublisherConnectionSecrets([]byte(raw)); err == nil {
			t.Fatalf("expected secret-like field to be rejected for %s", raw)
		}
	}

	raw := `{"repo":"owner/site","base_url":"https://example.com/blog","credential_ref":"publisher_credential:1b9d6bcd-bbfd-4b2d-9b5d-ab8dfbbd4bed"}`
	if err := rejectPublisherConnectionSecrets([]byte(raw)); err != nil {
		t.Fatalf("credential_ref should be allowed: %v", err)
	}

	envRef := `{"repo":"owner/site","base_url":"https://example.com/blog","credential_ref":"env:GITHUB_TOKEN"}`
	if err := rejectPublisherConnectionSecrets([]byte(envRef)); err == nil {
		t.Fatal("expected env credential_ref fallback to be rejected")
	}
}

func TestGitHubNextJSUpsertPreservesGitHubAppInstallation(t *testing.T) {
	cfgRaw, status, err := githubNextJSConfigForUpsert(githubNextJSPublisherInput{
		Repo:        "owner/site",
		Branch:      "main",
		ContentDir:  "content/blog",
		BaseURL:     "https://example.com/blog",
		PublishMode: "publish",
	}, json.RawMessage(`{"installation_id":"12345","repo":"old/site","base_url":"https://old.example/blog"}`))
	if err != nil {
		t.Fatal(err)
	}
	if status != "connected" {
		t.Fatalf("status = %q, want connected", status)
	}
	cfg := parseGithubConnConfig(cfgRaw)
	if cfg.InstallationID != "12345" {
		t.Fatalf("installation_id = %q, want preserved", cfg.InstallationID)
	}
	if cfg.Repo != "owner/site" || cfg.Branch != "main" || cfg.ContentDir != "content/blog" || cfg.BaseURL != "https://example.com/blog" {
		t.Fatalf("target fields were not updated: %#v", cfg)
	}
}

func TestPublisherConnectionTokenUsesGitHubAppInstallation(t *testing.T) {
	app := &fakeGitHubAppClient{configured: true, token: "installation-token"}
	s := &Server{githubAppClient: app}
	projectID := uuid.New()
	conn := db.PublisherConnection{
		ID:        uuid.New(),
		ProjectID: projectID,
		Kind:      publisher.ConnectionKindGitHubNextJS,
		Status:    "connected",
		Enabled:   true,
		Config: json.RawMessage(`{
			"installation_id":"12345",
			"repo":"owner/site",
			"branch":"main",
			"base_url":"https://example.com/blog"
		}`),
	}

	token, err := s.publisherConnectionToken(context.Background(), projectID, conn)
	if err != nil {
		t.Fatalf("publisherConnectionToken returned error: %v", err)
	}
	if token != "installation-token" {
		t.Fatalf("token = %q, want installation-token", token)
	}
	if app.gotInstallationID != "12345" {
		t.Fatalf("installation id = %q, want 12345", app.gotInstallationID)
	}
}

func TestPublisherCredentialTokenRejectsEnvFallback(t *testing.T) {
	s := &Server{Env: config.Env{GitHubToken: "fallback-token"}}
	conn := db.PublisherConnection{
		ID:            uuid.New(),
		ProjectID:     uuid.New(),
		Kind:          publisher.ConnectionKindGitHubNextJS,
		Status:        "connected",
		Enabled:       true,
		CredentialRef: strPtr("env:GITHUB_TOKEN"),
	}

	token, err := s.publisherCredentialToken(context.Background(), conn.ProjectID, conn)
	if err == nil || !strings.Contains(err.Error(), "project-scoped publisher credential") {
		t.Fatalf("expected env fallback rejection, token=%q err=%v", token, err)
	}
}

func TestPublisherConnectionResponseRedactsCredentialRefAndKeepsCapabilities(t *testing.T) {
	credentialID := uuid.New()
	connection := db.PublisherConnection{
		ID:            uuid.New(),
		ProjectID:     uuid.New(),
		Kind:          publisher.ConnectionKindGitHubNextJS,
		Label:         "GitHub",
		Status:        "connected",
		Enabled:       true,
		IsDefault:     true,
		Capabilities:  publisher.GitHubNextJSCapabilities().JSON(),
		CredentialRef: strPtr(publisher.PublisherCredentialRef(credentialID)),
		Config:        json.RawMessage(`{"repo":"owner/site","base_url":"https://example.com/blog"}`),
	}

	body, err := json.Marshal(publisherConnectionResponse(connection))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), credentialID.String()) {
		t.Fatalf("response leaked credential ref: %s", string(body))
	}
	if !strings.Contains(string(body), `"create_article":true`) {
		t.Fatalf("response missing create_article capability: %s", string(body))
	}
	if !strings.Contains(string(body), `"enabled":true`) {
		t.Fatalf("response missing enabled flag: %s", string(body))
	}
	if strings.Contains(string(body), "token") {
		t.Fatalf("response leaked token-like text: %s", string(body))
	}
}

type fakeGitHubAppClient struct {
	configured        bool
	token             string
	access            githubapp.InstallationAccess
	err               error
	gotInstallationID string
	calls             []string
}

func (f *fakeGitHubAppClient) Configured() bool {
	f.calls = append(f.calls, "Configured")
	return f.configured
}

func (f *fakeGitHubAppClient) InstallURL(string) string {
	f.calls = append(f.calls, "InstallURL")
	return ""
}

func (f *fakeGitHubAppClient) InstallationToken(_ context.Context, installationID string) (string, error) {
	f.calls = append(f.calls, "InstallationToken")
	f.gotInstallationID = installationID
	return f.token, f.err
}

func (f *fakeGitHubAppClient) InstallationAccess(_ context.Context, installationID string) (githubapp.InstallationAccess, error) {
	f.calls = append(f.calls, "InstallationAccess")
	f.gotInstallationID = installationID
	return f.access, f.err
}

func (f *fakeGitHubAppClient) ListRepos(context.Context, string) ([]githubapp.Repo, error) {
	f.calls = append(f.calls, "ListRepos")
	return nil, f.err
}
