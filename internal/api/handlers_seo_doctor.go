package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	seopkg "github.com/citeloop/citeloop/internal/seo"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const seoDoctorRunTimeout = 10 * time.Minute

type seoDoctorResponse struct {
	Run         *db.SeoDoctorRun      `json:"run"`
	Findings    []db.SeoDoctorFinding `json:"findings"`
	HumanReport any                   `json:"human_report,omitempty"`
}

func (s *Server) registerDoctorRoutes(r chi.Router, prefix string) {
	r.Get(prefix, s.getSEODoctor)
	r.Post(prefix+"/runs", s.createSEODoctorRun)
	r.Get(prefix+"/runs/{runID}", s.getSEODoctorRun)
	r.Get(prefix+"/runs/{runID}/findings", s.listSEODoctorRunFindings)
	r.Get(prefix+"/latest", s.getLatestSEODoctor)
	r.Post(prefix+"/findings/{findingID}/dismiss", s.dismissSEODoctorFinding)
	r.Post(prefix+"/findings/{findingID}/convert", s.convertSEODoctorFinding)
}

func (s *Server) registerCanonicalDoctorSiteFixRoutes(r chi.Router, prefix string) {
	r.Post(prefix+"/findings/{findingID}/site-fixes", s.createDoctorSiteFix)
	r.Get(prefix+"/site-fixes", s.listDoctorSiteFixes)
	r.Get(prefix+"/site-fixes/{fixID}", s.getDoctorSiteFix)
	r.Post(prefix+"/site-fixes/{fixID}/approve", s.approveDoctorSiteFix)
	r.Post(prefix+"/site-fixes/{fixID}/apply", s.applyDoctorSiteFix)
	r.Post(prefix+"/site-fixes/{fixID}/verify", s.verifyDoctorSiteFix)
	r.Post(prefix+"/site-fixes/{fixID}/terminate", s.terminateDoctorSiteFix)
}

func (s *Server) getSEODoctor(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	if active, err := s.Q.GetActiveSEODoctorRun(r.Context(), projectID); err == nil {
		writeJSON(w, http.StatusOK, seoDoctorResponse{Run: &active, Findings: []db.SeoDoctorFinding{}})
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	report, err := s.seoServiceForProject(r.Context(), projectID).DoctorLatest(r.Context(), projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusOK, seoDoctorResponse{Findings: []db.SeoDoctorFinding{}})
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeSEODoctorReport(w, http.StatusOK, report)
}

func (s *Server) createSEODoctorRun(w http.ResponseWriter, r *http.Request) {
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
	ownerID := strings.TrimSpace(s.ownerID(r))
	var createdBy *string
	if ownerID != "" {
		createdBy = &ownerID
	}
	run, created, err := s.seoServiceForProject(r.Context(), projectID).StartDoctorRun(r.Context(), seoDoctorRunRequest(projectID, in.SiteURL, createdBy))
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "manual seo doctor run limit") {
			status = http.StatusTooManyRequests
		}
		writeErr(w, status, err.Error())
		return
	}
	if created {
		s.startSEODoctorRun(projectID, run.ID)
		writeJSON(w, http.StatusCreated, run)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) getSEODoctorRun(w http.ResponseWriter, r *http.Request) {
	projectID, runID, ok := s.seoDoctorIDs(w, r, "runID")
	if !ok {
		return
	}
	run, err := s.Q.GetSEODoctorRun(r.Context(), db.GetSEODoctorRunParams{ID: runID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "doctor run not found")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) listSEODoctorRunFindings(w http.ResponseWriter, r *http.Request) {
	projectID, runID, ok := s.seoDoctorIDs(w, r, "runID")
	if !ok {
		return
	}
	findings, err := s.Q.ListSEODoctorFindingsForRun(r.Context(), db.ListSEODoctorFindingsForRunParams{
		ProjectID: projectID,
		RunID:     runID,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, emptySlice(findings))
}

func (s *Server) getLatestSEODoctor(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	report, err := s.seoServiceForProject(r.Context(), projectID).DoctorLatest(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "doctor report not found")
		return
	}
	writeSEODoctorReport(w, http.StatusOK, report)
}

func (s *Server) dismissSEODoctorFinding(w http.ResponseWriter, r *http.Request) {
	projectID, findingID, ok := s.seoDoctorIDs(w, r, "findingID")
	if !ok {
		return
	}
	finding, err := s.seoServiceForProject(r.Context(), projectID).DismissDoctorFinding(r.Context(), projectID, findingID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "doctor finding not found")
		return
	}
	writeJSON(w, http.StatusOK, finding)
}

// convertSEODoctorFinding is the deprecated compatibility alias. It delegates
// to the same canonical Site Fix service as the successor endpoint and never
// writes an Opportunity or Content Action.
func (s *Server) convertSEODoctorFinding(w http.ResponseWriter, r *http.Request) {
	projectID, findingID, ok := s.seoDoctorIDs(w, r, "findingID")
	if !ok {
		return
	}
	w.Header().Set("Deprecation", "true")
	w.Header().Set("Link", "</api/projects/"+projectID.String()+"/doctor/findings/"+findingID.String()+"/site-fixes>; rel=\"successor-version\"")
	s.createDoctorSiteFixForIDs(w, r, projectID, findingID)
}

// firstDoctorFindingURL returns the first non-empty URL from a finding's
// affected_urls / normalized_urls JSON array.
func firstDoctorFindingURL(raw json.RawMessage) string {
	var urls []string
	if err := json.Unmarshal(raw, &urls); err == nil {
		for _, u := range urls {
			if trimmed := strings.TrimSpace(u); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

// doctorFindingOpportunityType maps a finding to one of the fix-site-issue
// opportunity types so workTypeForOpportunity routes it to Site Fixes.
func doctorFindingOpportunityType(f db.SeoDoctorFinding) string {
	text := strings.ToLower(f.Category + " " + f.IssueType + " " + f.FixIntent)
	switch {
	case strings.Contains(text, "internal_link") || strings.Contains(text, "internal link"):
		return "internal_link_gap"
	case strings.Contains(text, "schema") || strings.Contains(text, "structured_data") || strings.Contains(text, "structured data") || strings.Contains(text, "json-ld"):
		return "schema_gap"
	case strings.Contains(text, "geo") || strings.Contains(text, "crawler_access") || strings.Contains(text, "answer engine"):
		return "geo_crawler_access_blocked"
	default:
		return "technical_visibility_issue"
	}
}

// doctorFindingAssetType maps a finding to a direct-action asset type so the
// created content action shows up on the Site Fixes surface.
func doctorFindingAssetType(f db.SeoDoctorFinding) string {
	text := strings.ToLower(f.Category + " " + f.IssueType + " " + f.FixIntent + " " + f.DeveloperInstructions)
	switch {
	case strings.Contains(text, "schema") || strings.Contains(text, "structured_data") || strings.Contains(text, "structured data") || strings.Contains(text, "json-ld"):
		return "schema_patch"
	case strings.Contains(text, "internal_link") || strings.Contains(text, "internal link"):
		return "internal_link_patch"
	case strings.Contains(text, "sitemap"):
		return "sitemap_update"
	case strings.Contains(text, "meta description") || strings.Contains(text, "meta_description") || strings.Contains(text, "metadata") || strings.Contains(text, "title tag"):
		return "metadata_rewrite"
	default:
		return "technical_fix"
	}
}

func doctorFindingPriority(severity string) float64 {
	switch strings.ToUpper(strings.TrimSpace(severity)) {
	case "P0":
		return 92
	case "P1":
		return 82
	case "P2":
		return 70
	default:
		return 60
	}
}

func normalizeDoctorRiskLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "high":
		return "high"
	case "low":
		return "low"
	default:
		return "medium"
	}
}

// doctorFindingOpportunityEvidence preserves the finding's own evidence and adds
// enough context for downstream copy and the evidence fingerprint.
func doctorFindingOpportunityEvidence(f db.SeoDoctorFinding) json.RawMessage {
	base := map[string]any{}
	if len(f.Evidence) > 0 {
		_ = json.Unmarshal(f.Evidence, &base)
	}
	base["source"] = "seo_doctor_finding"
	base["doctor_finding_id"] = f.ID.String()
	base["issue_type"] = f.IssueType
	base["severity"] = f.Severity
	base["category"] = f.Category
	if f.WhyItMatters != "" {
		base["why_it_matters"] = f.WhyItMatters
	}
	if f.FixIntent != "" {
		base["recommended_action"] = f.FixIntent
	}
	b, err := json.Marshal(base)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}

// doctorFindingDiffSnapshot carries the finding's developer instructions,
// likely surfaces, and acceptance tests into the content action so the Site Fix
// AI repair JSON has real fix content.
func doctorFindingDiffSnapshot(f db.SeoDoctorFinding) json.RawMessage {
	change := map[string]any{}
	if instr := strings.TrimSpace(f.DeveloperInstructions); instr != "" {
		change["instruction"] = instr
	}
	var surfaces []string
	if len(f.LikelyFilesOrSurfaces) > 0 {
		_ = json.Unmarshal(f.LikelyFilesOrSurfaces, &surfaces)
	}
	if len(surfaces) > 0 {
		change["likely_surfaces"] = surfaces
	}
	snapshot := map[string]any{
		"output_type": "technical_task",
		"source":      "seo_doctor_finding",
	}
	if len(change) > 0 {
		snapshot["proposed_changes"] = []any{change}
	}
	var acceptance []string
	if len(f.AcceptanceTests) > 0 {
		_ = json.Unmarshal(f.AcceptanceTests, &acceptance)
	}
	if len(acceptance) > 0 {
		snapshot["acceptance_tests"] = acceptance
	}
	b, err := json.Marshal(snapshot)
	if err != nil {
		return nil
	}
	return b
}

func (s *Server) startSEODoctorRun(projectID, runID uuid.UUID) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), seoDoctorRunTimeout)
		defer cancel()
		if _, err := s.seoServiceForProject(ctx, projectID).RunDoctor(ctx, projectID, runID); err != nil {
			log := s.Log
			if log == nil {
				log = slog.Default()
			}
			log.Warn("seo doctor run failed", "project_id", projectID, "run_id", runID, "err", err)
		}
	}()
}

func seoDoctorRunRequest(projectID uuid.UUID, siteURL string, createdBy *string) seopkg.DoctorRunRequest {
	return seopkg.DoctorRunRequest{
		ProjectID:       projectID,
		Trigger:         seopkg.DoctorTriggerManual,
		SiteURL:         siteURL,
		CreatedByUserID: createdBy,
	}
}

func (s *Server) seoDoctorIDs(w http.ResponseWriter, r *http.Request, param string) (uuid.UUID, uuid.UUID, bool) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return uuid.Nil, uuid.Nil, false
	}
	id, err := uuid.Parse(chi.URLParam(r, param))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad "+strings.TrimSuffix(param, "ID")+" id")
		return uuid.Nil, uuid.Nil, false
	}
	return projectID, id, true
}

func writeSEODoctorReport(w http.ResponseWriter, status int, report seopkg.DoctorReport) {
	writeJSON(w, status, seoDoctorResponse{
		Run:         &report.Run,
		Findings:    emptySlice(report.Findings),
		HumanReport: report.Human,
	})
}
