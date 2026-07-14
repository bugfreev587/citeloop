package platformcontract

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type Target struct {
	Platform             string    `json:"platform"`
	TargetKey            string    `json:"target_key,omitempty"`
	OutputType           string    `json:"output_type"`
	IsCanonical          bool      `json:"is_canonical"`
	ContractID           uuid.UUID `json:"platform_contract_id"`
	ContractVersion      string    `json:"platform_contract_version"`
	TargetContextID      uuid.UUID `json:"target_context_id,omitempty"`
	TargetContextVersion *int32    `json:"target_context_version,omitempty"`
	Rationale            string    `json:"rationale,omitempty"`
}

type PlanInput struct {
	ProjectID          uuid.UUID       `json:"project_id"`
	OpportunityID      uuid.UUID       `json:"opportunity_id,omitempty"`
	ContentActionID    uuid.UUID       `json:"content_action_id,omitempty"`
	AssetType          string          `json:"asset_type"`
	CanonicalTarget    Target          `json:"canonical_target"`
	Targets            []Target        `json:"target_platforms"`
	SelectionMode      string          `json:"selection_mode"`
	CapabilitySnapshot json.RawMessage `json:"capability_snapshot,omitempty"`
}

type Plan struct {
	PlanInput
	ID uuid.UUID `json:"id,omitempty"`
}

type planQueries interface {
	CreateContentTargetPlan(context.Context, db.CreateContentTargetPlanParams) (db.ContentTargetPlan, error)
	CreateContentTargetPlanItem(context.Context, db.CreateContentTargetPlanItemParams) (db.ContentTargetPlanItem, error)
}

func PreparePlan(input PlanInput) (Plan, error) {
	assetType, ok := CanonicalAssetType(input.AssetType)
	if !ok {
		return Plan{}, fmt.Errorf("unsupported content asset type %q", input.AssetType)
	}
	input.AssetType = assetType
	input.SelectionMode = strings.TrimSpace(input.SelectionMode)
	if input.SelectionMode == "" {
		input.SelectionMode = "contract_matrix"
	}
	if input.SelectionMode != "contract_matrix" && input.SelectionMode != "legacy_derived" {
		return Plan{}, fmt.Errorf("unsupported selection mode %q", input.SelectionMode)
	}
	canonical := normalizeTarget(input.CanonicalTarget)
	if canonical.Platform == "" {
		return Plan{}, fmt.Errorf("canonical target is required")
	}
	seen := map[string]Target{}
	for _, raw := range input.Targets {
		target := normalizeTarget(raw)
		if target.Platform == "" || target.OutputType == "" || target.ContractID == uuid.Nil || target.ContractVersion == "" {
			return Plan{}, fmt.Errorf("target requires platform, output type, and contract")
		}
		key := targetIdentity(target)
		if _, exists := seen[key]; !exists {
			seen[key] = target
		}
	}
	canonicalKey := targetIdentity(canonical)
	if _, ok := seen[canonicalKey]; !ok {
		return Plan{}, fmt.Errorf("canonical target must be included in target platforms")
	}
	targets := make([]Target, 0, len(seen))
	for key, target := range seen {
		target.IsCanonical = key == canonicalKey
		targets = append(targets, target)
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].IsCanonical != targets[j].IsCanonical {
			return targets[i].IsCanonical
		}
		return targetIdentity(targets[i]) < targetIdentity(targets[j])
	})
	canonical = seen[canonicalKey]
	canonical.IsCanonical = true
	input.CanonicalTarget = canonical
	input.Targets = targets
	if len(input.CapabilitySnapshot) == 0 {
		input.CapabilitySnapshot = json.RawMessage(`{}`)
	}
	return Plan{PlanInput: input}, nil
}

func CreatePlan(ctx context.Context, q planQueries, input PlanInput) (Plan, error) {
	plan, err := PreparePlan(input)
	if err != nil {
		return Plan{}, err
	}
	canonicalJSON, _ := json.Marshal(plan.CanonicalTarget)
	row, err := q.CreateContentTargetPlan(ctx, db.CreateContentTargetPlanParams{
		ProjectID: plan.ProjectID, OpportunityID: pgUUID(plan.OpportunityID), ContentActionID: pgUUID(plan.ContentActionID),
		AssetType: plan.AssetType, CanonicalTarget: string(canonicalJSON), SelectionMode: plan.SelectionMode,
		Status: "planned", CapabilitySnapshot: plan.CapabilitySnapshot,
	})
	if err != nil {
		return Plan{}, fmt.Errorf("create target plan: %w", err)
	}
	plan.ID = row.ID
	for ordinal, target := range plan.Targets {
		_, err = q.CreateContentTargetPlanItem(ctx, db.CreateContentTargetPlanItemParams{
			PlanID: row.ID, Ordinal: int32(ordinal), Platform: target.Platform, TargetKey: target.TargetKey,
			OutputType: target.OutputType, IsCanonical: target.IsCanonical,
			PlatformContractID: pgUUID(target.ContractID), PlatformContractVersion: target.ContractVersion,
			TargetContextID: pgUUID(target.TargetContextID), TargetContextVersion: target.TargetContextVersion,
			Rationale: target.Rationale, Status: "planned",
		})
		if err != nil {
			return Plan{}, fmt.Errorf("create target plan item: %w", err)
		}
	}
	return plan, nil
}

func LegacyPlanInput(base PlanInput, strategy string, contracts []db.PlatformContentContract) (PlanInput, error) {
	byPlatform := make(map[string]db.PlatformContentContract, len(contracts))
	for _, contract := range contracts {
		byPlatform[contract.Platform] = contract
	}
	strategy = strings.ToLower(strings.TrimSpace(strategy))
	platforms := []string{"blog"}
	if strategy == "syndication" || strategy == "both" {
		platforms = append(platforms, "dev_to", "hashnode", "reddit")
	}
	targets := make([]Target, 0, len(platforms))
	for _, name := range platforms {
		contract, ok := byPlatform[name]
		if !ok {
			return PlanInput{}, fmt.Errorf("active platform contract missing for %s", name)
		}
		outputs := decodeStrings(contract.AllowedOutputTypes)
		if len(outputs) == 0 {
			return PlanInput{}, fmt.Errorf("platform contract has no output type for %s", name)
		}
		targets = append(targets, Target{Platform: name, OutputType: outputs[0], ContractID: contract.ID, ContractVersion: contract.Version})
	}
	base.CanonicalTarget = Target{Platform: "blog"}
	base.Targets = targets
	base.SelectionMode = "legacy_derived"
	return base, nil
}

func DeriveChannel(plan Plan) string {
	hasBlog, hasExternal := false, false
	for _, target := range plan.Targets {
		if target.Platform == "blog" {
			hasBlog = true
		} else {
			hasExternal = true
		}
	}
	switch {
	case hasBlog && hasExternal:
		return "both"
	case hasBlog:
		return "blog"
	case hasExternal:
		return "syndication"
	default:
		return ""
	}
}

func normalizeTarget(target Target) Target {
	target.Platform = strings.ToLower(strings.TrimSpace(target.Platform))
	target.TargetKey = strings.ToLower(strings.TrimSpace(target.TargetKey))
	if target.Platform == "reddit" {
		target.TargetKey = normalizeRedditTarget(target.TargetKey)
	}
	target.OutputType = strings.ToLower(strings.TrimSpace(target.OutputType))
	target.ContractVersion = strings.TrimSpace(target.ContractVersion)
	target.Rationale = strings.TrimSpace(target.Rationale)
	return target
}

func targetIdentity(target Target) string { return target.Platform + "\x00" + target.TargetKey }

func pgUUID(value uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: value, Valid: value != uuid.Nil}
}
