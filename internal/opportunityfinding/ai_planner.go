package opportunityfinding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/citeloop/citeloop/internal/aicalls"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/growthradar"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/google/uuid"
)

const manualDiscoveryPromptVersion = "manual-growth-discovery-v1"

type ManualDiscoveryCandidate struct {
	Prompt      string `json:"prompt"`
	TargetTopic string `json:"target_topic"`
	Intent      string `json:"intent"`
	Audience    string `json:"audience"`
	WhyNow      string `json:"why_now"`
}

type ManualDiscoveryPlanRequest struct {
	ProjectID       uuid.UUID
	WorkflowID      uuid.UUID
	Stage           string
	ExistingPrompts []db.GeoPrompt
	RepairReasons   []string
}

type ManualDiscoveryPlanResult struct {
	Created        []db.GeoPrompt
	Proposed       int
	Accepted       int
	ProviderCalled bool
	CostUSD        float64
	TotalTokens    int
	Repair         bool
}

type ManualDiscoveryPlanner interface {
	Plan(context.Context, ManualDiscoveryPlanRequest) (ManualDiscoveryPlanResult, error)
}

type manualDiscoveryStore interface {
	aicalls.Store
	GetActiveProfile(context.Context, uuid.UUID) (db.ProductProfile, error)
	ListTopics(context.Context, uuid.UUID) ([]db.Topic, error)
	ListSEOOpportunities(context.Context, db.ListSEOOpportunitiesParams) ([]db.SeoOpportunity, error)
	CreateGEOPrompt(context.Context, db.CreateGEOPromptParams) (db.GeoPrompt, error)
	TargetGEOPrompt(context.Context, db.TargetGEOPromptParams) (db.GeoPrompt, error)
}

type AIManualDiscoveryPlanner struct {
	Store    manualDiscoveryStore
	Provider llm.Provider
	Model    string
}

func (p AIManualDiscoveryPlanner) Plan(ctx context.Context, req ManualDiscoveryPlanRequest) (ManualDiscoveryPlanResult, error) {
	result := ManualDiscoveryPlanResult{Repair: len(req.RepairReasons) > 0}
	if p.Store == nil || p.Provider == nil || req.ProjectID == uuid.Nil || req.WorkflowID == uuid.Nil {
		return result, errors.New("manual discovery planner is unavailable")
	}
	if len(req.ExistingPrompts) == 0 {
		return result, errors.New("manual discovery planner requires an active prompt set")
	}
	profile, err := p.Store.GetActiveProfile(ctx, req.ProjectID)
	if err != nil {
		return result, err
	}
	classification := growthradar.ClassifyContext(profile.Profile, growthradar.EvidenceIndex{})
	publicVocabulary := append([]string(nil), classification.AcceptedVocabulary...)
	if len(publicVocabulary) == 0 {
		return result, errors.New("no confirmed public capability is available for discovery")
	}
	topics, err := p.Store.ListTopics(ctx, req.ProjectID)
	if err != nil {
		return result, err
	}
	opportunities, err := p.Store.ListSEOOpportunities(ctx, db.ListSEOOpportunitiesParams{ProjectID: req.ProjectID, LimitRows: 100})
	if err != nil {
		return result, err
	}
	prompt := manualDiscoveryPrompt(req.Stage, publicVocabulary, classification.ConfirmedCompetitors, topics, opportunities, req.ExistingPrompts, req.RepairReasons)
	model := strings.TrimSpace(p.Model)
	if model == "" {
		model = llm.DefaultTokenGateModel
	}
	completionReq := llm.CompletionReq{
		System: "You are CiteLoop's stage-aware growth strategist. Return only valid JSON. Propose public, evidence-testable user questions; never invent product capabilities or include secrets.",
		Prompt: prompt, Model: model, MaxTokens: 1800, Temperature: 0.2, JSON: true,
	}
	completion, err := aicalls.New(p.Store).Complete(ctx, aicalls.Spec{
		ProjectID: req.ProjectID, RunID: req.WorkflowID, Stage: "opportunity_discovery",
		LinkedObjectType: "workflow_event", LinkedObjectID: req.WorkflowID,
		Provider: "runtime_route", Model: model, PromptVersion: manualDiscoveryPromptVersion,
		RequestFingerprint: aicalls.Fingerprint(completionReq),
	}, p.Provider, completionReq)
	result.ProviderCalled = completion.Call.ProviderCalled
	result.CostUSD = completion.Response.CostUSD
	result.TotalTokens = completion.Response.Tokens
	if err != nil {
		return result, err
	}
	existing := make(map[string]struct{}, len(req.ExistingPrompts))
	for _, item := range req.ExistingPrompts {
		existing[normalizePlanText(item.PromptText)] = struct{}{}
	}
	candidates, err := parseManualDiscoveryPlan(completion.Response.Text, manualPlanValidation{PublicVocabulary: publicVocabulary, ExistingPrompts: existing, Limit: 6})
	if err != nil {
		_, _ = aicalls.New(p.Store).FailOutput(context.WithoutCancel(ctx), completion.Call.ID, req.ProjectID, "invalid_discovery_plan")
		return result, err
	}
	result.Proposed = len(candidates)
	promptSetID := req.ExistingPrompts[0].PromptSetID
	for _, candidate := range candidates {
		created, createErr := p.Store.CreateGEOPrompt(ctx, db.CreateGEOPromptParams{
			ProjectID: req.ProjectID, PromptSetID: promptSetID, PromptText: candidate.Prompt,
			IntentType: candidate.Intent, TargetPersona: candidate.Audience, TargetTopic: candidate.TargetTopic,
			Locale: "en-US", TargetEngines: json.RawMessage(`["ChatGPT","Perplexity","Google AI Mode"]`),
			Priority: 10, Source: "ai_growth_planner", Status: "active",
		})
		if createErr != nil {
			return result, createErr
		}
		created, createErr = p.Store.TargetGEOPrompt(ctx, db.TargetGEOPromptParams{
			ProjectID: req.ProjectID, ID: created.ID, TargetedReason: "manual_" + strings.TrimSpace(req.Stage) + "_discovery",
		})
		if createErr != nil {
			return result, createErr
		}
		result.Created = append(result.Created, created)
	}
	result.Accepted = len(result.Created)
	return result, nil
}

type manualPlanValidation struct {
	PublicVocabulary []string
	ExistingPrompts  map[string]struct{}
	Limit            int
}

func parseManualDiscoveryPlan(raw string, validation manualPlanValidation) ([]ManualDiscoveryCandidate, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(strings.TrimSpace(raw), "```")
	}
	var payload struct {
		Candidates []ManualDiscoveryCandidate `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, fmt.Errorf("decode manual discovery plan: %w", err)
	}
	limit := validation.Limit
	if limit <= 0 || limit > 10 {
		limit = 6
	}
	allowedIntent := map[string]bool{
		"category_recommendation": true, "problem_solution": true, "workflow": true, "integration": true,
		"buyer_intent": true, "definition_entity": true, "comparison": true, "alternative": true,
	}
	seen := map[string]struct{}{}
	accepted := make([]ManualDiscoveryCandidate, 0, limit)
	for _, candidate := range payload.Candidates {
		candidate.Prompt = strings.TrimSpace(candidate.Prompt)
		candidate.TargetTopic = strings.TrimSpace(candidate.TargetTopic)
		candidate.Intent = strings.ToLower(strings.TrimSpace(candidate.Intent))
		candidate.Audience = strings.TrimSpace(candidate.Audience)
		candidate.WhyNow = strings.TrimSpace(candidate.WhyNow)
		key := normalizePlanText(candidate.Prompt)
		if key == "" || len(candidate.Prompt) > 300 || growthradar.ContainsInternalSensitiveTerm(candidate.Prompt) || growthradar.ContainsInternalSensitiveTerm(candidate.TargetTopic) || !allowedIntent[candidate.Intent] || !mapsToPublicVocabulary(candidate.TargetTopic, validation.PublicVocabulary) {
			continue
		}
		if _, exists := validation.ExistingPrompts[key]; exists {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		accepted = append(accepted, candidate)
		if len(accepted) == limit {
			break
		}
	}
	if len(accepted) == 0 {
		return nil, errors.New("AI discovery returned no grounded new public prompts")
	}
	return accepted, nil
}

func mapsToPublicVocabulary(topic string, vocabulary []string) bool {
	topic = normalizePlanText(topic)
	for _, value := range vocabulary {
		public := normalizePlanText(value)
		if public != "" && (topic == public || strings.Contains(topic, public) || strings.Contains(public, topic)) {
			return true
		}
	}
	return false
}

func normalizePlanText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func manualDiscoveryPrompt(stage string, vocabulary, competitors []string, topics []db.Topic, opportunities []db.SeoOpportunity, prompts []db.GeoPrompt, repairReasons []string) string {
	covered := make([]string, 0, len(topics)+len(opportunities)+len(prompts))
	for _, topic := range topics {
		covered = append(covered, topic.Title)
	}
	for _, opportunity := range opportunities {
		if opportunity.Query != nil {
			covered = append(covered, *opportunity.Query)
		}
	}
	for _, prompt := range prompts {
		covered = append(covered, prompt.PromptText)
	}
	covered = publicPlanStrings(covered, 60)
	vocabulary = publicPlanStrings(vocabulary, 40)
	competitors = publicPlanStrings(competitors, 20)
	sort.Strings(repairReasons)
	payload, _ := json.Marshal(map[string]any{
		"growth_stage": strings.TrimSpace(stage), "confirmed_public_vocabulary": vocabulary,
		"confirmed_competitors": competitors, "already_covered_or_handled": covered,
		"repair_rejection_codes": repairReasons,
		"requirements": []string{
			"Return 4-6 materially distinct questions not already covered.",
			"target_topic must map verbatim or closely to confirmed_public_vocabulary.",
			"Foundation prioritizes missing core capability, use-case, integration, comparison and citable-source coverage; Traction prioritizes observed demand; Scale prioritizes proven expansion; Optimize prioritizes refresh and competitive gaps.",
			"Use only supported intents: category_recommendation, problem_solution, workflow, integration, buyer_intent, definition_entity, comparison, alternative.",
		},
		"response_schema": map[string]any{"candidates": []map[string]string{{"prompt": "...", "target_topic": "...", "intent": "workflow", "audience": "...", "why_now": "..."}}},
	})
	return string(payload)
}

func publicPlanStrings(values []string, limit int) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, min(len(values), limit))
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := normalizePlanText(value)
		if value == "" || growthradar.ContainsInternalSensitiveTerm(value) {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
		if len(result) == limit {
			break
		}
	}
	return result
}
