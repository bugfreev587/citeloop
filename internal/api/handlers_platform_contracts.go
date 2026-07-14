package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/platformcontract"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Server) getPlatformContractCapabilities(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid project id")
		return
	}
	contracts, err := s.Q.ListActivePlatformContentContracts(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	contexts, err := s.Q.ListPlatformTargetContexts(r.Context(), db.ListPlatformTargetContextsParams{ProjectID: projectID, Platform: ""})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	assetType := strings.TrimSpace(r.URL.Query().Get("asset_type"))
	if assetType == "" {
		assetType = "long_form_article"
	}
	matrix, err := platformcontract.BuildMatrix(platformcontract.MatrixInput{
		AssetType: assetType, Contracts: contracts, Contexts: contexts,
		ConnectionReady: map[string]bool{}, Now: time.Now().UTC(),
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, matrix)
}

func (s *Server) listPlatformTargetContexts(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid project id")
		return
	}
	rows, err := s.Q.ListPlatformTargetContexts(r.Context(), db.ListPlatformTargetContextsParams{
		ProjectID: projectID, Platform: strings.ToLower(strings.TrimSpace(r.URL.Query().Get("platform"))),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, emptySlice(rows))
}

func (s *Server) confirmPlatformTargetContext(w http.ResponseWriter, r *http.Request) {
	var input platformcontract.ConfirmTargetContextInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.persistPlatformTargetContext(w, r, input, uuid.Nil)
}

func (s *Server) reconfirmPlatformTargetContext(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid project id")
		return
	}
	contextID, err := uuid.Parse(chi.URLParam(r, "contextID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid context id")
		return
	}
	row, err := s.Q.GetPlatformTargetContextForProject(r.Context(), db.GetPlatformTargetContextForProjectParams{ID: contextID, ProjectID: projectID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "target context not found")
		} else {
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	var allowed []string
	if err := json.Unmarshal(row.AllowedPostTypes, &allowed); err != nil {
		writeErr(w, http.StatusInternalServerError, "stored target context is invalid")
		return
	}
	s.persistPlatformTargetContext(w, r, platformcontract.ConfirmTargetContextInput{
		Platform: row.Platform, TargetKey: row.TargetKey, SourceURL: stringValue(row.SourceUrl),
		RulesURL: stringValue(row.RulesUrl), RulesText: row.RulesText, AllowedPostTypes: allowed,
		RequiredFlair: stringValue(row.RequiredFlair), LinkPolicy: row.LinkPolicy,
		SelfPromotionPolicy: row.SelfPromotionPolicy, DisclosureRequirements: row.DisclosureRequirements,
		Notes: row.Notes, Verified: true,
	}, row.ID)
}

func (s *Server) persistPlatformTargetContext(w http.ResponseWriter, r *http.Request, input platformcontract.ConfirmTargetContextInput, supersedes uuid.UUID) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid project id")
		return
	}
	prepared, err := platformcontract.PrepareTargetContext(input, 0, time.Now().UTC())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	tx, err := s.Pool.Begin(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(r.Context())
	q := db.New(tx)
	version, err := q.NextPlatformTargetContextVersion(r.Context(), db.NextPlatformTargetContextVersionParams{
		ProjectID: projectID, Platform: prepared.Platform, TargetKey: prepared.TargetKey,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	allowed, _ := json.Marshal(prepared.AllowedPostTypes)
	created, err := q.CreatePlatformTargetContext(r.Context(), db.CreatePlatformTargetContextParams{
		ProjectID: projectID, Platform: prepared.Platform, TargetKey: prepared.TargetKey, Version: version,
		SourceKind: prepared.SourceKind, SourceUrl: optionalString(prepared.SourceURL), RulesUrl: optionalString(prepared.RulesURL),
		RulesText: prepared.RulesText, AllowedPostTypes: allowed, RequiredFlair: optionalString(prepared.RequiredFlair),
		LinkPolicy: prepared.LinkPolicy, SelfPromotionPolicy: prepared.SelfPromotionPolicy,
		DisclosureRequirements: prepared.DisclosureRequirements, Notes: prepared.Notes, ContentHash: prepared.ContentHash,
		SupersedesContextID: pgtype.UUID{Bytes: supersedes, Valid: supersedes != uuid.Nil},
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := q.SupersedeCurrentPlatformTargetContext(r.Context(), db.SupersedeCurrentPlatformTargetContextParams{
		ProjectID: projectID, Platform: prepared.Platform, TargetKey: prepared.TargetKey,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	actor := s.ownerID(r)
	confirmed, err := q.ConfirmPlatformTargetContext(r.Context(), db.ConfirmPlatformTargetContextParams{
		ConfirmedBy: optionalString(actor), ConfirmedAt: pgtype.Timestamptz{Time: prepared.ConfirmedAt, Valid: true},
		ExpiresAt: pgtype.Timestamptz{Time: prepared.ExpiresAt, Valid: true}, ID: created.ID, ProjectID: projectID,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, confirmed)
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
