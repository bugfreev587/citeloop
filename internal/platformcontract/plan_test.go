package platformcontract

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestPrepareTargetPlanRequiresCanonicalAndPreservesExactTargets(t *testing.T) {
	blogContract := uuid.New()
	redditContract := uuid.New()
	prepared, err := PreparePlan(PlanInput{
		ProjectID: uuid.New(), AssetType: "integration_docs_page", CanonicalTarget: Target{Platform: "blog"},
		Targets: []Target{
			{Platform: "reddit", TargetKey: "r/SaaS", OutputType: "community_post", ContractID: redditContract, ContractVersion: "v1"},
			{Platform: "blog", OutputType: "long_form_article", ContractID: blogContract, ContractVersion: "v1"},
			{Platform: "reddit", TargetKey: "r/saas", OutputType: "community_post", ContractID: redditContract, ContractVersion: "v1"},
		}, SelectionMode: "contract_matrix",
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.AssetType != "integration_page" || prepared.CanonicalTarget.Platform != "blog" {
		t.Fatalf("plan identity = %+v", prepared)
	}
	if len(prepared.Targets) != 2 || prepared.Targets[0].Platform != "blog" || !prepared.Targets[0].IsCanonical {
		t.Fatalf("targets = %+v", prepared.Targets)
	}
	if prepared.Targets[1].TargetKey != "r/saas" || prepared.Targets[1].IsCanonical {
		t.Fatalf("reddit target = %+v", prepared.Targets[1])
	}
	if DeriveChannel(prepared) != "both" {
		t.Fatalf("channel = %q", DeriveChannel(prepared))
	}
}

func TestValidatePlanSelectionEnforcesPinnedCapabilityAndContext(t *testing.T) {
	projectID, blogID, hashnodeID := uuid.New(), uuid.New(), uuid.New()
	contracts := []db.PlatformContentContract{
		{ID: blogID, Platform: "blog", Version: "v1", Status: "active", GenerationSupported: true, AllowedOutputTypes: json.RawMessage(`["long_form_article"]`), CompatibleAssetTypes: json.RawMessage(`["blog_post"]`), RequiredContextFields: json.RawMessage(`[]`)},
		{ID: hashnodeID, Platform: "hashnode", Version: "v1", Status: "active", GenerationSupported: true, AllowedOutputTypes: json.RawMessage(`["long_form_article"]`), CompatibleAssetTypes: json.RawMessage(`["blog_post"]`), RequiredContextFields: json.RawMessage(`["publication"]`)},
	}
	input := PlanInput{ProjectID: projectID, AssetType: "blog_post", CanonicalTarget: Target{Platform: "blog"}, Targets: []Target{
		{Platform: "blog", OutputType: "long_form_article", ContractID: blogID, ContractVersion: "v1"},
		{Platform: "hashnode", TargetKey: "publication", OutputType: "long_form_article", ContractID: hashnodeID, ContractVersion: "v1"},
	}}
	if err := ValidatePlanSelection(input, contracts, nil, time.Now()); err == nil {
		t.Fatal("context-required target was accepted without a pinned context")
	}
	contextID := uuid.New()
	version := int32(2)
	input.Targets[1].TargetContextID, input.Targets[1].TargetContextVersion = contextID, &version
	contexts := []db.PlatformTargetContext{{ID: contextID, ProjectID: projectID, Platform: "hashnode", TargetKey: "publication", Version: version, Status: "confirmed", ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true}}}
	if err := ValidatePlanSelection(input, contracts, contexts, time.Now()); err != nil {
		t.Fatalf("valid exact selection rejected: %v", err)
	}
	input.Targets[1].ContractVersion = "stale"
	if err := ValidatePlanSelection(input, contracts, contexts, time.Now()); err == nil {
		t.Fatal("stale contract version accepted")
	}
}

func TestPrepareTargetPlanRejectsMissingOrUnplannedCanonical(t *testing.T) {
	base := PlanInput{ProjectID: uuid.New(), AssetType: "blog_post", Targets: []Target{{Platform: "dev_to", ContractID: uuid.New(), ContractVersion: "v1", OutputType: "long_form_article"}}}
	if _, err := PreparePlan(base); err == nil {
		t.Fatal("missing canonical should fail")
	}
	base.CanonicalTarget = Target{Platform: "blog"}
	if _, err := PreparePlan(base); err == nil {
		t.Fatal("canonical absent from targets should fail")
	}
}

func TestPrepareTargetPlanRejectsExternalCanonical(t *testing.T) {
	contractID := uuid.New()
	_, err := PreparePlan(PlanInput{
		ProjectID: uuid.New(), AssetType: "blog_post",
		CanonicalTarget: Target{Platform: "dev_to"},
		Targets:         []Target{{Platform: "dev_to", ContractID: contractID, ContractVersion: "v1", OutputType: "long_form_article"}},
	})
	if err == nil {
		t.Fatal("external platform was accepted as canonical")
	}
}

func TestDeriveChannelUsesLegacySummaryOnly(t *testing.T) {
	if got := DeriveChannel(Plan{PlanInput: PlanInput{Targets: []Target{{Platform: "blog", IsCanonical: true}}}}); got != "blog" {
		t.Fatalf("blog = %q", got)
	}
	if got := DeriveChannel(Plan{PlanInput: PlanInput{Targets: []Target{{Platform: "reddit"}}}}); got != "syndication" {
		t.Fatalf("syndication = %q", got)
	}
}
