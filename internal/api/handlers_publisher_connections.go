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
	Enabled                 bool            `json:"enabled"`
	Capabilities            json.RawMessage `json:"capabilities"`
	CapabilitySchemaVersion int32           `json:"capability_schema_version"`
	CredentialConfigured    bool            `json:"credential_configured"`
	Config                  json.RawMessage `json:"config"`
	LastVerifiedAt          any             `json:"last_verified_at,omitempty"`
	LastError               *string         `json:"last_error,omitempty"`
}

type githubNextJSPublisherInput struct {
	Label         string `json:"label"`
	Repo          string `json:"repo"`
	Branch        string `json:"branch"`
	ContentDir    string `json:"content_dir"`
	BaseURL       string `json:"base_url"`
	PublishMode   string `json:"publish_mode"`
	CredentialRef string `json:"credential_ref"`
}

type devToPublisherInput struct {
	Label    string `json:"label"`
	Username string `json:"username"`
}

type devToConnConfig struct {
	Username string `json:"username,omitempty"`
}

type devToProfile struct {
	Username string `json:"username"`
}

const devToAPIBaseURL = "https://dev.to"

func githubNextJSConfigForUpsert(in githubNextJSPublisherInput, existing json.RawMessage) (json.RawMessage, string, error) {
	cfg := githubConnConfig{
		Repo:           strings.TrimSpace(in.Repo),
		Branch:         strings.TrimSpace(in.Branch),
		ContentDir:     strings.TrimSpace(in.ContentDir),
		BaseURL:        strings.TrimSpace(in.BaseURL),
		PublishMode:    strings.TrimSpace(in.PublishMode),
		InstallationID: strings.TrimSpace(parseGithubConnConfig(existing).InstallationID),
	}
	cfgRaw := mustPublisherJSON(cfg)
	if _, err := publisher.ParseGitHubNextJSConfig(cfgRaw); err != nil {
		return nil, "", err
	}
	status := "missing"
	if cfg.InstallationID != "" {
		status = "connected"
	}
	return cfgRaw, status, nil
}

func devToConfigForUpsert(in devToPublisherInput) (json.RawMessage, string, error) {
	cfg := devToConnConfig{
		Username: strings.TrimSpace(in.Username),
	}
	return mustPublisherJSON(cfg), "missing", nil
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
	var in githubNextJSPublisherInput
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	var existing db.PublisherConnection
	var existingConfig json.RawMessage
	if conn, ok := s.githubConnection(r, projectID); ok {
		existing = conn
		existingConfig = conn.Config
	}
	cfgRaw, status, err := githubNextJSConfigForUpsert(in, existingConfig)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		label = "GitHub/Next.js"
	}
	credentialRef := existing.CredentialRef
	if ref := strings.TrimSpace(in.CredentialRef); ref != "" {
		credentialRef = &ref
	}
	conn, err := s.Q.UpsertDefaultPublisherConnection(r.Context(), db.UpsertDefaultPublisherConnectionParams{
		ProjectID:               projectID,
		Kind:                    publisher.ConnectionKindGitHubNextJS,
		Label:                   label,
		Status:                  status,
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

func (s *Server) upsertDevToPublisherConnection(w http.ResponseWriter, r *http.Request) {
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
	var in devToPublisherInput
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	var credentialRef *string
	if existing, err := s.Q.GetDefaultPublisherConnectionForProject(r.Context(), db.GetDefaultPublisherConnectionForProjectParams{
		ProjectID: projectID,
		Kind:      publisher.ConnectionKindDevTo,
	}); err == nil {
		credentialRef = existing.CredentialRef
	} else if !errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	cfgRaw, status, err := devToConfigForUpsert(in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		label = "Dev.to"
	}
	conn, err := s.Q.UpsertDefaultPublisherConnection(r.Context(), db.UpsertDefaultPublisherConnectionParams{
		ProjectID:               projectID,
		Kind:                    publisher.ConnectionKindDevTo,
		Label:                   label,
		Status:                  status,
		Capabilities:            publisher.DevToCapabilities().JSON(),
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
	switch conn.Kind {
	case publisher.ConnectionKindGitHubNextJS:
		if _, err := publisher.ParseGitHubNextJSConfig(conn.Config); err != nil {
			s.writePublisherConnectionError(w, r, http.StatusBadRequest, connectionID, projectID, err.Error())
			return
		}
	case publisher.ConnectionKindDevTo:
		// The Dev.to API key itself is checked after shared credential resolution.
	default:
		writeErr(w, http.StatusBadRequest, "publisher connection test not supported for "+conn.Kind)
		return
	}
	token, err := s.publisherConnectionToken(r.Context(), projectID, conn)
	if err != nil {
		s.writePublisherConnectionError(w, r, http.StatusFailedDependency, connectionID, projectID, "publisher credential cannot be resolved")
		return
	}
	if token == "" {
		s.writePublisherConnectionError(w, r, http.StatusFailedDependency, connectionID, projectID, "publisher credential unavailable")
		return
	}
	if conn.Kind == publisher.ConnectionKindDevTo {
		if _, err := verifyDevToAPIKey(r.Context(), http.DefaultClient, devToAPIBaseURL, token); err != nil {
			s.writePublisherConnectionError(w, r, http.StatusFailedDependency, connectionID, projectID, err.Error())
			return
		}
	}
	updated, err := s.Q.MarkPublisherConnectionVerified(r.Context(), db.MarkPublisherConnectionVerifiedParams{ID: connectionID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, publisherConnectionResponse(updated))
}

func (s *Server) writePublisherConnectionError(w http.ResponseWriter, r *http.Request, status int, connectionID uuid.UUID, projectID uuid.UUID, msg string) {
	updated, markErr := s.Q.MarkPublisherConnectionError(r.Context(), db.MarkPublisherConnectionErrorParams{
		ID:        connectionID,
		ProjectID: projectID,
		LastError: &msg,
	})
	if markErr == nil {
		writeJSON(w, status, publisherConnectionResponse(updated))
		return
	}
	writeErr(w, status, msg)
}

func (s *Server) deletePublisherConnection(w http.ResponseWriter, r *http.Request) {
	projectID, connectionID, ok := s.publisherConnectionPathIDs(w, r)
	if !ok {
		return
	}
	if s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	deleted, err := s.Q.DeletePublisherConnectionForProject(r.Context(), db.DeletePublisherConnectionForProjectParams{
		ID:        connectionID,
		ProjectID: projectID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "publisher connection not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, publisherConnectionResponse(deleted))
}

func (s *Server) setPublisherConnectionEnabled(w http.ResponseWriter, r *http.Request) {
	projectID, connectionID, ok := s.publisherConnectionPathIDs(w, r)
	if !ok {
		return
	}
	if s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := s.Q.SetPublisherConnectionEnabled(r.Context(), db.SetPublisherConnectionEnabledParams{
		ID:        connectionID,
		ProjectID: projectID,
		Enabled:   in.Enabled,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "publisher connection not found")
			return
		}
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
	kind, err := publisherCredentialKindForConnection(conn, in.Kind)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
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
	kind, err := publisherCredentialKindForConnection(conn, "")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := s.Q.RevokePublisherCredentialForConnection(r.Context(), db.RevokePublisherCredentialForConnectionParams{
		ProjectID:    projectID,
		ConnectionID: connectionID,
		Kind:         kind,
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
		Enabled:                 row.Enabled,
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
			if strings.EqualFold(strings.TrimSpace(k), "credential_ref") {
				if ref, ok := child.(string); ok && publisher.IsEnvPublisherCredentialRef(ref) {
					return errors.New("env:GITHUB_TOKEN publisher credential fallback is disabled")
				}
			}
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

func publisherCredentialKindForConnection(conn db.PublisherConnection, rawKind string) (string, error) {
	kind := strings.TrimSpace(rawKind)
	switch conn.Kind {
	case publisher.ConnectionKindGitHubNextJS:
		if kind == "" {
			kind = publisher.CredentialKindGitHubToken
		}
		if kind != publisher.CredentialKindGitHubToken {
			return "", fmt.Errorf("publisher credential kind %q not supported for %s", kind, conn.Kind)
		}
		return kind, nil
	case publisher.ConnectionKindDevTo:
		if kind == "" {
			kind = publisher.CredentialKindDevToAPIKey
		}
		if kind != publisher.CredentialKindDevToAPIKey {
			return "", fmt.Errorf("publisher credential kind %q not supported for %s", kind, conn.Kind)
		}
		return kind, nil
	default:
		return "", fmt.Errorf("publisher credential not supported for %s", conn.Kind)
	}
}

func verifyDevToAPIKey(ctx context.Context, client *http.Client, baseURL string, apiKey string) (devToProfile, error) {
	var profile devToProfile
	trimmedKey := strings.TrimSpace(apiKey)
	if trimmedKey == "" {
		return profile, errors.New("dev.to API key is required")
	}
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(strings.TrimSpace(baseURL), "/")+"/api/users/me", nil)
	if err != nil {
		return profile, err
	}
	req.Header.Set("accept", "application/vnd.forem.api-v1+json")
	req.Header.Set("api-key", trimmedKey)
	res, err := client.Do(req)
	if err != nil {
		return profile, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return profile, fmt.Errorf("dev.to API key rejected with status %d", res.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 32*1024)).Decode(&profile); err != nil {
		return profile, err
	}
	return profile, nil
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
	if publisher.IsEnvPublisherCredentialRef(ref) {
		return "", errors.New("project-scoped publisher credential is required; env:GITHUB_TOKEN fallback is disabled")
	}
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

func (s *Server) publisherConnectionToken(ctx context.Context, projectID uuid.UUID, conn db.PublisherConnection) (string, error) {
	if conn.Kind == publisher.ConnectionKindGitHubNextJS {
		cfg := parseGithubConnConfig(conn.Config)
		if installationID := strings.TrimSpace(cfg.InstallationID); installationID != "" {
			app := s.githubApp()
			if app.Configured() {
				token, err := app.InstallationToken(ctx, installationID)
				if err != nil {
					return "", err
				}
				if strings.TrimSpace(token) != "" {
					return token, nil
				}
			}
		}
	}
	return s.publisherCredentialToken(ctx, projectID, conn)
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
