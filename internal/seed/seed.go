// Package seed inserts the placeholder project (single-tenant run,
// multi-tenant ready — PRD §0). Real UniPost landing URL / repo are plugged in
// later via env + the insight run.
package seed

import (
	"context"
	"errors"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/jackc/pgx/v5"
)

const placeholderSlug = "unipost"

// EnsurePlaceholder creates the default project if it does not yet exist and
// returns it. Idempotent.
func EnsurePlaceholder(ctx context.Context, q *db.Queries) (db.Project, error) {
	p, err := q.GetProjectBySlug(ctx, placeholderSlug)
	if err == nil {
		return p, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.Project{}, err
	}
	return q.CreateProject(ctx, db.CreateProjectParams{
		OwnerID: "default",
		Name:    "UniPost (placeholder)",
		Slug:    placeholderSlug,
		Config:  config.Default().JSON(),
	})
}
