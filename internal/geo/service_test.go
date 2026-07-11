package geo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestServicePersistsRobotsDisallowedEvidenceWithoutGrowthOpportunity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("User-agent: OAI-SearchBot\nDisallow: /blocked\n"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>visible content</body></html>"))
	}))
	defer server.Close()

	projectID := uuid.New()
	articleID := uuid.New()
	runID := uuid.New()
	canonicalURL := server.URL + "/blocked/page"
	store := &geoStoreStub{
		property: db.SeoProperty{ID: uuid.New(), ProjectID: projectID, SiteUrl: server.URL},
		articles: []db.Article{{
			ID:           articleID,
			ProjectID:    projectID,
			CanonicalUrl: &canonicalURL,
		}},
		runID: runID,
	}

	result, err := Service{
		Q:          store,
		HTTPClient: server.Client(),
		Now:        func() time.Time { return time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC) },
	}.RunCrawlerAudit(context.Background(), projectID, CrawlerAuditRequest{TargetUserAgents: []string{"OAI-SearchBot"}})
	if err != nil {
		t.Fatalf("RunCrawlerAudit error: %v", err)
	}

	if !store.started || store.finishedStatus != "ok" {
		t.Fatalf("run started=%v finished=%q, want started and ok", store.started, store.finishedStatus)
	}
	if result.CreatedBlockers != 0 || store.opportunityCount != 0 {
		t.Fatalf("growth blockers result=%d store=%d, want 0; crawler repair belongs to Doctor", result.CreatedBlockers, store.opportunityCount)
	}
	if len(store.snapshots) == 0 {
		t.Fatal("no snapshots persisted")
	}
	found := false
	for _, snapshot := range store.snapshots {
		if snapshot.TargetUserAgent == "OAI-SearchBot" && snapshot.PageUrl == canonicalURL {
			found = true
			if snapshot.NormalizedPageUrl == "" {
				t.Fatal("normalized page URL is empty")
			}
			if snapshot.EvidenceType != EvidenceRobotsStatic || snapshot.Confidence != ConfidenceHigh || !snapshot.Inferred {
				t.Fatalf("snapshot = %+v, want robots_static high inferred", snapshot)
			}
		}
	}
	if !found {
		t.Fatalf("canonical snapshot for OAI-SearchBot not found: %+v", store.snapshots)
	}
}

func TestServiceDoesNotCreateBlockerForInferredWAFWarning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("User-agent: OAI-SearchBot\nAllow: /\n"))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("Cloudflare captcha challenge"))
	}))
	defer server.Close()

	projectID := uuid.New()
	store := &geoStoreStub{
		property: db.SeoProperty{ID: uuid.New(), ProjectID: projectID, SiteUrl: server.URL},
		runID:    uuid.New(),
	}

	result, err := Service{
		Q:          store,
		HTTPClient: server.Client(),
		Now:        func() time.Time { return time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC) },
	}.RunCrawlerAudit(context.Background(), projectID, CrawlerAuditRequest{TargetUserAgents: []string{"OAI-SearchBot"}})
	if err != nil {
		t.Fatalf("RunCrawlerAudit error: %v", err)
	}

	if result.CreatedBlockers != 0 || store.opportunityCount != 0 {
		t.Fatalf("blockers result=%d store=%d, want 0", result.CreatedBlockers, store.opportunityCount)
	}
	if len(store.snapshots) == 0 {
		t.Fatal("no snapshots persisted")
	}
	if store.snapshots[0].AccessState != AccessChallenge || !store.snapshots[0].Inferred || store.snapshots[0].Confidence != ConfidenceMedium {
		t.Fatalf("snapshot = %+v, want inferred medium challenge", store.snapshots[0])
	}
}

type geoStoreStub struct {
	property         db.SeoProperty
	articles         []db.Article
	runID            uuid.UUID
	promptSetID      uuid.UUID
	started          bool
	finishedStatus   string
	finishedOutput   json.RawMessage
	snapshots        []db.AiCrawlerAccessSnapshot
	opportunityCount int
	latestSnapshots  []db.AiCrawlerAccessSnapshot
	profile          db.ProductProfile
	profileErr       error
	topics           []db.Topic
	promptSets       []db.GeoPromptSet
	prompts          []db.GeoPrompt
	competitors      []db.GeoCompetitor
	surfaces         []db.GeoExternalSurface
	observations     []db.GeoObservation
	visibilityScores []db.GeoVisibilityScore
	opportunities    []db.UpsertGEOObservationOpportunityRow
	assetBriefID     uuid.UUID
	assetBriefs      []db.GeoAssetBrief
	createdTopics    []db.Topic
}

func (s *geoStoreStub) GetSEOPropertyForProject(_ context.Context, projectID uuid.UUID) (db.SeoProperty, error) {
	s.property.ProjectID = projectID
	return s.property, nil
}

func (s *geoStoreStub) ListPublishedCanonicalArticlesForSEO(context.Context, uuid.UUID) ([]db.Article, error) {
	return s.articles, nil
}

func (s *geoStoreStub) StartGEORun(_ context.Context, arg db.StartGEORunParams) (db.GeoRun, error) {
	s.started = true
	return db.GeoRun{ID: s.runID, ProjectID: arg.ProjectID, Agent: arg.Agent, Status: "degraded", Provider: arg.Provider, StartedAt: arg.StartedAt, Input: arg.Input}, nil
}

func (s *geoStoreStub) FinishGEORun(_ context.Context, arg db.FinishGEORunParams) (db.GeoRun, error) {
	s.finishedStatus = arg.Status
	s.finishedOutput = append(json.RawMessage{}, arg.Output...)
	return db.GeoRun{ID: arg.ID, ProjectID: arg.ProjectID, Status: arg.Status, FinishedAt: arg.FinishedAt, Output: arg.Output}, nil
}

func (s *geoStoreStub) UpsertAICrawlerAccessSnapshot(_ context.Context, arg db.UpsertAICrawlerAccessSnapshotParams) (db.AiCrawlerAccessSnapshot, error) {
	row := db.AiCrawlerAccessSnapshot{
		ID:                uuid.New(),
		ProjectID:         arg.ProjectID,
		RunID:             arg.RunID,
		PageUrl:           arg.PageUrl,
		NormalizedPageUrl: arg.NormalizedPageUrl,
		TargetUserAgent:   arg.TargetUserAgent,
		ProbeUserAgent:    arg.ProbeUserAgent,
		EvidenceType:      arg.EvidenceType,
		RobotsState:       arg.RobotsState,
		HttpStatus:        arg.HttpStatus,
		AccessState:       arg.AccessState,
		Confidence:        arg.Confidence,
		Inferred:          arg.Inferred,
		MetaRobotsState:   arg.MetaRobotsState,
		SitemapState:      arg.SitemapState,
		BodyExtractable:   arg.BodyExtractable,
		RawDetails:        arg.RawDetails,
		CheckedAt:         arg.CheckedAt,
	}
	s.snapshots = append(s.snapshots, row)
	return row, nil
}

func (s *geoStoreStub) ListLatestAICrawlerAccessSnapshots(context.Context, uuid.UUID) ([]db.AiCrawlerAccessSnapshot, error) {
	return s.latestSnapshots, nil
}

func (s *geoStoreStub) UpsertCrawlerAccessOpportunity(_ context.Context, arg db.UpsertCrawlerAccessOpportunityParams) (db.UpsertCrawlerAccessOpportunityRow, error) {
	s.opportunityCount++
	return db.UpsertCrawlerAccessOpportunityRow{
		ID:                uuid.New(),
		ProjectID:         arg.ProjectID,
		Type:              arg.Type,
		Status:            arg.Status,
		PriorityScore:     arg.PriorityScore,
		Confidence:        arg.Confidence,
		PageUrl:           arg.PageUrl,
		NormalizedPageUrl: arg.NormalizedPageUrl,
		Evidence:          append(json.RawMessage{}, arg.Evidence...),
		RecommendedAction: arg.RecommendedAction,
		ExpectedImpact:    arg.ExpectedImpact,
		Effort:            arg.Effort,
		RiskLevel:         arg.RiskLevel,
	}, nil
}

var _ Store = (*geoStoreStub)(nil)

func ts(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}
