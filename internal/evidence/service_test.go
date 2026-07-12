package evidence

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestCollectionSpecFingerprintSeparatesSemanticInputs(t *testing.T) {
	base := map[string]any{"user_agent": "OAI-SearchBot", "dimensions": []string{"page", "query"}, "provider": "perplexity", "prompt_version": "v1"}
	a, _, err := Fingerprint(base)
	if err != nil {
		t.Fatal(err)
	}
	b, _, _ := Fingerprint(map[string]any{"prompt_version": "v1", "provider": "perplexity", "dimensions": []string{"page", "query"}, "user_agent": "OAI-SearchBot"})
	if a != b {
		t.Fatal("map key order changed fingerprint")
	}
	for key, value := range map[string]any{"user_agent": "Googlebot", "dimensions": []string{"page"}, "provider": "openai", "prompt_version": "v2"} {
		variant := map[string]any{"user_agent": "OAI-SearchBot", "dimensions": []string{"page", "query"}, "provider": "perplexity", "prompt_version": "v1"}
		variant[key] = value
		got, _, _ := Fingerprint(variant)
		if got == a {
			t.Fatalf("changing %s did not change fingerprint", key)
		}
	}
}

func TestCollectReusesExactIdentityWithoutRepeatingProvider(t *testing.T) {
	store := newEvidenceStoreStub()
	svc := NewService(store)
	now := time.Date(2026, 7, 12, 3, 0, 0, 0, time.UTC)
	req := Request{ProjectID: uuid.New(), Source: "gsc", NormalizedTarget: "sc-domain:example.com", TargetKind: "integration", WindowStart: datePtr(now.AddDate(0, 0, -28)), WindowEnd: datePtr(now), CollectionSpec: map[string]any{"dimensions": []string{"page", "query"}, "row_limit": 25000}, RequestedBy: "opportunities", Now: now}
	calls := 0
	collector := func(context.Context) ([]Observation, error) {
		calls++
		return []Observation{{Key: "aggregate", State: StateObserved, Facts: map[string]any{"rows": 10}, RawSnapshot: map[string]any{"page_rows": []any{}}, Confidence: 1, Completeness: 1}}, nil
	}
	first, err := svc.Collect(context.Background(), req, collector)
	if err != nil {
		t.Fatal(err)
	}
	req.RequestedBy = "doctor"
	second, err := svc.Collect(context.Background(), req, collector)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || first.Reused || !second.Reused || first.Run.ID != second.Run.ID || len(second.Observations) != 1 {
		t.Fatalf("first=%#v second=%#v calls=%d", first, second, calls)
	}
}

func TestCollectPersistsPartialAndProviderUnavailableWithoutZeroInference(t *testing.T) {
	store := newEvidenceStoreStub()
	svc := NewService(store)
	req := Request{ProjectID: uuid.New(), Source: "ai_answer", NormalizedTarget: "best tools", TargetKind: "prompt", CollectionSpec: map[string]any{"provider": "perplexity", "prompt_version": "v2"}, RequestedBy: "opportunities", Now: time.Now().UTC()}
	providerErr := errors.New("provider timeout")
	result, err := svc.Collect(context.Background(), req, func(context.Context) ([]Observation, error) {
		return []Observation{{Key: "perplexity:en-US", State: StateProviderUnavailable, Facts: map[string]any{"coverage_gap": true}, RawSnapshot: map[string]any{"error": "timeout"}, Confidence: 0, Completeness: 0, CallStatus: stringPtr("failed"), ErrorCode: stringPtr("timeout")}}, providerErr
	})
	if !errors.Is(err, providerErr) || result.Run.Status != "partial" || len(result.Observations) != 1 || result.Observations[0].EvidenceState != StateProviderUnavailable {
		t.Fatalf("partial result=%#v err=%v", result, err)
	}
}

func TestFailedIdentityRetriesAsNewAttemptAndLinksOnlyConsumedAttempt(t *testing.T) {
	store := newEvidenceStoreStub()
	svc := NewService(store)
	consumerID := uuid.New()
	req := Request{ProjectID: uuid.New(), Source: "gsc", NormalizedTarget: "sc-domain:example.com", TargetKind: "integration", CollectionSpec: map[string]any{"dimensions": []string{"page"}}, RequestedBy: "opportunities", ConsumerType: "seo_run", ConsumerID: consumerID, Now: time.Now().UTC()}
	providerErr := errors.New("temporary provider failure")
	first, err := svc.Collect(context.Background(), req, func(context.Context) ([]Observation, error) { return nil, providerErr })
	if !errors.Is(err, providerErr) || first.Run.Status != "failed" || len(store.consumptions) != 0 {
		t.Fatalf("first attempt=%#v err=%v consumptions=%#v", first, err, store.consumptions)
	}
	second, err := svc.Collect(context.Background(), req, func(context.Context) ([]Observation, error) {
		return []Observation{{Key: "aggregate", State: StateObserved, Facts: map[string]any{"rows": 1}, RawSnapshot: map[string]any{"rows": 1}, Confidence: 1, Completeness: 1}}, nil
	})
	if err != nil || second.Run.AttemptNumber != 2 || len(second.Observations) != 1 || second.Observations[0].AttemptNumber != 2 {
		t.Fatalf("retry=%#v err=%v", second, err)
	}
	if len(store.consumptions) != 1 || store.consumptions[0].AttemptNumber != 2 {
		t.Fatalf("consumptions=%#v", store.consumptions)
	}
}

func TestObservationPersistenceFailureStillTerminalizesAttempt(t *testing.T) {
	store := newEvidenceStoreStub()
	store.createErr = errors.New("observation insert failed")
	req := Request{ProjectID: uuid.New(), Source: "crawl", NormalizedTarget: "https://example.com", TargetKind: "site", CollectionSpec: map[string]any{"user_agent": "Googlebot"}, RequestedBy: "doctor", Now: time.Now().UTC()}
	result, err := NewService(store).Collect(context.Background(), req, func(context.Context) ([]Observation, error) {
		return []Observation{{Key: "page", State: StateObserved, Facts: map[string]any{}, RawSnapshot: map[string]any{}, Confidence: 1, Completeness: 1}}, nil
	})
	if err == nil || result.Run.Status != "failed" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func datePtr(value time.Time) *time.Time { return &value }
func stringPtr(value string) *string     { return &value }

type evidenceStoreStub struct {
	mu           sync.Mutex
	runsByKey    map[string]db.EvidenceRun
	observations map[uuid.UUID][]db.EvidenceObservation
	consumptions []db.EvidenceConsumption
	createErr    error
}

func newEvidenceStoreStub() *evidenceStoreStub {
	return &evidenceStoreStub{runsByKey: map[string]db.EvidenceRun{}, observations: map[uuid.UUID][]db.EvidenceObservation{}}
}

func (s *evidenceStoreStub) AcquireEvidenceRun(_ context.Context, arg db.AcquireEvidenceRunParams) (db.AcquireEvidenceRunRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := arg.ProjectID.String() + arg.Source + arg.NormalizedTarget + arg.CollectionSpecFingerprint + arg.WindowStart.Time.String() + arg.WindowEnd.Time.String()
	if row, ok := s.runsByKey[key]; ok {
		if row.Status == "failed" || row.Status == "partial" || (row.Status == "running" && row.LeaseExpiresAt.Valid && !row.LeaseExpiresAt.Time.After(arg.StartedAt.Time)) {
			row.CollectionOwnerToken = arg.CollectionOwnerToken
			row.AttemptNumber++
			row.LeaseExpiresAt = arg.LeaseExpiresAt
			row.Status, row.ErrorSummary, row.FinishedAt = "running", nil, pgtype.Timestamptz{}
			s.runsByKey[key] = row
		}
		return acquireRow(row), nil
	}
	row := db.EvidenceRun{ID: arg.ID, ProjectID: arg.ProjectID, Source: arg.Source, NormalizedTarget: arg.NormalizedTarget, TargetKind: arg.TargetKind, WindowStart: arg.WindowStart, WindowEnd: arg.WindowEnd, CollectionSpec: arg.CollectionSpec, CollectionSpecFingerprint: arg.CollectionSpecFingerprint, CollectionOwnerToken: arg.CollectionOwnerToken, AttemptNumber: arg.AttemptNumber, LeaseExpiresAt: arg.LeaseExpiresAt, RequestedBy: arg.RequestedBy, Status: "running", StartedAt: arg.StartedAt}
	s.runsByKey[key] = row
	return acquireRow(row), nil
}

func acquireRow(row db.EvidenceRun) db.AcquireEvidenceRunRow {
	return db.AcquireEvidenceRunRow{
		ID: row.ID, ProjectID: row.ProjectID, Source: row.Source, NormalizedTarget: row.NormalizedTarget,
		TargetKind: row.TargetKind, WindowStart: row.WindowStart, WindowEnd: row.WindowEnd,
		CollectionSpec: row.CollectionSpec, CollectionSpecFingerprint: row.CollectionSpecFingerprint,
		CollectionOwnerToken: row.CollectionOwnerToken, AttemptNumber: row.AttemptNumber,
		LeaseExpiresAt: row.LeaseExpiresAt, ErrorHistory: row.ErrorHistory, RequestedBy: row.RequestedBy,
		Status: row.Status, ErrorSummary: row.ErrorSummary, StartedAt: row.StartedAt,
		FinishedAt: row.FinishedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func (s *evidenceStoreStub) CreateEvidenceObservation(_ context.Context, arg db.CreateEvidenceObservationParams) (db.EvidenceObservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.createErr != nil {
		return db.EvidenceObservation{}, s.createErr
	}
	for _, row := range s.observations[arg.RunID] {
		if row.AttemptNumber == arg.AttemptNumber && row.SourceObservationKey == arg.SourceObservationKey {
			return db.EvidenceObservation{}, pgx.ErrNoRows
		}
	}
	row := db.EvidenceObservation{ID: arg.ID, ProjectID: arg.ProjectID, RunID: arg.RunID, AttemptNumber: arg.AttemptNumber, Source: arg.Source, SourceObservationKey: arg.SourceObservationKey, NormalizedTarget: arg.NormalizedTarget, TargetKind: arg.TargetKind, EvidenceState: arg.EvidenceState, Facts: arg.Facts, RawSnapshot: arg.RawSnapshot, Confidence: arg.Confidence, Completeness: arg.Completeness, CallStatus: arg.CallStatus, ErrorCode: arg.ErrorCode, ObservedAt: arg.ObservedAt}
	s.observations[arg.RunID] = append(s.observations[arg.RunID], row)
	return row, nil
}

func (s *evidenceStoreStub) GetEvidenceObservation(_ context.Context, arg db.GetEvidenceObservationParams) (db.EvidenceObservation, error) {
	for _, row := range s.observations[arg.RunID] {
		if row.AttemptNumber == arg.AttemptNumber && row.SourceObservationKey == arg.SourceObservationKey {
			return row, nil
		}
	}
	return db.EvidenceObservation{}, pgx.ErrNoRows
}

func (s *evidenceStoreStub) ListEvidenceObservations(_ context.Context, arg db.ListEvidenceObservationsParams) ([]db.EvidenceObservation, error) {
	rows := []db.EvidenceObservation{}
	for _, row := range s.observations[arg.RunID] {
		if row.AttemptNumber == arg.AttemptNumber {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func (s *evidenceStoreStub) FinishEvidenceRun(_ context.Context, arg db.FinishEvidenceRunParams) (db.FinishEvidenceRunRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, row := range s.runsByKey {
		if row.ID == arg.ID && row.AttemptNumber == arg.AttemptNumber && row.CollectionOwnerToken == arg.CollectionOwnerToken {
			row.Status, row.ErrorSummary, row.FinishedAt = arg.Status, arg.ErrorSummary, arg.FinishedAt
			s.runsByKey[key] = row
			return finishRow(row), nil
		}
	}
	return db.FinishEvidenceRunRow{}, pgx.ErrNoRows
}

func finishRow(row db.EvidenceRun) db.FinishEvidenceRunRow {
	return db.FinishEvidenceRunRow{
		ID: row.ID, ProjectID: row.ProjectID, Source: row.Source, NormalizedTarget: row.NormalizedTarget,
		TargetKind: row.TargetKind, WindowStart: row.WindowStart, WindowEnd: row.WindowEnd,
		CollectionSpec: row.CollectionSpec, CollectionSpecFingerprint: row.CollectionSpecFingerprint,
		CollectionOwnerToken: row.CollectionOwnerToken, AttemptNumber: row.AttemptNumber,
		LeaseExpiresAt: row.LeaseExpiresAt, ErrorHistory: row.ErrorHistory, RequestedBy: row.RequestedBy,
		Status: row.Status, ErrorSummary: row.ErrorSummary, StartedAt: row.StartedAt,
		FinishedAt: row.FinishedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func (s *evidenceStoreStub) LinkEvidenceConsumption(_ context.Context, arg db.LinkEvidenceConsumptionParams) (db.EvidenceConsumption, error) {
	row := db.EvidenceConsumption{ID: uuid.New(), ProjectID: arg.ProjectID, EvidenceRunID: arg.EvidenceRunID, AttemptNumber: arg.AttemptNumber, ConsumerType: arg.ConsumerType, ConsumerID: arg.ConsumerID}
	s.consumptions = append(s.consumptions, row)
	return row, nil
}
