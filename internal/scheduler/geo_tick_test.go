package scheduler

import (
	"context"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/geo"
)

type fakeGEOAnswerProvider struct {
	available bool
}

func (f fakeGEOAnswerProvider) Name() string {
	return "fake_answer_provider"
}

func (f fakeGEOAnswerProvider) Available() bool {
	return f.available
}

func (f fakeGEOAnswerProvider) Observe(context.Context, []db.GeoPrompt) ([]geo.ProviderObservation, float64, error) {
	return nil, 0, nil
}

func TestSchedulerGEOTickUsesConfiguredAnswerProvider(t *testing.T) {
	s := &Scheduler{
		GEOAnswerProvider:       fakeGEOAnswerProvider{available: true},
		GEOProviderRunBudgetUSD: 0.25,
	}

	service := s.geoService(db.New(nil))
	if service.AnswerProvider == nil || !service.AnswerProvider.Available() {
		t.Fatal("geo service should include configured automatic answer provider")
	}

	req := s.geoObserveRequest()
	if req.Engine != "Perplexity" || req.MaxPrompts != 10 || req.BudgetUSD != 0.25 {
		t.Fatalf("observe request = %+v, want Perplexity top 10 with configured budget", req)
	}
}
