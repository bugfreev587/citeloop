package platformcontract

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type articleValidationStore struct {
	topic    db.Topic
	contract db.PlatformContentContract
	context  db.PlatformTargetContext
}

func (s *articleValidationStore) GetTopicForProject(context.Context, db.GetTopicForProjectParams) (db.Topic, error) {
	return s.topic, nil
}
func (s *articleValidationStore) GetPlatformContentContractByID(context.Context, db.GetPlatformContentContractByIDParams) (db.PlatformContentContract, error) {
	return s.contract, nil
}
func (s *articleValidationStore) GetPlatformTargetContextForProject(context.Context, db.GetPlatformTargetContextForProjectParams) (db.PlatformTargetContext, error) {
	return s.context, nil
}
func (s *articleValidationStore) UpdateArticleContractValidation(_ context.Context, arg db.UpdateArticleContractValidationParams) (db.Article, error) {
	return db.Article{ID: arg.ID, ProjectID: arg.ProjectID, ContractValidation: arg.ContractValidation}, nil
}

func TestRevalidateArticleRejectsLegacyAndEditedLinkOnlyArtifact(t *testing.T) {
	projectID, topicID, articleID, contractID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	assetType := "blog_post"
	store := &articleValidationStore{
		topic: db.Topic{ID: topicID, ProjectID: projectID, AssetType: &assetType},
		contract: db.PlatformContentContract{
			ID: contractID, Platform: "hacker_news", Version: "v1", Status: "active", GenerationSupported: true,
			AllowedOutputTypes: json.RawMessage(`["link_submission"]`), CompatibleAssetTypes: json.RawMessage(`["blog_post"]`), RequiredContextFields: json.RawMessage(`[]`),
		},
	}
	legacy := db.Article{ID: articleID, ProjectID: projectID, TopicID: topicID}
	_, report, err := RevalidateArticle(context.Background(), store, legacy, time.Now())
	if err != nil || report.Passed || report.Failures[0].Code != "unvalidated_legacy_artifact" {
		t.Fatalf("legacy report=%+v err=%v", report, err)
	}
	version := "v1"
	edited := db.Article{
		ID: articleID, ProjectID: projectID, TopicID: topicID, Kind: "syndication_variant", Platform: stringPtrTest("hacker_news"),
		OutputType: "link_submission", PlatformContractID: pgtype.UUID{Bytes: contractID, Valid: true}, PlatformContractVersion: &version,
		PlatformMetadata: json.RawMessage(`{"title":"A factual title","url":"{{CANONICAL_URL}}"}`), ContentMd: "A generated comment that violates link-only.",
	}
	_, report, err = RevalidateArticle(context.Background(), store, edited, time.Now())
	if err != nil || report.Passed || report.Failures[0].Code != "link_only" {
		t.Fatalf("edited HN report=%+v err=%v", report, err)
	}
}

func TestRevalidateArticlePreservesDeprecatedPinnedContract(t *testing.T) {
	projectID, topicID, articleID, contractID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	assetType, version := "blog_post", "legacy-v1"
	store := &articleValidationStore{
		topic: db.Topic{ID: topicID, ProjectID: projectID, AssetType: &assetType},
		contract: db.PlatformContentContract{
			ID: contractID, Platform: "blog", Version: version, Status: "deprecated",
			AllowedOutputTypes: json.RawMessage(`["long_form_article"]`), CompatibleAssetTypes: json.RawMessage(`["blog_post"]`), RequiredContextFields: json.RawMessage(`[]`),
		},
	}
	article := db.Article{
		ID: articleID, ProjectID: projectID, TopicID: topicID, Kind: "canonical", OutputType: "long_form_article",
		PlatformContractID: pgtype.UUID{Bytes: contractID, Valid: true}, PlatformContractVersion: &version,
		PlatformMetadata: json.RawMessage(`{"title":"Legacy guide","slug":"legacy-guide"}`), ContentMd: "# Legacy guide",
	}
	_, report, err := RevalidateArticle(context.Background(), store, article, time.Now())
	if err != nil || !report.Passed {
		t.Fatalf("deprecated pinned contract must remain usable: report=%+v err=%v", report, err)
	}
}

func stringPtrTest(value string) *string { return &value }
