package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/citeloop/citeloop/internal/workflow"
	"github.com/jackc/pgx/v5/pgtype"
)

// User-visible work types and routing/approval provenance for execution items
// (PRD-CiteLoop-Opportunity-Review-and-Work-Queues §5, §6, §11).
const (
	WorkTypeCreateContent = "create_content"
	WorkTypeImprovePage   = "improve_page"
	WorkTypeFixSiteIssue  = "fix_site_issue"

	RoutingSourceSystem       = "system_recommendation"
	RoutingSourceUserOverride = "user_override"
	RoutingSourcePolicy       = "policy"

	ApprovalSourceHumanReview     = "human_review"
	ApprovalSourceAutopilotPolicy = "autopilot_policy"
)

var fixSiteIssueOpportunityTypes = map[string]bool{
	"internal_link_gap":          true,
	"schema_gap":                 true,
	"technical_visibility_issue": true,
	"geo_crawler_access_blocked": true,
}

var improvePageOpportunityTypes = map[string]bool{
	"gsc_low_ctr_query":           true,
	"gsc_striking_distance_query": true,
	"gsc_content_decay":           true,
	"thin_evidence_page":          true,
	"gsc_query_cannibalization":   true,
	"citation_fact_expansion":     true,
}

// dualRouteOpportunityTypes may fit either Improve Page or Create Content;
// the recommended action decides the default and the user may override (§6.2).
var dualRouteOpportunityTypes = map[string]string{
	"gsc_query_gap":            WorkTypeCreateContent,
	"cold_start_evidence_page": WorkTypeCreateContent,
}

func opportunityRoutingText(opp db.SeoOpportunity) string {
	parts := []string{opp.Type}
	if opp.RecommendedAction != nil {
		parts = append(parts, *opp.RecommendedAction)
	}
	if opp.ExpectedImpact != nil {
		parts = append(parts, *opp.ExpectedImpact)
	}
	text := strings.ToLower(strings.Join(parts, " "))
	return strings.NewReplacer("_", " ", "-", " ").Replace(text)
}

func containsAny(text string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

// workTypeForOpportunity is the backend source of truth for the §6.1 mapping.
// It mirrors the presentation heuristics so system recommendation and stored
// routing agree.
func workTypeForOpportunity(opp db.SeoOpportunity) string {
	oppType := strings.ToLower(strings.TrimSpace(opp.Type))
	if fixSiteIssueOpportunityTypes[oppType] {
		return WorkTypeFixSiteIssue
	}
	if improvePageOpportunityTypes[oppType] {
		return WorkTypeImprovePage
	}
	if preferred, ok := dualRouteOpportunityTypes[oppType]; ok {
		return preferred
	}
	text := opportunityRoutingText(opp)
	if containsAny(text, "internal link", "schema", "index", "sitemap", "crawler", "robots", "canonical") {
		return WorkTypeFixSiteIssue
	}
	if containsAny(text, "refresh", "decay", "ctr", "title", "meta", "near", "cannibal", "consolidat", "evidence block", "source backed") {
		return WorkTypeImprovePage
	}
	return WorkTypeCreateContent
}

// allowedWorkTypesForOpportunity lists routes a user may pick in the review
// drawer. Fix Site Issue stays locked: technical certainty restricts the
// choice (§6.2).
func allowedWorkTypesForOpportunity(opp db.SeoOpportunity) []string {
	if workTypeForOpportunity(opp) == WorkTypeFixSiteIssue {
		return []string{WorkTypeFixSiteIssue}
	}
	return []string{WorkTypeCreateContent, WorkTypeImprovePage}
}

func workTypeAllowed(opp db.SeoOpportunity, workType string) bool {
	for _, allowed := range allowedWorkTypesForOpportunity(opp) {
		if allowed == workType {
			return true
		}
	}
	return false
}

func (s *Server) snoozeSEOOpportunity(w http.ResponseWriter, r *http.Request) {
	projectID, oppID, ok := s.seoIDs(w, r, "opportunityID")
	if !ok {
		return
	}
	var in struct {
		Days   int    `json:"days"`
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in.Days <= 0 {
		in.Days = 7
	}
	if in.Days > 90 {
		writeErr(w, http.StatusBadRequest, "snooze window too long")
		return
	}
	var reason *string
	if trimmed := strings.TrimSpace(in.Reason); trimmed != "" {
		reason = &trimmed
	}
	opp, err := s.Q.SnoozeSEOOpportunity(r.Context(), db.SnoozeSEOOpportunityParams{
		ID:           oppID,
		ProjectID:    projectID,
		SnoozedUntil: pgutil.TS(time.Now().UTC().Add(time.Duration(in.Days) * 24 * time.Hour)),
		SnoozeReason: reason,
	})
	if err != nil {
		writeErr(w, http.StatusNotFound, "opportunity not found or not snoozable")
		return
	}
	if err := s.recordSEOOpportunityReviewState(r.Context(), s.Q, projectID, oppID, pgtype.UUID{}, "snoozed", opp.SnoozedUntil); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, opp)
}

func (s *Server) unsnoozeSEOOpportunity(w http.ResponseWriter, r *http.Request) {
	projectID, oppID, ok := s.seoIDs(w, r, "opportunityID")
	if !ok {
		return
	}
	opp, err := s.Q.UnsnoozeSEOOpportunity(r.Context(), db.UnsnoozeSEOOpportunityParams{ID: oppID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "opportunity not found or not snoozed")
		return
	}
	writeJSON(w, http.StatusOK, opp)
}

// watchSEOOpportunity implements the Watch Result decision: it creates a
// Results Watchlist item instead of an execution item (§11.2).
func (s *Server) watchSEOOpportunity(w http.ResponseWriter, r *http.Request) {
	projectID, oppID, ok := s.seoIDs(w, r, "opportunityID")
	if !ok {
		return
	}
	var in struct {
		ObservationWindowDays int `json:"observation_window_days"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in.ObservationWindowDays <= 0 {
		in.ObservationWindowDays = 28
	}
	if in.ObservationWindowDays > 180 {
		writeErr(w, http.StatusBadRequest, "observation window too long")
		return
	}
	if _, err := s.Q.GetSEOOpportunity(r.Context(), db.GetSEOOpportunityParams{ID: oppID, ProjectID: projectID}); err != nil {
		writeErr(w, http.StatusNotFound, "opportunity not found")
		return
	}
	item, err := s.Q.CreateSEOWatchlistItem(r.Context(), db.CreateSEOWatchlistItemParams{
		ProjectID:             projectID,
		SourceOpportunityID:   oppID,
		ObservationWindowDays: int32(in.ObservationWindowDays),
		DueAt:                 pgutil.TS(time.Now().UTC().Add(time.Duration(in.ObservationWindowDays) * 24 * time.Hour)),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := s.Q.UpdateSEOOpportunityStatus(r.Context(), db.UpdateSEOOpportunityStatusParams{ID: oppID, ProjectID: projectID, Status: "watching"}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.recordSEOOpportunityReviewState(r.Context(), s.Q, projectID, oppID, pgtype.UUID{}, "watching", pgtype.Timestamptz{}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.enqueueWorkflowEvent(r.Context(), projectID, workflow.EventOpportunityReviewed, "seo_opportunity", oppID, workflowDedupeKey(workflow.EventOpportunityReviewed, projectID, oppID, "watching"), map[string]any{
		"opportunity_id":    oppID,
		"watchlist_item_id": item.ID,
		"status":            "watching",
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) listSEOWatchlist(w http.ResponseWriter, r *http.Request) {
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
	// Promote expired observation windows to their due state at read time so
	// the queue is correct without a scheduler tick.
	if _, err := s.Q.MarkDueSEOWatchlistItems(r.Context(), projectID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	items, err := s.Q.ListSEOWatchlistItems(r.Context(), db.ListSEOWatchlistItemsParams{
		ProjectID: projectID,
		Status:    r.URL.Query().Get("status"),
		LimitRows: int32(limit),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, emptySlice(items))
}

func (s *Server) closeSEOWatchlistItem(w http.ResponseWriter, r *http.Request) {
	projectID, itemID, ok := s.seoIDs(w, r, "watchlistItemID")
	if !ok {
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "closed"
	}
	if status != "closed" && status != "learned" {
		writeErr(w, http.StatusBadRequest, "status must be closed or learned")
		return
	}
	item, err := s.Q.CloseSEOWatchlistItem(r.Context(), db.CloseSEOWatchlistItemParams{
		ID:        itemID,
		ProjectID: projectID,
		Status:    status,
	})
	if err != nil {
		writeErr(w, http.StatusNotFound, "watchlist item not found")
		return
	}
	writeJSON(w, http.StatusOK, item)
}
