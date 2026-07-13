//go:build integration

package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestCanonicalSiteFixPostgresCatalog is the real-PostgreSQL rehearsal hook.
// Task 9 runs it against the migrated production-shaped database; ordinary CI
// remains hermetic when CITELOOP_TEST_DATABASE_URL is absent.
func TestCanonicalSiteFixPostgresCatalog(t *testing.T) {
	dsn := os.Getenv("CITELOOP_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CITELOOP_TEST_DATABASE_URL is not configured")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, table := range []string{"site_fixes", "site_fix_verifications", "site_change_applications", "work_signature_registry", "work_conflict_buckets", "doctor_ai_on_demand_triggers"} {
		var exists bool
		if err := pool.QueryRow(context.Background(), `select to_regclass('public.' || $1) is not null`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("table %s unavailable: exists=%v err=%v", table, exists, err)
		}
	}
	var definition string
	if err := pool.QueryRow(context.Background(), `select pg_get_constraintdef(oid) from pg_constraint where conname='site_fix_verifications_retry_classification_check' or (conrelid='site_fix_verifications'::regclass and pg_get_constraintdef(oid) ilike '%retry_classification%') limit 1`).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"not_applicable", "retryable", "retry_exhausted", "terminal"} {
		if !strings.Contains(definition, value) {
			t.Fatalf("retry classification catalog constraint missing %q: %s", value, definition)
		}
	}
}

// TestCanonicalSiteFixPostgresTransitions exercises the generated queries
// against real PostgreSQL. It deliberately uses separate pool connections so
// the claim assertion proves database serialization rather than an in-memory
// fake. The random project is cascade-deleted after the rehearsal.
func TestCanonicalSiteFixPostgresTransitions(t *testing.T) {
	dsn := os.Getenv("CITELOOP_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CITELOOP_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	projectID, fixID, appID := insertCanonicalSiteFixFixture(t, ctx, pool, "applying", "executing", "ready_for_pr")
	target := insertReadyCanonicalSiteFixGitHubConnection(t, ctx, pool, projectID, "owner/repo", "main", "https://example.com")

	tokens := []uuid.UUID{uuid.New(), uuid.New()}
	type claimResult struct {
		token uuid.UUID
		app   SiteChangeApplication
		err   error
	}
	results := make(chan claimResult, len(tokens))
	start := make(chan struct{})
	for _, token := range tokens {
		go func(token uuid.UUID) {
			<-start
			app, err := New(pool).ClaimCanonicalSiteFixGitHubPR(ctx, ClaimCanonicalSiteFixGitHubPRParams{
				PrClaimToken: pgtype.UUID{Bytes: token, Valid: true}, LeaseTtlSeconds: 60,
				ProjectID: projectID, ApplicationID: appID, SiteFixID: pgtype.UUID{Bytes: fixID, Valid: true},
				PublisherConnectionID: target.connectionID, ExpectedConnectionUpdatedAt: target.updatedAt,
				ExpectedRepoFullName: target.repoFullName, ExpectedBaseBranch: target.baseBranch,
			})
			results <- claimResult{token: token, app: app, err: err}
		}(token)
	}
	close(start)
	var winner claimResult
	leaders := 0
	for range tokens {
		result := <-results
		if result.err == nil {
			leaders++
			winner = result
		} else if !errors.Is(result.err, pgx.ErrNoRows) {
			t.Fatalf("unexpected concurrent claim error: %v", result.err)
		}
	}
	if leaders != 1 || winner.app.Status != "creating_pr" {
		t.Fatalf("PR claim leaders=%d winner=%+v", leaders, winner)
	}
	loser := tokens[0]
	if loser == winner.token {
		loser = tokens[1]
	}
	repo, branch, source, baseCommit, baseFile := "owner/repo", "main", "app/page.tsx", "base-commit", "blob-1"
	baseHash, proposedHash := "source-aggregate", "result-aggregate"
	paths := json.RawMessage(`["app/page.tsx","app/layout.tsx"]`)
	patch := json.RawMessage(`{"repo":"owner/repo","base_branch":"main","base_commit_sha":"base-commit","files":[{"path":"app/page.tsx","base_sha":"blob-1"},{"path":"app/layout.tsx","base_sha":"blob-2"}],"source_aggregate_sha256":"source-aggregate","result_aggregate_sha256":"result-aggregate"}`)
	diff := json.RawMessage(`{"files":[{"path":"app/page.tsx","changes":[{"before":"old","after":"new"}]}]}`)
	criteria := json.RawMessage(`{"asset_type":"metadata_rewrite"}`)
	saveArgs := SaveCanonicalSiteFixPreparedPatchParams{
		PublisherConnectionID: pgtype.UUID{Bytes: target.connectionID, Valid: true}, RepoFullName: &repo, BaseBranch: &branch,
		BaseCommitSha: &baseCommit, SourceFilePath: &source, SourceFilePaths: paths, BaseFileSha: &baseFile,
		BaseContentHash: &baseHash, ProposedContentHash: &proposedHash, SourceMappingConfidence: "high",
		SourceMappingReason: "immutable blob selection", PatchSnapshot: patch, DiffSnapshot: diff, ResolutionCriteria: criteria,
		ProjectID: projectID, ApplicationID: appID, SiteFixID: pgtype.UUID{Bytes: fixID, Valid: true},
		PrClaimToken: pgtype.UUID{Bytes: loser, Valid: true}, WriterAuthorityFingerprint: winner.app.PrClaimAuthorityFingerprint,
		ExpectedConnectionUpdatedAt: target.updatedAt, ExpectedRepoFullName: target.repoFullName, ExpectedBaseBranch: target.baseBranch,
	}
	if _, err := New(pool).SaveCanonicalSiteFixPreparedPatch(ctx, saveArgs); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("losing claim saved prepared patch: %v", err)
	}
	wrongFingerprint := "canonical:false:wrong"
	saveArgs.PrClaimToken = pgtype.UUID{Bytes: winner.token, Valid: true}
	saveArgs.WriterAuthorityFingerprint = &wrongFingerprint
	if _, err := New(pool).SaveCanonicalSiteFixPreparedPatch(ctx, saveArgs); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("wrong authority fingerprint saved prepared patch: %v", err)
	}
	saveArgs.WriterAuthorityFingerprint = winner.app.PrClaimAuthorityFingerprint
	prepared, err := New(pool).SaveCanonicalSiteFixPreparedPatch(ctx, saveArgs)
	if err != nil || prepared.Status != "creating_pr" || prepared.BaseCommitSha == nil || *prepared.BaseCommitSha != baseCommit ||
		prepared.BaseContentHash == nil || *prepared.BaseContentHash != baseHash || string(prepared.SourceFilePaths) != string(paths) {
		t.Fatalf("prepared patch app=%+v err=%v", prepared, err)
	}
	prNumber := int32(41)
	prURL, prState := "https://github.example/pr/41", "open"
	if _, err := New(pool).MarkCanonicalSiteFixGitHubPR(ctx, MarkCanonicalSiteFixGitHubPRParams{
		PublisherConnectionID: target.connectionID,
		ProjectID:             projectID, ApplicationID: appID, SiteFixID: pgtype.UUID{Bytes: fixID, Valid: true},
		PrClaimToken: pgtype.UUID{Bytes: loser, Valid: true}, GithubPrNumber: &prNumber, GithubPrUrl: &prURL,
		GithubPrState: prState, RepoFullName: &repo, BaseBranch: &branch, WorkingBranch: &branch, SourceFilePath: &source,
		ExpectedConnectionUpdatedAt: target.updatedAt, ExpectedRepoFullName: target.repoFullName, ExpectedBaseBranch: target.baseBranch,
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("losing claim marked external effect: %v", err)
	}
	app, err := New(pool).MarkCanonicalSiteFixGitHubPR(ctx, MarkCanonicalSiteFixGitHubPRParams{
		PublisherConnectionID: target.connectionID,
		ProjectID:             projectID, ApplicationID: appID, SiteFixID: pgtype.UUID{Bytes: fixID, Valid: true},
		PrClaimToken: pgtype.UUID{Bytes: winner.token, Valid: true}, GithubPrNumber: &prNumber, GithubPrUrl: &prURL,
		GithubPrState: prState, RepoFullName: &repo, BaseBranch: &branch, WorkingBranch: &branch, SourceFilePath: &source,
		ExpectedConnectionUpdatedAt: target.updatedAt, ExpectedRepoFullName: target.repoFullName, ExpectedBaseBranch: target.baseBranch,
	})
	if err != nil || app.Status != "github_pr_open" || app.PrClaimToken.Valid {
		t.Fatalf("winning claim mark app=%+v err=%v", app, err)
	}

	reason := "integration user termination"
	fix, err := New(pool).TerminalizeCanonicalSiteFix(ctx, TerminalizeCanonicalSiteFixParams{
		SiteFixID: fixID, ProjectID: projectID, ApplicationID: appID,
		VerificationSnapshot: []byte(`{"source":"integration_user_termination"}`), FailureReason: &reason, ForceTerminal: true,
	})
	if err != nil || fix.Status != "failed_terminal" {
		t.Fatalf("terminalize fix=%+v err=%v", fix, err)
	}
	var signatureStatus string
	var signatureActive bool
	if err := pool.QueryRow(ctx, `select w.status,w.active from work_signature_registry w join site_fixes sf on sf.work_signature_id=w.id and sf.project_id=w.project_id where sf.project_id=$1 and sf.id=$2`, projectID, fixID).Scan(&signatureStatus, &signatureActive); err != nil {
		t.Fatal(err)
	}
	if signatureStatus != "failed_terminal" || signatureActive {
		t.Fatalf("signature status=%s active=%v", signatureStatus, signatureActive)
	}

	t.Run("preparation failure remains safely retryable", func(t *testing.T) {
		pid, fid, _ := insertCanonicalSiteFixFixture(t, ctx, pool, "preparing", "preparing", "")
		rawFailure := "credential lookup failed: secret token"
		if _, err := New(pool).RecordCanonicalSiteFixPreparationFailure(ctx, RecordCanonicalSiteFixPreparationFailureParams{
			FailureCode: &rawFailure,
			ProjectID:   pid,
			SiteFixID:   fid,
		}); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("uncontrolled preparation failure was persisted: %v", err)
		}
		failureCode := "provider_unavailable"
		fix, err := New(pool).RecordCanonicalSiteFixPreparationFailure(ctx, RecordCanonicalSiteFixPreparationFailureParams{
			FailureCode: &failureCode,
			ProjectID:   pid,
			SiteFixID:   fid,
		})
		if err != nil || fix.Status != "preparing" || fix.FailureReason == nil || *fix.FailureReason != failureCode {
			t.Fatalf("preparation failure fix=%+v err=%v", fix, err)
		}
		var signatureState string
		var signatureStillActive bool
		if err := pool.QueryRow(ctx, `select status,active from work_signature_registry where project_id=$1 and reserved_work_id=$2`, pid, fid).Scan(&signatureState, &signatureStillActive); err != nil {
			t.Fatal(err)
		}
		if signatureState != "preparing" || !signatureStillActive {
			t.Fatalf("signature status=%s active=%v", signatureState, signatureStillActive)
		}
		ready, err := New(pool).MarkCanonicalSiteFixReadyToApply(ctx, MarkCanonicalSiteFixReadyToApplyParams{SiteFixID: fid, ProjectID: pid})
		if err != nil || ready.Status != "ready_to_apply" || ready.FailureReason != nil {
			t.Fatalf("successful preparation retry fix=%+v err=%v", ready, err)
		}
		if err := pool.QueryRow(ctx, `select status,active from work_signature_registry where project_id=$1 and reserved_work_id=$2`, pid, fid).Scan(&signatureState, &signatureStillActive); err != nil {
			t.Fatal(err)
		}
		if signatureState != "executing" || !signatureStillActive {
			t.Fatalf("successful retry signature status=%s active=%v", signatureState, signatureStillActive)
		}
	})

	t.Run("source conflict reset is claim fenced and re-enters preparation", func(t *testing.T) {
		pid, fid, aid := insertCanonicalSiteFixFixture(t, ctx, pool, "applying", "executing", "ready_for_pr")
		target := insertReadyCanonicalSiteFixGitHubConnection(t, ctx, pool, pid, "owner/repo", "main", "https://example.com")
		claimToken := uuid.New()
		claimed, err := New(pool).ClaimCanonicalSiteFixGitHubPR(ctx, ClaimCanonicalSiteFixGitHubPRParams{
			PrClaimToken: pgtype.UUID{Bytes: claimToken, Valid: true}, LeaseTtlSeconds: 60,
			ProjectID: pid, ApplicationID: aid, SiteFixID: pgtype.UUID{Bytes: fid, Valid: true},
			PublisherConnectionID: target.connectionID, ExpectedConnectionUpdatedAt: target.updatedAt,
			ExpectedRepoFullName: target.repoFullName, ExpectedBaseBranch: target.baseBranch,
		})
		if err != nil || claimed.Status != "creating_pr" {
			t.Fatalf("claim app=%+v err=%v", claimed, err)
		}
		if _, err := New(pool).ResetCanonicalSiteFixSourceConflictForReprepare(ctx, ResetCanonicalSiteFixSourceConflictForReprepareParams{
			ProjectID: pid, ApplicationID: aid, SiteFixID: pgtype.UUID{Bytes: fid, Valid: true},
			PrClaimToken: pgtype.UUID{Bytes: uuid.New(), Valid: true}, ReprepareReason: "repository_source_conflict",
		}); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("lost claim reset source conflict: %v", err)
		}
		failed, err := New(pool).ResetCanonicalSiteFixSourceConflictForReprepare(ctx, ResetCanonicalSiteFixSourceConflictForReprepareParams{
			ProjectID: pid, ApplicationID: aid, SiteFixID: pgtype.UUID{Bytes: fid, Valid: true},
			PrClaimToken: pgtype.UUID{Bytes: claimToken, Valid: true}, ReprepareReason: "repository_source_conflict",
		})
		if err != nil || failed.Status != "failed" || failed.FailureReason == nil || *failed.FailureReason != "repository_source_conflict" || failed.PrClaimToken.Valid {
			t.Fatalf("reset application=%+v err=%v", failed, err)
		}
		var fixState, signatureState string
		var signatureActive bool
		if err := pool.QueryRow(ctx, `select sf.status,w.status,w.active from site_fixes sf join work_signature_registry w on w.id=sf.work_signature_id and w.project_id=sf.project_id where sf.project_id=$1 and sf.id=$2`, pid, fid).Scan(&fixState, &signatureState, &signatureActive); err != nil {
			t.Fatal(err)
		}
		if fixState != "preparing" || signatureState != "preparing" || !signatureActive {
			t.Fatalf("reset fix=%s signature=%s active=%v", fixState, signatureState, signatureActive)
		}

		t.Run("repository target change shares the reprepare budget", func(t *testing.T) {
			secondAppID := uuid.New()
			if _, err := pool.Exec(ctx, `update site_fixes set status='applying',failure_reason=null,updated_at=now() where project_id=$1 and id=$2`, pid, fid); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `update work_signature_registry set status='executing',active=true,updated_at=now() where project_id=$1 and reserved_work_id=$2`, pid, fid); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `insert into site_change_applications(id,project_id,site_fix_id,application_kind,target_url,normalized_target_url,opportunity_key,status) values($1,$2,$3,'site_fix','https://example.com/','https://example.com/',$4,'ready_for_pr')`, secondAppID, pid, fid, "doctor:"+fid.String()+":repository-target-changed"); err != nil {
				t.Fatal(err)
			}
			secondToken := uuid.New()
			claimed, err := New(pool).ClaimCanonicalSiteFixGitHubPR(ctx, ClaimCanonicalSiteFixGitHubPRParams{
				PrClaimToken: pgtype.UUID{Bytes: secondToken, Valid: true}, LeaseTtlSeconds: 60,
				ProjectID: pid, ApplicationID: secondAppID, SiteFixID: pgtype.UUID{Bytes: fid, Valid: true},
				PublisherConnectionID: target.connectionID, ExpectedConnectionUpdatedAt: target.updatedAt,
				ExpectedRepoFullName: target.repoFullName, ExpectedBaseBranch: target.baseBranch,
			})
			if err != nil || claimed.Status != "creating_pr" {
				t.Fatalf("second claim app=%+v err=%v", claimed, err)
			}
			if _, err := New(pool).ResetCanonicalSiteFixSourceConflictForReprepare(ctx, ResetCanonicalSiteFixSourceConflictForReprepareParams{
				ProjectID: pid, ApplicationID: secondAppID, SiteFixID: pgtype.UUID{Bytes: fid, Valid: true},
				PrClaimToken: pgtype.UUID{Bytes: secondToken, Valid: true}, ReprepareReason: "repository_target_changed",
			}); !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("repository target change received a second reprepare reset: %v", err)
			}
			var appState string
			if err := pool.QueryRow(ctx, `select app.status,sf.status,w.status from site_change_applications app join site_fixes sf on sf.id=app.site_fix_id and sf.project_id=app.project_id join work_signature_registry w on w.id=sf.work_signature_id and w.project_id=sf.project_id where app.project_id=$1 and app.id=$2`, pid, secondAppID).Scan(&appState, &fixState, &signatureState); err != nil {
				t.Fatal(err)
			}
			if appState != "creating_pr" || fixState != "applying" || signatureState != "executing" {
				t.Fatalf("second reset mutated app=%s fix=%s signature=%s", appState, fixState, signatureState)
			}
		})
	})

	t.Run("source conflict reset rejects lost authority", func(t *testing.T) {
		pid, fid, aid := insertCanonicalSiteFixFixture(t, ctx, pool, "applying", "executing", "ready_for_pr")
		target := insertReadyCanonicalSiteFixGitHubConnection(t, ctx, pool, pid, "owner/repo", "main", "https://example.com")
		claimToken := uuid.New()
		if _, err := New(pool).ClaimCanonicalSiteFixGitHubPR(ctx, ClaimCanonicalSiteFixGitHubPRParams{
			PrClaimToken: pgtype.UUID{Bytes: claimToken, Valid: true}, LeaseTtlSeconds: 60,
			ProjectID: pid, ApplicationID: aid, SiteFixID: pgtype.UUID{Bytes: fid, Valid: true},
			PublisherConnectionID: target.connectionID, ExpectedConnectionUpdatedAt: target.updatedAt,
			ExpectedRepoFullName: target.repoFullName, ExpectedBaseBranch: target.baseBranch,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `update product_writer_authority set authority_changed_at=authority_changed_at+interval '1 second',updated_at=now() where project_id=$1 and product='doctor'`, pid); err != nil {
			t.Fatal(err)
		}
		if _, err := New(pool).ResetCanonicalSiteFixSourceConflictForReprepare(ctx, ResetCanonicalSiteFixSourceConflictForReprepareParams{
			ProjectID: pid, ApplicationID: aid, SiteFixID: pgtype.UUID{Bytes: fid, Valid: true},
			PrClaimToken: pgtype.UUID{Bytes: claimToken, Valid: true}, ReprepareReason: "repository_source_conflict",
		}); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("lost authority reset source conflict: %v", err)
		}
		var appState, fixState, signatureState string
		if err := pool.QueryRow(ctx, `select app.status,sf.status,w.status from site_change_applications app join site_fixes sf on sf.id=app.site_fix_id and sf.project_id=app.project_id join work_signature_registry w on w.id=sf.work_signature_id and w.project_id=sf.project_id where app.project_id=$1 and app.id=$2`, pid, aid).Scan(&appState, &fixState, &signatureState); err != nil {
			t.Fatal(err)
		}
		if appState != "creating_pr" || fixState != "applying" || signatureState != "executing" {
			t.Fatalf("lost authority mutated app=%s fix=%s signature=%s", appState, fixState, signatureState)
		}
	})

	t.Run("apply failure reopens without verification retry", func(t *testing.T) {
		pid, fid, aid := insertCanonicalSiteFixFixture(t, ctx, pool, "applying", "executing", "github_pr_open")
		reason := "closed PR"
		app, err := New(pool).MarkCanonicalSiteFixApplyFailure(ctx, MarkCanonicalSiteFixApplyFailureParams{ProjectID: pid, SiteFixID: pgtype.UUID{Bytes: fid, Valid: true}, ApplicationID: aid, FailureReason: &reason})
		if err != nil || app.Status != "needs_follow_up" {
			t.Fatalf("apply failure app=%+v err=%v", app, err)
		}
		var fixState, signatureState string
		if err := pool.QueryRow(ctx, `select sf.status,w.status from site_fixes sf join work_signature_registry w on w.id=sf.work_signature_id and w.project_id=sf.project_id where sf.project_id=$1 and sf.id=$2`, pid, fid).Scan(&fixState, &signatureState); err != nil {
			t.Fatal(err)
		}
		if fixState != "applying" || signatureState != "executing" {
			t.Fatalf("fix=%s signature=%s", fixState, signatureState)
		}
		app, err = New(pool).ReopenCanonicalSiteFixApply(ctx, ReopenCanonicalSiteFixApplyParams{ProjectID: pid, ApplicationID: aid, SiteFixID: pgtype.UUID{Bytes: fid, Valid: true}})
		if err != nil || app.Status != "ready_for_pr" {
			t.Fatalf("reopen app=%+v err=%v", app, err)
		}
	})

	t.Run("awaiting deployment failure uses legal verification enum", func(t *testing.T) {
		pid, fid, aid := insertCanonicalSiteFixFixture(t, ctx, pool, "awaiting_deploy", "awaiting_deploy", "deployment_pending")
		reason := "deployment not observed"
		if _, err := New(pool).AppendCanonicalSiteFixVerification(ctx, AppendCanonicalSiteFixVerificationParams{ID: uuid.New(), ProjectID: pid, SiteFixID: fid, AttemptNumber: 1, EvidenceRead: []byte(`{}`), AcceptanceResults: []byte(`[]`), Result: "failed", RetryClassification: "retryable", FailureReason: &reason, AttemptedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}}); err != nil {
			t.Fatal(err)
		}
		fix, err := New(pool).MarkCanonicalSiteFixRetryable(ctx, MarkCanonicalSiteFixRetryableParams{SiteFixID: fid, ProjectID: pid, ApplicationID: aid, VerificationSnapshot: []byte(`{"result":"failed"}`), FailureReason: &reason})
		if err != nil || fix.Status != "failed_retryable" {
			t.Fatalf("retry fix=%+v err=%v", fix, err)
		}
	})

	t.Run("manual application enters awaiting deploy", func(t *testing.T) {
		pid, fid, aid := insertCanonicalSiteFixFixture(t, ctx, pool, "applying", "executing", "manual_apply_required")
		if _, err := New(pool).MarkCanonicalSiteFixManualApplied(ctx, MarkCanonicalSiteFixManualAppliedParams{ProjectID: pid, SiteFixID: fid, ApplicationID: aid, DeploymentSnapshot: []byte(`{"source":"manual"}`), ManualAppliedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("merged application waits for production evidence before applied timestamps", func(t *testing.T) {
		pid, fid, aid := insertCanonicalSiteFixFixture(t, ctx, pool, "applying", "executing", "github_pr_open")
		observedMergedAt := pgtype.Timestamptz{Time: time.Now(), Valid: true}
		merged, err := New(pool).MarkCanonicalSiteFixPRMerged(ctx, MarkCanonicalSiteFixPRMergedParams{
			ProjectID: pid, SiteFixID: fid, ApplicationID: aid, ObservedMergedAt: observedMergedAt,
		})
		if err != nil || merged.Status != "awaiting_deploy" {
			t.Fatalf("merged fix=%+v err=%v", merged, err)
		}
		if merged.AppliedAt.Valid || merged.DeployedAt.Valid {
			t.Fatalf("merge prematurely recorded applied=%v deployed=%v", merged.AppliedAt, merged.DeployedAt)
		}
		var applicationStatus, signatureStatus string
		var applicationMergedAt pgtype.Timestamptz
		if err := pool.QueryRow(ctx, `select app.status,app.merged_at,w.status from site_change_applications app join site_fixes sf on sf.id=app.site_fix_id and sf.project_id=app.project_id join work_signature_registry w on w.id=sf.work_signature_id and w.project_id=sf.project_id where app.project_id=$1 and app.id=$2`, pid, aid).Scan(&applicationStatus, &applicationMergedAt, &signatureStatus); err != nil {
			t.Fatal(err)
		}
		if applicationStatus != "deployment_pending" || !applicationMergedAt.Valid || signatureStatus != "awaiting_deploy" {
			t.Fatalf("merged application=%s mergedAt=%v signature=%s", applicationStatus, applicationMergedAt, signatureStatus)
		}

		deployedAt := pgtype.Timestamptz{Time: time.Now().Add(time.Second), Valid: true}
		verifying, err := New(pool).MarkCanonicalSiteFixVerifying(ctx, MarkCanonicalSiteFixVerifyingParams{
			SiteFixID: fid, ProjectID: pid, ApplicationID: aid,
			DeploymentSnapshot: json.RawMessage(`{"source":"production_probe","result":"observed"}`), DeployedAt: deployedAt,
		})
		if err != nil || verifying.Status != "verifying" {
			t.Fatalf("verifying fix=%+v err=%v", verifying, err)
		}
		if !verifying.AppliedAt.Valid || !verifying.DeployedAt.Valid {
			t.Fatalf("production evidence did not record applied=%v deployed=%v", verifying.AppliedAt, verifying.DeployedAt)
		}
		if err := pool.QueryRow(ctx, `select app.status,w.status from site_change_applications app join site_fixes sf on sf.id=app.site_fix_id and sf.project_id=app.project_id join work_signature_registry w on w.id=sf.work_signature_id and w.project_id=sf.project_id where app.project_id=$1 and app.id=$2`, pid, aid).Scan(&applicationStatus, &signatureStatus); err != nil {
			t.Fatal(err)
		}
		if applicationStatus != "verification_pending" || signatureStatus != "verifying" {
			t.Fatalf("production application=%s signature=%s", applicationStatus, signatureStatus)
		}
	})

	t.Run("on demand processing reclaims linked call without replacement", func(t *testing.T) {
		pid, fid, _ := insertCanonicalSiteFixFixture(t, ctx, pool, "awaiting_deploy", "awaiting_deploy", "deployment_pending")
		if _, err := pool.Exec(ctx, `update projects set config='{"doctor_ai_enabled":true,"doctor_ai_run_policy":"on_demand"}' where id=$1`, pid); err != nil {
			t.Fatal(err)
		}
		requestID := uuid.New()
		if _, err := New(pool).ClaimDoctorAIOnDemandTrigger(ctx, ClaimDoctorAIOnDemandTriggerParams{ProjectID: pid, SiteFixID: fid, RequestID: requestID}); err != nil {
			t.Fatal(err)
		}
		firstToken := uuid.New()
		marker, err := New(pool).ClaimDoctorAIOnDemandProcessing(ctx, ClaimDoctorAIOnDemandProcessingParams{ProcessingToken: pgtype.UUID{Bytes: firstToken, Valid: true}, LeaseTtlSeconds: 60, ProjectID: pid, SiteFixID: fid})
		if err != nil {
			t.Fatal(err)
		}
		marker, err = New(pool).StartDoctorAIOnDemandCall(ctx, StartDoctorAIOnDemandCallParams{ProjectID: pid, SiteFixID: fid, RequestID: marker.RequestID, ProcessingToken: pgtype.UUID{Bytes: firstToken, Valid: true}, Provider: "fixture", Model: "fixture", PromptVersion: "v1", RequestFingerprint: "fixture"})
		if err != nil {
			t.Fatal(err)
		}
		originalCall := marker.AiCallID
		if _, err := pool.Exec(ctx, `update ai_call_records set status='ok',prompt_tokens=7,completion_tokens=3,total_tokens=10,cost_usd=1.25,finished_at=now() where project_id=$1 and id=$2`, pid, originalCall); err != nil {
			t.Fatal(err)
		}
		errorCode := "processing_reclaimed"
		if _, err := New(pool).FinishAICallRecordIfRunning(ctx, FinishAICallRecordIfRunningParams{ErrorCode: &errorCode, ID: uuid.UUID(originalCall.Bytes), ProjectID: pid}); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("finished ledger was overwritten: %v", err)
		}
		ledger, err := New(pool).GetAICallRecord(ctx, GetAICallRecordParams{ID: uuid.UUID(originalCall.Bytes), ProjectID: pid})
		if err != nil || ledger.Status != "ok" || ledger.TotalTokens != 10 {
			t.Fatalf("ledger=%+v err=%v", ledger, err)
		}
		if _, err := pool.Exec(ctx, `update doctor_ai_on_demand_triggers set processing_expires_at=clock_timestamp()-interval '1 second' where request_id=$1`, requestID); err != nil {
			t.Fatal(err)
		}
		secondToken := uuid.New()
		marker, err = New(pool).ClaimDoctorAIOnDemandProcessing(ctx, ClaimDoctorAIOnDemandProcessingParams{ProcessingToken: pgtype.UUID{Bytes: secondToken, Valid: true}, LeaseTtlSeconds: 60, ProjectID: pid, SiteFixID: fid})
		if err != nil || marker.AiCallID != originalCall {
			t.Fatalf("reclaimed marker=%+v err=%v", marker, err)
		}
		consumed, err := New(pool).ConsumeDoctorAIOnDemandProcessing(ctx, ConsumeDoctorAIOnDemandProcessingParams{ResultSnapshot: []byte(`{"decision":"inconclusive"}`), ProjectID: pid, SiteFixID: fid, RequestID: requestID, ProcessingToken: pgtype.UUID{Bytes: secondToken, Valid: true}, AiCallID: originalCall})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := New(pool).MarkDoctorAIOnDemandLifecycleApplied(ctx, MarkDoctorAIOnDemandLifecycleAppliedParams{ProjectID: pid, SiteFixID: fid, RequestID: requestID, AiCallID: consumed.AiCallID}); err != nil {
			t.Fatal(err)
		}
		if _, err := New(pool).ClaimDoctorAIOnDemandTrigger(ctx, ClaimDoctorAIOnDemandTriggerParams{ProjectID: pid, SiteFixID: fid, RequestID: requestID}); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("applied request id was reusable: %v", err)
		}
		newRequest := uuid.New()
		if _, err := New(pool).ClaimDoctorAIOnDemandTrigger(ctx, ClaimDoctorAIOnDemandTriggerParams{ProjectID: pid, SiteFixID: fid, RequestID: newRequest}); err != nil {
			t.Fatal(err)
		}
		reason := "deterministic evidence sufficient"
		rows, err := New(pool).RejectDoctorAIOnDemandTriggersForSiteFix(ctx, RejectDoctorAIOnDemandTriggersForSiteFixParams{ResultSnapshot: []byte(`{"decision":"rejected"}`), RejectionReason: &reason, ProjectID: pid, SiteFixID: fid})
		if err != nil || len(rows) != 1 || rows[0].Status != "rejected" {
			t.Fatalf("rejected=%+v err=%v", rows, err)
		}
	})

	t.Run("user termination works before application exists", func(t *testing.T) {
		pid, fid, _ := insertCanonicalSiteFixFixture(t, ctx, pool, "proposed", "proposed", "")
		reason := "user terminated"
		row, err := New(pool).TerminateCanonicalSiteFixByUser(ctx, TerminateCanonicalSiteFixByUserParams{ProjectID: pid, SiteFixID: fid, FailureReason: &reason, VerificationSnapshot: []byte(`{"source":"user"}`)})
		if err != nil || row.Status != "failed_terminal" {
			t.Fatalf("terminal row=%+v err=%v", row, err)
		}
	})

	t.Run("verification and marker application commit atomically", func(t *testing.T) {
		pid, fid, aid := insertCanonicalSiteFixFixture(t, ctx, pool, "verifying", "verifying", "verification_pending")
		call, err := New(pool).CreateAICallRecord(ctx, CreateAICallRecordParams{ProjectID: pid, Stage: "verification", LinkedObjectType: "site_fix", LinkedObjectID: fid, Provider: "fixture", Model: "fixture", PromptVersion: "v1", RequestFingerprint: "atomic", Status: "ok"})
		if err != nil {
			t.Fatal(err)
		}
		requestID := uuid.New()
		if _, err := pool.Exec(ctx, `insert into doctor_ai_on_demand_triggers(request_id,project_id,site_fix_id,requested_policy,status,ai_call_id,result_snapshot,consumed_at) values($1,$2,$3,'on_demand','consumed',$4,'{"decision":"passed"}',now())`, requestID, pid, fid, call.ID); err != nil {
			t.Fatal(err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		tq := New(tx)
		if _, err := tq.MarkCanonicalSiteFixVerified(ctx, MarkCanonicalSiteFixVerifiedParams{SiteFixID: fid, ProjectID: pid, ApplicationID: aid, DeploymentSnapshot: []byte(`{}`), VerificationSnapshot: []byte(`{"result":"passed"}`), VerifiedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}}); err != nil {
			t.Fatal(err)
		}
		wrongCall := pgtype.UUID{Bytes: uuid.New(), Valid: true}
		if _, err := tq.MarkDoctorAIOnDemandLifecycleApplied(ctx, MarkDoctorAIOnDemandLifecycleAppliedParams{ProjectID: pid, SiteFixID: fid, RequestID: requestID, AiCallID: wrongCall}); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("wrong marker CAS=%v", err)
		}
		_ = tx.Rollback(ctx)
		var fixState string
		if err := pool.QueryRow(ctx, `select status from site_fixes where project_id=$1 and id=$2`, pid, fid).Scan(&fixState); err != nil || fixState != "verifying" {
			t.Fatalf("rollback fix=%s err=%v", fixState, err)
		}
		tx, err = pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		tq = New(tx)
		if _, err := tq.MarkCanonicalSiteFixVerified(ctx, MarkCanonicalSiteFixVerifiedParams{SiteFixID: fid, ProjectID: pid, ApplicationID: aid, DeploymentSnapshot: []byte(`{}`), VerificationSnapshot: []byte(`{"result":"passed"}`), VerifiedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}}); err != nil {
			t.Fatal(err)
		}
		if _, err := tq.MarkDoctorAIOnDemandLifecycleApplied(ctx, MarkDoctorAIOnDemandLifecycleAppliedParams{ProjectID: pid, SiteFixID: fid, RequestID: requestID, AiCallID: pgtype.UUID{Bytes: call.ID, Valid: true}}); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		var applied bool
		if err := pool.QueryRow(ctx, `select status='verified' from site_fixes where project_id=$1 and id=$2`, pid, fid).Scan(&applied); err != nil || !applied {
			t.Fatalf("atomic commit applied=%v err=%v", applied, err)
		}
	})

	t.Run("policy off rejects pending and consumed markers", func(t *testing.T) {
		pid, fid, _ := insertCanonicalSiteFixFixture(t, ctx, pool, "awaiting_deploy", "awaiting_deploy", "deployment_pending")
		if _, err := pool.Exec(ctx, `update projects set config='{"doctor_ai_enabled":true,"doctor_ai_run_policy":"on_demand"}' where id=$1`, pid); err != nil {
			t.Fatal(err)
		}
		requestID := uuid.New()
		if _, err := New(pool).ClaimDoctorAIOnDemandTrigger(ctx, ClaimDoctorAIOnDemandTriggerParams{ProjectID: pid, SiteFixID: fid, RequestID: requestID}); err != nil {
			t.Fatal(err)
		}
		consumedCall, err := New(pool).CreateAICallRecord(ctx, CreateAICallRecordParams{ProjectID: pid, Stage: "verification", LinkedObjectType: "site_fix", LinkedObjectID: fid, Provider: "fixture", Model: "fixture", PromptVersion: "v1", RequestFingerprint: "policy-off-consumed", Status: "ok"})
		if err != nil {
			t.Fatal(err)
		}
		consumedRequest := uuid.New()
		if _, err := pool.Exec(ctx, `insert into doctor_ai_on_demand_triggers(request_id,project_id,site_fix_id,requested_policy,status,ai_call_id,result_snapshot,consumed_at) values($1,$2,$3,'on_demand','consumed',$4,'{"decision":"passed"}',now())`, consumedRequest, pid, fid, consumedCall.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `update projects set config='{"doctor_ai_enabled":false,"doctor_ai_run_policy":"on_demand"}' where id=$1`, pid); err != nil {
			t.Fatal(err)
		}
		rows, err := New(pool).RejectUnauthorizedDoctorAIOnDemandTriggers(ctx, RejectUnauthorizedDoctorAIOnDemandTriggersParams{ProjectID: pid, SiteFixID: fid})
		if err != nil || len(rows) != 2 || rows[0].Status != "rejected" || rows[1].Status != "rejected" {
			t.Fatalf("policy-off rejected=%+v err=%v", rows, err)
		}
		var preserved string
		if err := pool.QueryRow(ctx, `select result_snapshot->>'decision' from doctor_ai_on_demand_triggers where request_id=$1`, consumedRequest).Scan(&preserved); err != nil || preserved != "passed" {
			t.Fatalf("consumed result was not preserved: %q err=%v", preserved, err)
		}
	})

	t.Run("terminal processing marker finishes running call", func(t *testing.T) {
		pid, fid, aid := insertCanonicalSiteFixFixture(t, ctx, pool, "awaiting_deploy", "awaiting_deploy", "deployment_pending")
		if _, err := pool.Exec(ctx, `update projects set config='{"doctor_ai_enabled":true,"doctor_ai_run_policy":"on_demand"}' where id=$1`, pid); err != nil {
			t.Fatal(err)
		}
		requestID, token := uuid.New(), uuid.New()
		if _, err := New(pool).ClaimDoctorAIOnDemandTrigger(ctx, ClaimDoctorAIOnDemandTriggerParams{ProjectID: pid, SiteFixID: fid, RequestID: requestID}); err != nil {
			t.Fatal(err)
		}
		marker, err := New(pool).ClaimDoctorAIOnDemandProcessing(ctx, ClaimDoctorAIOnDemandProcessingParams{ProcessingToken: pgtype.UUID{Bytes: token, Valid: true}, LeaseTtlSeconds: 60, ProjectID: pid, SiteFixID: fid})
		if err != nil {
			t.Fatal(err)
		}
		marker, err = New(pool).StartDoctorAIOnDemandCall(ctx, StartDoctorAIOnDemandCallParams{ProjectID: pid, SiteFixID: fid, RequestID: marker.RequestID, ProcessingToken: pgtype.UUID{Bytes: token, Valid: true}, Provider: "fixture-provider", Model: "fixture-model", PromptVersion: "v1", RequestFingerprint: "terminal-processing"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `update ai_call_records set prompt_tokens=4,completion_tokens=5,total_tokens=9,cost_usd=0.5 where project_id=$1 and id=$2`, pid, marker.AiCallID); err != nil {
			t.Fatal(err)
		}
		reason := "terminal integration"
		if _, err := New(pool).TerminalizeCanonicalSiteFix(ctx, TerminalizeCanonicalSiteFixParams{SiteFixID: fid, ProjectID: pid, ApplicationID: aid, VerificationSnapshot: []byte(`{"result":"failed"}`), FailureReason: &reason, ForceTerminal: true}); err != nil {
			t.Fatal(err)
		}
		var markerStatus, callStatus, provider, model string
		var totalTokens int32
		if err := pool.QueryRow(ctx, `select marker.status,call.status,call.provider,call.model,call.total_tokens from doctor_ai_on_demand_triggers marker join ai_call_records call on call.id=marker.ai_call_id where marker.request_id=$1`, requestID).Scan(&markerStatus, &callStatus, &provider, &model, &totalTokens); err != nil {
			t.Fatal(err)
		}
		if markerStatus != "rejected" || callStatus != "failed" || provider != "fixture-provider" || model != "fixture-model" || totalTokens != 9 {
			t.Fatalf("marker=%s call=%s provider=%s model=%s tokens=%d", markerStatus, callStatus, provider, model, totalTokens)
		}
		lateCost := pgtype.Numeric{}
		if err := lateCost.Scan("0.75"); err != nil {
			t.Fatal(err)
		}
		lateProvider, lateModel := "late-provider", "late-model"
		late, err := New(pool).FinishCanonicalAICallFenced(ctx, FinishCanonicalAICallFencedParams{Status: "ok", ResolvedProvider: &lateProvider, ResolvedModel: &lateModel, PromptTokens: 11, CompletionTokens: 12, TotalTokens: 23, CostUsd: lateCost, ID: uuid.UUID(marker.AiCallID.Bytes), ProjectID: pid})
		if err != nil || late.Status != "failed" || late.ErrorCode == nil || *late.ErrorCode != "doctor_ai_marker_rejected" || late.Provider != "late-provider" || late.Model != "late-model" || late.TotalTokens != 23 {
			t.Fatalf("late fenced finish=%+v err=%v", late, err)
		}
		if _, err := New(pool).ConsumeDoctorAIOnDemandProcessing(ctx, ConsumeDoctorAIOnDemandProcessingParams{ResultSnapshot: []byte(`{"decision":"passed"}`), ProjectID: pid, SiteFixID: fid, RequestID: requestID, ProcessingToken: pgtype.UUID{Bytes: token, Valid: true}, AiCallID: marker.AiCallID}); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("old worker consume was not fenced: %v", err)
		}
	})

	t.Run("applied marker supersedes active siblings", func(t *testing.T) {
		pid, fid, _ := insertCanonicalSiteFixFixture(t, ctx, pool, "verifying", "verifying", "verification_pending")
		selectedCall, err := New(pool).CreateAICallRecord(ctx, CreateAICallRecordParams{ProjectID: pid, Stage: "verification", LinkedObjectType: "site_fix", LinkedObjectID: fid, Provider: "fixture", Model: "fixture", PromptVersion: "v1", RequestFingerprint: "selected", Status: "ok"})
		if err != nil {
			t.Fatal(err)
		}
		selectedRequest := uuid.New()
		if _, err := pool.Exec(ctx, `insert into doctor_ai_on_demand_triggers(request_id,project_id,site_fix_id,requested_policy,status,ai_call_id,result_snapshot,consumed_at) values($1,$2,$3,'on_demand','consumed',$4,'{"decision":"passed"}',now())`, selectedRequest, pid, fid, selectedCall.ID); err != nil {
			t.Fatal(err)
		}
		siblingCall, err := New(pool).CreateAICallRecord(ctx, CreateAICallRecordParams{ProjectID: pid, Stage: "verification", LinkedObjectType: "site_fix", LinkedObjectID: fid, Provider: "fixture", Model: "fixture", PromptVersion: "v1", RequestFingerprint: "sibling", Status: "ok"})
		if err != nil {
			t.Fatal(err)
		}
		siblingRequest := uuid.New()
		if _, err := pool.Exec(ctx, `insert into doctor_ai_on_demand_triggers(request_id,project_id,site_fix_id,requested_policy,status,ai_call_id,result_snapshot,consumed_at) values($1,$2,$3,'on_demand','consumed',$4,'{"decision":"failed"}',now())`, siblingRequest, pid, fid, siblingCall.ID); err != nil {
			t.Fatal(err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		tq := New(tx)
		if _, err := tq.MarkDoctorAIOnDemandLifecycleApplied(ctx, MarkDoctorAIOnDemandLifecycleAppliedParams{ProjectID: pid, SiteFixID: fid, RequestID: selectedRequest, AiCallID: pgtype.UUID{Bytes: selectedCall.ID, Valid: true}}); err != nil {
			t.Fatal(err)
		}
		siblingReason := "another result applied"
		if _, err := tq.SupersedeDoctorAIOnDemandSiblingTriggers(ctx, SupersedeDoctorAIOnDemandSiblingTriggersParams{RejectionReason: &siblingReason, ProjectID: pid, SiteFixID: fid, AppliedRequestID: selectedRequest}); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		var selectedApplied bool
		var siblingStatus, siblingCallStatus string
		if err := pool.QueryRow(ctx, `select lifecycle_applied_at is not null from doctor_ai_on_demand_triggers where request_id=$1`, selectedRequest).Scan(&selectedApplied); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `select marker.status,call.status from doctor_ai_on_demand_triggers marker join ai_call_records call on call.id=marker.ai_call_id where marker.request_id=$1`, siblingRequest).Scan(&siblingStatus, &siblingCallStatus); err != nil {
			t.Fatal(err)
		}
		if !selectedApplied || siblingStatus != "superseded" || siblingCallStatus != "ok" {
			t.Fatalf("selectedApplied=%v sibling=%s call=%s", selectedApplied, siblingStatus, siblingCallStatus)
		}
		if _, err := New(pool).MarkDoctorAIOnDemandLifecycleApplied(ctx, MarkDoctorAIOnDemandLifecycleAppliedParams{ProjectID: pid, SiteFixID: fid, RequestID: siblingRequest, AiCallID: pgtype.UUID{Bytes: siblingCall.ID, Valid: true}}); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("losing consumed marker applied after winner: %v", err)
		}
	})

	t.Run("legacy consumed terminal requires exact verification reference", func(t *testing.T) {
		for _, referenced := range []bool{false, true} {
			t.Run(fmt.Sprintf("referenced_%v", referenced), func(t *testing.T) {
				pid, fid, aid := insertCanonicalSiteFixFixture(t, ctx, pool, "verifying", "verifying", "verification_pending")
				call, err := New(pool).CreateAICallRecord(ctx, CreateAICallRecordParams{ProjectID: pid, Stage: "verification", LinkedObjectType: "site_fix", LinkedObjectID: fid, Provider: "fixture", Model: "fixture", PromptVersion: "v1", RequestFingerprint: fmt.Sprintf("legacy-%v", referenced), Status: "ok"})
				if err != nil {
					t.Fatal(err)
				}
				requestID := uuid.New()
				if _, err := pool.Exec(ctx, `insert into doctor_ai_on_demand_triggers(request_id,project_id,site_fix_id,requested_policy,status,ai_call_id,result_snapshot,consumed_at) values($1,$2,$3,'on_demand','consumed',$4,'{"decision":"passed"}',now())`, requestID, pid, fid, call.ID); err != nil {
					t.Fatal(err)
				}
				if referenced {
					if _, err := New(pool).AppendCanonicalSiteFixVerification(ctx, AppendCanonicalSiteFixVerificationParams{ID: uuid.New(), ProjectID: pid, SiteFixID: fid, AttemptNumber: 1, EvidenceRead: []byte(`{}`), AcceptanceResults: []byte(`[]`), AiCallID: pgtype.UUID{Bytes: call.ID, Valid: true}, Result: "passed", RetryClassification: "not_applicable", AttemptedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}}); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := pool.Exec(ctx, `update site_change_applications set status='verified' where project_id=$1 and id=$2; update site_fixes set status='verified' where project_id=$1 and id=$3; update work_signature_registry w set status='verified',active=false from site_fixes sf where sf.project_id=$1 and sf.id=$3 and w.project_id=sf.project_id and w.id=sf.work_signature_id`, pid, aid, fid); err != nil {
					t.Fatal(err)
				}
				markers, err := New(pool).ListDoctorAIOnDemandConsumedUnapplied(ctx, pid)
				if err != nil || len(markers) != 1 || markers[0].HasLifecycleReference != referenced {
					t.Fatalf("markers=%+v err=%v", markers, err)
				}
				if referenced {
					if _, err := New(pool).MarkDoctorAIOnDemandLifecycleApplied(ctx, MarkDoctorAIOnDemandLifecycleAppliedParams{ProjectID: pid, SiteFixID: fid, RequestID: requestID, AiCallID: pgtype.UUID{Bytes: call.ID, Valid: true}}); err != nil {
						t.Fatal(err)
					}
				} else if _, err := New(pool).RejectDoctorAIOnDemandConsumedWithoutLifecycleReference(ctx, RejectDoctorAIOnDemandConsumedWithoutLifecycleReferenceParams{ProjectID: pid, SiteFixID: fid, RequestID: requestID, AiCallID: pgtype.UUID{Bytes: call.ID, Valid: true}}); err != nil {
					t.Fatal(err)
				}
				var status string
				var applied bool
				if err := pool.QueryRow(ctx, `select status,lifecycle_applied_at is not null from doctor_ai_on_demand_triggers where request_id=$1`, requestID).Scan(&status, &applied); err != nil {
					t.Fatal(err)
				}
				if !applied || (referenced && status != "consumed") || (!referenced && status != "rejected") {
					t.Fatalf("referenced=%v status=%s applied=%v", referenced, status, applied)
				}
			})
		}
	})
}

func TestCanonicalSiteFixPostgresGitHubReadinessFences(t *testing.T) {
	dsn := os.Getenv("CITELOOP_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CITELOOP_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	for _, testCase := range []struct {
		prState          string
		applicationState string
		fixState         string
		signatureState   string
		failureReason    string
	}{
		{prState: "open", applicationState: "github_pr_open", fixState: "applying", signatureState: "executing"},
		{prState: "closed", applicationState: "needs_follow_up", fixState: "applying", signatureState: "executing", failureReason: "pull_request_closed_without_merge"},
		{prState: "merged", applicationState: "deployment_pending", fixState: "awaiting_deploy", signatureState: "awaiting_deploy"},
	} {
		t.Run("initial_pr_"+testCase.prState+"_transitions_atomically", func(t *testing.T) {
			projectID, fixID, appID := insertCanonicalSiteFixFixture(t, ctx, pool, "applying", "executing", "ready_for_pr")
			target := insertReadyCanonicalSiteFixGitHubConnection(t, ctx, pool, projectID, "owner/repo", "main", "https://example.com")
			token := uuid.New()
			claimed, err := New(pool).ClaimCanonicalSiteFixGitHubPR(ctx, ClaimCanonicalSiteFixGitHubPRParams{
				PrClaimToken: pgtype.UUID{Bytes: token, Valid: true}, LeaseTtlSeconds: 60,
				ProjectID: projectID, ApplicationID: appID, SiteFixID: pgtype.UUID{Bytes: fixID, Valid: true},
				PublisherConnectionID: target.connectionID, ExpectedConnectionUpdatedAt: target.updatedAt,
				ExpectedRepoFullName: target.repoFullName, ExpectedBaseBranch: target.baseBranch,
			})
			if err != nil || claimed.Status != "creating_pr" {
				t.Fatalf("claim app=%+v err=%v", claimed, err)
			}

			repo, baseBranch, workingBranch := target.repoFullName, target.baseBranch, "citeloop/site-fix"
			prNumber := int32(43)
			prURL := "https://github.example/pr/43"
			application, err := New(pool).MarkCanonicalSiteFixGitHubPR(ctx, MarkCanonicalSiteFixGitHubPRParams{
				ProjectID: projectID, PublisherConnectionID: target.connectionID,
				ExpectedConnectionUpdatedAt: target.updatedAt, ExpectedRepoFullName: target.repoFullName, ExpectedBaseBranch: target.baseBranch,
				GithubPrState: testCase.prState, ApplicationID: appID, SiteFixID: pgtype.UUID{Bytes: fixID, Valid: true},
				PrClaimToken: pgtype.UUID{Bytes: token, Valid: true}, RepoFullName: &repo, BaseBranch: &baseBranch,
				WorkingBranch: &workingBranch, GithubPrNumber: &prNumber, GithubPrUrl: &prURL,
			})
			if err != nil || application.ID != appID || application.Status != testCase.applicationState {
				t.Fatalf("initial %s application=%+v err=%v", testCase.prState, application, err)
			}
			if application.GithubPrState == nil || *application.GithubPrState != testCase.prState || application.GithubPrUrl == nil || *application.GithubPrUrl != prURL {
				t.Fatalf("initial %s PR identity was not preserved: %+v", testCase.prState, application)
			}
			if testCase.failureReason == "" {
				if application.FailureReason != nil {
					t.Fatalf("initial %s failure=%q", testCase.prState, *application.FailureReason)
				}
			} else if application.FailureReason == nil || *application.FailureReason != testCase.failureReason {
				t.Fatalf("initial %s failure=%v want %q", testCase.prState, application.FailureReason, testCase.failureReason)
			}

			var fixState, signatureState string
			var appliedAt, deployedAt pgtype.Timestamptz
			if err := pool.QueryRow(ctx, `select sf.status,sf.applied_at,sf.deployed_at,w.status from site_fixes sf join work_signature_registry w on w.id=sf.work_signature_id and w.project_id=sf.project_id where sf.project_id=$1 and sf.id=$2`, projectID, fixID).Scan(&fixState, &appliedAt, &deployedAt, &signatureState); err != nil {
				t.Fatal(err)
			}
			if fixState != testCase.fixState || signatureState != testCase.signatureState {
				t.Fatalf("initial %s fix=%s signature=%s", testCase.prState, fixState, signatureState)
			}
			if appliedAt.Valid || deployedAt.Valid {
				t.Fatalf("initial %s prematurely recorded applied=%v deployed=%v", testCase.prState, appliedAt, deployedAt)
			}
		})
	}

	for _, operation := range []string{"claim", "renew", "save", "mark"} {
		for _, mutation := range []string{"updated_at", "config", "status", "readiness_status"} {
			t.Run(operation+"_rejects_"+mutation+"_mutation", func(t *testing.T) {
				projectID, fixID, appID := insertCanonicalSiteFixFixture(t, ctx, pool, "applying", "executing", "ready_for_pr")
				target := insertReadyCanonicalSiteFixGitHubConnection(t, ctx, pool, projectID, "owner/repo", "main", "https://example.com")
				token := uuid.New()
				var claimed SiteChangeApplication
				if operation != "claim" {
					claimed, err = New(pool).ClaimCanonicalSiteFixGitHubPR(ctx, ClaimCanonicalSiteFixGitHubPRParams{
						PrClaimToken: pgtype.UUID{Bytes: token, Valid: true}, LeaseTtlSeconds: 60,
						ProjectID: projectID, ApplicationID: appID, SiteFixID: pgtype.UUID{Bytes: fixID, Valid: true},
						PublisherConnectionID: target.connectionID, ExpectedConnectionUpdatedAt: target.updatedAt,
						ExpectedRepoFullName: target.repoFullName, ExpectedBaseBranch: target.baseBranch,
					})
					if err != nil || claimed.Status != "creating_pr" {
						t.Fatalf("prepare claim app=%+v err=%v", claimed, err)
					}
				}

				switch mutation {
				case "updated_at":
					_, err = pool.Exec(ctx, `update publisher_connections set updated_at=updated_at+interval '1 second' where project_id=$1 and id=$2`, projectID, target.connectionID)
				case "config":
					_, err = pool.Exec(ctx, `update publisher_connections set config=jsonb_set(config,'{repo}','"owner/changed"') where project_id=$1 and id=$2`, projectID, target.connectionID)
				case "status":
					_, err = pool.Exec(ctx, `update publisher_connections set status='error' where project_id=$1 and id=$2`, projectID, target.connectionID)
				case "readiness_status":
					_, err = pool.Exec(ctx, `update publisher_connections set pr_readiness_status='permission_missing' where project_id=$1 and id=$2`, projectID, target.connectionID)
				}
				if err != nil {
					t.Fatal(err)
				}

				switch operation {
				case "claim":
					_, err = New(pool).ClaimCanonicalSiteFixGitHubPR(ctx, ClaimCanonicalSiteFixGitHubPRParams{
						PrClaimToken: pgtype.UUID{Bytes: token, Valid: true}, LeaseTtlSeconds: 60,
						ProjectID: projectID, ApplicationID: appID, SiteFixID: pgtype.UUID{Bytes: fixID, Valid: true},
						PublisherConnectionID: target.connectionID, ExpectedConnectionUpdatedAt: target.updatedAt,
						ExpectedRepoFullName: target.repoFullName, ExpectedBaseBranch: target.baseBranch,
					})
				case "renew":
					_, err = New(pool).RenewCanonicalSiteFixGitHubPRClaim(ctx, RenewCanonicalSiteFixGitHubPRClaimParams{
						LeaseTtlSeconds: 60, ProjectID: projectID, ApplicationID: appID,
						SiteFixID: pgtype.UUID{Bytes: fixID, Valid: true}, PrClaimToken: pgtype.UUID{Bytes: token, Valid: true},
						PublisherConnectionID: target.connectionID, ExpectedConnectionUpdatedAt: target.updatedAt,
						ExpectedRepoFullName: target.repoFullName, ExpectedBaseBranch: target.baseBranch,
					})
				case "save":
					repo, branch, source := target.repoFullName, target.baseBranch, "app/page.tsx"
					baseCommit, baseFile := "base-commit", "blob-1"
					baseHash, proposedHash := "source-aggregate", "result-aggregate"
					_, err = New(pool).SaveCanonicalSiteFixPreparedPatch(ctx, SaveCanonicalSiteFixPreparedPatchParams{
						PublisherConnectionID: pgtype.UUID{Bytes: target.connectionID, Valid: true}, RepoFullName: &repo, BaseBranch: &branch,
						BaseCommitSha: &baseCommit, SourceFilePath: &source, SourceFilePaths: json.RawMessage(`["app/page.tsx"]`),
						BaseFileSha: &baseFile, BaseContentHash: &baseHash, ProposedContentHash: &proposedHash,
						SourceMappingConfidence: "high", SourceMappingReason: "immutable blob selection",
						PatchSnapshot: json.RawMessage(`{"files":[]}`), DiffSnapshot: json.RawMessage(`{"files":[]}`), ResolutionCriteria: json.RawMessage(`{}`),
						ProjectID: projectID, ApplicationID: appID, SiteFixID: pgtype.UUID{Bytes: fixID, Valid: true},
						PrClaimToken: pgtype.UUID{Bytes: token, Valid: true}, WriterAuthorityFingerprint: claimed.PrClaimAuthorityFingerprint,
						ExpectedConnectionUpdatedAt: target.updatedAt, ExpectedRepoFullName: target.repoFullName, ExpectedBaseBranch: target.baseBranch,
					})
				case "mark":
					prNumber := int32(42)
					prURL, prState := "https://github.example/pr/42", "open"
					repo, branch := target.repoFullName, target.baseBranch
					_, err = New(pool).MarkCanonicalSiteFixGitHubPR(ctx, MarkCanonicalSiteFixGitHubPRParams{
						PublisherConnectionID: target.connectionID, RepoFullName: &repo, BaseBranch: &branch,
						WorkingBranch: &branch, GithubPrNumber: &prNumber, GithubPrUrl: &prURL, GithubPrState: prState,
						ProjectID: projectID, ApplicationID: appID, SiteFixID: pgtype.UUID{Bytes: fixID, Valid: true},
						PrClaimToken: pgtype.UUID{Bytes: token, Valid: true}, ExpectedConnectionUpdatedAt: target.updatedAt,
						ExpectedRepoFullName: target.repoFullName, ExpectedBaseBranch: target.baseBranch,
					})
				}
				if !errors.Is(err, pgx.ErrNoRows) {
					t.Fatalf("%s accepted %s-mutated readiness target: %v", operation, mutation, err)
				}

				expectedStatus := "creating_pr"
				if operation == "claim" {
					expectedStatus = "ready_for_pr"
				}
				var applicationStatus string
				if err := pool.QueryRow(ctx, `select status from site_change_applications where project_id=$1 and id=$2`, projectID, appID).Scan(&applicationStatus); err != nil {
					t.Fatal(err)
				}
				if applicationStatus != expectedStatus {
					t.Fatalf("application status=%s want %s", applicationStatus, expectedStatus)
				}
			})
		}
	}

	for _, testCase := range []struct {
		name       string
		baseURL    string
		baseBranch string
	}{
		{name: "development", baseURL: "https://dev.unipost.dev", baseBranch: "dev"},
		{name: "staging", baseURL: "https://staging.unipost.dev", baseBranch: "staging"},
		{name: "production", baseURL: "https://unipost.dev", baseBranch: "main"},
		{name: "customer", baseURL: "https://customer.example.com", baseBranch: "citeloop-content"},
	} {
		t.Run("missing_branch_uses_"+testCase.name+"_fallback", func(t *testing.T) {
			projectID, fixID, appID := insertCanonicalSiteFixFixture(t, ctx, pool, "applying", "executing", "ready_for_pr")
			target := insertReadyCanonicalSiteFixGitHubConnection(t, ctx, pool, projectID, "owner/repo", "", testCase.baseURL)
			wrongBranch := "main"
			if testCase.baseBranch == wrongBranch {
				wrongBranch = "citeloop-content"
			}
			claim := ClaimCanonicalSiteFixGitHubPRParams{
				PrClaimToken: pgtype.UUID{Bytes: uuid.New(), Valid: true}, LeaseTtlSeconds: 60,
				ProjectID: projectID, ApplicationID: appID, SiteFixID: pgtype.UUID{Bytes: fixID, Valid: true},
				PublisherConnectionID: target.connectionID, ExpectedConnectionUpdatedAt: target.updatedAt,
				ExpectedRepoFullName: target.repoFullName, ExpectedBaseBranch: wrongBranch,
			}
			if _, err := New(pool).ClaimCanonicalSiteFixGitHubPR(ctx, claim); !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("missing branch accepted non-derived fallback %q: %v", wrongBranch, err)
			}
			claim.ExpectedBaseBranch = testCase.baseBranch
			claimed, err := New(pool).ClaimCanonicalSiteFixGitHubPR(ctx, claim)
			if err != nil || claimed.Status != "creating_pr" {
				t.Fatalf("derived fallback %q claim=%+v err=%v", testCase.baseBranch, claimed, err)
			}
		})
	}
}

type canonicalSiteFixGitHubReadinessTarget struct {
	connectionID uuid.UUID
	updatedAt    pgtype.Timestamptz
	repoFullName string
	baseBranch   string
}

func insertReadyCanonicalSiteFixGitHubConnection(t *testing.T, ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID, repoFullName, baseBranch, baseURL string) canonicalSiteFixGitHubReadinessTarget {
	t.Helper()
	config := map[string]string{
		"repo":     repoFullName,
		"base_url": baseURL,
	}
	if baseBranch != "" {
		config["branch"] = baseBranch
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	target := canonicalSiteFixGitHubReadinessTarget{
		connectionID: uuid.New(),
		repoFullName: repoFullName,
		baseBranch:   baseBranch,
	}
	if err := pool.QueryRow(ctx, `insert into publisher_connections
		(id,project_id,kind,label,status,is_default,enabled,capabilities,capability_schema_version,config,pr_readiness_status,pr_readiness_checked_at)
		values($1,$2,'github_nextjs','Site Fix PR integration','connected',true,true,'{}',1,$3,'ready',now())
		returning updated_at`, target.connectionID, projectID, configJSON).Scan(&target.updatedAt); err != nil {
		t.Fatal(err)
	}
	return target
}

func insertCanonicalSiteFixFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fixStatus, signatureStatus, appStatus string) (uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	projectID, runID, findingID := uuid.New(), uuid.New(), uuid.New()
	shadowID, candidateID, bucketID := uuid.New(), uuid.New(), uuid.New()
	signatureID, fixID, appID := uuid.New(), uuid.New(), uuid.New()
	suffix := strings.ReplaceAll(projectID.String(), "-", "")
	statements := []struct {
		sql  string
		args []any
	}{
		{`insert into projects(id,owner_id,name,slug,config) values($1,'integration','integration',$2,'{}')`, []any{projectID, "integration-" + suffix}},
		{`update product_writer_authority set writer_authority='canonical',authority_changed_at=now(),updated_at=now() where project_id=$1 and product='doctor'`, []any{projectID}},
		{`insert into seo_doctor_runs(id,project_id,trigger,status) values($1,$2,'manual','completed')`, []any{runID, projectID}},
		{`insert into seo_doctor_findings(id,project_id,run_id,finding_key,severity,category,issue_type,status,finding_kind) values($1,$2,$3,$4,'P1','technical','canonical','active','broken')`, []any{findingID, projectID, runID, "integration-" + suffix}},
		{`insert into discovery_shadow_runs(id,project_id,mode,status,candidate_schema_version,signature_version) values($1,$2,'canonical','completed','v1','v1')`, []any{shadowID, projectID}},
		{`insert into discovery_candidates(id,project_id,shadow_run_id,source_kind,source_object_type,source_object_id,target_kind,issue_or_hypothesis_family,change_family,artifact_intent,verification_mode,suggested_owner,candidate_schema_version,status,evidence_fingerprint,exact_signature_hash,signature_payload,conflict_bucket_keys) values($1,$2,$3,'doctor','seo_doctor_finding',$4,'page','canonical','metadata_rewrite','repair_existing_surface','immediate','doctor','v1','identity_ready','fixture-evidence',$5,'{}','["fixture-bucket"]')`, []any{candidateID, projectID, shadowID, findingID.String(), "fixture-signature-" + suffix}},
		{`insert into work_conflict_buckets(id,project_id,bucket_key) values($1,$2,'fixture-bucket')`, []any{bucketID, projectID}},
		{`insert into work_signature_registry(id,project_id,candidate_id,shadow_run_id,mode,status,active,exact_signature_hash,signature_payload,conflict_bucket_keys,signature_version,owner,source_object_type,source_object_id,reserved_work_type,reserved_work_id,evidence_fingerprint) values($1,$2,$3,$4,'enforced',$5,true,$6,'{}','["fixture-bucket"]','v1','doctor','seo_doctor_finding',$7,'site_fix',$8,'fixture-evidence')`, []any{signatureID, projectID, candidateID, shadowID, signatureStatus, "fixture-signature-" + suffix, findingID.String(), fixID}},
		{`insert into site_fixes(id,project_id,doctor_finding_id,candidate_id,work_signature_id,status,finding_kind,target_urls,evidence_snapshot,proposed_fix,acceptance_tests) values($1,$2,$3,$4,$5,$6,'broken','["https://example.com/"]','{}','{}','[{"type":"canonical_present"}]')`, []any{fixID, projectID, findingID, candidateID, signatureID, fixStatus}},
	}
	if appStatus != "" {
		statements = append(statements, struct {
			sql  string
			args []any
		}{`insert into site_change_applications(id,project_id,site_fix_id,application_kind,target_url,normalized_target_url,opportunity_key,status) values($1,$2,$3,'site_fix','https://example.com/','https://example.com/',$4,$5)`, []any{appID, projectID, fixID, "doctor:" + fixID.String(), appStatus}})
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			_, _ = pool.Exec(ctx, `delete from projects where id=$1`, projectID)
			t.Fatalf("fixture insert failed (%s): %v", fmt.Sprintf("%.48s", statement.sql), err)
		}
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `delete from projects where id=$1`, projectID) })
	return projectID, fixID, appID
}
