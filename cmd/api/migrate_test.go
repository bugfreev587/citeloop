package main

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/citeloop/citeloop/internal/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestMigrationRunnerSafetyContract(t *testing.T) {
	raw, err := os.ReadFile("migrate.go")
	if err != nil {
		t.Fatal(err)
	}
	source := strings.ToLower(string(raw))
	requireSource := func(t *testing.T, required ...string) {
		t.Helper()
		for _, want := range required {
			if !strings.Contains(source, want) {
				t.Errorf("migration runner missing %q", want)
			}
		}
	}

	t.Run("dedicated connection and session lock", func(t *testing.T) {
		requireSource(t,
			"pool.acquire(ctx)",
			"pg_advisory_lock",
			"pg_advisory_unlock",
			"conn.release()",
		)
	})

	t.Run("ordinary migration marker is transactional", func(t *testing.T) {
		requireSource(t,
			"beginmigration(ctx)",
			"tx.queryrow",
			"tx.exec",
			"insert into schema_migrations",
			"tx.commit",
			"tx.rollback",
		)
	})

	t.Run("nontransactional directive uses a dedicated path", func(t *testing.T) {
		requireSource(t,
			"citeloop:migration-mode=nontransactional",
			"citeloop:index=",
			"migrationmodenontransactional",
			"applynontransactionalmigration",
		)
	})

	t.Run("invalid concurrent index is recovered safely", func(t *testing.T) {
		requireSource(t,
			"from pg_class",
			"join pg_index",
			"indisvalid",
			"drop index concurrently",
			"pgx.identifier",
			".sanitize()",
		)
	})

	t.Run("malformed directives and statements fail closed", func(t *testing.T) {
		requireSource(t,
			"invalid migration directive",
			"invalid concurrent index name",
			"exactly one create index concurrently statement",
		)
	})

	t.Run("nontransactional work has a bounded context", func(t *testing.T) {
		requireSource(t,
			"context.withtimeout",
			"nontransactionalmigrationtimeout",
		)
	})
}

func TestEmbeddedNontransactionalIndexMigrationContract(t *testing.T) {
	expectedConcurrent := map[string]string{
		"0049_01_doctor_findings_project_identity.sql":         "seo_doctor_findings_project_id_kind_key",
		"0049_02_discovery_candidates_project_identity.sql":    "discovery_candidates_project_id_id_key",
		"0049_03_work_signature_registry_project_identity.sql": "work_signature_registry_project_candidate_id_key",
		"0051_01_site_change_active_content_action.sql":        "idx_active_site_change_application_content_action",
		"0051_02_site_change_active_site_fix.sql":              "uniq_active_site_change_application_site_fix",
		"0051_03_site_change_site_fix_history.sql":             "idx_site_change_applications_site_fix",
		"0051_04_rollback_records_site_fix.sql":                "idx_rollback_records_site_fix_id",
		"0051_05_discovery_candidates_shadow_run.sql":          "idx_discovery_candidates_shadow_run_fk",
		"0051_06_work_signature_registry_shadow_run.sql":       "idx_work_signature_registry_shadow_run_fk",
	}

	for name, indexName := range expectedConcurrent {
		name, indexName := name, indexName
		t.Run(name, func(t *testing.T) {
			raw, err := fs.ReadFile(migrations.FS, name)
			if err != nil {
				t.Errorf("read embedded migration: %v", err)
				return
			}
			sql := strings.ToLower(string(raw))
			spec, err := parseMigrationSpec(name, raw)
			if err != nil {
				t.Fatalf("parse embedded migration: %v", err)
			}
			if spec.Mode != migrationModeNontransactional || spec.IndexName != indexName {
				t.Fatalf("unexpected embedded migration spec: %+v", spec)
			}
			for _, want := range []string{
				"-- citeloop:migration-mode=nontransactional",
				"-- citeloop:index=" + indexName,
				"index concurrently if not exists " + indexName,
			} {
				if !strings.Contains(sql, want) {
					t.Errorf("migration missing %q", want)
				}
			}
			if got := strings.Count(sql, "create "); got != 1 {
				t.Errorf("nontransactional migration must contain one CREATE statement, got %d", got)
			}
		})
	}

	grouped, err := fs.ReadFile(migrations.FS, "0050_doctor_site_fix_indexes.sql")
	if err != nil {
		t.Fatal(err)
	}
	groupedSQL := strings.ToLower(string(grouped))
	for _, populatedTable := range []string{
		"site_change_applications",
		"rollback_records",
		"discovery_candidates",
		"work_signature_registry",
	} {
		if strings.Contains(groupedSQL, "on "+populatedTable+" ") {
			t.Errorf("grouped migration must not build an ordinary index on populated table %s", populatedTable)
		}
	}
}

func TestParseMigrationSpecFailsClosed(t *testing.T) {
	valid := "-- citeloop:migration-mode=nontransactional\n-- citeloop:index=idx_safe\ncreate index concurrently if not exists idx_safe on widgets (id);"
	spec, err := parseMigrationSpec("valid.sql", []byte(valid))
	if err != nil {
		t.Fatalf("parse valid migration: %v", err)
	}
	if spec.Mode != migrationModeNontransactional || spec.IndexName != "idx_safe" {
		t.Fatalf("unexpected spec: %+v", spec)
	}

	for name, sql := range map[string]string{
		"unknown directive":   "-- citeloop:wat=yes\nselect 1;",
		"missing index":       "-- citeloop:migration-mode=nontransactional\ncreate index concurrently if not exists idx_safe on widgets (id);",
		"unsafe index":        "-- citeloop:migration-mode=nontransactional\n-- citeloop:index=idx_safe;drop_table\ncreate index concurrently if not exists idx_safe on widgets (id);",
		"name mismatch":       "-- citeloop:migration-mode=nontransactional\n-- citeloop:index=idx_other\ncreate index concurrently if not exists idx_safe on widgets (id);",
		"not concurrent":      "-- citeloop:migration-mode=nontransactional\n-- citeloop:index=idx_safe\ncreate index if not exists idx_safe on widgets (id);",
		"multiple statements": valid + "\nselect 1;",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseMigrationSpec(name+".sql", []byte(sql)); err == nil {
				t.Fatal("expected fail-closed parse error")
			}
		})
	}
}

type migrationTestRow struct {
	value bool
	err   error
}

func (r migrationTestRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != 1 {
		return errors.New("unexpected scan destination count")
	}
	value, ok := dest[0].(*bool)
	if !ok {
		return errors.New("unexpected scan destination type")
	}
	*value = r.value
	return nil
}

type migrationTestConn struct {
	events        []string
	marker        bool
	indexExists   bool
	indexValid    bool
	boundedNonTxn bool
}

func (c *migrationTestConn) record(event string) { c.events = append(c.events, event) }

func (c *migrationTestConn) Exec(ctx context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	lower := strings.ToLower(sql)
	switch {
	case strings.Contains(lower, "pg_advisory_lock"):
		c.record("lock")
	case strings.Contains(lower, "pg_advisory_unlock"):
		c.record("unlock")
	case strings.Contains(lower, "create table if not exists schema_migrations"):
		c.record("schema")
	case strings.HasPrefix(strings.TrimSpace(lower), "drop index concurrently"):
		c.record("drop-invalid")
		c.indexExists = false
		_, c.boundedNonTxn = ctx.Deadline()
	case strings.Contains(lower, "index concurrently"):
		c.record("create-concurrent")
		c.indexExists, c.indexValid = true, true
		_, c.boundedNonTxn = ctx.Deadline()
	case strings.Contains(lower, "insert into schema_migrations"):
		c.record("mark-nontransactional")
		c.marker = true
	default:
		c.record("conn-exec")
	}
	return pgconn.NewCommandTag("OK"), nil
}

func (c *migrationTestConn) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	lower := strings.ToLower(sql)
	if strings.Contains(lower, "schema_migrations") {
		c.record("marker-check")
		return migrationTestRow{value: c.marker}
	}
	c.record("index-check")
	if !c.indexExists {
		return migrationTestRow{err: pgx.ErrNoRows}
	}
	return migrationTestRow{value: c.indexValid}
}

func (c *migrationTestConn) BeginMigration(_ context.Context) (migrationTx, error) {
	c.record("begin")
	return &migrationTestTx{conn: c}, nil
}

type migrationTestTx struct {
	conn          *migrationTestConn
	pendingMarker bool
	closed        bool
}

func (tx *migrationTestTx) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	tx.conn.record("tx-marker-check")
	return migrationTestRow{value: tx.conn.marker}
}

func (tx *migrationTestTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	if strings.Contains(strings.ToLower(sql), "insert into schema_migrations") {
		tx.conn.record("tx-mark")
		tx.pendingMarker = true
	} else {
		tx.conn.record("tx-migration")
	}
	return pgconn.NewCommandTag("OK"), nil
}

func (tx *migrationTestTx) Commit(_ context.Context) error {
	tx.conn.record("commit")
	tx.conn.marker = tx.pendingMarker
	tx.closed = true
	return nil
}

func (tx *migrationTestTx) Rollback(_ context.Context) error {
	if !tx.closed {
		tx.conn.record("rollback")
	}
	return nil
}

func TestRunMigrationPassLocksAndCommitsOrdinaryMigrationAtomically(t *testing.T) {
	conn := &migrationTestConn{}
	migrationFS := fstest.MapFS{
		"0001_test.sql": &fstest.MapFile{Data: []byte("create table test_table (id int);")},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := runMigrationPass(context.Background(), conn, migrationFS, log); err != nil {
		t.Fatal(err)
	}
	want := []string{"lock", "schema", "begin", "tx-marker-check", "tx-migration", "tx-mark", "commit", "unlock"}
	if strings.Join(conn.events, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected migration order: got %v want %v", conn.events, want)
	}
	if !conn.marker {
		t.Fatal("transactional marker was not committed")
	}
}

func TestRunMigrationPassRecoversInvalidConcurrentIndex(t *testing.T) {
	conn := &migrationTestConn{indexExists: true, indexValid: false}
	migrationFS := fstest.MapFS{
		"0001_index.sql": &fstest.MapFile{Data: []byte("-- citeloop:migration-mode=nontransactional\n-- citeloop:index=idx_safe\ncreate index concurrently if not exists idx_safe on widgets (id);")},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := runMigrationPass(ctx, conn, migrationFS, log); err != nil {
		t.Fatal(err)
	}
	want := []string{"lock", "schema", "marker-check", "index-check", "drop-invalid", "create-concurrent", "mark-nontransactional", "unlock"}
	if strings.Join(conn.events, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected recovery order: got %v want %v", conn.events, want)
	}
	if !conn.boundedNonTxn {
		t.Fatal("nontransactional index execution must have a deadline")
	}
}

func TestRunMigrationPassMarksExistingValidConcurrentIndex(t *testing.T) {
	conn := &migrationTestConn{indexExists: true, indexValid: true}
	migrationFS := fstest.MapFS{
		"0001_index.sql": &fstest.MapFile{Data: []byte("-- citeloop:migration-mode=nontransactional\n-- citeloop:index=idx_safe\ncreate index concurrently if not exists idx_safe on widgets (id);")},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := runMigrationPass(context.Background(), conn, migrationFS, log); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(conn.events, ","), "create-concurrent") {
		t.Fatalf("valid existing index should be marked without rebuild: %v", conn.events)
	}
	if !conn.marker {
		t.Fatal("valid existing index was not marked applied")
	}
}
