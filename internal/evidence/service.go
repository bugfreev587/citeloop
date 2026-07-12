package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	StateObserved            = "observed"
	StateInferred            = "inferred"
	StateModelAssisted       = "model_assisted"
	StateMissing             = "missing"
	StateProviderUnavailable = "provider_unavailable"
)

var ErrCollectionInProgress = errors.New("evidence collection is already in progress")

const collectionLease = 15 * time.Minute

type Store interface {
	AcquireEvidenceRun(context.Context, db.AcquireEvidenceRunParams) (db.AcquireEvidenceRunRow, error)
	CreateEvidenceObservation(context.Context, db.CreateEvidenceObservationParams) (db.EvidenceObservation, error)
	GetEvidenceObservation(context.Context, db.GetEvidenceObservationParams) (db.EvidenceObservation, error)
	ListEvidenceObservations(context.Context, db.ListEvidenceObservationsParams) ([]db.EvidenceObservation, error)
	FinishEvidenceRun(context.Context, db.FinishEvidenceRunParams) (db.FinishEvidenceRunRow, error)
	LinkEvidenceConsumption(context.Context, db.LinkEvidenceConsumptionParams) (db.EvidenceConsumption, error)
}

type Service struct{ store Store }

func NewService(store Store) Service { return Service{store: store} }

type Request struct {
	ProjectID        uuid.UUID
	Source           string
	NormalizedTarget string
	TargetKind       string
	WindowStart      *time.Time
	WindowEnd        *time.Time
	CollectionSpec   map[string]any
	RequestedBy      string
	ConsumerType     string
	ConsumerID       uuid.UUID
	Now              time.Time
}

type Observation struct {
	Key              string
	State            string
	Facts            map[string]any
	RawSnapshot      any
	Confidence       float64
	Completeness     float64
	Provider         *string
	Model            *string
	ProviderVersion  *string
	PromptVersion    *string
	CallStatus       *string
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	CostUSD          float64
	PrivacyState     string
	PermissionState  string
	ErrorCode        *string
	ObservedAt       time.Time
}

type Collector func(context.Context) ([]Observation, error)

type Result struct {
	Run          db.EvidenceRun
	Observations []db.EvidenceObservation
	Reused       bool
}

func Fingerprint(spec any) (string, json.RawMessage, error) {
	raw, err := json.Marshal(spec)
	if err != nil {
		return "", nil, err
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return "", nil, errors.New("collection spec must be a JSON object")
	}
	canonical, err := json.Marshal(object)
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:]), canonical, nil
}

func (s Service) Collect(ctx context.Context, req Request, collector Collector) (Result, error) {
	if s.store == nil || collector == nil {
		return Result{}, errors.New("evidence store and collector are required")
	}
	if err := validateRequest(req); err != nil {
		return Result{}, err
	}
	fingerprint, spec, err := Fingerprint(req.CollectionSpec)
	if err != nil {
		return Result{}, err
	}
	now := req.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	ownerToken := uuid.New()
	acquired, err := s.store.AcquireEvidenceRun(ctx, db.AcquireEvidenceRunParams{
		ID: uuid.New(), ProjectID: req.ProjectID, Source: req.Source,
		NormalizedTarget: strings.TrimSpace(req.NormalizedTarget), TargetKind: req.TargetKind,
		WindowStart: date(req.WindowStart), WindowEnd: date(req.WindowEnd),
		CollectionSpec: spec, CollectionSpecFingerprint: fingerprint,
		CollectionOwnerToken: ownerToken, RequestedBy: mustJSON([]string{req.RequestedBy}),
		AttemptNumber: 1, LeaseExpiresAt: pgutil.TS(now.Add(collectionLease)), StartedAt: pgutil.TS(now),
	})
	if err != nil {
		return Result{}, err
	}
	run := evidenceRunFromAcquire(acquired)
	if run.CollectionOwnerToken != ownerToken {
		observations, listErr := s.store.ListEvidenceObservations(ctx, db.ListEvidenceObservationsParams{ProjectID: req.ProjectID, RunID: run.ID, AttemptNumber: run.AttemptNumber})
		result := Result{Run: run, Observations: observations, Reused: true}
		if listErr != nil {
			return result, listErr
		}
		if run.Status == "running" {
			return result, ErrCollectionInProgress
		}
		if len(observations) > 0 {
			if err := s.linkConsumption(ctx, req, run); err != nil {
				return result, err
			}
		}
		if run.Status == "failed" || run.Status == "partial" {
			if run.ErrorSummary != nil {
				return result, errors.New(*run.ErrorSummary)
			}
			return result, fmt.Errorf("reused evidence attempt ended %s", run.Status)
		}
		return result, nil
	}

	collected, collectErr := collector(ctx)
	persisted := make([]db.EvidenceObservation, 0, len(collected))
	for _, observation := range collected {
		row, persistErr := s.persistObservation(ctx, req, run, now, observation)
		if persistErr != nil {
			collectErr = errors.Join(collectErr, persistErr)
			break
		}
		persisted = append(persisted, row)
	}
	status := "completed"
	if collectErr != nil {
		status = "failed"
		if len(persisted) > 0 {
			status = "partial"
		}
	}
	var errorSummary *string
	if collectErr != nil {
		message := collectErr.Error()
		errorSummary = &message
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	finished, finishErr := s.store.FinishEvidenceRun(finishCtx, db.FinishEvidenceRunParams{
		Status: status, ErrorSummary: errorSummary, FinishedAt: pgutil.TS(time.Now().UTC()),
		ID: run.ID, ProjectID: req.ProjectID, AttemptNumber: run.AttemptNumber, CollectionOwnerToken: ownerToken,
	})
	run = evidenceRunFromFinish(finished)
	result := Result{Run: run, Observations: persisted}
	if finishErr != nil {
		return result, finishErr
	}
	if len(persisted) > 0 {
		if err := s.linkConsumption(finishCtx, req, run); err != nil {
			return result, err
		}
	}
	return result, collectErr
}

func evidenceRunFromFinish(row db.FinishEvidenceRunRow) db.EvidenceRun {
	return db.EvidenceRun{
		ID: row.ID, ProjectID: row.ProjectID, Source: row.Source, NormalizedTarget: row.NormalizedTarget,
		TargetKind: row.TargetKind, WindowStart: row.WindowStart, WindowEnd: row.WindowEnd,
		CollectionSpec: row.CollectionSpec, CollectionSpecFingerprint: row.CollectionSpecFingerprint,
		CollectionOwnerToken: row.CollectionOwnerToken, AttemptNumber: row.AttemptNumber,
		LeaseExpiresAt: row.LeaseExpiresAt, ErrorHistory: row.ErrorHistory, RequestedBy: row.RequestedBy,
		Status: row.Status, ErrorSummary: row.ErrorSummary, StartedAt: row.StartedAt,
		FinishedAt: row.FinishedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func evidenceRunFromAcquire(row db.AcquireEvidenceRunRow) db.EvidenceRun {
	return db.EvidenceRun{
		ID: row.ID, ProjectID: row.ProjectID, Source: row.Source, NormalizedTarget: row.NormalizedTarget,
		TargetKind: row.TargetKind, WindowStart: row.WindowStart, WindowEnd: row.WindowEnd,
		CollectionSpec: row.CollectionSpec, CollectionSpecFingerprint: row.CollectionSpecFingerprint,
		CollectionOwnerToken: row.CollectionOwnerToken, AttemptNumber: row.AttemptNumber,
		LeaseExpiresAt: row.LeaseExpiresAt, ErrorHistory: row.ErrorHistory, RequestedBy: row.RequestedBy,
		Status: row.Status, ErrorSummary: row.ErrorSummary, StartedAt: row.StartedAt,
		FinishedAt: row.FinishedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func (s Service) linkConsumption(ctx context.Context, req Request, run db.EvidenceRun) error {
	if strings.TrimSpace(req.ConsumerType) == "" || req.ConsumerID == uuid.Nil {
		return nil
	}
	_, err := s.store.LinkEvidenceConsumption(ctx, db.LinkEvidenceConsumptionParams{
		ProjectID: req.ProjectID, EvidenceRunID: run.ID, AttemptNumber: run.AttemptNumber,
		ConsumerType: req.ConsumerType, ConsumerID: req.ConsumerID,
	})
	return err
}

func (s Service) persistObservation(ctx context.Context, req Request, run db.EvidenceRun, now time.Time, observation Observation) (db.EvidenceObservation, error) {
	if !validState(observation.State) {
		return db.EvidenceObservation{}, fmt.Errorf("unsupported evidence state %q", observation.State)
	}
	observedAt := observation.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = now
	}
	privacy := strings.TrimSpace(observation.PrivacyState)
	if privacy == "" {
		privacy = "not_applicable"
	}
	permission := strings.TrimSpace(observation.PermissionState)
	if permission == "" {
		permission = "not_applicable"
	}
	key := strings.TrimSpace(observation.Key)
	if key == "" {
		key = "aggregate"
	}
	params := db.CreateEvidenceObservationParams{
		ID: uuid.New(), ProjectID: req.ProjectID, RunID: run.ID, AttemptNumber: run.AttemptNumber, Source: req.Source,
		SourceObservationKey: key, NormalizedTarget: strings.TrimSpace(req.NormalizedTarget), TargetKind: req.TargetKind,
		EvidenceState: observation.State, Facts: mustJSON(observation.Facts), RawSnapshot: mustJSON(observation.RawSnapshot),
		Confidence: pgutil.Numeric(clamp01(observation.Confidence)), Completeness: pgutil.Numeric(clamp01(observation.Completeness)),
		Provider: observation.Provider, Model: observation.Model, ProviderVersion: observation.ProviderVersion, PromptVersion: observation.PromptVersion,
		CallStatus: observation.CallStatus, PromptTokens: observation.PromptTokens,
		CompletionTokens: observation.CompletionTokens, TotalTokens: observation.TotalTokens,
		CostUsd: pgutil.Numeric(maxFloat(observation.CostUSD, 0)), PrivacyState: privacy,
		PermissionState: permission, ErrorCode: observation.ErrorCode, ObservedAt: pgutil.TS(observedAt),
		WindowStart: date(req.WindowStart), WindowEnd: date(req.WindowEnd),
	}
	row, err := s.store.CreateEvidenceObservation(ctx, params)
	if !errors.Is(err, pgx.ErrNoRows) {
		return row, err
	}
	return s.store.GetEvidenceObservation(ctx, db.GetEvidenceObservationParams{ProjectID: req.ProjectID, RunID: run.ID, AttemptNumber: run.AttemptNumber, SourceObservationKey: key})
}

func validateRequest(req Request) error {
	if req.ProjectID == uuid.Nil || strings.TrimSpace(req.Source) == "" || strings.TrimSpace(req.NormalizedTarget) == "" || strings.TrimSpace(req.TargetKind) == "" {
		return errors.New("project, source, normalized target, and target kind are required")
	}
	if req.WindowStart != nil && req.WindowEnd != nil && req.WindowEnd.Before(*req.WindowStart) {
		return errors.New("evidence window end precedes start")
	}
	return nil
}

func validState(state string) bool {
	switch state {
	case StateObserved, StateInferred, StateModelAssisted, StateMissing, StateProviderUnavailable:
		return true
	default:
		return false
	}
}

func date(value *time.Time) pgtype.Date {
	if value == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: value.UTC(), Valid: true}
}

func mustJSON(value any) json.RawMessage {
	if value == nil {
		return json.RawMessage(`{}`)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}

func clamp01(value float64) float64 { return maxFloat(0, minFloat(1, value)) }
func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
