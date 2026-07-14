package platformcontract

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestBuildMatrixDistinguishesGenerationContextAndPublishing(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	contracts := []db.PlatformContentContract{
		contractFixture("blog", "automatic", []string{"long_form_article"}, nil),
		contractFixture("dev_to", "semi_manual", []string{"long_form_article"}, nil),
		contractFixture("reddit", "manual", []string{"community_post", "link_submission"}, []string{"subreddit", "rules"}),
		contractFixture("hacker_news", "manual", []string{"link_submission"}, nil),
	}
	contexts := []db.PlatformTargetContext{{
		ID: uuid.New(), ProjectID: uuid.New(), Platform: "reddit", TargetKey: "r/saas",
		Version: 2, Status: "confirmed", ExpiresAt: pgtype.Timestamptz{Time: now.Add(24 * time.Hour), Valid: true},
	}}

	matrix, err := BuildMatrix(MatrixInput{
		AssetType: "blog_post", Contracts: contracts, Contexts: contexts, Now: now,
		ConnectionReady: map[string]bool{"blog": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	byPlatform := map[string]Capability{}
	for _, item := range matrix {
		byPlatform[item.Platform] = item
	}
	if !byPlatform["blog"].GenerationSupported || !byPlatform["blog"].ConnectionReady || byPlatform["blog"].PublishMode != "automatic" {
		t.Fatalf("blog capability = %+v", byPlatform["blog"])
	}
	if !byPlatform["dev_to"].GenerationSupported || byPlatform["dev_to"].ConnectionReady || byPlatform["dev_to"].PublishMode != "semi_manual" {
		t.Fatalf("dev_to capability = %+v", byPlatform["dev_to"])
	}
	if !byPlatform["reddit"].TargetContextReady || byPlatform["reddit"].OutputType != "community_post" {
		t.Fatalf("reddit capability = %+v", byPlatform["reddit"])
	}
	if byPlatform["hacker_news"].OutputType != "link_submission" {
		t.Fatalf("hacker_news capability = %+v", byPlatform["hacker_news"])
	}
}

func TestBuildMatrixBlocksRedditWithoutFreshContext(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	matrix, err := BuildMatrix(MatrixInput{
		AssetType: "comparison_page",
		Contracts: []db.PlatformContentContract{contractFixture("reddit", "manual", []string{"community_post"}, []string{"subreddit", "rules"})},
		Contexts: []db.PlatformTargetContext{{
			Platform: "reddit", TargetKey: "r/saas", Status: "confirmed",
			ExpiresAt: pgtype.Timestamptz{Time: now.Add(-time.Minute), Valid: true},
		}},
		Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(matrix) != 1 || matrix[0].TargetContextReady || len(matrix[0].BlockReasons) == 0 || matrix[0].BlockReasons[0] != "target_context_required" {
		t.Fatalf("matrix = %+v", matrix)
	}
}

func contractFixture(platform, mode string, outputs, context []string) db.PlatformContentContract {
	assets, _ := json.Marshal(CanonicalAssetTypes())
	outputJSON, _ := json.Marshal(outputs)
	contextJSON, _ := json.Marshal(context)
	return db.PlatformContentContract{
		ID: uuid.New(), Platform: platform, Version: "platform-contract-v1", Status: "active",
		PublishMode: mode, GenerationSupported: true, AllowedOutputTypes: outputJSON,
		CompatibleAssetTypes: assets, RequiredContextFields: contextJSON,
	}
}
