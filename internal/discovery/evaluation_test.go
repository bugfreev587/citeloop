package discovery

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestSemanticEvaluationCalculatesSafetyAndCapacityMetrics(t *testing.T) {
	cases := make([]SemanticGoldCase, 0, 102)
	for i := 0; i < 50; i++ {
		cases = append(cases, SemanticGoldCase{Label: GoldLabelEquivalent, ExpectedDecision: DecisionSuppress, ActualDecision: DecisionSuppress, Confidence: 0.95, Compared: true})
	}
	for i := 0; i < 50; i++ {
		cases = append(cases, SemanticGoldCase{Label: GoldLabelDistinct, ExpectedDecision: DecisionCreate, ActualDecision: DecisionCreate, Confidence: 0.93, Compared: true})
	}
	cases = append(cases,
		SemanticGoldCase{Label: GoldLabelConflict, ExpectedDecision: DecisionBlockOnOtherLine, ActualDecision: DecisionHold, Confidence: 0.60, Compared: true},
		SemanticGoldCase{Label: GoldLabelDistinct, ExpectedDecision: DecisionCreate, ActualDecision: DecisionHold, Confidence: 0.70, Compared: false},
	)

	result, err := EvaluateSemanticGoldSet(cases, SemanticEvaluationPolicy{
		DatasetVersion: "gold-2026-07", ConfidenceThreshold: 0.80,
		DuplicateSafetyRecallTarget: 0.95, FalseSuppressionRateTarget: 0.02,
		WeeklyOpsCapacity: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.DuplicateSafetyRecall != 1 || result.FalseSuppressionRate != 0 {
		t.Fatalf("safety metrics = %+v", result)
	}
	if result.ComparatorCoverage != float64(101)/102 || result.HoldRate != float64(2)/102 || result.ThresholdBacklog != 2 {
		t.Fatalf("coverage/backlog metrics = %+v", result)
	}
	if !result.LaunchReady || len(result.Blockers) != 0 {
		t.Fatalf("expected launch ready, got %+v", result)
	}
	blockersJSON, err := json.Marshal(result.Blockers)
	if err != nil {
		t.Fatal(err)
	}
	if string(blockersJSON) != "[]" {
		t.Fatalf("launch-ready blockers encoded as %s, want []", blockersJSON)
	}
}

func TestSemanticEvaluationFailsGateForFalseSuppressionOrUnsafeBacklog(t *testing.T) {
	cases := []SemanticGoldCase{
		{Label: GoldLabelEquivalent, ExpectedDecision: DecisionSuppress, ActualDecision: DecisionCreate, Confidence: 0.95, Compared: true},
		{Label: GoldLabelDistinct, ExpectedDecision: DecisionCreate, ActualDecision: DecisionSuppress, Confidence: 0.95, Compared: true},
		{Label: GoldLabelDistinct, ExpectedDecision: DecisionCreate, ActualDecision: DecisionHold, Confidence: 0.50, Compared: true},
	}
	result, err := EvaluateSemanticGoldSet(cases, SemanticEvaluationPolicy{
		DatasetVersion: "gold-v1", ConfidenceThreshold: 0.80,
		DuplicateSafetyRecallTarget: 0.95, FalseSuppressionRateTarget: 0.02,
		WeeklyOpsCapacity: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.LaunchReady {
		t.Fatalf("unsafe evaluation passed: %+v", result)
	}
	joined := strings.Join(result.Blockers, " ")
	for _, want := range []string{"duplicate safety recall", "false suppression", "capacity"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing blocker %q in %q", want, joined)
		}
	}
}

func TestSemanticEvaluationRejectsEmptyOrUnversionedDataset(t *testing.T) {
	if _, err := EvaluateSemanticGoldSet(nil, SemanticEvaluationPolicy{DatasetVersion: "v1", ConfidenceThreshold: .8}); err == nil {
		t.Fatal("empty dataset was accepted")
	}
	if _, err := EvaluateSemanticGoldSet([]SemanticGoldCase{{Label: GoldLabelDistinct, ExpectedDecision: DecisionCreate}}, SemanticEvaluationPolicy{ConfidenceThreshold: .8}); err == nil {
		t.Fatal("unversioned dataset was accepted")
	}
}

func TestSemanticEvaluationServiceRejectsAutomaticSuppressionBeforeGate(t *testing.T) {
	store := &semanticEvaluationStoreStub{
		cases: []SemanticGoldCase{
			{Label: GoldLabelEquivalent, ExpectedDecision: DecisionSuppress, ActualDecision: DecisionCreate, Confidence: .95, Compared: true},
			{Label: GoldLabelDistinct, ExpectedDecision: DecisionCreate, ActualDecision: DecisionCreate, Confidence: .95, Compared: true},
		},
	}
	_, err := NewSemanticEvaluationService(store).Run(context.Background(), uuid.New(), SemanticEvaluationPolicy{
		DatasetVersion: "gold-v1", ConfidenceThreshold: .8, DuplicateSafetyRecallTarget: .95,
		FalseSuppressionRateTarget: .02, WeeklyOpsCapacity: 10,
	}, true, "ops@example.com")
	if err == nil || !strings.Contains(err.Error(), "automatic suppression") {
		t.Fatalf("error = %v", err)
	}
	if store.saveCalls != 0 {
		t.Fatal("unsafe launch config was persisted")
	}
}

type semanticEvaluationStoreStub struct {
	cases     []SemanticGoldCase
	saveCalls int
}

func (s *semanticEvaluationStoreStub) LoadGoldCases(context.Context, uuid.UUID, string) ([]SemanticGoldCase, error) {
	return s.cases, nil
}

func (s *semanticEvaluationStoreStub) SaveSemanticEvaluation(_ context.Context, _ uuid.UUID, result SemanticEvaluationResult, _ bool, _ string) (SemanticEvaluationResult, error) {
	s.saveCalls++
	return result, nil
}
