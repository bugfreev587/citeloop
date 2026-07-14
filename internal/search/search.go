// Package search defines the SearchProvider abstraction (PRD §4/§5.2) plus a
// real Brave Search implementation and a deterministic mock. The Strategist
// uses this for keyword / AI-prompt research; failures degrade to pure-LLM
// topic selection (§5.2), they do not fail the whole run.
package search

import (
	"context"
	"time"
)

// Query is a single search request.
type Query struct {
	Text  string
	Count int
}

// Result mirrors PRD §4: { Title, URL, Snippet, Source, FetchedAt }.
type Result struct {
	Title         string    `json:"title"`
	URL           string    `json:"url"`
	Snippet       string    `json:"snippet"`
	Source        string    `json:"source"`
	FetchedAt     time.Time `json:"fetched_at"`
	ProviderOrder int       `json:"provider_order"`
}

type EvidenceProvider interface {
	Provider
	ProviderName() string
	Synthetic() bool
}

// Provider is the SearchProvider interface from PRD §4.
type Provider interface {
	Search(ctx context.Context, q Query) ([]Result, error)
}
