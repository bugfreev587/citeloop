package geo

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/growthradar"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestGeneratePromptSetCreatesPromptsCompetitorsAndOwnedSurface(t *testing.T) {
	projectID := uuid.New()
	runID := uuid.New()
	promptSetID := uuid.New()
	store := &geoStoreStub{
		property:    db.SeoProperty{ID: uuid.New(), ProjectID: projectID, SiteUrl: "https://unipost.dev"},
		runID:       runID,
		promptSetID: promptSetID,
		profile: db.ProductProfile{
			ID:        uuid.New(),
			ProjectID: projectID,
			Profile: json.RawMessage(`{
				"positioning": "UniPost is an all-in-one social publishing tool.",
				"value_props": ["Cross-post everywhere", "Schedule in advance"],
				"features": ["multi-platform scheduling", "analytics dashboard", "AI captions"],
				"icp": ["solo creators", "small marketing teams"],
				"key_terms": ["social scheduling", "cross-posting"],
				"competitors": ["Buffer", "Hootsuite"],
				"differentiators": ["single API for many platforms"]
			}`),
		},
		topics: []db.Topic{
			{ID: uuid.New(), ProjectID: projectID, Title: "The Complete Guide to Social Media Scheduling", TargetKeyword: stringPtr("social media scheduling tools"), Priority: 9},
			{ID: uuid.New(), ProjectID: projectID, Title: "Buffer vs UniPost", TargetPrompt: stringPtr("Buffer alternative for creators"), Priority: 8},
		},
	}

	result, err := Service{
		Q:   store,
		Now: func() time.Time { return time.Date(2026, 6, 8, 12, 30, 0, 0, time.UTC) },
	}.GeneratePromptSet(context.Background(), projectID, GeneratePromptSetRequest{Locale: "en-US", Status: "active"})
	if err != nil {
		t.Fatalf("GeneratePromptSet error: %v", err)
	}

	if !store.started || store.finishedStatus != "ok" {
		t.Fatalf("run started=%v finished=%q, want started and ok", store.started, store.finishedStatus)
	}
	if result.PromptSet.ID != promptSetID || result.PromptSet.Status != "active" {
		t.Fatalf("prompt set = %+v, want active prompt set %s", result.PromptSet, promptSetID)
	}
	if len(result.Prompts) < 30 {
		t.Fatalf("generated %d prompts, want at least 30", len(result.Prompts))
	}
	for _, intent := range []string{"category_recommendation", "problem_solution", "comparison", "alternative", "workflow", "integration", "buyer_intent", "definition_entity"} {
		if !hasIntent(result.Prompts, intent) {
			t.Fatalf("missing intent %q in prompts: %+v", intent, result.Prompts)
		}
	}
	if !hasCompetitor(store.competitors, "Buffer") || !hasCompetitor(store.competitors, "Hootsuite") {
		t.Fatalf("competitors = %+v, want Buffer and Hootsuite", store.competitors)
	}
	if !hasProjectSurface(store.surfaces, "https://unipost.dev") {
		t.Fatalf("surfaces = %+v, want project-owned unipost.dev surface", store.surfaces)
	}
	for _, prompt := range result.Prompts {
		if strings.TrimSpace(prompt.PromptText) == "" || prompt.Status != "active" || prompt.Locale != "en-US" {
			t.Fatalf("bad prompt = %+v", prompt)
		}
	}
}

func TestGeneratePromptSetWritesCompetitorDomainsFromProfileURLs(t *testing.T) {
	projectID := uuid.New()
	store := &geoStoreStub{
		property:    db.SeoProperty{ID: uuid.New(), ProjectID: projectID, SiteUrl: "https://unipost.dev"},
		promptSetID: uuid.New(),
		profile: db.ProductProfile{
			ID:        uuid.New(),
			ProjectID: projectID,
			Profile: json.RawMessage(`{
				"positioning": "UniPost is an all-in-one social publishing tool.",
				"value_props": ["Cross-post everywhere"],
				"features": ["multi-platform scheduling"],
				"icp": ["small marketing teams"],
				"key_terms": ["social scheduling"],
				"competitors": ["PostSyncer https://postsyncer.com/tools", "https://buffer.com/resources/social-media"],
				"differentiators": ["single API for many platforms"]
			}`),
		},
	}

	_, err := Service{
		Q:   store,
		Now: func() time.Time { return time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC) },
	}.GeneratePromptSet(context.Background(), projectID, GeneratePromptSetRequest{Locale: "en-US", Status: "active"})
	if err != nil {
		t.Fatalf("GeneratePromptSet error: %v", err)
	}

	if !competitorHasDomain(store.competitors, "PostSyncer https://postsyncer.com/tools", "postsyncer.com") {
		t.Fatalf("competitors = %+v, want PostSyncer profile competitor to store postsyncer.com domain", store.competitors)
	}
	if !competitorHasDomain(store.competitors, "https://buffer.com/resources/social-media", "buffer.com") {
		t.Fatalf("competitors = %+v, want Buffer URL competitor to store buffer.com domain", store.competitors)
	}
}

func TestGeneratePromptSetDegradesWhenActiveProfileMissing(t *testing.T) {
	projectID := uuid.New()
	runID := uuid.New()
	store := &geoStoreStub{
		runID:      runID,
		profileErr: pgx.ErrNoRows,
	}

	result, err := Service{
		Q:   store,
		Now: func() time.Time { return time.Date(2026, 6, 23, 8, 30, 0, 0, time.UTC) },
	}.GeneratePromptSet(context.Background(), projectID, GeneratePromptSetRequest{Locale: "en-US", Status: "active"})
	if err != nil {
		t.Fatalf("GeneratePromptSet error = %v, want degraded result", err)
	}

	if !store.started || store.finishedStatus != "degraded" {
		t.Fatalf("run started=%v finished=%q, want started and degraded", store.started, store.finishedStatus)
	}
	if result.Run.ID != runID || result.Run.Status != "degraded" {
		t.Fatalf("run = %+v, want degraded run %s", result.Run, runID)
	}
	if result.PromptSet.ID != uuid.Nil || len(result.Prompts) != 0 || len(result.Competitors) != 0 {
		t.Fatalf("result = %+v, want no prompt artifacts without an active profile", result)
	}
	if !hasNote(result.DataSourceNotes, "missing_active_profile") {
		t.Fatalf("data source notes = %+v, want missing_active_profile", result.DataSourceNotes)
	}
}

func TestTargetTopicsRejectInternalTermsFromExistingTopics(t *testing.T) {
	unsafe := "AES-256-GCM"
	public := "social publishing analytics"
	topics := targetTopics(profileFields{Features: []string{"audit history"}, KeyTerms: []string{"API key"}}, []db.Topic{
		{Title: unsafe, TargetKeyword: &unsafe},
		{Title: public, TargetKeyword: &public},
	})
	for _, topic := range topics {
		if growthradar.ContainsInternalSensitiveTerm(topic) {
			t.Fatalf("internal topic reached prompt generation: %q in %#v", topic, topics)
		}
	}
	if !containsText(topics, public) || !containsText(topics, "audit history") {
		t.Fatalf("public topics missing: %#v", topics)
	}
}

func containsText(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func TestImportManualFixtureObservationsComputesScore(t *testing.T) {
	projectID := uuid.New()
	runID := uuid.New()
	store := &geoStoreStub{
		runID: runID,
		surfaces: []db.GeoExternalSurface{{
			ID:            uuid.New(),
			ProjectID:     projectID,
			Url:           "https://unipost.dev",
			NormalizedUrl: "https://unipost.dev/",
			OwnerType:     "project",
		}},
		competitors: []db.GeoCompetitor{{
			ID:        uuid.New(),
			ProjectID: projectID,
			Name:      "Buffer",
			Status:    "active",
		}},
	}
	for i := 0; i < 12; i++ {
		store.prompts = append(store.prompts, db.GeoPrompt{
			ID:          uuid.New(),
			ProjectID:   projectID,
			PromptSetID: uuid.New(),
			PromptText:  "prompt",
			IntentType:  "buyer_intent",
			Locale:      "en-US",
			Status:      "active",
			Priority:    8,
		})
	}
	fixtures := make([]ManualFixtureObservationInput, 0, 10)
	for i := 0; i < 10; i++ {
		cited := []string{"https://buffer.com/resources/social-scheduling"}
		if i%2 == 0 {
			cited = append(cited, "https://unipost.dev/blog/social-scheduling")
		}
		fixtures = append(fixtures, ManualFixtureObservationInput{
			PromptID:            store.prompts[i].ID,
			AnswerSummary:       "Manual answer summary",
			CitedURLs:           cited,
			BrandMentioned:      i%3 != 0,
			CompetitorMentions:  []string{"Buffer"},
			CompetitorCitations: []string{"Buffer"},
			EvidenceSnippets:    []string{"answer cited Buffer and sometimes UniPost"},
			ProjectCitationRank: 2,
		})
	}

	result, err := Service{
		Q:   store,
		Now: func() time.Time { return time.Date(2026, 6, 8, 13, 0, 0, 0, time.UTC) },
	}.ImportManualFixtureObservations(context.Background(), projectID, ImportManualFixtureRequest{
		Engine:       "Perplexity",
		Locale:       "en-US",
		Observations: fixtures,
	})
	if err != nil {
		t.Fatalf("ImportManualFixtureObservations error: %v", err)
	}

	if !store.started || store.finishedStatus != "ok" {
		t.Fatalf("run started=%v finished=%q, want started and ok", store.started, store.finishedStatus)
	}
	if len(result.Observations) != 10 {
		t.Fatalf("observations = %d, want 10", len(result.Observations))
	}
	projectCited := 0
	for _, observation := range result.Observations {
		if observation.SourceType != ProviderManualFixture || observation.ObservationState != "observed" {
			t.Fatalf("observation = %+v, want manual observed", observation)
		}
		if observation.ProjectCitationCount > 0 {
			projectCited++
		}
	}
	if projectCited != 5 {
		t.Fatalf("project cited observations = %d, want 5", projectCited)
	}
	if result.Score.PromptCountTotal != 12 || result.Score.PromptCountObserved != 10 || result.Score.EngineCountObserved != 1 {
		t.Fatalf("score counts = %+v, want total=12 observed=10 engines=1", result.Score)
	}
	if pgutil.Float(result.Score.Coverage) <= 0 || result.Score.Confidence != "medium" {
		t.Fatalf("score = %+v, want non-zero medium confidence", result.Score)
	}
	if !strings.Contains(string(result.Score.Breakdown), "brand_mention_rate") {
		t.Fatalf("breakdown = %s, want brand_mention_rate", string(result.Score.Breakdown))
	}
	for _, field := range []string{"geo_pr6_v2", "crawler_access_health", "citation_rank_score", "external_surface_coverage", "observed_count"} {
		if !strings.Contains(string(result.Score.Breakdown), field) {
			t.Fatalf("breakdown = %s, want %s", string(result.Score.Breakdown), field)
		}
	}
	if !hasCitedSurfaceTimestamp(store.surfaces, "https://unipost.dev/") {
		t.Fatalf("surfaces = %+v, want cited project-owned surface timestamp", store.surfaces)
	}
}

func hasIntent(prompts []db.GeoPrompt, intent string) bool {
	for _, prompt := range prompts {
		if prompt.IntentType == intent {
			return true
		}
	}
	return false
}

func hasCompetitor(competitors []db.GeoCompetitor, name string) bool {
	for _, competitor := range competitors {
		if strings.EqualFold(competitor.Name, name) && competitor.Status == "active" {
			return true
		}
	}
	return false
}

func competitorHasDomain(competitors []db.GeoCompetitor, name, domain string) bool {
	for _, competitor := range competitors {
		if !strings.EqualFold(competitor.Name, name) {
			continue
		}
		var domains []string
		if err := json.Unmarshal(competitor.Domains, &domains); err != nil {
			return false
		}
		for _, got := range domains {
			if got == domain {
				return true
			}
		}
	}
	return false
}

func hasProjectSurface(surfaces []db.GeoExternalSurface, url string) bool {
	for _, surface := range surfaces {
		if surface.Url == url && surface.OwnerType == "project" {
			return true
		}
	}
	return false
}

func hasCitedSurfaceTimestamp(surfaces []db.GeoExternalSurface, normalizedURL string) bool {
	for _, surface := range surfaces {
		if surface.NormalizedUrl == normalizedURL && surface.LastCitedAt.Valid {
			return true
		}
	}
	return false
}

func hasNote(notes []string, want string) bool {
	for _, note := range notes {
		if note == want {
			return true
		}
	}
	return false
}

func (s *geoStoreStub) GetActiveProfile(_ context.Context, projectID uuid.UUID) (db.ProductProfile, error) {
	if s.profileErr != nil {
		return db.ProductProfile{}, s.profileErr
	}
	s.profile.ProjectID = projectID
	return s.profile, nil
}

func (s *geoStoreStub) ListTopics(context.Context, uuid.UUID) ([]db.Topic, error) {
	return s.topics, nil
}

func (s *geoStoreStub) CreateGEOPromptSet(_ context.Context, arg db.CreateGEOPromptSetParams) (db.GeoPromptSet, error) {
	id := s.promptSetID
	if id == uuid.Nil {
		id = uuid.New()
	}
	row := db.GeoPromptSet{ID: id, ProjectID: arg.ProjectID, Name: arg.Name, Status: arg.Status, Locale: arg.Locale, CreatedByRunID: arg.CreatedByRunID}
	s.promptSets = append(s.promptSets, row)
	return row, nil
}

func (s *geoStoreStub) ListGEOPromptSets(context.Context, db.ListGEOPromptSetsParams) ([]db.GeoPromptSet, error) {
	return s.promptSets, nil
}

func (s *geoStoreStub) GetGEOPromptSetForProject(_ context.Context, arg db.GetGEOPromptSetForProjectParams) (db.GeoPromptSet, error) {
	for _, row := range s.promptSets {
		if row.ID == arg.ID && row.ProjectID == arg.ProjectID {
			return row, nil
		}
	}
	return db.GeoPromptSet{}, nil
}

func (s *geoStoreStub) UpdateGEOPromptSet(_ context.Context, arg db.UpdateGEOPromptSetParams) (db.GeoPromptSet, error) {
	row := db.GeoPromptSet{ID: arg.ID, ProjectID: arg.ProjectID, Name: arg.Name, Status: arg.Status, Locale: arg.Locale}
	for i := range s.promptSets {
		if s.promptSets[i].ID == arg.ID {
			s.promptSets[i] = row
			return row, nil
		}
	}
	s.promptSets = append(s.promptSets, row)
	return row, nil
}

func (s *geoStoreStub) CreateGEOPrompt(_ context.Context, arg db.CreateGEOPromptParams) (db.GeoPrompt, error) {
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
	s.prompts = append(s.prompts, row)
	return row, nil
}

func (s *geoStoreStub) ListGEOPrompts(context.Context, db.ListGEOPromptsParams) ([]db.GeoPrompt, error) {
	return s.prompts, nil
}

func (s *geoStoreStub) ListActiveGEOPrompts(context.Context, uuid.UUID) ([]db.GeoPrompt, error) {
	out := make([]db.GeoPrompt, 0, len(s.prompts))
	for _, prompt := range s.prompts {
		if prompt.Status == "active" {
			out = append(out, prompt)
		}
	}
	return out, nil
}

func (s *geoStoreStub) UpdateGEOPrompt(_ context.Context, arg db.UpdateGEOPromptParams) (db.GeoPrompt, error) {
	row := db.GeoPrompt{
		ID:            arg.ID,
		ProjectID:     arg.ProjectID,
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
	for i := range s.prompts {
		if s.prompts[i].ID == arg.ID {
			row.PromptSetID = s.prompts[i].PromptSetID
			s.prompts[i] = row
			return row, nil
		}
	}
	s.prompts = append(s.prompts, row)
	return row, nil
}

func (s *geoStoreStub) UpsertGEOCompetitor(_ context.Context, arg db.UpsertGEOCompetitorParams) (db.GeoCompetitor, error) {
	nameKey := strings.ToLower(strings.TrimSpace(arg.Name))
	row := db.GeoCompetitor{
		ID:        uuid.New(),
		ProjectID: arg.ProjectID,
		Name:      arg.Name,
		NameKey:   &nameKey,
		Domains:   append(json.RawMessage{}, arg.Domains...),
		Aliases:   append(json.RawMessage{}, arg.Aliases...),
		Source:    arg.Source,
		Status:    arg.Status,
	}
	s.competitors = append(s.competitors, row)
	return row, nil
}

func (s *geoStoreStub) ListGEOCompetitors(context.Context, db.ListGEOCompetitorsParams) ([]db.GeoCompetitor, error) {
	return s.competitors, nil
}

func (s *geoStoreStub) UpdateGEOCompetitor(_ context.Context, arg db.UpdateGEOCompetitorParams) (db.GeoCompetitor, error) {
	nameKey := strings.ToLower(strings.TrimSpace(arg.Name))
	row := db.GeoCompetitor{ID: arg.ID, ProjectID: arg.ProjectID, Name: arg.Name, NameKey: &nameKey, Domains: arg.Domains, Aliases: arg.Aliases, Source: arg.Source, Status: arg.Status}
	for i := range s.competitors {
		if s.competitors[i].ID == arg.ID {
			s.competitors[i] = row
			return row, nil
		}
	}
	s.competitors = append(s.competitors, row)
	return row, nil
}

func (s *geoStoreStub) UpsertGEOExternalSurface(_ context.Context, arg db.UpsertGEOExternalSurfaceParams) (db.GeoExternalSurface, error) {
	row := db.GeoExternalSurface{
		ID:                 uuid.New(),
		ProjectID:          arg.ProjectID,
		Url:                arg.Url,
		NormalizedUrl:      arg.NormalizedUrl,
		Platform:           arg.Platform,
		SurfaceType:        arg.SurfaceType,
		OwnerType:          arg.OwnerType,
		CanonicalTargetUrl: arg.CanonicalTargetUrl,
		BacklinkState:      arg.BacklinkState,
		LastHttpStatus:     arg.LastHttpStatus,
		LastCitedAt:        arg.LastCitedAt,
	}
	s.surfaces = append(s.surfaces, row)
	return row, nil
}

func (s *geoStoreStub) ListGEOExternalSurfaces(context.Context, db.ListGEOExternalSurfacesParams) ([]db.GeoExternalSurface, error) {
	return s.surfaces, nil
}

func (s *geoStoreStub) ListProjectOwnedGEOExternalSurfaces(context.Context, uuid.UUID) ([]db.GeoExternalSurface, error) {
	out := make([]db.GeoExternalSurface, 0, len(s.surfaces))
	for _, surface := range s.surfaces {
		if surface.OwnerType == "project" {
			out = append(out, surface)
		}
	}
	return out, nil
}

func (s *geoStoreStub) CreateGEOObservation(_ context.Context, arg db.CreateGEOObservationParams) (db.GeoObservation, error) {
	row := db.GeoObservation{
		ID:                      uuid.New(),
		ProjectID:               arg.ProjectID,
		RunID:                   arg.RunID,
		PromptID:                arg.PromptID,
		Engine:                  arg.Engine,
		Locale:                  arg.Locale,
		SourceType:              arg.SourceType,
		BrandMentioned:          arg.BrandMentioned,
		BrandPosition:           arg.BrandPosition,
		ProjectCitationCount:    arg.ProjectCitationCount,
		ProjectCitationRankBest: arg.ProjectCitationRankBest,
		ProjectCitedSurfaceIds:  append(json.RawMessage{}, arg.ProjectCitedSurfaceIds...),
		CitedUrls:               append(json.RawMessage{}, arg.CitedUrls...),
		CompetitorMentions:      append(json.RawMessage{}, arg.CompetitorMentions...),
		CompetitorCitations:     append(json.RawMessage{}, arg.CompetitorCitations...),
		ObservationState:        arg.ObservationState,
		AnswerSummary:           arg.AnswerSummary,
		EvidenceSnippets:        append(json.RawMessage{}, arg.EvidenceSnippets...),
		Confidence:              arg.Confidence,
		ObservedAt:              arg.ObservedAt,
	}
	s.observations = append(s.observations, row)
	return row, nil
}

func (s *geoStoreStub) ListGEOObservations(context.Context, db.ListGEOObservationsParams) ([]db.GeoObservation, error) {
	return s.observations, nil
}

func (s *geoStoreStub) ListGEOObservationsForRun(context.Context, db.ListGEOObservationsForRunParams) ([]db.GeoObservation, error) {
	return s.observations, nil
}

func (s *geoStoreStub) CreateGEOVisibilityScore(_ context.Context, arg db.CreateGEOVisibilityScoreParams) (db.GeoVisibilityScore, error) {
	row := db.GeoVisibilityScore{
		ID:                  uuid.New(),
		ProjectID:           arg.ProjectID,
		RunID:               arg.RunID,
		Score:               arg.Score,
		Coverage:            arg.Coverage,
		Confidence:          arg.Confidence,
		Breakdown:           append(json.RawMessage{}, arg.Breakdown...),
		PromptCountTotal:    arg.PromptCountTotal,
		PromptCountObserved: arg.PromptCountObserved,
		EngineCountObserved: arg.EngineCountObserved,
		ComputedAt:          arg.ComputedAt,
	}
	s.visibilityScores = append(s.visibilityScores, row)
	return row, nil
}

func (s *geoStoreStub) GetLatestGEOVisibilityScore(context.Context, uuid.UUID) (db.GeoVisibilityScore, error) {
	if len(s.visibilityScores) == 0 {
		return db.GeoVisibilityScore{}, nil
	}
	return s.visibilityScores[len(s.visibilityScores)-1], nil
}

func (s *geoStoreStub) ListGEOVisibilityScores(context.Context, db.ListGEOVisibilityScoresParams) ([]db.GeoVisibilityScore, error) {
	return s.visibilityScores, nil
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: id != uuid.Nil}
}
