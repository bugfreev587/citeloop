package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	migrationAdvisoryLockKey         int64 = 0x436974654c6f6f70
	nonTransactionalMigrationTimeout       = 2 * time.Minute
	migrationUnlockTimeout                 = 5 * time.Second
	nonTransactionalModeDirective          = "-- citeloop:migration-mode=nontransactional"
)

type migrationMode string

const (
	migrationModeTransactional    migrationMode = "transactional"
	migrationModeNontransactional migrationMode = "nontransactional"
)

type migrationSpec struct {
	Name      string
	SQL       string
	Mode      migrationMode
	IndexName string
}

type migrationTx interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Commit(context.Context) error
	Rollback(context.Context) error
}

type migrationConnection interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	BeginMigration(context.Context) (migrationTx, error)
}

type pgxMigrationConnection struct {
	conn *pgxpool.Conn
}

func (c *pgxMigrationConnection) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return c.conn.Exec(ctx, sql, args...)
}

func (c *pgxMigrationConnection) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return c.conn.QueryRow(ctx, sql, args...)
}

func (c *pgxMigrationConnection) BeginMigration(ctx context.Context) (migrationTx, error) {
	return c.conn.Begin(ctx)
}

// runMigrations applies embedded migrations while holding one dedicated-session
// advisory lock for the entire pass.
func runMigrations(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()

	return runMigrationPass(ctx, &pgxMigrationConnection{conn: conn}, migrations.FS, log)
}

func runMigrationPass(ctx context.Context, conn migrationConnection, migrationFS fs.FS, log *slog.Logger) (retErr error) {
	if _, err := conn.Exec(ctx, `select pg_advisory_lock($1)`, migrationAdvisoryLockKey); err != nil {
		return fmt.Errorf("acquire migration advisory lock: %w", err)
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), migrationUnlockTimeout)
		defer cancel()
		if _, err := conn.Exec(unlockCtx, `select pg_advisory_unlock($1)`, migrationAdvisoryLockKey); err != nil && retErr == nil {
			retErr = fmt.Errorf("release migration advisory lock: %w", err)
		}
	}()

	if _, err := conn.Exec(ctx, `create table if not exists schema_migrations (
		name text primary key, applied_at timestamptz not null default now())`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationFS, ".")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		raw, err := fs.ReadFile(migrationFS, name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		spec, err := parseMigrationSpec(name, raw)
		if err != nil {
			return err
		}

		switch spec.Mode {
		case migrationModeTransactional:
			err = applyTransactionalMigration(ctx, conn, spec)
		case migrationModeNontransactional:
			err = applyNontransactionalMigration(ctx, conn, spec)
		default:
			err = fmt.Errorf("migration %s has unsupported execution mode %q", name, spec.Mode)
		}
		if err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
		log.Info("migration applied", "name", name)
	}
	return nil
}

func applyTransactionalMigration(ctx context.Context, conn migrationConnection, spec migrationSpec) error {
	tx, err := conn.BeginMigration(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	var applied bool
	if err := tx.QueryRow(ctx, `select exists(select 1 from schema_migrations where name=$1)`, spec.Name).Scan(&applied); err != nil {
		return fmt.Errorf("recheck migration marker: %w", err)
	}
	if applied {
		return nil
	}
	if _, err := tx.Exec(ctx, spec.SQL); err != nil {
		return fmt.Errorf("execute transactional migration: %w", err)
	}
	if _, err := tx.Exec(ctx, `insert into schema_migrations(name) values($1)`, spec.Name); err != nil {
		return fmt.Errorf("insert migration marker: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}
	return nil
}

func applyNontransactionalMigration(ctx context.Context, conn migrationConnection, spec migrationSpec) error {
	migrationCtx, cancel := context.WithTimeout(ctx, nonTransactionalMigrationTimeout)
	defer cancel()

	var applied bool
	if err := conn.QueryRow(migrationCtx, `select exists(select 1 from schema_migrations where name=$1)`, spec.Name).Scan(&applied); err != nil {
		return fmt.Errorf("recheck nontransactional migration marker: %w", err)
	}
	if applied {
		return nil
	}

	var valid bool
	err := conn.QueryRow(migrationCtx, `
		select i.indisvalid
		from pg_class c
		join pg_index i on i.indexrelid = c.oid
		join pg_namespace n on n.oid = c.relnamespace
		where n.nspname = current_schema() and c.relname = $1`, spec.IndexName).Scan(&valid)
	switch {
	case err == nil && valid:
		// A previous attempt may have committed the index and crashed before its marker.
	case err == nil:
		identifier := pgx.Identifier{spec.IndexName}.Sanitize()
		if _, err := conn.Exec(migrationCtx, "drop index concurrently "+identifier); err != nil {
			return fmt.Errorf("drop invalid concurrent index: %w", err)
		}
		if _, err := conn.Exec(migrationCtx, spec.SQL); err != nil {
			return fmt.Errorf("execute concurrent index migration: %w", err)
		}
	case errors.Is(err, pgx.ErrNoRows):
		if _, err := conn.Exec(migrationCtx, spec.SQL); err != nil {
			return fmt.Errorf("execute concurrent index migration: %w", err)
		}
	default:
		return fmt.Errorf("inspect concurrent index: %w", err)
	}

	if _, err := conn.Exec(migrationCtx, `insert into schema_migrations(name) values($1)`, spec.Name); err != nil {
		return fmt.Errorf("insert nontransactional migration marker: %w", err)
	}
	return nil
}

var migrationIndexNamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)
var concurrentIndexStatementPattern = regexp.MustCompile(`(?is)^create\s+(?:unique\s+)?index\s+concurrently\s+if\s+not\s+exists\s+([a-z_][a-z0-9_]*)\s+on\s+.+;$`)

func parseMigrationSpec(name string, raw []byte) (migrationSpec, error) {
	spec := migrationSpec{Name: name, SQL: string(raw), Mode: migrationModeTransactional}
	var modeSeen, indexSeen, executableSeen bool

	for _, line := range strings.Split(spec.SQL, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "-- citeloop:") {
			if executableSeen {
				return migrationSpec{}, fmt.Errorf("invalid migration directive in %s: directives must precede SQL", name)
			}
			switch {
			case strings.HasPrefix(trimmed, "-- citeloop:migration-mode="):
				if modeSeen || trimmed != nonTransactionalModeDirective {
					return migrationSpec{}, fmt.Errorf("invalid migration directive in %s: %s", name, trimmed)
				}
				modeSeen = true
				spec.Mode = migrationModeNontransactional
			case strings.HasPrefix(trimmed, "-- citeloop:index="):
				if indexSeen {
					return migrationSpec{}, fmt.Errorf("invalid migration directive in %s: duplicate index", name)
				}
				indexSeen = true
				spec.IndexName = strings.TrimPrefix(trimmed, "-- citeloop:index=")
			default:
				return migrationSpec{}, fmt.Errorf("invalid migration directive in %s: %s", name, trimmed)
			}
			continue
		}
		if trimmed != "" && !strings.HasPrefix(trimmed, "--") {
			executableSeen = true
		}
	}

	if spec.Mode == migrationModeTransactional {
		if indexSeen {
			return migrationSpec{}, fmt.Errorf("invalid migration directive in %s: index requires nontransactional mode", name)
		}
		return spec, nil
	}
	if !indexSeen || !migrationIndexNamePattern.MatchString(spec.IndexName) {
		return migrationSpec{}, fmt.Errorf("invalid concurrent index name in %s", name)
	}

	executable, err := migrationExecutableSQL(spec.SQL)
	if err != nil {
		return migrationSpec{}, fmt.Errorf("%s: %w", name, err)
	}
	match := concurrentIndexStatementPattern.FindStringSubmatch(executable)
	if len(match) != 2 || match[1] != spec.IndexName {
		return migrationSpec{}, fmt.Errorf("%s must contain exactly one CREATE INDEX CONCURRENTLY statement matching %q", name, spec.IndexName)
	}
	return spec, nil
}

func migrationExecutableSQL(sql string) (string, error) {
	if strings.Contains(sql, "/*") || strings.Contains(sql, "*/") {
		return "", errors.New("nontransactional migrations do not allow block comments")
	}
	lines := make([]string, 0)
	for _, line := range strings.Split(sql, "\n") {
		if comment := strings.Index(line, "--"); comment >= 0 {
			line = line[:comment]
		}
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	executable := strings.TrimSpace(strings.Join(lines, "\n"))
	if strings.Count(executable, ";") != 1 || !strings.HasSuffix(executable, ";") {
		return "", errors.New("nontransactional migration must contain exactly one CREATE INDEX CONCURRENTLY statement")
	}
	return executable, nil
}
