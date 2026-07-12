package opportunityfinding

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type memoryCheckpointStore struct {
	rows       map[string]db.OpportunityFindingStageCheckpoint
	leases     map[string]pgtype.Timestamptz
	finishErr  error
	acquireErr error
}

func newMemoryCheckpointStore() *memoryCheckpointStore {
	return &memoryCheckpointStore{rows: map[string]db.OpportunityFindingStageCheckpoint{}, leases: map[string]pgtype.Timestamptz{}}
}

func (s *memoryCheckpointStore) AcquireOpportunityFindingStage(_ context.Context, arg db.AcquireOpportunityFindingStageParams) (db.OpportunityFindingStageCheckpoint, error) {
	if s.acquireErr != nil {
		return db.OpportunityFindingStageCheckpoint{}, s.acquireErr
	}
	if row, ok := s.rows[arg.Stage]; ok {
		return row, nil
	}
	row := db.OpportunityFindingStageCheckpoint{
		ID: arg.ID, ProjectID: arg.ProjectID, WorkflowEventID: arg.WorkflowEventID,
		Stage: arg.Stage, StageOrder: arg.StageOrder, RequestFingerprint: arg.RequestFingerprint,
		Status: "running", AttemptNumber: 1, OwnerToken: arg.OwnerToken,
		LeaseExpiresAt: arg.LeaseExpiresAt, OutputSummary: json.RawMessage(`{}`),
	}
	s.leases[arg.Stage] = arg.LeaseExpiresAt
	s.rows[arg.Stage] = row
	return row, nil
}

func TestRunCheckpointedOpportunityFindingRefreshesLeaseForEachStage(t *testing.T) {
	store := newMemoryCheckpointStore()
	now := time.Date(2026, 7, 12, 1, 0, 0, 0, time.UTC)
	request := WorkflowRequest{
		ProjectID: uuid.New(), WorkflowEventID: uuid.New(), Inputs: map[string]any{"trigger": "manual"},
		Now: func() time.Time { return now },
	}
	_, err := RunCheckpointedWorkflow(context.Background(), store, request, func(_ context.Context, _ Stage, _ []StageProgress) StageOutcome {
		now = now.Add(5 * time.Minute)
		return StageOutcome{}
	})
	if err != nil {
		t.Fatal(err)
	}
	first := store.leases[string(StageEvidenceRefresh)].Time
	second := store.leases[string(StageDeterministicSignals)].Time
	if !second.After(first) || second.Sub(first) != 5*time.Minute {
		t.Fatalf("stage leases were not refreshed: first=%s second=%s", first, second)
	}
}

func (s *memoryCheckpointStore) FinishOpportunityFindingStage(_ context.Context, arg db.FinishOpportunityFindingStageParams) (db.OpportunityFindingStageCheckpoint, error) {
	if s.finishErr != nil {
		return db.OpportunityFindingStageCheckpoint{}, s.finishErr
	}
	row := s.rows[arg.Stage]
	if row.OwnerToken != arg.OwnerToken || row.Status != "running" {
		return db.OpportunityFindingStageCheckpoint{}, errors.New("checkpoint fenced")
	}
	row.Status = arg.Status
	row.OutputSummary = arg.OutputSummary
	row.Error = arg.Error
	row.LeaseExpiresAt = pgtype.Timestamptz{}
	row.FinishedAt = pgtype.Timestamptz{Valid: true}
	s.rows[arg.Stage] = row
	return row, nil
}

func (s *memoryCheckpointStore) ListOpportunityFindingStages(_ context.Context, _ db.ListOpportunityFindingStagesParams) ([]db.OpportunityFindingStageCheckpoint, error) {
	rows := make([]db.OpportunityFindingStageCheckpoint, 0, len(OrderedStages))
	for _, stage := range OrderedStages {
		if row, ok := s.rows[string(stage)]; ok {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func TestRunCheckpointedOpportunityFindingRunsEveryStageOnce(t *testing.T) {
	store := newMemoryCheckpointStore()
	request := WorkflowRequest{ProjectID: uuid.New(), WorkflowEventID: uuid.New(), Inputs: map[string]any{"trigger": "manual", "version": 1}}
	calls := map[Stage]int{}
	summary, err := RunCheckpointedWorkflow(context.Background(), store, request, func(_ context.Context, stage Stage, _ []StageProgress) StageOutcome {
		calls[stage]++
		return StageOutcome{Summary: map[string]any{"stage": stage}}
	})
	if err != nil {
		t.Fatalf("RunCheckpointedWorkflow error: %v", err)
	}
	if summary.Status != "completed" || len(summary.Stages) != len(OrderedStages) {
		t.Fatalf("summary = %#v", summary)
	}
	for _, stage := range OrderedStages {
		if calls[stage] != 1 {
			t.Fatalf("stage %s calls = %d, want 1", stage, calls[stage])
		}
	}
}

func TestRunCheckpointedOpportunityFindingReusesCompletedBillableStage(t *testing.T) {
	store := newMemoryCheckpointStore()
	request := WorkflowRequest{ProjectID: uuid.New(), WorkflowEventID: uuid.New(), Inputs: map[string]any{"trigger": "scheduled"}}
	fingerprint, err := StageFingerprint(request, StageAIHypotheses)
	if err != nil {
		t.Fatal(err)
	}
	store.rows[string(StageAIHypotheses)] = db.OpportunityFindingStageCheckpoint{
		ID: uuid.New(), ProjectID: request.ProjectID, WorkflowEventID: request.WorkflowEventID,
		Stage: string(StageAIHypotheses), StageOrder: 3, RequestFingerprint: fingerprint,
		Status: "succeeded", AttemptNumber: 1, OwnerToken: uuid.New(),
		OutputSummary: json.RawMessage(`{"provider_calls":2}`), FinishedAt: pgtype.Timestamptz{Valid: true},
	}
	calls := map[Stage]int{}
	_, err = RunCheckpointedWorkflow(context.Background(), store, request, func(_ context.Context, stage Stage, _ []StageProgress) StageOutcome {
		calls[stage]++
		return StageOutcome{}
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls[StageAIHypotheses] != 0 {
		t.Fatalf("completed billable stage repeated %d times", calls[StageAIHypotheses])
	}
}

func TestRunCheckpointedOpportunityFindingContinuesAfterPartialFailure(t *testing.T) {
	store := newMemoryCheckpointStore()
	request := WorkflowRequest{ProjectID: uuid.New(), WorkflowEventID: uuid.New(), Inputs: map[string]any{"trigger": "manual"}}
	calls := map[Stage]int{}
	summary, err := RunCheckpointedWorkflow(context.Background(), store, request, func(_ context.Context, stage Stage, _ []StageProgress) StageOutcome {
		calls[stage]++
		if stage == StageEvidenceRefresh {
			return StageOutcome{Err: errors.New("GA4 permission denied"), Summary: map[string]any{"gsc": "completed", "ga4": "failed"}}
		}
		return StageOutcome{}
	})
	if err != nil {
		t.Fatalf("partial source failure must not fail orchestration: %v", err)
	}
	if summary.Status != "partial" || calls[StageSummary] != 1 || calls[StageMaterialization] != 1 {
		t.Fatalf("partial summary/calls = %#v / %#v", summary, calls)
	}
	if summary.ErrorCount != 1 {
		t.Fatalf("error count = %d, want 1", summary.ErrorCount)
	}
}

func TestStageFingerprintIsStableAndInputSensitive(t *testing.T) {
	request := WorkflowRequest{ProjectID: uuid.New(), WorkflowEventID: uuid.New(), Inputs: map[string]any{"trigger": "manual", "sources": []string{"gsc", "ai"}}}
	first, err := StageFingerprint(request, StageEvidenceRefresh)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := StageFingerprint(request, StageEvidenceRefresh)
	if first != second {
		t.Fatalf("fingerprint unstable: %q != %q", first, second)
	}
	request.Inputs["trigger"] = "scheduled"
	changed, _ := StageFingerprint(request, StageEvidenceRefresh)
	if first == changed {
		t.Fatal("fingerprint did not change with stage inputs")
	}
}
