package main

import (
	"io"
	"log/slog"
	"testing"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/llm"
)

func TestSelectLLMProviderPrefersTokenGateWhenConfigured(t *testing.T) {
	provider := selectLLMProvider(config.Env{
		TokenGateAPIKey:  "tg-test-key",
		TokenGateBaseURL: "https://tokengate-production.up.railway.app/v1",
		TokenGateModel:   "claude-haiku-4-5-20251001",
		AnthropicAPIKey:  "anthropic-key",
		AnthropicModel:   "claude-opus-4-8",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	openai, ok := provider.(*llm.OpenAIChat)
	if !ok {
		t.Fatalf("provider = %T, want *llm.OpenAIChat", provider)
	}
	if openai.Model != "claude-haiku-4-5-20251001" {
		t.Fatalf("model = %q", openai.Model)
	}
}

func TestSelectLLMProviderFallsBackToClaudeThenMock(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	claudeProvider := selectLLMProvider(config.Env{
		AnthropicAPIKey: "anthropic-key",
		AnthropicModel:  "claude-opus-4-8",
	}, log)
	if _, ok := claudeProvider.(*llm.Claude); !ok {
		t.Fatalf("provider = %T, want *llm.Claude", claudeProvider)
	}

	mockProvider := selectLLMProvider(config.Env{}, log)
	if _, ok := mockProvider.(*llm.Mock); !ok {
		t.Fatalf("provider = %T, want *llm.Mock", mockProvider)
	}
}
