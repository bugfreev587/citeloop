package sitefix

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
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrLifecycleConflict = errors.New("canonical Site Fix lifecycle conflict")

type GenerationCall struct {
	Provider           string
	Model              string
	PromptVersion      string
	RequestFingerprint string
}

type GenerationResult struct {
	Provider         string
	Model            string
	Status           string
	ErrorCode        string
	PromptTokens     int32
	CompletionTokens int32
	TotalTokens      int32
	CostUSD          float64
}

type ApplicationPlan struct {
	TargetURL               string
	NormalizedTargetURL     string
	OpportunityKey          string
	SourceFilePaths         json.RawMessage
	SourceMappingConfidence string
	SourceMappingReason     string
	PatchSnapshot           json.RawMessage
	DiffSnapshot            json.RawMessage
	ResolutionCriteria      json.RawMessage
	Status                  string
}

type ApplyResult struct {
	SiteFix     db.SiteFix               `json:"site_fix"`
	Application db.SiteChangeApplication `json:"application"`
}

type FixGenerator interface {
	Describe(db.SiteFix) GenerationCall
	Generate(context.Context, db.SiteFix) (ApplicationPlan, GenerationResult, error)
}

type ApplyStore interface {
	Load(context.Context, uuid.UUID, uuid.UUID) (db.SiteFix, error)
	FindApplication(context.Context, db.SiteFix) (db.SiteChangeApplication, bool, error)
	MarkPreparing(context.Context, db.SiteFix) (db.SiteFix, error)
	StartGeneration(context.Context, db.SiteFix, GenerationCall) (uuid.UUID, error)
	FinishGeneration(context.Context, db.SiteFix, uuid.UUID, GenerationResult) error
	Finalize(context.Context, db.SiteFix, ApplicationPlan) (db.SiteFix, db.SiteChangeApplication, error)
}

type ApplyService struct {
	Store     ApplyStore
	Generator FixGenerator
}

func (s ApplyService) Apply(ctx context.Context, projectID, fixID uuid.UUID) (ApplyResult, error) {
	if s.Store == nil || s.Generator == nil {
		return ApplyResult{}, errors.New("canonical Site Fix apply dependencies unavailable")
	}
	fix, err := s.Store.Load(ctx, projectID, fixID)
	if err != nil {
		return ApplyResult{}, err
	}
	if fix.ProjectID != projectID || fix.ID != fixID {
		return ApplyResult{}, ErrProjectMismatch
	}
	if app, ok, err := s.Store.FindApplication(ctx, fix); err != nil {
		return ApplyResult{}, err
	} else if ok {
		if !app.SiteFixID.Valid || app.ContentActionID.Valid || uuid.UUID(app.SiteFixID.Bytes) != fix.ID {
			return ApplyResult{}, errors.New("canonical application returned an invalid source")
		}
		return ApplyResult{SiteFix: fix, Application: app}, nil
	}
	switch fix.Status {
	case "approved":
		fix, err = s.Store.MarkPreparing(ctx, fix)
		if err != nil {
			return ApplyResult{}, err
		}
	case "preparing":
		// A previous provider attempt may have failed. Retrying creates a new,
		// append-only ai_call_record below.
	default:
		return ApplyResult{}, fmt.Errorf("%w: cannot apply from %s", ErrLifecycleConflict, fix.Status)
	}

	descriptor := s.Generator.Describe(fix)
	if strings.TrimSpace(descriptor.Provider) == "" || strings.TrimSpace(descriptor.Model) == "" ||
		strings.TrimSpace(descriptor.PromptVersion) == "" || strings.TrimSpace(descriptor.RequestFingerprint) == "" {
		return ApplyResult{}, errors.New("fix generation descriptor is incomplete")
	}
	callID, err := s.Store.StartGeneration(ctx, fix, descriptor)
	if err != nil {
		return ApplyResult{}, err
	}
	plan, generation, generateErr := s.Generator.Generate(ctx, fix)
	if generateErr != nil && generation.Status == "" {
		generation.Status = "failed"
	}
	if generateErr == nil {
		if planErr := validateApplicationPlan(plan); planErr != nil {
			generation.Status = "failed"
			generation.ErrorCode = "invalid_output"
			generateErr = planErr
		}
	}
	finishCtx, cancelFinish := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelFinish()
	if finishErr := s.Store.FinishGeneration(finishCtx, fix, callID, generation); finishErr != nil {
		if generateErr != nil {
			return ApplyResult{}, errors.Join(generateErr, finishErr)
		}
		return ApplyResult{}, finishErr
	}
	if generateErr != nil {
		return ApplyResult{}, generateErr
	}
	if generation.Status != "ok" && generation.Status != "partial" && generation.Status != "skipped" {
		return ApplyResult{}, fmt.Errorf("fix generation ended in %q", generation.Status)
	}
	fix, app, err := s.Store.Finalize(ctx, fix, plan)
	if err != nil {
		return ApplyResult{}, err
	}
	if !app.SiteFixID.Valid || app.ContentActionID.Valid || uuid.UUID(app.SiteFixID.Bytes) != fix.ID {
		return ApplyResult{}, errors.New("canonical application returned an invalid source")
	}
	return ApplyResult{SiteFix: fix, Application: app}, nil
}

func validateApplicationPlan(plan ApplicationPlan) error {
	if strings.TrimSpace(plan.TargetURL) == "" || strings.TrimSpace(plan.NormalizedTargetURL) == "" || strings.TrimSpace(plan.OpportunityKey) == "" {
		return errors.New("generated canonical application is missing target identity")
	}
	switch plan.Status {
	case "ready_for_pr", "manual_apply_required", "source_mapping_required", "draft_ready":
	default:
		return fmt.Errorf("unsupported canonical application status %q", plan.Status)
	}
	for _, raw := range []json.RawMessage{plan.SourceFilePaths, plan.PatchSnapshot, plan.DiffSnapshot, plan.ResolutionCriteria} {
		if len(raw) == 0 || !json.Valid(raw) {
			return errors.New("generated canonical application contains invalid JSON")
		}
	}
	return nil
}

type PostgresApplyStore struct {
	Pool *pgxpool.Pool
	Q    *db.Queries
}

func (s PostgresApplyStore) Load(ctx context.Context, projectID, fixID uuid.UUID) (db.SiteFix, error) {
	return s.Q.GetCanonicalSiteFix(ctx, db.GetCanonicalSiteFixParams{ID: fixID, ProjectID: projectID})
}

func (s PostgresApplyStore) MarkPreparing(ctx context.Context, fix db.SiteFix) (db.SiteFix, error) {
	if _, err := s.Q.MarkCanonicalSiteFixPreparing(ctx, db.MarkCanonicalSiteFixPreparingParams{SiteFixID: fix.ID, ProjectID: fix.ProjectID}); err != nil {
		return db.SiteFix{}, lifecycleError(err)
	}
	return s.Load(ctx, fix.ProjectID, fix.ID)
}

func (s PostgresApplyStore) FindApplication(ctx context.Context, fix db.SiteFix) (db.SiteChangeApplication, bool, error) {
	app, err := s.Q.GetLatestCanonicalSiteFixApplication(ctx, db.GetLatestCanonicalSiteFixApplicationParams{ProjectID: fix.ProjectID, SiteFixID: validPGUUID(fix.ID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.SiteChangeApplication{}, false, nil
	}
	return app, err == nil, err
}

func (s PostgresApplyStore) StartGeneration(ctx context.Context, fix db.SiteFix, call GenerationCall) (uuid.UUID, error) {
	row, err := s.Q.CreateAICallRecord(ctx, db.CreateAICallRecordParams{
		ProjectID: fix.ProjectID, Stage: "fix_generation", LinkedObjectType: "site_fix", LinkedObjectID: fix.ID,
		Provider: call.Provider, Model: call.Model, PromptVersion: call.PromptVersion,
		RequestFingerprint: call.RequestFingerprint, Status: "running",
	})
	if err != nil {
		return uuid.Nil, err
	}
	return row.ID, nil
}

func (s PostgresApplyStore) FinishGeneration(ctx context.Context, fix db.SiteFix, callID uuid.UUID, result GenerationResult) error {
	cost := pgtype.Numeric{}
	if err := cost.Scan(fmt.Sprintf("%.8f", max(result.CostUSD, 0))); err != nil {
		return err
	}
	var errorCode *string
	if strings.TrimSpace(result.ErrorCode) != "" {
		errorCode = &result.ErrorCode
	}
	_, err := s.Q.FinishAICallRecord(ctx, db.FinishAICallRecordParams{
		Status: result.Status, ErrorCode: errorCode, ResolvedProvider: emptyStringPtr(result.Provider), ResolvedModel: emptyStringPtr(result.Model), PromptTokens: max(result.PromptTokens, 0),
		CompletionTokens: max(result.CompletionTokens, 0), TotalTokens: max(result.TotalTokens, 0),
		CostUsd: cost, ID: callID, ProjectID: fix.ProjectID,
	})
	return err
}

func emptyStringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func (s PostgresApplyStore) Finalize(ctx context.Context, fix db.SiteFix, plan ApplicationPlan) (db.SiteFix, db.SiteChangeApplication, error) {
	if s.Pool == nil {
		return db.SiteFix{}, db.SiteChangeApplication{}, errors.New("canonical Site Fix database unavailable")
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return db.SiteFix{}, db.SiteChangeApplication{}, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	if _, err := q.MarkCanonicalSiteFixReadyToApply(ctx, db.MarkCanonicalSiteFixReadyToApplyParams{SiteFixID: fix.ID, ProjectID: fix.ProjectID}); err != nil {
		return db.SiteFix{}, db.SiteChangeApplication{}, lifecycleError(err)
	}
	if _, err := q.ClaimCanonicalSiteFixApplying(ctx, db.ClaimCanonicalSiteFixApplyingParams{SiteFixID: fix.ID, ProjectID: fix.ProjectID}); err != nil {
		return db.SiteFix{}, db.SiteChangeApplication{}, lifecycleError(err)
	}
	app, err := q.CreateCanonicalSiteFixApplication(ctx, db.CreateCanonicalSiteFixApplicationParams{
		ID: uuid.New(), ProjectID: fix.ProjectID, SiteFixID: validPGUUID(fix.ID),
		TargetUrl: plan.TargetURL, NormalizedTargetUrl: plan.NormalizedTargetURL,
		OpportunityKey: plan.OpportunityKey, SourceFilePaths: plan.SourceFilePaths,
		SourceMappingConfidence: plan.SourceMappingConfidence, SourceMappingReason: plan.SourceMappingReason,
		PatchSnapshot: plan.PatchSnapshot, DiffSnapshot: plan.DiffSnapshot,
		ResolutionCriteria: plan.ResolutionCriteria, Status: plan.Status,
	})
	if err != nil {
		return db.SiteFix{}, db.SiteChangeApplication{}, lifecycleError(err)
	}
	updated, err := q.GetCanonicalSiteFix(ctx, db.GetCanonicalSiteFixParams{ID: fix.ID, ProjectID: fix.ProjectID})
	if err != nil {
		return db.SiteFix{}, db.SiteChangeApplication{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.SiteFix{}, db.SiteChangeApplication{}, err
	}
	return updated, app, nil
}

func lifecycleError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLifecycleConflict
	}
	return err
}

func validPGUUID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }

// LLMApplicationGenerator grounds a narrow manual/PR-ready handoff in the
// canonical Site Fix snapshots. It performs no database work.
type LLMApplicationGenerator struct {
	Provider llm.Provider
	Model    string
}

// DeterministicApplicationGenerator is the default for projects that have not
// explicitly granted Doctor provider-call authority. It creates a reviewable
// manual handoff from already-approved canonical snapshots and records the
// generation attempt as skipped rather than calling an AI provider.
type DeterministicApplicationGenerator struct{}

func (DeterministicApplicationGenerator) Describe(fix db.SiteFix) GenerationCall {
	payload := append(append([]byte{}, fix.ProposedFix...), fix.AcceptanceTests...)
	sum := sha256.Sum256(payload)
	return GenerationCall{Provider: "none", Model: "none", PromptVersion: "doctor-fix-deterministic-v1", RequestFingerprint: hex.EncodeToString(sum[:])}
}

func (DeterministicApplicationGenerator) Generate(_ context.Context, fix db.SiteFix) (ApplicationPlan, GenerationResult, error) {
	target, err := firstTargetURL(fix.TargetUrls)
	if err != nil {
		return ApplicationPlan{}, GenerationResult{Status: "skipped", ErrorCode: "invalid_target"}, err
	}
	criteria, err := json.Marshal(map[string]any{"verification_mode": "manual_evidence", "acceptance_tests": json.RawMessage(fix.AcceptanceTests)})
	if err != nil {
		return ApplicationPlan{}, GenerationResult{Status: "skipped", ErrorCode: "invalid_snapshot"}, err
	}
	return ApplicationPlan{
		TargetURL: target, NormalizedTargetURL: target, OpportunityKey: "doctor:" + fix.ID.String(),
		SourceFilePaths: json.RawMessage(`[]`), SourceMappingConfidence: "low",
		SourceMappingReason: "Doctor AI assistance is not authorized; use the approved deterministic manual handoff.",
		PatchSnapshot:       rawJSONObject(fix.ProposedFix), DiffSnapshot: rawJSONObject(fix.ProposedFix),
		ResolutionCriteria: criteria, Status: "manual_apply_required",
	}, GenerationResult{Provider: "none", Model: "none", Status: "skipped", ErrorCode: "doctor_ai_not_authorized"}, nil
}

func rawJSONObject(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || !json.Valid(raw) {
		return json.RawMessage(`{}`)
	}
	return raw
}

func (g LLMApplicationGenerator) Describe(fix db.SiteFix) GenerationCall {
	payload := append(append(append([]byte{}, fix.ProposedFix...), fix.EvidenceSnapshot...), fix.AcceptanceTests...)
	sum := sha256.Sum256(payload)
	return GenerationCall{Provider: "tokengate", Model: firstNonEmpty(g.Model, llm.DefaultTokenGateModel), PromptVersion: "doctor-fix-generation-v1", RequestFingerprint: hex.EncodeToString(sum[:])}
}

func (g LLMApplicationGenerator) Generate(ctx context.Context, fix db.SiteFix) (ApplicationPlan, GenerationResult, error) {
	if g.Provider == nil {
		return ApplicationPlan{}, GenerationResult{Provider: "none", Model: "none", Status: "failed", ErrorCode: "provider_unavailable"}, errors.New("Doctor fix generation provider is unavailable")
	}
	target, err := firstTargetURL(fix.TargetUrls)
	if err != nil {
		return ApplicationPlan{}, GenerationResult{Status: "failed", ErrorCode: "invalid_target"}, err
	}
	prompt, _ := json.Marshal(map[string]any{"target_url": target, "evidence": fix.EvidenceSnapshot, "proposed_fix": fix.ProposedFix, "acceptance_tests": fix.AcceptanceTests})
	resp, err := g.Provider.Complete(ctx, llm.CompletionReq{
		System:  "You prepare a narrow Doctor Site Fix for an existing surface. Do not create new content, claims, routes, offers, or growth hypotheses. Return JSON only.",
		Prompt:  "Return a JSON object with patch_snapshot, diff_snapshot, resolution_criteria, source_file_paths, source_mapping_confidence, and source_mapping_reason. Preserve the target URL.\n" + string(prompt),
		Purpose: llm.PurposeSiteFix, Model: firstNonEmpty(g.Model, llm.DefaultTokenGateModel), JSON: true, MaxTokens: 1400,
	})
	result := GenerationResult{Provider: firstNonEmpty(resp.Provider, "tokengate"), Model: firstNonEmpty(resp.Model, g.Model), Status: "ok", PromptTokens: int32(max(resp.PromptTokens, 0)), CompletionTokens: int32(max(resp.CompletionTokens, 0)), TotalTokens: int32(max(resp.Tokens, 0)), CostUSD: resp.CostUSD}
	if err != nil {
		result.Status, result.ErrorCode = "failed", "provider_error"
		return ApplicationPlan{}, result, err
	}
	var generated struct {
		PatchSnapshot           json.RawMessage `json:"patch_snapshot"`
		DiffSnapshot            json.RawMessage `json:"diff_snapshot"`
		ResolutionCriteria      json.RawMessage `json:"resolution_criteria"`
		SourceFilePaths         json.RawMessage `json:"source_file_paths"`
		SourceMappingConfidence string          `json:"source_mapping_confidence"`
		SourceMappingReason     string          `json:"source_mapping_reason"`
	}
	if err := decodeJSONObject(resp.Text, &generated); err != nil {
		result.Status, result.ErrorCode = "failed", "invalid_response"
		return ApplicationPlan{}, result, err
	}
	plan := ApplicationPlan{
		TargetURL: target, NormalizedTargetURL: target, OpportunityKey: "doctor:" + fix.ID.String(),
		SourceFilePaths: generated.SourceFilePaths, SourceMappingConfidence: firstNonEmpty(generated.SourceMappingConfidence, "low"),
		SourceMappingReason: generated.SourceMappingReason, PatchSnapshot: generated.PatchSnapshot,
		DiffSnapshot: generated.DiffSnapshot, ResolutionCriteria: generated.ResolutionCriteria,
		Status: "manual_apply_required",
	}
	var paths []string
	if json.Unmarshal(generated.SourceFilePaths, &paths) == nil && len(paths) == 1 && strings.TrimSpace(paths[0]) != "" {
		plan.Status = "ready_for_pr"
	}
	return plan, result, nil
}

func firstTargetURL(raw json.RawMessage) (string, error) {
	var targets []string
	if json.Unmarshal(raw, &targets) != nil || len(targets) == 0 || strings.TrimSpace(targets[0]) == "" {
		return "", errors.New("canonical Site Fix target URL is missing")
	}
	return strings.TrimSpace(targets[0]), nil
}

func decodeJSONObject(text string, out any) error {
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return errors.New("provider response did not contain a JSON object")
	}
	return json.Unmarshal([]byte(text[start:end+1]), out)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
