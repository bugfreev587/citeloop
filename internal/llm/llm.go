// Package llm defines the LLMProvider abstraction (PRD §4), the TokenGate
// OpenAI-compatible client, and a deterministic mock for tests / no-key runs.
package llm

import (
	"context"
	"errors"
)

const (
	// DefaultTokenGateModel is the environment fallback when no admin model is
	// saved. Admin settings can override this without redeploying.
	DefaultTokenGateModel    = "claude-sonnet-4-6"
	DefaultOpenAIModel       = "gpt-5.1"
	DefaultOpenAIWriterModel = "gpt-5.1"
	DefaultOpenAIQAModel     = "gpt-5.5"
)

type CompletionPurpose string

const (
	PurposeDefault CompletionPurpose = ""
	PurposeWriter  CompletionPurpose = "writer"
	PurposeQA      CompletionPurpose = "qa"
	PurposeSiteFix CompletionPurpose = "site_fix"
)

// CompletionReq is a provider-agnostic completion request.
type CompletionReq struct {
	System  string
	Prompt  string
	Purpose CompletionPurpose
	// Model optionally overrides the provider's default model for this request.
	Model       string
	MaxTokens   int
	Temperature float64
	// JSON asks the provider to bias toward a single JSON object response.
	JSON bool
	// DisableProviderFallback keeps a role-specific model error explicit instead
	// of retrying through the legacy OpenAI fallback.
	DisableProviderFallback bool
	// AttemptObserver is set by the canonical AI call recorder. Providers that
	// implement AttemptObservable notify it once per physical provider request,
	// including model fallback attempts.
	AttemptObserver AttemptObserver
}

// CompletionResp carries the text plus accounting used by the cost breaker (§5.4).
type CompletionResp struct {
	Text             string
	Provider         string
	Model            string
	PromptTokens     int
	CompletionTokens int
	Tokens           int
	CostUSD          float64
}

// Provider is the LLMProvider interface from PRD §4.
type Provider interface {
	Complete(ctx context.Context, req CompletionReq) (CompletionResp, error)
}

type AttemptObserver interface {
	StartAttempt(context.Context, string) (string, error)
	FinishAttempt(context.Context, string, CompletionResp, error) error
}

type AttemptObservable interface {
	ObservesProviderAttempts()
}

// CompleteObserved wraps providers that cannot emit attempt callbacks while
// preserving native callbacks for providers that can distinguish preflight
// work from the physical network call.
func CompleteObserved(ctx context.Context, provider Provider, req CompletionReq) (CompletionResp, error) {
	if provider == nil {
		return CompletionResp{}, errors.New("AI provider is unavailable")
	}
	if req.AttemptObserver == nil {
		return provider.Complete(ctx, req)
	}
	if _, observable := provider.(AttemptObservable); observable {
		return provider.Complete(ctx, req)
	}
	attemptID, err := req.AttemptObserver.StartAttempt(ctx, req.Model)
	if err != nil {
		return CompletionResp{}, err
	}
	resp, providerErr := provider.Complete(ctx, req)
	finishErr := req.AttemptObserver.FinishAttempt(context.WithoutCancel(ctx), attemptID, resp, providerErr)
	return resp, errors.Join(providerErr, finishErr)
}
