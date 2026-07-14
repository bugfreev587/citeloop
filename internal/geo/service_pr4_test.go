package geo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
)

func TestObserveAnswerProviderUnavailableMarksRunDegraded(t *testing.T) {
	projectID := uuid.New()
	promptID := uuid.New()
	store := &geoStoreStub{
		runID: uuid.New(),
		prompts: []db.GeoPrompt{{
			ID:         promptID,
			ProjectID:  projectID,
			PromptText: "best tools for social scheduling",
			Locale:     "en-US",
			Status:     "active",
			Priority:   8,
		}},
	}

	result, err := Service{Q: store}.ObserveAnswerProvider(context.Background(), projectID, ObserveAnswerProviderRequest{
		Engine:     "Perplexity",
		MaxPrompts: 1,
	})
	if err != nil {
		t.Fatalf("ObserveAnswerProvider error: %v", err)
	}
	if store.finishedStatus != "degraded" {
		t.Fatalf("finished status = %q, want degraded", store.finishedStatus)
	}
	if len(result.Observations) != 1 {
		t.Fatalf("observations = %d, want 1", len(result.Observations))
	}
	observation := result.Observations[0]
	if observation.ObservationState != "provider_unavailable" || observation.SourceType != SourceTypeAnswerEngine {
		t.Fatalf("observation = %+v, want answer_engine provider_unavailable", observation)
	}
	if len(store.visibilityScores) != 0 {
		t.Fatalf("visibility scores = %d, want 0 for unavailable provider", len(store.visibilityScores))
	}
}

func TestObserveAnswerProviderRecordsSkippedMetadata(t *testing.T) {
	projectID := uuid.New()
	store := &geoStoreStub{
		runID: uuid.New(),
		prompts: []db.GeoPrompt{
			{ID: uuid.New(), ProjectID: projectID, PromptText: "best tools for social scheduling", Locale: "en-US", Status: "active", Priority: 8},
			{ID: uuid.New(), ProjectID: projectID, PromptText: "alternatives to Buffer", Locale: "en-US", Status: "active", Priority: 7},
		},
	}

	result, err := Service{Q: store}.ObserveAnswerProvider(context.Background(), projectID, ObserveAnswerProviderRequest{
		Engine:     "Perplexity",
		MaxPrompts: 1,
	})
	if err != nil {
		t.Fatalf("ObserveAnswerProvider error: %v", err)
	}
	if len(result.SkippedPrompts) != 1 || result.SkippedPrompts[0].Reason != "max_prompts" {
		t.Fatalf("skipped prompts = %+v, want one max_prompts skip", result.SkippedPrompts)
	}
	if len(result.SkippedEngines) != 1 || result.SkippedEngines[0] != "Perplexity" {
		t.Fatalf("skipped engines = %+v, want Perplexity", result.SkippedEngines)
	}
	if !strings.Contains(string(store.finishedOutput), "skipped_prompts") || !strings.Contains(string(store.finishedOutput), "skipped_engines") {
		t.Fatalf("finished output = %s, want skipped metadata", string(store.finishedOutput))
	}
}

func TestObserveAnswerProviderPersistsObservedCitationsAndScore(t *testing.T) {
	projectID := uuid.New()
	promptID := uuid.New()
	store := &geoStoreStub{
		runID: uuid.New(),
		prompts: []db.GeoPrompt{{
			ID:         promptID,
			ProjectID:  projectID,
			PromptText: "best tools for social scheduling",
			Locale:     "en-US",
			Status:     "active",
			Priority:   8,
		}},
		surfaces: []db.GeoExternalSurface{{
			ID:            uuid.New(),
			ProjectID:     projectID,
			Url:           "https://unipost.dev",
			NormalizedUrl: "https://unipost.dev/",
			OwnerType:     "project",
			SurfaceType:   "domain",
		}},
	}
	provider := fakeAnswerProvider{responses: []ProviderObservation{{
		PromptID:            promptID,
		Engine:              "Perplexity",
		Locale:              "en-US",
		AnswerSummary:       "UniPost and Buffer are mentioned.",
		CitedURLs:           []string{"https://unipost.dev/blog/social-scheduling", "https://buffer.com/resources"},
		BrandMentioned:      true,
		CompetitorMentions:  []string{"Buffer"},
		CompetitorCitations: []string{"Buffer"},
		Confidence:          ConfidenceMedium,
		CostUSD:             0.02,
	}}}

	result, err := Service{Q: store, AnswerProvider: provider}.ObserveAnswerProvider(context.Background(), projectID, ObserveAnswerProviderRequest{
		Engine:     "Perplexity",
		MaxPrompts: 1,
	})
	if err != nil {
		t.Fatalf("ObserveAnswerProvider error: %v", err)
	}
	if store.finishedStatus != "ok" {
		t.Fatalf("finished status = %q, want ok", store.finishedStatus)
	}
	if len(result.Observations) != 1 {
		t.Fatalf("observations = %d, want 1", len(result.Observations))
	}
	observation := result.Observations[0]
	if observation.ObservationState != "observed" || observation.SourceType != SourceTypeAnswerEngine {
		t.Fatalf("observation = %+v, want answer_engine observed", observation)
	}
	if observation.ProjectCitationCount != 1 {
		t.Fatalf("project citation count = %d, want 1", observation.ProjectCitationCount)
	}
	if len(store.visibilityScores) != 1 {
		t.Fatalf("visibility scores = %d, want 1", len(store.visibilityScores))
	}
	if pgutil.Float(result.Score.Coverage) <= 0 {
		t.Fatalf("score coverage = %+v, want positive", result.Score.Coverage)
	}
	if !hasCitedSurfaceTimestamp(store.surfaces, "https://unipost.dev/") {
		t.Fatalf("surfaces = %+v, want provider citation to update surface timestamp", store.surfaces)
	}
}

func TestObserveAnswerProviderClassifiesKnownCompetitorCitationsFromURLs(t *testing.T) {
	projectID := uuid.New()
	promptID := uuid.New()
	store := &geoStoreStub{
		runID: uuid.New(),
		prompts: []db.GeoPrompt{{
			ID:         promptID,
			ProjectID:  projectID,
			PromptText: "best social publishing tools",
			Locale:     "en-US",
			Status:     "active",
			Priority:   8,
		}},
		competitors: []db.GeoCompetitor{{
			ID:        uuid.New(),
			ProjectID: projectID,
			Name:      "PostSyncer",
			Domains:   json.RawMessage(`["postsyncer.com"]`),
			Status:    "active",
		}},
	}
	provider := fakeAnswerProvider{responses: []ProviderObservation{{
		PromptID:       promptID,
		Engine:         "Perplexity",
		Locale:         "en-US",
		AnswerSummary:  "PostSyncer is cited by the answer.",
		CitedURLs:      []string{"https://postsyncer.com/tools"},
		BrandMentioned: false,
		Confidence:     ConfidenceMedium,
		CostUSD:        0.02,
	}}}

	result, err := Service{Q: store, AnswerProvider: provider}.ObserveAnswerProvider(context.Background(), projectID, ObserveAnswerProviderRequest{
		Engine:     "Perplexity",
		MaxPrompts: 1,
	})
	if err != nil {
		t.Fatalf("ObserveAnswerProvider error: %v", err)
	}
	if len(result.Observations) != 1 {
		t.Fatalf("observations = %d, want 1", len(result.Observations))
	}
	var citations []string
	if err := json.Unmarshal(result.Observations[0].CompetitorCitations, &citations); err != nil {
		t.Fatalf("decode competitor citations: %v", err)
	}
	if len(citations) != 1 || citations[0] != "https://postsyncer.com/tools" {
		t.Fatalf("competitor citations = %#v, want cited PostSyncer URL", citations)
	}
	if len(store.classificationAuditRecords) != 1 {
		t.Fatalf("classification audit records = %d, want 1", len(store.classificationAuditRecords))
	}
	record := store.classificationAuditRecords[0]
	if record.ClassifierType != "citation_url_entity" || !record.ObservationID.Valid || record.ObservationID.Bytes != result.Observations[0].ID {
		t.Fatalf("audit record metadata = %+v, want citation_url_entity linked to observation", record)
	}
	for _, want := range []string{"https://postsyncer.com/tools", "postsyncer.com", "PostSyncer"} {
		if !strings.Contains(string(record.Input), want) && !strings.Contains(string(record.Output), want) {
			t.Fatalf("audit record missing %q: input=%s output=%s", want, record.Input, record.Output)
		}
	}
	if !strings.Contains(string(record.ReasonCodes), "known_competitor_domain") {
		t.Fatalf("audit reason codes = %s, want known_competitor_domain", record.ReasonCodes)
	}
}

func TestPerplexityProviderUsesSonarContract(t *testing.T) {
	var sawAuth bool
	var sawModel bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sonar" || r.Method != http.MethodPost {
			t.Fatalf("request = %s %s, want POST /v1/sonar", r.Method, r.URL.Path)
		}
		sawAuth = r.Header.Get("Authorization") == "Bearer test-key"
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		sawModel = body["model"] == "sonar-pro"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{"content": "UniPost is cited."},
			}},
			"citations": []string{"https://unipost.dev/blog/social-scheduling"},
			"usage": map[string]any{
				"cost": map[string]any{"total_cost": 0.01},
			},
		})
	}))
	defer server.Close()

	provider := NewPerplexityProvider("test-key", server.URL, "sonar-pro", server.Client())
	rows, cost, err := provider.Observe(context.Background(), []db.GeoPrompt{{
		ID:         uuid.New(),
		PromptText: "best tools",
		Locale:     "en-US",
	}})
	if err != nil {
		t.Fatalf("Observe error: %v", err)
	}
	if !sawAuth || !sawModel {
		t.Fatalf("sawAuth=%v sawModel=%v, want both true", sawAuth, sawModel)
	}
	if cost != 0.01 {
		t.Fatalf("cost = %f, want 0.01", cost)
	}
	if len(rows) != 1 || len(rows[0].CitedURLs) != 1 || rows[0].CitedURLs[0] != "https://unipost.dev/blog/social-scheduling" {
		t.Fatalf("rows = %+v, want cited URL", rows)
	}
}

func TestMonitorExternalSurfacesUsesHonestUAAndUpdatesStatus(t *testing.T) {
	var userAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent = r.UserAgent()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	projectID := uuid.New()
	store := &geoStoreStub{
		runID: uuid.New(),
		surfaces: []db.GeoExternalSurface{{
			ID:            uuid.New(),
			ProjectID:     projectID,
			Url:           server.URL + "/citation",
			NormalizedUrl: server.URL + "/citation",
			OwnerType:     "project",
			SurfaceType:   "page",
			BacklinkState: "unknown",
		}},
	}

	result, err := Service{
		Q:          store,
		HTTPClient: server.Client(),
		Now:        func() time.Time { return time.Date(2026, 6, 8, 15, 0, 0, 0, time.UTC) },
	}.MonitorExternalSurfaces(context.Background(), projectID, MonitorExternalSurfacesRequest{Limit: 10})
	if err != nil {
		t.Fatalf("MonitorExternalSurfaces error: %v", err)
	}
	if store.finishedStatus != "ok" {
		t.Fatalf("finished status = %q, want ok", store.finishedStatus)
	}
	if !strings.Contains(userAgent, "CiteLoop GEO external surface monitor") {
		t.Fatalf("user agent = %q, want honest CiteLoop monitor UA", userAgent)
	}
	if len(result.Surfaces) != 1 || result.Surfaces[0].LastHttpStatus == nil || *result.Surfaces[0].LastHttpStatus != http.StatusAccepted {
		t.Fatalf("surfaces = %+v, want HTTP 202", result.Surfaces)
	}
}

type fakeAnswerProvider struct {
	responses []ProviderObservation
	available bool
	err       error
	cost      float64
}

func (f fakeAnswerProvider) Name() string {
	return "fake_answer_provider"
}

func (f fakeAnswerProvider) EvidenceIdentity() AnswerProviderEvidenceIdentity {
	return AnswerProviderEvidenceIdentity{Model: "fake-model", ProviderVersion: "fake-provider-v1"}
}

func (f fakeAnswerProvider) Available() bool {
	if f.available {
		return true
	}
	return len(f.responses) > 0
}

func (f fakeAnswerProvider) Observe(context.Context, []db.GeoPrompt) ([]ProviderObservation, float64, error) {
	cost := f.cost
	if cost == 0 {
		for _, response := range f.responses {
			cost += response.CostUSD
		}
	}
	return f.responses, cost, f.err
}
