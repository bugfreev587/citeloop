//go:build integration

package db

import (
	"context"
	"os"
	"testing"

	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestAICallLedgerLateAccountingAndOutputReclassification exercises the
// rolling-deploy trigger and generated queries together against real Postgres.
// All schema and fixture mutations are rolled back.
func TestAICallLedgerLateAccountingAndOutputReclassification(t *testing.T) {
	dsn := os.Getenv("CITELOOP_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CITELOOP_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	migration, err := os.ReadFile("../migrations/0078_ai_call_records.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply ledger migration in rollback transaction: %v", err)
	}
	var projectID uuid.UUID
	if err := tx.QueryRow(ctx, `select id from projects order by created_at limit 1`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	q := New(tx)
	spendBefore, err := q.MonthlySpend(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	queued, err := q.CreateAICallRecord(ctx, CreateAICallRecordParams{
		ProjectID: projectID, Stage: "qa", LinkedObjectType: "integration_test", LinkedObjectID: uuid.New(),
		Provider: "runtime_route", Model: "planned", PromptVersion: "integration-v1", RequestFingerprint: "queued-start", Status: "queued",
	})
	if err != nil || queued.ProviderCalled || queued.ProviderStartedAt.Valid || queued.Status != "queued" {
		t.Fatalf("queued preflight row=%+v err=%v", queued, err)
	}
	resolvedModel := "resolved-at-boundary"
	started, err := q.MarkAICallProviderStarted(ctx, MarkAICallProviderStartedParams{ResolvedModel: &resolvedModel, ID: queued.ID, ProjectID: projectID})
	if err != nil || !started.ProviderCalled || !started.ProviderStartedAt.Valid || started.Status != "running" || started.Model != resolvedModel {
		t.Fatalf("physical provider start row=%+v err=%v", started, err)
	}
	preflight := createQueuedIntegrationAICall(t, ctx, q, projectID, "preflight-skip")
	providerNotCalled := "provider_not_called"
	skipped, err := q.FinishCanonicalAICallFenced(ctx, FinishCanonicalAICallFencedParams{
		Status: "skipped", ErrorCode: &providerNotCalled, CostUsd: pgutil.Numeric(0), ID: preflight.ID, ProjectID: projectID,
	})
	if err != nil || skipped.Status != "skipped" || skipped.ProviderCalled || skipped.ProviderStartedAt.Valid || skipped.ErrorCode == nil || *skipped.ErrorCode != providerNotCalled {
		t.Fatalf("preflight skip row=%+v err=%v", skipped, err)
	}

	reclaimed := createIntegrationAICall(t, ctx, q, projectID, "late-accounting")
	cleanupError := "stale_running_call"
	if _, err := q.FinishAICallRecordIfRunning(ctx, FinishAICallRecordIfRunningParams{ErrorCode: &cleanupError, ID: reclaimed.ID, ProjectID: projectID}); err != nil {
		t.Fatal(err)
	}
	provider, model := "tokengate", "resolved-model"
	late, err := q.FinishCanonicalAICallFenced(ctx, FinishCanonicalAICallFencedParams{
		Status: "ok", ResolvedProvider: &provider, ResolvedModel: &model,
		PromptTokens: 7, CompletionTokens: 3, TotalTokens: 10, CostUsd: pgutil.Numeric(0.125),
		ID: reclaimed.ID, ProjectID: projectID,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertNumeric := func(value pgtype.Numeric, want float64) {
		t.Helper()
		got, err := value.Float64Value()
		if err != nil || !got.Valid || got.Float64 != want {
			t.Fatalf("numeric=%+v want=%v err=%v", got, want, err)
		}
	}
	if late.Status != "failed" || late.ErrorCode == nil || *late.ErrorCode != cleanupError || late.Provider != provider || late.Model != model || late.TotalTokens != 10 {
		t.Fatalf("late accounting did not preserve cleanup verdict and provider usage: %+v", late)
	}
	assertNumeric(late.CostUsd, 0.125)

	zeroUsage := createIntegrationAICall(t, ctx, q, projectID, "late-identity")
	if _, err := q.FinishAICallRecordIfRunning(ctx, FinishAICallRecordIfRunningParams{ErrorCode: &cleanupError, ID: zeroUsage.ID, ProjectID: projectID}); err != nil {
		t.Fatal(err)
	}
	identityOnly, err := q.FinishCanonicalAICallFenced(ctx, FinishCanonicalAICallFencedParams{
		Status: "failed", ErrorCode: stringPointerIntegration("provider_failure"), ResolvedProvider: &provider, ResolvedModel: &model,
		CostUsd: pgutil.Numeric(0), ID: zeroUsage.ID, ProjectID: projectID,
	})
	if err != nil || identityOnly.Status != "failed" || identityOnly.ErrorCode == nil || *identityOnly.ErrorCode != cleanupError || identityOnly.Provider != provider || identityOnly.Model != model {
		t.Fatalf("zero-usage late provider identity was lost: row=%+v err=%v", identityOnly, err)
	}

	validOutput := createIntegrationAICall(t, ctx, q, projectID, "invalid-output")
	finished, err := q.FinishCanonicalAICallFenced(ctx, FinishCanonicalAICallFencedParams{
		Status: "ok", ResolvedProvider: &provider, ResolvedModel: &model,
		PromptTokens: 4, CompletionTokens: 2, TotalTokens: 6, CostUsd: pgutil.Numeric(0.05),
		ID: validOutput.ID, ProjectID: projectID,
	})
	if err != nil || finished.Status != "ok" {
		t.Fatalf("finish output call: row=%+v err=%v", finished, err)
	}
	invalidOutput := "invalid_output"
	reclassified, err := q.ReclassifyAICallRecordOutputFailure(ctx, ReclassifyAICallRecordOutputFailureParams{ErrorCode: &invalidOutput, ID: validOutput.ID, ProjectID: projectID})
	if err != nil {
		t.Fatal(err)
	}
	if reclassified.Status != "failed" || reclassified.ErrorCode == nil || *reclassified.ErrorCode != invalidOutput || reclassified.TotalTokens != 6 {
		t.Fatalf("invalid output reclassification failed: %+v", reclassified)
	}
	assertNumeric(reclassified.CostUsd, 0.05)
	skipReason := "provider_unavailable"
	if _, err := q.CreateSkippedAICallRecord(ctx, CreateSkippedAICallRecordParams{
		ProjectID: projectID, Stage: "qa", LinkedObjectType: validOutput.LinkedObjectType, LinkedObjectID: validOutput.LinkedObjectID,
		Provider: "none", Model: "none", PromptVersion: "integration-v1", RequestFingerprint: "skipped-provider",
		ErrorCode: &skipReason,
	}); err != nil {
		t.Fatal(err)
	}
	aggregate, err := q.AggregateAICallsForObject(ctx, AggregateAICallsForObjectParams{
		ProjectID: projectID, LinkedObjectType: validOutput.LinkedObjectType, LinkedObjectID: validOutput.LinkedObjectID,
	})
	if err != nil || aggregate.CallCount != 2 || aggregate.SkippedCount != 1 || aggregate.UnsuccessfulCount != 1 || aggregate.TotalTokens != 6 {
		t.Fatalf("recomputed object aggregate=%+v err=%v", aggregate, err)
	}
	assertNumeric(aggregate.CostUsd, 0.05)

	legacyError := "rolling old binary"
	if _, err := q.InsertGenerationRun(ctx, InsertGenerationRunParams{
		ProjectID: projectID, Agent: "writer", Input: []byte(`{}`), Output: []byte(`{}`),
		CostUsd: pgutil.Numeric(1.2), Status: "error", Error: &legacyError,
	}); err != nil {
		t.Fatal(err)
	}
	spendAfter, err := q.MonthlySpend(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if delta := pgutil.Float(spendAfter) - pgutil.Float(spendBefore); delta < 1.374999 || delta > 1.375001 {
		t.Fatalf("rolling monthly spend delta=%v, want canonical 0.175 + legacy 1.2", delta)
	}
}

func stringPointerIntegration(value string) *string { return &value }

func createIntegrationAICall(t *testing.T, ctx context.Context, q *Queries, projectID uuid.UUID, fingerprint string) AiCallRecord {
	t.Helper()
	row, err := q.CreateAICallRecord(ctx, CreateAICallRecordParams{
		ProjectID: projectID, Stage: "qa", LinkedObjectType: "integration_test", LinkedObjectID: uuid.New(),
		Provider: "planned", Model: "planned", PromptVersion: "integration-v1",
		RequestFingerprint: fingerprint, Status: "running",
	})
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func createQueuedIntegrationAICall(t *testing.T, ctx context.Context, q *Queries, projectID uuid.UUID, fingerprint string) AiCallRecord {
	t.Helper()
	row, err := q.CreateAICallRecord(ctx, CreateAICallRecordParams{
		ProjectID: projectID, Stage: "qa", LinkedObjectType: "integration_test", LinkedObjectID: uuid.New(),
		Provider: "runtime_route", Model: "planned", PromptVersion: "integration-v1",
		RequestFingerprint: fingerprint, Status: "queued",
	})
	if err != nil {
		t.Fatal(err)
	}
	return row
}
