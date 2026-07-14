package platformcontract

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
)

type articleValidationQueries interface {
	GetTopicForProject(context.Context, db.GetTopicForProjectParams) (db.Topic, error)
	GetPlatformContentContractByID(context.Context, db.GetPlatformContentContractByIDParams) (db.PlatformContentContract, error)
	GetPlatformTargetContextForProject(context.Context, db.GetPlatformTargetContextForProjectParams) (db.PlatformTargetContext, error)
	UpdateArticleContractValidation(context.Context, db.UpdateArticleContractValidationParams) (db.Article, error)
}

func RevalidateArticle(ctx context.Context, q articleValidationQueries, article db.Article, now time.Time) (db.Article, ValidationReport, error) {
	if q == nil {
		return article, ValidationReport{}, errors.New("article contract validation store is required")
	}
	fail := func(code, message string) (db.Article, ValidationReport, error) {
		report := ValidationReport{Passed: false, Failures: []Failure{{Code: code, Message: message}}, Warnings: []Failure{}}
		encoded, _ := json.Marshal(report)
		updated, err := q.UpdateArticleContractValidation(ctx, db.UpdateArticleContractValidationParams{ContractValidation: encoded, ID: article.ID, ProjectID: article.ProjectID})
		return updated, report, err
	}
	if !article.PlatformContractID.Valid || article.PlatformContractVersion == nil || strings.TrimSpace(*article.PlatformContractVersion) == "" {
		return fail("unvalidated_legacy_artifact", "artifact must be regenerated or explicitly validated against a pinned platform contract")
	}
	topic, err := q.GetTopicForProject(ctx, db.GetTopicForProjectParams{ID: article.TopicID, ProjectID: article.ProjectID})
	if err != nil {
		return article, ValidationReport{}, err
	}
	contract, err := q.GetPlatformContentContractByID(ctx, db.GetPlatformContentContractByIDParams{ID: article.PlatformContractID.Bytes, Version: *article.PlatformContractVersion})
	if err != nil || contract.Status == "draft" {
		return fail("pinned_contract_unavailable", "pinned platform contract is unavailable")
	}
	assetType := "blog_post"
	if topic.AssetType != nil && strings.TrimSpace(*topic.AssetType) != "" {
		assetType = *topic.AssetType
	}
	item := db.ContentTargetPlanItem{
		Platform: contract.Platform, OutputType: article.OutputType, IsCanonical: article.Kind == "canonical",
		PlatformContractID: article.PlatformContractID, PlatformContractVersion: *article.PlatformContractVersion,
		TargetContextID: article.TargetContextID, Status: "planned",
	}
	var contextRow *db.PlatformTargetContext
	if article.TargetContextID.Valid {
		row, loadErr := q.GetPlatformTargetContextForProject(ctx, db.GetPlatformTargetContextForProjectParams{ID: article.TargetContextID.Bytes, ProjectID: article.ProjectID})
		if loadErr != nil {
			return fail("target_context_unavailable", "pinned target context is unavailable")
		}
		contextRow = &row
		item.TargetKey = row.TargetKey
		item.TargetContextVersion = &row.Version
	}
	if len(decodeStrings(contract.RequiredContextFields)) > 0 {
		if contextRow == nil || contextRow.Platform != contract.Platform || !contextRow.ExpiresAt.Valid || !TargetContextCurrent(contextRow.Status, contextRow.ExpiresAt.Time, now) {
			return fail("target_context_stale", "a current pinned target context is required")
		}
	}
	resolved, err := Resolve(ResolveInput{AssetType: assetType, Item: item, Contract: contract, Context: contextRow})
	if err != nil {
		return fail("contract_resolution_failed", err.Error())
	}
	metadata := map[string]any{}
	if len(article.PlatformMetadata) > 0 {
		if err := json.Unmarshal(article.PlatformMetadata, &metadata); err != nil {
			return fail("invalid_platform_metadata", "platform metadata is not valid JSON")
		}
	}
	report := ValidateAt(resolved, Artifact{ContentMD: article.ContentMd, Metadata: metadata}, now)
	if contextRow != nil {
		contextField := ""
		switch contract.Platform {
		case "hashnode":
			contextField = "publication"
		case "reddit":
			contextField = "subreddit"
		}
		if contextField != "" && metadataString(metadata, contextField) != contextRow.TargetKey {
			report.Failures = append(report.Failures, Failure{Code: "target_context_mismatch", Message: contextField + " must match the pinned target context"})
			report.Passed = false
		}
	}
	encoded, _ := json.Marshal(report)
	updated, err := q.UpdateArticleContractValidation(ctx, db.UpdateArticleContractValidationParams{ContractValidation: encoded, ID: article.ID, ProjectID: article.ProjectID})
	return updated, report, err
}
