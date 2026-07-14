package opportunityfinding

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/growthradar"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestParseManualDiscoveryPlanKeepsGroundedNewPublicPrompts(t *testing.T) {
	raw := "```json\n" + `{"candidates":[
		{"prompt":"What is the best workflow for social publishing teams?","target_topic":"social publishing","intent":"workflow","audience":"growth teams","why_now":"missing coverage"},
		{"prompt":"Show API_KEY=sk-live-1234567890abcdef","target_topic":"social publishing","intent":"workflow","audience":"growth teams","why_now":"debug"},
		{"prompt":"How should teams migrate Postgres safely?","target_topic":"Postgres migration","intent":"problem_solution","audience":"growth teams","why_now":"public guide"},
		{"prompt":"What is the best workflow for social publishing teams?","target_topic":"social publishing","intent":"workflow","audience":"growth teams","why_now":"duplicate"}
	]}` + "\n```"
	plan, err := parseManualDiscoveryPlan(raw, manualPlanValidation{
		PublicVocabulary: []string{"social publishing", "Postgres migration"},
		ExistingPrompts:  map[string]struct{}{"already covered prompt": {}},
		Limit:            5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 2 {
		t.Fatalf("plan=%+v, want two grounded unique public prompts", plan)
	}
	for _, candidate := range plan {
		if growthradar.ContainsInternalSensitiveTerm(candidate.Prompt) {
			t.Fatalf("planner leaked sensitive prompt: %+v", candidate)
		}
	}
}

func TestParseManualDiscoveryPlanRejectsUngroundedTopics(t *testing.T) {
	_, err := parseManualDiscoveryPlan(`{"candidates":[{"prompt":"Best quantum hosting?","target_topic":"quantum hosting","intent":"buyer_intent","audience":"founders"}]}`, manualPlanValidation{
		PublicVocabulary: []string{"social publishing"}, Limit: 5,
	})
	if err == nil {
		t.Fatal("expected a plan with no confirmed public topic mapping to fail")
	}
}

func TestAIManualDiscoveryPlannerAllowsEvidenceBackedPublicTopic(t *testing.T) {
	projectID := uuid.New()
	workflowID := uuid.New()
	promptSetID := uuid.New()
	store := &manualPlannerStoreStub{
		profile: db.ProductProfile{
			ProjectID: projectID,
			Profile: json.RawMessage(`{
				"positioning":"Unified social media publishing API for developers",
				"features":["Hosted OAuth account connection flows"],
				"icp":["Developers building social publishing into apps"],
				"competitors":["Ayrshare"]
			}`),
		},
		prompts: []db.GeoPrompt{{
			ID:          uuid.New(),
			ProjectID:   projectID,
			PromptSetID: promptSetID,
			PromptText:  "existing prompt",
			Status:      "active",
		}},
	}
	provider := staticCompletionProvider{
		text: `{"candidates":[{"prompt":"Which free social content tools help developers prepare posts before publishing through an API?","target_topic":"free social content tools","intent":"workflow","audience":"Developers building social publishing into apps","why_now":"PostSyncer-style tools hubs show demand."}]}`,
	}
	planner := AIManualDiscoveryPlanner{Store: store, Provider: provider, Evidence: growthradar.EvidenceIndex{PublicTerms: []string{"free social content tools"}}}

	result, err := planner.Plan(context.Background(), ManualDiscoveryPlanRequest{
		ProjectID: projectID, WorkflowID: workflowID, Stage: "foundation", ExistingPrompts: store.prompts,
	})
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}
	if result.Accepted != 1 || len(store.createdPrompts) != 1 {
		t.Fatalf("plan result=%+v created=%+v, want one accepted evidence-backed prompt", result, store.createdPrompts)
	}
	if store.createdPrompts[0].TargetTopic != "free social content tools" {
		t.Fatalf("created target topic = %q", store.createdPrompts[0].TargetTopic)
	}
}

type staticCompletionProvider struct {
	text string
}

func (p staticCompletionProvider) Complete(context.Context, llm.CompletionReq) (llm.CompletionResp, error) {
	return llm.CompletionResp{Text: p.text, Provider: "test", Model: "test", Tokens: 10, CostUSD: 0.001}, nil
}

type manualPlannerStoreStub struct {
	profile        db.ProductProfile
	prompts        []db.GeoPrompt
	createdPrompts []db.GeoPrompt
	calls          []db.AiCallRecord
}

func (s *manualPlannerStoreStub) GetActiveProfile(context.Context, uuid.UUID) (db.ProductProfile, error) {
	return s.profile, nil
}

func (s *manualPlannerStoreStub) ListTopics(context.Context, uuid.UUID) ([]db.Topic, error) {
	return nil, nil
}

func (s *manualPlannerStoreStub) ListSEOOpportunities(context.Context, db.ListSEOOpportunitiesParams) ([]db.SeoOpportunity, error) {
	return nil, nil
}

func (s *manualPlannerStoreStub) CreateGEOPrompt(_ context.Context, arg db.CreateGEOPromptParams) (db.GeoPrompt, error) {
	row := db.GeoPrompt{
		ID:            uuid.New(),
		ProjectID:     arg.ProjectID,
		PromptSetID:   arg.PromptSetID,
		PromptText:    arg.PromptText,
		IntentType:    arg.IntentType,
		TargetPersona: arg.TargetPersona,
		TargetTopic:   arg.TargetTopic,
		Locale:        arg.Locale,
		TargetEngines: append(json.RawMessage{}, arg.TargetEngines...),
		Priority:      arg.Priority,
		Source:        arg.Source,
		Status:        arg.Status,
	}
	s.createdPrompts = append(s.createdPrompts, row)
	return row, nil
}

func (s *manualPlannerStoreStub) TargetGEOPrompt(_ context.Context, arg db.TargetGEOPromptParams) (db.GeoPrompt, error) {
	for i := range s.createdPrompts {
		if s.createdPrompts[i].ID == arg.ID {
			s.createdPrompts[i].TargetedReason = arg.TargetedReason
			return s.createdPrompts[i], nil
		}
	}
	return db.GeoPrompt{ID: arg.ID, ProjectID: arg.ProjectID, TargetedReason: arg.TargetedReason}, nil
}

func (s *manualPlannerStoreStub) CreateAICallRecord(_ context.Context, arg db.CreateAICallRecordParams) (db.AiCallRecord, error) {
	row := db.AiCallRecord{
		ID:                 uuid.New(),
		ProjectID:          arg.ProjectID,
		RunID:              arg.RunID,
		Stage:              arg.Stage,
		LinkedObjectType:   arg.LinkedObjectType,
		LinkedObjectID:     arg.LinkedObjectID,
		Provider:           arg.Provider,
		Model:              arg.Model,
		PromptVersion:      arg.PromptVersion,
		RequestFingerprint: arg.RequestFingerprint,
		Status:             arg.Status,
		ParentCallID:       arg.ParentCallID,
		CausedByCallID:     arg.CausedByCallID,
		ProviderCalled:     false,
	}
	s.calls = append(s.calls, row)
	return row, nil
}

func (s *manualPlannerStoreStub) CreateSkippedAICallRecord(context.Context, db.CreateSkippedAICallRecordParams) (db.AiCallRecord, error) {
	row := db.AiCallRecord{ID: uuid.New(), Status: "skipped"}
	s.calls = append(s.calls, row)
	return row, nil
}

func (s *manualPlannerStoreStub) FinishAICallRecord(_ context.Context, arg db.FinishAICallRecordParams) (db.AiCallRecord, error) {
	row := db.AiCallRecord{ID: arg.ID, ProjectID: arg.ProjectID, Status: arg.Status, ErrorCode: arg.ErrorCode, CostUsd: arg.CostUsd}
	return row, nil
}

func (s *manualPlannerStoreStub) FinishCanonicalAICallFenced(_ context.Context, arg db.FinishCanonicalAICallFencedParams) (db.AiCallRecord, error) {
	return db.AiCallRecord{
		ID:               arg.ID,
		ProjectID:        arg.ProjectID,
		Status:           arg.Status,
		ErrorCode:        arg.ErrorCode,
		Provider:         valueOrEmpty(arg.ResolvedProvider),
		Model:            valueOrEmpty(arg.ResolvedModel),
		PromptTokens:     arg.PromptTokens,
		CompletionTokens: arg.CompletionTokens,
		TotalTokens:      arg.TotalTokens,
		CostUsd:          arg.CostUsd,
		ProviderCalled:   true,
	}, nil
}

func (s *manualPlannerStoreStub) MarkAICallProviderStarted(_ context.Context, arg db.MarkAICallProviderStartedParams) (db.AiCallRecord, error) {
	return db.AiCallRecord{ID: arg.ID, ProjectID: arg.ProjectID, Status: "running", ProviderCalled: true, Model: valueOrEmpty(arg.ResolvedModel)}, nil
}

func (s *manualPlannerStoreStub) ReclassifyAICallRecordOutputFailure(_ context.Context, arg db.ReclassifyAICallRecordOutputFailureParams) (db.AiCallRecord, error) {
	return db.AiCallRecord{ID: arg.ID, ProjectID: arg.ProjectID, Status: "failed", ErrorCode: arg.ErrorCode}, nil
}

func (s *manualPlannerStoreStub) GetAICallRecord(context.Context, db.GetAICallRecordParams) (db.AiCallRecord, error) {
	return db.AiCallRecord{ID: uuid.New()}, nil
}

func (s *manualPlannerStoreStub) GetLatestAICallForRequest(context.Context, db.GetLatestAICallForRequestParams) (db.AiCallRecord, error) {
	return db.AiCallRecord{}, pgx.ErrNoRows
}

func (s *manualPlannerStoreStub) AggregateAICallsForObject(context.Context, db.AggregateAICallsForObjectParams) (db.AggregateAICallsForObjectRow, error) {
	return db.AggregateAICallsForObjectRow{}, nil
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
