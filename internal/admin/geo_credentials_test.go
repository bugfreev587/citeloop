package admin

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestGEOCredentialScopesIncludePhaseTwoProviders(t *testing.T) {
	want := []GEOProviderScope{
		GEOProviderPerplexity,
		GEOProviderOpenAI,
		GEOProviderAnthropic,
		GEOProviderGemini,
	}

	if got := GEOCredentialScopes(); len(got) != len(want) {
		t.Fatalf("scope count = %d, want %d (%v)", len(got), len(want), want)
	}
	for _, scope := range want {
		if !ValidGEOCredentialScope(string(scope)) {
			t.Fatalf("scope %q should be valid", scope)
		}
	}
	if ValidGEOCredentialScope("native_perplexity") {
		t.Fatal("native provider keys must not be accepted; CiteLoop stores TokenGate-issued keys only")
	}
}

func TestGEOStatusFromCredentialMasksTokenGateKey(t *testing.T) {
	updatedAt := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	status := GEOStatusFromCredential(&GEOCredentials{
		Scope:     GEOProviderPerplexity,
		Provider:  ProviderTokenGate,
		APIKey:    "tg-perplexity-secret-abcdef",
		BaseURL:   "https://tokengate.example/v1",
		Model:     "sonar-pro",
		Enabled:   true,
		UpdatedAt: updatedAt,
	})

	if status.Provider != "tokengate" {
		t.Fatalf("provider = %q, want tokengate", status.Provider)
	}
	if status.Scope != "perplexity" || !status.Configured || !status.Enabled {
		t.Fatalf("status = %+v, want enabled configured perplexity", status)
	}
	if status.KeyTail != "cdef" {
		t.Fatalf("key tail = %q", status.KeyTail)
	}
	if status.BaseURL != "https://tokengate.example/v1" || status.Model != "sonar-pro" {
		t.Fatalf("base/model = %q/%q", status.BaseURL, status.Model)
	}
	raw, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "tg-perplexity-secret") {
		t.Fatalf("status leaked raw API key: %s", raw)
	}
}

func TestApplyGEOUpdateForcesTokenGateAndPreservesExistingKey(t *testing.T) {
	next, err := ApplyGEOUpdate(&GEOCredentials{
		Scope:    GEOProviderGemini,
		Provider: ProviderTokenGate,
		APIKey:   "tg-existing-gemini-key",
		BaseURL:  "https://tokengate.example/v1",
		Model:    "gemini-2.5-pro",
		Enabled:  true,
	}, GEOUpdateInput{
		Provider: "gemini",
		APIKey:   "   ",
		BaseURL:  "   ",
		Model:    " gemini-2.5-flash ",
		Enabled:  boolPtr(false),
	})
	if err != nil {
		t.Fatal(err)
	}

	if next.Provider != ProviderTokenGate {
		t.Fatalf("provider = %q, want tokengate", next.Provider)
	}
	if next.APIKey != "tg-existing-gemini-key" {
		t.Fatalf("api key = %q", next.APIKey)
	}
	if next.BaseURL != "https://tokengate.example/v1" {
		t.Fatalf("base url = %q", next.BaseURL)
	}
	if next.Model != "gemini-2.5-flash" {
		t.Fatalf("model = %q", next.Model)
	}
	if next.Enabled {
		t.Fatal("enabled should follow explicit false")
	}
}

func TestApplyGEOUpdateRequiresKeyForNewProvider(t *testing.T) {
	_, err := ApplyGEOUpdate(&GEOCredentials{
		Scope:    GEOProviderOpenAI,
		Provider: ProviderTokenGate,
		Enabled:  true,
	}, GEOUpdateInput{
		APIKey:  "  ",
		BaseURL: DefaultTokenGateBaseURL,
		Model:   "gpt-5.1",
	})
	if err == nil {
		t.Fatal("expected api_key required")
	}
}

func boolPtr(value bool) *bool {
	return &value
}
