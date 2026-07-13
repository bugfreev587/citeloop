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
	verifier := &patchVerifierStub{store: store, verification: PatchVerification{
		Approved: true, PrimaryIntentPreserved: true, PreservedPropositions: []string{},
	}}
	service := applyServiceForTest(store, generator, verifier)

	result, err := service.Apply(context.Background(), projectID, fixID)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Application.SiteFixID.Valid || result.Application.ContentActionID.Valid {
		t.Fatalf("application source = site_fix:%v content_action:%v", result.Application.SiteFixID.Valid, result.Application.ContentActionID.Valid)
	}
	want := []string{"load", "find_application", "preparing", "start_selector", "selector", "finish_selector:ok", "start_ai", "provider", "finish_ai:ok", "start_verifier", "verifier", "finish_verifier:ok", "finalize"}
	if !reflect.DeepEqual(store.events, want) {
		t.Fatalf("events = %#v, want %#v", store.events, want)
	}
	if store.providerSawLifecycleTransaction || store.verifierSawLifecycleTransaction {
		t.Fatal("provider or independent verifier was called while the lifecycle transaction was open")
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
	result, err := applyServiceForTest(store, generator, &patchVerifierStub{store: store}).Apply(context.Background(), projectID, fixID)
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

func TestCanonicalApplyPreflightFailureIsSkippedNotProviderCall(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	store := &applyStoreStub{fix: groundedOptimizationFix()}
	store.fix.ID, store.fix.ProjectID, store.fix.Status = fixID, projectID, "approved"
	generator := &fixGeneratorStub{store: store, err: errors.New("credential lookup failed"), skipAttempt: true}
	_, err := applyServiceForTest(store, generator, &patchVerifierStub{store: store}).Apply(context.Background(), projectID, fixID)
	if err == nil {
		t.Fatal("expected preflight error")
	}
	if got := store.events[len(store.events)-1]; got != "finish_ai:skipped" {
		t.Fatalf("events=%v", store.events)
	}
	if store.preparationFailure == "" || strings.Contains(store.preparationFailure, "credential lookup failed") {
		t.Fatalf("preparation failure was not stored as a controlled code: %q", store.preparationFailure)
	}
}

func TestCanonicalApplyRejectsSilentObservableGeneration(t *testing.T) {
	fix := groundedOptimizationFix()
	fix.Status = "approved"
	store := &applyStoreStub{fix: fix}
	provider := silentSuccessSiteFixProvider{response: `{
		"patch_snapshot":{"change":"make supported fact extractable"},"diff_snapshot":{"added_propositions":[]},
		"resolution_criteria":{"acceptance":"supported fact remains sourced"},"source_file_paths":[],
		"source_mapping_confidence":"low","source_mapping_reason":"manual mapping",
		"grounding":{"context_profile_version":1,"primary_intent_before":"describe product","primary_intent_after":"describe product","preserved_propositions":["Existing supported fact."],"added_propositions":[],"removed_propositions":[],"unsupported_claims":[],"source_association_changes":[]}}`}
	_, err := applyServiceForTest(store, LLMApplicationGenerator{Provider: provider, Model: "test"}, &patchVerifierStub{store: store}).Apply(context.Background(), fix.ProjectID, fix.ID)
	if err == nil || store.events[len(store.events)-1] != "finish_ai:skipped" || slicesContain(store.events, "finalize") {
		t.Fatalf("err=%v events=%v", err, store.events)
	}
}

func TestCanonicalApplyRejectsSilentObservableGrounding(t *testing.T) {
	fix := groundedOptimizationFix()
	fix.Status = "approved"
	store := &applyStoreStub{fix: fix}
	generator := &fixGeneratorStub{store: store, plan: ApplicationPlan{
		TargetURL: "https://example.com/product", NormalizedTargetURL: "https://example.com/product", OpportunityKey: "doctor:" + fix.ID.String(), Status: "manual_apply_required",
		SourceFilePaths: json.RawMessage(`[]`), PatchSnapshot: json.RawMessage(`{"change":"safe"}`), DiffSnapshot: json.RawMessage(`{}`), ResolutionCriteria: json.RawMessage(`{"acceptance":"safe"}`),
	}}
	provider := silentSuccessSiteFixProvider{response: `{"approved":true,"primary_intent_preserved":true,"preserved_propositions":["Existing supported fact."],"added_propositions":[],"removed_propositions":[],"unsupported_claims":[],"intent_drift":false,"reason":"grounded"}`}
	_, err := applyServiceForTest(store, generator, LLMPatchGroundingVerifier{Provider: provider, Model: "test"}).Apply(context.Background(), fix.ProjectID, fix.ID)
	if err == nil || store.events[len(store.events)-1] != "finish_verifier:skipped" || slicesContain(store.events, "finalize") {
		t.Fatalf("err=%v events=%v", err, store.events)
	}
}

func slicesContain(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type silentSuccessSiteFixProvider struct{ response string }

func (silentSuccessSiteFixProvider) ObservesProviderAttempts() {}
func (p silentSuccessSiteFixProvider) Complete(context.Context, llm.CompletionReq) (llm.CompletionResp, error) {
	return llm.CompletionResp{Text: p.response, Provider: "silent", Model: "silent"}, nil
}

func TestCanonicalApplyIndependentlyRejectsUnsupportedClaimHiddenByGeneratorSelfReport(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	fix := groundedOptimizationFix()
	fix.ID, fix.ProjectID, fix.Status = fixID, projectID, "approved"
	store := &applyStoreStub{fix: fix, generationContext: GenerationContext{
		ProductProfile: json.RawMessage(`{"positioning":"Existing product context"}`),
		ProfileVersion: 7, ObservedEvidence: fix.EvidenceSnapshot,
	}}
	generator := &fixGeneratorStub{store: store, plan: ApplicationPlan{
		TargetURL: "https://example.com/product", NormalizedTargetURL: "https://example.com/product",
		OpportunityKey: "doctor:" + fixID.String(), Status: "ready_for_pr",
		SourceFilePaths:    json.RawMessage(`["app/page.tsx"]`),
		PatchSnapshot:      json.RawMessage(`{"replacement_text":"CiteLoop guarantees 300% revenue growth."}`),
		DiffSnapshot:       json.RawMessage(`{"added_propositions":[]}`),
		ResolutionCriteria: json.RawMessage(`{"acceptance":"existing page remains available"}`),
	}}
	provider := &groundingProviderStub{response: `{
		"approved":false,
		"primary_intent_preserved":false,
		"preserved_propositions":["Existing supported fact."],
		"added_propositions":["CiteLoop guarantees 300% revenue growth."],
		"removed_propositions":[],
		"unsupported_claims":["CiteLoop guarantees 300% revenue growth."],
		"intent_drift":true,
		"reason":"The replacement introduces an unsupported commercial guarantee."
	}`}
	verifier := LLMPatchGroundingVerifier{Provider: provider, Model: "verifier-model"}

	_, err := applyServiceForTest(store, generator, verifier).Apply(context.Background(), projectID, fixID)
	if err == nil || !errors.Is(err, ErrPatchGroundingRejected) {
		t.Fatalf("Apply error = %v, want independent grounding rejection", err)
	}
	if !strings.Contains(provider.request.Prompt, "CiteLoop guarantees 300% revenue growth.") {
		t.Fatalf("independent verifier did not receive actual patch text: %s", provider.request.Prompt)
	}
	if strings.Contains(provider.request.Prompt, "grounding_validation") {
		t.Fatalf("independent verifier received the generator's grounding self-report: %s", provider.request.Prompt)
	}
	if strings.Contains(strings.Join(store.events, ","), "finalize") {
		t.Fatalf("rejected patch was finalized: %v", store.events)
	}
	wantTail := []string{"finish_ai:ok", "start_verifier", "finish_verifier:failed"}
	joined := strings.Join(store.events, ",")
	for _, event := range wantTail {
		if !strings.Contains(joined, event) {
			t.Fatalf("verifier call was not fully ledgered; missing %q in %v", event, store.events)
		}
	}
}

func TestGroundingVerifierRequestRejectsUnrelatedRepositoryEdits(t *testing.T) {
	fix := groundedOptimizationFix()
	contextSnapshot := GenerationContext{ProductProfile: json.RawMessage(`{"positioning":"Existing product context"}`), ProfileVersion: 7, ObservedEvidence: fix.EvidenceSnapshot}
	plan := ApplicationPlan{
		PatchSnapshot:      json.RawMessage(`{"repo":"acme/site","base_branch":"main","base_commit_sha":"commit-1","files":[{"path":"app/page.tsx","base_sha":"blob-1","replacements":[{"old_text":"Old","new_text":"New"}]}]}`),
		DiffSnapshot:       json.RawMessage(`{"files":[{"path":"app/page.tsx","base_sha":"blob-1","changes":[{"before":"Old","after":"New"}]}]}`),
		ResolutionCriteria: json.RawMessage(`{"acceptance_tests":[]}`),
	}
	verifier := LLMPatchGroundingVerifier{Model: "verifier-model"}
	descriptor := verifier.Describe(fix, contextSnapshot, plan)
	request := verifier.completionRequest(fix, contextSnapshot, plan)
	if descriptor.PromptVersion != "doctor-patch-grounding-verification-v2" {
		t.Fatalf("prompt version=%q", descriptor.PromptVersion)
	}
	contract := strings.ToLower(request.System + "\n" + request.Prompt)
	for _, required := range []string{"actual repository diff", "selected source identities", "unrelated file", "unrelated replacement", "source-association change"} {
		if !strings.Contains(contract, required) {
			t.Fatalf("grounding request omitted %q: %s", required, contract)
		}
	}
	if !strings.Contains(request.Prompt, `"path":"app/page.tsx"`) || !strings.Contains(request.Prompt, `"base_sha":"blob-1"`) {
		t.Fatalf("grounding request omitted exact source identity or actual diff: %s", request.Prompt)
	}
}

func TestCanonicalApplyFailsClosedAndLedgersUnavailableIndependentVerifier(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	fix := groundedOptimizationFix()
	fix.ID, fix.ProjectID, fix.Status = fixID, projectID, "approved"
	store := &applyStoreStub{fix: fix, generationContext: GenerationContext{
		ProductProfile: json.RawMessage(`{"positioning":"Existing product context"}`),
		ProfileVersion: 7, ObservedEvidence: fix.EvidenceSnapshot,
	}}
	generator := &fixGeneratorStub{store: store, plan: ApplicationPlan{
		TargetURL: "https://example.com/product", NormalizedTargetURL: "https://example.com/product",
		OpportunityKey: "doctor:" + fixID.String(), Status: "manual_apply_required",
		SourceFilePaths: json.RawMessage(`[]`), PatchSnapshot: json.RawMessage(`{"change":"safe"}`),
		DiffSnapshot: json.RawMessage(`{}`), ResolutionCriteria: json.RawMessage(`{"acceptance":"safe"}`),
	}}

	_, err := applyServiceForTest(store, generator, LLMPatchGroundingVerifier{}).Apply(context.Background(), projectID, fixID)
	if err == nil {
		t.Fatal("provider-unavailable independent verifier must fail closed")
	}
	if strings.Contains(strings.Join(store.events, ","), "finalize") {
		t.Fatalf("unverified patch was finalized: %v", store.events)
	}
	if got := store.events[len(store.events)-1]; got != "finish_verifier:skipped" {
		t.Fatalf("last event = %q, want skipped verifier ledger completion because no provider call occurred; events=%v", got, store.events)
	}
}

func TestDeterministicApplyDoesNotCallProviderWithoutAuthority(t *testing.T) {
	fix := db.SiteFix{ID: uuid.New(), ProjectID: uuid.New(), TargetUrls: json.RawMessage(`["https://example.com/"]`), EvidenceSnapshot: json.RawMessage(`{"finding":{"preserved_propositions":[]}}`), ProposedFix: json.RawMessage(`{"mutations":[{"field":"canonical","operation":"replace"}]}`), AcceptanceTests: json.RawMessage(`[{"type":"canonical_present"}]`)}
	generationContext := GenerationContext{ProductProfile: json.RawMessage(`{}`), ObservedEvidence: fix.EvidenceSnapshot}
	plan, result, err := (DeterministicApplicationGenerator{}).Generate(context.Background(), fix, generationContext, RepositorySnapshot{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "skipped" || result.ErrorCode != "doctor_ai_not_authorized" || plan.Status != "manual_apply_required" {
		t.Fatalf("plan=%+v result=%+v", plan, result)
	}
}

func TestDoctorAIApplicationIsGroundedInContextAndObservedEvidence(t *testing.T) {
	provider := &groundingProviderStub{response: `{"files":[{"path":"app/page.tsx","base_sha":"blob-1","replacements":[{"old_text":"Old title","new_text":"New title"}]}]}`}
	fix := groundedOptimizationFix()
	contextSnapshot := GenerationContext{ProductProfile: json.RawMessage(`{"positioning":"Existing product context"}`), ProfileVersion: 7, ObservedEvidence: fix.EvidenceSnapshot}
	plan, _, err := (LLMApplicationGenerator{Provider: provider, Model: "test-model"}).Generate(context.Background(), fix, contextSnapshot, testRepositorySnapshot(), nil)
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

func TestDoctorAIApplicationDerivesGroundingFromApprovedEvidence(t *testing.T) {
	provider := &groundingProviderStub{response: `{"files":[{"path":"app/page.tsx","base_sha":"blob-1","replacements":[{"old_text":"Old title","new_text":"New title"}]}]}`}
	fix := groundedOptimizationFix()
	contextSnapshot := GenerationContext{ProductProfile: json.RawMessage(`{"positioning":"Existing product context"}`), ProfileVersion: 7, ObservedEvidence: fix.EvidenceSnapshot}
	plan, _, err := (LLMApplicationGenerator{Provider: provider, Model: "test-model"}).Generate(context.Background(), fix, contextSnapshot, testRepositorySnapshot(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateApplicationPlan(fix, contextSnapshot, plan); err != nil {
		t.Fatalf("system-derived grounding should validate without a model self-report: %v", err)
	}
	var grounding map[string]any
	if err := json.Unmarshal(plan.GroundingSnapshot, &grounding); err != nil {
		t.Fatal(err)
	}
	if grounding["context_profile_version"] != float64(7) || grounding["primary_intent_before"] != "describe product" {
		t.Fatalf("grounding = %#v", grounding)
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
	verifierSawLifecycleTransaction bool
	existing                        db.SiteChangeApplication
	generationContext               GenerationContext
	selectionCallID                 uuid.UUID
	generationCausedBy              uuid.UUID
	preparationFailure              string
}

func (s *applyStoreStub) Load(_ context.Context, projectID, fixID uuid.UUID) (db.SiteFix, error) {
	s.events = append(s.events, "load")
	if s.fix.ProjectID != projectID || s.fix.ID != fixID {
		return db.SiteFix{}, ErrProjectMismatch
	}
	return s.fix, nil
}
func (s *applyStoreStub) LoadGenerationContext(_ context.Context, fix db.SiteFix) (GenerationContext, error) {
	if len(s.generationContext.ObservedEvidence) > 0 {
		return s.generationContext, nil
	}
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
func (s *applyStoreStub) StartSourceSelection(_ context.Context, _ db.SiteFix, _ GenerationCall) (uuid.UUID, siteFixAICallAttempt, error) {
	s.events = append(s.events, "start_selector")
	s.selectionCallID = uuid.New()
	return s.selectionCallID, &siteFixAttemptSpy{}, nil
}
func (s *applyStoreStub) FinishSourceSelection(_ context.Context, _ db.SiteFix, _ uuid.UUID, result GenerationResult) error {
	s.events = append(s.events, "finish_selector:"+result.Status)
	return nil
}
func (s *applyStoreStub) StartGeneration(_ context.Context, _ db.SiteFix, _ GenerationCall, causedBy uuid.UUID) (uuid.UUID, siteFixAICallAttempt, error) {
	s.events = append(s.events, "start_ai")
	s.startedCalls++
	s.generationCausedBy = causedBy
	return uuid.New(), &siteFixAttemptSpy{}, nil
}
func (s *applyStoreStub) FinishGeneration(_ context.Context, _ db.SiteFix, _ uuid.UUID, result GenerationResult) error {
	s.events = append(s.events, "finish_ai:"+result.Status)
	s.finishedCalls++
	return nil
}
func (s *applyStoreStub) StartGroundingVerification(_ context.Context, _ db.SiteFix, _ GenerationCall, _ uuid.UUID) (uuid.UUID, siteFixAICallAttempt, error) {
	s.events = append(s.events, "start_verifier")
	return uuid.New(), &siteFixAttemptSpy{}, nil
}
func (s *applyStoreStub) FinishGroundingVerification(_ context.Context, _ db.SiteFix, _ uuid.UUID, result GenerationResult) error {
	s.events = append(s.events, "finish_verifier:"+result.Status)
	return nil
}
func (s *applyStoreStub) RecordPreparationFailure(_ context.Context, _ db.SiteFix, code string) error {
	s.preparationFailure = code
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
	store       *applyStoreStub
	plan        ApplicationPlan
	err         error
	skipAttempt bool
}

func (g *fixGeneratorStub) Describe(db.SiteFix, GenerationContext, RepositorySnapshot) GenerationCall {
	return GenerationCall{Provider: "test", Model: "model", PromptVersion: "doctor-fix-v1", RequestFingerprint: "fingerprint"}
}
func (g *fixGeneratorStub) Generate(ctx context.Context, fix db.SiteFix, generationContext GenerationContext, _ RepositorySnapshot, attempt siteFixAICallAttempt) (ApplicationPlan, GenerationResult, error) {
	if !g.skipAttempt {
		_, _ = attempt.StartAttempt(ctx, "generator-test")
	}
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

type patchVerifierStub struct {
	store        *applyStoreStub
	verification PatchVerification
	err          error
}

func (v *patchVerifierStub) Describe(db.SiteFix, GenerationContext, ApplicationPlan) GenerationCall {
	return GenerationCall{Provider: "test", Model: "verifier", PromptVersion: "doctor-patch-grounding-v1", RequestFingerprint: "verifier-fingerprint"}
}

func (v *patchVerifierStub) Verify(ctx context.Context, _ db.SiteFix, _ GenerationContext, _ ApplicationPlan, attempt siteFixAICallAttempt) (PatchVerification, GenerationResult, error) {
	_, _ = attempt.StartAttempt(ctx, "verifier-test")
	v.store.events = append(v.store.events, "verifier")
	v.store.verifierSawLifecycleTransaction = v.store.lifecycleTransactionOpen
	if v.err != nil {
		return PatchVerification{}, GenerationResult{Status: "failed", ErrorCode: "provider_unavailable"}, v.err
	}
	return v.verification, GenerationResult{Provider: "test", Model: "verifier", Status: "ok"}, nil
}

type siteFixAttemptSpy struct{ started bool }

func (s *siteFixAttemptSpy) StartAttempt(context.Context, string) (string, error) {
	s.started = true
	return "site-fix-attempt", nil
}
func (*siteFixAttemptSpy) FinishAttempt(context.Context, string, llm.CompletionResp, error) error {
	return nil
}
func (s *siteFixAttemptSpy) Started() bool { return s.started }

func applyServiceForTest(store *applyStoreStub, generator FixGenerator, verifier PatchGroundingVerifier) ApplyService {
	repository := testRepositorySnapshot()
	loader := &repositoryLoaderStub{
		target:     RepositoryTarget{Repo: repository.Repo, Branch: repository.Branch, BaseCommitSHA: repository.BaseCommitSHA},
		candidates: []RepositorySourceCandidate{{Path: repository.Sources[0].Path, SHA: repository.Sources[0].SHA, Size: int64(len(repository.Sources[0].Content))}},
		snapshot:   repository,
	}
	return ApplyService{
		Store: store, SourceLoader: loader,
		SourceSelector: &repositorySelectorStub{store: store, paths: []string{repository.Sources[0].Path}},
		Generator:      generator, Verifier: verifier,
	}
}

func testRepositorySnapshot() RepositorySnapshot {
	return RepositorySnapshot{
		Repo: "acme/site", Branch: "main", BaseCommitSHA: "commit-1",
		Sources: []RepositorySource{{Path: "app/page.tsx", SHA: "blob-1", Content: "export const title = 'Old title'\n"}},
	}
}
