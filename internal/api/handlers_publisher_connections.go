package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/publisher"
	"github.com/citeloop/citeloop/internal/secretbox"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type publisherConnectionDTO struct {
	ID                      uuid.UUID       `json:"id"`
	ProjectID               uuid.UUID       `json:"project_id"`
	Kind                    string          `json:"kind"`
	Label                   string          `json:"label"`
	Status                  string          `json:"status"`
	IsDefault               bool            `json:"is_default"`
	Capabilities            json.RawMessage `json:"capabilities"`
	CapabilitySchemaVersion int32           `json:"capability_schema_version"`
	CredentialConfigured    bool            `json:"credential_configured"`
	Config                  json.RawMessage `json:"config"`
	LastVerifiedAt          any             `json:"last_verified_at,omitempty"`
	LastError               *string         `json:"last_error,omitempty"`
}

func (s *Server) listPublisherConnections(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	if s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	rows, err := s.Q.ListPublisherConnections(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]publisherConnectionDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, publisherConnectionResponse(row))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) upsertGitHubNextJSPublisherConnection(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := rejectPublisherConnectionSecrets(raw); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var in struct {
		Label         string `json:"label"`
		Repo          string `json:"repo"`
		Branch        string `json:"branch"`
		ContentDir    string `json:"content_dir"`
		BaseURL       string `json:"base_url"`
		PublishMode   string `json:"publish_mode"`
		CredentialRef string `json:"credential_ref"`
	}
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	cfgRaw := mustPublisherJSON(map[string]string{
		"repo":         strings.TrimSpace(in.Repo),
		"branch":       strings.TrimSpace(in.Branch),
		"content_dir":  strings.TrimSpace(in.ContentDir),
		"base_url":     strings.TrimSpace(in.BaseURL),
		"publish_mode": strings.TrimSpace(in.PublishMode),
	})
	if _, err := publisher.ParseGitHubNextJSConfig(cfgRaw); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		label = "GitHub/Next.js"
	}
	credentialRef := strPtr(strings.TrimSpace(in.CredentialRef))
	conn, err := s.Q.UpsertDefaultPublisherConnection(r.Context(), db.UpsertDefaultPublisherConnectionParams{
		ProjectID:               projectID,
		Kind:                    publisher.ConnectionKindGitHubNextJS,
		Label:                   label,
		Status:                  "missing",
		Capabilities:            publisher.GitHubNextJSCapabilities().JSON(),
		CapabilitySchemaVersion: 1,
		CredentialRef:           credentialRef,
		Config:                  cfgRaw,
		LastVerifiedAt:          pgtype.Timestamptz{},
		LastError:               nil,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, publisherConnectionResponse(conn))
}

func (s *Server) testPublisherConnection(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	connectionID, err := uuid.Parse(chi.URLParam(r, "connectionID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad publisher connection id")
		return
	}
	if s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	conn, err := s.Q.GetPublisherConnectionForProject(r.Context(), db.GetPublisherConnectionForProjectParams{ID: connectionID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "publisher connection not found")
		return
	}
	if conn.Kind != publisher.ConnectionKindGitHubNextJS {
		writeErr(w, http.StatusBadRequest, "publisher connection test not supported for "+conn.Kind)
		return
	}
	if _, err := publisher.ParseGitHubNextJSConfig(conn.Config); err != nil {
		updated, markErr := s.Q.MarkPublisherConnectionError(r.Context(), db.MarkPublisherConnectionErrorParams{ID: connectionID, ProjectID: projectID, LastError: strPtr(err.Error())})
		if markErr == nil {
			writeJSON(w, http.StatusBadRequest, publisherConnectionResponse(updated))
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	token, err := s.publisherCredentialToken(r.Context(), projectID, conn)
	if err != nil {
		msg := "publisher credential cannot be resolved"
		updated, markErr := s.Q.MarkPublisherConnectionError(r.Context(), db.MarkPublisherConnectionErrorParams{ID: connectionID, ProjectID: projectID, LastError: &msg})
		if markErr == nil {
			writeJSON(w, http.StatusFailedDependency, publisherConnectionResponse(updated))
			return
		}
		writeErr(w, http.StatusFailedDependency, msg)
		return
	}
	if token == "" {
		msg := "publisher credential unavailable"
		updated, markErr := s.Q.MarkPublisherConnectionError(r.Context(), db.MarkPublisherConnectionErrorParams{ID: connectionID, ProjectID: projectID, LastError: &msg})
		if markErr == nil {
			writeJSON(w, http.StatusFailedDependency, publisherConnectionResponse(updated))
			return
		}
		writeErr(w, http.StatusFailedDependency, msg)
		return
	}
	updated, err := s.Q.MarkPublisherConnectionVerified(r.Context(), db.MarkPublisherConnectionVerifiedParams{ID: connectionID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, publisherConnectionResponse(updated))
}

func (s *Server) upsertPublisherCredential(w http.ResponseWriter, r *http.Request) {
	projectID, connectionID, ok := s.publisherConnectionPathIDs(w, r)
	if !ok {
		return
	}
	if s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	if s.Env.NotificationSecretKey == "" {
		writeErr(w, http.StatusInternalServerError, "NOTIFICATION_SECRET_KEY is required")
		return
	}
	var in struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	kind := strings.TrimSpace(in.Kind)
	if kind == "" {
		kind = publisher.CredentialKindGitHubToken
	}
	if kind != publisher.CredentialKindGitHubToken {
		writeErr(w, http.StatusBadRequest, "publisher credential kind not supported")
		return
	}
	value := strings.TrimSpace(in.Value)
	if value == "" {
		writeErr(w, http.StatusBadRequest, "publisher credential value is required")
		return
	}
	conn, err := s.Q.GetPublisherConnectionForProject(r.Context(), db.GetPublisherConnectionForProjectParams{ID: connectionID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "publisher connection not found")
		return
	}
	if conn.Kind != publisher.ConnectionKindGitHubNextJS {
		writeErr(w, http.StatusBadRequest, "publisher credential not supported for "+conn.Kind)
		return
	}
	encrypted, err := secretbox.EncryptString(value, s.Env.NotificationSecretKey)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "publisher credential cannot be encrypted")
		return
	}
	cred, err := s.Q.UpsertPublisherCredential(r.Context(), db.UpsertPublisherCredentialParams{
		ProjectID:      projectID,
		ConnectionID:   connectionID,
		Kind:           kind,
		EncryptedValue: encrypted,
		RedactedValue:  publisher.RedactCredentialValue(kind, value),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	ref := publisher.PublisherCredentialRef(cred.ID)
	updated, err := s.Q.SetPublisherConnectionCredentialRef(r.Context(), db.SetPublisherConnectionCredentialRefParams{
		ID:            connectionID,
		ProjectID:     projectID,
		CredentialRef: &ref,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, publisherConnectionResponse(updated))
}

func (s *Server) revokePublisherCredential(w http.ResponseWriter, r *http.Request) {
	projectID, connectionID, ok := s.publisherConnectionPathIDs(w, r)
	if !ok {
		return
	}
	if s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	conn, err := s.Q.GetPublisherConnectionForProject(r.Context(), db.GetPublisherConnectionForProjectParams{ID: connectionID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "publisher connection not found")
		return
	}
	if conn.Kind != publisher.ConnectionKindGitHubNextJS {
		writeErr(w, http.StatusBadRequest, "publisher credential not supported for "+conn.Kind)
		return
	}
	if _, err := s.Q.RevokePublisherCredentialForConnection(r.Context(), db.RevokePublisherCredentialForConnectionParams{
		ProjectID:    projectID,
		ConnectionID: connectionID,
		Kind:         publisher.CredentialKindGitHubToken,
	}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	updated, err := s.Q.ClearPublisherConnectionCredentialRef(r.Context(), db.ClearPublisherConnectionCredentialRefParams{ID: connectionID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, publisherConnectionResponse(updated))
}

func (s *Server) publisherConnectionPathIDs(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return uuid.UUID{}, uuid.UUID{}, false
	}
	connectionID, err := uuid.Parse(chi.URLParam(r, "connectionID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad publisher connection id")
		return uuid.UUID{}, uuid.UUID{}, false
	}
	return projectID, connectionID, true
}

func publisherConnectionResponse(row db.PublisherConnection) publisherConnectionDTO {
	return publisherConnectionDTO{
		ID:                      row.ID,
		ProjectID:               row.ProjectID,
		Kind:                    row.Kind,
		Label:                   row.Label,
		Status:                  row.Status,
		IsDefault:               row.IsDefault,
		Capabilities:            safeJSON(row.Capabilities, "{}"),
		CapabilitySchemaVersion: row.CapabilitySchemaVersion,
		CredentialConfigured:    row.CredentialRef != nil && strings.TrimSpace(*row.CredentialRef) != "",
		Config:                  sanitizePublisherConfig(row.Config),
		LastVerifiedAt:          nullableTime(row.LastVerifiedAt),
		LastError:               row.LastError,
	}
}

func rejectPublisherConnectionSecrets(raw []byte) error {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	return walkPublisherJSON(v)
}

func walkPublisherJSON(v any) error {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			if isSecretLikePublisherKey(k) {
				return fmt.Errorf("publisher connection field %q must be stored as a credential_ref, not raw secret material", k)
			}
			if err := walkPublisherJSON(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range x {
			if err := walkPublisherJSON(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func isSecretLikePublisherKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
	if normalized == "credential_ref" {
		return false
	}
	return strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "api_key") ||
		(strings.Contains(normalized, "webhook") && strings.Contains(normalized, "url"))
}

func sanitizePublisherConfig(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("{}")
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return json.RawMessage("{}")
	}
	for k := range m {
		if isSecretLikePublisherKey(k) {
			delete(m, k)
		}
	}
	return mustPublisherJSON(m)
}

func safeJSON(raw json.RawMessage, fallback string) json.RawMessage {
	if len(raw) == 0 || !json.Valid(raw) {
		return json.RawMessage(fallback)
	}
	return raw
}

func (s *Server) publisherCredentialToken(ctx context.Context, projectID uuid.UUID, conn db.PublisherConnection) (string, error) {
	if conn.CredentialRef == nil {
		return "", nil
	}
	ref := strings.TrimSpace(*conn.CredentialRef)
	switch ref {
	case "env:GITHUB_TOKEN", "GITHUB_TOKEN":
		return strings.TrimSpace(s.Env.GitHubToken), nil
	default:
		credentialID, ok := publisher.ParsePublisherCredentialRef(ref)
		if !ok {
			return "", nil
		}
		if s.Q == nil {
			return "", errors.New("database unavailable")
		}
		if s.Env.NotificationSecretKey == "" {
			return "", errors.New("NOTIFICATION_SECRET_KEY is required")
		}
		cred, err := s.Q.GetActivePublisherCredential(ctx, db.GetActivePublisherCredentialParams{
			ID:           credentialID,
			ProjectID:    projectID,
			ConnectionID: conn.ID,
		})
		if err != nil {
			return "", err
		}
		return secretbox.DecryptString(cred.EncryptedValue, s.Env.NotificationSecretKey)
	}
}

func mustPublisherJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func nullableTime(ts pgtype.Timestamptz) any {
	if !ts.Valid {
		return nil
	}
	return ts.Time
}
