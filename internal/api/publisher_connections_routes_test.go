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
		{http.MethodDelete, "/api/projects/not-a-uuid/publisher-connections/not-a-connection"},
		{http.MethodPut, "/api/projects/not-a-uuid/publisher-connections/not-a-connection/enabled"},
		{http.MethodPost, "/api/projects/not-a-uuid/publisher-connections/not-a-connection/test"},
		{http.MethodPut, "/api/projects/not-a-uuid/publisher-connections/not-a-connection/credential"},
		{http.MethodDelete, "/api/projects/not-a-uuid/publisher-connections/not-a-connection/credential"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("%s %s status = %d, want %d", tc.method, tc.path, res.Code, http.StatusBadRequest)
		}
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
