package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/sitefix"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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

type adminMigrationReviewItem struct {
	db.MigrationReviewItem
	AgeSeconds int64  `json:"age_seconds"`
	SLAStatus  string `json:"sla_status"`
}

func (s *Server) listAdminMigrationReviews(w http.ResponseWriter, r *http.Request) {
	projectID, ok := adminMigrationUUID(w, chi.URLParam(r, "projectID"), "project")
	if !ok {
		return
	}
	if s.Q == nil {
		writeErr(w, http.StatusServiceUnavailable, "migration review store unavailable")
		return
	}
	minAgeSeconds, err := parseMigrationReviewMinAge(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	overdueOnly := false
	if raw := strings.TrimSpace(r.URL.Query().Get("overdue_only")); raw != "" {
		overdueOnly, err = strconv.ParseBool(raw)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "overdue_only must be true or false")
			return
		}
	}
	status := optionalQuery(r, "status")
	if status != nil && *status != "pending" && *status != "resolved" && *status != "dismissed" {
		writeErr(w, http.StatusBadRequest, "unsupported migration review status")
		return
	}
	items, err := s.Q.ListOperationalMigrationReviewItems(r.Context(), db.ListOperationalMigrationReviewItemsParams{
		ProjectID: projectID, Status: status, InternalOwner: optionalQuery(r, "internal_owner"),
		MinAgeSeconds: minAgeSeconds, OverdueOnly: overdueOnly,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list migration reviews")
		return
	}
	now := time.Now().UTC()
	result := make([]adminMigrationReviewItem, 0, len(items))
	for _, item := range items {
		age := int64(0)
		if item.CreatedAt.Valid && now.After(item.CreatedAt.Time) {
			age = int64(now.Sub(item.CreatedAt.Time).Seconds())
		}
		slaStatus := "within_sla"
		if item.Status != "pending" {
			slaStatus = "closed"
		} else if item.DueAt.Valid && !now.Before(item.DueAt.Time) {
			slaStatus = "overdue"
		}
		result = append(result, adminMigrationReviewItem{MigrationReviewItem: item, AgeSeconds: age, SLAStatus: slaStatus})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func parseMigrationReviewMinAge(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("min_age_seconds"))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, errors.New("min_age_seconds must be a non-negative integer")
	}
	return value, nil
}

type adminMigrationReviewResolutionRequest struct {
	Status             string         `json:"status"`
	ResolutionSnapshot map[string]any `json:"resolution_snapshot"`
}

func (s *Server) resolveAdminMigrationReview(w http.ResponseWriter, r *http.Request) {
	projectID, ok := adminMigrationUUID(w, chi.URLParam(r, "projectID"), "project")
	if !ok {
		return
	}
	reviewID, ok := adminMigrationUUID(w, chi.URLParam(r, "reviewID"), "review")
	if !ok {
		return
	}
	if s.Q == nil {
		writeErr(w, http.StatusServiceUnavailable, "migration review store unavailable")
		return
	}
	var request adminMigrationReviewResolutionRequest
	if err := decodeAdminDiscoveryJSON(r, &request); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	request.Status = strings.TrimSpace(request.Status)
	if request.Status != "resolved" && request.Status != "dismissed" {
		writeErr(w, http.StatusBadRequest, "status must be resolved or dismissed")
		return
	}
	if request.ResolutionSnapshot == nil {
		writeErr(w, http.StatusBadRequest, "resolution_snapshot is required")
		return
	}
	resolution, err := json.Marshal(request.ResolutionSnapshot)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "resolution_snapshot must be a JSON object")
		return
	}
	actor := s.adminAuditActor(r)
	item, err := s.Q.ResolveMigrationReviewItem(r.Context(), db.ResolveMigrationReviewItemParams{
		Status: request.Status, ResolutionSnapshot: resolution, ResolvedBy: &actor,
		ResolvedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}, ProjectID: projectID, ID: reviewID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusConflict, "migration review is missing or already resolved")
			return
		}
		writeErr(w, http.StatusInternalServerError, "could not resolve migration review")
		return
	}
	writeJSON(w, http.StatusOK, item)
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
