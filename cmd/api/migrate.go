package main

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/citeloop/citeloop/internal/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

// runMigrations applies each migrations/*.sql once, tracked in schema_migrations.
// Simple forward-only runner — good enough for the MVP (no rollback).
func runMigrations(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	if _, err := pool.Exec(ctx, `create table if not exists schema_migrations (
		name text primary key, applied_at timestamptz not null default now())`); err != nil {
		return err
	}
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Name() != "embed.go" {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var exists bool
		if err := pool.QueryRow(ctx, `select exists(select 1 from schema_migrations where name=$1)`, name).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		sqlBytes, err := migrations.FS.ReadFile(name)
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx, `insert into schema_migrations(name) values($1)`, name); err != nil {
			return err
		}
		log.Info("migration applied", "name", name)
	}
	return nil
}
