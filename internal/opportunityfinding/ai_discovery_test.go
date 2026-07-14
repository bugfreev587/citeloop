package opportunityfinding

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/geo"
	"github.com/citeloop/citeloop/internal/growthradar"
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

	if !sameCalls(service.calls, []string{"generate_prompt_set", "crawler_audit", "observe_provider", "external_surfaces", "analyze"}) || service.calls[0] != "generate_prompt_set" || service.calls[len(service.calls)-1] != "analyze" {
		t.Fatalf("calls = %#v, want prompt generation first, parallel evidence, then analysis", service.calls)
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

	if !sameCalls(service.calls, []string{"crawler_audit", "observe_provider", "external_surfaces", "analyze"}) || service.calls[len(service.calls)-1] != "analyze" {
		t.Fatalf("calls = %#v, want parallel evidence before analysis", service.calls)
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

func TestRunAIDiscoveryOffMakesNoDiscoveryCalls(t *testing.T) {
	service := &fakeAIDiscoveryService{}
	result, err := RunAIDiscovery(context.Background(), uuid.New(), &fakePromptStore{}, service, AIDiscoveryOptions{GrowthRadarMode: GrowthRadarOff})
	if err != nil {
		t.Fatal(err)
	}
	if len(service.calls) != 0 || result.Funnel.Status != "skipped" {
		t.Fatalf("off mode result=%+v calls=%v", result, service.calls)
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
	if want := []string{"crawler_audit", "observe_provider", "external_surfaces"}; !sameCalls(service.calls, want) {
		t.Fatalf("evidence calls = %#v, want %#v", service.calls, want)
	}
	if evidenceResult.OpportunityCount != 0 || evidenceResult.ObservationCount != 1 {
		t.Fatalf("evidence result = %+v", evidenceResult)
	}

	audit := &candidateRunStore{runID: uuid.New()}
	hypothesisResult, err := MaterializeAIDiscoveryHypotheses(context.Background(), projectID, service, audit)
	if err != nil {
		t.Fatalf("MaterializeAIDiscoveryHypotheses error: %v", err)
	}
	if want := []string{"crawler_audit", "observe_provider", "external_surfaces", "analyze"}; !sameCalls(service.calls, want) || service.calls[len(service.calls)-1] != "analyze" {
		t.Fatalf("all calls = %#v, want %#v", service.calls, want)
	}
	if hypothesisResult.OpportunityCount != 1 || hypothesisResult.AssetBriefCount != 1 {
		t.Fatalf("hypothesis result = %+v", hypothesisResult)
	}
}

func sameCalls(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	counts := map[string]int{}
	for _, value := range got {
		counts[value]++
	}
	for _, value := range want {
		counts[value]--
	}
	for _, count := range counts {
		if count != 0 {
			return false
		}
	}
	return true
}

type fakePromptStore struct {
	prompts []db.GeoPrompt
	err     error
	runID   uuid.UUID
}

type fakeManualPlanner struct {
	calls []ManualDiscoveryPlanRequest
	store *fakePromptStore
}

func (p *fakeManualPlanner) Plan(_ context.Context, req ManualDiscoveryPlanRequest) (ManualDiscoveryPlanResult, error) {
	p.calls = append(p.calls, req)
	created := db.GeoPrompt{ID: uuid.New(), ProjectID: req.ProjectID, PromptSetID: req.ExistingPrompts[0].PromptSetID, PromptText: "new stage-aware question", Status: "active", TargetedReason: "manual_foundation_discovery"}
	p.store.prompts = append(p.store.prompts, created)
	return ManualDiscoveryPlanResult{Created: []db.GeoPrompt{created}, Proposed: 1, Accepted: 1, ProviderCalled: true, TotalTokens: 42}, nil
}

func TestManualAIDiscoveryPlansStageAwarePromptsBeforeEvidence(t *testing.T) {
	projectID := uuid.New()
	workflowID := uuid.New()
	store := &fakePromptStore{prompts: []db.GeoPrompt{{ID: uuid.New(), ProjectID: projectID, PromptSetID: uuid.New(), PromptText: "existing prompt", Status: "active"}}}
	planner := &fakeManualPlanner{store: store}
	service := &fakeAIDiscoveryService{}

	result, err := RefreshAIDiscoveryEvidence(context.Background(), projectID, store, service, AIDiscoveryOptions{
		Planner: planner, Stage: "foundation", WorkflowID: workflowID, FreshEvidenceKey: workflowID.String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(planner.calls) != 1 || planner.calls[0].Stage != "foundation" || result.PlannerAccepted != 1 || result.PlannerTokens != 42 {
		t.Fatalf("planner calls=%+v result=%+v", planner.calls, result)
	}
	if len(service.observeRequests) != 1 || len(service.observeRequests[0].PromptIDs) != 2 {
		t.Fatalf("observation did not include refreshed prompt portfolio: %+v", service.observeRequests)
	}
}

func (s *fakePromptStore) ListActiveGEOPrompts(context.Context, uuid.UUID) ([]db.GeoPrompt, error) {
	return s.prompts, s.err
}
func (s *fakePromptStore) CreateGrowthRadarRun(context.Context, db.CreateGrowthRadarRunParams) (db.GrowthRadarRun, error) {
	if s.runID == uuid.Nil {
		s.runID = uuid.New()
	}
	return db.GrowthRadarRun{ID: s.runID}, nil
}
func (s *fakePromptStore) CreateGrowthRadarItem(context.Context, db.CreateGrowthRadarItemParams) (db.GrowthRadarItem, error) {
	return db.GrowthRadarItem{ID: uuid.New()}, nil
}
func (s *fakePromptStore) UpdateGrowthRadarRun(context.Context, db.UpdateGrowthRadarRunParams) (db.GrowthRadarRun, error) {
	return db.GrowthRadarRun{ID: s.runID}, nil
}
func (s *fakePromptStore) UpsertGrowthRadarWatchlistItem(context.Context, db.UpsertGrowthRadarWatchlistItemParams) (db.GrowthRadarWatchlist, error) {
	return db.GrowthRadarWatchlist{}, nil
}
func (s *fakePromptStore) ResolveGrowthRadarWatchlistItem(context.Context, db.ResolveGrowthRadarWatchlistItemParams) error {
	return nil
}
func (s *fakePromptStore) ExpireGrowthRadarWatchlist(context.Context, db.ExpireGrowthRadarWatchlistParams) ([]db.GrowthRadarWatchlist, error) {
	return nil, nil
}

type fakeAIDiscoveryService struct {
	mu               sync.Mutex
	calls            []string
	generatedPrompts []db.GeoPrompt
	analyzeRequests  []geo.AnalyzeObservationsRequest
	observeRequests  []geo.ObserveAnswerProviderRequest
}

func (s *fakeAIDiscoveryService) recordCall(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, name)
}

func (s *fakeAIDiscoveryService) GeneratePromptSet(context.Context, uuid.UUID, geo.GeneratePromptSetRequest) (geo.GeneratePromptSetResult, error) {
	s.recordCall("generate_prompt_set")
	return geo.GeneratePromptSetResult{
		Run:     db.GeoRun{ID: uuid.New(), Status: "ok"},
		Prompts: s.generatedPrompts,
	}, nil
}

func (s *fakeAIDiscoveryService) RunCrawlerAudit(context.Context, uuid.UUID, geo.CrawlerAuditRequest) (geo.CrawlerAuditResult, error) {
	s.recordCall("crawler_audit")
	return geo.CrawlerAuditResult{Run: db.GeoRun{ID: uuid.New(), Status: "ok"}, CheckedURLs: 2}, nil
}

func (s *fakeAIDiscoveryService) ObserveAnswerProvider(_ context.Context, _ uuid.UUID, req geo.ObserveAnswerProviderRequest) (geo.ObserveAnswerProviderResult, error) {
	s.recordCall("observe_provider")
	s.mu.Lock()
	s.observeRequests = append(s.observeRequests, req)
	s.mu.Unlock()
	return geo.ObserveAnswerProviderResult{
		Run:          db.GeoRun{ID: uuid.New(), Status: "ok"},
		Observations: []db.GeoObservation{{ID: uuid.New()}},
		CostUSD:      0.03,
	}, nil
}

func TestManualAIDiscoveryCarriesFreshEvidenceIdentity(t *testing.T) {
	projectID := uuid.New()
	workflowID := uuid.New().String()
	store := &fakePromptStore{prompts: []db.GeoPrompt{{ID: uuid.New(), ProjectID: projectID, PromptText: "best social publishing tools", Status: "active"}}}
	service := &fakeAIDiscoveryService{}

	_, err := RefreshAIDiscoveryEvidence(context.Background(), projectID, store, service, AIDiscoveryOptions{FreshEvidenceKey: workflowID})
	if err != nil {
		t.Fatal(err)
	}
	if len(service.observeRequests) != 1 || service.observeRequests[0].FreshEvidenceKey != workflowID {
		t.Fatalf("observe requests = %+v, want fresh identity %q", service.observeRequests, workflowID)
	}
}

func (s *fakeAIDiscoveryService) MonitorExternalSurfaces(context.Context, uuid.UUID, geo.MonitorExternalSurfacesRequest) (geo.MonitorExternalSurfacesResult, error) {
	s.recordCall("external_surfaces")
	return geo.MonitorExternalSurfacesResult{Run: db.GeoRun{ID: uuid.New(), Status: "ok"}, Checked: 1}, nil
}

func (s *fakeAIDiscoveryService) AnalyzeObservations(_ context.Context, _ uuid.UUID, req geo.AnalyzeObservationsRequest) (geo.AnalyzeObservationsResult, error) {
	s.recordCall("analyze")
	s.analyzeRequests = append(s.analyzeRequests, req)
	candidate := geo.GrowthRadarCandidate{Identity: "candidate-1", Disposition: "opportunity", Reason: "score_threshold", Score: growthradar.Score{FormulaVersion: growthradar.FormulaVersion, Final: 80, Disposition: "opportunity"}}
	if req.BeforeCreate != nil {
		if err := req.BeforeCreate(candidate); err != nil {
			return geo.AnalyzeObservationsResult{}, err
		}
	}
	return geo.AnalyzeObservationsResult{
		Run:            db.GeoRun{ID: uuid.New(), Status: "ok"},
		Opportunities:  []db.UpsertGEOObservationOpportunityRow{{ID: uuid.New(), PriorityScore: pgutil.Numeric(80)}},
		AssetBriefs:    []db.GeoAssetBrief{{ID: uuid.New()}},
		CandidateCount: 1,
		Candidates:     []geo.GrowthRadarCandidate{candidate},
	}, nil
}

type concurrentEvidenceService struct {
	entered chan string
	release chan struct{}
}

func (s *concurrentEvidenceService) wait(name string) {
	s.entered <- name
	<-s.release
}

func (s *concurrentEvidenceService) GeneratePromptSet(context.Context, uuid.UUID, geo.GeneratePromptSetRequest) (geo.GeneratePromptSetResult, error) {
	return geo.GeneratePromptSetResult{}, nil
}
func (s *concurrentEvidenceService) RunCrawlerAudit(context.Context, uuid.UUID, geo.CrawlerAuditRequest) (geo.CrawlerAuditResult, error) {
	s.wait("crawler")
	return geo.CrawlerAuditResult{Run: db.GeoRun{Status: "ok"}}, nil
}
func (s *concurrentEvidenceService) ObserveAnswerProvider(context.Context, uuid.UUID, geo.ObserveAnswerProviderRequest) (geo.ObserveAnswerProviderResult, error) {
	s.wait("answer")
	return geo.ObserveAnswerProviderResult{Run: db.GeoRun{Status: "ok"}}, nil
}
func (s *concurrentEvidenceService) MonitorExternalSurfaces(context.Context, uuid.UUID, geo.MonitorExternalSurfacesRequest) (geo.MonitorExternalSurfacesResult, error) {
	s.wait("surfaces")
	return geo.MonitorExternalSurfacesResult{Run: db.GeoRun{Status: "ok"}}, nil
}
func (s *concurrentEvidenceService) AnalyzeObservations(context.Context, uuid.UUID, geo.AnalyzeObservationsRequest) (geo.AnalyzeObservationsResult, error) {
	return geo.AnalyzeObservationsResult{}, nil
}

func TestAIDiscoveryRunsIndependentEvidenceCollectorsConcurrently(t *testing.T) {
	projectID := uuid.New()
	store := &fakePromptStore{prompts: []db.GeoPrompt{{ID: uuid.New(), ProjectID: projectID, PromptText: "social publishing", Status: "active"}}}
	service := &concurrentEvidenceService{entered: make(chan string, 3), release: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		_, err := RefreshAIDiscoveryEvidence(context.Background(), projectID, store, service, AIDiscoveryOptions{})
		done <- err
	}()

	seen := map[string]bool{}
	for len(seen) < 3 {
		select {
		case name := <-service.entered:
			seen[name] = true
		case <-time.After(500 * time.Millisecond):
			close(service.release)
			t.Fatalf("collectors did not overlap; entered=%v", seen)
		}
	}
	close(service.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
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
	created, err := MaterializeAIDiscoveryHypothesesWithMode(context.Background(), projectID, service, GrowthRadarCreate, &candidateRunStore{runID: uuid.New()})
	if err != nil || len(service.analyzeRequests) != 2 || service.analyzeRequests[1].DryRun || created.OpportunityCount != 1 {
		t.Fatalf("create mode must materialize: result=%+v requests=%+v err=%v", created, service.analyzeRequests, err)
	}
}

type candidateRunStore struct {
	runID     uuid.UUID
	items     []db.CreateGrowthRadarItemParams
	itemErr   error
	updateErr error
	watchlist []db.UpsertGrowthRadarWatchlistItemParams
}

func (s *candidateRunStore) CreateGrowthRadarRun(context.Context, db.CreateGrowthRadarRunParams) (db.GrowthRadarRun, error) {
	return db.GrowthRadarRun{ID: s.runID}, nil
}
func (s *candidateRunStore) CreateGrowthRadarItem(_ context.Context, item db.CreateGrowthRadarItemParams) (db.GrowthRadarItem, error) {
	if s.itemErr != nil {
		return db.GrowthRadarItem{}, s.itemErr
	}
	s.items = append(s.items, item)
	return db.GrowthRadarItem{ID: uuid.New()}, nil
}
func (s *candidateRunStore) UpdateGrowthRadarRun(context.Context, db.UpdateGrowthRadarRunParams) (db.GrowthRadarRun, error) {
	return db.GrowthRadarRun{ID: s.runID}, s.updateErr
}
func (s *candidateRunStore) UpsertGrowthRadarWatchlistItem(_ context.Context, arg db.UpsertGrowthRadarWatchlistItemParams) (db.GrowthRadarWatchlist, error) {
	s.watchlist = append(s.watchlist, arg)
	return db.GrowthRadarWatchlist{ProjectID: arg.ProjectID, CandidateIdentity: arg.CandidateIdentity}, nil
}
func (s *candidateRunStore) ResolveGrowthRadarWatchlistItem(context.Context, db.ResolveGrowthRadarWatchlistItemParams) error {
	return nil
}
func (s *candidateRunStore) ExpireGrowthRadarWatchlist(context.Context, db.ExpireGrowthRadarWatchlistParams) ([]db.GrowthRadarWatchlist, error) {
	return nil, nil
}

func TestCandidateAnalysisPersistsReplayableScoreItems(t *testing.T) {
	store := &candidateRunStore{runID: uuid.New()}
	candidate := geo.GrowthRadarCandidate{Identity: "candidate-1", Disposition: "watchlist", Reason: "watchlist", Score: growthradar.Score{FormulaVersion: growthradar.FormulaVersion, Final: 70}, Snapshot: growthradar.Snapshot{QualifiedRecurrence: 2}}
	if err := persistAIDiscoveryFunnelWithCandidates(context.Background(), store, uuid.New(), "candidate_analysis", growthradar.Funnel{Status: "ok"}, []geo.GrowthRadarCandidate{candidate}); err != nil {
		t.Fatal(err)
	}
	if len(store.items) != 1 || store.items[0].CandidateIdentity != candidate.Identity || len(store.items[0].Score) == 0 || len(store.items[0].ScoringSnapshot) == 0 {
		t.Fatalf("persisted items = %+v", store.items)
	}
	if len(store.watchlist) != 1 || store.watchlist[0].EvidenceFingerprint == "" || !store.watchlist[0].ExpiresAt.Valid {
		t.Fatalf("durable watchlist = %+v", store.watchlist)
	}
}

func TestDurableWatchlistFingerprintChangesWithRawEvidence(t *testing.T) {
	store := &candidateRunStore{runID: uuid.New()}
	base := geo.GrowthRadarCandidate{
		Identity: "candidate-1", Disposition: "watchlist", Reason: "watchlist",
		Score:    growthradar.Score{FormulaVersion: growthradar.FormulaVersion, Final: 70},
		Snapshot: growthradar.Snapshot{QualifiedRecurrence: 1}, Evidence: json.RawMessage(`{"observation_id":"one","cited_urls":["https://one.example"]}`),
	}
	if err := persistGrowthRadarCandidate(context.Background(), store, store.runID, uuid.New(), base); err != nil {
		t.Fatal(err)
	}
	base.Evidence = json.RawMessage(`{"observation_id":"two","cited_urls":["https://two.example"]}`)
	if err := persistGrowthRadarCandidate(context.Background(), store, store.runID, uuid.New(), base); err != nil {
		t.Fatal(err)
	}
	if len(store.watchlist) != 2 || store.watchlist[0].EvidenceFingerprint == store.watchlist[1].EvidenceFingerprint {
		t.Fatalf("raw evidence must change durable fingerprint: %+v", store.watchlist)
	}
}

func TestCreateModeBlocksOpportunityWhenCandidateAuditFails(t *testing.T) {
	auditErr := errors.New("audit unavailable")
	service := &fakeAIDiscoveryService{}
	result, err := MaterializeAIDiscoveryHypothesesWithMode(context.Background(), uuid.New(), service, GrowthRadarCreate, &candidateRunStore{runID: uuid.New(), itemErr: auditErr})
	if err == nil || !errors.Is(err, auditErr) || result.OpportunityCount != 0 {
		t.Fatalf("result=%+v err=%v, want audit failure before opportunity creation", result, err)
	}
}
