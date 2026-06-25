// Package llm defines the LLMProvider abstraction (PRD §4), the TokenGate
// OpenAI-compatible client, and a deterministic mock for tests / no-key runs.
package llm

import "context"

const (
	// DefaultTokenGateModel is the environment fallback when no admin model is
	// saved. Admin settings can override this without redeploying.
	DefaultTokenGateModel = "claude-sonnet-4-6"
)

type CompletionPurpose string

const (
	PurposeDefault CompletionPurpose = ""
	PurposeWriter  CompletionPurpose = "writer"
	PurposeQA      CompletionPurpose = "qa"
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
