package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/scheduler"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const adminDeleteTimeout = 110 * time.Second

func adminDeleteContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, adminDeleteTimeout)
}

func writeAdminDeleteError(w http.ResponseWriter, ctx context.Context, err error) {
	if adminDeleteTimedOut(ctx, err) {
		writeErr(w, http.StatusGatewayTimeout, "admin delete timed out; background jobs may still be writing project data. Refresh and retry in a moment.")
		return
	}
	writeErr(w, http.StatusInternalServerError, err.Error())
}

func adminDeleteTimedOut(ctx context.Context, err error) bool {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "canceling statement due to user request")
}

func (s *Server) deleteAdminProjectRecord(ctx context.Context, projectID uuid.UUID) (db.Project, error) {
	if s.Pool == nil {
		return s.Q.DeleteProject(ctx, projectID)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return db.Project{}, err
	}
	defer tx.Rollback(ctx)

	if err := lockAdminProjectDelete(ctx, tx, projectID); err != nil {
		return db.Project{}, err
	}
	project, err := db.New(tx).DeleteProject(ctx, projectID)
	if err != nil {
		return db.Project{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Project{}, err
	}
	return project, nil
}

func (s *Server) deleteAdminOwnerProjects(ctx context.Context, ownerID string) ([]db.Project, error) {
	if s.Pool == nil {
		return s.Q.DeleteProjectsByOwner(ctx, ownerID)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	q := db.New(tx)
	projects, err := q.ListProjectsByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	for _, project := range projects {
		if err := lockAdminProjectDelete(ctx, tx, project.ID); err != nil {
			return nil, err
		}
	}
	deleted, err := q.DeleteProjectsByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return deleted, nil
}

func lockAdminProjectDelete(ctx context.Context, tx pgx.Tx, projectID uuid.UUID) error {
	_, err := tx.Exec(ctx, "select pg_advisory_xact_lock($1)", scheduler.LockKey(projectID))
	return err
}
