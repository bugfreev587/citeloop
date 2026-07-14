package sitefix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		OpportunityKey: "doctor:" + fixID.String(), Status: "ready_for_pr",
		SourceFilePaths:    json.RawMessage(`[]`),
		PatchSnapshot:      json.RawMessage(`{"change":"canonical"}`),
		DiffSnapshot:       json.RawMessage(`{}`),
		ResolutionCriteria: json.RawMessage(`{"asset_type":"canonical"}`),
	}}
	verifier := &patchVerifierStub{store: store, verification: completeApprovedPatchVerification()}
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

func TestCanonicalApplyFeedsRejectionBackForCorrectableGenerationFailures(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	store := &applyStoreStub{fix: db.SiteFix{ID: fixID, ProjectID: projectID, Status: "approved", EvidenceSnapshot: json.RawMessage(`{"finding":{"preserved_propositions":[]}}`)}}
	generator := &fixGeneratorStub{store: store, failuresBeforeSuccess: 1, failureCode: "invalid_repository_patch", plan: ApplicationPlan{
		TargetURL: "https://example.com/", NormalizedTargetURL: "https://example.com/",
		OpportunityKey: "doctor:" + fixID.String(), Status: "ready_for_pr",
		SourceFilePaths:    json.RawMessage(`["app/sitemap.ts"]`),
		PatchSnapshot:      json.RawMessage(`{"change":"canonical"}`),
		DiffSnapshot:       json.RawMessage(`{}`),
		ResolutionCriteria: json.RawMessage(`{"asset_type":"canonical"}`),
	}}
	verifier := &patchVerifierStub{store: store, verification: completeApprovedPatchVerification()}
	result, err := applyServiceForTest(store, generator, verifier).Apply(context.Background(), projectID, fixID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Application.Status != "ready_for_pr" {
		t.Fatalf("application status = %q", result.Application.Status)
	}
	if store.startedCalls != 2 || store.finishedCalls != 2 {
		t.Fatalf("generation records started=%d finished=%d, want 2/2", store.startedCalls, store.finishedCalls)
	}
	if len(generator.feedback) != 2 || !reflect.DeepEqual(generator.feedback[0], GenerationFeedback{}) ||
		generator.feedback[1].Kind != generationFeedbackRepositoryPatch ||
		generator.feedback[1].Code != "invalid_repository_patch" ||
		!strings.Contains(generator.feedback[1].Explanation, "must occur exactly once") {
		t.Fatalf("feedback = %#v", generator.feedback)
	}
	// The failed round stays on the audit trail and the correction round is
	// chained to it, not to the source selection call.
	if store.generationCausedBys[0] != store.selectionCallID || store.generationCausedBys[1] != store.generationCallIDs[0] {
		t.Fatalf("causedBy chain = %v, callIDs = %v, selection = %v", store.generationCausedBys, store.generationCallIDs, store.selectionCallID)
	}
	wantTail := []string{"start_ai", "provider", "finish_ai:failed", "start_ai", "provider", "finish_ai:ok"}
	tail := store.events[len(store.events)-len(wantTail)-4 : len(store.events)-4]
	if !reflect.DeepEqual(tail, wantTail) {
		t.Fatalf("events = %#v", store.events)
	}
}

func TestCanonicalApplyCorrectsCompleteGroundingRejection(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	store := &applyStoreStub{fix: db.SiteFix{ID: fixID, ProjectID: projectID, Status: "approved", EvidenceSnapshot: json.RawMessage(`{"finding":{"preserved_propositions":[]}}`)}}
	generator := &fixGeneratorStub{store: store, plan: applicationPlanForApplyTest(fixID)}
	rejected := newPatchGroundingRejectionError(PatchVerification{
		PrimaryIntentPreserved: true,
		PreservedPropositions:  []string{},
		AddedPropositions:      []string{},
		RemovedPropositions:    []string{},
		UnsupportedClaims:      []string{"Unsupported promise."},
		Reason:                 "The patch introduces an unsupported promise.",
	})
	approved := completeApprovedPatchVerification()
	verifier := &patchVerifierStub{
		store:     store,
		decisions: []PatchVerification{rejected.Decision, approved},
		results: []GenerationResult{
			{Provider: "test", Model: "verifier", Status: "failed", ErrorCode: "grounding_rejected"},
			{Provider: "test", Model: "verifier", Status: "ok"},
		},
		errors: []error{rejected, nil},
	}

	result, err := applyServiceForTest(store, generator, verifier).Apply(context.Background(), projectID, fixID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Application.Status != "ready_for_pr" {
		t.Fatalf("application status = %q", result.Application.Status)
	}
	if generator.calls != 2 || verifier.calls != 2 {
		t.Fatalf("generator/verifier calls = %d/%d, want 2/2", generator.calls, verifier.calls)
	}
	if len(generator.feedback) != 2 || !reflect.DeepEqual(generator.feedback[0], GenerationFeedback{}) {
		t.Fatalf("feedback = %#v", generator.feedback)
	}
	wantFeedback := newGroundingGenerationFeedback(rejected.Decision)
	if !reflect.DeepEqual(generator.feedback[1], wantFeedback) {
		t.Fatalf("grounding feedback = %#v, want %#v", generator.feedback[1], wantFeedback)
	}
	if store.finalizeCount != 1 {
		t.Fatalf("finalize count = %d, want 1", store.finalizeCount)
	}
	assertGroundingRejectionResults(t, store.verificationResults, 1)
}

func TestCanonicalApplyChainsGroundingCorrectionThroughVerifierCall(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	store := &applyStoreStub{fix: db.SiteFix{ID: fixID, ProjectID: projectID, Status: "approved", EvidenceSnapshot: json.RawMessage(`{"finding":{"preserved_propositions":[]}}`)}}
	generator := &fixGeneratorStub{store: store, plan: applicationPlanForApplyTest(fixID)}
	rejected := newPatchGroundingRejectionError(PatchVerification{
		PrimaryIntentPreserved: true, PreservedPropositions: []string{}, IntentDrift: true,
		AddedPropositions: []string{}, RemovedPropositions: []string{}, UnsupportedClaims: []string{},
		Reason: "The patch changes the approved intent.",
	})
	verifier := &patchVerifierStub{
		store:     store,
		decisions: []PatchVerification{rejected.Decision, completeApprovedPatchVerification()},
		results: []GenerationResult{
			{Provider: "test", Model: "verifier", Status: "failed", ErrorCode: "grounding_rejected"},
			{Provider: "test", Model: "verifier", Status: "ok"},
		},
		errors: []error{rejected, nil},
	}

	if _, err := applyServiceForTest(store, generator, verifier).Apply(context.Background(), projectID, fixID); err != nil {
		t.Fatal(err)
	}
	if len(store.generationCallIDs) != 2 || len(store.verifierCallIDs) != 2 ||
		len(store.generationCausedBys) != 2 || len(store.verifierCausedBys) != 2 {
		t.Fatalf("generation IDs/causes=%v/%v verifier IDs/causes=%v/%v",
			store.generationCallIDs, store.generationCausedBys, store.verifierCallIDs, store.verifierCausedBys)
	}
	if store.generationCausedBys[0] != store.selectionCallID ||
		store.verifierCausedBys[0] != store.generationCallIDs[0] ||
		store.generationCausedBys[1] != store.verifierCallIDs[0] ||
		store.verifierCausedBys[1] != store.generationCallIDs[1] {
		t.Fatalf("causal chain selection=%s generations=%v generation causes=%v verifiers=%v verifier causes=%v",
			store.selectionCallID, store.generationCallIDs, store.generationCausedBys, store.verifierCallIDs, store.verifierCausedBys)
	}
	wantTail := []string{
		"start_ai", "provider", "finish_ai:ok", "start_verifier", "verifier", "finish_verifier:failed",
		"start_ai", "provider", "finish_ai:ok", "start_verifier", "verifier", "finish_verifier:ok", "finalize",
	}
	if tail := store.events[len(store.events)-len(wantTail):]; !reflect.DeepEqual(tail, wantTail) {
		t.Fatalf("event tail = %#v, want %#v", tail, wantTail)
	}
	assertGroundingRejectionResults(t, store.verificationResults, 1)
}

func TestCanonicalApplyExhaustsSharedBudgetAfterGroundingRejections(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	store := &applyStoreStub{fix: db.SiteFix{ID: fixID, ProjectID: projectID, Status: "approved", EvidenceSnapshot: json.RawMessage(`{"finding":{"preserved_propositions":[]}}`)}}
	generator := &fixGeneratorStub{store: store, plan: applicationPlanForApplyTest(fixID)}
	decisions := make([]PatchVerification, 0, 1+maxGenerationCorrectionRounds)
	results := make([]GenerationResult, 0, 1+maxGenerationCorrectionRounds)
	verifierErrors := make([]error, 0, 1+maxGenerationCorrectionRounds)
	for round := 0; round < 1+maxGenerationCorrectionRounds; round++ {
		rejected := newPatchGroundingRejectionError(PatchVerification{
			PrimaryIntentPreserved: true, PreservedPropositions: []string{},
			AddedPropositions: []string{}, RemovedPropositions: []string{},
			UnsupportedClaims: []string{fmt.Sprintf("Unsupported claim %d.", round)},
			Reason:            fmt.Sprintf("Grounding rejection %d.", round),
		})
		decisions = append(decisions, rejected.Decision)
		results = append(results, GenerationResult{Provider: "test", Model: "verifier", Status: "failed", ErrorCode: "grounding_rejected"})
		verifierErrors = append(verifierErrors, fmt.Errorf("wrapped grounding rejection %d: %w", round, rejected))
	}
	verifier := &patchVerifierStub{store: store, decisions: decisions, results: results, errors: verifierErrors}

	_, err := applyServiceForTest(store, generator, verifier).Apply(context.Background(), projectID, fixID)
	if err == nil || !errors.Is(err, ErrPatchGroundingRejected) {
		t.Fatalf("Apply error = %v, want grounding rejection", err)
	}
	if generator.calls != 1+maxGenerationCorrectionRounds || verifier.calls != 1+maxGenerationCorrectionRounds {
		t.Fatalf("generator/verifier calls = %d/%d, want %d/%d", generator.calls, verifier.calls,
			1+maxGenerationCorrectionRounds, 1+maxGenerationCorrectionRounds)
	}
	if store.finalizeCount != 0 {
		t.Fatalf("finalize count = %d, want 0", store.finalizeCount)
	}
	if store.preparationFailure != "grounding_rejected" {
		t.Fatalf("preparation failure = %q, want grounding_rejected", store.preparationFailure)
	}
	var exhaustedRejection *PatchGroundingRejectionError
	if !errors.As(err, &exhaustedRejection) || exhaustedRejection == nil || !completePatchVerificationDecision(exhaustedRejection.Decision) {
		t.Fatalf("Apply error = %v, want complete typed grounding rejection", err)
	}
	assertGroundingRejectionResults(t, store.verificationResults, 1+maxGenerationCorrectionRounds)
}

func TestCanonicalApplyDoesNotCorrectUntrustedVerifierErrors(t *testing.T) {
	providerErr := errors.New("verifier provider unavailable")
	incompleteErr := errors.New("independent patch grounding verifier returned an incomplete decision")
	incompleteTypedErr := &PatchGroundingRejectionError{}
	tests := []struct {
		name            string
		result          GenerationResult
		err             error
		wantIs          error
		wantNotIs       []error
		wantContains    string
		wantPreparation string
	}{
		{
			name: "provider error", result: GenerationResult{Provider: "test", Model: "verifier", Status: "failed", ErrorCode: "provider_error"},
			err: providerErr, wantIs: providerErr, wantNotIs: []error{ErrPatchGroundingRejected}, wantPreparation: "provider_unavailable",
		},
		{
			name: "incomplete decision error", result: GenerationResult{Provider: "test", Model: "verifier", Status: "failed", ErrorCode: "invalid_response"},
			err: incompleteErr, wantIs: errInvalidModelResponse, wantNotIs: []error{incompleteErr, ErrPatchGroundingRejected}, wantPreparation: "invalid_response",
		},
		{
			name: "typed rejection without complete decision", result: GenerationResult{Provider: "test", Model: "verifier", Status: "failed", ErrorCode: "grounding_rejected"},
			err: incompleteTypedErr, wantIs: errInvalidModelResponse, wantNotIs: []error{incompleteTypedErr, ErrPatchGroundingRejected}, wantPreparation: "invalid_response",
		},
		{
			name: "grounding sentinel without typed decision", result: GenerationResult{Provider: "test", Model: "verifier", Status: "failed", ErrorCode: "grounding_rejected"},
			err: ErrPatchGroundingRejected, wantNotIs: []error{ErrPatchGroundingRejected},
			wantContains: "missing its typed decision", wantPreparation: "preparation_failed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectID, fixID := uuid.New(), uuid.New()
			store := &applyStoreStub{fix: db.SiteFix{ID: fixID, ProjectID: projectID, Status: "approved", EvidenceSnapshot: json.RawMessage(`{"finding":{"preserved_propositions":[]}}`)}}
			generator := &fixGeneratorStub{store: store, plan: applicationPlanForApplyTest(fixID)}
			verifier := &patchVerifierStub{
				store: store, decisions: []PatchVerification{{}}, results: []GenerationResult{test.result}, errors: []error{test.err},
			}

			_, applyErr := applyServiceForTest(store, generator, verifier).Apply(context.Background(), projectID, fixID)
			if applyErr == nil {
				t.Fatal("expected terminal verifier error")
			}
			if test.wantIs != nil && !errors.Is(applyErr, test.wantIs) {
				t.Fatalf("Apply error = %v, want errors.Is(..., %v)", applyErr, test.wantIs)
			}
			for _, unwanted := range test.wantNotIs {
				if errors.Is(applyErr, unwanted) {
					t.Fatalf("Apply error = %v unexpectedly retained %v", applyErr, unwanted)
				}
			}
			if test.wantContains != "" && !strings.Contains(applyErr.Error(), test.wantContains) {
				t.Fatalf("Apply error = %v, want text %q", applyErr, test.wantContains)
			}
			if generator.calls != 1 || verifier.calls != 1 || len(generator.feedback) != 1 {
				t.Fatalf("generator/verifier/feedback calls = %d/%d/%d, want 1/1/1", generator.calls, verifier.calls, len(generator.feedback))
			}
			if store.finalizeCount != 0 {
				t.Fatalf("finalize count = %d, want 0", store.finalizeCount)
			}
			if store.preparationFailure != test.wantPreparation {
				t.Fatalf("preparation failure = %q, want %q", store.preparationFailure, test.wantPreparation)
			}
		})
	}
}

func TestCanonicalApplyRejectsIncompleteRawVerifierDecisions(t *testing.T) {
	tests := []struct {
		name     string
		decision PatchVerification
	}{
		{
			name: "approved without required lists or reason",
			decision: PatchVerification{
				Approved: true, PrimaryIntentPreserved: true, PreservedPropositions: []string{},
			},
		},
		{
			name:     "rejected with reason only",
			decision: PatchVerification{Reason: "The verifier rejected the patch."},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectID, fixID := uuid.New(), uuid.New()
			store := &applyStoreStub{fix: db.SiteFix{ID: fixID, ProjectID: projectID, Status: "approved", EvidenceSnapshot: json.RawMessage(`{"finding":{"preserved_propositions":[]}}`)}}
			generator := &fixGeneratorStub{store: store, plan: applicationPlanForApplyTest(fixID)}
			verifier := &patchVerifierStub{
				store:     store,
				decisions: []PatchVerification{test.decision},
				results:   []GenerationResult{{Provider: "test", Model: "verifier", Status: "ok"}},
				errors:    []error{nil},
			}

			_, err := applyServiceForTest(store, generator, verifier).Apply(context.Background(), projectID, fixID)
			if err == nil || !errors.Is(err, errInvalidModelResponse) || errors.Is(err, ErrPatchGroundingRejected) ||
				!strings.Contains(err.Error(), "incomplete decision") {
				t.Fatalf("Apply error = %v, want terminal invalid-response error", err)
			}
			if generator.calls != 1 || verifier.calls != 1 || store.finalizeCount != 0 {
				t.Fatalf("generator/verifier/finalize calls = %d/%d/%d, want 1/1/0", generator.calls, verifier.calls, store.finalizeCount)
			}
			if len(store.verificationResults) != 1 || store.verificationResults[0].Status != "failed" ||
				store.verificationResults[0].ErrorCode != "invalid_response" {
				t.Fatalf("persisted verifier results = %+v, want one failed/invalid_response", store.verificationResults)
			}
			if store.preparationFailure != "invalid_response" {
				t.Fatalf("preparation failure = %q, want invalid_response", store.preparationFailure)
			}
		})
	}
}

func TestCanonicalApplyRejectsInconsistentGroundingRejectionLedgerResults(t *testing.T) {
	tests := []struct {
		name   string
		result GenerationResult
	}{
		{
			name:   "successful status",
			result: GenerationResult{Provider: "test", Model: "verifier", Status: "ok"},
		},
		{
			name:   "wrong error code",
			result: GenerationResult{Provider: "test", Model: "verifier", Status: "failed", ErrorCode: "provider_error"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectID, fixID := uuid.New(), uuid.New()
			store := &applyStoreStub{fix: db.SiteFix{ID: fixID, ProjectID: projectID, Status: "approved", EvidenceSnapshot: json.RawMessage(`{"finding":{"preserved_propositions":[]}}`)}}
			generator := &fixGeneratorStub{store: store, plan: applicationPlanForApplyTest(fixID)}
			rejected := newPatchGroundingRejectionError(PatchVerification{
				PrimaryIntentPreserved: true, PreservedPropositions: []string{}, IntentDrift: true,
				AddedPropositions: []string{}, RemovedPropositions: []string{}, UnsupportedClaims: []string{},
				Reason: "The patch changes the approved intent.",
			})
			verifier := &patchVerifierStub{
				store: store, decisions: []PatchVerification{rejected.Decision},
				results: []GenerationResult{test.result}, errors: []error{rejected},
			}

			_, err := applyServiceForTest(store, generator, verifier).Apply(context.Background(), projectID, fixID)
			if err == nil || errors.Is(err, ErrPatchGroundingRejected) || !strings.Contains(err.Error(), "ledger result is inconsistent") {
				t.Fatalf("Apply error = %v, want terminal grounding ledger invariant error", err)
			}
			if generator.calls != 1 || verifier.calls != 1 || store.finalizeCount != 0 {
				t.Fatalf("generator/verifier/finalize calls = %d/%d/%d, want 1/1/0", generator.calls, verifier.calls, store.finalizeCount)
			}
			if store.preparationFailure != "preparation_failed" {
				t.Fatalf("preparation failure = %q, want preparation_failed", store.preparationFailure)
			}
			assertGroundingRejectionResults(t, store.verificationResults, 1)
		})
	}
}

func TestCanonicalApplyRejectsTypedNilGroundingRejectionWithoutPanic(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	store := &applyStoreStub{fix: db.SiteFix{ID: fixID, ProjectID: projectID, Status: "approved", EvidenceSnapshot: json.RawMessage(`{"finding":{"preserved_propositions":[]}}`)}}
	generator := &fixGeneratorStub{store: store, plan: applicationPlanForApplyTest(fixID)}
	var typedNil *PatchGroundingRejectionError
	verifier := &patchVerifierStub{
		store: store, decisions: []PatchVerification{{}},
		results: []GenerationResult{{Provider: "test", Model: "verifier", Status: "failed", ErrorCode: "grounding_rejected"}},
		errors:  []error{typedNil},
	}

	var applyErr error
	var panicValue any
	func() {
		defer func() { panicValue = recover() }()
		_, applyErr = applyServiceForTest(store, generator, verifier).Apply(context.Background(), projectID, fixID)
	}()
	if panicValue != nil {
		t.Fatalf("Apply panicked for typed-nil grounding rejection: %v", panicValue)
	}
	if applyErr == nil || !strings.Contains(applyErr.Error(), "nil grounding rejection") {
		t.Fatalf("Apply error = %v, want terminal nil-rejection invariant error", applyErr)
	}
	if errors.Is(applyErr, ErrPatchGroundingRejected) {
		t.Fatalf("Apply error retained trusted grounding sentinel: %v", applyErr)
	}
	if generator.calls != 1 || verifier.calls != 1 || store.finalizeCount != 0 {
		t.Fatalf("generator/verifier/finalize calls = %d/%d/%d, want 1/1/0", generator.calls, verifier.calls, store.finalizeCount)
	}
	if len(store.verificationResults) != 1 || store.verificationResults[0].Status != "failed" ||
		store.verificationResults[0].ErrorCode != "invalid_response" {
		t.Fatalf("persisted verifier results = %+v, want one failed/invalid_response", store.verificationResults)
	}
	if store.preparationFailure != "preparation_failed" {
		t.Fatalf("preparation failure = %q, want preparation_failed", store.preparationFailure)
	}
}

func TestCanonicalApplyCorrectsWrappedCompleteGroundingRejection(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	store := &applyStoreStub{fix: db.SiteFix{ID: fixID, ProjectID: projectID, Status: "approved", EvidenceSnapshot: json.RawMessage(`{"finding":{"preserved_propositions":[]}}`)}}
	generator := &fixGeneratorStub{store: store, plan: applicationPlanForApplyTest(fixID)}
	rejected := newPatchGroundingRejectionError(PatchVerification{
		PrimaryIntentPreserved: true, PreservedPropositions: []string{}, IntentDrift: true,
		AddedPropositions: []string{}, RemovedPropositions: []string{}, UnsupportedClaims: []string{},
		Reason: "The patch changes the approved intent.",
	})
	verifier := &patchVerifierStub{
		store:     store,
		decisions: []PatchVerification{rejected.Decision, completeApprovedPatchVerification()},
		results: []GenerationResult{
			{Provider: "test", Model: "verifier", Status: "failed", ErrorCode: "grounding_rejected"},
			{Provider: "test", Model: "verifier", Status: "ok"},
		},
		errors: []error{fmt.Errorf("wrapped verifier rejection: %w", rejected), nil},
	}

	if _, err := applyServiceForTest(store, generator, verifier).Apply(context.Background(), projectID, fixID); err != nil {
		t.Fatal(err)
	}
	if generator.calls != 2 || verifier.calls != 2 || store.finalizeCount != 1 {
		t.Fatalf("generator/verifier/finalize calls = %d/%d/%d, want 2/2/1", generator.calls, verifier.calls, store.finalizeCount)
	}
	if generator.feedback[1].Kind != generationFeedbackGrounding {
		t.Fatalf("wrapped rejection feedback = %#v, want grounding feedback", generator.feedback)
	}
	assertGroundingRejectionResults(t, store.verificationResults, 1)
}

func TestCanonicalApplySharesBudgetBetweenRepositoryAndGroundingCorrections(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	store := &applyStoreStub{fix: db.SiteFix{ID: fixID, ProjectID: projectID, Status: "approved", EvidenceSnapshot: json.RawMessage(`{"finding":{"preserved_propositions":[]}}`)}}
	generator := &fixGeneratorStub{
		store: store, failuresBeforeSuccess: 1, failureCode: "invalid_repository_patch", plan: applicationPlanForApplyTest(fixID),
	}
	decisions := make([]PatchVerification, 0, maxGenerationCorrectionRounds)
	results := make([]GenerationResult, 0, maxGenerationCorrectionRounds)
	verifierErrors := make([]error, 0, maxGenerationCorrectionRounds)
	for round := 0; round < maxGenerationCorrectionRounds; round++ {
		rejected := newPatchGroundingRejectionError(PatchVerification{
			PrimaryIntentPreserved: true, PreservedPropositions: []string{}, IntentDrift: true,
			AddedPropositions: []string{}, RemovedPropositions: []string{}, UnsupportedClaims: []string{},
			Reason: fmt.Sprintf("Grounding rejection %d.", round),
		})
		decisions = append(decisions, rejected.Decision)
		results = append(results, GenerationResult{Provider: "test", Model: "verifier", Status: "failed", ErrorCode: "grounding_rejected"})
		verifierErrors = append(verifierErrors, rejected)
	}
	verifier := &patchVerifierStub{store: store, decisions: decisions, results: results, errors: verifierErrors}

	_, err := applyServiceForTest(store, generator, verifier).Apply(context.Background(), projectID, fixID)
	if err == nil || !errors.Is(err, ErrPatchGroundingRejected) {
		t.Fatalf("Apply error = %v, want grounding rejection", err)
	}
	if generator.calls != 1+maxGenerationCorrectionRounds || verifier.calls != maxGenerationCorrectionRounds {
		t.Fatalf("generator/verifier calls = %d/%d, want %d/%d", generator.calls, verifier.calls,
			1+maxGenerationCorrectionRounds, maxGenerationCorrectionRounds)
	}
	if store.finalizeCount != 0 {
		t.Fatalf("finalize count = %d, want 0", store.finalizeCount)
	}
	if generator.feedback[1].Kind != generationFeedbackRepositoryPatch ||
		generator.feedback[2].Kind != generationFeedbackGrounding {
		t.Fatalf("feedback sequence = %#v", generator.feedback)
	}
	assertGroundingRejectionResults(t, store.verificationResults, maxGenerationCorrectionRounds)
}

func TestCanonicalApplyBoundsGenerationCorrectionRounds(t *testing.T) {
	projectID, fixID := uuid.New(), uuid.New()
	store := &applyStoreStub{fix: db.SiteFix{ID: fixID, ProjectID: projectID, Status: "approved", EvidenceSnapshot: json.RawMessage(`{"finding":{"preserved_propositions":[]}}`)}}
	generator := &fixGeneratorStub{store: store, failuresBeforeSuccess: 99, failureCode: "invalid_repository_patch"}
	verifier := &patchVerifierStub{store: store}
	_, err := applyServiceForTest(store, generator, verifier).Apply(context.Background(), projectID, fixID)
	if err == nil {
		t.Fatal("expected exhausted correction rounds to fail")
	}
	if store.startedCalls != 1+maxGenerationCorrectionRounds || store.finishedCalls != store.startedCalls {
		t.Fatalf("generation records started=%d finished=%d, want %d", store.startedCalls, store.finishedCalls, 1+maxGenerationCorrectionRounds)
	}
	if store.preparationFailure != "invalid_repository_patch" {
		t.Fatalf("preparation failure = %q", store.preparationFailure)
	}
	if verifier.calls != 0 || store.finalizeCount != 0 {
		t.Fatalf("verifier/finalize calls = %d/%d, want 0/0", verifier.calls, store.finalizeCount)
	}

	// Non-correctable failures never get a correction round.
	store2 := &applyStoreStub{fix: db.SiteFix{ID: fixID, ProjectID: projectID, Status: "approved", EvidenceSnapshot: json.RawMessage(`{"finding":{"preserved_propositions":[]}}`)}}
	generator2 := &fixGeneratorStub{store: store2, err: errors.New("tokengate api key not set")}
	if _, err := applyServiceForTest(store2, generator2, &patchVerifierStub{store: store2}).Apply(context.Background(), projectID, fixID); err == nil {
		t.Fatal("expected provider error")
	}
	if store2.startedCalls != 1 {
		t.Fatalf("non-correctable failure ran %d generation rounds, want 1", store2.startedCalls)
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

func TestCanonicalApplyRejectsEveryNonPRApplicationStatus(t *testing.T) {
	fix := groundedOptimizationFix()
	generationContext := GenerationContext{ProductProfile: json.RawMessage(`{"positioning":"Existing product context"}`), ProfileVersion: 7, ObservedEvidence: fix.EvidenceSnapshot}
	grounding, err := approvedGroundingSnapshot(fix, generationContext)
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"manual_apply_required", "source_mapping_required", "draft_ready"} {
		plan := ApplicationPlan{
			TargetURL: "https://example.com/product", NormalizedTargetURL: "https://example.com/product",
			OpportunityKey: "doctor:" + fix.ID.String(), Status: status,
			SourceFilePaths: json.RawMessage(`["app/page.tsx"]`), PatchSnapshot: json.RawMessage(`{"files":[]}`),
			DiffSnapshot: json.RawMessage(`{"files":[]}`), ResolutionCriteria: json.RawMessage(`{"acceptance_tests":[]}`),
			GroundingSnapshot: grounding,
		}
		if err := validateApplicationPlan(fix, generationContext, plan); err == nil {
			t.Fatalf("non-PR status %q was accepted", status)
		}
	}
}

func TestCanonicalApplyIgnoresTerminalFailedApplicationForReprepare(t *testing.T) {
	projectID, fixID, failedAppID := uuid.New(), uuid.New(), uuid.New()
	fix := db.SiteFix{ID: fixID, ProjectID: projectID, Status: "preparing", TargetUrls: json.RawMessage(`["https://example.com/"]`), EvidenceSnapshot: json.RawMessage(`{"finding":{"preserved_propositions":[]}}`), AcceptanceTests: json.RawMessage(`[]`)}
	store := &applyStoreStub{
		fix:      fix,
		existing: db.SiteChangeApplication{ID: failedAppID, ProjectID: projectID, SiteFixID: validPGUUID(fixID), Status: "failed"},
	}
	generator := &fixGeneratorStub{store: store, plan: ApplicationPlan{
		TargetURL: "https://example.com/", NormalizedTargetURL: "https://example.com/", OpportunityKey: "doctor:" + fixID.String(), Status: "ready_for_pr",
		SourceFilePaths: json.RawMessage(`["app/page.tsx"]`), PatchSnapshot: json.RawMessage(`{"change":"safe"}`), DiffSnapshot: json.RawMessage(`{}`), ResolutionCriteria: json.RawMessage(`{"acceptance_tests":[]}`),
	}}
	verifier := &patchVerifierStub{store: store, verification: completeApprovedPatchVerification()}
	result, err := applyServiceForTest(store, generator, verifier).Apply(context.Background(), projectID, fixID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Application.ID == failedAppID || store.startedCalls != 1 {
		t.Fatalf("failed app was reused: result=%+v generation starts=%d", result.Application, store.startedCalls)
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
		TargetURL: "https://example.com/product", NormalizedTargetURL: "https://example.com/product", OpportunityKey: "doctor:" + fix.ID.String(), Status: "ready_for_pr",
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

func TestGroundingVerifierReturnsTypedBoundedRejectionDecision(t *testing.T) {
	fix := groundedOptimizationFix()
	contextSnapshot := GenerationContext{ProductProfile: json.RawMessage(`{"positioning":"Existing product context"}`), ProfileVersion: 7, ObservedEvidence: fix.EvidenceSnapshot}
	longReason := strings.Repeat(" private verifier detail ", maxGenerationFeedbackExplanationRunes)
	added := make([]string, 0, maxGenerationFeedbackItems+2)
	for i := 0; i < maxGenerationFeedbackItems+2; i++ {
		added = append(added, fmt.Sprintf("  Added proposition %d   with spacing  ", i))
	}
	response, err := json.Marshal(map[string]any{
		"approved": false, "primary_intent_preserved": false,
		"preserved_propositions": []string{"Existing supported fact."},
		"added_propositions":     added,
		"removed_propositions":   []string{"  Existing proposition removed.  "},
		"unsupported_claims":     []string{"  Unsupported commercial promise.  "},
		"intent_drift":           true, "reason": longReason,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := &groundingProviderStub{response: string(response)}
	verification, result, err := (LLMPatchGroundingVerifier{Provider: provider, Model: "verifier-model"}).Verify(
		context.Background(), fix, contextSnapshot, ApplicationPlan{}, nil,
	)
	if result.Status != "failed" || result.ErrorCode != "grounding_rejected" {
		t.Fatalf("result = %+v", result)
	}
	if !errors.Is(err, ErrPatchGroundingRejected) {
		t.Fatalf("Verify error = %v, want grounding rejection", err)
	}
	var rejection *PatchGroundingRejectionError
	if !errors.As(err, &rejection) {
		t.Fatalf("Verify error type = %T, want *PatchGroundingRejectionError", err)
	}
	if err.Error() != ErrPatchGroundingRejected.Error() || strings.Contains(err.Error(), "private verifier detail") {
		t.Fatalf("public rejection error exposed private reason: %q", err)
	}
	if !reflect.DeepEqual(verification, rejection.Decision) {
		t.Fatalf("returned verification = %#v, rejection decision = %#v", verification, rejection.Decision)
	}
	if got := len(rejection.Decision.AddedPropositions); got != maxGenerationFeedbackItems {
		t.Fatalf("bounded added propositions = %d, want %d", got, maxGenerationFeedbackItems)
	}
	if len([]rune(rejection.Decision.Reason)) > maxGenerationFeedbackExplanationRunes {
		t.Fatalf("bounded reason has %d runes", len([]rune(rejection.Decision.Reason)))
	}
	if rejection.Decision.AddedPropositions[0] != "Added proposition 0 with spacing" ||
		rejection.Decision.RemovedPropositions[0] != "Existing proposition removed." ||
		rejection.Decision.UnsupportedClaims[0] != "Unsupported commercial promise." {
		t.Fatalf("decision lists were not normalized: %#v", rejection.Decision)
	}
}

func TestGroundingVerifierMalformedOrIncompleteOutputHasNoTrustedDecision(t *testing.T) {
	fix := groundedOptimizationFix()
	contextSnapshot := GenerationContext{ProductProfile: json.RawMessage(`{"positioning":"Existing product context"}`), ProfileVersion: 7, ObservedEvidence: fix.EvidenceSnapshot}
	tests := []struct {
		name     string
		response string
	}{
		{name: "malformed", response: `not json`},
		{name: "incomplete", response: `{"approved":false,"primary_intent_preserved":false,"preserved_propositions":[],"added_propositions":[],"removed_propositions":[],"unsupported_claims":[],"intent_drift":true}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &groundingProviderStub{response: test.response}
			verification, result, err := (LLMPatchGroundingVerifier{Provider: provider, Model: "verifier-model"}).Verify(
				context.Background(), fix, contextSnapshot, ApplicationPlan{}, nil,
			)
			if err == nil || result.Status != "failed" || result.ErrorCode != "invalid_response" {
				t.Fatalf("verification=%#v result=%+v err=%v", verification, result, err)
			}
			if !reflect.DeepEqual(verification, PatchVerification{}) {
				t.Fatalf("untrusted verifier output escaped: %#v", verification)
			}
			var rejection *PatchGroundingRejectionError
			if errors.As(err, &rejection) || errors.Is(err, ErrPatchGroundingRejected) {
				t.Fatalf("invalid response became a trusted grounding decision: %v", err)
			}
		})
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
		OpportunityKey: "doctor:" + fixID.String(), Status: "ready_for_pr",
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

func TestDoctorAIApplicationIsGroundedInContextAndObservedEvidence(t *testing.T) {
	provider := &groundingProviderStub{response: `{"files":[{"path":"app/page.tsx","base_sha":"blob-1","replacements":[{"old_text":"Old title","new_text":"New title"}]}]}`}
	fix := groundedOptimizationFix()
	contextSnapshot := GenerationContext{ProductProfile: json.RawMessage(`{"positioning":"Existing product context"}`), ProfileVersion: 7, ObservedEvidence: fix.EvidenceSnapshot}
	plan, _, err := (LLMApplicationGenerator{Provider: provider, Model: "test-model"}).Generate(context.Background(), fix, contextSnapshot, testRepositorySnapshot(), GenerationFeedback{}, nil)
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
	plan, _, err := (LLMApplicationGenerator{Provider: provider, Model: "test-model"}).Generate(context.Background(), fix, contextSnapshot, testRepositorySnapshot(), GenerationFeedback{}, nil)
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

func TestDecodeJSONObjectRequiresOneObjectWithoutProse(t *testing.T) {
	var output struct {
		OK bool `json:"ok"`
	}
	rejected := []string{
		`before {"ok":true}`, `{"ok":true} after`, `{"ok":true} {"extra":true}`, `[ {"ok":true} ]`,
		"prose before\n```json\n{\"ok\":true}\n```", "```json\n{\"ok\":true}\n```\nprose after",
		"```json\n{\"ok\":true}\n```\n```json\n{\"extra\":true}\n```", "```json\n[ {\"ok\":true} ]\n```",
	}
	for _, raw := range rejected {
		err := decodeJSONObject(raw, &output)
		if err == nil {
			t.Fatalf("non-object-only provider response was accepted: %q", raw)
		}
		if !errors.Is(err, errInvalidModelResponse) {
			t.Fatalf("decode failure lost its sentinel: %v", err)
		}
	}
	accepted := []string{
		" \n\t{\"ok\":true}\r\n",
		"```json\n{\"ok\":true}\n```",
		"```\n{\"ok\":true}\n```",
		"  ```json\n{\"ok\":true}\n```  ",
	}
	for _, raw := range accepted {
		output.OK = false
		if err := decodeJSONObject(raw, &output); err != nil || !output.OK {
			t.Fatalf("single JSON object was rejected: raw=%q output=%+v err=%v", raw, output, err)
		}
	}
}

func TestSafePreparationFailureCodeSeparatesModelResponseFromProviderOutage(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{errInvalidModelResponse, "invalid_response"},
		{fmt.Errorf("generation failed: %w", errInvalidModelResponse), "invalid_response"},
		{errors.New("Doctor fix generation provider is unavailable"), "provider_unavailable"},
		{errors.New("provider fallback disabled for site_fix"), "provider_unavailable"},
	}
	for _, test := range tests {
		if got := safePreparationFailureCode(test.err); got != test.want {
			t.Fatalf("safePreparationFailureCode(%v) = %q, want %q", test.err, got, test.want)
		}
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
	generationCausedBys             []uuid.UUID
	generationCallIDs               []uuid.UUID
	verifierCausedBys               []uuid.UUID
	verifierCallIDs                 []uuid.UUID
	verificationResults             []GenerationResult
	finalizeCount                   int
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
	s.generationCausedBys = append(s.generationCausedBys, causedBy)
	callID := uuid.New()
	s.generationCallIDs = append(s.generationCallIDs, callID)
	return callID, &siteFixAttemptSpy{}, nil
}
func (s *applyStoreStub) FinishGeneration(_ context.Context, _ db.SiteFix, _ uuid.UUID, result GenerationResult) error {
	s.events = append(s.events, "finish_ai:"+result.Status)
	s.finishedCalls++
	return nil
}
func (s *applyStoreStub) StartGroundingVerification(_ context.Context, _ db.SiteFix, _ GenerationCall, causedBy uuid.UUID) (uuid.UUID, siteFixAICallAttempt, error) {
	s.events = append(s.events, "start_verifier")
	s.verifierCausedBys = append(s.verifierCausedBys, causedBy)
	callID := uuid.New()
	s.verifierCallIDs = append(s.verifierCallIDs, callID)
	return callID, &siteFixAttemptSpy{}, nil
}
func (s *applyStoreStub) FinishGroundingVerification(_ context.Context, _ db.SiteFix, _ uuid.UUID, result GenerationResult) error {
	s.events = append(s.events, "finish_verifier:"+result.Status)
	s.verificationResults = append(s.verificationResults, result)
	return nil
}
func (s *applyStoreStub) RecordPreparationFailure(_ context.Context, _ db.SiteFix, code string) error {
	s.preparationFailure = code
	return nil
}
func (s *applyStoreStub) Finalize(_ context.Context, fix db.SiteFix, plan ApplicationPlan) (db.SiteFix, db.SiteChangeApplication, error) {
	s.events = append(s.events, "finalize")
	s.finalizeCount++
	s.lifecycleTransactionOpen = true
	defer func() { s.lifecycleTransactionOpen = false }()
	fix.Status = "applying"
	s.fix = fix
	return fix, db.SiteChangeApplication{ID: uuid.New(), ProjectID: fix.ProjectID, SiteFixID: validPGUUID(fix.ID), Status: plan.Status}, nil
}

type fixGeneratorStub struct {
	store                 *applyStoreStub
	plan                  ApplicationPlan
	err                   error
	skipAttempt           bool
	failuresBeforeSuccess int
	failureCode           string
	calls                 int
	feedback              []GenerationFeedback
}

func (g *fixGeneratorStub) Describe(db.SiteFix, GenerationContext, RepositorySnapshot, GenerationFeedback) GenerationCall {
	return GenerationCall{Provider: "test", Model: "model", PromptVersion: "doctor-fix-v1", RequestFingerprint: "fingerprint"}
}
func (g *fixGeneratorStub) Generate(ctx context.Context, fix db.SiteFix, generationContext GenerationContext, _ RepositorySnapshot, feedback GenerationFeedback, attempt siteFixAICallAttempt) (ApplicationPlan, GenerationResult, error) {
	if !g.skipAttempt {
		_, _ = attempt.StartAttempt(ctx, "generator-test")
	}
	g.calls++
	g.feedback = append(g.feedback, feedback)
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
	if g.failureCode != "" && g.calls <= g.failuresBeforeSuccess {
		return ApplicationPlan{}, GenerationResult{Status: "failed", ErrorCode: g.failureCode},
			fmt.Errorf("repository patch old_text must occur exactly once in %q on call %d", "app/sitemap.ts", g.calls)
	}
	if g.err != nil {
		return ApplicationPlan{}, GenerationResult{Status: "failed", ErrorCode: "provider_unavailable"}, g.err
	}
	return g.plan, GenerationResult{Status: "ok", TotalTokens: 42}, nil
}

type patchVerifierStub struct {
	store        *applyStoreStub
	verification PatchVerification
	err          error
	decisions    []PatchVerification
	results      []GenerationResult
	errors       []error
	calls        int
}

func (v *patchVerifierStub) Describe(db.SiteFix, GenerationContext, ApplicationPlan) GenerationCall {
	return GenerationCall{Provider: "test", Model: "verifier", PromptVersion: "doctor-patch-grounding-v1", RequestFingerprint: "verifier-fingerprint"}
}

func (v *patchVerifierStub) Verify(ctx context.Context, _ db.SiteFix, _ GenerationContext, _ ApplicationPlan, attempt siteFixAICallAttempt) (PatchVerification, GenerationResult, error) {
	_, _ = attempt.StartAttempt(ctx, "verifier-test")
	v.store.events = append(v.store.events, "verifier")
	v.store.verifierSawLifecycleTransaction = v.store.lifecycleTransactionOpen
	call := v.calls
	v.calls++
	decision := v.verification
	result := GenerationResult{Provider: "test", Model: "verifier", Status: "ok"}
	verifyErr := v.err
	if v.err != nil {
		result = GenerationResult{Status: "failed", ErrorCode: "provider_unavailable"}
	}
	sequenceLength := max(len(v.decisions), len(v.results), len(v.errors))
	if sequenceLength > 0 {
		if call >= sequenceLength {
			return PatchVerification{}, GenerationResult{Status: "failed", ErrorCode: "test_sequence_exhausted"}, errors.New("patch verifier stub outcome sequence exhausted")
		}
		if call < len(v.decisions) {
			decision = v.decisions[call]
		}
		if call < len(v.results) {
			result = v.results[call]
		}
		if call < len(v.errors) {
			verifyErr = v.errors[call]
		}
	}
	return decision, result, verifyErr
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

func applicationPlanForApplyTest(fixID uuid.UUID) ApplicationPlan {
	return ApplicationPlan{
		TargetURL: "https://example.com/", NormalizedTargetURL: "https://example.com/",
		OpportunityKey: "doctor:" + fixID.String(), Status: "ready_for_pr",
		SourceFilePaths:    json.RawMessage(`["app/page.tsx"]`),
		PatchSnapshot:      json.RawMessage(`{"change":"canonical"}`),
		DiffSnapshot:       json.RawMessage(`{}`),
		ResolutionCriteria: json.RawMessage(`{"asset_type":"canonical"}`),
	}
}

func completeApprovedPatchVerification(propositions ...string) PatchVerification {
	return PatchVerification{
		Approved: true, PrimaryIntentPreserved: true,
		PreservedPropositions: append([]string{}, propositions...),
		AddedPropositions:     []string{},
		RemovedPropositions:   []string{},
		UnsupportedClaims:     []string{},
		Reason:                "The patch preserves the approved intent and proposition set.",
	}
}

func assertGroundingRejectionResults(t *testing.T, results []GenerationResult, want int) {
	t.Helper()
	if len(results) < want {
		t.Fatalf("persisted verifier results = %+v, want at least %d grounding rejections", results, want)
	}
	for index := 0; index < want; index++ {
		if results[index].Status != "failed" || results[index].ErrorCode != "grounding_rejected" {
			t.Fatalf("persisted verifier result %d = %+v, want failed/grounding_rejected", index, results[index])
		}
	}
}

func testRepositorySnapshot() RepositorySnapshot {
	return RepositorySnapshot{
		Repo: "acme/site", Branch: "main", BaseCommitSHA: "commit-1",
		Sources: []RepositorySource{{Path: "app/page.tsx", SHA: "blob-1", Content: "export const title = 'Old title'\n"}},
	}
}
