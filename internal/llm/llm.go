// Package llm defines the LLMProvider abstraction (PRD §4) and concrete
// implementations: a real Anthropic Claude provider and a deterministic mock
// for tests / no-key runs.
package llm

import "context"

const (
	// ModelClaudeSonnet is the default draft-writing model.
	ModelClaudeSonnet = "claude-sonnet-4-6"
	// ModelClaudeOpus is used for higher-stakes QA, repair, and analysis work.
	ModelClaudeOpus = "claude-opus-4-8"
)

// CompletionReq is a provider-agnostic completion request.
type CompletionReq struct {
	System string
	Prompt string
	// Model optionally overrides the provider's default model for this request.
	Model       string
	MaxTokens   int
	Temperature float64
	// JSON asks the provider to bias toward a single JSON object response.
	JSON bool
}

// CompletionResp carries the text plus accounting used by the cost breaker (§5.4).
type CompletionResp struct {
	Text    string
	Model   string
	Tokens  int
	CostUSD float64
}

// Provider is the LLMProvider interface from PRD §4.
type Provider interface {
	Complete(ctx context.Context, req CompletionReq) (CompletionResp, error)
}
