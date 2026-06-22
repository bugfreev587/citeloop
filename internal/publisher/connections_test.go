package publisher

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestGitHubNextJSCapabilitiesExposeSafePublishSurface(t *testing.T) {
	caps := GitHubNextJSCapabilities()

	for _, capability := range []string{
		CapabilityCreateArticle,
		CapabilityUpdateArticle,
		CapabilityMetadataUpdate,
		CapabilityCanonical,
		CapabilityPublishMode,
	} {
		if !caps[capability] {
			t.Fatalf("expected %s capability to be enabled", capability)
		}
	}
	if caps[CapabilityDraftMode] {
		t.Fatal("github_nextjs should not claim draft mode support")
	}
	if caps[CapabilityRollback] {
		t.Fatal("github_nextjs should not claim rollback support")
	}
}

func TestParseGitHubNextJSConfigNormalizesDefaults(t *testing.T) {
	raw := json.RawMessage(`{
		"repo":" owner/unipost ",
		"base_url":" https://customer.example/blog/ "
	}`)

	cfg, err := ParseGitHubNextJSConfig(raw)
	if err != nil {
		t.Fatalf("ParseGitHubNextJSConfig returned error: %v", err)
	}
	if cfg.Repo != "owner/unipost" {
		t.Fatalf("repo = %q", cfg.Repo)
	}
	if cfg.Branch != "citeloop-content" {
		t.Fatalf("branch = %q", cfg.Branch)
	}
	if cfg.ContentDir != "content/citeloop/blog" {
		t.Fatalf("content dir = %q", cfg.ContentDir)
	}
	if cfg.BaseURL != "https://customer.example/blog" {
		t.Fatalf("base url = %q", cfg.BaseURL)
	}
	if cfg.PublishMode != "publish" {
		t.Fatalf("publish mode = %q", cfg.PublishMode)
	}
}

func TestParseGitHubNextJSConfigDerivesUniPostBranchFromBaseURL(t *testing.T) {
	for _, tt := range []struct {
		name    string
		baseURL string
		branch  string
		wantURL string
	}{
		{
			name:    "development",
			baseURL: " https://dev.unipost.dev/blog/ ",
			branch:  "dev",
			wantURL: "https://dev.unipost.dev/blog",
		},
		{
			name:    "staging",
			baseURL: "https://staging.unipost.dev/blog",
			branch:  "staging",
			wantURL: "https://staging.unipost.dev/blog",
		},
		{
			name:    "production",
			baseURL: "https://unipost.dev/blog",
			branch:  "main",
			wantURL: "https://unipost.dev/blog",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			raw := json.RawMessage(`{
				"repo":"owner/unipost",
				"base_url":` + strconv.Quote(tt.baseURL) + `
			}`)

			cfg, err := ParseGitHubNextJSConfig(raw)
			if err != nil {
				t.Fatalf("ParseGitHubNextJSConfig returned error: %v", err)
			}
			if cfg.Branch != tt.branch {
				t.Fatalf("branch = %q, want %q", cfg.Branch, tt.branch)
			}
			if cfg.BaseURL != tt.wantURL {
				t.Fatalf("base url = %q, want %q", cfg.BaseURL, tt.wantURL)
			}
		})
	}
}

func TestParseGitHubNextJSConfigKeepsCustomerDomains(t *testing.T) {
	raw := json.RawMessage(`{
		"repo":"owner/customer-site",
		"base_url":" https://dev.customer.example/blog/ "
	}`)

	cfg, err := ParseGitHubNextJSConfig(raw)
	if err != nil {
		t.Fatalf("ParseGitHubNextJSConfig returned error: %v", err)
	}
	if cfg.BaseURL != "https://dev.customer.example/blog" {
		t.Fatalf("base url = %q", cfg.BaseURL)
	}
}

func TestParseGitHubNextJSConfigRejectsMissingRequiredFields(t *testing.T) {
	_, err := ParseGitHubNextJSConfig(json.RawMessage(`{"repo":"owner/unipost"}`))
	if err == nil {
		t.Fatal("expected missing base_url to fail")
	}

	_, err = ParseGitHubNextJSConfig(json.RawMessage(`{"base_url":"https://example.com/blog"}`))
	if err == nil {
		t.Fatal("expected missing repo to fail")
	}
}

func TestPublisherCredentialRefAndRedaction(t *testing.T) {
	id := uuid.New()
	ref := PublisherCredentialRef(id)
	if ref != "publisher_credential:"+id.String() {
		t.Fatalf("ref = %q", ref)
	}
	parsed, ok := ParsePublisherCredentialRef(ref)
	if !ok || parsed != id {
		t.Fatalf("parsed ref = %s, ok=%v", parsed, ok)
	}

	redacted := RedactCredentialValue(CredentialKindGitHubToken, "ghp_abcdefghijklmnopqrstuvwxyz")
	if strings.Contains(redacted, "abcdefghijklmnopqrstuv") {
		t.Fatalf("redaction leaked too much secret: %s", redacted)
	}
	if !strings.HasSuffix(redacted, "wxyz") {
		t.Fatalf("redaction should preserve tail for recognition: %s", redacted)
	}
}
