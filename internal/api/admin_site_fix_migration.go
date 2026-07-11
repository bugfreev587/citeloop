package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/citeloop/citeloop/internal/sitefix"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type siteFixMigrationService interface {
	DryRun(context.Context, uuid.UUID, string) (sitefix.MigrationDryRunReport, error)
	Apply(context.Context, uuid.UUID, string, string) (sitefix.MigrationBatchReport, error)
	Rollback(context.Context, uuid.UUID, uuid.UUID, string, string) (sitefix.MigrationRollbackReport, error)
	Report(context.Context, uuid.UUID, uuid.UUID) (sitefix.MigrationBatchReport, error)
}

type siteFixMigrationMutationRequest struct {
	ExpectedSnapshotHash string `json:"expected_snapshot_hash"`
}

func (s *Server) adminSiteFixMigrationService() siteFixMigrationService {
	if s.SiteFixMigration != nil {
		return s.SiteFixMigration
	}
	if s.Pool == nil {
		return nil
	}
	return sitefix.NewMigrationService(sitefix.NewPostgresMigrationStore(s.Pool))
}

func (s *Server) dryRunAdminSiteFixMigration(w http.ResponseWriter, r *http.Request) {
	projectID, ok := adminMigrationUUID(w, chi.URLParam(r, "projectID"), "project")
	if !ok {
		return
	}
	service := s.adminSiteFixMigrationService()
	if service == nil {
		writeErr(w, http.StatusServiceUnavailable, "migration service unavailable")
		return
	}
	report, err := service.DryRun(r.Context(), projectID, s.ownerID(r))
	if err != nil {
		s.writeAdminMigrationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) applyAdminSiteFixMigration(w http.ResponseWriter, r *http.Request) {
	projectID, ok := adminMigrationUUID(w, chi.URLParam(r, "projectID"), "project")
	if !ok {
		return
	}
	request, ok := decodeMigrationMutation(w, r)
	if !ok {
		return
	}
	service := s.adminSiteFixMigrationService()
	if service == nil {
		writeErr(w, http.StatusServiceUnavailable, "migration service unavailable")
		return
	}
	report, err := service.Apply(r.Context(), projectID, request.ExpectedSnapshotHash, s.ownerID(r))
	if err != nil {
		s.writeAdminMigrationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) rollbackAdminSiteFixMigration(w http.ResponseWriter, r *http.Request) {
	projectID, ok := adminMigrationUUID(w, chi.URLParam(r, "projectID"), "project")
	if !ok {
		return
	}
	batchID, ok := adminMigrationUUID(w, chi.URLParam(r, "batchID"), "batch")
	if !ok {
		return
	}
	request, ok := decodeMigrationMutation(w, r)
	if !ok {
		return
	}
	service := s.adminSiteFixMigrationService()
	if service == nil {
		writeErr(w, http.StatusServiceUnavailable, "migration service unavailable")
		return
	}
	report, err := service.Rollback(r.Context(), projectID, batchID, request.ExpectedSnapshotHash, s.ownerID(r))
	if err != nil {
		s.writeAdminMigrationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) getAdminSiteFixMigrationReport(w http.ResponseWriter, r *http.Request) {
	projectID, ok := adminMigrationUUID(w, chi.URLParam(r, "projectID"), "project")
	if !ok {
		return
	}
	batchID, ok := adminMigrationUUID(w, chi.URLParam(r, "batchID"), "batch")
	if !ok {
		return
	}
	service := s.adminSiteFixMigrationService()
	if service == nil {
		writeErr(w, http.StatusServiceUnavailable, "migration service unavailable")
		return
	}
	report, err := service.Report(r.Context(), projectID, batchID)
	if err != nil {
		s.writeAdminMigrationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func decodeMigrationMutation(w http.ResponseWriter, r *http.Request) (siteFixMigrationMutationRequest, bool) {
	var request siteFixMigrationMutationRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || strings.TrimSpace(request.ExpectedSnapshotHash) == "" {
		writeErr(w, http.StatusConflict, "expected_snapshot_hash is required")
		return request, false
	}
	request.ExpectedSnapshotHash = strings.TrimSpace(request.ExpectedSnapshotHash)
	return request, true
}

func adminMigrationUUID(w http.ResponseWriter, raw, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(raw)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad "+name+" id")
		return uuid.Nil, false
	}
	return id, true
}

func (s *Server) writeAdminMigrationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sitefix.ErrMigrationSnapshotDrift):
		writeErr(w, http.StatusConflict, "migration snapshot drifted; run dry-run again")
	case errors.Is(err, sitefix.ErrMigrationRollbackBlocked):
		writeErr(w, http.StatusConflict, "rollback blocked because canonical writes cannot be losslessly restored")
	default:
		if s.Log != nil {
			s.Log.Error("site fix migration failed", "error", err)
		}
		writeErr(w, http.StatusInternalServerError, "site fix migration failed")
	}
}
