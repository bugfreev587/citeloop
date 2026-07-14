package platformcontract

import (
	"testing"
	"time"
)

func withTargetContext(contract ResolvedContract, targetKey, status string, expiresAt time.Time, allowed []string) ResolvedContract {
	contract.TargetContext = &TargetContextRules{TargetKey: targetKey, Status: status, ExpiresAt: expiresAt, AllowedPostTypes: allowed}
	return contract
}

func TestNativeArtifactContractsDifferByPlatform(t *testing.T) {
	contracts := ContractsV1()
	for _, platform := range []string{"blog", "dev_to", "hashnode", "medium", "linkedin", "reddit", "hacker_news"} {
		contract, ok := contracts[platform]
		if !ok || contract.OutputType == "" || contract.Prompt == "" {
			t.Fatalf("missing native contract for %s: %+v", platform, contract)
		}
	}
	if contracts["dev_to"].Prompt == contracts["linkedin"].Prompt || contracts["reddit"].Prompt == contracts["hacker_news"].Prompt {
		t.Fatal("platform prompts must be native, not renamed copies")
	}
}

func TestValidateNativeArtifacts(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		contract ResolvedContract
		artifact Artifact
		passed   bool
	}{
		{name: "blog mdx", contract: ContractsV1()["blog"], artifact: Artifact{ContentMD: "# Guide\n\nUseful body.", Metadata: map[string]any{"title": "Guide", "slug": "guide"}}, passed: true},
		{name: "devto canonical", contract: ContractsV1()["dev_to"], artifact: Artifact{ContentMD: "# Guide\n\nBody", Metadata: map[string]any{"title": "Guide", "canonical_url": canonicalURLPlaceholder}}, passed: true},
		{name: "devto mdx leak", contract: ContractsV1()["dev_to"], artifact: Artifact{ContentMD: "<Callout>bad</Callout>", Metadata: map[string]any{"title": "Guide", "canonical_url": canonicalURLPlaceholder}}, passed: false},
		{name: "medium missing canonical", contract: ContractsV1()["medium"], artifact: Artifact{ContentMD: "Body", Metadata: map[string]any{"title": "Guide"}}, passed: false},
		{name: "reddit current context", contract: withTargetContext(ContractsV1()["reddit"], "r/saas", "confirmed", now.Add(time.Hour), []string{"community_post"}), artifact: Artifact{ContentMD: "Here is what we learned. Source: " + canonicalURLPlaceholder, Metadata: map[string]any{"title": "A practical workflow", "post_type": "community_post", "subreddit": "r/saas"}}, passed: true},
		{name: "reddit stale context", contract: withTargetContext(ContractsV1()["reddit"], "r/saas", "confirmed", now, []string{"community_post"}), artifact: Artifact{ContentMD: "Body", Metadata: map[string]any{"title": "Guide", "post_type": "community_post", "subreddit": "r/saas"}}, passed: false},
		{name: "hn link package", contract: ContractsV1()["hacker_news"], artifact: Artifact{Metadata: map[string]any{"title": "How deterministic content contracts work", "url": canonicalURLPlaceholder}}, passed: true},
		{name: "hn promotional comment", contract: ContractsV1()["hacker_news"], artifact: Artifact{ContentMD: "Please upvote", Metadata: map[string]any{"title": "Amazing best AI SEO tool", "url": canonicalURLPlaceholder}}, passed: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := ValidateAt(tt.contract, tt.artifact, now)
			if report.Passed != tt.passed {
				t.Fatalf("report = %+v", report)
			}
		})
	}
}
