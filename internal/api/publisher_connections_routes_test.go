package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
		{http.MethodPost, "/api/projects/not-a-uuid/publisher-connections/not-a-connection/test"},
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

	raw := `{"repo":"owner/site","base_url":"https://example.com/blog","credential_ref":"env:GITHUB_TOKEN"}`
	if err := rejectPublisherConnectionSecrets([]byte(raw)); err != nil {
		t.Fatalf("credential_ref should be allowed: %v", err)
	}
}

func TestPublisherConnectionResponseRedactsCredentialRefAndKeepsCapabilities(t *testing.T) {
	connection := db.PublisherConnection{
		ID:            uuid.New(),
		ProjectID:     uuid.New(),
		Kind:          publisher.ConnectionKindGitHubNextJS,
		Label:         "GitHub",
		Status:        "connected",
		IsDefault:     true,
		Capabilities:  publisher.GitHubNextJSCapabilities().JSON(),
		CredentialRef: strPtr("env:GITHUB_TOKEN"),
		Config:        json.RawMessage(`{"repo":"owner/site","base_url":"https://example.com/blog"}`),
	}

	body, err := json.Marshal(publisherConnectionResponse(connection))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "env:GITHUB_TOKEN") {
		t.Fatalf("response leaked credential ref: %s", string(body))
	}
	if !strings.Contains(string(body), `"create_article":true`) {
		t.Fatalf("response missing create_article capability: %s", string(body))
	}
	if strings.Contains(string(body), "token") {
		t.Fatalf("response leaked token-like text: %s", string(body))
	}
}
