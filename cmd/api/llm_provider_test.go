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
		TokenGateModel:   "claude-sonnet-4-6",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	openai, ok := provider.(*llm.OpenAIChat)
	if !ok {
		t.Fatalf("provider = %T, want *llm.OpenAIChat", provider)
	}
	if openai.Model != "claude-sonnet-4-6" {
		t.Fatalf("model = %q", openai.Model)
	}
}

func TestSelectLLMProviderFallsBackToMockWithoutTokenGate(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	mockProvider := selectLLMProvider(config.Env{}, log)
	if _, ok := mockProvider.(*llm.Mock); !ok {
		t.Fatalf("provider = %T, want *llm.Mock", mockProvider)
	}
}
