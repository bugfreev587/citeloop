// Package llm defines the LLMProvider abstraction (PRD §4) and concrete
// implementations: a real Anthropic Claude provider and a deterministic mock
// for tests / no-key runs.
package llm

import "context"

// CompletionReq is a provider-agnostic completion request.
type CompletionReq struct {
	System      string
	Prompt      string
	MaxTokens   int
	Temperature float64
	// JSON asks the provider to bias toward a single JSON object response.
	JSON bool
}

// CompletionResp carries the text plus accounting used by the cost breaker (§5.4).
type CompletionResp struct {
	Text     string
	Model    string
	Tokens   int
	CostUSD  float64
}

// Provider is the LLMProvider interface from PRD §4.
type Provider interface {
	Complete(ctx context.Context, req CompletionReq) (CompletionResp, error)
}
