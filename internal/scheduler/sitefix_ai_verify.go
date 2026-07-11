package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type canonicalAIVerificationResult struct {
	CallID            uuid.UUID
	Decision          string
	Confidence        float64
	AcceptanceResults json.RawMessage
}

type canonicalAIVerificationStore interface {
	Start(context.Context, uuid.UUID, uuid.UUID, string) (uuid.UUID, error)
	Finish(context.Context, uuid.UUID, uuid.UUID, string, string, string, string, int, int, int, float64) error
}

type canonicalAIVerificationReviewer struct {
	store    canonicalAIVerificationStore
	provider llm.Provider
	model    string
}

func (r canonicalAIVerificationReviewer) Review(ctx context.Context, projectID uuid.UUID, fix db.SiteFix, page canonicalPageEvidence) (canonicalAIVerificationResult, error) {
	if r.store == nil || r.provider == nil {
		return canonicalAIVerificationResult{}, errors.New("authorized Doctor AI verifier is unavailable")
	}
	if ok, reason := canonicalAIVerificationCapability(fix.AcceptanceTests, page); !ok {
		return canonicalAIVerificationResult{Decision: "inconclusive"}, fmt.Errorf("AI verification evidence capability is insufficient: %s", reason)
	}
	fingerprintEvidence, _ := json.Marshal(canonicalPageEvidenceMap("", page, nil))
	fingerprintInput := append(append([]byte{}, fix.AcceptanceTests...), fingerprintEvidence...)
	sum := sha256.Sum256(fingerprintInput)
	callID, err := r.store.Start(ctx, projectID, fix.ID, hex.EncodeToString(sum[:]))
	if err != nil {
		return canonicalAIVerificationResult{}, err
	}
	return r.reviewWithCallID(ctx, projectID, fix, page, callID)
}

func (r canonicalAIVerificationReviewer) reviewWithCallID(ctx context.Context, projectID uuid.UUID, fix db.SiteFix, page canonicalPageEvidence, callID uuid.UUID) (canonicalAIVerificationResult, error) {
	model := strings.TrimSpace(r.model)
	if model == "" {
		model = llm.DefaultTokenGateModel
	}
	evidenceHTML := page.Body
	if len(evidenceHTML) > 50000 {
		evidenceHTML = evidenceHTML[:50000]
	}
	prompt, _ := json.Marshal(map[string]any{
		"acceptance_tests": fix.AcceptanceTests,
		"target_urls":      fix.TargetUrls,
		"http_status":      page.StatusCode,
		"response_headers": canonicalEvidenceHeaders(page.Headers),
		"final_url":        page.FinalURL,
		"redirect_chain":   page.RedirectChain,
		"rendered_html":    evidenceHTML,
	})
	resp, providerErr := r.provider.Complete(ctx, llm.CompletionReq{
		System:  "You verify an already-applied Doctor Site Fix. Evaluate only the stored acceptance tests against the supplied production evidence. Do not generate content or infer missing evidence. Return strict JSON.",
		Prompt:  "Return {\"decision\":\"passed|failed|inconclusive\",\"confidence\":0..1,\"acceptance_results\":[{\"index\":0,\"status\":\"passed|failed|inconclusive\",\"evidence\":{...}}]}. Every stored test must appear exactly once.\n" + string(prompt),
		Purpose: llm.PurposeQA, Model: model, JSON: true, MaxTokens: 1200, DisableProviderFallback: true,
	})
	if providerErr != nil {
		finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if finishErr := r.store.Finish(finishCtx, projectID, callID, "failed", "provider_error", resp.Provider, resp.Model, resp.PromptTokens, resp.CompletionTokens, resp.Tokens, resp.CostUSD); finishErr != nil {
			return canonicalAIVerificationResult{CallID: callID}, errors.Join(providerErr, finishErr)
		}
		return canonicalAIVerificationResult{CallID: callID}, providerErr
	}
	result, parseErr := parseCanonicalAIVerificationResponse(resp.Text, fix.AcceptanceTests)
	status, errorCode := "ok", ""
	if parseErr != nil {
		status, errorCode = "failed", "invalid_response"
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if finishErr := r.store.Finish(finishCtx, projectID, callID, status, errorCode, resp.Provider, resp.Model, resp.PromptTokens, resp.CompletionTokens, resp.Tokens, resp.CostUSD); finishErr != nil {
		if parseErr != nil {
			return canonicalAIVerificationResult{CallID: callID}, errors.Join(parseErr, finishErr)
		}
		return canonicalAIVerificationResult{CallID: callID}, finishErr
	}
	if parseErr != nil {
		return canonicalAIVerificationResult{CallID: callID}, parseErr
	}
	result.CallID = callID
	return result, nil
}

func canonicalAIVerificationCapability(storedTests json.RawMessage, page canonicalPageEvidence) (bool, string) {
	if page.StatusCode == 0 || strings.TrimSpace(page.FinalURL) == "" || len(page.RedirectChain) == 0 || page.Headers == nil || strings.TrimSpace(page.Body) == "" {
		return false, "complete fetched PageEvidence is required"
	}
	var tests []struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(storedTests, &tests) != nil || len(tests) == 0 {
		return false, "typed acceptance tests are required"
	}
	for _, test := range tests {
		switch strings.ToLower(strings.TrimSpace(test.Type)) {
		case "rendered_text_contains", "rendered_text_equals", "content_evidence_present", "accessibility_semantics":
		default:
			return false, "test family " + strings.TrimSpace(test.Type) + " requires evidence that was not collected"
		}
	}
	return true, ""
}

func parseCanonicalAIVerificationResponse(text string, storedTests json.RawMessage) (canonicalAIVerificationResult, error) {
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return canonicalAIVerificationResult{}, errors.New("AI verification response has no JSON object")
	}
	var payload struct {
		Decision   string  `json:"decision"`
		Confidence float64 `json:"confidence"`
		Results    []struct {
			Index    int             `json:"index"`
			Status   string          `json:"status"`
			Evidence json.RawMessage `json:"evidence"`
		} `json:"acceptance_results"`
	}
	if err := json.Unmarshal([]byte(text[start:end+1]), &payload); err != nil {
		return canonicalAIVerificationResult{}, err
	}
	var tests []json.RawMessage
	if json.Unmarshal(storedTests, &tests) != nil || len(tests) == 0 || len(payload.Results) != len(tests) {
		return canonicalAIVerificationResult{}, errors.New("AI verification result count does not match stored acceptance tests")
	}
	decision := strings.ToLower(strings.TrimSpace(payload.Decision))
	if decision != "passed" && decision != "failed" && decision != "inconclusive" {
		return canonicalAIVerificationResult{}, fmt.Errorf("invalid AI verification decision %q", decision)
	}
	if payload.Confidence < 0 || payload.Confidence > 1 {
		return canonicalAIVerificationResult{}, errors.New("AI verification confidence must be between 0 and 1")
	}
	seen := make(map[int]bool, len(tests))
	for _, result := range payload.Results {
		var evidence map[string]any
		if result.Index < 0 || result.Index >= len(tests) || seen[result.Index] ||
			(result.Status != "passed" && result.Status != "failed" && result.Status != "inconclusive") ||
			json.Unmarshal(result.Evidence, &evidence) != nil || len(evidence) == 0 {
			return canonicalAIVerificationResult{}, errors.New("invalid AI acceptance result")
		}
		if decision == "passed" && result.Status != "passed" {
			return canonicalAIVerificationResult{}, errors.New("AI passed decision contains a non-passing acceptance result")
		}
		seen[result.Index] = true
	}
	rawResults, _ := json.Marshal(payload.Results)
	return canonicalAIVerificationResult{Decision: decision, Confidence: payload.Confidence, AcceptanceResults: rawResults}, nil
}

type postgresCanonicalAIVerificationStore struct{ q *db.Queries }

func (s postgresCanonicalAIVerificationStore) Start(ctx context.Context, projectID, fixID uuid.UUID, fingerprint string) (uuid.UUID, error) {
	row, err := s.q.CreateAICallRecord(ctx, db.CreateAICallRecordParams{
		ProjectID: projectID, Stage: "verification", LinkedObjectType: "site_fix", LinkedObjectID: fixID,
		Provider: "tokengate", Model: llm.DefaultTokenGateModel, PromptVersion: "doctor-verification-v1",
		RequestFingerprint: fingerprint, Status: "running",
	})
	if err != nil {
		return uuid.Nil, err
	}
	return row.ID, nil
}

func (s postgresCanonicalAIVerificationStore) Finish(ctx context.Context, projectID, callID uuid.UUID, status, errorCode, provider, model string, promptTokens, completionTokens, tokens int, costUSD float64) error {
	cost := pgtype.Numeric{}
	if err := cost.Scan(fmt.Sprintf("%.8f", max(costUSD, 0))); err != nil {
		return err
	}
	var code *string
	if errorCode != "" {
		code = &errorCode
	}
	_, err := s.q.FinishCanonicalAICallFenced(ctx, db.FinishCanonicalAICallFencedParams{
		Status: status, ErrorCode: code, ResolvedProvider: optionalAIString(provider), ResolvedModel: optionalAIString(model),
		PromptTokens: int32(max(promptTokens, 0)), CompletionTokens: int32(max(completionTokens, 0)), TotalTokens: int32(max(tokens, 0)), CostUsd: cost,
		ID: callID, ProjectID: projectID,
	})
	return err
}

func optionalAIString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
