package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/publisher"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type publishingHealthDTO struct {
	Status           string                  `json:"status"`
	Ready            bool                    `json:"ready"`
	ConnectionStatus string                  `json:"connection_status"`
	CredentialStatus string                  `json:"credential_status"`
	Reasons          []string                `json:"reasons"`
	NextAction       string                  `json:"next_action"`
	Capabilities     map[string]bool         `json:"capabilities"`
	Connection       *publisherConnectionDTO `json:"connection,omitempty"`
}

func (s *Server) getPublishingHealth(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	health, err := s.publisherHealth(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, health)
}

func (s *Server) publisherHealth(ctx context.Context, projectID uuid.UUID) (publishingHealthDTO, error) {
	health := publishingHealthDTO{
		Status:           "blocked",
		Ready:            false,
		ConnectionStatus: "missing",
		CredentialStatus: "missing",
		Reasons:          []string{"publisher_missing"},
		NextAction:       "Open Settings and connect a GitHub/Next.js publisher.",
		Capabilities:     map[string]bool{},
	}
	if s.Q == nil {
		health.Status = "error"
		health.Reasons = []string{"database_unavailable"}
		health.NextAction = "Retry after the database is available."
		return health, nil
	}

	conn, err := s.Q.GetDefaultPublisherConnectionForProject(ctx, db.GetDefaultPublisherConnectionForProjectParams{
		ProjectID: projectID,
		Kind:      publisher.ConnectionKindGitHubNextJS,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return health, nil
		}
		return health, err
	}

	dto := publisherConnectionResponse(conn)
	health.Connection = &dto
	health.Capabilities = publisher.GitHubNextJSCapabilities()
	health.ConnectionStatus = "in_progress"
	health.Reasons = []string{}
	health.NextAction = "Save a GitHub token, then run Test."
	if conn.Status == "connected" {
		health.ConnectionStatus = "connected"
		health.NextAction = "No action needed."
	} else if conn.Status == "error" {
		health.ConnectionStatus = "error"
		health.Status = "error"
		health.Reasons = append(health.Reasons, "publisher_connection_error")
		health.NextAction = "Fix the publisher settings, save, then run Test."
	}

	if _, err := publisher.ParseGitHubNextJSConfig(conn.Config); err != nil {
		health.Status = "error"
		health.ConnectionStatus = "error"
		health.Reasons = appendReason(health.Reasons, "publisher_config_invalid")
		health.NextAction = "Add repository and base URL in Settings, then save publisher."
	}

	token, err := s.publisherCredentialToken(ctx, projectID, conn)
	if err != nil {
		health.Status = "error"
		health.CredentialStatus = "error"
		health.Reasons = appendReason(health.Reasons, "publisher_credential_unavailable")
		if health.NextAction == "" || health.NextAction == "No action needed." {
			health.NextAction = "Re-save the GitHub token in Settings."
		}
	} else if strings.TrimSpace(token) == "" {
		health.CredentialStatus = "missing"
		health.Reasons = appendReason(health.Reasons, "publisher_credential_missing")
		if health.Status != "error" {
			health.Status = "blocked"
		}
		health.NextAction = "Save a GitHub token in Settings, then run Test."
	} else {
		health.CredentialStatus = "configured"
	}

	if len(health.Reasons) == 0 && health.ConnectionStatus == "connected" && health.CredentialStatus == "configured" {
		health.Status = "ready"
		health.Ready = true
		health.NextAction = "Publisher is ready for automatic canonical publishing."
	} else if health.Status == "" {
		health.Status = "blocked"
	}
	return health, nil
}

func appendReason(reasons []string, reason string) []string {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}
