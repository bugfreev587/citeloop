//go:build integration

package db

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestGitHubPRReadinessPostgresPersistence(t *testing.T) {
	harness := newGitHubPRReadinessPostgresHarness(t)
	defer harness.close()

	t.Run("current version persists ready state", func(t *testing.T) {
		projectID := harness.insertProject(t)
		initial := harness.insertConnectedGitHubConnection(
			t,
			projectID,
			"not_checked",
			nil,
			nil,
			githubPRReadinessFixedTime(1),
		)
		checkedAt := githubPRReadinessFixedTime(2)
		detail := "installation can create pull requests"

		ready, err := harness.queries.SetGitHubPRReadinessIfUnchanged(
			harness.ctx,
			SetGitHubPRReadinessIfUnchangedParams{
				PrReadinessStatus:    "ready",
				PrReadinessCheckedAt: githubPRReadinessTimestamptz(checkedAt),
				PrReadinessDetail:    &detail,
				ConnectionID:         initial.ID,
				ProjectID:            projectID,
				ExpectedUpdatedAt:    initial.UpdatedAt,
			},
		)
		if err != nil {
			t.Fatalf("set readiness with current updated_at: %v", err)
		}
		assertGitHubPRReadiness(t, ready, "ready", &checkedAt, &detail)
		if !ready.UpdatedAt.Valid || !ready.UpdatedAt.Time.Equal(initial.UpdatedAt.Time) {
			t.Fatalf("readiness-only CAS changed connection version: before=%v after=%v", initial.UpdatedAt, ready.UpdatedAt)
		}

		persisted := harness.getConnection(t, projectID, initial.ID)
		assertGitHubPRReadiness(t, persisted, "ready", &checkedAt, &detail)
		assertGitHubPRReadinessTimestamp(t, persisted.UpdatedAt, ready.UpdatedAt, "persisted updated_at")

		refreshedAt := githubPRReadinessFixedTime(3)
		refreshed, err := harness.queries.SetGitHubPRReadinessIfUnchanged(harness.ctx, SetGitHubPRReadinessIfUnchangedParams{
			PrReadinessStatus: "ready", PrReadinessCheckedAt: githubPRReadinessTimestamptz(refreshedAt),
			PrReadinessDetail: &detail, ConnectionID: initial.ID, ProjectID: projectID, ExpectedUpdatedAt: initial.UpdatedAt,
		})
		if err != nil {
			t.Fatalf("refresh readiness on the same connection version: %v", err)
		}
		assertGitHubPRReadinessTimestamp(t, refreshed.UpdatedAt, initial.UpdatedAt, "refreshed connection updated_at")
	})

	t.Run("older readiness cannot overwrite a newer downgrade", func(t *testing.T) {
		projectID := harness.insertProject(t)
		initial := harness.insertConnectedGitHubConnection(t, projectID, "ready", nil, nil, githubPRReadinessFixedTime(20))
		newerAt, olderAt := githubPRReadinessFixedTime(22), githubPRReadinessFixedTime(21)
		detail := "repository permission was revoked"
		newer, err := harness.queries.SetGitHubPRReadinessIfUnchanged(harness.ctx, SetGitHubPRReadinessIfUnchangedParams{
			PrReadinessStatus: "permission_missing", PrReadinessCheckedAt: githubPRReadinessTimestamptz(newerAt),
			PrReadinessDetail: &detail, ConnectionID: initial.ID, ProjectID: projectID, ExpectedUpdatedAt: initial.UpdatedAt,
		})
		if err != nil {
			t.Fatalf("persist newer downgrade: %v", err)
		}
		_, err = harness.queries.SetGitHubPRReadinessIfUnchanged(harness.ctx, SetGitHubPRReadinessIfUnchangedParams{
			PrReadinessStatus: "ready", PrReadinessCheckedAt: githubPRReadinessTimestamptz(olderAt),
			ConnectionID: initial.ID, ProjectID: projectID, ExpectedUpdatedAt: initial.UpdatedAt,
		})
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("older readiness overwrite err=%v, want pgx.ErrNoRows", err)
		}
		persisted := harness.getConnection(t, projectID, initial.ID)
		assertGitHubPRReadiness(t, persisted, "permission_missing", &newerAt, &detail)
		assertGitHubPRReadinessTimestamp(t, persisted.UpdatedAt, newer.UpdatedAt, "downgrade connection updated_at")
	})

	t.Run("stale version cannot overwrite connection invalidation", func(t *testing.T) {
		projectID := harness.insertProject(t)
		initial := harness.insertConnectedGitHubConnection(
			t,
			projectID,
			"not_checked",
			nil,
			nil,
			githubPRReadinessFixedTime(3),
		)
		staleUpdatedAt := initial.UpdatedAt

		invalidated, err := harness.queries.SetPublisherConnectionEnabled(
			harness.ctx,
			SetPublisherConnectionEnabledParams{
				ID:        initial.ID,
				ProjectID: projectID,
				Enabled:   false,
			},
		)
		if err != nil {
			t.Fatalf("disable connection: %v", err)
		}
		assertGitHubPRReadiness(t, invalidated, "not_connected", nil, nil)
		if invalidated.Enabled {
			t.Fatal("disabled connection remained enabled")
		}
		if !invalidated.UpdatedAt.Valid || invalidated.UpdatedAt.Time.Equal(staleUpdatedAt.Time) {
			t.Fatalf("connection mutation did not produce a distinct updated_at: before=%v after=%v", staleUpdatedAt, invalidated.UpdatedAt)
		}

		checkedAt := githubPRReadinessFixedTime(4)
		staleDetail := "stale readiness result"
		_, err = harness.queries.SetGitHubPRReadinessIfUnchanged(
			harness.ctx,
			SetGitHubPRReadinessIfUnchangedParams{
				PrReadinessStatus:    "ready",
				PrReadinessCheckedAt: githubPRReadinessTimestamptz(checkedAt),
				PrReadinessDetail:    &staleDetail,
				ConnectionID:         initial.ID,
				ProjectID:            projectID,
				ExpectedUpdatedAt:    staleUpdatedAt,
			},
		)
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("stale CAS error=%v, want pgx.ErrNoRows", err)
		}

		persisted := harness.getConnection(t, projectID, initial.ID)
		if persisted.Enabled {
			t.Fatal("stale CAS re-enabled the connection")
		}
		assertGitHubPRReadiness(t, persisted, "not_connected", nil, nil)
		assertGitHubPRReadinessTimestamp(t, persisted.UpdatedAt, invalidated.UpdatedAt, "updated_at after rejected stale CAS")
	})

	t.Run("readiness lookup and CAS enforce project scope", func(t *testing.T) {
		projectID := harness.insertProject(t)
		wrongProjectID := harness.insertProject(t)
		checkedAt := githubPRReadinessFixedTime(5)
		detail := "owner tenant state"
		connection := harness.insertConnectedGitHubConnection(
			t,
			projectID,
			"ready",
			&checkedAt,
			&detail,
			githubPRReadinessFixedTime(6),
		)

		_, err := harness.queries.GetGitHubPRReadinessForProject(harness.ctx, wrongProjectID)
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("wrong-project readiness lookup error=%v, want pgx.ErrNoRows", err)
		}

		wrongCheckedAt := githubPRReadinessFixedTime(7)
		wrongDetail := "cross-tenant overwrite"
		_, err = harness.queries.SetGitHubPRReadinessIfUnchanged(
			harness.ctx,
			SetGitHubPRReadinessIfUnchangedParams{
				PrReadinessStatus:    "error",
				PrReadinessCheckedAt: githubPRReadinessTimestamptz(wrongCheckedAt),
				PrReadinessDetail:    &wrongDetail,
				ConnectionID:         connection.ID,
				ProjectID:            wrongProjectID,
				ExpectedUpdatedAt:    connection.UpdatedAt,
			},
		)
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("wrong-project readiness CAS error=%v, want pgx.ErrNoRows", err)
		}

		persisted := harness.getConnection(t, projectID, connection.ID)
		assertGitHubPRReadiness(t, persisted, "ready", &checkedAt, &detail)
		assertGitHubPRReadinessTimestamp(t, persisted.UpdatedAt, connection.UpdatedAt, "owner connection updated_at")
		if count := harness.connectionCount(t, wrongProjectID); count != 0 {
			t.Fatalf("wrong project gained %d publisher connections, want 0", count)
		}
	})

	t.Run("credential upsert persists and invalidates atomically", func(t *testing.T) {
		projectID := harness.insertProject(t)
		wrongProjectID := harness.insertProject(t)
		checkedAt := githubPRReadinessFixedTime(8)
		detail := "ready before credential rotation"
		connection := harness.insertConnectedGitHubConnection(
			t,
			projectID,
			"ready",
			&checkedAt,
			&detail,
			githubPRReadinessFixedTime(9),
		)

		credential, err := harness.queries.UpsertPublisherCredential(
			harness.ctx,
			UpsertPublisherCredentialParams{
				ProjectID:      projectID,
				ConnectionID:   connection.ID,
				Kind:           "github_token",
				EncryptedValue: "encrypted-valid-token",
				RedactedValue:  "ghp_...valid",
			},
		)
		if err != nil {
			t.Fatalf("upsert valid credential: %v", err)
		}
		assertGitHubPRReadinessCredential(
			t,
			credential,
			projectID,
			connection.ID,
			"encrypted-valid-token",
			"ghp_...valid",
			false,
		)
		persistedCredential := harness.getCredential(t, projectID, connection.ID, "github_token")
		assertGitHubPRReadinessCredential(
			t,
			persistedCredential,
			projectID,
			connection.ID,
			"encrypted-valid-token",
			"ghp_...valid",
			false,
		)

		invalidated := harness.getConnection(t, projectID, connection.ID)
		assertGitHubPRReadiness(t, invalidated, "not_checked", nil, nil)
		if !invalidated.UpdatedAt.Valid || invalidated.UpdatedAt.Time.Equal(connection.UpdatedAt.Time) {
			t.Fatalf("credential upsert did not advance connection updated_at: before=%v after=%v", connection.UpdatedAt, invalidated.UpdatedAt)
		}

		missingConnectionID := uuid.New()
		_, err = harness.queries.UpsertPublisherCredential(
			harness.ctx,
			UpsertPublisherCredentialParams{
				ProjectID:      projectID,
				ConnectionID:   missingConnectionID,
				Kind:           "github_token",
				EncryptedValue: "encrypted-missing",
				RedactedValue:  "missing",
			},
		)
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("missing-connection credential upsert error=%v, want pgx.ErrNoRows", err)
		}
		if count := harness.credentialCount(t, projectID, missingConnectionID, "github_token"); count != 0 {
			t.Fatalf("missing connection gained %d credentials, want 0", count)
		}

		_, err = harness.queries.UpsertPublisherCredential(
			harness.ctx,
			UpsertPublisherCredentialParams{
				ProjectID:      wrongProjectID,
				ConnectionID:   connection.ID,
				Kind:           "github_token",
				EncryptedValue: "encrypted-cross-tenant",
				RedactedValue:  "cross-tenant",
			},
		)
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("wrong-project credential upsert error=%v, want pgx.ErrNoRows", err)
		}
		if count := harness.credentialCount(t, wrongProjectID, connection.ID, "github_token"); count != 0 {
			t.Fatalf("wrong project gained %d credentials, want 0", count)
		}

		persistedCredential = harness.getCredential(t, projectID, connection.ID, "github_token")
		assertGitHubPRReadinessCredential(
			t,
			persistedCredential,
			projectID,
			connection.ID,
			"encrypted-valid-token",
			"ghp_...valid",
			false,
		)
		persistedConnection := harness.getConnection(t, projectID, connection.ID)
		assertGitHubPRReadiness(t, persistedConnection, "not_checked", nil, nil)
		assertGitHubPRReadinessTimestamp(t, persistedConnection.UpdatedAt, invalidated.UpdatedAt, "connection updated_at after rejected credential upserts")
	})

	t.Run("credential revoke persists and invalidates atomically", func(t *testing.T) {
		projectID := harness.insertProject(t)
		wrongProjectID := harness.insertProject(t)
		checkedAt := githubPRReadinessFixedTime(10)
		detail := "ready before credential revoke"
		connection := harness.insertConnectedGitHubConnection(
			t,
			projectID,
			"ready",
			&checkedAt,
			&detail,
			githubPRReadinessFixedTime(11),
		)
		harness.insertCredential(
			t,
			projectID,
			connection.ID,
			"github_token",
			"encrypted-active-token",
			"ghp_...active",
		)

		_, err := harness.queries.RevokePublisherCredentialForConnection(
			harness.ctx,
			RevokePublisherCredentialForConnectionParams{
				Kind:         "github_token",
				ConnectionID: uuid.New(),
				ProjectID:    projectID,
			},
		)
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("missing-connection credential revoke error=%v, want pgx.ErrNoRows", err)
		}

		_, err = harness.queries.RevokePublisherCredentialForConnection(
			harness.ctx,
			RevokePublisherCredentialForConnectionParams{
				Kind:         "webhook_secret",
				ConnectionID: connection.ID,
				ProjectID:    projectID,
			},
		)
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("missing-credential revoke error=%v, want pgx.ErrNoRows", err)
		}

		_, err = harness.queries.RevokePublisherCredentialForConnection(
			harness.ctx,
			RevokePublisherCredentialForConnectionParams{
				Kind:         "github_token",
				ConnectionID: connection.ID,
				ProjectID:    wrongProjectID,
			},
		)
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("wrong-project credential revoke error=%v, want pgx.ErrNoRows", err)
		}

		untouchedCredential := harness.getCredential(t, projectID, connection.ID, "github_token")
		assertGitHubPRReadinessCredential(
			t,
			untouchedCredential,
			projectID,
			connection.ID,
			"encrypted-active-token",
			"ghp_...active",
			false,
		)
		untouchedConnection := harness.getConnection(t, projectID, connection.ID)
		assertGitHubPRReadiness(t, untouchedConnection, "ready", &checkedAt, &detail)
		assertGitHubPRReadinessTimestamp(t, untouchedConnection.UpdatedAt, connection.UpdatedAt, "connection updated_at after rejected credential revokes")

		revoked, err := harness.queries.RevokePublisherCredentialForConnection(
			harness.ctx,
			RevokePublisherCredentialForConnectionParams{
				Kind:         "github_token",
				ConnectionID: connection.ID,
				ProjectID:    projectID,
			},
		)
		if err != nil {
			t.Fatalf("revoke active credential: %v", err)
		}
		assertGitHubPRReadinessCredential(
			t,
			revoked,
			projectID,
			connection.ID,
			"encrypted-active-token",
			"ghp_...active",
			true,
		)

		persistedCredential := harness.getCredential(t, projectID, connection.ID, "github_token")
		assertGitHubPRReadinessCredential(
			t,
			persistedCredential,
			projectID,
			connection.ID,
			"encrypted-active-token",
			"ghp_...active",
			true,
		)
		if !persistedCredential.RevokedAt.Time.Equal(revoked.RevokedAt.Time) {
			t.Fatalf("persisted revoked_at=%v, returned revoked_at=%v", persistedCredential.RevokedAt, revoked.RevokedAt)
		}
		_, err = harness.queries.GetActivePublisherCredentialForConnection(
			harness.ctx,
			GetActivePublisherCredentialForConnectionParams{
				ProjectID:    projectID,
				ConnectionID: connection.ID,
				Kind:         "github_token",
			},
		)
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("active credential lookup after revoke error=%v, want pgx.ErrNoRows", err)
		}
		if count := harness.credentialCount(t, projectID, connection.ID, "github_token"); count != 1 {
			t.Fatalf("credential row count after revoke=%d, want 1", count)
		}

		persistedConnection := harness.getConnection(t, projectID, connection.ID)
		assertGitHubPRReadiness(t, persistedConnection, "not_checked", nil, nil)
		if !persistedConnection.UpdatedAt.Valid || persistedConnection.UpdatedAt.Time.Equal(connection.UpdatedAt.Time) {
			t.Fatalf("credential revoke did not advance connection updated_at: before=%v after=%v", connection.UpdatedAt, persistedConnection.UpdatedAt)
		}
	})
}

type githubPRReadinessPostgresHarness struct {
	t       *testing.T
	ctx     context.Context
	pool    *pgxpool.Pool
	tx      pgx.Tx
	queries *Queries
}

func newGitHubPRReadinessPostgresHarness(t *testing.T) *githubPRReadinessPostgresHarness {
	t.Helper()
	dsn := os.Getenv("CITELOOP_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CITELOOP_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("configure integration database: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		pool.Close()
		t.Fatalf("begin integration transaction: %v", err)
	}
	return &githubPRReadinessPostgresHarness{
		t:       t,
		ctx:     ctx,
		pool:    pool,
		tx:      tx,
		queries: New(tx),
	}
}

func (h *githubPRReadinessPostgresHarness) close() {
	h.t.Helper()
	if err := h.tx.Rollback(h.ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		h.t.Errorf("rollback integration transaction: %v", err)
	}
	h.pool.Close()
}

func (h *githubPRReadinessPostgresHarness) insertProject(t *testing.T) uuid.UUID {
	t.Helper()
	projectID := uuid.New()
	if _, err := h.tx.Exec(
		h.ctx,
		`insert into projects (id, owner_id, name, slug, config)
		 values ($1, $2, $3, $4, '{}')`,
		projectID,
		"github-pr-readiness-integration",
		"GitHub PR readiness integration",
		"github-pr-readiness-"+projectID.String(),
	); err != nil {
		t.Fatalf("insert project fixture: %v", err)
	}
	return projectID
}

func (h *githubPRReadinessPostgresHarness) insertConnectedGitHubConnection(
	t *testing.T,
	projectID uuid.UUID,
	readinessStatus string,
	checkedAt *time.Time,
	detail *string,
	updatedAt time.Time,
) PublisherConnection {
	t.Helper()
	connectionID := uuid.New()
	var checkedAtValue any
	if checkedAt != nil {
		checkedAtValue = *checkedAt
	}
	var detailValue any
	if detail != nil {
		detailValue = *detail
	}
	if _, err := h.tx.Exec(
		h.ctx,
		`insert into publisher_connections
		   (id, project_id, kind, label, status, is_default, enabled, capabilities,
		    capability_schema_version, config, pr_readiness_status,
		    pr_readiness_checked_at, pr_readiness_detail, updated_at)
		 values
		   ($1, $2, 'github_nextjs', 'Integration GitHub', 'connected', true, true,
		    '{}', 1, '{}', $3, $4, $5, $6)`,
		connectionID,
		projectID,
		readinessStatus,
		checkedAtValue,
		detailValue,
		updatedAt,
	); err != nil {
		t.Fatalf("insert GitHub connection fixture: %v", err)
	}
	return h.getConnection(t, projectID, connectionID)
}

func (h *githubPRReadinessPostgresHarness) insertCredential(
	t *testing.T,
	projectID uuid.UUID,
	connectionID uuid.UUID,
	kind string,
	encryptedValue string,
	redactedValue string,
) {
	t.Helper()
	if _, err := h.tx.Exec(
		h.ctx,
		`insert into publisher_credentials
		   (project_id, connection_id, kind, encrypted_value, redacted_value)
		 values ($1, $2, $3, $4, $5)`,
		projectID,
		connectionID,
		kind,
		encryptedValue,
		redactedValue,
	); err != nil {
		t.Fatalf("insert credential fixture: %v", err)
	}
}

func (h *githubPRReadinessPostgresHarness) getConnection(t *testing.T, projectID, connectionID uuid.UUID) PublisherConnection {
	t.Helper()
	connection, err := h.queries.GetPublisherConnectionForProject(
		h.ctx,
		GetPublisherConnectionForProjectParams{ID: connectionID, ProjectID: projectID},
	)
	if err != nil {
		t.Fatalf("load publisher connection: %v", err)
	}
	return connection
}

func (h *githubPRReadinessPostgresHarness) getCredential(
	t *testing.T,
	projectID uuid.UUID,
	connectionID uuid.UUID,
	kind string,
) PublisherCredential {
	t.Helper()
	var credential PublisherCredential
	err := h.tx.QueryRow(
		h.ctx,
		`select id, project_id, connection_id, kind, encrypted_value, redacted_value,
		        created_at, updated_at, revoked_at
		 from publisher_credentials
		 where project_id = $1 and connection_id = $2 and kind = $3`,
		projectID,
		connectionID,
		kind,
	).Scan(
		&credential.ID,
		&credential.ProjectID,
		&credential.ConnectionID,
		&credential.Kind,
		&credential.EncryptedValue,
		&credential.RedactedValue,
		&credential.CreatedAt,
		&credential.UpdatedAt,
		&credential.RevokedAt,
	)
	if err != nil {
		t.Fatalf("load publisher credential: %v", err)
	}
	return credential
}

func (h *githubPRReadinessPostgresHarness) connectionCount(t *testing.T, projectID uuid.UUID) int {
	t.Helper()
	var count int
	if err := h.tx.QueryRow(
		h.ctx,
		`select count(*) from publisher_connections where project_id = $1`,
		projectID,
	).Scan(&count); err != nil {
		t.Fatalf("count publisher connections: %v", err)
	}
	return count
}

func (h *githubPRReadinessPostgresHarness) credentialCount(
	t *testing.T,
	projectID uuid.UUID,
	connectionID uuid.UUID,
	kind string,
) int {
	t.Helper()
	var count int
	if err := h.tx.QueryRow(
		h.ctx,
		`select count(*)
		 from publisher_credentials
		 where project_id = $1 and connection_id = $2 and kind = $3`,
		projectID,
		connectionID,
		kind,
	).Scan(&count); err != nil {
		t.Fatalf("count publisher credentials: %v", err)
	}
	return count
}

func assertGitHubPRReadiness(
	t *testing.T,
	connection PublisherConnection,
	wantStatus string,
	wantCheckedAt *time.Time,
	wantDetail *string,
) {
	t.Helper()
	if connection.PrReadinessStatus != wantStatus {
		t.Fatalf("pr_readiness_status=%q, want %q", connection.PrReadinessStatus, wantStatus)
	}
	if wantCheckedAt == nil {
		if connection.PrReadinessCheckedAt.Valid {
			t.Fatalf("pr_readiness_checked_at=%v, want null", connection.PrReadinessCheckedAt)
		}
	} else if !connection.PrReadinessCheckedAt.Valid || !connection.PrReadinessCheckedAt.Time.Equal(*wantCheckedAt) {
		t.Fatalf("pr_readiness_checked_at=%v, want %v", connection.PrReadinessCheckedAt, *wantCheckedAt)
	}
	if wantDetail == nil {
		if connection.PrReadinessDetail != nil {
			t.Fatalf("pr_readiness_detail=%q, want null", *connection.PrReadinessDetail)
		}
	} else if connection.PrReadinessDetail == nil || *connection.PrReadinessDetail != *wantDetail {
		t.Fatalf("pr_readiness_detail=%v, want %q", connection.PrReadinessDetail, *wantDetail)
	}
}

func assertGitHubPRReadinessCredential(
	t *testing.T,
	credential PublisherCredential,
	wantProjectID uuid.UUID,
	wantConnectionID uuid.UUID,
	wantEncryptedValue string,
	wantRedactedValue string,
	wantRevoked bool,
) {
	t.Helper()
	if credential.ProjectID != wantProjectID || credential.ConnectionID != wantConnectionID {
		t.Fatalf(
			"credential tenant=(%s,%s), want (%s,%s)",
			credential.ProjectID,
			credential.ConnectionID,
			wantProjectID,
			wantConnectionID,
		)
	}
	if credential.Kind != "github_token" {
		t.Fatalf("credential kind=%q, want github_token", credential.Kind)
	}
	if credential.EncryptedValue != wantEncryptedValue || credential.RedactedValue != wantRedactedValue {
		t.Fatalf(
			"credential values=(%q,%q), want (%q,%q)",
			credential.EncryptedValue,
			credential.RedactedValue,
			wantEncryptedValue,
			wantRedactedValue,
		)
	}
	if credential.RevokedAt.Valid != wantRevoked {
		t.Fatalf("credential revoked_at=%v, want revoked=%v", credential.RevokedAt, wantRevoked)
	}
}

func assertGitHubPRReadinessTimestamp(
	t *testing.T,
	got pgtype.Timestamptz,
	want pgtype.Timestamptz,
	field string,
) {
	t.Helper()
	if !got.Valid || !want.Valid || !got.Time.Equal(want.Time) {
		t.Fatalf("%s=%v, want %v", field, got, want)
	}
}

func githubPRReadinessTimestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func githubPRReadinessFixedTime(offset int) time.Time {
	return time.Date(2001, time.January, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(offset) * time.Hour)
}
