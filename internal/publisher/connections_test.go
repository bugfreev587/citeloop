package publisher

import (
	"encoding/json"
	"testing"
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
		"base_url":" https://dev.unipost.dev/blog/ "
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
	if cfg.BaseURL != "https://dev.unipost.dev/blog" {
		t.Fatalf("base url = %q", cfg.BaseURL)
	}
	if cfg.PublishMode != "publish" {
		t.Fatalf("publish mode = %q", cfg.PublishMode)
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
