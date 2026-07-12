package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/google/uuid"
)

func TestAuthorizedAIVerificationRecordsCallOutsideLifecycleTransaction(t *testing.T) {
	store := &aiVerificationStoreStub{}
	provider := &aiVerificationProviderStub{store: store, response: llm.CompletionResp{Text: `{"decision":"passed","confidence":0.97,"acceptance_results":[{"index":0,"status":"passed","evidence":{"selector":"main"}}]}`, Provider: "resolved-route", Model: "resolved-model", PromptTokens: 20, CompletionTokens: 11, Tokens: 31, CostUSD: 0.02}}
	reviewer := canonicalAIVerificationReviewer{store: store, provider: provider}
	fix := db.SiteFix{ID: uuid.New(), AcceptanceTests: json.RawMessage(`[{"type":"content_evidence_present"}]`), TargetUrls: json.RawMessage(`["https://example.com/"]`)}
	result, err := reviewer.Review(context.Background(), uuid.New(), fix, completeAIPageEvidence(`<main>rendered</main>`))
	if err != nil || result.Decision != "passed" || result.Confidence != 0.97 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if want := []string{"start", "provider", "finish:ok"}; !reflect.DeepEqual(store.events, want) {
		t.Fatalf("events=%v want=%v", store.events, want)
	}
	if provider.calledWithTransactionOpen {
		t.Fatal("AI provider was called while a lifecycle transaction was open")
	}
	if store.provider != "resolved-route" || store.model != "resolved-model" || store.promptTokens != 20 || store.completionTokens != 11 || store.tokens != 31 || store.cost != 0.02 {
		t.Fatalf("resolved usage not audited: %+v", store)
	}
}

func TestAuthorizedAIVerificationProviderFailureIsAuditedAndCannotPass(t *testing.T) {
	store := &aiVerificationStoreStub{}
	provider := &aiVerificationProviderStub{store: store, err: errors.New("provider down")}
	fix := db.SiteFix{ID: uuid.New(), AcceptanceTests: json.RawMessage(`[{"type":"rendered_text_contains"}]`)}
	result, err := (canonicalAIVerificationReviewer{store: store, provider: provider}).Review(context.Background(), uuid.New(), fix, completeAIPageEvidence(`<html></html>`))
	if err == nil || result.Decision == "passed" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if want := []string{"start", "provider", "finish:failed"}; !reflect.DeepEqual(store.events, want) {
		t.Fatalf("events=%v want=%v", store.events, want)
	}
}

func TestAuthorizedAIVerificationPreflightFailureIsSkipped(t *testing.T) {
	store := &aiVerificationStoreStub{}
	fix := db.SiteFix{ID: uuid.New(), AcceptanceTests: json.RawMessage(`[{"type":"rendered_text_contains"}]`)}
	_, err := (canonicalAIVerificationReviewer{store: store, provider: verificationPreflightFailureProvider{}}).Review(context.Background(), uuid.New(), fix, completeAIPageEvidence(`<html></html>`))
	if err == nil {
		t.Fatal("expected preflight error")
	}
	if want := []string{"start", "finish:skipped"}; !reflect.DeepEqual(store.events, want) {
		t.Fatalf("events=%v want=%v", store.events, want)
	}
}

type verificationPreflightFailureProvider struct{}

func (verificationPreflightFailureProvider) ObservesProviderAttempts() {}
func (verificationPreflightFailureProvider) Complete(context.Context, llm.CompletionReq) (llm.CompletionResp, error) {
	return llm.CompletionResp{}, errors.New("credential lookup failed")
}

func TestDoctorAIVerificationAuthorityDefaultsOff(t *testing.T) {
	defaultCfg, _ := config.Parse(json.RawMessage(`{}`))
	disabledCfg, _ := config.Parse(json.RawMessage(`{"doctor_ai_enabled":false,"doctor_ai_run_policy":"automatic"}`))
	if defaultCfg.AllowsDoctorAI(config.DoctorAITriggerVerificationScheduler) || disabledCfg.AllowsDoctorAI(config.DoctorAITriggerVerificationScheduler) {
		t.Fatal("existing projects must not gain Doctor provider-call authority")
	}
	enabledCfg, _ := config.Parse(json.RawMessage(`{"doctor_ai_enabled":true,"doctor_ai_run_policy":"automatic"}`))
	if !enabledCfg.AllowsDoctorAI(config.DoctorAITriggerVerificationScheduler) {
		t.Fatal("explicit Doctor AI authority was not recognized")
	}
}

func TestAuthorizedAIVerificationRejectsUncollectedCrawlEvidenceBeforeProvider(t *testing.T) {
	store := &aiVerificationStoreStub{}
	provider := &aiVerificationProviderStub{store: store}
	fix := db.SiteFix{ID: uuid.New(), AcceptanceTests: json.RawMessage(`[{"type":"random_url_sample_passes"},{"type":"internal_link_crawl_passes"}]`)}
	result, err := (canonicalAIVerificationReviewer{store: store, provider: provider}).Review(context.Background(), uuid.New(), fix, completeAIPageEvidence(`<html></html>`))
	if err == nil || result.Decision == "passed" || len(store.events) != 0 {
		t.Fatalf("result=%+v err=%v events=%v", result, err, store.events)
	}
}

func completeAIPageEvidence(body string) canonicalPageEvidence {
	return canonicalPageEvidence{Body: body, StatusCode: 200, Headers: http.Header{"Content-Type": {"text/html"}}, FinalURL: "https://example.com/", RedirectChain: []string{"https://example.com/"}}
}

type aiVerificationStoreStub struct {
	events           []string
	txOpen           bool
	provider         string
	model            string
	promptTokens     int
	completionTokens int
	tokens           int
	cost             float64
}

func (s *aiVerificationStoreStub) Start(context.Context, uuid.UUID, uuid.UUID, string) (uuid.UUID, canonicalAICallAttempt, error) {
	s.events = append(s.events, "start")
	return uuid.New(), &canonicalAICallAttemptSpy{}, nil
}

type canonicalAICallAttemptSpy struct{ started bool }

func (s *canonicalAICallAttemptSpy) StartAttempt(context.Context, string) (string, error) {
	s.started = true
	return "verification-attempt", nil
}
func (*canonicalAICallAttemptSpy) FinishAttempt(context.Context, string, llm.CompletionResp, error) error {
	return nil
}
func (s *canonicalAICallAttemptSpy) Started() bool { return s.started }
func (s *aiVerificationStoreStub) Finish(_ context.Context, _, _ uuid.UUID, status, _, provider, model string, promptTokens, completionTokens, tokens int, cost float64) error {
	s.events = append(s.events, "finish:"+status)
	s.provider, s.model, s.promptTokens, s.completionTokens, s.tokens, s.cost = provider, model, promptTokens, completionTokens, tokens, cost
	return nil
}

type aiVerificationProviderStub struct {
	store                     *aiVerificationStoreStub
	response                  llm.CompletionResp
	err                       error
	calledWithTransactionOpen bool
}

func (p *aiVerificationProviderStub) Complete(context.Context, llm.CompletionReq) (llm.CompletionResp, error) {
	p.store.events = append(p.store.events, "provider")
	p.calledWithTransactionOpen = p.store.txOpen
	return p.response, p.err
}
