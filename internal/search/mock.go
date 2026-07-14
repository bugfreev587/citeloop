package search

import (
	"context"
	"time"
)

// Mock is a deterministic SearchProvider for tests / no-key runs.
type Mock struct {
	// Fail, when true, makes Search return an error so the Strategist's
	// degrade-to-pure-LLM path (§5.2) can be exercised.
	Fail bool
	now  func() time.Time
}

func NewMock() *Mock { return &Mock{now: func() time.Time { return time.Unix(0, 0).UTC() }} }

func (m *Mock) ProviderName() string { return "mock_search" }
func (m *Mock) Synthetic() bool      { return true }

func (m *Mock) Search(_ context.Context, q Query) ([]Result, error) {
	if m.Fail {
		return nil, context.DeadlineExceeded
	}
	return []Result{
		{Title: "Best social scheduling tools 2026", URL: "https://example.com/a", Snippet: "A roundup of scheduling tools.", Source: "mock", FetchedAt: m.now()},
		{Title: q.Text + " — guide", URL: "https://example.com/b", Snippet: "Everything about " + q.Text, Source: "mock", FetchedAt: m.now()},
	}, nil
}
