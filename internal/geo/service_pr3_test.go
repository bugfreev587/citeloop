package geo

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
)

func TestAnalyzeObservationsCreatesIdempotentGEOOpportunitiesAndBriefs(t *testing.T) {
	projectID := uuid.New()
	runID := uuid.New()
	promptID := uuid.New()
	store := &geoStoreStub{
		runID: runID,
		prompts: []db.GeoPrompt{{
			ID:          promptID,
			ProjectID:   projectID,
			PromptText:  "best social scheduling tools",
			IntentType:  "category_recommendation",
			TargetTopic: "social scheduling",
			Status:      "active",
			Priority:    8,
		}},
		observations: []db.GeoObservation{
			{
				ID:                   uuid.New(),
				ProjectID:            projectID,
				PromptID:             pgUUID(promptID),
				Engine:               "Perplexity",
				SourceType:           ProviderManualFixture,
				BrandMentioned:       false,
				ProjectCitationCount: 0,
				CompetitorCitations:  json.RawMessage(`["Buffer"]`),
				CitedUrls:            json.RawMessage(`["https://buffer.com/resources"]`),
				ObservationState:     "observed",
				Confidence:           ConfidenceMedium,
			},
			{
				ID:                   uuid.New(),
				ProjectID:            projectID,
				PromptID:             pgUUID(promptID),
				Engine:               "Perplexity",
				SourceType:           ProviderManualFixture,
				BrandMentioned:       true,
				ProjectCitationCount: 0,
				CompetitorCitations:  json.RawMessage(`[]`),
				CitedUrls:            json.RawMessage(`[]`),
				ObservationState:     "observed",
				Confidence:           ConfidenceMedium,
			},
		},
	}

	service := Service{Q: store, Now: func() time.Time { return time.Date(2026, 6, 8, 14, 0, 0, 0, time.UTC) }}
	result, err := service.AnalyzeObservations(context.Background(), projectID, AnalyzeObservationsRequest{Limit: 50})
	if err != nil {
		t.Fatalf("AnalyzeObservations error: %v", err)
	}
	if len(result.Opportunities) != 2 || len(result.AssetBriefs) != 2 {
		t.Fatalf("opportunities=%d briefs=%d, want 2 and 2", len(result.Opportunities), len(result.AssetBriefs))
	}
	again, err := service.AnalyzeObservations(context.Background(), projectID, AnalyzeObservationsRequest{Limit: 50})
	if err != nil {
		t.Fatalf("AnalyzeObservations second run error: %v", err)
	}
	if len(store.opportunities) != 2 || len(store.assetBriefs) != 2 {
		t.Fatalf("after rerun store opportunities=%d briefs=%d, want idempotent 2 and 2; second result %+v", len(store.opportunities), len(store.assetBriefs), again)
	}
	if !hasOpportunityType(result.Opportunities, "geo_competitor_cited_project_absent") {
		t.Fatalf("missing competitor gap opportunity: %+v", result.Opportunities)
	}
	if !hasOpportunityType(result.Opportunities, "geo_project_mentioned_without_citation") {
		t.Fatalf("missing mention without citation opportunity: %+v", result.Opportunities)
	}
}

func TestAcceptAssetBriefCreatesTopic(t *testing.T) {
	projectID := uuid.New()
	briefID := uuid.New()
	opportunityID := uuid.New()
	store := &geoStoreStub{
		assetBriefs: []db.GeoAssetBrief{{
			ID:                 briefID,
			ProjectID:          projectID,
			OpportunityID:      opportunityID,
			AssetType:          "comparison_page",
			Status:             "ready_for_review",
			TargetPrompts:      json.RawMessage(`["best social scheduling tools"]`),
			RequiredEvidence:   json.RawMessage(`["first-party comparison proof","competitor citation evidence: Buffer"]`),
			RecommendedOutline: json.RawMessage(`["What to compare","Evidence","When to choose UniPost"]`),
			InternalLinkPlan:   json.RawMessage(`["/blog/social-scheduling"]`),
			PublicationSurface: "blog",
		}},
	}

	result, err := Service{Q: store}.AcceptGEOAssetBrief(context.Background(), projectID, briefID)
	if err != nil {
		t.Fatalf("AcceptGEOAssetBrief error: %v", err)
	}
	if result.Brief.Status != "converted" {
		t.Fatalf("brief status = %q, want converted", result.Brief.Status)
	}
	if result.Topic.ID == uuid.Nil || result.Topic.Status != "backlog" || result.Topic.ProjectID != projectID {
		t.Fatalf("topic = %+v, want backlog topic", result.Topic)
	}
	if len(store.createdTopics) != 1 {
		t.Fatalf("created topics = %d, want 1", len(store.createdTopics))
	}
	if result.Topic.Angle == nil || *result.Topic.Angle != "comparison_page" {
		t.Fatalf("topic angle = %v, want comparison_page asset type", result.Topic.Angle)
	}
	if result.Topic.Format == nil || *result.Topic.Format != "geo_asset_brief" {
		t.Fatalf("topic format = %v, want geo_asset_brief", result.Topic.Format)
	}
	var metadata struct {
		AssetBriefID       string   `json:"asset_brief_id"`
		Links              []string `json:"links"`
		SourceEvidence     []string `json:"source_evidence"`
		RecommendedOutline []string `json:"recommended_outline"`
	}
	if err := json.Unmarshal(result.Topic.InternalLinks, &metadata); err != nil {
		t.Fatalf("topic internal_links should carry asset metadata object: %v; raw=%s", err, string(result.Topic.InternalLinks))
	}
	if metadata.AssetBriefID != briefID.String() {
		t.Fatalf("metadata asset brief id = %q, want %s", metadata.AssetBriefID, briefID)
	}
	if len(metadata.SourceEvidence) != 2 || metadata.SourceEvidence[0] != "first-party comparison proof" {
		t.Fatalf("metadata source evidence = %#v, want brief evidence", metadata.SourceEvidence)
	}
	if len(metadata.RecommendedOutline) != 3 || metadata.RecommendedOutline[0] != "What to compare" {
		t.Fatalf("metadata outline = %#v, want brief outline", metadata.RecommendedOutline)
	}
	if len(metadata.Links) != 1 || metadata.Links[0] != "/blog/social-scheduling" {
		t.Fatalf("metadata links = %#v, want internal link plan", metadata.Links)
	}
}

func hasOpportunityType(rows []db.UpsertGEOObservationOpportunityRow, kind string) bool {
	for _, row := range rows {
		if row.Type == kind {
			return true
		}
	}
	return false
}

func (s *geoStoreStub) UpsertGEOObservationOpportunity(_ context.Context, arg db.UpsertGEOObservationOpportunityParams) (db.UpsertGEOObservationOpportunityRow, error) {
	for _, row := range s.opportunities {
		if row.ProjectID == arg.ProjectID && row.Type == arg.Type && row.NormalizedPageUrl == arg.NormalizedPageUrl && sameQuery(row.Query, arg.Query) {
			return row, nil
		}
	}
	row := db.UpsertGEOObservationOpportunityRow{
		ID:                uuid.New(),
		ProjectID:         arg.ProjectID,
		Type:              arg.Type,
		Status:            arg.Status,
		PriorityScore:     arg.PriorityScore,
		Confidence:        arg.Confidence,
		PageUrl:           arg.PageUrl,
		NormalizedPageUrl: arg.NormalizedPageUrl,
		Query:             arg.Query,
		Evidence:          append(json.RawMessage{}, arg.Evidence...),
		RecommendedAction: arg.RecommendedAction,
		ExpectedImpact:    arg.ExpectedImpact,
		Effort:            arg.Effort,
		RiskLevel:         arg.RiskLevel,
	}
	s.opportunities = append(s.opportunities, row)
	return row, nil
}

func (s *geoStoreStub) CreateGEOAssetBrief(_ context.Context, arg db.CreateGEOAssetBriefParams) (db.GeoAssetBrief, error) {
	for _, row := range s.assetBriefs {
		if row.ProjectID == arg.ProjectID && row.OpportunityID == arg.OpportunityID {
			return row, nil
		}
	}
	id := s.assetBriefID
	if id == uuid.Nil {
		id = uuid.New()
	}
	row := db.GeoAssetBrief{
		ID:                 id,
		ProjectID:          arg.ProjectID,
		OpportunityID:      arg.OpportunityID,
		AssetType:          arg.AssetType,
		Status:             arg.Status,
		TargetPrompts:      append(json.RawMessage{}, arg.TargetPrompts...),
		RequiredEvidence:   append(json.RawMessage{}, arg.RequiredEvidence...),
		RecommendedOutline: append(json.RawMessage{}, arg.RecommendedOutline...),
		InternalLinkPlan:   append(json.RawMessage{}, arg.InternalLinkPlan...),
		PublicationSurface: arg.PublicationSurface,
		CreatedByRunID:     arg.CreatedByRunID,
	}
	s.assetBriefs = append(s.assetBriefs, row)
	return row, nil
}

func (s *geoStoreStub) ListGEOAssetBriefs(context.Context, db.ListGEOAssetBriefsParams) ([]db.GeoAssetBrief, error) {
	return s.assetBriefs, nil
}

func (s *geoStoreStub) GetGEOAssetBriefForProject(_ context.Context, arg db.GetGEOAssetBriefForProjectParams) (db.GeoAssetBrief, error) {
	for _, row := range s.assetBriefs {
		if row.ID == arg.ID && row.ProjectID == arg.ProjectID {
			return row, nil
		}
	}
	return db.GeoAssetBrief{}, nil
}

func (s *geoStoreStub) UpdateGEOAssetBriefStatus(_ context.Context, arg db.UpdateGEOAssetBriefStatusParams) (db.GeoAssetBrief, error) {
	for i := range s.assetBriefs {
		if s.assetBriefs[i].ID == arg.ID && s.assetBriefs[i].ProjectID == arg.ProjectID {
			s.assetBriefs[i].Status = arg.Status
			return s.assetBriefs[i], nil
		}
	}
	row := db.GeoAssetBrief{ID: arg.ID, ProjectID: arg.ProjectID, Status: arg.Status}
	s.assetBriefs = append(s.assetBriefs, row)
	return row, nil
}

func (s *geoStoreStub) CreateTopic(_ context.Context, arg db.CreateTopicParams) (db.Topic, error) {
	row := db.Topic{
		ID:            uuid.New(),
		ProjectID:     arg.ProjectID,
		Channel:       arg.Channel,
		Title:         arg.Title,
		TargetKeyword: arg.TargetKeyword,
		TargetPrompt:  arg.TargetPrompt,
		Angle:         arg.Angle,
		Format:        arg.Format,
		Priority:      arg.Priority,
		InternalLinks: append(json.RawMessage{}, arg.InternalLinks...),
		Status:        arg.Status,
		ScheduledAt:   arg.ScheduledAt,
	}
	s.createdTopics = append(s.createdTopics, row)
	return row, nil
}

func sameQuery(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
