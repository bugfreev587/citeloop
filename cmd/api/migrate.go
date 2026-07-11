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
	transactionalMigrationTimeout          = 5 * time.Minute
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
	Name            string
	SQL             string
	Mode            migrationMode
	IndexName       string
	IndexDefinition string
	IndexSpec       supportedIndexDefinition
}

type supportedIndexKey struct {
	Column string
	Desc   bool
}

type supportedIndexPredicate struct {
	Column string
	Kind   string
	Values []string
}

type supportedIndexDefinition struct {
	Name       string
	Schema     string
	Table      string
	Unique     bool
	Keys       []supportedIndexKey
	Predicates []supportedIndexPredicate
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
	migrationCtx, cancel := context.WithTimeout(ctx, transactionalMigrationTimeout)
	defer cancel()

	tx, err := conn.BeginMigration(migrationCtx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(migrationCtx)) }()

	var applied bool
	if err := tx.QueryRow(migrationCtx, `select exists(select 1 from schema_migrations where name=$1)`, spec.Name).Scan(&applied); err != nil {
		return fmt.Errorf("recheck migration marker: %w", err)
	}
	if applied {
		return nil
	}
	if _, err := tx.Exec(migrationCtx, spec.SQL); err != nil {
		return fmt.Errorf("execute transactional migration: %w", err)
	}
	if _, err := tx.Exec(migrationCtx, `insert into schema_migrations(name) values($1)`, spec.Name); err != nil {
		return fmt.Errorf("insert migration marker: %w", err)
	}
	if err := tx.Commit(migrationCtx); err != nil {
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

	state, err := inspectMigrationIndex(migrationCtx, conn, spec.IndexName)
	if err != nil {
		return err
	}
	if state.Exists {
		if !state.IsIndex {
			return fmt.Errorf("index definition mismatch for %s: same-name relation is not an index", spec.IndexName)
		}
		if err := validateMigrationIndexDefinition(state, spec); err != nil {
			return err
		}
		if state.Valid && !state.Ready {
			return fmt.Errorf("valid concurrent index %s is not ready", spec.IndexName)
		}
	}

	switch {
	case state.Exists && state.Valid:
		// A previous attempt may have committed the exact index and crashed before its marker.
	case state.Exists:
		identifier := pgx.Identifier{spec.IndexName}.Sanitize()
		if _, err := conn.Exec(migrationCtx, "drop index concurrently "+identifier); err != nil {
			return fmt.Errorf("drop invalid concurrent index: %w", err)
		}
		if _, err := conn.Exec(migrationCtx, spec.SQL); err != nil {
			return fmt.Errorf("execute concurrent index migration: %w", err)
		}
		if err := revalidateMigrationIndex(migrationCtx, conn, spec); err != nil {
			return err
		}
	default:
		if _, err := conn.Exec(migrationCtx, spec.SQL); err != nil {
			return fmt.Errorf("execute concurrent index migration: %w", err)
		}
		if err := revalidateMigrationIndex(migrationCtx, conn, spec); err != nil {
			return err
		}
	}

	if _, err := conn.Exec(migrationCtx, `insert into schema_migrations(name) values($1)`, spec.Name); err != nil {
		return fmt.Errorf("insert nontransactional migration marker: %w", err)
	}
	return nil
}

type migrationIndexState struct {
	Exists                bool
	IsIndex               bool
	Valid                 bool
	Ready                 bool
	Unique                bool
	TargetInCurrentSchema bool
	TargetTable           string
	AccessMethod          string
	IndKey                string
	IndexExpressions      string
	IndOption             string
	Opclasses             string
	Collations            string
	Definition            string
	Predicate             string
}

func inspectMigrationIndex(ctx context.Context, conn migrationConnection, indexName string) (migrationIndexState, error) {
	var state migrationIndexState
	err := conn.QueryRow(ctx, `
		select c.relkind in ('i','I'), coalesce(i.indisvalid, false),
		       coalesce(i.indisready, false), coalesce(i.indisunique, false),
		       coalesce(tn.nspname = current_schema(), false), coalesce(t.relname, ''),
		       coalesce(am.amname, ''), coalesce(i.indkey::text, ''),
		       coalesce(pg_get_expr(i.indexprs, i.indrelid, true), ''),
		       coalesce(i.indoption::text, ''), coalesce(i.indclass::text, ''),
		       coalesce(i.indcollation::text, ''),
		       case when i.indexrelid is null then '' else pg_get_indexdef(c.oid) end,
		       coalesce(pg_get_expr(i.indpred, i.indrelid, true), '')
		from pg_class c
		left join pg_index i on i.indexrelid = c.oid
		left join pg_class t on t.oid = i.indrelid
		left join pg_namespace tn on tn.oid = t.relnamespace
		left join pg_am am on am.oid = c.relam
		join pg_namespace n on n.oid = c.relnamespace
		where n.nspname = current_schema() and c.relname = $1`, indexName).
		Scan(
			&state.IsIndex, &state.Valid, &state.Ready, &state.Unique,
			&state.TargetInCurrentSchema, &state.TargetTable, &state.AccessMethod,
			&state.IndKey, &state.IndexExpressions, &state.IndOption,
			&state.Opclasses, &state.Collations, &state.Definition, &state.Predicate,
		)
	if errors.Is(err, pgx.ErrNoRows) {
		return migrationIndexState{}, nil
	}
	if err != nil {
		return migrationIndexState{}, fmt.Errorf("inspect concurrent index: %w", err)
	}
	state.Exists = true
	return state, nil
}

func revalidateMigrationIndex(ctx context.Context, conn migrationConnection, spec migrationSpec) error {
	state, err := inspectMigrationIndex(ctx, conn, spec.IndexName)
	if err != nil {
		return err
	}
	if !state.Exists || !state.IsIndex || !state.Valid || !state.Ready {
		return fmt.Errorf("created concurrent index %s is missing or invalid", spec.IndexName)
	}
	if err := validateMigrationIndexDefinition(state, spec); err != nil {
		return fmt.Errorf("index definition mismatch for %s after creation", spec.IndexName)
	}
	return nil
}

func validateMigrationIndexDefinition(state migrationIndexState, spec migrationSpec) error {
	if !state.TargetInCurrentSchema || state.TargetTable != spec.IndexSpec.Table {
		return fmt.Errorf("index definition mismatch for %s: target table", spec.IndexName)
	}
	if state.Unique != spec.IndexSpec.Unique || state.AccessMethod != "btree" {
		return fmt.Errorf("index definition mismatch for %s: uniqueness or access method", spec.IndexName)
	}
	if state.IndexExpressions != "" {
		return fmt.Errorf("index definition mismatch for %s: expressions are unsupported", spec.IndexName)
	}
	if !catalogVectorMatchesKeys(state.IndKey, len(spec.IndexSpec.Keys), false) ||
		state.IndOption != expectedIndexOptions(spec.IndexSpec.Keys) ||
		len(strings.Fields(state.Opclasses)) != len(spec.IndexSpec.Keys) ||
		len(strings.Fields(state.Collations)) != len(spec.IndexSpec.Keys) {
		return fmt.Errorf("index definition mismatch for %s: key catalog attributes", spec.IndexName)
	}
	actual, err := parseSupportedIndexDefinition(state.Definition)
	if err != nil || actual.signature() != spec.IndexDefinition {
		return fmt.Errorf("index definition mismatch for %s", spec.IndexName)
	}
	predicates, err := parseSupportedPredicate(state.Predicate)
	if err != nil || predicateSignature(predicates) != predicateSignature(spec.IndexSpec.Predicates) {
		return fmt.Errorf("index definition mismatch for %s: predicate", spec.IndexName)
	}
	return nil
}

func expectedIndexOptions(keys []supportedIndexKey) string {
	options := make([]string, len(keys))
	for i, key := range keys {
		if key.Desc {
			options[i] = "3"
		} else {
			options[i] = "0"
		}
	}
	return strings.Join(options, " ")
}

func catalogVectorMatchesKeys(vector string, keyCount int, options bool) bool {
	fields := strings.Fields(vector)
	if len(fields) != keyCount {
		return false
	}
	for _, field := range fields {
		if options {
			if field != "0" && field != "3" {
				return false
			}
		} else if field == "0" {
			return false
		}
	}
	return true
}

var migrationIndexNamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)
var concurrentIndexStatementPattern = regexp.MustCompile(`(?is)^create\s+(?:unique\s+)?index\s+concurrently\s+if\s+not\s+exists\s+([a-z_][a-z0-9_]*)\s+on\s+.+;$`)
var supportedIndexHeaderPattern = regexp.MustCompile(`(?is)^create\s+(unique\s+)?index\s+(?:concurrently\s+)?(?:if\s+not\s+exists\s+)?([a-z_][a-z0-9_]*)\s+on\s+(.+)$`)
var supportedIndexTablePattern = regexp.MustCompile(`(?is)^(?:([a-z_][a-z0-9_]*)\.)?([a-z_][a-z0-9_]*)\s+(.+)$`)
var supportedIndexKeyPattern = regexp.MustCompile(`(?i)^([a-z_][a-z0-9_]*)(?:\s+(asc|desc))?$`)
var notNullPredicatePattern = regexp.MustCompile(`(?i)^([a-z_][a-z0-9_]*)\s+is\s+not\s+null$`)
var inPredicatePattern = regexp.MustCompile(`(?is)^([a-z_][a-z0-9_]*)\s+in\s*\((.*)\)$`)
var anyArrayPredicatePattern = regexp.MustCompile(`(?is)^([a-z_][a-z0-9_]*)\s*=\s*any\s*\(\s*array\s*\[(.*)\]\s*(?:::text\[\])?\s*\)$`)

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
	spec.IndexSpec, err = parseSupportedIndexDefinition(executable)
	if err != nil {
		return migrationSpec{}, fmt.Errorf("canonicalize index definition in %s: %w", name, err)
	}
	if spec.IndexSpec.Name != spec.IndexName || spec.IndexSpec.Schema != "" {
		return migrationSpec{}, fmt.Errorf("%s has unsupported index name or target schema", name)
	}
	spec.IndexDefinition = spec.IndexSpec.signature()
	return spec, nil
}

func canonicalIndexDefinition(sql string) (string, error) {
	definition, err := parseSupportedIndexDefinition(sql)
	if err != nil {
		return "", err
	}
	return definition.signature(), nil
}

func parseSupportedIndexDefinition(sql string) (supportedIndexDefinition, error) {
	definition := strings.TrimSpace(strings.TrimSuffix(stripSQLLineComments(sql), ";"))
	match := supportedIndexHeaderPattern.FindStringSubmatch(definition)
	if len(match) != 4 {
		return supportedIndexDefinition{}, errors.New("unsupported index definition header")
	}
	tableMatch := supportedIndexTablePattern.FindStringSubmatch(strings.TrimSpace(match[3]))
	if len(tableMatch) != 4 {
		return supportedIndexDefinition{}, errors.New("unsupported index target")
	}
	remainder := strings.TrimSpace(tableMatch[3])
	if strings.HasPrefix(strings.ToLower(remainder), "using ") {
		fields := strings.Fields(remainder)
		if len(fields) < 3 || !strings.EqualFold(fields[1], "btree") {
			return supportedIndexDefinition{}, errors.New("only btree indexes are supported")
		}
		remainder = strings.TrimSpace(remainder[len(fields[0])+1+len(fields[1]):])
	}
	keySQL, trailing, err := extractLeadingParenthesized(remainder)
	if err != nil {
		return supportedIndexDefinition{}, err
	}
	keyParts, err := splitTopLevel(keySQL, ',')
	if err != nil || len(keyParts) == 0 {
		return supportedIndexDefinition{}, errors.New("unsupported index keys")
	}
	keys := make([]supportedIndexKey, 0, len(keyParts))
	for _, keySQL := range keyParts {
		keyMatch := supportedIndexKeyPattern.FindStringSubmatch(strings.TrimSpace(keySQL))
		if len(keyMatch) != 3 {
			return supportedIndexDefinition{}, errors.New("index expressions, casts, collations, and opclasses are unsupported")
		}
		keys = append(keys, supportedIndexKey{Column: strings.ToLower(keyMatch[1]), Desc: strings.EqualFold(keyMatch[2], "desc")})
	}
	var predicates []supportedIndexPredicate
	trailing = strings.TrimSpace(trailing)
	if trailing != "" {
		if len(trailing) < len("where ") || !strings.EqualFold(trailing[:len("where ")], "where ") {
			return supportedIndexDefinition{}, errors.New("unsupported index suffix")
		}
		predicates, err = parseSupportedPredicate(strings.TrimSpace(trailing[len("where "):]))
		if err != nil {
			return supportedIndexDefinition{}, err
		}
	}
	return supportedIndexDefinition{
		Name: strings.ToLower(match[2]), Schema: strings.ToLower(tableMatch[1]),
		Table: strings.ToLower(tableMatch[2]), Unique: match[1] != "", Keys: keys, Predicates: predicates,
	}, nil
}

func (definition supportedIndexDefinition) signature() string {
	var out strings.Builder
	fmt.Fprintf(&out, "unique=%t;name=%s;table=%s;keys=", definition.Unique, definition.Name, definition.Table)
	for i, key := range definition.Keys {
		if i > 0 {
			out.WriteByte(',')
		}
		fmt.Fprintf(&out, "%s:%t", key.Column, key.Desc)
	}
	out.WriteString(";predicate=")
	out.WriteString(predicateSignature(definition.Predicates))
	return out.String()
}

func predicateSignature(predicates []supportedIndexPredicate) string {
	var out strings.Builder
	for i, predicate := range predicates {
		if i > 0 {
			out.WriteString("&&")
		}
		fmt.Fprintf(&out, "%s:%s", predicate.Column, predicate.Kind)
		for _, value := range predicate.Values {
			fmt.Fprintf(&out, ":%q", value)
		}
	}
	return out.String()
}

func parseSupportedPredicate(sql string) ([]supportedIndexPredicate, error) {
	if strings.TrimSpace(sql) == "" {
		return nil, nil
	}
	sql = stripOuterParentheses(strings.TrimSpace(sql))
	terms, err := splitTopLevelKeyword(sql, "and")
	if err != nil || len(terms) == 0 {
		return nil, errors.New("unsupported index predicate")
	}
	predicates := make([]supportedIndexPredicate, 0, len(terms))
	for _, term := range terms {
		term = stripOuterParentheses(strings.TrimSpace(term))
		if match := notNullPredicatePattern.FindStringSubmatch(term); len(match) == 2 {
			predicates = append(predicates, supportedIndexPredicate{Column: strings.ToLower(match[1]), Kind: "not-null"})
			continue
		}
		var match []string
		if candidate := inPredicatePattern.FindStringSubmatch(term); len(candidate) == 3 {
			match = candidate
		} else if candidate := anyArrayPredicatePattern.FindStringSubmatch(term); len(candidate) == 3 {
			match = candidate
		}
		if len(match) != 3 {
			return nil, errors.New("only IS NOT NULL and literal IN predicates joined by AND are supported")
		}
		values, err := parseSQLStringList(match[2])
		if err != nil || len(values) == 0 {
			return nil, errors.New("unsupported index predicate literals")
		}
		predicates = append(predicates, supportedIndexPredicate{Column: strings.ToLower(match[1]), Kind: "in", Values: values})
	}
	return predicates, nil
}

func parseSQLStringList(sql string) ([]string, error) {
	parts, err := splitTopLevel(sql, ',')
	if err != nil {
		return nil, err
	}
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if len(part) < 2 || part[0] != '\'' {
			return nil, errors.New("predicate values must be string literals")
		}
		var value strings.Builder
		end := -1
		for i := 1; i < len(part); i++ {
			if part[i] != '\'' {
				value.WriteByte(part[i])
				continue
			}
			if i+1 < len(part) && part[i+1] == '\'' {
				value.WriteByte('\'')
				i++
				continue
			}
			end = i
			break
		}
		if end < 0 {
			return nil, errors.New("unterminated predicate literal")
		}
		suffix := strings.TrimSpace(part[end+1:])
		if suffix != "" && !strings.EqualFold(suffix, "::text") {
			return nil, errors.New("unsupported predicate literal cast")
		}
		values = append(values, value.String())
	}
	return values, nil
}

func extractLeadingParenthesized(sql string) (string, string, error) {
	sql = strings.TrimSpace(sql)
	if sql == "" || sql[0] != '(' {
		return "", "", errors.New("index keys must be parenthesized")
	}
	depth, quoted := 0, false
	for i := 0; i < len(sql); i++ {
		if sql[i] == '\'' {
			if quoted && i+1 < len(sql) && sql[i+1] == '\'' {
				i++
				continue
			}
			quoted = !quoted
			continue
		}
		if quoted {
			continue
		}
		switch sql[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return sql[1:i], sql[i+1:], nil
			}
			if depth < 0 {
				return "", "", errors.New("unbalanced index definition")
			}
		}
	}
	return "", "", errors.New("unbalanced index definition")
}

func splitTopLevel(sql string, separator byte) ([]string, error) {
	var parts []string
	start, depth, quoted := 0, 0, false
	for i := 0; i < len(sql); i++ {
		if sql[i] == '\'' {
			if quoted && i+1 < len(sql) && sql[i+1] == '\'' {
				i++
				continue
			}
			quoted = !quoted
			continue
		}
		if quoted {
			continue
		}
		switch sql[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return nil, errors.New("unbalanced SQL expression")
			}
		default:
			if sql[i] == separator && depth == 0 {
				parts = append(parts, strings.TrimSpace(sql[start:i]))
				start = i + 1
			}
		}
	}
	if quoted || depth != 0 {
		return nil, errors.New("unbalanced SQL expression")
	}
	parts = append(parts, strings.TrimSpace(sql[start:]))
	return parts, nil
}

func splitTopLevelKeyword(sql, keyword string) ([]string, error) {
	var parts []string
	start, depth, quoted := 0, 0, false
	for i := 0; i < len(sql); i++ {
		if sql[i] == '\'' {
			if quoted && i+1 < len(sql) && sql[i+1] == '\'' {
				i++
				continue
			}
			quoted = !quoted
			continue
		}
		if quoted {
			continue
		}
		if sql[i] == '(' {
			depth++
			continue
		}
		if sql[i] == ')' {
			depth--
			if depth < 0 {
				return nil, errors.New("unbalanced SQL predicate")
			}
			continue
		}
		if depth == 0 && i+len(keyword) <= len(sql) && strings.EqualFold(sql[i:i+len(keyword)], keyword) &&
			(i == 0 || !isIdentifierByte(sql[i-1])) && (i+len(keyword) == len(sql) || !isIdentifierByte(sql[i+len(keyword)])) {
			parts = append(parts, strings.TrimSpace(sql[start:i]))
			i += len(keyword) - 1
			start = i + 1
		}
	}
	if quoted || depth != 0 {
		return nil, errors.New("unbalanced SQL predicate")
	}
	parts = append(parts, strings.TrimSpace(sql[start:]))
	return parts, nil
}

func stripOuterParentheses(sql string) string {
	for {
		inside, trailing, err := extractLeadingParenthesized(sql)
		if err != nil || strings.TrimSpace(trailing) != "" {
			return strings.TrimSpace(sql)
		}
		sql = strings.TrimSpace(inside)
	}
}

func stripSQLLineComments(sql string) string {
	lines := make([]string, 0)
	for _, line := range strings.Split(sql, "\n") {
		quoted := false
		for i := 0; i+1 < len(line); i++ {
			if line[i] == '\'' {
				if quoted && i+1 < len(line) && line[i+1] == '\'' {
					i++
					continue
				}
				quoted = !quoted
				continue
			}
			if !quoted && line[i:i+2] == "--" {
				line = line[:i]
				break
			}
		}
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func isIdentifierByte(value byte) bool {
	return value == '_' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}

func migrationExecutableSQL(sql string) (string, error) {
	if strings.Contains(sql, "/*") || strings.Contains(sql, "*/") {
		return "", errors.New("nontransactional migrations do not allow block comments")
	}
	executable := strings.TrimSpace(stripSQLLineComments(sql))
	if strings.Count(executable, ";") != 1 || !strings.HasSuffix(executable, ";") {
		return "", errors.New("nontransactional migration must contain exactly one CREATE INDEX CONCURRENTLY statement")
	}
	return executable, nil
}
