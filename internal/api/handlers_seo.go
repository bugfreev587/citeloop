package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/pgutil"
	seopkg "github.com/citeloop/citeloop/internal/seo"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Server) seoService() seopkg.Service {
	return seopkg.Service{Q: s.Q, BlogBaseURL: s.Env.BlogBaseURL, GoogleData: s.SEOData}
}

func (s *Server) getSEOOverview(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	overview, err := s.seoService().Overview(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (s *Server) syncSEO(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	var in struct {
		SiteURL string `json:"site_url"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&in)
	}
	svc := s.seoService()
	syncResult, err := svc.Sync(r.Context(), projectID, in.SiteURL)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	analyzeResult, err := svc.Analyze(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sync": syncResult, "analyze": analyzeResult})
}

func (s *Server) analyzeSEO(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	result, err := s.seoService().Analyze(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) listSEORuns(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
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
	runs, err := s.Q.ListSEORuns(r.Context(), db.ListSEORunsParams{
		ProjectID:       projectID,
		Agent:           r.URL.Query().Get("agent"),
		Status:          r.URL.Query().Get("status"),
		CursorStartedAt: pgutil.TS(cursor),
		LimitRows:       int32(limit),
	})
	if cursor.IsZero() {
		runs, err = s.Q.ListSEORuns(r.Context(), db.ListSEORunsParams{
			ProjectID: projectID,
			Agent:     r.URL.Query().Get("agent"),
			Status:    r.URL.Query().Get("status"),
			LimitRows: int32(limit),
		})
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) listSEOOpportunities(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
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
	params := db.ListSEOOpportunitiesParams{
		ProjectID: projectID,
		Type:      r.URL.Query().Get("type"),
		Status:    r.URL.Query().Get("status"),
		LimitRows: int32(limit),
	}
	if !cursor.IsZero() {
		params.CursorCreatedAt = pgutil.TS(cursor)
	}
	opps, err := s.Q.ListSEOOpportunities(r.Context(), params)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, opps)
}

func (s *Server) getSEOOpportunity(w http.ResponseWriter, r *http.Request) {
	projectID, oppID, ok := s.seoIDs(w, r, "opportunityID")
	if !ok {
		return
	}
	opp, err := s.Q.GetSEOOpportunity(r.Context(), db.GetSEOOpportunityParams{ID: oppID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "opportunity not found")
		return
	}
	writeJSON(w, http.StatusOK, opp)
}

func (s *Server) acceptSEOOpportunity(w http.ResponseWriter, r *http.Request) {
	s.updateSEOOpportunityStatus(w, r, "accepted")
}

func (s *Server) dismissSEOOpportunity(w http.ResponseWriter, r *http.Request) {
	s.updateSEOOpportunityStatus(w, r, "dismissed")
}

func (s *Server) updateSEOOpportunityStatus(w http.ResponseWriter, r *http.Request, status string) {
	projectID, oppID, ok := s.seoIDs(w, r, "opportunityID")
	if !ok {
		return
	}
	opp, err := s.Q.UpdateSEOOpportunityStatus(r.Context(), db.UpdateSEOOpportunityStatusParams{
		ID:        oppID,
		ProjectID: projectID,
		Status:    status,
	})
	if err != nil {
		writeErr(w, http.StatusNotFound, "opportunity not found")
		return
	}
	writeJSON(w, http.StatusOK, opp)
}

func (s *Server) createSEOContentAction(w http.ResponseWriter, r *http.Request) {
	projectID, oppID, ok := s.seoIDs(w, r, "opportunityID")
	if !ok {
		return
	}
	var in struct {
		ActionType string `json:"action_type"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	opp, err := s.Q.GetSEOOpportunity(r.Context(), db.GetSEOOpportunityParams{ID: oppID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "opportunity not found")
		return
	}
	actionType := strings.TrimSpace(in.ActionType)
	if actionType == "" && opp.RecommendedAction != nil {
		actionType = *opp.RecommendedAction
	}
	if actionType == "" {
		actionType = "technical SEO fix task"
	}
	var targetHash *string
	if opp.ArticleID.Valid {
		article, err := s.Q.GetArticleForProject(r.Context(), db.GetArticleForProjectParams{
			ID:        uuid.UUID(opp.ArticleID.Bytes),
			ProjectID: projectID,
		})
		if err == nil {
			targetHash = article.ContentHash
		}
	}
	action, err := s.Q.CreateContentAction(r.Context(), db.CreateContentActionParams{
		ProjectID:               projectID,
		OpportunityID:           oppID,
		ActionType:              actionType,
		Status:                  "ready_for_review",
		TargetArticleID:         opp.ArticleID,
		TargetUrl:               opp.PageUrl,
		NormalizedTargetUrl:     strPtrFrom(opp.NormalizedPageUrl),
		TargetContentHashBefore: targetHash,
		BaselineWindow:          json.RawMessage(`{"days":28}`),
		MeasurementWindow:       json.RawMessage(`{"checkpoints_days":[7,14,28]}`),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, _ = s.Q.UpdateSEOOpportunityStatus(r.Context(), db.UpdateSEOOpportunityStatusParams{ID: oppID, ProjectID: projectID, Status: "converted"})
	writeJSON(w, http.StatusCreated, action)
}

func (s *Server) listSEOContentActions(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
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
	params := db.ListContentActionsParams{
		ProjectID: projectID,
		Status:    r.URL.Query().Get("status"),
		LimitRows: int32(limit),
	}
	if !cursor.IsZero() {
		params.CursorCreatedAt = pgutil.TS(cursor)
	}
	actions, err := s.Q.ListContentActions(r.Context(), params)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, actions)
}

func (s *Server) getSEOContentAction(w http.ResponseWriter, r *http.Request) {
	projectID, actionID, ok := s.seoIDs(w, r, "actionID")
	if !ok {
		return
	}
	action, err := s.Q.GetContentAction(r.Context(), db.GetContentActionParams{ID: actionID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "action not found")
		return
	}
	writeJSON(w, http.StatusOK, action)
}

func (s *Server) updateSEOContentActionStatus(w http.ResponseWriter, r *http.Request, status string) {
	projectID, actionID, ok := s.seoIDs(w, r, "actionID")
	if !ok {
		return
	}
	action, err := s.Q.UpdateContentActionStatus(r.Context(), db.UpdateContentActionStatusParams{
		ID:        actionID,
		ProjectID: projectID,
		Status:    status,
	})
	if err != nil {
		writeErr(w, http.StatusNotFound, "action not found")
		return
	}
	writeJSON(w, http.StatusOK, action)
}

func (s *Server) getSEOBrief(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	brief, err := s.seoService().Brief(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, brief)
}

func (s *Server) getSEOSettings(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	var prop *db.SeoProperty
	if p, err := s.Q.GetSEOPropertyForProject(r.Context(), projectID); err == nil {
		prop = &p
	}
	integrations, err := s.Q.ListSEOIntegrations(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"property": prop, "integrations": integrations})
}

func (s *Server) updateSEOSettings(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	var in struct {
		SiteURL        string          `json:"site_url"`
		GSCSiteURL     string          `json:"gsc_site_url"`
		GA4PropertyID  string          `json:"ga4_property_id"`
		Normalize      json.RawMessage `json:"url_normalization_config"`
		DefaultCountry string          `json:"default_country"`
		DefaultLang    string          `json:"default_language"`
		CredentialRef  string          `json:"gsc_credential_ref"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(in.SiteURL) == "" {
		writeErr(w, http.StatusBadRequest, "site_url required")
		return
	}
	if len(in.Normalize) == 0 {
		in.Normalize = json.RawMessage(`{}`)
	}
	prop, err := s.Q.UpsertSEOProperty(r.Context(), db.UpsertSEOPropertyParams{
		ProjectID:              projectID,
		SiteUrl:                strings.TrimSpace(in.SiteURL),
		GscSiteUrl:             strPtrFrom(in.GSCSiteURL),
		Ga4PropertyID:          strPtrFrom(in.GA4PropertyID),
		UrlNormalizationConfig: in.Normalize,
		DefaultCountry:         strPtrFrom(in.DefaultCountry),
		DefaultLanguage:        strPtrFrom(in.DefaultLang),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	status := "missing"
	var verified pgtype.Timestamptz
	if strings.TrimSpace(in.CredentialRef) != "" {
		status = "connected"
		verified = pgutil.TS(time.Now().UTC())
	}
	gscIntegration, err := s.Q.UpsertSEOIntegration(r.Context(), db.UpsertSEOIntegrationParams{
		ProjectID:      projectID,
		Provider:       seopkg.ProviderGSC,
		Status:         status,
		CredentialRef:  strPtrFrom(in.CredentialRef),
		LastVerifiedAt: verified,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var ga4Integration *db.SeoIntegration
	if strings.TrimSpace(in.GA4PropertyID) != "" || strings.TrimSpace(in.CredentialRef) != "" {
		row, err := s.Q.UpsertSEOIntegration(r.Context(), db.UpsertSEOIntegrationParams{
			ProjectID:      projectID,
			Provider:       seopkg.ProviderGA4,
			Status:         status,
			CredentialRef:  strPtrFrom(in.CredentialRef),
			LastVerifiedAt: verified,
		})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		ga4Integration = &row
	}
	writeJSON(w, http.StatusOK, map[string]any{"property": prop, "integration": gscIntegration, "ga4_integration": ga4Integration})
}

func (s *Server) seoIDs(w http.ResponseWriter, r *http.Request, param string) (uuid.UUID, uuid.UUID, bool) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return uuid.Nil, uuid.Nil, false
	}
	entityID, err := uuid.Parse(chi.URLParam(r, param))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad "+param)
		return uuid.Nil, uuid.Nil, false
	}
	return projectID, entityID, true
}

func parseLimit(r *http.Request, def, max int) (int, error) {
	limit := def
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			if err == nil {
				err = fmt.Errorf("limit must be positive")
			}
			return 0, err
		}
		limit = n
	}
	if limit > max {
		limit = max
	}
	return limit, nil
}

func parseCursor(r *http.Request) (time.Time, error) {
	raw := r.URL.Query().Get("cursor")
	if raw == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, raw)
}

func strPtrFrom(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
