package sitefix

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/google/uuid"
)

func TestCanonicalApplyRecordsEveryGenerationAttemptOutsideLifecycleTransaction(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	store := &applyStoreStub{fix: db.SiteFix{ID: fixID, ProjectID: projectID, Status: "approved", EvidenceSnapshot: json.RawMessage(`{"finding":{"preserved_propositions":[]}}`)}}
	generator := &fixGeneratorStub{store: store, plan: ApplicationPlan{
		TargetURL: "https://example.com/", NormalizedTargetURL: "https://example.com/",
		OpportunityKey: "doctor:" + fixID.String(), Status: "manual_apply_required",
		SourceFilePaths:    json.RawMessage(`[]`),
		PatchSnapshot:      json.RawMessage(`{"change":"canonical"}`),
		DiffSnapshot:       json.RawMessage(`{}`),
		ResolutionCriteria: json.RawMessage(`{"asset_type":"canonical"}`),
	}}
	service := ApplyService{Store: store, Generator: generator}

	result, err := service.Apply(context.Background(), projectID, fixID)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Application.SiteFixID.Valid || result.Application.ContentActionID.Valid {
		t.Fatalf("application source = site_fix:%v content_action:%v", result.Application.SiteFixID.Valid, result.Application.ContentActionID.Valid)
	}
	want := []string{"load", "find_application", "preparing", "start_ai", "provider", "finish_ai:ok", "finalize"}
	if !reflect.DeepEqual(store.events, want) {
		t.Fatalf("events = %#v, want %#v", store.events, want)
	}
	if store.providerSawLifecycleTransaction {
		t.Fatal("provider was called while the lifecycle transaction was open")
	}

	// A retry is a distinct call record; failed records are never overwritten.
	store.fix.Status = "preparing"
	generator.err = errors.New("provider unavailable")
	if _, err := service.Apply(context.Background(), projectID, fixID); err == nil {
		t.Fatal("expected provider error")
	}
	if store.startedCalls != 2 || store.finishedCalls != 2 {
		t.Fatalf("AI records started=%d finished=%d, want 2/2", store.startedCalls, store.finishedCalls)
	}
	if got := store.events[len(store.events)-1]; got != "finish_ai:failed" {
		t.Fatalf("last event = %q", got)
	}
}

func TestCanonicalApplyRetryReusesExistingApplicationWithoutAI(t *testing.T) {
	projectID, fixID, appID := uuid.New(), uuid.New(), uuid.New()
	store := &applyStoreStub{
		fix:      db.SiteFix{ID: fixID, ProjectID: projectID, Status: "applying"},
		existing: db.SiteChangeApplication{ID: appID, ProjectID: projectID, SiteFixID: validPGUUID(fixID), Status: "ready_for_pr"},
	}
	generator := &fixGeneratorStub{store: store}
	result, err := (ApplyService{Store: store, Generator: generator}).Apply(context.Background(), projectID, fixID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Application.ID != appID || store.startedCalls != 0 {
		t.Fatalf("result=%+v AI starts=%d", result.Application, store.startedCalls)
	}
	if want := []string{"load", "find_application"}; !reflect.DeepEqual(store.events, want) {
		t.Fatalf("events=%v want=%v", store.events, want)
	}
}

func TestDeterministicApplyDoesNotCallProviderWithoutAuthority(t *testing.T) {
	fix := db.SiteFix{ID: uuid.New(), ProjectID: uuid.New(), TargetUrls: json.RawMessage(`["https://example.com/"]`), EvidenceSnapshot: json.RawMessage(`{"finding":{"preserved_propositions":[]}}`), ProposedFix: json.RawMessage(`{"mutations":[{"field":"canonical","operation":"replace"}]}`), AcceptanceTests: json.RawMessage(`[{"type":"canonical_present"}]`)}
	generationContext := GenerationContext{ProductProfile: json.RawMessage(`{}`), ObservedEvidence: fix.EvidenceSnapshot}
	plan, result, err := (DeterministicApplicationGenerator{}).Generate(context.Background(), fix, generationContext)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "skipped" || result.ErrorCode != "doctor_ai_not_authorized" || plan.Status != "manual_apply_required" {
		t.Fatalf("plan=%+v result=%+v", plan, result)
	}
}

func TestDoctorAIApplicationIsGroundedInContextAndObservedEvidence(t *testing.T) {
	provider := &groundingProviderStub{response: `{
		"patch_snapshot":{"change":"make supported fact extractable"},
		"diff_snapshot":{"added_propositions":[]},
		"resolution_criteria":{"acceptance":"supported fact remains sourced"},
		"source_file_paths":["app/page.tsx"],
		"source_mapping_confidence":"high",
		"source_mapping_reason":"mapped from observed target",
		"grounding":{"context_profile_version":7,"primary_intent_before":"describe product","primary_intent_after":"describe product","preserved_propositions":["Existing supported fact."],"added_propositions":[],"removed_propositions":[],"unsupported_claims":[],"source_association_changes":[]}
	}`}
	fix := groundedOptimizationFix()
	contextSnapshot := GenerationContext{ProductProfile: json.RawMessage(`{"positioning":"Existing product context"}`), ProfileVersion: 7, ObservedEvidence: fix.EvidenceSnapshot}
	plan, _, err := (LLMApplicationGenerator{Provider: provider, Model: "test-model"}).Generate(context.Background(), fix, contextSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(provider.request.Prompt, "Existing product context") || !strings.Contains(provider.request.Prompt, "Existing supported fact.") {
		t.Fatalf("prompt is not grounded in Context and evidence: %s", provider.request.Prompt)
	}
	if err := validateApplicationPlan(fix, contextSnapshot, plan); err != nil {
		t.Fatal(err)
	}
}

func TestDoctorAIApplicationRejectsNewFactsAndIntentDrift(t *testing.T) {
	fix := groundedOptimizationFix()
	contextSnapshot := GenerationContext{ProductProfile: json.RawMessage(`{"positioning":"Existing product context"}`), ProfileVersion: 7, ObservedEvidence: fix.EvidenceSnapshot}
	base := ApplicationPlan{
		TargetURL: "https://example.com/product", NormalizedTargetURL: "https://example.com/product",
		OpportunityKey: "doctor:" + fix.ID.String(), Status: "ready_for_pr",
		SourceFilePaths: json.RawMessage(`["app/page.tsx"]`), PatchSnapshot: json.RawMessage(`{"change":"safe"}`),
		DiffSnapshot: json.RawMessage(`{"added_propositions":[]}`), ResolutionCriteria: json.RawMessage(`{"acceptance":"safe"}`),
		GroundingSnapshot: json.RawMessage(`{"context_profile_version":7,"primary_intent_before":"describe product","primary_intent_after":"describe product","preserved_propositions":["Existing supported fact."],"added_propositions":[],"removed_propositions":[],"unsupported_claims":[],"source_association_changes":[]}`),
	}
	tests := []struct {
		name   string
		mutate func(*ApplicationPlan)
	}{
		{"declared added proposition", func(plan *ApplicationPlan) {
			plan.GroundingSnapshot = json.RawMessage(`{"context_profile_version":7,"primary_intent_before":"describe product","primary_intent_after":"describe product","preserved_propositions":["Existing supported fact."],"added_propositions":["Invented claim."],"removed_propositions":[],"unsupported_claims":[],"source_association_changes":[]}`)
		}},
		{"unsupported claim", func(plan *ApplicationPlan) {
			plan.GroundingSnapshot = json.RawMessage(`{"context_profile_version":7,"primary_intent_before":"describe product","primary_intent_after":"describe product","preserved_propositions":["Existing supported fact."],"added_propositions":[],"removed_propositions":[],"unsupported_claims":["Invented claim."],"source_association_changes":[]}`)
		}},
		{"intent drift", func(plan *ApplicationPlan) {
			plan.GroundingSnapshot = json.RawMessage(`{"context_profile_version":7,"primary_intent_before":"describe product","primary_intent_after":"sell a new offer","preserved_propositions":["Existing supported fact."],"added_propositions":[],"removed_propositions":[],"unsupported_claims":[],"source_association_changes":[]}`)
		}},
		{"hidden added proposition", func(plan *ApplicationPlan) {
			plan.PatchSnapshot = json.RawMessage(`{"added_propositions":["Invented claim."]}`)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := base
			tt.mutate(&plan)
			if err := validateApplicationPlan(fix, contextSnapshot, plan); err == nil {
				t.Fatal("unsafe generated application was accepted")
			}
		})
	}
}

func groundedOptimizationFix() db.SiteFix {
	return db.SiteFix{
		ID: uuid.New(), ProjectID: uuid.New(), Status: "preparing", FindingKind: "optimization",
		TargetUrls:       json.RawMessage(`["https://example.com/product"]`),
		EvidenceSnapshot: json.RawMessage(`{"finding":{"primary_intent_before":"describe product","primary_intent_after":"describe product","preserved_propositions":["Existing supported fact."],"added_propositions":[],"removed_propositions":[]}}`),
		ProposedFix:      json.RawMessage(`{"fix_intent":"make existing fact extractable"}`), AcceptanceTests: json.RawMessage(`[{"type":"content_evidence_present"}]`),
	}
}

type groundingProviderStub struct {
	request  llm.CompletionReq
	response string
}

func (p *groundingProviderStub) Complete(_ context.Context, req llm.CompletionReq) (llm.CompletionResp, error) {
	p.request = req
	return llm.CompletionResp{Text: p.response, Provider: "test", Model: "test-model"}, nil
}

type applyStoreStub struct {
	fix                             db.SiteFix
	events                          []string
	startedCalls                    int
	finishedCalls                   int
	lifecycleTransactionOpen        bool
	providerSawLifecycleTransaction bool
	existing                        db.SiteChangeApplication
}

func (s *applyStoreStub) Load(_ context.Context, projectID, fixID uuid.UUID) (db.SiteFix, error) {
	s.events = append(s.events, "load")
	if s.fix.ProjectID != projectID || s.fix.ID != fixID {
		return db.SiteFix{}, ErrProjectMismatch
	}
	return s.fix, nil
}
func (s *applyStoreStub) LoadGenerationContext(_ context.Context, fix db.SiteFix) (GenerationContext, error) {
	return GenerationContext{ProductProfile: json.RawMessage(`{"positioning":"test context"}`), ProfileVersion: 1, ObservedEvidence: fix.EvidenceSnapshot}, nil
}
func (s *applyStoreStub) MarkPreparing(_ context.Context, fix db.SiteFix) (db.SiteFix, error) {
	s.events = append(s.events, "preparing")
	fix.Status = "preparing"
	s.fix = fix
	return fix, nil
}
func (s *applyStoreStub) FindApplication(_ context.Context, _ db.SiteFix) (db.SiteChangeApplication, bool, error) {
	s.events = append(s.events, "find_application")
	return s.existing, s.existing.ID != uuid.Nil, nil
}
func (s *applyStoreStub) StartGeneration(_ context.Context, _ db.SiteFix, _ GenerationCall) (uuid.UUID, error) {
	s.events = append(s.events, "start_ai")
	s.startedCalls++
	return uuid.New(), nil
}
func (s *applyStoreStub) FinishGeneration(_ context.Context, _ db.SiteFix, _ uuid.UUID, result GenerationResult) error {
	s.events = append(s.events, "finish_ai:"+result.Status)
	s.finishedCalls++
	return nil
}
func (s *applyStoreStub) Finalize(_ context.Context, fix db.SiteFix, plan ApplicationPlan) (db.SiteFix, db.SiteChangeApplication, error) {
	s.events = append(s.events, "finalize")
	s.lifecycleTransactionOpen = true
	defer func() { s.lifecycleTransactionOpen = false }()
	fix.Status = "applying"
	s.fix = fix
	return fix, db.SiteChangeApplication{ID: uuid.New(), ProjectID: fix.ProjectID, SiteFixID: validPGUUID(fix.ID), Status: plan.Status}, nil
}

type fixGeneratorStub struct {
	store *applyStoreStub
	plan  ApplicationPlan
	err   error
}

func (g *fixGeneratorStub) Describe(db.SiteFix, GenerationContext) GenerationCall {
	return GenerationCall{Provider: "test", Model: "model", PromptVersion: "doctor-fix-v1", RequestFingerprint: "fingerprint"}
}
func (g *fixGeneratorStub) Generate(_ context.Context, fix db.SiteFix, generationContext GenerationContext) (ApplicationPlan, GenerationResult, error) {
	g.store.events = append(g.store.events, "provider")
	g.store.providerSawLifecycleTransaction = g.store.lifecycleTransactionOpen
	plan, result, err := g.generate()
	if err == nil && len(plan.GroundingSnapshot) == 0 {
		plan.GroundingSnapshot, err = approvedGroundingSnapshot(fix, generationContext)
	}
	return plan, result, err
}
func (g *fixGeneratorStub) generate() (ApplicationPlan, GenerationResult, error) {
	// This test generator cannot see the store directly; ApplyService emits the
	// provider test seam event before invoking it.
	if g.err != nil {
		return ApplicationPlan{}, GenerationResult{Status: "failed", ErrorCode: "provider_unavailable"}, g.err
	}
	return g.plan, GenerationResult{Status: "ok", TotalTokens: 42}, nil
}
