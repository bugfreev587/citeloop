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
			"beginmigration(migrationctx)",
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
			"pg_get_indexdef",
			"indisready",
			"indisunique",
			"indkey",
			"indexprs",
			"indoption",
			"indclass",
			"indcollation",
			"pg_get_expr(i.indpred",
			"index definition mismatch",
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

	t.Run("transactional work has a bounded context", func(t *testing.T) {
		requireSource(t,
			"transactionalmigrationtimeout",
			"applytransactionalmigration",
			"context.withtimeout",
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
		"0052_01_work_signature_shadow_candidate.sql":          "uniq_work_signature_registry_shadow_candidate",
		"0054_01_discovery_arbitration_project_identity.sql":   "discovery_arbitration_decisions_project_id_id_key",
		"0057_02_site_fix_pr_claim_index.sql":                  "site_change_applications_pr_claim_expiry_idx",
		"0059_01_legacy_cutover_project_indexes.sql":           "seo_opportunity_review_states_project_id_id_key",
		"0059_02_content_action_canonical_site_fix_index.sql":  "idx_content_actions_canonical_site_fix",
		"0059_03_opportunity_canonical_site_fix_index.sql":     "idx_seo_opportunities_canonical_site_fix",
		"0059_04_work_review_legacy_state_index.sql":           "idx_work_review_memory_legacy_state",
		"0059_05_active_legacy_alias_index.sql":                "uniq_active_legacy_object_alias",
		"0064_canonical_growth_legacy_index.sql":               "idx_seo_opportunities_legacy_growth_migration",
	}
	actualConcurrent := make(map[string]struct{})
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		raw, err := fs.ReadFile(migrations.FS, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(string(raw)), nonTransactionalModeDirective) {
			actualConcurrent[entry.Name()] = struct{}{}
		}
	}
	if len(actualConcurrent) != len(expectedConcurrent) {
		t.Fatalf("embedded nontransactional migrations = %v, expected %v", actualConcurrent, expectedConcurrent)
	}
	for name := range actualConcurrent {
		if _, ok := expectedConcurrent[name]; !ok {
			t.Fatalf("unexpected nontransactional embedded migration %s", name)
		}
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

	base, err := fs.ReadFile(migrations.FS, "0048_doctor_site_fixes.sql")
	if err != nil {
		t.Fatal(err)
	}
	baseSQL := strings.ToLower(string(base))
	for _, want := range []string{
		"set local lock_timeout = '5s'",
		"set local statement_timeout = '4min'",
		"reset statement_timeout",
		"reset lock_timeout",
	} {
		if !strings.Contains(baseSQL, want) {
			t.Errorf("0048 migration missing %q", want)
		}
	}
}

func TestCanonicalGrowthLegacyIndexMigrationRecoversInvalidIndex(t *testing.T) {
	raw, err := fs.ReadFile(migrations.FS, "0064_canonical_growth_legacy_index.sql")
	if err != nil {
		t.Fatal(err)
	}
	definition := "create index idx_seo_opportunities_legacy_growth_migration on seo_opportunities using btree (project_id, created_at, id) where canonical_growth = false and status in ('open', 'accepted', 'converted', 'snoozed', 'watching')"
	conn := &migrationTestConn{
		indexExists:          true,
		indexValid:           false,
		indexDefinition:      definition,
		postCreateDefinition: definition,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	migrationFS := fstest.MapFS{
		"0064_canonical_growth_legacy_index.sql": &fstest.MapFile{Data: raw},
	}
	if err := runMigrationPass(context.Background(), conn, migrationFS, log); err != nil {
		t.Fatal(err)
	}
	want := []string{"lock", "schema", "marker-check", "index-check", "drop-invalid", "create-concurrent", "index-check", "mark-nontransactional", "unlock"}
	if strings.Join(conn.events, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected canonical Growth index recovery order: got %v want %v", conn.events, want)
	}
}

func TestPhaseTwoMigrationOrderingKeepsDDLBeforeIndexAndValidationAfterAdd(t *testing.T) {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	positions := make(map[string]int)
	for index, entry := range entries {
		positions[entry.Name()] = index
	}
	ordered := []string{
		"0063_canonical_growth_writer_cutover.sql",
		"0064_canonical_growth_legacy_index.sql",
		"0065_fix_grounding_verification_stage.sql",
		"0066_validate_fix_grounding_verification_stage.sql",
	}
	for index, name := range ordered {
		position, ok := positions[name]
		if !ok {
			t.Fatalf("ordered migration %s is not embedded", name)
		}
		if index > 0 && position <= positions[ordered[index-1]] {
			t.Fatalf("migration %s must run after %s", name, ordered[index-1])
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

func TestCanonicalIndexDefinitionMatchesPostgresDeparse(t *testing.T) {
	migrationSQL := `create unique index concurrently if not exists idx_safe
		on widgets (project_id, email, updated_at desc)
		where deleted_at is not null and status in ('ready','failed');`
	postgresDefinition := `CREATE UNIQUE INDEX idx_safe ON public.widgets USING btree
		(project_id, email, updated_at DESC)
		WHERE ((deleted_at IS NOT NULL) AND (status = ANY (ARRAY['ready'::text, 'failed'::text])))`
	want, err := canonicalIndexDefinition(migrationSQL)
	if err != nil {
		t.Fatal(err)
	}
	got, err := canonicalIndexDefinition(postgresDefinition)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("canonical definitions differ:\n migration: %s\n postgres:  %s", want, got)
	}
}

func TestCanonicalSingleLiteralPredicateMatchesPostgresDeparse(t *testing.T) {
	migrationSQL := `create unique index concurrently if not exists idx_safe
		on work_signature_registry (candidate_id)
		where mode in ('sha''dow');`
	deparsed := []string{
		`CREATE UNIQUE INDEX idx_safe ON public.work_signature_registry USING btree
			(candidate_id) WHERE (mode = 'sha''dow'::text)`,
		`CREATE UNIQUE INDEX idx_safe ON public.work_signature_registry USING btree
			(candidate_id) WHERE (mode = ANY (ARRAY['sha''dow'::text]))`,
	}
	want, err := canonicalIndexDefinition(migrationSQL)
	if err != nil {
		t.Fatal(err)
	}
	for _, postgresDefinition := range deparsed {
		got, err := canonicalIndexDefinition(postgresDefinition)
		if err != nil {
			t.Fatalf("parse PostgreSQL deparse %q: %v", postgresDefinition, err)
		}
		if got != want {
			t.Fatalf("canonical definitions differ:\n migration: %s\n postgres:  %s", want, got)
		}
	}
}

func TestCanonicalLiteralEqualityPredicateFailsClosed(t *testing.T) {
	for name, sql := range map[string]string{
		"identifier rhs": "create index idx_safe on widgets (id) where mode = other_mode;",
		"numeric rhs":    "create index idx_safe on widgets (id) where mode = 1;",
		"or predicate":   "create index idx_safe on widgets (id) where mode = 'shadow' or active is not null;",
		"unsafe cast":    "create index idx_safe on widgets (id) where mode = 'shadow'::regclass;",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := canonicalIndexDefinition(sql); err == nil {
				t.Fatal("expected unsupported equality predicate to fail closed")
			}
		})
	}
}

func TestCanonicalIndexDefinitionPreservesSQLSemantics(t *testing.T) {
	for name, pair := range map[string][2]string{
		"literal case": {
			"create index idx_safe on widgets (id) where status in ('ready');",
			"create index idx_safe on widgets (id) where status in ('READY');",
		},
		"boolean grouping": {
			"create index idx_safe on widgets (id) where (a or b) and c;",
			"create index idx_safe on widgets (id) where a or (b and c);",
		},
		"expression grouping": {
			"create index idx_safe on widgets ((a + b) * c);",
			"create index idx_safe on widgets (a + (b * c));",
		},
		"identifier versus cast expression": {
			"create index idx_safe on widgets (id);",
			"create index idx_safe on widgets (((id)::text));",
		},
	} {
		t.Run(name, func(t *testing.T) {
			left, leftErr := canonicalIndexDefinition(pair[0])
			right, rightErr := canonicalIndexDefinition(pair[1])
			if leftErr == nil && rightErr == nil && left == right {
				t.Fatalf("semantically distinct definitions collapsed to %q", left)
			}
		})
	}
}

type migrationTestRow struct {
	value           bool
	ready           bool
	relationIsIndex bool
	definition      string
	err             error
}

func (r migrationTestRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	switch len(dest) {
	case 1:
		value, ok := dest[0].(*bool)
		if !ok {
			return errors.New("unexpected scan destination type")
		}
		*value = r.value
	case 3:
		isIndex, okIndex := dest[0].(*bool)
		valid, okValid := dest[1].(*bool)
		definition, okDefinition := dest[2].(*string)
		if !okIndex || !okValid || !okDefinition {
			return errors.New("unexpected index scan destination types")
		}
		*isIndex, *valid, *definition = r.relationIsIndex, r.value, r.definition
	case 14:
		isIndex, okIndex := dest[0].(*bool)
		valid, okValid := dest[1].(*bool)
		ready, okReady := dest[2].(*bool)
		unique, okUnique := dest[3].(*bool)
		currentSchema, okSchema := dest[4].(*bool)
		stringDestinations := make([]*string, 0, 9)
		stringsOK := true
		for _, destination := range dest[5:] {
			value, ok := destination.(*string)
			stringsOK = stringsOK && ok
			stringDestinations = append(stringDestinations, value)
		}
		if !okIndex || !okValid || !okReady || !okUnique || !okSchema || !stringsOK {
			return errors.New("unexpected catalog scan destination types")
		}
		definition, parseErr := parseSupportedIndexDefinition(r.definition)
		keyCount := len(definition.Keys)
		if keyCount == 0 {
			keyCount = 1
		}
		indKey, indOption := make([]string, keyCount), make([]string, keyCount)
		opclasses, collations := make([]string, keyCount), make([]string, keyCount)
		for i := 0; i < keyCount; i++ {
			indKey[i], indOption[i], opclasses[i], collations[i] = "1", "0", "1978", "0"
			if parseErr == nil && definition.Keys[i].Desc {
				indOption[i] = "3"
			}
		}
		predicate := ""
		if where := strings.Index(strings.ToLower(r.definition), " where "); where >= 0 {
			predicate = strings.TrimSpace(r.definition[where+len(" where "):])
		}
		indexExpressions := ""
		if parseErr != nil && (strings.Contains(strings.ToLower(r.definition), "lower(") || strings.Contains(strings.ToLower(r.definition), "::text")) {
			indexExpressions = "expression"
			indKey[0] = "0"
		}
		*isIndex, *valid, *ready = r.relationIsIndex, r.value, r.ready
		*unique, *currentSchema = strings.Contains(strings.ToLower(r.definition), "create unique index"), true
		table := definition.Table
		if table == "" {
			table = "widgets"
		}
		values := []string{
			table, "btree", strings.Join(indKey, " "), indexExpressions,
			strings.Join(indOption, " "), strings.Join(opclasses, " "), strings.Join(collations, " "),
			r.definition, predicate,
		}
		for i, value := range values {
			*stringDestinations[i] = value
		}
	default:
		return errors.New("unexpected scan destination count")
	}
	return nil
}

type migrationTestConn struct {
	events               []string
	marker               bool
	indexExists          bool
	indexValid           bool
	indexDefinition      string
	nonIndexCollision    bool
	postCreateDefinition string
	indexNotReady        bool
	boundedNonTxn        bool
	boundedTxn           bool
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
		c.indexNotReady = false
		if c.postCreateDefinition != "" {
			c.indexDefinition = c.postCreateDefinition
		} else {
			c.indexDefinition = sql
		}
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
	return migrationTestRow{
		value:           c.indexValid,
		ready:           !c.indexNotReady,
		relationIsIndex: !c.nonIndexCollision,
		definition:      c.indexDefinition,
	}
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

func (tx *migrationTestTx) Exec(ctx context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	if strings.Contains(strings.ToLower(sql), "insert into schema_migrations") {
		tx.conn.record("tx-mark")
		tx.pendingMarker = true
	} else {
		tx.conn.record("tx-migration")
		_, tx.conn.boundedTxn = ctx.Deadline()
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
	if !conn.boundedTxn {
		t.Fatal("transactional execution must have a deadline")
	}
}

func TestRunMigrationPassRecoversInvalidConcurrentIndex(t *testing.T) {
	conn := &migrationTestConn{
		indexExists:     true,
		indexValid:      false,
		indexDefinition: "create index idx_safe on public.widgets using btree (id)",
	}
	migrationFS := fstest.MapFS{
		"0001_index.sql": &fstest.MapFile{Data: []byte("-- citeloop:migration-mode=nontransactional\n-- citeloop:index=idx_safe\ncreate index concurrently if not exists idx_safe on widgets (id);")},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := runMigrationPass(ctx, conn, migrationFS, log); err != nil {
		t.Fatal(err)
	}
	want := []string{"lock", "schema", "marker-check", "index-check", "drop-invalid", "create-concurrent", "index-check", "mark-nontransactional", "unlock"}
	if strings.Join(conn.events, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected recovery order: got %v want %v", conn.events, want)
	}
	if !conn.boundedNonTxn {
		t.Fatal("nontransactional index execution must have a deadline")
	}
}

func TestRunMigrationPassRecoversExactInvalidNotReadyConcurrentIndex(t *testing.T) {
	conn := &migrationTestConn{
		indexExists:     true,
		indexValid:      false,
		indexNotReady:   true,
		indexDefinition: "create index idx_safe on public.widgets using btree (id)",
	}
	migrationFS := fstest.MapFS{
		"0001_index.sql": &fstest.MapFile{Data: []byte("-- citeloop:migration-mode=nontransactional\n-- citeloop:index=idx_safe\ncreate index concurrently if not exists idx_safe on widgets (id);")},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := runMigrationPass(context.Background(), conn, migrationFS, log); err != nil {
		t.Fatalf("recover exact invalid/not-ready index: %v", err)
	}
	want := []string{"lock", "schema", "marker-check", "index-check", "drop-invalid", "create-concurrent", "index-check", "mark-nontransactional", "unlock"}
	if strings.Join(conn.events, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected recovery order: got %v want %v", conn.events, want)
	}
}

func TestRunMigrationPassMarksExistingValidConcurrentIndex(t *testing.T) {
	conn := &migrationTestConn{
		indexExists:     true,
		indexValid:      true,
		indexDefinition: "create index idx_safe on public.widgets using btree (id)",
	}
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

func TestRunMigrationPassRejectsValidMismatchedIndexDefinition(t *testing.T) {
	migrationSQL := "-- citeloop:migration-mode=nontransactional\n-- citeloop:index=idx_safe\ncreate index concurrently if not exists idx_safe on widgets (id) where active is not null;"
	for name, definition := range map[string]string{
		"wrong table":      "create index idx_safe on other_widgets using btree (id) where active is not null",
		"wrong uniqueness": "create unique index idx_safe on widgets using btree (id) where active is not null",
		"wrong columns":    "create index idx_safe on widgets using btree (other_id) where active is not null",
		"wrong expression": "create index idx_safe on widgets using btree (lower(id)) where active is not null",
		"wrong predicate":  "create index idx_safe on widgets using btree (id) where archived is not null",
	} {
		t.Run(name, func(t *testing.T) {
			conn := &migrationTestConn{indexExists: true, indexValid: true, indexDefinition: definition}
			migrationFS := fstest.MapFS{"0001_index.sql": &fstest.MapFile{Data: []byte(migrationSQL)}}
			log := slog.New(slog.NewTextHandler(io.Discard, nil))
			if err := runMigrationPass(context.Background(), conn, migrationFS, log); err == nil {
				t.Fatal("expected valid mismatched index to fail closed")
			}
			if strings.Contains(strings.Join(conn.events, ","), "drop-invalid") {
				t.Fatal("valid mismatched index must not be dropped")
			}
		})
	}
}

func TestRunMigrationPassRejectsSameNameNonIndexCollision(t *testing.T) {
	conn := &migrationTestConn{indexExists: true, indexValid: true, nonIndexCollision: true}
	migrationFS := fstest.MapFS{
		"0001_index.sql": &fstest.MapFile{Data: []byte("-- citeloop:migration-mode=nontransactional\n-- citeloop:index=idx_safe\ncreate index concurrently if not exists idx_safe on widgets (id);")},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := runMigrationPass(context.Background(), conn, migrationFS, log); err == nil {
		t.Fatal("expected same-name non-index collision to fail closed")
	}
}

func TestRunMigrationPassRejectsNotReadyIndex(t *testing.T) {
	conn := &migrationTestConn{
		indexExists:     true,
		indexValid:      true,
		indexNotReady:   true,
		indexDefinition: "create index idx_safe on public.widgets using btree (id)",
	}
	migrationFS := fstest.MapFS{
		"0001_index.sql": &fstest.MapFile{Data: []byte("-- citeloop:migration-mode=nontransactional\n-- citeloop:index=idx_safe\ncreate index concurrently if not exists idx_safe on widgets (id);")},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := runMigrationPass(context.Background(), conn, migrationFS, log); err == nil {
		t.Fatal("expected not-ready index to fail before marking")
	}
}

func TestRunMigrationPassDoesNotDropInvalidMismatchedIndex(t *testing.T) {
	conn := &migrationTestConn{
		indexExists:     true,
		indexValid:      false,
		indexDefinition: "create index idx_safe on other_widgets using btree (id)",
	}
	migrationFS := fstest.MapFS{
		"0001_index.sql": &fstest.MapFile{Data: []byte("-- citeloop:migration-mode=nontransactional\n-- citeloop:index=idx_safe\ncreate index concurrently if not exists idx_safe on widgets (id);")},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := runMigrationPass(context.Background(), conn, migrationFS, log); err == nil {
		t.Fatal("expected invalid mismatched index to fail closed")
	}
	if strings.Contains(strings.Join(conn.events, ","), "drop-invalid") {
		t.Fatal("invalid mismatched index must not be dropped")
	}
}

func TestRunMigrationPassRevalidatesCreatedConcurrentIndex(t *testing.T) {
	conn := &migrationTestConn{
		postCreateDefinition: "create index idx_safe on other_widgets using btree (id)",
	}
	migrationFS := fstest.MapFS{
		"0001_index.sql": &fstest.MapFile{Data: []byte("-- citeloop:migration-mode=nontransactional\n-- citeloop:index=idx_safe\ncreate index concurrently if not exists idx_safe on widgets (id);")},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := runMigrationPass(context.Background(), conn, migrationFS, log); err == nil {
		t.Fatal("expected post-create definition mismatch to fail before marking")
	}
	if conn.marker {
		t.Fatal("mismatched created index must not be marked applied")
	}
}
