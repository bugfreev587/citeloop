package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	geopkg "github.com/citeloop/citeloop/internal/geo"
	"github.com/citeloop/citeloop/internal/pgutil"
	seopkg "github.com/citeloop/citeloop/internal/seo"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Server) getGEOOverview(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.geoProjectID(w, r)
	if !ok {
		return
	}
	score, scoreErr := s.Q.GetLatestGEOVisibilityScore(r.Context(), projectID)
	if scoreErr != nil && !errors.Is(scoreErr, pgx.ErrNoRows) {
		writeErr(w, http.StatusInternalServerError, scoreErr.Error())
		return
	}
	promptSets, err := s.Q.ListGEOPromptSets(r.Context(), db.ListGEOPromptSetsParams{ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	prompts, err := s.Q.ListGEOPrompts(r.Context(), db.ListGEOPromptsParams{ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	competitors, err := s.Q.ListGEOCompetitors(r.Context(), db.ListGEOCompetitorsParams{ProjectID: projectID, Status: "active"})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	surfaces, err := s.Q.ListGEOExternalSurfaces(r.Context(), db.ListGEOExternalSurfacesParams{ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	observations, err := s.Q.ListGEOObservations(r.Context(), db.ListGEOObservationsParams{ProjectID: projectID, LimitRows: 50})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var scoreOut any
	if scoreErr == nil {
		scoreOut = score
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"score":             scoreOut,
		"prompt_sets":       emptySlice(promptSets),
		"prompts":           emptySlice(prompts),
		"competitors":       emptySlice(competitors),
		"external_surfaces": emptySlice(surfaces),
		"observations":      emptySlice(observations),
	})
}

func (s *Server) generateGEOPromptSet(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.geoProjectID(w, r)
	if !ok {
		return
	}
	var in geopkg.GeneratePromptSetRequest
	if err := decodeOptionalJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := s.geoService(r.Context()).GeneratePromptSet(r.Context(), projectID, in)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) listGEOPromptSets(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.geoProjectID(w, r)
	if !ok {
		return
	}
	sets, err := s.Q.ListGEOPromptSets(r.Context(), db.ListGEOPromptSetsParams{ProjectID: projectID, Status: r.URL.Query().Get("status")})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	prompts, err := s.Q.ListGEOPrompts(r.Context(), db.ListGEOPromptsParams{ProjectID: projectID, Status: r.URL.Query().Get("prompt_status")})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	competitors, err := s.Q.ListGEOCompetitors(r.Context(), db.ListGEOCompetitorsParams{ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"prompt_sets": emptySlice(sets),
		"prompts":     emptySlice(prompts),
		"competitors": emptySlice(competitors),
	})
}

func (s *Server) updateGEOPromptSet(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.geoProjectID(w, r)
	if !ok {
		return
	}
	promptSetID, err := uuid.Parse(chi.URLParam(r, "promptSetID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad prompt set id")
		return
	}
	existing, err := s.Q.GetGEOPromptSetForProject(r.Context(), db.GetGEOPromptSetForProjectParams{ID: promptSetID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "prompt set not found")
		return
	}
	var in struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Locale string `json:"locale"`
	}
	if err := decodeOptionalJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		in.Name = existing.Name
	}
	if strings.TrimSpace(in.Status) == "" {
		in.Status = existing.Status
	}
	if strings.TrimSpace(in.Locale) == "" {
		in.Locale = existing.Locale
	}
	row, err := s.Q.UpdateGEOPromptSet(r.Context(), db.UpdateGEOPromptSetParams{
		ID:        promptSetID,
		ProjectID: projectID,
		Name:      strings.TrimSpace(in.Name),
		Status:    strings.TrimSpace(in.Status),
		Locale:    strings.TrimSpace(in.Locale),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) updateGEOPrompt(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.geoProjectID(w, r)
	if !ok {
		return
	}
	promptID, err := uuid.Parse(chi.URLParam(r, "promptID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad prompt id")
		return
	}
	existing, err := s.Q.GetGEOPromptForProject(r.Context(), db.GetGEOPromptForProjectParams{ID: promptID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "prompt not found")
		return
	}
	var in struct {
		PromptText    string   `json:"prompt_text"`
		IntentType    string   `json:"intent_type"`
		TargetPersona string   `json:"target_persona"`
		TargetTopic   string   `json:"target_topic"`
		Locale        string   `json:"locale"`
		TargetEngines []string `json:"target_engines"`
		Priority      *int32   `json:"priority"`
		Source        string   `json:"source"`
		Status        string   `json:"status"`
	}
	if err := decodeOptionalJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(in.PromptText) == "" {
		in.PromptText = existing.PromptText
	}
	if strings.TrimSpace(in.IntentType) == "" {
		in.IntentType = existing.IntentType
	}
	if strings.TrimSpace(in.TargetPersona) == "" {
		in.TargetPersona = existing.TargetPersona
	}
	if strings.TrimSpace(in.TargetTopic) == "" {
		in.TargetTopic = existing.TargetTopic
	}
	if strings.TrimSpace(in.Locale) == "" {
		in.Locale = existing.Locale
	}
	if len(in.TargetEngines) == 0 {
		in.TargetEngines = jsonStringList(existing.TargetEngines)
	}
	priority := existing.Priority
	if in.Priority != nil {
		priority = *in.Priority
	}
	if strings.TrimSpace(in.Source) == "" {
		in.Source = existing.Source
	}
	if strings.TrimSpace(in.Status) == "" {
		in.Status = existing.Status
	}
	row, err := s.Q.UpdateGEOPrompt(r.Context(), db.UpdateGEOPromptParams{
		ID:            promptID,
		ProjectID:     projectID,
		PromptText:    strings.TrimSpace(in.PromptText),
		IntentType:    strings.TrimSpace(in.IntentType),
		TargetPersona: strings.TrimSpace(in.TargetPersona),
		TargetTopic:   strings.TrimSpace(in.TargetTopic),
		Locale:        strings.TrimSpace(in.Locale),
		TargetEngines: jsonBytes(in.TargetEngines),
		Priority:      priority,
		Source:        strings.TrimSpace(in.Source),
		Status:        strings.TrimSpace(in.Status),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) updateGEOCompetitor(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.geoProjectID(w, r)
	if !ok {
		return
	}
	competitorID, err := uuid.Parse(chi.URLParam(r, "competitorID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad competitor id")
		return
	}
	existing, err := s.Q.GetGEOCompetitorForProject(r.Context(), db.GetGEOCompetitorForProjectParams{ID: competitorID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "competitor not found")
		return
	}
	var in struct {
		Name    string   `json:"name"`
		Domains []string `json:"domains"`
		Aliases []string `json:"aliases"`
		Source  string   `json:"source"`
		Status  string   `json:"status"`
	}
	if err := decodeOptionalJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		in.Name = existing.Name
	}
	if len(in.Domains) == 0 {
		in.Domains = jsonStringList(existing.Domains)
	}
	if len(in.Aliases) == 0 {
		in.Aliases = jsonStringList(existing.Aliases)
	}
	if strings.TrimSpace(in.Source) == "" {
		in.Source = existing.Source
	}
	if strings.TrimSpace(in.Status) == "" {
		in.Status = existing.Status
	}
	row, err := s.Q.UpdateGEOCompetitor(r.Context(), db.UpdateGEOCompetitorParams{
		ID:        competitorID,
		ProjectID: projectID,
		Name:      strings.TrimSpace(in.Name),
		Domains:   jsonBytes(in.Domains),
		Aliases:   jsonBytes(in.Aliases),
		Source:    strings.TrimSpace(in.Source),
		Status:    strings.TrimSpace(in.Status),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) observeGEOManualFixtures(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.geoProjectID(w, r)
	if !ok {
		return
	}
	var in geopkg.ImportManualFixtureRequest
	if err := decodeOptionalJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := s.geoService(r.Context()).ImportManualFixtureObservations(r.Context(), projectID, in)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) observeGEOAnswerProvider(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.geoProjectID(w, r)
	if !ok {
		return
	}
	var in geopkg.ObserveAnswerProviderRequest
	if err := decodeOptionalJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if in.BudgetUSD <= 0 {
		in.BudgetUSD = s.Env.GEOProviderRunBudgetUSD
	}
	result, err := s.geoService(r.Context()).ObserveAnswerProvider(r.Context(), projectID, in)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) listGEORuns(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.geoProjectID(w, r)
	if !ok {
		return
	}
	limit, err := parseLimit(r, 50, 100)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad limit")
		return
	}
	cursor, err := parseCursor(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad cursor")
		return
	}
	params := db.ListGEORunsParams{
		ProjectID: projectID,
		Agent:     r.URL.Query().Get("agent"),
		Status:    r.URL.Query().Get("status"),
		LimitRows: int32(limit),
	}
	if !cursor.IsZero() {
		params.CursorStartedAt = pgutil.TS(cursor)
	}
	rows, err := s.Q.ListGEORuns(r.Context(), params)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, emptySlice(rows))
}

func (s *Server) listGEOObservations(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.geoProjectID(w, r)
	if !ok {
		return
	}
	limit, err := parseLimit(r, 50, 100)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad limit")
		return
	}
	params := db.ListGEOObservationsParams{
		ProjectID:  projectID,
		Engine:     r.URL.Query().Get("engine"),
		SourceType: r.URL.Query().Get("source_type"),
		LimitRows:  int32(limit),
	}
	if rawPromptID := strings.TrimSpace(r.URL.Query().Get("prompt_id")); rawPromptID != "" {
		promptID, err := uuid.Parse(rawPromptID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "bad prompt id")
			return
		}
		params.PromptID = pgtype.UUID{Bytes: promptID, Valid: true}
	}
	rows, err := s.Q.ListGEOObservations(r.Context(), params)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, emptySlice(rows))
}

func (s *Server) listGEOExternalSurfaces(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.geoProjectID(w, r)
	if !ok {
		return
	}
	rows, err := s.Q.ListGEOExternalSurfaces(r.Context(), db.ListGEOExternalSurfacesParams{ProjectID: projectID, OwnerType: r.URL.Query().Get("owner_type")})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, emptySlice(rows))
}

func (s *Server) createGEOExternalSurface(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.geoProjectID(w, r)
	if !ok {
		return
	}
	var in struct {
		URL                  string          `json:"url"`
		NormalizedURL        string          `json:"normalized_url"`
		Platform             string          `json:"platform"`
		SurfaceType          string          `json:"surface_type"`
		OwnerType            string          `json:"owner_type"`
		CanonicalTargetURL   string          `json:"canonical_target_url"`
		BacklinkState        string          `json:"backlink_state"`
		SourceURL            string          `json:"source_url"`
		CanonicalStatus      string          `json:"canonical_status"`
		IndexabilityStatus   string          `json:"indexability_status"`
		PublicationStatus    string          `json:"publication_status"`
		OwnerConfidence      string          `json:"owner_confidence"`
		VerificationSnapshot json.RawMessage `json:"verification_snapshot"`
		RelatedActionIDs     json.RawMessage `json:"related_action_ids"`
	}
	if err := decodeOptionalJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(in.URL) == "" {
		writeErr(w, http.StatusBadRequest, "url required")
		return
	}
	normalized := strings.TrimSpace(in.NormalizedURL)
	if normalized == "" {
		var err error
		normalized, err = seopkg.NormalizeURL(in.URL, in.URL, seopkg.URLNormalizationConfig{})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	platform := strings.TrimSpace(in.Platform)
	if platform == "" {
		platform = platformForURL(normalized)
	}
	surfaceType := strings.TrimSpace(in.SurfaceType)
	if surfaceType == "" {
		surfaceType = "page"
	}
	ownerType := strings.TrimSpace(in.OwnerType)
	if ownerType == "" {
		ownerType = "project"
	}
	backlinkState := strings.TrimSpace(in.BacklinkState)
	if backlinkState == "" {
		backlinkState = "unknown"
	}
	row, err := s.Q.UpsertGEOExternalSurface(r.Context(), db.UpsertGEOExternalSurfaceParams{
		ProjectID:          projectID,
		Url:                strings.TrimSpace(in.URL),
		NormalizedUrl:      normalized,
		Platform:           platform,
		SurfaceType:        surfaceType,
		OwnerType:          ownerType,
		CanonicalTargetUrl: strPtrFrom(in.CanonicalTargetURL),
		BacklinkState:      backlinkState,
		LastCitedAt:        pgtype.Timestamptz{},
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	row, err = s.Q.UpdateGEOExternalSurfaceMetadata(r.Context(), db.UpdateGEOExternalSurfaceMetadataParams{
		ID:                   row.ID,
		ProjectID:            projectID,
		SourceUrl:            strPtrFrom(in.SourceURL),
		CanonicalStatus:      textOr(in.CanonicalStatus, row.CanonicalStatus, "unknown"),
		IndexabilityStatus:   textOr(in.IndexabilityStatus, row.IndexabilityStatus, "unknown"),
		PublicationStatus:    textOr(in.PublicationStatus, row.PublicationStatus, "unknown"),
		OwnerConfidence:      ownerConfidenceOr(in.OwnerConfidence, row.OwnerConfidence),
		LastVerifiedAt:       pgtype.Timestamptz{},
		VerificationSnapshot: rawOrExisting(in.VerificationSnapshot, row.VerificationSnapshot, `{}`),
		RelatedActionIds:     rawOrExisting(in.RelatedActionIDs, row.RelatedActionIds, `[]`),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) monitorGEOExternalSurfaces(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.geoProjectID(w, r)
	if !ok {
		return
	}
	var in geopkg.MonitorExternalSurfacesRequest
	if err := decodeOptionalJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := s.geoService(r.Context()).MonitorExternalSurfaces(r.Context(), projectID, in)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) analyzeGEOOpportunities(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.geoProjectID(w, r)
	if !ok {
		return
	}
	var in geopkg.AnalyzeObservationsRequest
	if err := decodeOptionalJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	service, err := s.geoServiceForProject(r.Context(), projectID, config.GrowthAITriggerManual)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	result, err := service.AnalyzeObservations(r.Context(), projectID, in)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) listGEOAssetBriefs(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.geoProjectID(w, r)
	if !ok {
		return
	}
	limit, err := parseLimit(r, 50, 100)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad limit")
		return
	}
	rows, err := s.Q.ListGEOAssetBriefs(r.Context(), db.ListGEOAssetBriefsParams{
		ProjectID: projectID,
		Status:    r.URL.Query().Get("status"),
		LimitRows: int32(limit),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, emptySlice(rows))
}

func (s *Server) acceptGEOAssetBrief(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.geoProjectID(w, r)
	if !ok {
		return
	}
	briefID, err := uuid.Parse(chi.URLParam(r, "briefID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad asset brief id")
		return
	}
	service, err := s.geoServiceForProject(r.Context(), projectID, config.GrowthAITriggerManual)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	result, err := service.AcceptGEOAssetBrief(r.Context(), projectID, briefID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.Pool != nil && s.LLM != nil && result.Topic.ID != uuid.Nil {
		started, startErr := s.Q.StartTopicGenerationForProject(r.Context(), db.StartTopicGenerationForProjectParams{
			ID:        result.Topic.ID,
			ProjectID: projectID,
		})
		if startErr != nil && !errors.Is(startErr, pgx.ErrNoRows) {
			writeErr(w, http.StatusInternalServerError, startErr.Error())
			return
		}
		if startErr == nil {
			result.Topic = started
			s.startTopicGeneration(projectID, started.ID)
			writeJSON(w, http.StatusAccepted, result)
			return
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) geoProjectID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return uuid.Nil, false
	}
	if s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database not configured")
		return uuid.Nil, false
	}
	return projectID, true
}

func decodeOptionalJSON(r *http.Request, out any) error {
	if r.Body == nil {
		return nil
	}
	err := json.NewDecoder(r.Body).Decode(out)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func platformForURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return "site"
	}
	return parsed.Host
}

func textOr(value, existing, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(existing); trimmed != "" {
		return trimmed
	}
	return fallback
}

func ownerConfidenceOr(value, existing string) string {
	switch strings.TrimSpace(value) {
	case "high", "medium", "low":
		return strings.TrimSpace(value)
	}
	switch strings.TrimSpace(existing) {
	case "high", "medium", "low":
		return strings.TrimSpace(existing)
	}
	return "medium"
}

func rawOrExisting(raw, existing json.RawMessage, fallback string) json.RawMessage {
	if len(raw) > 0 && json.Valid(raw) {
		return raw
	}
	if len(existing) > 0 && json.Valid(existing) {
		return existing
	}
	return json.RawMessage(fallback)
}

func jsonStringList(raw json.RawMessage) []string {
	var values []string
	_ = json.Unmarshal(raw, &values)
	if values == nil {
		return []string{}
	}
	return values
}

func jsonBytes(value any) json.RawMessage {
	b, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`[]`)
	}
	return b
}
