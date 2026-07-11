package seo

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/google/uuid"
)

func TestDoctorDiagnosisAuthorityDoesNotExpandExistingProjectDefaults(t *testing.T) {
	cfg := config.Default()
	for _, trigger := range []DoctorTrigger{DoctorTriggerManual, DoctorTriggerWeekly, DoctorTriggerOnboarding, DoctorTriggerPostPublish} {
		if doctorDiagnosisAIAuthorized(cfg, trigger) {
			t.Fatalf("default config authorized diagnosis for trigger %q", trigger)
		}
	}

	cfg.DoctorAIEnabled = true
	cfg.DoctorAIRunPolicy = config.DoctorAIRunPolicyManualOnly
	if !doctorDiagnosisAIAuthorized(cfg, DoctorTriggerManual) {
		t.Fatal("explicitly enabled manual Doctor run should authorize diagnosis AI")
	}
	if doctorDiagnosisAIAuthorized(cfg, DoctorTriggerWeekly) {
		t.Fatal("manual-only Doctor AI policy must not authorize weekly diagnosis")
	}
	cfg.DoctorAIRunPolicy = config.DoctorAIRunPolicyAutomatic
	if !doctorDiagnosisAIAuthorized(cfg, DoctorTriggerWeekly) {
		t.Fatal("explicit automatic Doctor AI policy should authorize weekly diagnosis")
	}
}

func TestDoctorDiagnosisLedgerUsesCanonicalSchemaStage(t *testing.T) {
	if doctorDiagnosisAICallStage != "doctor_diagnosis" {
		t.Fatalf("AI call stage = %q, want schema-allowed doctor_diagnosis", doctorDiagnosisAICallStage)
	}
}

func TestDoctorGSCAndGA4ContextPrioritizesWithoutChangingCompletionContract(t *testing.T) {
	candidate := doctorFindingCandidate{
		FindingKey:           "finding-1",
		NormalizedURLs:       []string{"https://example.com/high-value"},
		AffectedURLs:         []string{"https://example.com/high-value"},
		Evidence:             map[string]any{"canonical_status": "missing"},
		ImportanceMultiplier: 1,
	}
	inputs := []doctorPagePriorityInput{{
		NormalizedPageURL: "https://example.com/high-value",
		GSCClicks28D:      42,
		GSCImpressions28D: 1200,
		GA4Sessions28D:    320,
		GA4KeyEvents28D:   7,
	}}

	got := applyDoctorPagePriorityInputs([]doctorFindingCandidate{candidate}, inputs)
	if len(got) != 1 || got[0].FindingKey != candidate.FindingKey {
		t.Fatalf("prioritization changed deterministic finding identity: %#v", got)
	}
	if got[0].ImportanceMultiplier <= 1 || got[0].ImportanceMultiplier > doctorMaximumImpactMultiplier {
		t.Fatalf("importance multiplier = %v, want bounded impact increase", got[0].ImportanceMultiplier)
	}
	impact, ok := got[0].Evidence["impact_context"].(map[string]any)
	if !ok {
		t.Fatalf("impact context = %#v", got[0].Evidence["impact_context"])
	}
	if impact["gsc_impressions_28d"] != float64(1200) || impact["ga4_sessions_28d"] != float64(320) {
		t.Fatalf("impact context = %#v, want GSC and GA4 inputs", impact)
	}
	if impact["completion_contract"] != "immediate_evidence_only" {
		t.Fatalf("completion contract = %#v, delayed uplift must not verify Doctor work", impact["completion_contract"])
	}
}

func TestDoctorDiagnosisDisabledDoesNotCallProviderOrLedger(t *testing.T) {
	provider := &doctorLLMStub{err: errors.New("provider must not be called")}
	ledger := &doctorAILedgerSpy{}
	candidates := []doctorFindingCandidate{{FindingKey: "finding-1", Evidence: map[string]any{"status": "missing"}}}

	got, state := runDoctorDiagnosisAI(context.Background(), doctorDiagnosisAIRequest{
		ProjectID: uuid.New(), RunID: uuid.New(), Authorized: false,
		Candidates: candidates, Provider: provider, Ledger: ledger,
	})

	if provider.calls != 0 || ledger.starts != 0 || ledger.finishes != 0 {
		t.Fatalf("disabled authority made external work: provider=%d ledger=%d/%d", provider.calls, ledger.starts, ledger.finishes)
	}
	if len(got) != 1 || got[0].FindingKey != "finding-1" || state.Status != doctorAIStatusDisabled {
		t.Fatalf("disabled result = %#v / %#v", got, state)
	}
}

func TestDoctorDiagnosisProviderUnavailablePreservesDeterministicFindings(t *testing.T) {
	ledger := &doctorAILedgerSpy{}
	candidates := []doctorFindingCandidate{{FindingKey: "finding-1", Evidence: map[string]any{"status": "missing"}}}

	got, state := runDoctorDiagnosisAI(context.Background(), doctorDiagnosisAIRequest{
		ProjectID: uuid.New(), RunID: uuid.New(), Authorized: true,
		Candidates: candidates, Ledger: ledger,
	})

	if len(got) != 1 || got[0].FindingKey != "finding-1" {
		t.Fatalf("provider unavailable fabricated or removed findings: %#v", got)
	}
	if ledger.starts != 0 || state.Status != doctorAIStatusUnavailable || !state.Degraded {
		t.Fatalf("provider unavailable state = %#v, ledger starts = %d", state, ledger.starts)
	}
}

func TestDoctorDiagnosisAILedgersCallAndOnlyEnrichesKnownFinding(t *testing.T) {
	provider := &doctorLLMStub{response: llm.CompletionResp{
		Text:     `{"priorities":[{"finding_key":"finding-1","priority":"high","reason":"High observed search demand amplifies the affected page.","evidence_keys":["impact_context"]},{"finding_key":"invented","priority":"high","reason":"Invented.","evidence_keys":["unknown"]}]}`,
		Provider: "fixture", Model: "fixture-model", PromptTokens: 20, CompletionTokens: 10, Tokens: 30, CostUSD: 0.02,
	}}
	ledger := &doctorAILedgerSpy{}
	candidates := []doctorFindingCandidate{{
		FindingKey: "finding-1", ImportanceMultiplier: 1,
		Evidence: map[string]any{"impact_context": map[string]any{"gsc_impressions_28d": float64(1200)}},
	}}

	got, state := runDoctorDiagnosisAI(context.Background(), doctorDiagnosisAIRequest{
		ProjectID: uuid.New(), RunID: uuid.New(), Authorized: true,
		Candidates: candidates, Context: []byte(`{"positioning":"fixture"}`), Provider: provider, Ledger: ledger,
	})

	if provider.calls != 1 || ledger.starts != 1 || ledger.finishes != 1 {
		t.Fatalf("AI call accounting provider=%d ledger=%d/%d", provider.calls, ledger.starts, ledger.finishes)
	}
	if ledger.finished.Status != "ok" || ledger.finished.TotalTokens != 30 || ledger.finished.CostUSD != 0.02 {
		t.Fatalf("ledger finish = %#v", ledger.finished)
	}
	if len(got) != 1 || got[0].FindingKey != "finding-1" {
		t.Fatalf("AI output changed deterministic finding set: %#v", got)
	}
	if got[0].ImportanceMultiplier <= 1 || state.Status != doctorAIStatusApplied || state.AppliedPriorities != 1 {
		t.Fatalf("AI priority was not safely applied: %#v / %#v", got[0], state)
	}
	if _, ok := got[0].Evidence["ai_diagnosis_review"]; !ok {
		t.Fatalf("missing auditable AI review evidence: %#v", got[0].Evidence)
	}
}

func TestDoctorDiagnosisInvalidResponseFailsDegradedWithoutFalseFinding(t *testing.T) {
	provider := &doctorLLMStub{response: llm.CompletionResp{Text: `{"findings":[{"finding_key":"invented"}]}`, Provider: "fixture", Model: "fixture"}}
	ledger := &doctorAILedgerSpy{}
	candidates := []doctorFindingCandidate{{FindingKey: "finding-1", Evidence: map[string]any{"status": "missing"}}}

	got, state := runDoctorDiagnosisAI(context.Background(), doctorDiagnosisAIRequest{
		ProjectID: uuid.New(), RunID: uuid.New(), Authorized: true,
		Candidates: candidates, Provider: provider, Ledger: ledger,
	})

	if len(got) != 1 || got[0].FindingKey != "finding-1" {
		t.Fatalf("invalid response changed deterministic findings: %#v", got)
	}
	if state.Status != doctorAIStatusInvalid || !state.Degraded || ledger.finished.Status != "failed" {
		t.Fatalf("invalid response state=%#v ledger=%#v", state, ledger.finished)
	}
}

func TestOptionalDoctorIntelligenceCoverageDoesNotTurnDeterministicHealthIntoFailure(t *testing.T) {
	coverage := []doctorCheckCoverage{
		{Check: "http_status", CheckedURLs: []string{"https://example.com"}, PassedURLs: []string{"https://example.com"}},
		{Check: "gsc_priority_context", SkippedURLs: []string{"gsc_unavailable"}},
		{Check: "ga4_priority_context", SkippedURLs: []string{"ga4_unavailable"}},
		{Check: "doctor_ai_review", SkippedURLs: []string{"diagnosis_ai:disabled"}},
	}
	if !doctorCoverageComplete(coverage) {
		t.Fatal("optional intelligence sources should report degraded coverage without invalidating passed deterministic health contracts")
	}
}

func TestDoctorFindingCarriesBoundedContextSnapshotForSiteFixGrounding(t *testing.T) {
	candidates := applyDoctorPagePriorityInputs([]doctorFindingCandidate{{
		FindingKey: "finding-1", NormalizedURLs: []string{"https://example.com/page"}, Evidence: map[string]any{"canonical_status": "missing"},
	}}, []doctorPagePriorityInput{{NormalizedPageURL: "https://example.com/page", GSCImpressions28D: 800, GA4Sessions28D: 90}})
	candidates = attachDoctorContextSnapshot(candidates, doctorProductContextSnapshot{Version: 7, Profile: []byte(`{"positioning":"Evidence-backed product fact"}`)})

	snapshot, ok := candidates[0].Evidence["context_snapshot"].(map[string]any)
	if !ok || snapshot["product_context_version"] != int32(7) || snapshot["product_context"] == nil || snapshot["performance_evidence"] == nil {
		t.Fatalf("context snapshot = %#v", candidates[0].Evidence["context_snapshot"])
	}
	raw, err := json.Marshal(snapshot)
	if err != nil || len(raw) > doctorContextSnapshotByteLimit+2048 {
		t.Fatalf("context snapshot must be bounded: bytes=%d err=%v", len(raw), err)
	}
}

type doctorLLMStub struct {
	response llm.CompletionResp
	err      error
	calls    int
}

func (s *doctorLLMStub) Complete(context.Context, llm.CompletionReq) (llm.CompletionResp, error) {
	s.calls++
	return s.response, s.err
}

type doctorAILedgerSpy struct {
	starts   int
	finishes int
	started  doctorAICallStart
	finished doctorAICallFinish
}

func (s *doctorAILedgerSpy) StartDoctorAICall(_ context.Context, call doctorAICallStart) (uuid.UUID, error) {
	s.starts++
	s.started = call
	return uuid.New(), nil
}

func (s *doctorAILedgerSpy) FinishDoctorAICall(_ context.Context, call doctorAICallFinish) error {
	s.finishes++
	s.finished = call
	return nil
}
