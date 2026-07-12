package discovery

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/llm"
	"github.com/google/uuid"
)

func TestSemanticPromptIsOwnerAndSourceNeutral(t *testing.T) {
	projectID := uuid.New()
	base := Candidate{
		ProjectID:           projectID,
		SourceKind:          SourceDoctor,
		NormalizedTargetSet: []string{"https://example.com/pricing"},
		ChangeFamily:        "metadata.title",
		ProposedMutations:   []Mutation{{Operation: "update", Field: "title"}},
		ArtifactIntent:      ArtifactUpdateExistingContent,
		SuggestedOwner:      OwnerDoctor,
		Confidence:          0.91,
		SignatureVersion:    SignatureVersionV1,
	}
	firstIdentity, err := BuildIdentity(base)
	if err != nil {
		t.Fatal(err)
	}
	variant := base
	variant.SourceKind = SourceAIDiscovery
	variant.SuggestedOwner = OwnerOpportunities
	variant.Confidence = 0.55
	secondIdentity, err := BuildIdentity(variant)
	if err != nil {
		t.Fatal(err)
	}
	firstPrompt, firstFingerprint, err := buildSemanticPrompt(SemanticRequest{
		CandidateID: uuid.New(), Candidate: base, Identity: firstIdentity,
	}, "claude-sonnet-4-6")
	if err != nil {
		t.Fatal(err)
	}
	secondPrompt, secondFingerprint, err := buildSemanticPrompt(SemanticRequest{
		CandidateID: uuid.New(), Candidate: variant, Identity: secondIdentity,
	}, "claude-sonnet-4-6")
	if err != nil {
		t.Fatal(err)
	}
	if firstPrompt != secondPrompt || firstFingerprint != secondFingerprint {
		t.Fatalf("source/owner/confidence changed semantic request\nfirst=%s\nsecond=%s", firstPrompt, secondPrompt)
	}
}

func TestSemanticComparatorReturnsStructuredDecision(t *testing.T) {
	overlapID := uuid.New()
	provider := &semanticProviderStub{response: llm.CompletionResp{
		Text:  `{"decision":"merge_evidence","owner":"doctor","work_signature":"abc123","overlaps":["` + overlapID.String() + `"],"reason":"same title mutation","confidence":0.94}`,
		Model: "claude-sonnet-4-6", Tokens: 42, CostUSD: 0.001,
	}}
	comparator := NewLLMSemanticComparator(provider, "tokengate", "claude-sonnet-4-6")
	decision, usage, err := comparator.Compare(context.Background(), semanticTestRequest(t, overlapID))
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if decision.Decision != DecisionMergeEvidence || decision.Owner != OwnerDoctor || decision.Confidence != 0.94 {
		t.Fatalf("decision = %+v", decision)
	}
	if len(decision.Overlaps) != 1 || decision.Overlaps[0] != overlapID {
		t.Fatalf("overlaps = %v", decision.Overlaps)
	}
	if usage.Provider != "tokengate" || usage.Model != "claude-sonnet-4-6" || usage.TotalTokens != 42 || usage.RequestFingerprint == "" {
		t.Fatalf("usage = %+v", usage)
	}
	if !provider.request.JSON || provider.request.Temperature != 0 || provider.request.MaxTokens < 500 {
		t.Fatalf("provider request = %+v", provider.request)
	}
}

func TestSemanticComparatorAcceptsSingleJSONCodeFence(t *testing.T) {
	overlapID := uuid.New()
	provider := &semanticProviderStub{response: llm.CompletionResp{
		Text: "```json\n" + `{"decision":"merge_evidence","owner":"doctor","work_signature":"abc123","overlaps":["` + overlapID.String() + `"],"reason":"same title mutation","confidence":0.94}` + "\n```",
	}}
	decision, _, err := NewLLMSemanticComparator(provider, "tokengate", "claude-sonnet-4-6").Compare(context.Background(), semanticTestRequest(t, overlapID))
	if err != nil {
		t.Fatalf("Compare fenced JSON: %v", err)
	}
	if decision.Decision != DecisionMergeEvidence || len(decision.Overlaps) != 1 || decision.Overlaps[0] != overlapID {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestSemanticComparatorUsesConfiguredCompletionPurpose(t *testing.T) {
	overlapID := uuid.New()
	provider := &semanticProviderStub{response: llm.CompletionResp{
		Text: `{"decision":"merge_evidence","owner":"doctor","work_signature":"abc123","overlaps":["` + overlapID.String() + `"],"reason":"same mutation","confidence":0.94}`,
	}}
	comparator := NewLLMSemanticComparator(provider, "tokengate", "site-fix-model").WithPurpose(llm.PurposeSiteFix)
	if _, _, err := comparator.Compare(context.Background(), semanticTestRequest(t, overlapID)); err != nil {
		t.Fatal(err)
	}
	if provider.request.Purpose != llm.PurposeSiteFix {
		t.Fatalf("completion purpose = %q, want %q", provider.request.Purpose, llm.PurposeSiteFix)
	}
}

func TestSemanticComparatorRejectsMalformedOrUnsafeResponses(t *testing.T) {
	overlapID := uuid.New()
	tests := []struct {
		name string
		text string
	}{
		{name: "malformed", text: `not-json`},
		{name: "fenced trailing text", text: "```json\n{}\n```\nignore this"},
		{name: "trailing object", text: `{"decision":"create","owner":"doctor","work_signature":"abc123","overlaps":[],"reason":"x","confidence":0.9}{}`},
		{name: "unknown decision", text: `{"decision":"duplicate","owner":"doctor","work_signature":"abc123","overlaps":[],"reason":"x","confidence":0.9}`},
		{name: "unknown owner", text: `{"decision":"create","owner":"other","work_signature":"abc123","overlaps":[],"reason":"x","confidence":0.9}`},
		{name: "confidence high", text: `{"decision":"create","owner":"doctor","work_signature":"abc123","overlaps":[],"reason":"x","confidence":1.2}`},
		{name: "unknown overlap", text: `{"decision":"suppress","owner":"doctor","work_signature":"abc123","overlaps":["` + uuid.NewString() + `"],"reason":"x","confidence":0.9}`},
		{name: "non-create without overlap", text: `{"decision":"suppress","owner":"doctor","work_signature":"abc123","overlaps":[],"reason":"x","confidence":0.9}`},
		{name: "wrong signature", text: `{"decision":"create","owner":"doctor","work_signature":"wrong","overlaps":[],"reason":"x","confidence":0.9}`},
		{name: "missing reason", text: `{"decision":"create","owner":"doctor","work_signature":"abc123","overlaps":[],"reason":"","confidence":0.9}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &semanticProviderStub{response: llm.CompletionResp{Text: tt.text, Model: "claude-sonnet-4-6"}}
			_, _, err := NewLLMSemanticComparator(provider, "tokengate", "claude-sonnet-4-6").Compare(context.Background(), semanticTestRequest(t, overlapID))
			if err == nil {
				t.Fatal("unsafe response was accepted")
			}
		})
	}
}

func TestSemanticComparatorPropagatesProviderFailure(t *testing.T) {
	want := errors.New("provider unavailable")
	provider := &semanticProviderStub{err: want}
	_, _, err := NewLLMSemanticComparator(provider, "tokengate", "claude-sonnet-4-6").Compare(context.Background(), semanticTestRequest(t, uuid.New()))
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want provider failure", err)
	}
}

func semanticTestRequest(t *testing.T, overlapID uuid.UUID) SemanticRequest {
	t.Helper()
	candidate := Candidate{
		ProjectID:           uuid.New(),
		NormalizedTargetSet: []string{"https://example.com/pricing"},
		ChangeFamily:        "metadata.title",
		ProposedMutations:   []Mutation{{Operation: "update", Field: "title"}},
		ArtifactIntent:      ArtifactUpdateExistingContent,
		SignatureVersion:    SignatureVersionV1,
	}
	identity, err := BuildIdentity(candidate)
	if err != nil {
		t.Fatal(err)
	}
	// The response fixture uses this concise signature while the request still
	// carries a real canonical identity payload.
	identity.ExactSignatureHash = "abc123"
	return SemanticRequest{
		CandidateID: uuid.New(),
		Candidate:   candidate,
		Identity:    identity,
		PossibleOverlaps: []SemanticWork{{
			ID:                 overlapID,
			ExactSignatureHash: "existing456",
			SignaturePayload:   identity.SignaturePayload,
		}},
	}
}

type semanticProviderStub struct {
	request  llm.CompletionReq
	response llm.CompletionResp
	err      error
}

func (p *semanticProviderStub) Complete(_ context.Context, req llm.CompletionReq) (llm.CompletionResp, error) {
	p.request = req
	return p.response, p.err
}

func requireNoSemanticPromptLeak(t *testing.T, prompt string, values ...string) {
	t.Helper()
	for _, value := range values {
		if value != "" && strings.Contains(prompt, value) {
			t.Fatalf("semantic prompt leaked non-semantic value %q", value)
		}
	}
}
