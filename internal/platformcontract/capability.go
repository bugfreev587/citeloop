package platformcontract

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
)

func BuildMatrix(input MatrixInput) ([]Capability, error) {
	assetType, ok := CanonicalAssetType(input.AssetType)
	if !ok {
		return nil, fmt.Errorf("unsupported content asset type %q", input.AssetType)
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	contracts := append([]db.PlatformContentContract(nil), input.Contracts...)
	sort.Slice(contracts, func(i, j int) bool { return contracts[i].Platform < contracts[j].Platform })
	result := make([]Capability, 0, len(contracts))
	for _, contract := range contracts {
		if contract.Status != "active" {
			continue
		}
		assets := decodeStrings(contract.CompatibleAssetTypes)
		outputs := decodeStrings(contract.AllowedOutputTypes)
		requiredContext := decodeStrings(contract.RequiredContextFields)
		capability := Capability{
			Platform: contract.Platform, ContractID: contract.ID, ContractVersion: contract.Version,
			GenerationSupported: contract.GenerationSupported && contains(assets, assetType),
			TargetContextReady:  len(requiredContext) == 0,
			ConnectionReady:     input.ConnectionReady[contract.Platform],
			PublishMode:         contract.PublishMode,
			OutputType:          first(outputs),
			CanonicalRequired:   contract.Platform != "blog",
			SourceURLRequired:   contract.Platform != "blog",
			ImageRolesSupported: imageRoles(contract.Platform),
			BlockReasons:        []string{},
		}
		if !capability.GenerationSupported {
			capability.BlockReasons = append(capability.BlockReasons, "asset_type_incompatible")
		}
		if len(requiredContext) > 0 && hasFreshContext(contract.Platform, input.Contexts, now) {
			capability.TargetContextReady = true
		}
		if !capability.TargetContextReady {
			capability.BlockReasons = append(capability.BlockReasons, "target_context_required")
		}
		result = append(result, capability)
	}
	return result, nil
}

func hasFreshContext(platform string, contexts []db.PlatformTargetContext, now time.Time) bool {
	for _, context := range contexts {
		if context.Platform != platform || context.Status != "confirmed" || !context.ExpiresAt.Valid {
			continue
		}
		if context.ExpiresAt.Time.After(now) {
			return true
		}
	}
	return false
}

func decodeStrings(raw json.RawMessage) []string {
	var values []string
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &values)
	}
	for i := range values {
		values[i] = strings.TrimSpace(values[i])
	}
	return values
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func imageRoles(platform string) []string {
	switch platform {
	case "hacker_news":
		return []string{}
	case "reddit":
		return []string{"hero"}
	default:
		return []string{"hero", "inline_explainer", "comparison_visual", "workflow_visual"}
	}
}

func SupportsImageRole(platform, role string) bool {
	if role == "inline_1" || role == "inline_2" {
		role = "inline_explainer"
	}
	for _, supported := range imageRoles(strings.ToLower(strings.TrimSpace(platform))) {
		if supported == role {
			return true
		}
	}
	return false
}
