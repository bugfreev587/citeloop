package growthradar

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/search"
	"github.com/google/uuid"
)

type searchProviderStub struct {
	synthetic bool
	calls     int
	err       error
}

func (p *searchProviderStub) ProviderName() string { return "brave_web_search" }
func (p *searchProviderStub) Synthetic() bool      { return p.synthetic }
func (p *searchProviderStub) Search(_ context.Context, q search.Query) ([]search.Result, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	return []search.Result{{Title: "Result", URL: "https://example.com", Snippet: q.Text, Source: "brave_web_search", ProviderOrder: 1}}, nil
}

type searchStoreStub struct {
	cached *EvidenceSet
	budget SearchBudget
	saved  []EvidenceSet
}

func (s *searchStoreStub) FindSearchEvidence(_ context.Context, _ uuid.UUID, _ string, _ time.Time) (*EvidenceSet, error) {
	return s.cached, nil
}
func (s *searchStoreStub) SearchUsage(_ context.Context, _ uuid.UUID, _ time.Time) (SearchBudget, error) {
	return s.budget, nil
}
func (s *searchStoreStub) SaveSearchEvidence(_ context.Context, set EvidenceSet) error {
	s.saved = append(s.saved, set)
	return nil
}

func TestSearchEvidenceUsesSevenDayCacheAndBraveProvenance(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	provider := &searchProviderStub{}
	store := &searchStoreStub{}
	collector := SearchCollector{Provider: provider, Store: store, Now: func() time.Time { return now }}
	first, err := collector.Collect(context.Background(), CollectSearchRequest{ProjectID: uuid.New(), Query: "  AI   Visibility  "})
	if err != nil {
		t.Fatal(err)
	}
	if first.Provider != "brave_web_search" || first.ProviderOrderIsRank || first.ExpiresAt != now.Add(7*24*time.Hour) || !first.UsableForScoring {
		t.Fatalf("evidence = %+v", first)
	}
	store.cached = &first
	second, err := collector.Collect(context.Background(), CollectSearchRequest{ProjectID: first.ProjectID, Query: "ai visibility"})
	if err != nil || !second.Reused || provider.calls != 1 {
		t.Fatalf("cache = %+v calls=%d err=%v", second, provider.calls, err)
	}
}

func TestSearchBudgetStopsBeforeEveryHardLimit(t *testing.T) {
	tests := []SearchBudget{
		{DailyRequests: 30}, {WeeklyRebuildRequests: 60}, {RollingRequests: 600}, {RollingCostUSD: 3}, {InstallationCostUSD: 25},
	}
	for index, budget := range tests {
		provider := &searchProviderStub{}
		store := &searchStoreStub{budget: budget}
		_, err := (SearchCollector{Provider: provider, Store: store}).Collect(context.Background(), CollectSearchRequest{ProjectID: uuid.New(), Query: "query", Trigger: "weekly_rebuild"})
		if !errors.Is(err, ErrSearchBudgetExhausted) || provider.calls != 0 {
			t.Errorf("case %d err=%v calls=%d", index, err, provider.calls)
		}
	}
}

func TestSearchEvidenceExcludesMockAndDegradesWithoutKey(t *testing.T) {
	for _, provider := range []*searchProviderStub{{synthetic: true}, {err: errors.New("search api key not set")}} {
		set, err := (SearchCollector{Provider: provider, Store: &searchStoreStub{}}).Collect(context.Background(), CollectSearchRequest{ProjectID: uuid.New(), Query: "query"})
		if provider.synthetic {
			if err != nil || set.UsableForScoring || !set.Synthetic {
				t.Fatalf("mock = %+v err=%v", set, err)
			}
		} else if err == nil || set.Status != "degraded" {
			t.Fatalf("no-key set=%+v err=%v", set, err)
		}
	}
}
