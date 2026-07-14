package opportunityfinding

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/growthradar"
	"github.com/citeloop/citeloop/internal/growthspec"
	"github.com/citeloop/citeloop/internal/platformcontract"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type memoryCheckpointStore struct {
	rows       map[string]db.OpportunityFindingStageCheckpoint
	leases     map[string]pgtype.Timestamptz
	finishErr  error
	acquireErr error
}

func TestGrowthRadarAcceptanceFlowProducesExactDecisionReadyOpportunity(t *testing.T) {
	profile := json.RawMessage(`{"positioning":"AI search visibility","features":["citation monitoring","DATABASE_PASSWORD=hunter2-production"],"icp":["growth leaders"],"competitors":["VisibleCo"]}`)
	discoveryProfile := growthradar.DiscoveryProfile(profile, growthradar.EvidenceIndex{PublicTerms: []string{"answer engine optimization"}})
	if string(discoveryProfile) == "" || containsJSONText(discoveryProfile, "hunter2-production") {
		t.Fatalf("discovery profile leaked sensitive context: %s", discoveryProfile)
	}

	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	old := now.Add(-72 * time.Hour)
	selection := SelectPrompts(now, []PromptState{
		{ID: uuid.New(), Priority: 100, ClusterKey: "old", LastObservedAt: &old},
		{ID: uuid.New(), Priority: 50, ClusterKey: "new"},
	}, 1)
	if len(selection.Prompts) != 1 || selection.Reasons[selection.Prompts[0].ID] != "never_observed" {
		t.Fatalf("prompt rotation did not select unseen evidence: %+v", selection)
	}

	age := 1
	score, err := growthradar.ScoreCandidate(growthradar.Snapshot{
		CurrentImpressions: 1000, PreviousImpressions: 400, QualifiedRecurrence: 5,
		PrimaryCoverage: "none", InternalLinkPaths: 0, SelectedExternalTargets: 1,
		CapabilityConfirmed: true, AudienceConfirmed: true, IntentSupported: true,
		Intent: "comparison", JourneyStage: "decision", ConversionMapping: "high",
		NewestEvidenceAgeDays: &age, MaterialChange: "new_query", CompatibleExternalTargets: 1, AdditionalOutputTypes: 2,
		EvidenceSources: []growthradar.EvidenceSource{
			{Class: "search_console", Qualified: true, FirstParty: true, CompleteProvenance: true},
			{Class: "owned_inventory", Qualified: true, FirstParty: true, CompleteProvenance: true},
			{Class: "brave_search", Qualified: true, CompleteProvenance: true},
		},
	})
	if err != nil || score.Disposition != "opportunity" {
		t.Fatalf("deterministic score = %+v, err=%v", score, err)
	}
	contractID := uuid.New()
	target := platformcontract.Target{Platform: "blog", OutputType: "canonical_article", ContractID: contractID, ContractVersion: "2026-07-13"}
	materialized := growthradar.MaterializeOpportunitySpec(growthradar.MaterializationCandidate{
		ProjectID: uuid.New(), ClusterID: "ai-visibility", Topic: "ai visibility tools", Intent: "comparison", JourneyStage: "decision",
		Audience: "growth leaders", AssetType: "comparison_page", Action: "Create an evidence-backed comparison",
		ExpectedUserValue: "Compare verifiable capabilities", Evidence: json.RawMessage(`{"search_evidence_ids":["search-1"],"gsc_window":"28d"}`),
		SuccessMetric: growthspec.SuccessMetric{Name: "gsc_clicks", WindowDays: 56},
		Target:        growthspec.TargetSpec{CanonicalTarget: target, TargetPlatforms: []platformcontract.Target{target}, SelectionMode: "contract_matrix"},
		Score:         score, SourceVersions: map[string]string{"scoring": growthradar.FormulaVersion, "target_contract": "2026-07-13"},
	})
	if materialized.Disposition != "opportunity" || materialized.Spec.State != growthspec.StateDecisionReady || materialized.Spec.Spec.Targets.CanonicalTarget.Platform != "blog" {
		t.Fatalf("materialized Opportunity v2 = %+v", materialized)
	}
}

func containsJSONText(raw json.RawMessage, text string) bool {
	return strings.Contains(strings.ToLower(string(raw)), strings.ToLower(text))
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
