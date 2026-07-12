package sitefix

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/citeloop/citeloop/internal/aicalls"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/llm"
)

// LLMPatchGroundingVerifier makes a fresh provider call over the actual
// generated artifacts. Its input includes Product Context and observed
// evidence, but excludes the generator's grounding self-report from the facts
// it may rely on.
type LLMPatchGroundingVerifier struct {
	Provider llm.Provider
	Model    string
}

func (v LLMPatchGroundingVerifier) Describe(fix db.SiteFix, generationContext GenerationContext, plan ApplicationPlan) GenerationCall {
	return GenerationCall{
		Provider: "tokengate", Model: firstNonEmpty(v.Model, llm.DefaultTokenGateModel),
		PromptVersion: "doctor-patch-grounding-verification-v1", RequestFingerprint: aicalls.Fingerprint(v.completionRequest(fix, generationContext, plan)),
	}
}

func (v LLMPatchGroundingVerifier) completionRequest(fix db.SiteFix, generationContext GenerationContext, plan ApplicationPlan) llm.CompletionReq {
	prompt, _ := json.Marshal(patchVerificationInput(fix, generationContext, plan))
	return llm.CompletionReq{
		System:  "You are CiteLoop's independent final patch grounding verifier. Inspect the actual patch_snapshot, diff_snapshot, replacement text, and resolution criteria. Compare them only with the approved Product Context, observed evidence, approved primary intent, and preserved propositions. Do not trust or infer safety from any prior generator self-report. Reject every added, removed, or unsupported proposition, commercial promise, offer, capability, or intent change. Return strict JSON only.",
		Prompt:  "Return exactly: {approved:boolean, primary_intent_preserved:boolean, preserved_propositions:string[], added_propositions:string[], removed_propositions:string[], unsupported_claims:string[], intent_drift:boolean, reason:string}. Fail closed: approved may be true only when the actual generated artifacts preserve the full approved proposition set and intent with no unsupported claim.\n" + string(prompt),
		Purpose: llm.PurposeSiteFix, Model: firstNonEmpty(v.Model, llm.DefaultTokenGateModel), JSON: true, MaxTokens: 900, DisableProviderFallback: true,
	}
}

func (v LLMPatchGroundingVerifier) Verify(ctx context.Context, fix db.SiteFix, generationContext GenerationContext, plan ApplicationPlan, attempt siteFixAICallAttempt) (PatchVerification, GenerationResult, error) {
	if v.Provider == nil {
		return PatchVerification{}, GenerationResult{Provider: "none", Model: "none", Status: "skipped", ErrorCode: "provider_unavailable"}, errors.New("independent patch grounding verification provider is unavailable")
	}
	req := v.completionRequest(fix, generationContext, plan)
	req.AttemptObserver = attempt
	resp, err := llm.CompleteObserved(ctx, v.Provider, req)
	result := GenerationResult{
		Provider: firstNonEmpty(resp.Provider, "tokengate"), Model: firstNonEmpty(resp.Model, v.Model), Status: "ok",
		PromptTokens: int32(max(resp.PromptTokens, 0)), CompletionTokens: int32(max(resp.CompletionTokens, 0)),
		TotalTokens: int32(max(resp.Tokens, 0)), CostUSD: resp.CostUSD,
	}
	if err != nil {
		result.Status, result.ErrorCode = "failed", "provider_error"
		return PatchVerification{}, result, err
	}
	var output struct {
		Approved               *bool     `json:"approved"`
		PrimaryIntentPreserved *bool     `json:"primary_intent_preserved"`
		PreservedPropositions  *[]string `json:"preserved_propositions"`
		AddedPropositions      *[]string `json:"added_propositions"`
		RemovedPropositions    *[]string `json:"removed_propositions"`
		UnsupportedClaims      *[]string `json:"unsupported_claims"`
		IntentDrift            *bool     `json:"intent_drift"`
		Reason                 *string   `json:"reason"`
	}
	if err := decodeJSONObject(resp.Text, &output); err != nil || output.Approved == nil || output.PrimaryIntentPreserved == nil ||
		output.PreservedPropositions == nil || output.AddedPropositions == nil || output.RemovedPropositions == nil ||
		output.UnsupportedClaims == nil || output.IntentDrift == nil || output.Reason == nil || strings.TrimSpace(*output.Reason) == "" {
		result.Status, result.ErrorCode = "failed", "invalid_response"
		if err != nil {
			return PatchVerification{}, result, err
		}
		return PatchVerification{}, result, errors.New("independent patch grounding verifier returned an incomplete decision")
	}
	verification := PatchVerification{
		Approved: *output.Approved, PrimaryIntentPreserved: *output.PrimaryIntentPreserved,
		PreservedPropositions: *output.PreservedPropositions, AddedPropositions: *output.AddedPropositions,
		RemovedPropositions: *output.RemovedPropositions, UnsupportedClaims: *output.UnsupportedClaims,
		IntentDrift: *output.IntentDrift, Reason: strings.TrimSpace(*output.Reason),
	}
	if err := validatePatchVerification(fix, verification); err != nil {
		result.Status, result.ErrorCode = "failed", "grounding_rejected"
		return verification, result, err
	}
	return verification, result, nil
}

func patchVerificationInput(fix db.SiteFix, generationContext GenerationContext, plan ApplicationPlan) map[string]any {
	intent, propositions, _ := approvedPropositionContract(fix.EvidenceSnapshot)
	return map[string]any{
		"product_context": generationContext.ProductProfile, "context_profile_version": generationContext.ProfileVersion,
		"observed_evidence": generationContext.ObservedEvidence, "approved_primary_intent": intent,
		"approved_preserved_propositions": propositions, "patch_snapshot": plan.PatchSnapshot,
		"diff_snapshot": plan.DiffSnapshot, "resolution_criteria": independentResolutionCriteria(plan.ResolutionCriteria),
	}
}

func independentResolutionCriteria(raw json.RawMessage) json.RawMessage {
	var criteria map[string]any
	if json.Unmarshal(raw, &criteria) != nil || criteria == nil {
		return raw
	}
	delete(criteria, "grounding_validation")
	clean, err := json.Marshal(criteria)
	if err != nil {
		return raw
	}
	return clean
}

// DeterministicPatchGroundingVerifier validates the non-AI manual handoff
// against the approved snapshots without making a provider call.
type DeterministicPatchGroundingVerifier struct{}

func (DeterministicPatchGroundingVerifier) Describe(fix db.SiteFix, generationContext GenerationContext, plan ApplicationPlan) GenerationCall {
	payload, _ := json.Marshal(patchVerificationInput(fix, generationContext, plan))
	sum := sha256.Sum256(payload)
	return GenerationCall{Provider: "none", Model: "none", PromptVersion: "doctor-patch-grounding-deterministic-v1", RequestFingerprint: hex.EncodeToString(sum[:])}
}

func (DeterministicPatchGroundingVerifier) Verify(_ context.Context, fix db.SiteFix, generationContext GenerationContext, plan ApplicationPlan, _ siteFixAICallAttempt) (PatchVerification, GenerationResult, error) {
	if plan.Status != "manual_apply_required" || !sameJSON(plan.PatchSnapshot, fix.ProposedFix) || !sameJSON(plan.DiffSnapshot, fix.ProposedFix) {
		return PatchVerification{}, GenerationResult{Provider: "none", Model: "none", Status: "skipped", ErrorCode: "deterministic_snapshot_mismatch"}, ErrPatchGroundingRejected
	}
	if err := validateApplicationPlan(fix, generationContext, plan); err != nil {
		return PatchVerification{}, GenerationResult{Provider: "none", Model: "none", Status: "skipped", ErrorCode: "invalid_output"}, err
	}
	_, propositions, err := approvedPropositionContract(fix.EvidenceSnapshot)
	if err != nil {
		return PatchVerification{}, GenerationResult{Provider: "none", Model: "none", Status: "skipped", ErrorCode: "invalid_evidence"}, err
	}
	verification := PatchVerification{Approved: true, PrimaryIntentPreserved: true, PreservedPropositions: propositions, Reason: "Approved canonical snapshots are unchanged."}
	return verification, GenerationResult{Provider: "none", Model: "none", Status: "skipped", ErrorCode: "deterministic_verification"}, nil
}
