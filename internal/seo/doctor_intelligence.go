package seo

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
	"strings"

	"github.com/citeloop/citeloop/internal/aicalls"
	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	doctorDiagnosisPromptVersion       = "doctor-diagnosis-priority-v1"
	doctorDiagnosisAICallStage         = "doctor_diagnosis"
	doctorDiagnosisCandidateLimit      = 25
	doctorContextSnapshotByteLimit     = 12 * 1024
	doctorMaximumImpactMultiplier      = 1.8
	doctorMaximumAIImpactMultiplier    = 1.15
	doctorAIStatusDisabled             = "disabled"
	doctorAIStatusUnavailable          = "provider_unavailable"
	doctorAIStatusApplied              = "applied"
	doctorAIStatusInvalid              = "invalid_response"
	doctorAIStatusFailed               = "provider_failed"
	doctorAIStatusNoCandidates         = "no_candidates"
	doctorDiagnosisPlannedProviderName = "tokengate"
)

type doctorPagePriorityInput struct {
	NormalizedPageURL    string  `json:"normalized_page_url"`
	GSCClicks28D         float64 `json:"gsc_clicks_28d"`
	GSCImpressions28D    float64 `json:"gsc_impressions_28d"`
	GA4Sessions28D       float64 `json:"ga4_sessions_28d"`
	GA4Engaged28D        float64 `json:"ga4_engaged_sessions_28d"`
	GA4KeyEvents28D      float64 `json:"ga4_key_events_28d"`
	EvidenceFreshThrough string  `json:"evidence_fresh_through,omitempty"`
}

type doctorProductContextSnapshot struct {
	Version int32           `json:"version,omitempty"`
	Profile json.RawMessage `json:"profile,omitempty"`
}

type doctorIntelligenceInputs struct {
	Priority       []doctorPagePriorityInput
	ProductContext doctorProductContextSnapshot
	Coverage       []doctorCheckCoverage
}

type doctorDiagnosisAIState struct {
	Status            string `json:"status"`
	Degraded          bool   `json:"degraded"`
	AppliedPriorities int    `json:"applied_priorities"`
	AICallID          string `json:"ai_call_id,omitempty"`
}

type doctorAICallStart struct {
	ProjectID          uuid.UUID
	RunID              uuid.UUID
	Provider           string
	Model              string
	PromptVersion      string
	RequestFingerprint string
}

type doctorAICallFinish struct {
	ID               uuid.UUID
	ProjectID        uuid.UUID
	Status           string
	ErrorCode        string
	Provider         string
	Model            string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CostUSD          float64
}

type doctorAICallLedger interface {
	StartDoctorAICall(context.Context, doctorAICallStart) (uuid.UUID, doctorAICallAttempt, error)
	FinishDoctorAICall(context.Context, doctorAICallFinish) error
}

type doctorAICallAttempt interface {
	llm.AttemptObserver
	Started() bool
}

type doctorPostgresAILedger struct{ q *db.Queries }

func (l doctorPostgresAILedger) StartDoctorAICall(ctx context.Context, call doctorAICallStart) (uuid.UUID, doctorAICallAttempt, error) {
	row, err := l.q.CreateAICallRecord(ctx, db.CreateAICallRecordParams{
		ProjectID: call.ProjectID, RunID: uuidToPG(call.RunID), Stage: doctorDiagnosisAICallStage,
		LinkedObjectType: "seo_doctor_run", LinkedObjectID: call.RunID,
		Provider: call.Provider, Model: call.Model, PromptVersion: call.PromptVersion,
		RequestFingerprint: call.RequestFingerprint, Status: "queued",
	})
	if err != nil {
		return uuid.Nil, nil, err
	}
	return row.ID, aicalls.NewExistingAttemptObserver(l.q, call.ProjectID, row.ID), nil
}

func (l doctorPostgresAILedger) FinishDoctorAICall(ctx context.Context, call doctorAICallFinish) error {
	var errorCode *string
	if strings.TrimSpace(call.ErrorCode) != "" {
		errorCode = &call.ErrorCode
	}
	_, err := l.q.FinishCanonicalAICallFenced(ctx, db.FinishCanonicalAICallFencedParams{
		Status: call.Status, ErrorCode: errorCode,
		ResolvedProvider: optionalDoctorString(call.Provider), ResolvedModel: optionalDoctorString(call.Model),
		PromptTokens: doctorBoundedInt32(call.PromptTokens), CompletionTokens: doctorBoundedInt32(call.CompletionTokens), TotalTokens: doctorBoundedInt32(call.TotalTokens),
		CostUsd: pgutil.Numeric(max(call.CostUSD, 0)), ID: call.ID, ProjectID: call.ProjectID,
	})
	return err
}

type doctorDiagnosisAIRequest struct {
	ProjectID  uuid.UUID
	RunID      uuid.UUID
	Authorized bool
	Candidates []doctorFindingCandidate
	Context    json.RawMessage
	Provider   llm.Provider
	Model      string
	Ledger     doctorAICallLedger
}

type doctorDiagnosisPriority struct {
	FindingKey   string   `json:"finding_key"`
	Priority     string   `json:"priority"`
	Reason       string   `json:"reason"`
	EvidenceKeys []string `json:"evidence_keys"`
}

type doctorDiagnosisOutput struct {
	Priorities []doctorDiagnosisPriority `json:"priorities"`
}

func doctorDiagnosisAIAuthorized(cfg config.ProjectConfig, trigger DoctorTrigger) bool {
	switch trigger {
	case DoctorTriggerManual:
		return cfg.AllowsDoctorAI(config.DoctorAITriggerDiagnosisUser)
	case DoctorTriggerWeekly, DoctorTriggerPostPublish, DoctorTriggerOnboarding:
		return cfg.AllowsDoctorAI(config.DoctorAITriggerDiagnosisScheduler)
	default:
		return false
	}
}

func (s Service) loadDoctorIntelligenceInputs(ctx context.Context, projectID uuid.UUID) doctorIntelligenceInputs {
	out := doctorIntelligenceInputs{Priority: []doctorPagePriorityInput{}, Coverage: []doctorCheckCoverage{}}
	rows, metricsErr := s.Q.ListDoctorPagePriorityInputs(ctx, db.ListDoctorPagePriorityInputsParams{ProjectID: projectID, LimitRows: 50})
	if metricsErr == nil {
		for _, row := range rows {
			input := doctorPagePriorityInput{
				NormalizedPageURL: row.NormalizedPageUrl,
				GSCClicks28D:      doctorNumericFloat(row.GscClicks28d), GSCImpressions28D: doctorNumericFloat(row.GscImpressions28d),
				GA4Sessions28D: doctorNumericFloat(row.Ga4Sessions28d), GA4Engaged28D: doctorNumericFloat(row.Ga4EngagedSessions28d),
				GA4KeyEvents28D: doctorNumericFloat(row.Ga4KeyEvents28d),
			}
			if row.EvidenceFreshThrough.Valid {
				input.EvidenceFreshThrough = row.EvidenceFreshThrough.Time.Format("2006-01-02")
			}
			out.Priority = append(out.Priority, input)
		}
	}
	integrations, integrationErr := s.Q.ListSEOIntegrations(ctx, projectID)
	gscAvailable := metricsErr == nil && integrationErr == nil && isProviderConnected(integrations, ProviderGSC)
	ga4Available := metricsErr == nil && integrationErr == nil && isProviderConnected(integrations, ProviderGA4)
	gscMarker := "gsc_unavailable"
	if gscAvailable {
		gscMarker = "no_recent_gsc_evidence"
	}
	ga4Marker := "ga4_unavailable"
	if ga4Available {
		ga4Marker = "no_recent_ga4_evidence"
	}
	out.Coverage = append(out.Coverage,
		doctorIntelligenceCoverage("gsc_priority_context", gscMarker, out.Priority, func(input doctorPagePriorityInput) bool {
			return gscAvailable && (input.GSCClicks28D > 0 || input.GSCImpressions28D > 0)
		}),
		doctorIntelligenceCoverage("ga4_priority_context", ga4Marker, out.Priority, func(input doctorPagePriorityInput) bool {
			return ga4Available && (input.GA4Sessions28D > 0 || input.GA4Engaged28D > 0 || input.GA4KeyEvents28D > 0)
		}),
	)
	if profile, err := s.Q.GetActiveProfile(ctx, projectID); err == nil {
		out.ProductContext = doctorProductContextSnapshot{Version: profile.Version, Profile: profile.Profile}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		out.Coverage = append(out.Coverage, doctorCheckCoverage{
			Check: "product_context", CheckedURLs: []string{}, PassedURLs: []string{}, FailedURLs: []string{}, SkippedURLs: []string{"product_context_unavailable"},
		})
	}
	return out
}

func (s Service) doctorDiagnosisAuthority(ctx context.Context, projectID uuid.UUID, trigger DoctorTrigger) (bool, bool) {
	project, err := s.Q.GetProject(ctx, projectID)
	if err != nil {
		return false, true
	}
	cfg, err := config.Parse(project.Config)
	if err != nil {
		return false, true
	}
	return doctorDiagnosisAIAuthorized(cfg, trigger), false
}

func applyDoctorPagePriorityInputs(candidates []doctorFindingCandidate, inputs []doctorPagePriorityInput) []doctorFindingCandidate {
	byURL := make(map[string]doctorPagePriorityInput, len(inputs))
	for _, input := range inputs {
		url := strings.TrimSpace(input.NormalizedPageURL)
		if url != "" {
			byURL[url] = input
		}
	}
	out := append([]doctorFindingCandidate(nil), candidates...)
	for i := range out {
		var impact doctorPagePriorityInput
		matched := false
		candidateURLs := sortedUniqueStrings(append(append([]string(nil), out[i].NormalizedURLs...), out[i].AffectedURLs...))
		for _, rawURL := range candidateURLs {
			if candidateImpact, ok := byURL[strings.TrimSpace(rawURL)]; ok {
				impact = mergeDoctorPagePriorityInput(impact, candidateImpact)
				matched = true
			}
		}
		if !matched {
			continue
		}
		if out[i].Evidence == nil {
			out[i].Evidence = map[string]any{}
		} else {
			out[i].Evidence = cloneDoctorEvidence(out[i].Evidence)
		}
		impactEvidence := map[string]any{
			"gsc_clicks_28d": impact.GSCClicks28D, "gsc_impressions_28d": impact.GSCImpressions28D,
			"ga4_sessions_28d": impact.GA4Sessions28D, "ga4_engaged_sessions_28d": impact.GA4Engaged28D,
			"ga4_key_events_28d": impact.GA4KeyEvents28D, "completion_contract": "immediate_evidence_only",
		}
		if impact.EvidenceFreshThrough != "" {
			impactEvidence["evidence_fresh_through"] = impact.EvidenceFreshThrough
		}
		out[i].Evidence["impact_context"] = impactEvidence
		base := out[i].ImportanceMultiplier
		if base <= 0 {
			base = 1
		}
		boost := math.Log10(impact.GSCImpressions28D+1)/10 + math.Log10(impact.GA4Sessions28D+1)/10
		out[i].ImportanceMultiplier = math.Min(doctorMaximumImpactMultiplier, base*(1+boost))
	}
	return out
}

func attachDoctorContextSnapshot(candidates []doctorFindingCandidate, contextSnapshot doctorProductContextSnapshot) []doctorFindingCandidate {
	out := append([]doctorFindingCandidate(nil), candidates...)
	profile := boundedDoctorJSON(contextSnapshot.Profile, doctorContextSnapshotByteLimit)
	for i := range out {
		if out[i].Evidence == nil {
			out[i].Evidence = map[string]any{}
		} else {
			out[i].Evidence = cloneDoctorEvidence(out[i].Evidence)
		}
		snapshot := map[string]any{"product_context_version": contextSnapshot.Version}
		if profile != nil {
			snapshot["product_context"] = profile
		}
		if impact, ok := out[i].Evidence["impact_context"]; ok {
			snapshot["performance_evidence"] = impact
		}
		out[i].Evidence["context_snapshot"] = snapshot
	}
	return out
}

func runDoctorDiagnosisAI(ctx context.Context, req doctorDiagnosisAIRequest) ([]doctorFindingCandidate, doctorDiagnosisAIState, error) {
	unchanged := append([]doctorFindingCandidate(nil), req.Candidates...)
	if !req.Authorized {
		return unchanged, doctorDiagnosisAIState{Status: doctorAIStatusDisabled}, nil
	}
	if len(req.Candidates) == 0 {
		return unchanged, doctorDiagnosisAIState{Status: doctorAIStatusNoCandidates}, nil
	}
	if req.Ledger == nil {
		return unchanged, doctorDiagnosisAIState{Status: doctorAIStatusUnavailable, Degraded: true}, errors.New("Doctor AI call ledger is unavailable")
	}
	prompt := doctorDiagnosisPrompt(req.Context, req.Candidates)
	fingerprint := sha256.Sum256([]byte(prompt))
	model := strings.TrimSpace(req.Model)
	callID, attempt, err := req.Ledger.StartDoctorAICall(ctx, doctorAICallStart{
		ProjectID: req.ProjectID, RunID: req.RunID, Provider: doctorDiagnosisPlannedProviderName, Model: model,
		PromptVersion: doctorDiagnosisPromptVersion, RequestFingerprint: hex.EncodeToString(fingerprint[:]),
	})
	if err != nil {
		return unchanged, doctorDiagnosisAIState{Status: doctorAIStatusUnavailable, Degraded: true}, err
	}
	state := doctorDiagnosisAIState{AICallID: callID.String()}
	if req.Provider == nil {
		if finishErr := req.Ledger.FinishDoctorAICall(context.WithoutCancel(ctx), doctorAICallFinish{
			ID: callID, ProjectID: req.ProjectID, Status: "skipped", ErrorCode: "provider_unavailable", Provider: "none", Model: "none",
		}); finishErr != nil {
			return unchanged, doctorDiagnosisAIState{Status: doctorAIStatusUnavailable, Degraded: true}, finishErr
		}
		state.Status, state.Degraded = doctorAIStatusUnavailable, true
		return unchanged, state, nil
	}
	response, providerErr := llm.CompleteObserved(ctx, req.Provider, llm.CompletionReq{
		System: "You are CiteLoop Doctor's evidence-grounded diagnosis reviewer. You may only prioritize supplied findings; never create findings, facts, patches, content, or success claims.",
		Prompt: prompt, Model: model, MaxTokens: 1600, Temperature: 0, JSON: true, DisableProviderFallback: true, AttemptObserver: attempt,
	})
	if !attempt.Started() {
		if finishErr := req.Ledger.FinishDoctorAICall(context.WithoutCancel(ctx), doctorAICallFinish{
			ID: callID, ProjectID: req.ProjectID, Status: "skipped", ErrorCode: "provider_not_called", Provider: doctorDiagnosisPlannedProviderName, Model: model,
		}); finishErr != nil {
			return unchanged, state, errors.Join(providerErr, finishErr)
		}
		state.Status, state.Degraded = doctorAIStatusFailed, true
		return unchanged, state, nil
	}
	if providerErr != nil {
		if finishErr := req.Ledger.FinishDoctorAICall(context.WithoutCancel(ctx), doctorAICallFinish{
			ID: callID, ProjectID: req.ProjectID, Status: "failed", ErrorCode: "provider_failure", Provider: response.Provider, Model: response.Model,
			PromptTokens: response.PromptTokens, CompletionTokens: response.CompletionTokens, TotalTokens: response.Tokens, CostUSD: response.CostUSD,
		}); finishErr != nil {
			return unchanged, state, errors.Join(providerErr, finishErr)
		}
		state.Status, state.Degraded = doctorAIStatusFailed, true
		return unchanged, state, nil
	}
	output, parseErr := parseDoctorDiagnosisOutput(response.Text)
	if parseErr != nil {
		if finishErr := req.Ledger.FinishDoctorAICall(context.WithoutCancel(ctx), doctorAICallFinish{
			ID: callID, ProjectID: req.ProjectID, Status: "failed", ErrorCode: "invalid_response", Provider: response.Provider, Model: response.Model,
			PromptTokens: response.PromptTokens, CompletionTokens: response.CompletionTokens, TotalTokens: response.Tokens, CostUSD: response.CostUSD,
		}); finishErr != nil {
			return unchanged, state, errors.Join(parseErr, finishErr)
		}
		state.Status, state.Degraded = doctorAIStatusInvalid, true
		return unchanged, state, nil
	}
	out, applied := applyDoctorDiagnosisPriorities(req.Candidates, output.Priorities)
	if err := req.Ledger.FinishDoctorAICall(context.WithoutCancel(ctx), doctorAICallFinish{
		ID: callID, ProjectID: req.ProjectID, Status: "ok", Provider: response.Provider, Model: response.Model,
		PromptTokens: response.PromptTokens, CompletionTokens: response.CompletionTokens, TotalTokens: response.Tokens, CostUSD: response.CostUSD,
	}); err != nil {
		state.Status, state.Degraded = doctorAIStatusFailed, true
		return unchanged, state, err
	}
	state.Status, state.AppliedPriorities = doctorAIStatusApplied, applied
	return out, state, nil
}

func doctorDiagnosisPrompt(contextSnapshot json.RawMessage, candidates []doctorFindingCandidate) string {
	type promptCandidate struct {
		FindingKey string         `json:"finding_key"`
		IssueType  string         `json:"issue_type"`
		Severity   string         `json:"severity"`
		URLs       []string       `json:"urls"`
		Evidence   map[string]any `json:"observed_evidence"`
	}
	limit := min(len(candidates), doctorDiagnosisCandidateLimit)
	items := make([]promptCandidate, 0, limit)
	for _, candidate := range candidates[:limit] {
		items = append(items, promptCandidate{
			FindingKey: candidate.FindingKey, IssueType: candidate.IssueType, Severity: candidate.Severity,
			URLs:     sortedUniqueStrings(append(append([]string(nil), candidate.NormalizedURLs...), candidate.AffectedURLs...)),
			Evidence: boundedDoctorEvidence(candidate.Evidence),
		})
	}
	payload := map[string]any{
		"contract": map[string]any{
			"allowed":   "Prioritize existing finding_key values using only observed_evidence and product_context.",
			"forbidden": []string{"new findings", "new facts", "technical changes", "content", "uplift as Doctor completion"},
			"output":    map[string]any{"priorities": []string{"finding_key", "priority(high|medium|low)", "reason", "evidence_keys"}},
		},
		"product_context": boundedDoctorJSON(contextSnapshot, doctorContextSnapshotByteLimit),
		"findings":        items,
	}
	b, _ := json.Marshal(payload)
	return "[[DOCTOR_DIAGNOSIS_PRIORITY]]\n" + string(b)
}

func parseDoctorDiagnosisOutput(text string) (doctorDiagnosisOutput, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(strings.TrimSpace(text)))
	decoder.DisallowUnknownFields()
	var out doctorDiagnosisOutput
	if err := decoder.Decode(&out); err != nil {
		return out, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return out, errors.New("diagnosis response contains trailing JSON")
	}
	if out.Priorities == nil {
		return out, errors.New("diagnosis response priorities are required")
	}
	return out, nil
}

func applyDoctorDiagnosisPriorities(candidates []doctorFindingCandidate, priorities []doctorDiagnosisPriority) ([]doctorFindingCandidate, int) {
	byKey := make(map[string]doctorDiagnosisPriority, len(priorities))
	for _, priority := range priorities {
		key := strings.TrimSpace(priority.FindingKey)
		level := strings.ToLower(strings.TrimSpace(priority.Priority))
		if key == "" || (level != "high" && level != "medium" && level != "low") || strings.TrimSpace(priority.Reason) == "" || len(priority.EvidenceKeys) == 0 {
			continue
		}
		priority.FindingKey, priority.Priority = key, level
		byKey[key] = priority
	}
	out := append([]doctorFindingCandidate(nil), candidates...)
	applied := 0
	for i := range out {
		priority, ok := byKey[out[i].FindingKey]
		if !ok || !doctorPriorityEvidenceKeysValid(out[i].Evidence, priority.EvidenceKeys) {
			continue
		}
		if out[i].Evidence == nil {
			continue
		}
		out[i].Evidence = cloneDoctorEvidence(out[i].Evidence)
		out[i].Evidence["ai_diagnosis_review"] = map[string]any{
			"priority": priority.Priority, "reason": truncateDoctorText(priority.Reason, 500),
			"evidence_keys": sortedUniqueStrings(priority.EvidenceKeys), "prompt_version": doctorDiagnosisPromptVersion,
		}
		base := out[i].ImportanceMultiplier
		if base <= 0 {
			base = 1
		}
		boost := 1.0
		if priority.Priority == "high" {
			boost = doctorMaximumAIImpactMultiplier
		} else if priority.Priority == "medium" {
			boost = 1.05
		}
		out[i].ImportanceMultiplier = math.Min(doctorMaximumImpactMultiplier, base*boost)
		applied++
	}
	return out, applied
}

func doctorPriorityEvidenceKeysValid(evidence map[string]any, keys []string) bool {
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			return false
		}
		if _, ok := evidence[key]; !ok {
			return false
		}
	}
	return true
}

func mergeDoctorPagePriorityInput(a, b doctorPagePriorityInput) doctorPagePriorityInput {
	a.GSCClicks28D += b.GSCClicks28D
	a.GSCImpressions28D += b.GSCImpressions28D
	a.GA4Sessions28D += b.GA4Sessions28D
	a.GA4Engaged28D += b.GA4Engaged28D
	a.GA4KeyEvents28D += b.GA4KeyEvents28D
	if b.EvidenceFreshThrough > a.EvidenceFreshThrough {
		a.EvidenceFreshThrough = b.EvidenceFreshThrough
	}
	return a
}

func boundedDoctorEvidence(evidence map[string]any) map[string]any {
	out := make(map[string]any, min(len(evidence), 30))
	count := 0
	for key, value := range evidence {
		if count >= 30 {
			break
		}
		out[truncateDoctorText(key, 100)] = boundedDoctorValue(value, 0)
		count++
	}
	return out
}

func boundedDoctorValue(value any, depth int) any {
	if depth >= 4 {
		return "[bounded]"
	}
	switch typed := value.(type) {
	case string:
		return truncateDoctorText(typed, 500)
	case json.RawMessage:
		return boundedDoctorJSON(typed, 2048)
	case map[string]any:
		out := map[string]any{}
		count := 0
		for key, child := range typed {
			if count >= 30 {
				break
			}
			out[truncateDoctorText(key, 100)] = boundedDoctorValue(child, depth+1)
			count++
		}
		return out
	case []any:
		limit := min(len(typed), 20)
		out := make([]any, 0, limit)
		for _, child := range typed[:limit] {
			out = append(out, boundedDoctorValue(child, depth+1))
		}
		return out
	case []string:
		limit := min(len(typed), 20)
		out := make([]string, 0, limit)
		for _, child := range typed[:limit] {
			out = append(out, truncateDoctorText(child, 500))
		}
		return out
	default:
		return typed
	}
}

func boundedDoctorJSON(raw json.RawMessage, byteLimit int) any {
	if len(raw) == 0 || byteLimit <= 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	value = boundedDoctorValue(value, 0)
	b, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	if len(b) <= byteLimit {
		return value
	}
	return map[string]any{"sha256": doctorJSONFingerprint(raw), "bounded": true}
}

func doctorJSONFingerprint(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func cloneDoctorEvidence(input map[string]any) map[string]any {
	out := make(map[string]any, len(input)+2)
	for key, value := range input {
		out[key] = value
	}
	return out
}

func truncateDoctorText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

func optionalDoctorString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func doctorNumericFloat(value pgtype.Numeric) float64 {
	converted, err := value.Float64Value()
	if err != nil || !converted.Valid || converted.Float64 < 0 {
		return 0
	}
	return converted.Float64
}

func doctorBoundedInt32(value int) int32 {
	if value <= 0 {
		return 0
	}
	if value > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(value)
}

func doctorIntelligenceCoverage(check, unavailableMarker string, inputs []doctorPagePriorityInput, hasData func(doctorPagePriorityInput) bool) doctorCheckCoverage {
	coverage := doctorCheckCoverage{Check: check, CheckedURLs: []string{}, PassedURLs: []string{}, FailedURLs: []string{}, SkippedURLs: []string{}}
	for _, input := range inputs {
		if !hasData(input) {
			continue
		}
		coverage.CheckedURLs = append(coverage.CheckedURLs, input.NormalizedPageURL)
		coverage.PassedURLs = append(coverage.PassedURLs, input.NormalizedPageURL)
	}
	if len(coverage.CheckedURLs) == 0 {
		coverage.SkippedURLs = append(coverage.SkippedURLs, unavailableMarker)
	}
	normalizeDoctorCoverage(&coverage)
	return coverage
}

func doctorAIReviewCoverage(state doctorDiagnosisAIState) doctorCheckCoverage {
	coverage := doctorCheckCoverage{Check: "doctor_ai_review", CheckedURLs: []string{}, PassedURLs: []string{}, FailedURLs: []string{}, SkippedURLs: []string{}}
	if state.Status == doctorAIStatusApplied {
		coverage.CheckedURLs = append(coverage.CheckedURLs, "diagnosis_call:"+state.AICallID)
		coverage.PassedURLs = append(coverage.PassedURLs, "diagnosis_call:"+state.AICallID)
	} else if state.Status != doctorAIStatusNoCandidates {
		coverage.SkippedURLs = append(coverage.SkippedURLs, "diagnosis_ai:"+state.Status)
	}
	normalizeDoctorCoverage(&coverage)
	return coverage
}

func validateDoctorIntelligenceInputs(inputs []doctorPagePriorityInput) error {
	if len(inputs) > 50 {
		return fmt.Errorf("doctor page priority inputs exceed bound: %d", len(inputs))
	}
	return nil
}
