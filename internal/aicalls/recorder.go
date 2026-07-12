// Package aicalls owns the canonical, provider-call-level accounting contract.
package aicalls

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type Store interface {
	CreateAICallRecord(context.Context, db.CreateAICallRecordParams) (db.AiCallRecord, error)
	CreateSkippedAICallRecord(context.Context, db.CreateSkippedAICallRecordParams) (db.AiCallRecord, error)
	FinishAICallRecord(context.Context, db.FinishAICallRecordParams) (db.AiCallRecord, error)
	FinishCanonicalAICallFenced(context.Context, db.FinishCanonicalAICallFencedParams) (db.AiCallRecord, error)
	MarkAICallProviderStarted(context.Context, db.MarkAICallProviderStartedParams) (db.AiCallRecord, error)
	ReclassifyAICallRecordOutputFailure(context.Context, db.ReclassifyAICallRecordOutputFailureParams) (db.AiCallRecord, error)
	GetAICallRecord(context.Context, db.GetAICallRecordParams) (db.AiCallRecord, error)
	GetLatestAICallForRequest(context.Context, db.GetLatestAICallForRequestParams) (db.AiCallRecord, error)
	AggregateAICallsForObject(context.Context, db.AggregateAICallsForObjectParams) (db.AggregateAICallsForObjectRow, error)
}

type Recorder struct{ Store Store }

func New(store Store) Recorder { return Recorder{Store: store} }

type Spec struct {
	ProjectID          uuid.UUID
	RunID              uuid.UUID
	Stage              string
	LinkedObjectType   string
	LinkedObjectID     uuid.UUID
	Provider           string
	Model              string
	PromptVersion      string
	RequestFingerprint string
	ParentCallID       uuid.UUID
	CausedByCallID     uuid.UUID
}

type Finish struct {
	Status           string
	ErrorCode        string
	ResolvedProvider string
	ResolvedModel    string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CostUSD          float64
}

type Completion struct {
	Call     db.AiCallRecord
	Response llm.CompletionResp
}

// ExistingAttemptObserver turns a predeclared queued ledger row into a running
// physical call only when the provider boundary is actually crossed.
type ExistingAttemptObserver struct {
	Store     Store
	ProjectID uuid.UUID
	CallID    uuid.UUID
	started   bool
}

func NewExistingAttemptObserver(store Store, projectID, callID uuid.UUID) *ExistingAttemptObserver {
	return &ExistingAttemptObserver{Store: store, ProjectID: projectID, CallID: callID}
}

func (o *ExistingAttemptObserver) StartAttempt(ctx context.Context, model string) (string, error) {
	if o == nil || o.Store == nil || o.ProjectID == uuid.Nil || o.CallID == uuid.Nil {
		return "", errors.New("existing AI attempt observer is incomplete")
	}
	if o.started {
		return "", errors.New("existing AI call row cannot represent multiple physical attempts")
	}
	_, err := o.Store.MarkAICallProviderStarted(ctx, db.MarkAICallProviderStartedParams{
		ResolvedModel: optional(model), ID: o.CallID, ProjectID: o.ProjectID,
	})
	if err != nil {
		return "", err
	}
	o.started = true
	return o.CallID.String(), nil
}

func (*ExistingAttemptObserver) FinishAttempt(context.Context, string, llm.CompletionResp, error) error {
	return nil
}

func (o *ExistingAttemptObserver) Started() bool { return o != nil && o.started }

func Fingerprint(req llm.CompletionReq) string {
	raw, _ := json.Marshal(struct {
		System                  string                `json:"system"`
		Prompt                  string                `json:"prompt"`
		Purpose                 llm.CompletionPurpose `json:"purpose"`
		Model                   string                `json:"model"`
		MaxTokens               int                   `json:"max_tokens"`
		Temperature             float64               `json:"temperature"`
		JSON                    bool                  `json:"json"`
		DisableProviderFallback bool                  `json:"disable_provider_fallback"`
	}{req.System, req.Prompt, req.Purpose, req.Model, req.MaxTokens, req.Temperature, req.JSON, req.DisableProviderFallback})
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (r Recorder) Start(ctx context.Context, spec Spec) (db.AiCallRecord, error) {
	if err := validateSpec(spec); err != nil {
		return db.AiCallRecord{}, err
	}
	if r.Store == nil {
		return db.AiCallRecord{}, errors.New("AI call ledger store is required")
	}
	return r.Store.CreateAICallRecord(ctx, db.CreateAICallRecordParams{
		ProjectID: spec.ProjectID, RunID: pgUUID(spec.RunID), Stage: spec.Stage,
		LinkedObjectType: spec.LinkedObjectType, LinkedObjectID: spec.LinkedObjectID,
		Provider: spec.Provider, Model: spec.Model, PromptVersion: spec.PromptVersion,
		RequestFingerprint: spec.RequestFingerprint, Status: "running", ParentCallID: pgUUID(spec.ParentCallID), CausedByCallID: pgUUID(spec.CausedByCallID),
	})
}

func (r Recorder) Skip(ctx context.Context, spec Spec, reason string) (db.AiCallRecord, error) {
	if err := validateSpec(spec); err != nil {
		return db.AiCallRecord{}, err
	}
	if r.Store == nil {
		return db.AiCallRecord{}, errors.New("AI call ledger store is required")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return db.AiCallRecord{}, errors.New("skipped AI call requires a reason")
	}
	return r.Store.CreateSkippedAICallRecord(ctx, db.CreateSkippedAICallRecordParams{
		ProjectID: spec.ProjectID, RunID: pgUUID(spec.RunID), Stage: spec.Stage,
		LinkedObjectType: spec.LinkedObjectType, LinkedObjectID: spec.LinkedObjectID,
		Provider: spec.Provider, Model: spec.Model, PromptVersion: spec.PromptVersion,
		RequestFingerprint: spec.RequestFingerprint, ErrorCode: &reason, ParentCallID: pgUUID(spec.ParentCallID), CausedByCallID: pgUUID(spec.CausedByCallID),
	})
}

func (r Recorder) Finish(ctx context.Context, callID, projectID uuid.UUID, finish Finish) (db.AiCallRecord, error) {
	if r.Store == nil {
		return db.AiCallRecord{}, errors.New("AI call ledger store is required")
	}
	if callID == uuid.Nil || projectID == uuid.Nil {
		return db.AiCallRecord{}, errors.New("AI call and project IDs are required")
	}
	if finish.Status == "skipped" {
		return db.AiCallRecord{}, errors.New("use Skip when no provider call occurred")
	}
	if finish.Status != "ok" && finish.Status != "partial" && finish.Status != "failed" {
		return db.AiCallRecord{}, fmt.Errorf("unsupported terminal AI call status %q", finish.Status)
	}
	if (finish.Status == "failed" || finish.Status == "partial") && strings.TrimSpace(finish.ErrorCode) == "" {
		return db.AiCallRecord{}, errors.New("failed or partial AI call requires an error code")
	}
	params := db.FinishCanonicalAICallFencedParams{
		Status: finish.Status, ResolvedProvider: optional(finish.ResolvedProvider), ResolvedModel: optional(finish.ResolvedModel),
		ErrorCode: optional(finish.ErrorCode), PromptTokens: bounded(finish.PromptTokens),
		CompletionTokens: bounded(finish.CompletionTokens), TotalTokens: bounded(finish.TotalTokens),
		CostUsd: pgutil.Numeric(max(finish.CostUSD, 0)), ID: callID, ProjectID: projectID,
	}
	var lastErr error
	for range 3 {
		finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		row, err := r.Store.FinishCanonicalAICallFenced(finishCtx, params)
		cancel()
		if err == nil {
			if finishAppliedOrTerminalized(row, finish) {
				return row, nil
			}
			return row, fmt.Errorf("AI call terminal accounting conflicts with provider result: call=%s status=%s tokens=%d/%d/%d cost=%s", row.ID, row.Status, row.PromptTokens, row.CompletionTokens, row.TotalTokens, row.CostUsd.Int)
		}
		lastErr = errors.Join(lastErr, err)
	}
	getCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	row, getErr := r.Store.GetAICallRecord(getCtx, db.GetAICallRecordParams{ID: callID, ProjectID: projectID})
	cancel()
	if getErr == nil && finishAppliedOrTerminalized(row, finish) {
		return row, nil
	}
	return row, errors.Join(lastErr, getErr)
}

func finishAppliedOrTerminalized(row db.AiCallRecord, finish Finish) bool {
	if row.PromptTokens != bounded(finish.PromptTokens) || row.CompletionTokens != bounded(finish.CompletionTokens) || row.TotalTokens != bounded(finish.TotalTokens) {
		return false
	}
	if strings.TrimSpace(finish.ResolvedProvider) != "" && row.Provider != strings.TrimSpace(finish.ResolvedProvider) {
		return false
	}
	if strings.TrimSpace(finish.ResolvedModel) != "" && row.Model != strings.TrimSpace(finish.ResolvedModel) {
		return false
	}
	actualCost, err := row.CostUsd.Float64Value()
	wantCost, wantCostErr := pgutil.Numeric(max(finish.CostUSD, 0)).Float64Value()
	if err != nil || wantCostErr != nil || !actualCost.Valid || !wantCost.Valid || math.Abs(actualCost.Float64-wantCost.Float64) > 0.00000001 {
		return false
	}
	if row.Status == "failed" && row.ErrorCode != nil && isCleanupTerminalError(*row.ErrorCode) {
		return true
	}
	if row.Status != finish.Status {
		return false
	}
	wantError := strings.TrimSpace(finish.ErrorCode)
	return (row.ErrorCode == nil) == (wantError == "") && (row.ErrorCode == nil || *row.ErrorCode == wantError)
}

func isCleanupTerminalError(code string) bool {
	switch strings.TrimSpace(code) {
	case "processing_reclaimed", "doctor_ai_marker_rejected", "stale_running_call":
		return true
	default:
		return false
	}
}

func (r Recorder) Latest(ctx context.Context, spec Spec) (db.AiCallRecord, error) {
	if r.Store == nil {
		return db.AiCallRecord{}, errors.New("AI call ledger store is required")
	}
	return r.Store.GetLatestAICallForRequest(ctx, db.GetLatestAICallForRequestParams{
		ProjectID: spec.ProjectID, Stage: spec.Stage, LinkedObjectType: spec.LinkedObjectType,
		LinkedObjectID: spec.LinkedObjectID, RequestFingerprint: spec.RequestFingerprint,
	})
}

// FailOutput reclassifies a provider-successful call when its returned payload
// cannot satisfy the versioned output contract. Usage remains immutable.
func (r Recorder) FailOutput(ctx context.Context, callID, projectID uuid.UUID, errorCode string) (db.AiCallRecord, error) {
	if r.Store == nil {
		return db.AiCallRecord{}, errors.New("AI call ledger store is required")
	}
	errorCode = strings.TrimSpace(errorCode)
	if callID == uuid.Nil || projectID == uuid.Nil || errorCode == "" {
		return db.AiCallRecord{}, errors.New("AI call, project, and output error code are required")
	}
	return r.Store.ReclassifyAICallRecordOutputFailure(ctx, db.ReclassifyAICallRecordOutputFailureParams{
		ErrorCode: &errorCode, ID: callID, ProjectID: projectID,
	})
}

func (r Recorder) Complete(ctx context.Context, spec Spec, provider llm.Provider, req llm.CompletionReq) (Completion, error) {
	if spec.RequestFingerprint == "" {
		spec.RequestFingerprint = Fingerprint(req)
	}
	if provider == nil {
		call, err := r.Skip(ctx, spec, "provider_unavailable")
		if err != nil {
			return Completion{}, err
		}
		return Completion{Call: call}, errors.New("AI provider is unavailable")
	}
	if _, observable := provider.(llm.AttemptObservable); observable {
		observer := &ledgerAttemptObserver{recorder: r, spec: spec}
		req.AttemptObserver = observer
		resp, providerErr := provider.Complete(ctx, req)
		if observer.count == 0 {
			call, skipErr := r.Skip(context.WithoutCancel(ctx), spec, "provider_not_called")
			return Completion{Call: call, Response: resp}, errors.Join(providerErr, errors.New("provider returned without reporting a physical attempt"), skipErr)
		}
		return Completion{Call: observer.lastCall, Response: resp}, providerErr
	}
	call, err := r.Start(ctx, spec)
	if err != nil {
		return Completion{}, err
	}
	resp, providerErr := provider.Complete(ctx, req)
	status, errorCode := "ok", ""
	if providerErr != nil {
		status, errorCode = "failed", "provider_failure"
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	finished, finishErr := r.Finish(finishCtx, call.ID, spec.ProjectID, Finish{
		Status: status, ErrorCode: errorCode, ResolvedProvider: resp.Provider, ResolvedModel: resp.Model,
		PromptTokens: resp.PromptTokens, CompletionTokens: resp.CompletionTokens, TotalTokens: resp.Tokens, CostUSD: resp.CostUSD,
	})
	if finishErr != nil {
		return Completion{Call: call, Response: resp}, errors.Join(providerErr, finishErr)
	}
	return Completion{Call: finished, Response: resp}, providerErr
}

type ledgerAttemptObserver struct {
	recorder Recorder
	spec     Spec
	lastID   uuid.UUID
	lastCall db.AiCallRecord
	count    int
}

func (o *ledgerAttemptObserver) StartAttempt(ctx context.Context, model string) (string, error) {
	spec := o.spec
	if o.lastID != uuid.Nil {
		spec.ParentCallID = o.lastID
		spec.CausedByCallID = uuid.Nil
	}
	if strings.TrimSpace(model) != "" {
		spec.Model = strings.TrimSpace(model)
	}
	row, err := o.recorder.Start(ctx, spec)
	if err != nil {
		return "", err
	}
	o.lastID, o.lastCall = row.ID, row
	o.count++
	return row.ID.String(), nil
}

func (o *ledgerAttemptObserver) FinishAttempt(ctx context.Context, attemptID string, resp llm.CompletionResp, providerErr error) error {
	callID, err := uuid.Parse(attemptID)
	if err != nil {
		return err
	}
	status, errorCode := "ok", ""
	if providerErr != nil {
		status, errorCode = "failed", "provider_failure"
	}
	row, err := o.recorder.Finish(ctx, callID, o.spec.ProjectID, Finish{
		Status: status, ErrorCode: errorCode, ResolvedProvider: resp.Provider, ResolvedModel: resp.Model,
		PromptTokens: resp.PromptTokens, CompletionTokens: resp.CompletionTokens, TotalTokens: resp.Tokens, CostUSD: resp.CostUSD,
	})
	if err == nil {
		o.lastCall = row
	}
	return err
}

func (r Recorder) Aggregate(ctx context.Context, projectID uuid.UUID, objectType string, objectID uuid.UUID) (db.AggregateAICallsForObjectRow, error) {
	if r.Store == nil {
		return db.AggregateAICallsForObjectRow{}, errors.New("AI call ledger store is required")
	}
	return r.Store.AggregateAICallsForObject(ctx, db.AggregateAICallsForObjectParams{
		ProjectID: projectID, LinkedObjectType: objectType, LinkedObjectID: objectID,
	})
}

func validateSpec(spec Spec) error {
	if spec.ProjectID == uuid.Nil || spec.LinkedObjectID == uuid.Nil || strings.TrimSpace(spec.LinkedObjectType) == "" {
		return errors.New("project and linked object are required")
	}
	for name, value := range map[string]string{
		"stage": spec.Stage, "provider": spec.Provider, "model": spec.Model,
		"prompt version": spec.PromptVersion, "request fingerprint": spec.RequestFingerprint,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("AI call %s is required", name)
		}
	}
	return nil
}

func pgUUID(value uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: value, Valid: value != uuid.Nil}
}

func optional(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func bounded(value int) int32 {
	if value <= 0 {
		return 0
	}
	if value > int(^uint32(0)>>1) {
		return int32(^uint32(0) >> 1)
	}
	return int32(value)
}
