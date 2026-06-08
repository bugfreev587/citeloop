package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/citeloop/citeloop/internal/buildinfo"
	"github.com/jackc/pgx/v5"
)

type versionResponse struct {
	Status      string                  `json:"status"`
	Build       buildinfo.Info          `json:"build"`
	Database    databaseVersionSnapshot `json:"database"`
	GeneratedAt time.Time               `json:"generated_at"`
}

type databaseVersionSnapshot struct {
	Connected         bool   `json:"connected"`
	MigrationStatus   string `json:"migration_status"`
	AppliedMigrations int    `json:"applied_migrations,omitempty"`
	LatestMigration   string `json:"latest_migration,omitempty"`
	Error             string `json:"error,omitempty"`
}

func (s *Server) getHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.versionSnapshot(r.Context()))
}

func (s *Server) getVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.versionSnapshot(r.Context()))
}

func (s *Server) versionSnapshot(ctx context.Context) versionResponse {
	return versionResponse{
		Status:      "ok",
		Build:       buildinfo.Current("citeloop-api"),
		Database:    s.databaseVersionSnapshot(ctx),
		GeneratedAt: time.Now().UTC(),
	}
}

func (s *Server) databaseVersionSnapshot(ctx context.Context) databaseVersionSnapshot {
	if s.Pool == nil {
		return databaseVersionSnapshot{MigrationStatus: "unavailable"}
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var latest string
	var count int
	err := s.Pool.QueryRow(ctx, `
		select coalesce(max(name), ''), count(*)::int
		from schema_migrations
	`).Scan(&latest, &count)
	if err != nil {
		status := "error"
		if errors.Is(err, pgx.ErrNoRows) {
			status = "not_initialized"
		}
		return databaseVersionSnapshot{
			Connected:       true,
			MigrationStatus: status,
			Error:           err.Error(),
		}
	}

	return databaseVersionSnapshot{
		Connected:         true,
		MigrationStatus:   "ok",
		AppliedMigrations: count,
		LatestMigration:   latest,
	}
}
