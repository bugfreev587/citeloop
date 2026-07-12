//go:build integration

package db

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOpportunityFindingStageCheckpointFencingAndRecovery(t *testing.T) {
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
	migration, err := os.ReadFile("../migrations/0079_opportunity_finding_stages.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply checkpoint migration in rollback transaction: %v", err)
	}
	var projectID uuid.UUID
	if err := tx.QueryRow(ctx, `select id from projects order by created_at limit 1`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	workflowID := uuid.New()
	if _, err := tx.Exec(ctx, `insert into workflow_events (id,project_id,event_type,dedupe_key,payload) values ($1,$2,'opportunity_finding.requested',$3,'{}')`, workflowID, projectID, "integration-opportunity-finding:"+workflowID.String()); err != nil {
		t.Fatal(err)
	}
	q := New(tx)
	owner := uuid.New()
	checkpoint, err := q.AcquireOpportunityFindingStage(ctx, AcquireOpportunityFindingStageParams{
		ID: uuid.New(), ProjectID: projectID, WorkflowEventID: workflowID,
		Stage: "ai_hypotheses", StageOrder: 3, RequestFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		OwnerToken: owner, LeaseExpiresAt: pgutil.TS(time.Now().Add(30 * time.Minute)),
	})
	if err != nil || checkpoint.Status != "running" || checkpoint.AttemptNumber != 1 || checkpoint.OwnerToken != owner {
		t.Fatalf("initial checkpoint=%+v err=%v", checkpoint, err)
	}
	contender := uuid.New()
	reused, err := q.AcquireOpportunityFindingStage(ctx, AcquireOpportunityFindingStageParams{
		ID: uuid.New(), ProjectID: projectID, WorkflowEventID: workflowID,
		Stage: "ai_hypotheses", StageOrder: 3, RequestFingerprint: checkpoint.RequestFingerprint,
		OwnerToken: contender, LeaseExpiresAt: pgutil.TS(time.Now().Add(30 * time.Minute)),
	})
	if err != nil || reused.OwnerToken != owner || reused.AttemptNumber != 1 {
		t.Fatalf("live checkpoint was stolen: row=%+v err=%v", reused, err)
	}
	if _, err := q.FinishOpportunityFindingStage(ctx, FinishOpportunityFindingStageParams{
		Status: "succeeded", OutputSummary: []byte(`{"provider_calls":1}`), ID: checkpoint.ID,
		ProjectID: projectID, WorkflowEventID: workflowID, Stage: checkpoint.Stage, OwnerToken: contender,
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("losing owner finalized checkpoint: %v", err)
	}
	finished, err := q.FinishOpportunityFindingStage(ctx, FinishOpportunityFindingStageParams{
		Status: "succeeded", OutputSummary: []byte(`{"provider_calls":1}`), ID: checkpoint.ID,
		ProjectID: projectID, WorkflowEventID: workflowID, Stage: checkpoint.Stage, OwnerToken: owner,
	})
	if err != nil || finished.Status != "succeeded" {
		t.Fatalf("finish checkpoint=%+v err=%v", finished, err)
	}
	terminal, err := q.AcquireOpportunityFindingStage(ctx, AcquireOpportunityFindingStageParams{
		ID: uuid.New(), ProjectID: projectID, WorkflowEventID: workflowID,
		Stage: "ai_hypotheses", StageOrder: 3, RequestFingerprint: checkpoint.RequestFingerprint,
		OwnerToken: contender, LeaseExpiresAt: pgutil.TS(time.Now().Add(30 * time.Minute)),
	})
	if err != nil || terminal.Status != "succeeded" || terminal.OwnerToken != owner || terminal.AttemptNumber != 1 {
		t.Fatalf("terminal checkpoint was repeated: row=%+v err=%v", terminal, err)
	}

	staleOwner := uuid.New()
	stale, err := q.AcquireOpportunityFindingStage(ctx, AcquireOpportunityFindingStageParams{
		ID: uuid.New(), ProjectID: projectID, WorkflowEventID: workflowID,
		Stage: "materialization", StageOrder: 5, RequestFingerprint: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		OwnerToken: staleOwner, LeaseExpiresAt: pgutil.TS(time.Now().Add(-time.Minute)),
	})
	if err != nil {
		t.Fatal(err)
	}
	recoveryOwner := uuid.New()
	recovered, err := q.AcquireOpportunityFindingStage(ctx, AcquireOpportunityFindingStageParams{
		ID: uuid.New(), ProjectID: projectID, WorkflowEventID: workflowID,
		Stage: stale.Stage, StageOrder: stale.StageOrder, RequestFingerprint: stale.RequestFingerprint,
		OwnerToken: recoveryOwner, LeaseExpiresAt: pgutil.TS(time.Now().Add(30 * time.Minute)),
	})
	if err != nil || recovered.OwnerToken != recoveryOwner || recovered.AttemptNumber != 2 {
		t.Fatalf("stale checkpoint recovery=%+v err=%v", recovered, err)
	}
}
