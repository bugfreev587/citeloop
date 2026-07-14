package platformcontract

import (
	"testing"

	"github.com/google/uuid"
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

func TestDeriveChannelUsesLegacySummaryOnly(t *testing.T) {
	if got := DeriveChannel(Plan{PlanInput: PlanInput{Targets: []Target{{Platform: "blog", IsCanonical: true}}}}); got != "blog" {
		t.Fatalf("blog = %q", got)
	}
	if got := DeriveChannel(Plan{PlanInput: PlanInput{Targets: []Target{{Platform: "reddit"}}}}); got != "syndication" {
		t.Fatalf("syndication = %q", got)
	}
}
