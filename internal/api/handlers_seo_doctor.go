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
	Run                *db.SeoDoctorRun      `json:"run"`
	Findings           []db.SeoDoctorFinding `json:"findings"`
	HumanReport        any                   `json:"human_report,omitempty"`
	AICodingToolReport any                   `json:"ai_coding_tool_report,omitempty"`
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

func (s *Server) convertSEODoctorFinding(w http.ResponseWriter, r *http.Request) {
	projectID, findingID, ok := s.seoDoctorIDs(w, r, "findingID")
	if !ok {
		return
	}
	action, err := s.seoServiceForProject(r.Context(), projectID).ConvertDoctorFinding(r.Context(), projectID, findingID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, action)
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
		Run:                &report.Run,
		Findings:           emptySlice(report.Findings),
		HumanReport:        report.Human,
		AICodingToolReport: report.AICodingToolReport,
	})
}
