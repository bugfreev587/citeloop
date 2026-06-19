package admin

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/llm"
)

func TestStatusFromCredentialsMasksAPIKey(t *testing.T) {
	updatedAt := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	status := StatusFromCredentials(&Credentials{
		Provider:  ProviderOpenAI,
		APIKey:    "sk-live-secret-abcdef",
		BaseURL:   "https://api.openai.com/v1",
		UpdatedAt: updatedAt,
	})

	if !status.Configured {
		t.Fatal("status should be configured")
	}
	if status.Provider != "openai" {
		t.Fatalf("provider = %q", status.Provider)
	}
	if status.KeyTail != "cdef" {
		t.Fatalf("key tail = %q", status.KeyTail)
	}
	if status.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("base url = %q", status.BaseURL)
	}
	raw, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sk-live-secret") {
		t.Fatalf("status leaked the raw api key: %s", raw)
	}
}

func TestApplyUpdatePreservesExistingKeyWhenBlank(t *testing.T) {
	next, err := ApplyUpdate(&Credentials{
		Provider: ProviderClaude,
		APIKey:   "anthropic-existing-key",
	}, UpdateInput{
		Provider: "claude",
		APIKey:   "   ",
	})
	if err != nil {
		t.Fatal(err)
	}

	if next.APIKey != "anthropic-existing-key" {
		t.Fatalf("api key = %q", next.APIKey)
	}
	if next.Provider != ProviderClaude {
		t.Fatalf("provider = %q", next.Provider)
	}
}

func TestApplyUpdateSupportsTokenGateBaseURL(t *testing.T) {
	next, err := ApplyUpdate(nil, UpdateInput{
		Provider: "tokengate",
		APIKey:   "tg-new-key",
		BaseURL:  " https://tokengate-production.up.railway.app/v1/ ",
	})
	if err != nil {
		t.Fatal(err)
	}

	if next.Provider != ProviderTokenGate {
		t.Fatalf("provider = %q", next.Provider)
	}
	if next.BaseURL != "https://tokengate-production.up.railway.app/v1" {
		t.Fatalf("base url = %q", next.BaseURL)
	}
}

func TestApplyUpdatePreservesExistingBaseURLWhenBlank(t *testing.T) {
	next, err := ApplyUpdate(&Credentials{
		Provider: ProviderTokenGate,
		APIKey:   "tg-existing-key",
		BaseURL:  "https://tokengate.example/v1",
	}, UpdateInput{
		Provider: "tokengate",
		APIKey:   "   ",
		BaseURL:  "   ",
	})
	if err != nil {
		t.Fatal(err)
	}

	if next.APIKey != "tg-existing-key" {
		t.Fatalf("api key = %q", next.APIKey)
	}
	if next.BaseURL != "https://tokengate.example/v1" {
		t.Fatalf("base url = %q", next.BaseURL)
	}
}

func TestApplyUpdateRequiresKeyWhenChangingProvider(t *testing.T) {
	_, err := ApplyUpdate(&Credentials{
		Provider: ProviderOpenAI,
		APIKey:   "openai-existing-key",
	}, UpdateInput{
		Provider: "claude",
		APIKey:   "",
	})
	if err == nil {
		t.Fatal("expected error when changing provider without a new key")
	}
}

func TestProviderFromCredentialsUsesTokenGateBaseURL(t *testing.T) {
	provider := ProviderFromCredentials(Credentials{
		Provider: ProviderTokenGate,
		APIKey:   "tg-test-key",
		BaseURL:  "https://tokengate.example/v1",
	}, config.Env{TokenGateModel: "claude-sonnet-4-6"})

	openai, ok := provider.(*llm.OpenAIChat)
	if !ok {
		t.Fatalf("provider = %T, want *llm.OpenAIChat", provider)
	}
	if openai.APIKey != "tg-test-key" {
		t.Fatalf("api key = %q", openai.APIKey)
	}
	if openai.BaseURL != "https://tokengate.example/v1" {
		t.Fatalf("base url = %q", openai.BaseURL)
	}
}
