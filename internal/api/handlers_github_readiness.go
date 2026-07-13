package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/publisher"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type githubPRReadinessStore interface {
	GetGitHubPRReadinessForProject(context.Context, uuid.UUID) (db.PublisherConnection, error)
	SetGitHubPRReadinessIfUnchanged(context.Context, db.SetGitHubPRReadinessIfUnchangedParams) (db.PublisherConnection, error)
}

type githubPRReadinessChecker interface {
	Check(context.Context, uuid.UUID) (publisher.GitHubPRReadiness, githubPRReadinessTarget, error)
}

// githubPRReadinessTarget is the exact connection/target/authority snapshot a
// live check proved. The credential fields are intentionally private and this
// value is never used as an HTTP response.
type githubPRReadinessTarget struct {
	ConnectionID      uuid.UUID
	ExpectedUpdatedAt pgtype.Timestamptz
	Repo              string
	Branch            string

	credentialKind publisher.GitHubPRCredentialKind
	token          string
	permissions    map[string]string
}

type serverGitHubPRReadinessChecker struct {
	server *Server
}

var (
	errGitHubPRNotReady         = errors.New("GitHub is not ready to create repair pull requests")
	errGitHubPRReadinessChanged = errors.New("GitHub connection changed during the final readiness check")
)

// authorizeGitHubPRMutation is the write-path readiness boundary. It first
// honors the lightweight persisted gate, then performs one final live check
// and strictly compare-and-sets that result. The returned target is the exact
// credential/repository/branch snapshot proved by the check, retaining the
// connection/config version so later mutations can fence concurrent changes.
func (s *Server) authorizeGitHubPRMutation(ctx context.Context, projectID uuid.UUID) (githubPRReadinessTarget, error) {
	store := s.gitHubPRReadinessStore()
	if store == nil {
		return githubPRReadinessTarget{}, errors.New("database unavailable")
	}
	stored, err := store.GetGitHubPRReadinessForProject(ctx, projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return githubPRReadinessTarget{}, errGitHubPRNotReady
	}
	if err != nil {
		return githubPRReadinessTarget{}, err
	}
	if publisher.GitHubPRReadinessStatus(stored.PrReadinessStatus) != publisher.GitHubPRReadinessReady {
		return githubPRReadinessTarget{}, errGitHubPRNotReady
	}

	checkedAt := s.gitHubPRReadinessTime().UTC()
	readiness, target, err := s.gitHubPRReadinessChecker().Check(ctx, projectID)
	if err != nil {
		return githubPRReadinessTarget{}, err
	}
	readiness = controlledGitHubPRReadiness(readiness)
	if target.ConnectionID == uuid.Nil || target.ConnectionID != stored.ID || target.ExpectedUpdatedAt != stored.UpdatedAt {
		return githubPRReadinessTarget{}, errGitHubPRReadinessChanged
	}
	var detail *string
	if readiness.Detail != "" {
		detailValue := readiness.Detail
		detail = &detailValue
	}
	updated, err := store.SetGitHubPRReadinessIfUnchanged(ctx, db.SetGitHubPRReadinessIfUnchangedParams{
		PrReadinessStatus:    string(readiness.Status),
		PrReadinessCheckedAt: pgtype.Timestamptz{Time: checkedAt, Valid: true},
		PrReadinessDetail:    detail,
		ConnectionID:         target.ConnectionID,
		ProjectID:            projectID,
		ExpectedUpdatedAt:    target.ExpectedUpdatedAt,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return githubPRReadinessTarget{}, errGitHubPRReadinessChanged
	}
	if err != nil {
		return githubPRReadinessTarget{}, err
	}
	if updated.ID != target.ConnectionID || updated.ProjectID != projectID || !updated.UpdatedAt.Valid {
		return githubPRReadinessTarget{}, errGitHubPRReadinessChanged
	}
	target.ExpectedUpdatedAt = updated.UpdatedAt
	if readiness.Status != publisher.GitHubPRReadinessReady || updated.PrReadinessStatus != string(publisher.GitHubPRReadinessReady) {
		return githubPRReadinessTarget{}, errGitHubPRNotReady
	}
	if strings.TrimSpace(target.Repo) == "" || strings.TrimSpace(target.Branch) == "" || strings.TrimSpace(target.token) == "" {
		return githubPRReadinessTarget{}, errGitHubPRReadinessChanged
	}
	return target, nil
}

func (s *Server) downgradeGitHubPRReadinessAfterMutationFailure(ctx context.Context, projectID uuid.UUID, target githubPRReadinessTarget, cause error) error {
	var statusError interface{ StatusCode() int }
	if !errors.As(cause, &statusError) {
		return nil
	}
	var status publisher.GitHubPRReadinessStatus
	switch statusError.StatusCode() {
	case http.StatusUnauthorized, http.StatusForbidden:
		status = publisher.GitHubPRReadinessPermissionMissing
	case http.StatusNotFound:
		status = publisher.GitHubPRReadinessRepositoryUnavailable
	default:
		return nil
	}
	store := s.gitHubPRReadinessStore()
	if store == nil {
		return errors.New("database unavailable")
	}
	detailValue := controlledGitHubPRReadinessDetail(status, "")
	_, err := store.SetGitHubPRReadinessIfUnchanged(ctx, db.SetGitHubPRReadinessIfUnchangedParams{
		PrReadinessStatus:    string(status),
		PrReadinessCheckedAt: pgtype.Timestamptz{Time: s.gitHubPRReadinessTime().UTC(), Valid: true},
		PrReadinessDetail:    &detailValue,
		ConnectionID:         target.ConnectionID,
		ProjectID:            projectID,
		ExpectedUpdatedAt:    target.ExpectedUpdatedAt,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// A newer readiness observation or connection/config mutation already
		// invalidated this result.
		return nil
	}
	return err
}

func (s *Server) ensureGitHubPRTargetCurrent(ctx context.Context, projectID uuid.UUID, target githubPRReadinessTarget) error {
	store := s.gitHubPRReadinessStore()
	if store == nil {
		return errors.New("database unavailable")
	}
	current, err := store.GetGitHubPRReadinessForProject(ctx, projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return errGitHubPRReadinessChanged
	}
	if err != nil {
		return err
	}
	if current.ID != target.ConnectionID || current.ProjectID != projectID || current.UpdatedAt != target.ExpectedUpdatedAt ||
		current.Kind != publisher.ConnectionKindGitHubNextJS || current.Status != "connected" || !current.IsDefault || !current.Enabled ||
		current.RevokedAt.Valid || current.PrReadinessStatus != string(publisher.GitHubPRReadinessReady) {
		return errGitHubPRReadinessChanged
	}
	config, err := publisher.ParseGitHubNextJSConfig(current.Config)
	if err != nil || config.Repo != strings.TrimSpace(target.Repo) || config.Branch != strings.TrimSpace(target.Branch) {
		return errGitHubPRReadinessChanged
	}
	return nil
}

func (s *Server) getGitHubPRReadiness(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	store := s.gitHubPRReadinessStore()
	if store == nil {
		writeErr(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	connection, err := store.GetGitHubPRReadinessForProject(r.Context(), projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusOK, githubPRReadinessWithoutConnection())
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "GitHub readiness could not be loaded")
		return
	}
	writeJSON(w, http.StatusOK, githubPRReadinessFromConnection(connection))
}

func (s *Server) checkGitHubPRReadiness(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	checker := s.gitHubPRReadinessChecker()
	checkedAt := s.gitHubPRReadinessTime().UTC()
	readiness, target, err := checker.Check(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "GitHub readiness could not be checked")
		return
	}
	readiness = controlledGitHubPRReadiness(readiness)
	if target.ConnectionID == uuid.Nil {
		readiness.CheckedAt = nil
		writeJSON(w, http.StatusOK, readiness)
		return
	}
	readiness.CheckedAt = &checkedAt
	readiness.Repo = strings.TrimSpace(target.Repo)
	readiness.Branch = strings.TrimSpace(target.Branch)
	store := s.gitHubPRReadinessStore()
	if store == nil {
		writeErr(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	var detail *string
	if readiness.Detail != "" {
		detailValue := readiness.Detail
		detail = &detailValue
	}
	updated, err := store.SetGitHubPRReadinessIfUnchanged(r.Context(), db.SetGitHubPRReadinessIfUnchangedParams{
		PrReadinessStatus:    string(readiness.Status),
		PrReadinessCheckedAt: pgtype.Timestamptz{Time: checkedAt, Valid: true},
		PrReadinessDetail:    detail,
		ConnectionID:         target.ConnectionID,
		ProjectID:            projectID,
		ExpectedUpdatedAt:    target.ExpectedUpdatedAt,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		current, reloadErr := store.GetGitHubPRReadinessForProject(r.Context(), projectID)
		if errors.Is(reloadErr, pgx.ErrNoRows) {
			writeJSON(w, http.StatusOK, githubPRReadinessWithoutConnection())
			return
		}
		if reloadErr != nil {
			writeErr(w, http.StatusInternalServerError, "GitHub readiness could not be loaded")
			return
		}
		writeJSON(w, http.StatusOK, githubPRReadinessFromConnection(current))
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "GitHub readiness could not be saved")
		return
	}
	writeJSON(w, http.StatusOK, githubPRReadinessFromConnection(updated))
}

func (c serverGitHubPRReadinessChecker) Check(ctx context.Context, projectID uuid.UUID) (publisher.GitHubPRReadiness, githubPRReadinessTarget, error) {
	store := c.server.gitHubPRReadinessStore()
	if store == nil {
		return publisher.GitHubPRReadiness{}, githubPRReadinessTarget{}, errors.New("database unavailable")
	}
	connection, err := store.GetGitHubPRReadinessForProject(ctx, projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return githubPRReadinessWithoutConnection(), githubPRReadinessTarget{}, nil
	}
	if err != nil {
		return publisher.GitHubPRReadiness{}, githubPRReadinessTarget{}, err
	}
	rawConfig := parseGithubConnConfig(connection.Config)
	target := githubPRReadinessTarget{
		ConnectionID:      connection.ID,
		ExpectedUpdatedAt: connection.UpdatedAt,
		Repo:              strings.TrimSpace(rawConfig.Repo),
		Branch:            strings.TrimSpace(rawConfig.Branch),
	}
	config, configErr := publisher.ParseGitHubNextJSConfig(connection.Config)
	if configErr == nil {
		target.Repo = config.Repo
		target.Branch = config.Branch
	}
	if connection.Kind != publisher.ConnectionKindGitHubNextJS || !connection.IsDefault || connection.Status != "connected" || !connection.Enabled || connection.RevokedAt.Valid {
		return controlledGitHubPRReadiness(publisher.GitHubPRReadiness{
			Status: publisher.GitHubPRReadinessNotConnected,
			Repo:   target.Repo,
			Branch: target.Branch,
		}), target, nil
	}
	if configErr != nil {
		return controlledGitHubPRReadiness(publisher.GitHubPRReadiness{
			Status: publisher.GitHubPRReadinessRepositoryUnavailable,
			Repo:   target.Repo,
			Branch: target.Branch,
		}), target, nil
	}
	probeInput := publisher.GitHubPRReadinessProbeInput{Repo: target.Repo, Branch: target.Branch}
	if installationID := strings.TrimSpace(rawConfig.InstallationID); installationID != "" {
		app := c.server.githubApp()
		if !app.Configured() {
			return controlledGitHubPRReadiness(publisher.GitHubPRReadiness{
				Status: publisher.GitHubPRReadinessNotConnected,
				Repo:   target.Repo,
				Branch: target.Branch,
			}), target, nil
		}
		access, accessErr := app.InstallationAccess(ctx, installationID)
		if accessErr != nil {
			return controlledGitHubPRReadiness(publisher.GitHubPRReadiness{
				Status: githubPRReadinessStatusFromError(accessErr),
				Repo:   target.Repo,
				Branch: target.Branch,
			}), target, nil
		}
		target.credentialKind = publisher.GitHubPRCredentialGitHubApp
		target.token = access.Token
		target.permissions = cloneGitHubPermissions(access.Permissions)
		probeInput.CredentialKind = target.credentialKind
		probeInput.Token = target.token
		probeInput.Permissions = target.permissions
	} else {
		token, tokenErr := c.server.publisherConnectionToken(ctx, projectID, connection)
		if errors.Is(tokenErr, pgx.ErrNoRows) || (tokenErr == nil && strings.TrimSpace(token) == "") {
			return controlledGitHubPRReadiness(publisher.GitHubPRReadiness{
				Status: publisher.GitHubPRReadinessNotConnected,
				Repo:   target.Repo,
				Branch: target.Branch,
			}), target, nil
		}
		if tokenErr != nil {
			return publisher.GitHubPRReadiness{}, target, tokenErr
		}
		target.credentialKind = publisher.GitHubPRCredentialAdvancedToken
		target.token = token
		probeInput.CredentialKind = target.credentialKind
		probeInput.Token = target.token
	}
	readiness := publisher.ProbeGitHubPRReadiness(
		ctx,
		c.server.githubReadinessHTTPClient,
		c.server.githubReadinessAPIBase,
		probeInput,
	)
	return controlledGitHubPRReadiness(readiness), target, nil
}

func (s *Server) gitHubPRReadinessStore() githubPRReadinessStore {
	if s.githubReadinessStore != nil {
		return s.githubReadinessStore
	}
	if s.Q == nil {
		return nil
	}
	return s.Q
}

func (s *Server) gitHubPRReadinessChecker() githubPRReadinessChecker {
	if s.githubReadinessChecker != nil {
		return s.githubReadinessChecker
	}
	return serverGitHubPRReadinessChecker{server: s}
}

func (s *Server) gitHubPRReadinessTime() time.Time {
	if s.githubReadinessNow != nil {
		return s.githubReadinessNow()
	}
	return time.Now()
}

func githubPRReadinessFromConnection(connection db.PublisherConnection) publisher.GitHubPRReadiness {
	rawConfig := parseGithubConnConfig(connection.Config)
	repo := strings.TrimSpace(rawConfig.Repo)
	branch := strings.TrimSpace(rawConfig.Branch)
	if config, err := publisher.ParseGitHubNextJSConfig(connection.Config); err == nil {
		repo = config.Repo
		branch = config.Branch
	}
	readiness := controlledGitHubPRReadiness(publisher.GitHubPRReadiness{
		Status: publisher.GitHubPRReadinessStatus(connection.PrReadinessStatus),
		Repo:   repo,
		Branch: branch,
	})
	if connection.PrReadinessCheckedAt.Valid {
		checkedAt := connection.PrReadinessCheckedAt.Time.UTC()
		readiness.CheckedAt = &checkedAt
	}
	if connection.PrReadinessDetail != nil {
		readiness.Detail = controlledGitHubPRReadinessDetail(readiness.Status, *connection.PrReadinessDetail)
	}
	return readiness
}

func githubPRReadinessWithoutConnection() publisher.GitHubPRReadiness {
	return controlledGitHubPRReadiness(publisher.GitHubPRReadiness{Status: publisher.GitHubPRReadinessNotConnected})
}

func controlledGitHubPRReadiness(readiness publisher.GitHubPRReadiness) publisher.GitHubPRReadiness {
	switch readiness.Status {
	case publisher.GitHubPRReadinessNotConnected,
		publisher.GitHubPRReadinessNotChecked,
		publisher.GitHubPRReadinessReady,
		publisher.GitHubPRReadinessPermissionMissing,
		publisher.GitHubPRReadinessRepositoryUnavailable,
		publisher.GitHubPRReadinessError:
	default:
		readiness.Status = publisher.GitHubPRReadinessError
	}
	readiness.Detail = controlledGitHubPRReadinessDetail(readiness.Status, readiness.Detail)
	readiness.Repo = strings.TrimSpace(readiness.Repo)
	readiness.Branch = strings.TrimSpace(readiness.Branch)
	return readiness
}

func controlledGitHubPRReadinessDetail(status publisher.GitHubPRReadinessStatus, candidate string) string {
	const (
		ready           = "GitHub can create pull requests for the selected repository and branch."
		appPermission   = "The GitHub App needs contents: write and pull requests: write permissions."
		tokenPermission = "Repository write authority could not be proven for this token. Connect the GitHub App instead."
		repository      = "The selected GitHub repository or base branch is unavailable."
		checkError      = "GitHub readiness could not be checked. Try again."
		notConnected    = "Connect and enable GitHub with a project-scoped credential to create repair pull requests."
		notChecked      = "Run a GitHub readiness check for the selected repository and branch."
	)
	candidate = strings.TrimSpace(candidate)
	for _, controlled := range []string{ready, appPermission, tokenPermission, repository, checkError, notConnected, notChecked} {
		if candidate == controlled {
			return controlled
		}
	}
	switch status {
	case publisher.GitHubPRReadinessNotConnected:
		return notConnected
	case publisher.GitHubPRReadinessNotChecked:
		return notChecked
	case publisher.GitHubPRReadinessReady:
		return ready
	case publisher.GitHubPRReadinessPermissionMissing:
		return appPermission
	case publisher.GitHubPRReadinessRepositoryUnavailable:
		return repository
	default:
		return checkError
	}
}

func cloneGitHubPermissions(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func githubPRReadinessStatusFromError(err error) publisher.GitHubPRReadinessStatus {
	var statusError interface{ StatusCode() int }
	if errors.As(err, &statusError) {
		switch statusError.StatusCode() {
		case http.StatusUnauthorized, http.StatusForbidden:
			return publisher.GitHubPRReadinessPermissionMissing
		}
	}
	return publisher.GitHubPRReadinessError
}
