package growthradar

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/citeloop/citeloop/internal/growthstage"
)

const FormulaVersion = "growth-radar-score-v1"
const StageFormulaVersion = "growth-radar-stage-score-v1"

type EvidenceSource struct {
	Class              string `json:"class"`
	Qualified          bool   `json:"qualified"`
	FirstParty         bool   `json:"first_party"`
	CompleteProvenance bool   `json:"complete_provenance"`
}

type Snapshot struct {
	CurrentImpressions        int              `json:"current_impressions"`
	PreviousImpressions       int              `json:"previous_impressions"`
	QualifiedRecurrence       int              `json:"qualified_recurrence"`
	PrimaryCoverage           string           `json:"primary_coverage"`
	InternalLinkPaths         int              `json:"internal_link_paths"`
	WeakOrStaleLinkPath       bool             `json:"weak_or_stale_link_path"`
	SelectedExternalTargets   int              `json:"selected_external_targets"`
	CoveredExternalTargets    int              `json:"covered_external_targets"`
	CapabilityConfirmed       bool             `json:"capability_confirmed"`
	AudienceConfirmed         bool             `json:"audience_confirmed"`
	IntentSupported           bool             `json:"intent_supported"`
	Intent                    string           `json:"intent"`
	JourneyStage              string           `json:"journey_stage"`
	ConversionMapping         string           `json:"conversion_mapping"`
	NewestEvidenceAgeDays     *int             `json:"newest_evidence_age_days"`
	MaterialChange            string           `json:"material_change"`
	CompatibleExternalTargets int              `json:"compatible_external_targets"`
	AdditionalOutputTypes     int              `json:"additional_output_types"`
	EvidenceSources           []EvidenceSource `json:"evidence_sources"`
	NearDuplicate             bool             `json:"near_duplicate"`
	Cannibalization           bool             `json:"cannibalization"`
	ExactDuplicate            bool             `json:"exact_duplicate"`
	UnresolvedConflict        bool             `json:"unresolved_conflict"`
	LLMOnlyEvidence           bool             `json:"llm_only_evidence"`
	SensitiveOrUnsupported    bool             `json:"sensitive_or_unsupported"`
	DismissedWithoutChange    bool             `json:"dismissed_without_change"`
	IndependentGEOProviders   int              `json:"independent_geo_providers"`
	HasSuccessSignal          bool             `json:"has_success_signal"`
	HasResolvedExpansion      bool             `json:"has_resolved_expansion"`
	HasMaterialChangeEvidence bool             `json:"has_material_change_evidence"`
	MissingStageConfiguration bool             `json:"missing_stage_configuration"`
	LLMText                   string           `json:"-"`
}

type Penalty struct {
	Code   string `json:"code"`
	Points int    `json:"points"`
}

type Score struct {
	FormulaVersion        string                `json:"formula_version"`
	SnapshotHash          string                `json:"snapshot_hash"`
	Demand                int                   `json:"demand"`
	CoverageGap           int                   `json:"coverage_gap"`
	Relevance             int                   `json:"relevance"`
	CommercialValue       int                   `json:"commercial_value"`
	Freshness             int                   `json:"freshness"`
	ReusePotential        int                   `json:"reuse_potential"`
	EvidenceQuality       int                   `json:"evidence_quality"`
	Penalties             []Penalty             `json:"penalties"`
	Final                 int                   `json:"final"`
	Disposition           string                `json:"disposition"`
	Stage                 string                `json:"stage,omitempty"`
	StageProfileVersion   string                `json:"stage_profile_version,omitempty"`
	RawComponents         *growthstage.Raw      `json:"raw_components,omitempty"`
	WeightedContributions *growthstage.Weighted `json:"weighted_contributions,omitempty"`
	ReasonCodes           []string              `json:"reason_codes,omitempty"`
}

func ScoreCandidate(snapshot Snapshot) (Score, error) {
	for _, source := range snapshot.EvidenceSources {
		if source.Qualified && source.Class == "" {
			return Score{}, fmt.Errorf("qualified evidence source class is required")
		}
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return Score{}, err
	}
	score := Score{FormulaVersion: FormulaVersion, SnapshotHash: hashText(string(encoded)), Penalties: []Penalty{}}
	score.Demand = impressionPoints(snapshot.CurrentImpressions) + growthPoints(snapshot.CurrentImpressions, snapshot.PreviousImpressions) + min(max(snapshot.QualifiedRecurrence, 0), 5)
	switch snapshot.PrimaryCoverage {
	case "none", "":
		score.CoverageGap += 12
	case "stale", "failed":
		score.CoverageGap += 6
	}
	switch {
	case snapshot.InternalLinkPaths <= 0:
		score.CoverageGap += 4
	case snapshot.InternalLinkPaths == 1 || snapshot.WeakOrStaleLinkPath:
		score.CoverageGap += 2
	}
	if snapshot.SelectedExternalTargets > 0 {
		switch {
		case snapshot.CoveredExternalTargets <= 0:
			score.CoverageGap += 4
		case snapshot.CoveredExternalTargets < snapshot.SelectedExternalTargets:
			score.CoverageGap += 2
		}
	}
	if snapshot.CapabilityConfirmed {
		score.Relevance += 8
	}
	if snapshot.AudienceConfirmed {
		score.Relevance += 4
	}
	if snapshot.IntentSupported {
		score.Relevance += 3
	}
	score.CommercialValue = intentValue(snapshot.Intent) + journeyValue(snapshot.JourneyStage) + conversionValue(snapshot.ConversionMapping)
	if snapshot.NewestEvidenceAgeDays != nil {
		switch age := *snapshot.NewestEvidenceAgeDays; {
		case age <= 1:
			score.Freshness += 5
		case age <= 7:
			score.Freshness += 4
		case age <= 30:
			score.Freshness += 3
		case age <= 90:
			score.Freshness += 1
		}
	}
	switch snapshot.MaterialChange {
	case "new_query", "new_confirmation", "new_competitor_asset", "growth_over_100":
		score.Freshness += 5
	case "growth_25_100", "top5_result_hash_changed":
		score.Freshness += 3
	case "content_hash_changed":
		score.Freshness += 1
	}
	score.ReusePotential = min(max(snapshot.CompatibleExternalTargets, 0)*2, 6) + min(max(snapshot.AdditionalOutputTypes, 0)*2, 4)
	classes := map[string]struct{}{}
	firstParty, allComplete, qualified := false, true, 0
	for _, source := range snapshot.EvidenceSources {
		if !source.Qualified {
			continue
		}
		qualified++
		classes[source.Class] = struct{}{}
		firstParty = firstParty || source.FirstParty
		allComplete = allComplete && source.CompleteProvenance
	}
	score.EvidenceQuality = min(len(classes), 3)
	if firstParty {
		score.EvidenceQuality++
	}
	if qualified > 0 && allComplete {
		score.EvidenceQuality++
	}
	positive := score.Demand + score.CoverageGap + score.Relevance + score.CommercialValue + score.Freshness + score.ReusePotential + score.EvidenceQuality
	if snapshot.NearDuplicate {
		score.Penalties = append(score.Penalties, Penalty{Code: "near_duplicate", Points: -40})
	}
	if snapshot.Cannibalization {
		score.Penalties = append(score.Penalties, Penalty{Code: "cannibalization", Points: -30})
	}
	penalty := 0
	for _, item := range score.Penalties {
		penalty += item.Points
	}
	score.Final = max(0, positive+penalty)
	score.Disposition = disposition(snapshot, score.Final)
	return score, nil
}

// ScoreCandidateForStage preserves the canonical raw signal calculation and
// applies one immutable project-stage profile. The snapshot remains the only
// scoring input, so historical results can be replayed without model output.
func ScoreCandidateForStage(snapshot Snapshot, stage growthstage.Stage) (Score, error) {
	rawScore, err := ScoreCandidate(snapshot)
	if err != nil {
		return Score{}, err
	}
	profile, err := growthstage.ProfileFor(stage)
	if err != nil {
		return Score{}, err
	}
	raw := growthstage.Raw{
		Demand: rawScore.Demand, Coverage: rawScore.CoverageGap, Relevance: rawScore.Relevance,
		Commercial: rawScore.CommercialValue, Freshness: rawScore.Freshness,
		Reuse: rawScore.ReusePotential, Evidence: rawScore.EvidenceQuality,
	}
	weighted := growthstage.Apply(raw, profile)
	penalty := 0
	for _, item := range rawScore.Penalties {
		penalty += item.Points
	}
	rawScore.FormulaVersion = StageFormulaVersion
	rawScore.Stage = string(stage)
	rawScore.StageProfileVersion = profile.Version
	rawScore.RawComponents = &raw
	rawScore.WeightedContributions = &weighted
	rawScore.Demand = weighted.Demand
	rawScore.CoverageGap = weighted.Coverage
	rawScore.Relevance = weighted.Relevance
	rawScore.CommercialValue = weighted.Commercial
	rawScore.Freshness = weighted.Freshness
	rawScore.ReusePotential = weighted.Reuse
	rawScore.EvidenceQuality = weighted.Evidence
	rawScore.Final = max(0, weighted.Total()+penalty)
	rawScore.Disposition, rawScore.ReasonCodes = stageDisposition(snapshot, rawScore.Final, profile)
	return rawScore, nil
}

func stageDisposition(snapshot Snapshot, final int, profile growthstage.Profile) (string, []string) {
	switch {
	case snapshot.ExactDuplicate:
		return "merged", []string{"duplicate.exact"}
	case snapshot.SensitiveOrUnsupported:
		return "filtered", []string{"context.internal_sensitive"}
	case snapshot.DismissedWithoutChange:
		return "dismissed", []string{"candidate.dismissed_without_change"}
	case !snapshot.CapabilityConfirmed:
		return "hold", []string{"context.capability_unconfirmed"}
	case snapshot.NearDuplicate:
		return "near_duplicate", []string{"duplicate.near"}
	case snapshot.UnresolvedConflict || snapshot.Cannibalization:
		return "arbitration", []string{"conflict.canonical"}
	case snapshot.MissingStageConfiguration:
		return "hold", []string{"target.no_project_target"}
	}

	qualified := 0
	for _, source := range snapshot.EvidenceSources {
		if source.Qualified {
			qualified++
		}
	}
	gateReason := ""
	switch profile.Stage {
	case growthstage.Foundation:
		if qualified < 2 {
			gateReason = "demand.single_geo_provider"
		}
	case growthstage.Traction:
		if rawDemand(snapshot) <= 0 || qualified < 2 {
			gateReason = "stage.traction_gate"
		}
	case growthstage.Scale:
		if rawDemand(snapshot) <= 0 || !snapshot.HasSuccessSignal || !snapshot.HasResolvedExpansion {
			gateReason = "stage.scale_gate"
		}
	case growthstage.Optimize:
		if !snapshot.HasMaterialChangeEvidence {
			gateReason = "stage.optimize_gate"
		}
	}
	if gateReason != "" {
		if qualified > 0 && final >= profile.WatchlistThreshold {
			return "watchlist", []string{gateReason}
		}
		return "filtered", []string{gateReason}
	}
	if final >= profile.OpportunityThreshold {
		return "opportunity", nil
	}
	if final >= profile.WatchlistThreshold {
		return "watchlist", []string{"score.watchlist_range"}
	}
	return "filtered", []string{"score.below_stage_threshold"}
}

func rawDemand(snapshot Snapshot) int {
	return impressionPoints(snapshot.CurrentImpressions) + growthPoints(snapshot.CurrentImpressions, snapshot.PreviousImpressions) + min(max(snapshot.QualifiedRecurrence, 0), 5)
}

func impressionPoints(value int) int {
	switch {
	case value <= 0:
		return 0
	case value <= 9:
		return 3
	case value <= 49:
		return 6
	case value <= 199:
		return 9
	case value <= 999:
		return 12
	default:
		return 15
	}
}
func growthPoints(current, previous int) int {
	if previous < 10 {
		if current >= 10 {
			return 5
		}
		if current > 0 {
			return 2
		}
		return 0
	}
	change := float64(current-previous) / float64(previous)
	switch {
	case change <= -.25:
		return 0
	case change <= 0:
		return 1
	case change <= .25:
		return 2
	case change <= 1:
		return 4
	default:
		return 5
	}
}
func intentValue(value string) int {
	switch value {
	case "transactional", "comparison", "alternative", "integration":
		return 8
	case "use_case", "template":
		return 6
	case "how_to", "problem_solving":
		return 4
	case "glossary", "evidence", "benchmark", "informational":
		return 2
	case "navigational":
		return 1
	default:
		return 0
	}
}
func journeyValue(value string) int {
	switch value {
	case "decision":
		return 4
	case "consideration":
		return 3
	case "adoption", "expansion":
		return 2
	case "awareness":
		return 1
	default:
		return 0
	}
}
func conversionValue(value string) int {
	switch value {
	case "high":
		return 3
	case "standard":
		return 1
	default:
		return 0
	}
}
func disposition(snapshot Snapshot, final int) string {
	switch {
	case snapshot.ExactDuplicate:
		return "merged"
	case snapshot.SensitiveOrUnsupported:
		return "filtered"
	case snapshot.DismissedWithoutChange:
		return "dismissed"
	case !snapshot.CapabilityConfirmed:
		return "hold"
	case snapshot.NearDuplicate:
		return "near_duplicate"
	case snapshot.UnresolvedConflict || snapshot.Cannibalization:
		return "arbitration"
	case snapshot.LLMOnlyEvidence:
		return "watchlist"
	default:
		return dispositionForScore(final)
	}
}
func dispositionForScore(final int) string {
	if final >= 75 {
		return "opportunity"
	}
	if final >= 60 {
		return "watchlist"
	}
	return "filtered"
}
func min(a, b int) int { return int(math.Min(float64(a), float64(b))) }
func max(a, b int) int { return int(math.Max(float64(a), float64(b))) }

func sortedEvidenceSources(values []EvidenceSource) []EvidenceSource {
	copy := append([]EvidenceSource(nil), values...)
	sort.Slice(copy, func(i, j int) bool { return copy[i].Class < copy[j].Class })
	return copy
}
