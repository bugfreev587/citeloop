package opportunityfinding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
)

type Stage string

const (
	StageEvidenceRefresh      Stage = "evidence_refresh"
	StageDeterministicSignals Stage = "deterministic_signals"
	StageAIHypotheses         Stage = "ai_hypotheses"
	StageArbitration          Stage = "arbitration"
	StageMaterialization      Stage = "materialization"
	StageSummary              Stage = "summary"
)

var OrderedStages = []Stage{
	StageEvidenceRefresh,
	StageDeterministicSignals,
	StageAIHypotheses,
	StageArbitration,
	StageMaterialization,
	StageSummary,
}

const (
	stageExecutionTimeout = 3 * time.Minute
	stageLease            = stageExecutionTimeout + time.Minute
)

var ErrStageInProgress = errors.New("Opportunity Finding stage is already in progress")

type CheckpointStore interface {
	AcquireOpportunityFindingStage(context.Context, db.AcquireOpportunityFindingStageParams) (db.OpportunityFindingStageCheckpoint, error)
	FinishOpportunityFindingStage(context.Context, db.FinishOpportunityFindingStageParams) (db.FinishOpportunityFindingStageRow, error)
	ListOpportunityFindingStages(context.Context, db.ListOpportunityFindingStagesParams) ([]db.OpportunityFindingStageCheckpoint, error)
}

type WorkflowRequest struct {
	ProjectID       uuid.UUID
	WorkflowEventID uuid.UUID
	Inputs          map[string]any
	Now             func() time.Time
}

type StageOutcome struct {
	Status  string
	Summary map[string]any
	Err     error
}

type StageProgress struct {
	Stage              Stage          `json:"stage"`
	Order              int16          `json:"order"`
	Status             string         `json:"status"`
	AttemptNumber      int32          `json:"attempt_number"`
	RequestFingerprint string         `json:"request_fingerprint"`
	Summary            map[string]any `json:"summary"`
	Error              string         `json:"error,omitempty"`
	Reused             bool           `json:"reused"`
}

type WorkflowSummary struct {
	Status     string          `json:"status"`
	ErrorCount int             `json:"error_count"`
	Stages     []StageProgress `json:"stages"`
}

type StageExecutor func(context.Context, Stage, []StageProgress) StageOutcome

func StageFingerprint(request WorkflowRequest, stage Stage) (string, error) {
	payload := struct {
		Version   string         `json:"version"`
		ProjectID uuid.UUID      `json:"project_id"`
		Stage     Stage          `json:"stage"`
		Inputs    map[string]any `json:"inputs"`
	}{Version: "opportunity-finding-workflow/v1", ProjectID: request.ProjectID, Stage: stage, Inputs: request.Inputs}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func RunCheckpointedWorkflow(ctx context.Context, store CheckpointStore, request WorkflowRequest, execute StageExecutor) (WorkflowSummary, error) {
	if store == nil || execute == nil || request.ProjectID == uuid.Nil || request.WorkflowEventID == uuid.Nil {
		return WorkflowSummary{}, errors.New("checkpoint store, executor, project, and workflow event are required")
	}
	now := time.Now
	if request.Now != nil {
		now = request.Now
	}
	progress := make([]StageProgress, 0, len(OrderedStages))
	for index, stage := range OrderedStages {
		fingerprint, err := StageFingerprint(request, stage)
		if err != nil {
			return WorkflowSummary{}, err
		}
		ownerToken := uuid.New()
		leaseStart := now().UTC()
		checkpoint, err := store.AcquireOpportunityFindingStage(ctx, db.AcquireOpportunityFindingStageParams{
			ID: uuid.New(), ProjectID: request.ProjectID, WorkflowEventID: request.WorkflowEventID,
			Stage: string(stage), StageOrder: int16(index + 1), RequestFingerprint: fingerprint,
			OwnerToken: ownerToken, LeaseExpiresAt: pgutil.TS(leaseStart.Add(stageLease)),
		})
		if err != nil {
			return WorkflowSummary{}, fmt.Errorf("acquire %s checkpoint: %w", stage, err)
		}
		if checkpoint.RequestFingerprint != fingerprint {
			return WorkflowSummary{}, fmt.Errorf("%s checkpoint fingerprint changed within workflow event", stage)
		}
		if checkpoint.Status != "running" {
			progress = append(progress, progressFromCheckpoint(checkpoint, true))
			continue
		}
		if checkpoint.OwnerToken != ownerToken {
			return WorkflowSummary{}, fmt.Errorf("%w: %s", ErrStageInProgress, stage)
		}

		stageCtx, cancelStage := context.WithTimeout(ctx, stageExecutionTimeout)
		outcome := execute(stageCtx, stage, append([]StageProgress(nil), progress...))
		cancelStage()
		status := outcome.Status
		if status == "" {
			status = "succeeded"
			if outcome.Err != nil {
				status = "failed"
				if len(outcome.Summary) > 0 {
					status = "partial"
				}
			}
		}
		if !terminalStageStatus(status) {
			return WorkflowSummary{}, fmt.Errorf("invalid terminal status %q for %s", status, stage)
		}
		summary := outcome.Summary
		if summary == nil {
			summary = map[string]any{}
		}
		raw, err := json.Marshal(summary)
		if err != nil {
			return WorkflowSummary{}, fmt.Errorf("marshal %s summary: %w", stage, err)
		}
		var errText *string
		if outcome.Err != nil {
			message := outcome.Err.Error()
			errText = &message
		}
		finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		finished, finishErr := store.FinishOpportunityFindingStage(finishCtx, db.FinishOpportunityFindingStageParams{
			Status: status, OutputSummary: raw, Error: errText, ID: checkpoint.ID,
			ProjectID: request.ProjectID, WorkflowEventID: request.WorkflowEventID,
			Stage: string(stage), OwnerToken: ownerToken,
		})
		cancel()
		if finishErr != nil {
			return WorkflowSummary{}, fmt.Errorf("finish %s checkpoint: %w", stage, finishErr)
		}
		progress = append(progress, progressFromCheckpoint(finishRowToCheckpoint(finished), false))
	}
	return summarizeProgress(progress), nil
}

func finishRowToCheckpoint(row db.FinishOpportunityFindingStageRow) db.OpportunityFindingStageCheckpoint {
	return db.OpportunityFindingStageCheckpoint{
		ID: row.ID, ProjectID: row.ProjectID, WorkflowEventID: row.WorkflowEventID,
		Stage: row.Stage, StageOrder: row.StageOrder, RequestFingerprint: row.RequestFingerprint,
		Status: row.Status, AttemptNumber: row.AttemptNumber, OwnerToken: row.OwnerToken,
		LeaseExpiresAt: row.LeaseExpiresAt, OutputSummary: row.OutputSummary, Error: row.Error,
		StartedAt: row.StartedAt, FinishedAt: row.FinishedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func progressFromCheckpoint(checkpoint db.OpportunityFindingStageCheckpoint, reused bool) StageProgress {
	summary := map[string]any{}
	_ = json.Unmarshal(checkpoint.OutputSummary, &summary)
	progress := StageProgress{
		Stage: Stage(checkpoint.Stage), Order: checkpoint.StageOrder, Status: checkpoint.Status,
		AttemptNumber: checkpoint.AttemptNumber, RequestFingerprint: checkpoint.RequestFingerprint,
		Summary: summary, Reused: reused,
	}
	if checkpoint.Error != nil {
		progress.Error = *checkpoint.Error
	}
	return progress
}

func summarizeProgress(stages []StageProgress) WorkflowSummary {
	summary := WorkflowSummary{Status: "completed", Stages: stages}
	for _, stage := range stages {
		if stage.Status == "partial" || stage.Status == "failed" || stage.Error != "" {
			summary.ErrorCount++
		}
	}
	if summary.ErrorCount > 0 {
		summary.Status = "partial"
	}
	return summary
}

func terminalStageStatus(status string) bool {
	switch status {
	case "succeeded", "partial", "failed", "skipped":
		return true
	default:
		return false
	}
}
