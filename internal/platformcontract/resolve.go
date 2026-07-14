package platformcontract

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/citeloop/citeloop/internal/db"
)

type ResolveInput struct {
	AssetType string
	Item      db.ContentTargetPlanItem
	Contract  db.PlatformContentContract
	Context   *db.PlatformTargetContext
}

func Resolve(input ResolveInput) (ResolvedContract, error) {
	assetType, ok := CanonicalAssetType(input.AssetType)
	if !ok {
		return ResolvedContract{}, fmt.Errorf("unsupported asset type %q", input.AssetType)
	}
	if input.Item.Platform != input.Contract.Platform || !input.Item.PlatformContractID.Valid || input.Item.PlatformContractID.Bytes != input.Contract.ID || input.Item.PlatformContractVersion != input.Contract.Version {
		return ResolvedContract{}, fmt.Errorf("target item does not match pinned platform contract")
	}
	if !contains(decodeStrings(input.Contract.CompatibleAssetTypes), assetType) {
		return ResolvedContract{}, fmt.Errorf("asset type %s is incompatible with %s", assetType, input.Item.Platform)
	}
	base, ok := ContractsV1()[input.Item.Platform]
	if !ok {
		return ResolvedContract{}, fmt.Errorf("unsupported platform %q", input.Item.Platform)
	}
	if input.Item.OutputType != "" {
		base.OutputType = input.Item.OutputType
	}
	if !contains(decodeStrings(input.Contract.AllowedOutputTypes), base.OutputType) {
		return ResolvedContract{}, fmt.Errorf("output type %s is not allowed for %s", base.OutputType, base.Platform)
	}
	base.Version = input.Contract.Version
	base.AssetType = assetType
	if prompt := strings.TrimSpace(input.Contract.PromptTemplate); prompt != "" {
		base.Prompt += "\n\nContract directive: " + prompt
	}
	if input.Item.Platform == "reddit" {
		if input.Context == nil || !input.Item.TargetContextID.Valid || input.Context.ID != input.Item.TargetContextID.Bytes {
			return ResolvedContract{}, fmt.Errorf("reddit target requires the pinned target context")
		}
		var allowed []string
		if err := json.Unmarshal(input.Context.AllowedPostTypes, &allowed); err != nil {
			return ResolvedContract{}, fmt.Errorf("decode target context post types: %w", err)
		}
		base.TargetContext = &TargetContextRules{
			TargetKey: input.Context.TargetKey, Status: input.Context.Status, AllowedPostTypes: allowed,
			RequiredFlair: stringPtr(input.Context.RequiredFlair), LinkPolicy: input.Context.LinkPolicy,
		}
		if input.Context.ExpiresAt.Valid {
			base.TargetContext.ExpiresAt = input.Context.ExpiresAt.Time
		}
	}
	return base, nil
}

func stringPtr(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
