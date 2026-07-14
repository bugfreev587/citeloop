package discovery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	"github.com/citeloop/citeloop/internal/llm"
	"github.com/google/uuid"
)

const (
	SemanticPromptVersionV1      = "discovery-semantic-v1"
	SemanticFingerprintVersionV1 = "discovery-semantic-fingerprint-v1"
)

type DecisionKind string

const (
	DecisionCreate           DecisionKind = "create"
	DecisionMergeEvidence    DecisionKind = "merge_evidence"
	DecisionSuppress         DecisionKind = "suppress"
	DecisionBlockOnOtherLine DecisionKind = "block_on_other_line"
)

type SemanticWork struct {
	ID                  uuid.UUID
	ExactSignatureHash  string
	SignaturePayload    json.RawMessage
	SemanticFingerprint string
}

type SemanticRequest struct {
	CandidateID      uuid.UUID
	Candidate        Candidate
	Identity         Identity
	PossibleOverlaps []SemanticWork
	AttemptObserver  llm.AttemptObserver
}

type SemanticDecision struct {
	Decision            DecisionKind
	Owner               Owner
	WorkSignature       string
	Overlaps            []uuid.UUID
	Reason              string
	Confidence          float64
	SemanticFingerprint string
}

type CallUsage struct {
	Provider           string
	Model              string
	PromptVersion      string
	RequestFingerprint string
	PromptTokens       int
	CompletionTokens   int
	TotalTokens        int
	CostUSD            float64
}

var ErrInvalidSemanticResponse = errors.New("invalid semantic comparison response")

type SemanticComparator interface {
	Compare(context.Context, SemanticRequest) (SemanticDecision, CallUsage, error)
}

type LLMSemanticComparator struct {
	provider     llm.Provider
	providerName string
	model        string
	purpose      llm.CompletionPurpose
}

// WithPurpose returns a comparator whose provider request is routed for the
// same runtime purpose used by its caller's authority fingerprint.
func (c *LLMSemanticComparator) WithPurpose(purpose llm.CompletionPurpose) *LLMSemanticComparator {
	if c == nil {
		return nil
	}
	configured := *c
	configured.purpose = purpose
	return &configured
}

func NewLLMSemanticComparator(provider llm.Provider, providerName, model string) *LLMSemanticComparator {
	return &LLMSemanticComparator{
		provider:     provider,
		providerName: strings.TrimSpace(providerName),
		model:        strings.TrimSpace(model),
	}
}

func (c *LLMSemanticComparator) Compare(ctx context.Context, request SemanticRequest) (SemanticDecision, CallUsage, error) {
	if c == nil || c.provider == nil {
		return SemanticDecision{}, CallUsage{}, errors.New("semantic comparison provider is required")
	}
	prompt, requestFingerprint, err := buildSemanticPrompt(request, c.model)
	if err != nil {
		return SemanticDecision{}, CallUsage{}, err
	}
	resp, err := llm.CompleteObserved(ctx, c.provider, llm.CompletionReq{
		Purpose:         c.purpose,
		System:          "You are CiteLoop's work arbitration comparator. Compare only the structured work specifications supplied. Return the required JSON decision without markdown.",
		Prompt:          prompt,
		Model:           c.model,
		MaxTokens:       900,
		Temperature:     0,
		JSON:            true,
		AttemptObserver: request.AttemptObserver,
	})
	usage := CallUsage{
		Provider:           firstNonEmpty(resp.Provider, c.providerName),
		Model:              firstNonEmpty(resp.Model, c.model),
		PromptVersion:      SemanticPromptVersionV1,
		RequestFingerprint: requestFingerprint,
		PromptTokens:       resp.PromptTokens,
		CompletionTokens:   resp.CompletionTokens,
		TotalTokens:        resp.Tokens,
		CostUSD:            resp.CostUSD,
	}
	if err != nil {
		return SemanticDecision{}, usage, err
	}
	decision, err := parseSemanticDecision(resp.Text, request)
	if err != nil {
		return SemanticDecision{}, usage, fmt.Errorf("%w: %v", ErrInvalidSemanticResponse, err)
	}
	decision.SemanticFingerprint, err = semanticFingerprint(request.Identity, firstNonEmpty(resp.Model, c.model))
	if err != nil {
		return SemanticDecision{}, usage, fmt.Errorf("%w: %v", ErrInvalidSemanticResponse, err)
	}
	return decision, usage, nil
}

type semanticPromptWork struct {
	WorkID              string          `json:"work_id"`
	WorkSignature       string          `json:"work_signature"`
	SignaturePayload    json.RawMessage `json:"signature_payload"`
	SemanticFingerprint string          `json:"semantic_fingerprint,omitempty"`
}

type semanticPromptEnvelope struct {
	PromptVersion    string               `json:"prompt_version"`
	Candidate        semanticPromptWork   `json:"candidate"`
	PossibleOverlaps []semanticPromptWork `json:"possible_overlaps"`
	OutputContract   map[string]any       `json:"output_contract"`
}

func buildSemanticPrompt(request SemanticRequest, model string) (string, string, error) {
	if strings.TrimSpace(request.Identity.ExactSignatureHash) == "" || len(request.Identity.SignaturePayload) == 0 {
		return "", "", fmt.Errorf("semantic request requires a complete candidate identity")
	}
	overlaps := append([]SemanticWork(nil), request.PossibleOverlaps...)
	sort.Slice(overlaps, func(i, j int) bool { return overlaps[i].ID.String() < overlaps[j].ID.String() })
	promptOverlaps := make([]semanticPromptWork, 0, len(overlaps))
	for _, work := range overlaps {
		if work.ID == uuid.Nil || strings.TrimSpace(work.ExactSignatureHash) == "" || len(work.SignaturePayload) == 0 {
			return "", "", fmt.Errorf("possible overlap requires id, signature, and payload")
		}
		promptOverlaps = append(promptOverlaps, semanticPromptWork{
			WorkID:              work.ID.String(),
			WorkSignature:       work.ExactSignatureHash,
			SignaturePayload:    work.SignaturePayload,
			SemanticFingerprint: work.SemanticFingerprint,
		})
	}
	envelope := semanticPromptEnvelope{
		PromptVersion: SemanticPromptVersionV1,
		Candidate: semanticPromptWork{
			WorkSignature:    request.Identity.ExactSignatureHash,
			SignaturePayload: request.Identity.SignaturePayload,
		},
		PossibleOverlaps: promptOverlaps,
		OutputContract: map[string]any{
			"decision":        []string{string(DecisionCreate), string(DecisionMergeEvidence), string(DecisionSuppress), string(DecisionBlockOnOtherLine)},
			"owner":           []string{string(OwnerDoctor), string(OwnerOpportunities)},
			"overlaps":        "array of work_id strings from possible_overlaps; use an empty array when none apply",
			"required_fields": []string{"decision", "owner", "work_signature", "overlaps", "reason", "confidence"},
		},
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return "", "", fmt.Errorf("marshal semantic prompt: %w", err)
	}
	fingerprintInput := struct {
		Model string          `json:"model"`
		Body  json.RawMessage `json:"body"`
	}{Model: strings.TrimSpace(model), Body: raw}
	fingerprintRaw, err := json.Marshal(fingerprintInput)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(fingerprintRaw)
	return string(raw), hex.EncodeToString(sum[:]), nil
}

type semanticDecisionResponse struct {
	Decision      string            `json:"decision"`
	Owner         string            `json:"owner"`
	WorkSignature string            `json:"work_signature"`
	Overlaps      []json.RawMessage `json:"overlaps"`
	Reason        string            `json:"reason"`
	Confidence    float64           `json:"confidence"`
}

func parseSemanticDecision(raw string, request SemanticRequest) (SemanticDecision, error) {
	payload, err := semanticJSONPayload(raw)
	if err != nil {
		return SemanticDecision{}, err
	}
	var response semanticDecisionResponse
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return SemanticDecision{}, fmt.Errorf("decode semantic decision: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return SemanticDecision{}, fmt.Errorf("semantic decision must contain exactly one JSON object")
	}
	decision := DecisionKind(strings.TrimSpace(response.Decision))
	switch decision {
	case DecisionCreate, DecisionMergeEvidence, DecisionSuppress, DecisionBlockOnOtherLine:
	default:
		return SemanticDecision{}, fmt.Errorf("unsupported semantic decision %q", response.Decision)
	}
	owner := Owner(strings.TrimSpace(response.Owner))
	if owner != OwnerDoctor && owner != OwnerOpportunities {
		return SemanticDecision{}, fmt.Errorf("unsupported semantic owner %q", response.Owner)
	}
	if response.WorkSignature != request.Identity.ExactSignatureHash {
		return SemanticDecision{}, fmt.Errorf("semantic decision signature does not match candidate")
	}
	if math.IsNaN(response.Confidence) || math.IsInf(response.Confidence, 0) || response.Confidence < 0 || response.Confidence > 1 {
		return SemanticDecision{}, fmt.Errorf("semantic decision confidence must be between 0 and 1")
	}
	if strings.TrimSpace(response.Reason) == "" {
		return SemanticDecision{}, fmt.Errorf("semantic decision reason is required")
	}
	allowed := make(map[uuid.UUID]struct{}, len(request.PossibleOverlaps))
	for _, work := range request.PossibleOverlaps {
		allowed[work.ID] = struct{}{}
	}
	overlaps := make([]uuid.UUID, 0, len(response.Overlaps))
	seen := make(map[uuid.UUID]struct{}, len(response.Overlaps))
	for _, rawOverlap := range response.Overlaps {
		value, err := semanticOverlapWorkID(rawOverlap)
		if err != nil {
			return SemanticDecision{}, err
		}
		id, err := uuid.Parse(value)
		if err != nil {
			return SemanticDecision{}, fmt.Errorf("invalid overlap id %q", value)
		}
		if _, ok := allowed[id]; !ok {
			return SemanticDecision{}, fmt.Errorf("semantic decision referenced unknown overlap %s", id)
		}
		if _, ok := seen[id]; !ok {
			overlaps = append(overlaps, id)
			seen[id] = struct{}{}
		}
	}
	sort.Slice(overlaps, func(i, j int) bool { return overlaps[i].String() < overlaps[j].String() })
	if decision != DecisionCreate && len(overlaps) == 0 {
		return SemanticDecision{}, fmt.Errorf("semantic decision %q requires at least one overlap", decision)
	}
	return SemanticDecision{
		Decision:      decision,
		Owner:         owner,
		WorkSignature: response.WorkSignature,
		Overlaps:      overlaps,
		Reason:        strings.TrimSpace(response.Reason),
		Confidence:    response.Confidence,
	}, nil
}

func semanticOverlapWorkID(raw json.RawMessage) (string, error) {
	payload := bytes.TrimSpace(raw)
	if len(payload) == 0 {
		return "", fmt.Errorf("semantic overlap reference is empty")
	}
	if payload[0] == '"' {
		var value string
		if err := json.Unmarshal(payload, &value); err != nil {
			return "", fmt.Errorf("decode semantic overlap id: %w", err)
		}
		return strings.TrimSpace(value), nil
	}
	var reference struct {
		WorkID string `json:"work_id"`
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&reference); err != nil {
		return "", fmt.Errorf("decode semantic overlap reference: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("semantic overlap reference must contain exactly one JSON value")
	}
	if strings.TrimSpace(reference.WorkID) == "" {
		return "", fmt.Errorf("semantic overlap reference requires work_id")
	}
	return strings.TrimSpace(reference.WorkID), nil
}

func semanticJSONPayload(raw string) (string, error) {
	payload := strings.TrimSpace(raw)
	if !strings.HasPrefix(payload, "```") {
		return payload, nil
	}
	lineEnd := strings.IndexByte(payload, '\n')
	if lineEnd < 0 {
		return "", fmt.Errorf("semantic decision code fence is incomplete")
	}
	opening := strings.TrimSpace(payload[:lineEnd])
	if opening != "```" && !strings.EqualFold(opening, "```json") {
		return "", fmt.Errorf("semantic decision code fence must contain JSON")
	}
	if !strings.HasSuffix(payload, "```") {
		return "", fmt.Errorf("semantic decision code fence is incomplete")
	}
	body := strings.TrimSpace(payload[lineEnd+1 : len(payload)-3])
	if body == "" || strings.Contains(body, "```") {
		return "", fmt.Errorf("semantic decision must contain one fenced JSON object")
	}
	return body, nil
}

func semanticFingerprint(identity Identity, model string) (string, error) {
	payload := struct {
		Version          string          `json:"version"`
		Model            string          `json:"model"`
		SignaturePayload json.RawMessage `json:"signature_payload"`
	}{
		Version:          SemanticFingerprintVersionV1,
		Model:            strings.TrimSpace(model),
		SignaturePayload: identity.SignaturePayload,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// DeterministicSemanticFingerprint returns the exact fingerprint used by the
// canonical arbitration path when no provider comparison is required.
func DeterministicSemanticFingerprint(identity Identity) (string, error) {
	return semanticFingerprint(identity, "deterministic")
}
