package geo

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/evidence"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestCrawlerEvidenceQualityDoesNotTreatUncheckedAsHealthy(t *testing.T) {
	state, completeness, confidence, status, failed := crawlerEvidenceQuality([]AuditResult{{PageURL: "https://example.com/a", AccessState: AccessTimeout}}, 0)
	if state != evidence.StateMissing || completeness != 0 || confidence != 0 || status != "failed" || len(failed) != 1 {
		t.Fatalf("all-timeout quality = %q %.2f %.2f %q %#v", state, completeness, confidence, status, failed)
	}
	state, completeness, confidence, status, _ = crawlerEvidenceQuality([]AuditResult{
		{PageURL: "https://example.com/a", AccessState: AccessOK},
		{PageURL: "https://example.com/b", AccessState: AccessError},
	}, 1)
	if state != evidence.StateObserved || completeness != 1.0/3.0 || confidence != 1.0/3.0 || status != "partial" {
		t.Fatalf("partial quality = %q %.4f %.4f %q", state, completeness, confidence, status)
	}
}

func TestAnswerUsageFallsBackPerRowWhenProviderOmitsTotal(t *testing.T) {
	usage := answerUsageFromRows([]ProviderObservation{
		{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5},
		{PromptTokens: 7, CompletionTokens: 4},
	}, 0.03)
	if usage.PromptTokens != 10 || usage.CompletionTokens != 6 || usage.TotalTokens != 16 || usage.CostUSD != 0.03 {
		t.Fatalf("usage=%+v", usage)
	}
}

func TestAnswerEvidenceUsesWeeklyFreshnessBucket(t *testing.T) {
	got := startOfEvidenceWeek(time.Date(2026, 7, 12, 14, 30, 0, 0, time.UTC))
	want := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("week start = %s, want %s", got, want)
	}
}

func TestAnswerProviderEvidenceIdentityIncludesModelAndEndpointVersion(t *testing.T) {
	first := NewPerplexityProvider("key", "https://api.example/v1", "sonar-a", nil).EvidenceIdentity()
	second := NewPerplexityProvider("key", "https://api.example/v2", "sonar-b", nil).EvidenceIdentity()
	if first.Model == second.Model || first.ProviderVersion == second.ProviderVersion {
		t.Fatalf("provider identities collapsed: first=%#v second=%#v", first, second)
	}
}

func TestObserveAnswerPromptsBoundedConcurrency(t *testing.T) {
	const wantedConcurrency = 3
	prompts := answerPromptFixtures(10)
	provider := &concurrentPromptProvider{started: make(chan uuid.UUID, len(prompts)), release: make(chan struct{})}
	service := Service{AnswerProvider: provider, AICallStore: newPromptCallStore()}
	type result struct {
		rows  []ProviderObservation
		usage answerCallUsage
		err   error
	}
	done := make(chan result, 1)
	go func() {
		rows, usage, err := service.observeAnswerPrompts(context.Background(), uuid.New(), uuid.New(), prompts, ObserveAnswerProviderRequest{}, provider.Name(), provider.EvidenceIdentity())
		done <- result{rows: rows, usage: usage, err: err}
	}()

	for range wantedConcurrency {
		select {
		case <-provider.started:
		case <-time.After(250 * time.Millisecond):
			t.Fatal("three prompt calls did not start concurrently")
		}
	}
	select {
	case <-provider.started:
		t.Fatal("more than three prompt calls started before a worker was released")
	case <-time.After(30 * time.Millisecond):
	}
	close(provider.release)

	got := <-done
	if got.err != nil {
		t.Fatalf("observeAnswerPrompts error: %v", got.err)
	}
	if provider.maximumActive() != wantedConcurrency {
		t.Fatalf("maximum active calls = %d, want %d", provider.maximumActive(), wantedConcurrency)
	}
	if len(got.rows) != len(prompts) || got.usage.TotalTokens != len(prompts)*3 {
		t.Fatalf("rows=%d usage=%+v", len(got.rows), got.usage)
	}
	for index, row := range got.rows {
		if row.PromptID != prompts[index].ID {
			t.Fatalf("row %d prompt = %s, want %s", index, row.PromptID, prompts[index].ID)
		}
	}
}

func TestObserveAnswerPromptsPreservesPartialSuccess(t *testing.T) {
	prompts := answerPromptFixtures(4)
	provider := &concurrentPromptProvider{failPromptID: prompts[1].ID}
	service := Service{AnswerProvider: provider, AICallStore: newPromptCallStore()}

	rows, usage, err := service.observeAnswerPrompts(context.Background(), uuid.New(), uuid.New(), prompts, ObserveAnswerProviderRequest{}, provider.Name(), provider.EvidenceIdentity())
	if err == nil {
		t.Fatal("observeAnswerPrompts error = nil, want provider failure")
	}
	if len(rows) != 3 || rows[0].PromptID != prompts[0].ID || rows[1].PromptID != prompts[2].ID || rows[2].PromptID != prompts[3].ID {
		t.Fatalf("partial rows = %#v, want successful rows in input order", rows)
	}
	if usage.TotalTokens != len(prompts)*3 || provider.callCount() != len(prompts) {
		t.Fatalf("usage=%+v calls=%d, want accounting for every started prompt", usage, provider.callCount())
	}
}

func answerPromptFixtures(count int) []db.GeoPrompt {
	prompts := make([]db.GeoPrompt, count)
	for index := range prompts {
		prompts[index] = db.GeoPrompt{ID: uuid.New(), PromptText: "prompt"}
	}
	return prompts
}

type concurrentPromptProvider struct {
	mu           sync.Mutex
	active       int
	maxActive    int
	calls        int
	started      chan uuid.UUID
	release      chan struct{}
	failPromptID uuid.UUID
}

func (p *concurrentPromptProvider) Name() string { return "concurrent-test" }

func (p *concurrentPromptProvider) EvidenceIdentity() AnswerProviderEvidenceIdentity {
	return AnswerProviderEvidenceIdentity{Model: "test-model", ProviderVersion: "test-v1"}
}

func (p *concurrentPromptProvider) Available() bool { return true }

func (p *concurrentPromptProvider) Observe(context.Context, []db.GeoPrompt) ([]ProviderObservation, float64, error) {
	return nil, 0, errors.New("bulk Observe must not be used")
}

func (p *concurrentPromptProvider) ObservePrompt(ctx context.Context, prompt db.GeoPrompt) (ProviderObservation, float64, error) {
	p.mu.Lock()
	p.active++
	p.calls++
	if p.active > p.maxActive {
		p.maxActive = p.active
	}
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.active--
		p.mu.Unlock()
	}()
	if p.started != nil {
		p.started <- prompt.ID
	}
	if p.release != nil {
		select {
		case <-p.release:
		case <-ctx.Done():
			return ProviderObservation{}, 0, ctx.Err()
		}
	}
	row := ProviderObservation{PromptID: prompt.ID, PromptTokens: 2, CompletionTokens: 1, TotalTokens: 3, CostUSD: 0.01}
	if prompt.ID == p.failPromptID {
		return row, row.CostUSD, errors.New("test provider failure")
	}
	return row, row.CostUSD, nil
}

func (p *concurrentPromptProvider) maximumActive() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.maxActive
}

func (p *concurrentPromptProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

type promptCallStore struct {
	mu   sync.Mutex
	rows map[uuid.UUID]db.AiCallRecord
}

func newPromptCallStore() *promptCallStore {
	return &promptCallStore{rows: map[uuid.UUID]db.AiCallRecord{}}
}

func (s *promptCallStore) CreateAICallRecord(_ context.Context, arg db.CreateAICallRecordParams) (db.AiCallRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row := db.AiCallRecord{ID: uuid.New(), ProjectID: arg.ProjectID, Status: arg.Status, Provider: arg.Provider, Model: arg.Model, RequestFingerprint: arg.RequestFingerprint, ProviderCalled: true}
	s.rows[row.ID] = row
	return row, nil
}

func (s *promptCallStore) CreateSkippedAICallRecord(context.Context, db.CreateSkippedAICallRecordParams) (db.AiCallRecord, error) {
	return db.AiCallRecord{}, errors.New("unexpected skipped call")
}

func (s *promptCallStore) FinishAICallRecord(context.Context, db.FinishAICallRecordParams) (db.AiCallRecord, error) {
	return db.AiCallRecord{}, errors.New("legacy finish is not expected")
}

func (s *promptCallStore) FinishCanonicalAICallFenced(_ context.Context, arg db.FinishCanonicalAICallFencedParams) (db.AiCallRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.rows[arg.ID]
	if !ok {
		return db.AiCallRecord{}, pgx.ErrNoRows
	}
	row.Status, row.ErrorCode = arg.Status, arg.ErrorCode
	row.PromptTokens, row.CompletionTokens, row.TotalTokens, row.CostUsd = arg.PromptTokens, arg.CompletionTokens, arg.TotalTokens, arg.CostUsd
	if arg.ResolvedProvider != nil {
		row.Provider = *arg.ResolvedProvider
	}
	if arg.ResolvedModel != nil {
		row.Model = *arg.ResolvedModel
	}
	s.rows[row.ID] = row
	return row, nil
}

func (s *promptCallStore) MarkAICallProviderStarted(context.Context, db.MarkAICallProviderStartedParams) (db.AiCallRecord, error) {
	return db.AiCallRecord{}, errors.New("unexpected provider marker")
}

func (s *promptCallStore) ReclassifyAICallRecordOutputFailure(context.Context, db.ReclassifyAICallRecordOutputFailureParams) (db.AiCallRecord, error) {
	return db.AiCallRecord{}, errors.New("unexpected output reclassification")
}

func (s *promptCallStore) GetAICallRecord(_ context.Context, arg db.GetAICallRecordParams) (db.AiCallRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.rows[arg.ID]
	if !ok {
		return db.AiCallRecord{}, pgx.ErrNoRows
	}
	return row, nil
}

func (s *promptCallStore) GetLatestAICallForRequest(context.Context, db.GetLatestAICallForRequestParams) (db.AiCallRecord, error) {
	return db.AiCallRecord{}, pgx.ErrNoRows
}

func (s *promptCallStore) AggregateAICallsForObject(context.Context, db.AggregateAICallsForObjectParams) (db.AggregateAICallsForObjectRow, error) {
	return db.AggregateAICallsForObjectRow{CostUsd: pgutil.Numeric(0)}, nil
}
