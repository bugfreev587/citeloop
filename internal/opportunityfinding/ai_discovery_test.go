package opportunityfinding

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/geo"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
)

func TestRunAIDiscoveryGeneratesPromptsBeforeProviderObservation(t *testing.T) {
	projectID := uuid.New()
	store := &fakePromptStore{}
	service := &fakeAIDiscoveryService{
		generatedPrompts: []db.GeoPrompt{{ID: uuid.New(), ProjectID: projectID, PromptText: "best citation tools", Status: "active"}},
	}

	result, err := RunAIDiscovery(context.Background(), projectID, store, service, AIDiscoveryOptions{
		ObserveRequest: geo.ObserveAnswerProviderRequest{Engine: "OpenAI", MaxPrompts: 10, BudgetUSD: 0.25},
	})
	if err != nil {
		t.Fatalf("RunAIDiscovery error: %v", err)
	}

	wantCalls := []string{"generate_prompt_set", "crawler_audit", "observe_provider", "external_surfaces", "analyze"}
	if !reflect.DeepEqual(service.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", service.calls, wantCalls)
	}
	if !result.PromptSetGenerated || result.ActivePromptCount != 1 {
		t.Fatalf("prompt result = %+v, want generated prompt count", result)
	}
	if result.ObservationCount != 1 || result.ObservationCostUSD != 0.03 {
		t.Fatalf("observation summary = %+v, want one observation and cost", result)
	}
	if result.OpportunityCount != 1 || result.AssetBriefCount != 1 {
		t.Fatalf("analysis summary = %+v, want opportunity and brief counts", result)
	}
}

func TestRunAIDiscoveryReusesActivePrompts(t *testing.T) {
	projectID := uuid.New()
	store := &fakePromptStore{prompts: []db.GeoPrompt{{ID: uuid.New(), ProjectID: projectID, PromptText: "best tools", Status: "active"}}}
	service := &fakeAIDiscoveryService{}

	result, err := RunAIDiscovery(context.Background(), projectID, store, service, AIDiscoveryOptions{})
	if err != nil {
		t.Fatalf("RunAIDiscovery error: %v", err)
	}

	wantCalls := []string{"crawler_audit", "observe_provider", "external_surfaces", "analyze"}
	if !reflect.DeepEqual(service.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", service.calls, wantCalls)
	}
	if result.PromptSetGenerated || result.ActivePromptCount != 1 {
		t.Fatalf("prompt result = %+v, want existing active prompt count", result)
	}
}

func TestRunAIDiscoveryReturnsPromptListErrors(t *testing.T) {
	projectID := uuid.New()
	store := &fakePromptStore{err: errors.New("database unavailable")}
	service := &fakeAIDiscoveryService{}

	_, err := RunAIDiscovery(context.Background(), projectID, store, service, AIDiscoveryOptions{})
	if err == nil || !errors.Is(err, store.err) {
		t.Fatalf("RunAIDiscovery error = %v, want prompt store error", err)
	}
	if len(service.calls) != 0 {
		t.Fatalf("service calls = %#v, want none after prompt list failure", service.calls)
	}
}

func TestAIDiscoverySeparatesEvidenceRefreshFromHypothesisMaterialization(t *testing.T) {
	projectID := uuid.New()
	store := &fakePromptStore{prompts: []db.GeoPrompt{{ID: uuid.New(), ProjectID: projectID, PromptText: "citation tools", Status: "active"}}}
	service := &fakeAIDiscoveryService{}

	evidenceResult, err := RefreshAIDiscoveryEvidence(context.Background(), projectID, store, service, AIDiscoveryOptions{})
	if err != nil {
		t.Fatalf("RefreshAIDiscoveryEvidence error: %v", err)
	}
	if want := []string{"crawler_audit", "observe_provider", "external_surfaces"}; !reflect.DeepEqual(service.calls, want) {
		t.Fatalf("evidence calls = %#v, want %#v", service.calls, want)
	}
	if evidenceResult.OpportunityCount != 0 || evidenceResult.ObservationCount != 1 {
		t.Fatalf("evidence result = %+v", evidenceResult)
	}

	hypothesisResult, err := MaterializeAIDiscoveryHypotheses(context.Background(), projectID, service)
	if err != nil {
		t.Fatalf("MaterializeAIDiscoveryHypotheses error: %v", err)
	}
	if want := []string{"crawler_audit", "observe_provider", "external_surfaces", "analyze"}; !reflect.DeepEqual(service.calls, want) {
		t.Fatalf("all calls = %#v, want %#v", service.calls, want)
	}
	if hypothesisResult.OpportunityCount != 1 || hypothesisResult.AssetBriefCount != 1 {
		t.Fatalf("hypothesis result = %+v", hypothesisResult)
	}
}

type fakePromptStore struct {
	prompts []db.GeoPrompt
	err     error
}

func (s *fakePromptStore) ListActiveGEOPrompts(context.Context, uuid.UUID) ([]db.GeoPrompt, error) {
	return s.prompts, s.err
}

type fakeAIDiscoveryService struct {
	calls            []string
	generatedPrompts []db.GeoPrompt
	analyzeRequests  []geo.AnalyzeObservationsRequest
}

func (s *fakeAIDiscoveryService) GeneratePromptSet(context.Context, uuid.UUID, geo.GeneratePromptSetRequest) (geo.GeneratePromptSetResult, error) {
	s.calls = append(s.calls, "generate_prompt_set")
	return geo.GeneratePromptSetResult{
		Run:     db.GeoRun{ID: uuid.New(), Status: "ok"},
		Prompts: s.generatedPrompts,
	}, nil
}

func (s *fakeAIDiscoveryService) RunCrawlerAudit(context.Context, uuid.UUID, geo.CrawlerAuditRequest) (geo.CrawlerAuditResult, error) {
	s.calls = append(s.calls, "crawler_audit")
	return geo.CrawlerAuditResult{Run: db.GeoRun{ID: uuid.New(), Status: "ok"}, CheckedURLs: 2}, nil
}

func (s *fakeAIDiscoveryService) ObserveAnswerProvider(context.Context, uuid.UUID, geo.ObserveAnswerProviderRequest) (geo.ObserveAnswerProviderResult, error) {
	s.calls = append(s.calls, "observe_provider")
	return geo.ObserveAnswerProviderResult{
		Run:          db.GeoRun{ID: uuid.New(), Status: "ok"},
		Observations: []db.GeoObservation{{ID: uuid.New()}},
		CostUSD:      0.03,
	}, nil
}

func (s *fakeAIDiscoveryService) MonitorExternalSurfaces(context.Context, uuid.UUID, geo.MonitorExternalSurfacesRequest) (geo.MonitorExternalSurfacesResult, error) {
	s.calls = append(s.calls, "external_surfaces")
	return geo.MonitorExternalSurfacesResult{Run: db.GeoRun{ID: uuid.New(), Status: "ok"}, Checked: 1}, nil
}

func (s *fakeAIDiscoveryService) AnalyzeObservations(_ context.Context, _ uuid.UUID, req geo.AnalyzeObservationsRequest) (geo.AnalyzeObservationsResult, error) {
	s.calls = append(s.calls, "analyze")
	s.analyzeRequests = append(s.analyzeRequests, req)
	return geo.AnalyzeObservationsResult{
		Run:           db.GeoRun{ID: uuid.New(), Status: "ok"},
		Opportunities: []db.UpsertGEOObservationOpportunityRow{{ID: uuid.New(), PriorityScore: pgutil.Numeric(80)}},
		AssetBriefs:   []db.GeoAssetBrief{{ID: uuid.New()}},
	}, nil
}

func TestMaterializeAIDiscoveryHypothesesHonorsRolloutMode(t *testing.T) {
	projectID := uuid.New()
	service := &fakeAIDiscoveryService{}

	off, err := MaterializeAIDiscoveryHypothesesWithMode(context.Background(), projectID, service, GrowthRadarOff)
	if err != nil || len(service.calls) != 0 || off.Funnel.Status != "skipped" {
		t.Fatalf("off result = %+v, calls=%v, err=%v", off, service.calls, err)
	}
	observed, err := MaterializeAIDiscoveryHypothesesWithMode(context.Background(), projectID, service, GrowthRadarObserve)
	if err != nil || len(service.analyzeRequests) != 1 || !service.analyzeRequests[0].DryRun {
		t.Fatalf("observe-only must dry-run: result=%+v requests=%+v err=%v", observed, service.analyzeRequests, err)
	}
	created, err := MaterializeAIDiscoveryHypothesesWithMode(context.Background(), projectID, service, GrowthRadarCreate)
	if err != nil || len(service.analyzeRequests) != 2 || service.analyzeRequests[1].DryRun || created.OpportunityCount != 1 {
		t.Fatalf("create mode must materialize: result=%+v requests=%+v err=%v", created, service.analyzeRequests, err)
	}
}
