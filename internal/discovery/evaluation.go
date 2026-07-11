package discovery

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/google/uuid"
)

type SemanticGoldLabel string

const (
	GoldLabelEquivalent SemanticGoldLabel = "equivalent"
	GoldLabelDistinct   SemanticGoldLabel = "distinct"
	GoldLabelConflict   SemanticGoldLabel = "conflict"
)

type SemanticGoldCase struct {
	ID               uuid.UUID
	Label            SemanticGoldLabel
	ExpectedDecision DecisionKind
	ActualDecision   DecisionKind
	Confidence       float64
	Compared         bool
}

type SemanticEvaluationPolicy struct {
	DatasetVersion              string
	ConfidenceThreshold         float64
	DuplicateSafetyRecallTarget float64
	FalseSuppressionRateTarget  float64
	WeeklyOpsCapacity           int
}

type SemanticEvaluationResult struct {
	ID                           uuid.UUID `json:"id,omitempty"`
	DatasetVersion               string    `json:"dataset_version"`
	ConfidenceThreshold          float64   `json:"confidence_threshold"`
	DuplicateSafetyRecallTarget  float64   `json:"duplicate_safety_recall_target"`
	FalseSuppressionRateTarget   float64   `json:"false_suppression_rate_target"`
	TotalCases                   int       `json:"total_cases"`
	DuplicateSafetyCases         int       `json:"duplicate_safety_cases"`
	DistinctCases                int       `json:"distinct_cases"`
	DuplicateSafetyRecall        float64   `json:"duplicate_safety_recall"`
	FalseSuppressionRate         float64   `json:"false_suppression_rate"`
	ComparatorCoverage           float64   `json:"comparator_coverage"`
	AutomatedDispositionCoverage float64   `json:"automated_disposition_coverage"`
	HoldRate                     float64   `json:"hold_rate"`
	ThresholdBacklog             int       `json:"threshold_backlog"`
	WeeklyOpsCapacity            int       `json:"weekly_ops_capacity"`
	LaunchReady                  bool      `json:"launch_ready"`
	Blockers                     []string  `json:"blockers"`
}

type SemanticEvaluationStore interface {
	LoadGoldCases(context.Context, uuid.UUID, string) ([]SemanticGoldCase, error)
	SaveSemanticEvaluation(context.Context, uuid.UUID, SemanticEvaluationResult, bool, string) (SemanticEvaluationResult, error)
}

type SemanticEvaluationService struct {
	store SemanticEvaluationStore
}

func NewSemanticEvaluationService(store SemanticEvaluationStore) *SemanticEvaluationService {
	return &SemanticEvaluationService{store: store}
}

func (s *SemanticEvaluationService) Run(ctx context.Context, projectID uuid.UUID, policy SemanticEvaluationPolicy, enableAutomaticSuppression bool, evaluatedBy string) (SemanticEvaluationResult, error) {
	if s == nil || s.store == nil {
		return SemanticEvaluationResult{}, errors.New("semantic evaluation store is required")
	}
	if projectID == uuid.Nil || strings.TrimSpace(evaluatedBy) == "" {
		return SemanticEvaluationResult{}, errors.New("project and evaluator are required")
	}
	cases, err := s.store.LoadGoldCases(ctx, projectID, strings.TrimSpace(policy.DatasetVersion))
	if err != nil {
		return SemanticEvaluationResult{}, fmt.Errorf("load semantic gold cases: %w", err)
	}
	result, err := EvaluateSemanticGoldSet(cases, policy)
	if err != nil {
		return SemanticEvaluationResult{}, err
	}
	if enableAutomaticSuppression && !result.LaunchReady {
		return SemanticEvaluationResult{}, errors.New("automatic suppression cannot be enabled before the semantic launch gate passes")
	}
	return s.store.SaveSemanticEvaluation(ctx, projectID, result, enableAutomaticSuppression, strings.TrimSpace(evaluatedBy))
}

func EvaluateSemanticGoldSet(cases []SemanticGoldCase, policy SemanticEvaluationPolicy) (SemanticEvaluationResult, error) {
	policy = normalizeSemanticEvaluationPolicy(policy)
	if strings.TrimSpace(policy.DatasetVersion) == "" {
		return SemanticEvaluationResult{}, errors.New("semantic gold dataset version is required")
	}
	if len(cases) == 0 {
		return SemanticEvaluationResult{}, errors.New("semantic gold dataset must not be empty")
	}
	result := SemanticEvaluationResult{
		DatasetVersion: policy.DatasetVersion, ConfidenceThreshold: policy.ConfidenceThreshold,
		DuplicateSafetyRecallTarget: policy.DuplicateSafetyRecallTarget,
		FalseSuppressionRateTarget:  policy.FalseSuppressionRateTarget,
		TotalCases:                  len(cases), WeeklyOpsCapacity: policy.WeeklyOpsCapacity,
		Blockers: []string{},
	}
	duplicateDetected := 0
	falseSuppressed := 0
	compared := 0
	automated := 0
	holds := 0
	for index, gold := range cases {
		if err := validateSemanticGoldCase(gold); err != nil {
			return SemanticEvaluationResult{}, fmt.Errorf("gold case %d: %w", index+1, err)
		}
		if gold.ExpectedDecision == DecisionCreate {
			result.DistinctCases++
			if gold.ActualDecision == DecisionSuppress || gold.ActualDecision == DecisionMergeEvidence {
				falseSuppressed++
			}
		} else {
			result.DuplicateSafetyCases++
			if gold.ActualDecision != DecisionCreate {
				duplicateDetected++
			}
		}
		if gold.Compared {
			compared++
		}
		hold := !gold.Compared || gold.ActualDecision == DecisionHold || gold.Confidence < policy.ConfidenceThreshold
		if hold {
			holds++
		} else {
			automated++
		}
	}
	result.DuplicateSafetyRecall = ratio(duplicateDetected, result.DuplicateSafetyCases)
	result.FalseSuppressionRate = ratio(falseSuppressed, result.DistinctCases)
	result.ComparatorCoverage = ratio(compared, result.TotalCases)
	result.AutomatedDispositionCoverage = ratio(automated, result.TotalCases)
	result.HoldRate = ratio(holds, result.TotalCases)
	result.ThresholdBacklog = holds
	if result.DuplicateSafetyCases == 0 {
		result.Blockers = append(result.Blockers, "dataset has no duplicate/safety positive cases")
	} else if result.DuplicateSafetyRecall < policy.DuplicateSafetyRecallTarget {
		result.Blockers = append(result.Blockers, fmt.Sprintf("duplicate safety recall %.4f is below %.4f", result.DuplicateSafetyRecall, policy.DuplicateSafetyRecallTarget))
	}
	if result.DistinctCases == 0 {
		result.Blockers = append(result.Blockers, "dataset has no distinct negative cases")
	} else if result.FalseSuppressionRate >= policy.FalseSuppressionRateTarget {
		result.Blockers = append(result.Blockers, fmt.Sprintf("false suppression rate %.4f is not below %.4f", result.FalseSuppressionRate, policy.FalseSuppressionRateTarget))
	}
	if policy.WeeklyOpsCapacity <= 0 || result.ThresholdBacklog > policy.WeeklyOpsCapacity {
		result.Blockers = append(result.Blockers, fmt.Sprintf("review backlog %d exceeds configured weekly Ops capacity %d", result.ThresholdBacklog, policy.WeeklyOpsCapacity))
	}
	sort.Strings(result.Blockers)
	result.LaunchReady = len(result.Blockers) == 0
	return result, nil
}

func normalizeSemanticEvaluationPolicy(policy SemanticEvaluationPolicy) SemanticEvaluationPolicy {
	policy.DatasetVersion = strings.TrimSpace(policy.DatasetVersion)
	if policy.ConfidenceThreshold <= 0 || policy.ConfidenceThreshold > 1 {
		policy.ConfidenceThreshold = DefaultArbitrationConfidenceThreshold
	}
	if policy.DuplicateSafetyRecallTarget <= 0 || policy.DuplicateSafetyRecallTarget > 1 {
		policy.DuplicateSafetyRecallTarget = 0.95
	}
	if policy.FalseSuppressionRateTarget <= 0 || policy.FalseSuppressionRateTarget > 1 {
		policy.FalseSuppressionRateTarget = 0.02
	}
	return policy
}

func validateSemanticGoldCase(gold SemanticGoldCase) error {
	switch gold.Label {
	case GoldLabelEquivalent, GoldLabelDistinct, GoldLabelConflict:
	default:
		return fmt.Errorf("unsupported label %q", gold.Label)
	}
	switch gold.ExpectedDecision {
	case DecisionCreate, DecisionMergeEvidence, DecisionSuppress, DecisionBlockOnOtherLine:
	default:
		return fmt.Errorf("unsupported expected decision %q", gold.ExpectedDecision)
	}
	switch gold.ActualDecision {
	case DecisionCreate, DecisionMergeEvidence, DecisionSuppress, DecisionBlockOnOtherLine, DecisionHold:
	default:
		return fmt.Errorf("unsupported actual decision %q", gold.ActualDecision)
	}
	if math.IsNaN(gold.Confidence) || math.IsInf(gold.Confidence, 0) || gold.Confidence < 0 || gold.Confidence > 1 {
		return errors.New("confidence must be between 0 and 1")
	}
	return nil
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}
