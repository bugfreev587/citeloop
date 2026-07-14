package growthradar

import (
	"encoding/json"

	"github.com/citeloop/citeloop/internal/growthspec"
	"github.com/google/uuid"
)

type MaterializationCandidate struct {
	ProjectID                                                   uuid.UUID
	ClusterID, Topic, Intent, JourneyStage, Audience, AssetType string
	Action, ExpectedUserValue                                   string
	Evidence                                                    json.RawMessage
	ImageBrief                                                  *growthspec.ImageBrief
	SuccessMetric                                               growthspec.SuccessMetric
	Target                                                      growthspec.TargetSpec
	Score                                                       Score
	SourceVersions                                              map[string]string
	RelatedExistingWork                                         []string
}

type MaterializationResult struct {
	Disposition string
	Spec        growthspec.Result
	Input       growthspec.V2Input
}

func MaterializeOpportunitySpec(candidate MaterializationCandidate) MaterializationResult {
	disposition := candidate.Score.Disposition
	if disposition == "" {
		disposition = dispositionForScore(candidate.Score.Final)
	}
	if disposition != "opportunity" {
		return MaterializationResult{Disposition: disposition, Spec: growthspec.Result{State: growthspec.StateNeedsSpecification, Version: growthspec.VersionV2, Missing: []string{"candidate_disposition"}}}
	}
	dedupe := DedupeIdentity(TopicIdentityInput{ProjectID: candidate.ProjectID.String(), Cluster: candidate.Topic, Intent: candidate.Intent, Audience: candidate.Audience, AssetType: candidate.AssetType, CanonicalTarget: candidate.Target.CanonicalTarget.Platform + ":" + candidate.Target.CanonicalTarget.TargetKey})
	scoreJSON, _ := json.Marshal(candidate.Score)
	input := growthspec.V2Input{
		Intent: candidate.Intent, JourneyStage: candidate.JourneyStage, Audience: []string{candidate.Audience}, TopicClusterID: candidate.ClusterID,
		NormalizedTopic: candidate.Topic, AssetType: candidate.AssetType, RecommendedAction: candidate.Action, ExpectedUserValue: candidate.ExpectedUserValue,
		Target: candidate.Target, Evidence: candidate.Evidence, ImageBrief: candidate.ImageBrief, SuccessMetric: candidate.SuccessMetric,
		DedupeIdentity: dedupe, RelatedExistingWork: candidate.RelatedExistingWork, Score: scoreJSON, SourceVersions: candidate.SourceVersions,
	}
	spec := growthspec.BuildV2(input)
	return MaterializationResult{Disposition: disposition, Spec: spec, Input: input}
}
