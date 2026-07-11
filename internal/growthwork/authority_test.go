package growthwork

import (
	"context"
	"testing"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/discovery"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/google/uuid"
)

func TestComparatorAuthorityRequiresExplicitGrowthConsent(t *testing.T) {
	for _, doctorEnabled := range []bool{false, true} {
		cfg := config.Default()
		cfg.GrowthAIEnabled = false
		cfg.DoctorAIEnabled = doctorEnabled
		provider := &authorityProviderStub{}
		comparator := (ComparatorAuthority{Provider: provider, Model: "planning-model"}).ForConfig(cfg, config.GrowthAITriggerManual)
		if comparator != nil {
			t.Fatalf("Doctor AI=%v leaked authority into Growth", doctorEnabled)
		}
		if provider.calls != 0 {
			t.Fatalf("provider calls=%d, want 0", provider.calls)
		}
	}
}

func TestComparatorAuthorityUsesPlanningPurposeAndPreservesUsage(t *testing.T) {
	cfg := config.Default()
	cfg.GrowthAIEnabled = true
	cfg.GrowthAIRunPolicy = config.GrowthAIRunPolicyManualOnly
	provider := &authorityProviderStub{response: llm.CompletionResp{
		Text:  `{"decision":"create","owner":"opportunities","work_signature":"candidate","overlaps":[],"reason":"distinct","confidence":0.99}`,
		Model: "planning-model", Tokens: 17, CostUSD: 0.0042,
	}}
	comparator := (ComparatorAuthority{Provider: provider, Model: "planning-model"}).ForConfig(cfg, config.GrowthAITriggerManual)
	if comparator == nil {
		t.Fatal("authorized Growth comparator is nil")
	}
	_, usage, err := comparator.Compare(context.Background(), discovery.SemanticRequest{
		CandidateID: uuid.New(), Candidate: discovery.Candidate{ProjectID: uuid.New(), SuggestedOwner: discovery.OwnerOpportunities},
		Identity: discovery.Identity{ExactSignatureHash: "candidate", SignaturePayload: []byte(`{"project_id":"p","change_family":"growth","normalized_target_set":["https://example.com"],"normalized_mutations":[{"operation":"add","field":"content"}]}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.request.Purpose != llm.PurposeDefault || usage.TotalTokens != 17 || usage.CostUSD != 0.0042 {
		t.Fatalf("purpose=%q usage=%+v", provider.request.Purpose, usage)
	}
}

type authorityProviderStub struct {
	calls    int
	request  llm.CompletionReq
	response llm.CompletionResp
}

func (p *authorityProviderStub) Complete(_ context.Context, req llm.CompletionReq) (llm.CompletionResp, error) {
	p.calls++
	p.request = req
	return p.response, nil
}
