package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/githubapp"
	"github.com/citeloop/citeloop/internal/publisher"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Server) githubApp() *githubapp.Service {
	return githubapp.New(githubapp.Config{
		AppID:         s.Env.GitHubAppID,
		Slug:          s.Env.GitHubAppSlug,
		ClientID:      s.Env.GitHubAppClientID,
		ClientSecret:  s.Env.GitHubAppClientSecret,
		PrivateKeyPEM: s.Env.GitHubAppPrivateKey,
	})
}

// githubConnConfig is the subset of the github_nextjs connection config the App
// flow reads/writes. installation_id is not secret material (the token is minted
// on demand from the App key), so it lives in config, not a credential.
type githubConnConfig struct {
	Repo           string `json:"repo,omitempty"`
	Branch         string `json:"branch,omitempty"`
	ContentDir     string `json:"content_dir,omitempty"`
	BaseURL        string `json:"base_url,omitempty"`
	PublishMode    string `json:"publish_mode,omitempty"`
	InstallationID string `json:"installation_id,omitempty"`
}

func parseGithubConnConfig(raw json.RawMessage) githubConnConfig {
	var c githubConnConfig
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &c)
	}
	return c
}

func (s *Server) githubConnection(r *http.Request, projectID uuid.UUID) (db.PublisherConnection, bool) {
	conn, err := s.Q.GetDefaultPublisherConnectionForProject(r.Context(), db.GetDefaultPublisherConnectionForProjectParams{
		ProjectID: projectID,
		Kind:      publisher.ConnectionKindGitHubNextJS,
	})
	if err != nil {
		return db.PublisherConnection{}, false
	}
	return conn, true
}

func (s *Server) saveGithubConnection(r *http.Request, projectID uuid.UUID, cfg githubConnConfig, status string) (db.PublisherConnection, error) {
	return s.Q.UpsertDefaultPublisherConnection(r.Context(), db.UpsertDefaultPublisherConnectionParams{
		ProjectID:               projectID,
		Kind:                    publisher.ConnectionKindGitHubNextJS,
		Label:                   "GitHub/Next.js",
		Status:                  status,
		Capabilities:            publisher.GitHubNextJSCapabilities().JSON(),
		CapabilitySchemaVersion: 1,
		CredentialRef:           nil,
		Config:                  mustPublisherJSON(cfg),
		LastVerifiedAt:          pgtype.Timestamptz{},
		LastError:               nil,
	})
}

type githubIntegrationStatus struct {
	Configured     bool   `json:"configured"`      // App env set on the server
	Connected      bool   `json:"connected"`       // an installation is stored
	InstallationID string `json:"installation_id,omitempty"`
	Repo           string `json:"repo,omitempty"`
	Branch         string `json:"branch,omitempty"`
	ContentDir     string `json:"content_dir,omitempty"`
	BaseURL        string `json:"base_url,omitempty"`
	InstallURL     string `json:"install_url,omitempty"`
	// ReusableInstallationID is an installation the SAME owner already linked on
	// another project. A GitHub App installs once per account, so the UI can
	// offer to reuse it instead of re-running the install (which dead-ends on
	// GitHub's "already installed" page).
	ReusableInstallationID string `json:"reusable_installation_id,omitempty"`
}

func (s *Server) getGithubIntegration(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	app := s.githubApp()
	out := githubIntegrationStatus{Configured: app.Configured()}
	if app.Configured() {
		out.InstallURL = app.InstallURL(projectID.String())
	}
	if conn, ok := s.githubConnection(r, projectID); ok {
		cfg := parseGithubConnConfig(conn.Config)
		out.InstallationID = cfg.InstallationID
		out.Connected = strings.TrimSpace(cfg.InstallationID) != ""
		out.Repo, out.Branch, out.ContentDir, out.BaseURL = cfg.Repo, cfg.Branch, cfg.ContentDir, cfg.BaseURL
	}
	// Not yet connected but the App is set up: surface an installation the same
	// owner already linked elsewhere so the UI can reuse it (one install per
	// GitHub account) rather than re-running the dead-ending install flow.
	if app.Configured() && !out.Connected {
		if reuse, err := s.Q.FindReusableGitHubInstallation(r.Context(), projectID); err == nil && strings.TrimSpace(reuse) != "" {
			out.ReusableInstallationID = strings.TrimSpace(reuse)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// storeGithubInstallation records the installation_id GitHub redirected back
// with, so we can mint tokens for it. The route is project-scoped + owner-gated.
func (s *Server) storeGithubInstallation(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	if !s.githubApp().Configured() {
		writeErr(w, http.StatusFailedDependency, "GitHub App is not configured on the server")
		return
	}
	var in struct {
		InstallationID string `json:"installation_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if strings.TrimSpace(in.InstallationID) == "" {
		writeErr(w, http.StatusBadRequest, "installation_id required")
		return
	}
	cfg := githubConnConfig{}
	if conn, ok := s.githubConnection(r, projectID); ok {
		cfg = parseGithubConnConfig(conn.Config)
	}
	cfg.InstallationID = strings.TrimSpace(in.InstallationID)
	if _, err := s.saveGithubConnection(r, projectID, cfg, "missing"); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.listGithubRepos(w, r)
}

func (s *Server) listGithubRepos(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	conn, ok := s.githubConnection(r, projectID)
	if !ok {
		writeErr(w, http.StatusNotFound, "no GitHub connection")
		return
	}
	cfg := parseGithubConnConfig(conn.Config)
	if strings.TrimSpace(cfg.InstallationID) == "" {
		writeErr(w, http.StatusFailedDependency, "GitHub is not connected yet")
		return
	}
	repos, err := s.githubApp().ListRepos(r.Context(), cfg.InstallationID)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"repositories": repos})
}

// selectGithubRepo saves the repo the operator picked + publish target, and
// marks the connection connected (it can publish via an installation token).
func (s *Server) selectGithubRepo(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	conn, ok := s.githubConnection(r, projectID)
	if !ok {
		writeErr(w, http.StatusNotFound, "no GitHub connection")
		return
	}
	cfg := parseGithubConnConfig(conn.Config)
	if strings.TrimSpace(cfg.InstallationID) == "" {
		writeErr(w, http.StatusFailedDependency, "GitHub is not connected yet")
		return
	}
	var in struct {
		Repo       string `json:"repo"`
		Branch     string `json:"branch"`
		ContentDir string `json:"content_dir"`
		BaseURL    string `json:"base_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg.Repo = strings.TrimSpace(in.Repo)
	cfg.Branch = strings.TrimSpace(in.Branch)
	cfg.ContentDir = strings.TrimSpace(in.ContentDir)
	cfg.BaseURL = strings.TrimSpace(in.BaseURL)
	if cfg.Repo == "" || cfg.BaseURL == "" {
		writeErr(w, http.StatusBadRequest, "repo and base_url are required")
		return
	}
	updated, err := s.saveGithubConnection(r, projectID, cfg, "connected")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, publisherConnectionResponse(updated))
}
