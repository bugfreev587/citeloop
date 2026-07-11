package sitefix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/discovery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresMigrationStore struct{ pool *pgxpool.Pool }

func NewPostgresMigrationStore(pool *pgxpool.Pool) *PostgresMigrationStore {
	return &PostgresMigrationStore{pool: pool}
}

func (s *PostgresMigrationStore) Snapshot(ctx context.Context, projectID uuid.UUID) ([]LegacyTechnicalAction, string, error) {
	if s == nil || s.pool == nil {
		return nil, "", errors.New("migration database is unavailable")
	}
	q := db.New(s.pool)
	authority, err := q.GetProductWriterAuthority(ctx, db.GetProductWriterAuthorityParams{ProjectID: projectID, Product: "doctor"})
	if err != nil {
		return nil, "", fmt.Errorf("load Doctor writer authority: %w", err)
	}
	rows, err := q.ListLegacyTechnicalActionsForMigration(ctx, projectID)
	if err != nil {
		return nil, "", fmt.Errorf("list legacy technical actions: %w", err)
	}
	opportunities, err := q.ListLegacyTechnicalOpportunitiesWithoutActions(ctx, projectID)
	if err != nil {
		return nil, "", fmt.Errorf("list legacy technical opportunities: %w", err)
	}
	activeRows, err := q.ListActiveWorkForLegacyMigration(ctx, projectID)
	if err != nil {
		return nil, "", fmt.Errorf("list active work for migration: %w", err)
	}
	active := migrationActiveWork(activeRows)
	plannedRows, err := q.ListPlannedWorkForLegacyMigration(ctx, projectID)
	if err != nil {
		return nil, "", fmt.Errorf("list planned work for migration: %w", err)
	}
	active = append(active, migrationPlannedWork(plannedRows)...)
	memoryRows, err := q.ListActiveReviewMemoryForLegacyMigration(ctx, projectID)
	if err != nil {
		return nil, "", fmt.Errorf("list active review memory for migration: %w", err)
	}
	activeMemory := migrationActiveReviewMemory(memoryRows)
	result := append(legacyRows(rows), legacyOpportunityRows(opportunities)...)
	bucketVersions, err := migrationBucketVersionSnapshot(ctx, q, projectID, result)
	if err != nil {
		return nil, "", err
	}
	for index := range result {
		result[index].ActiveWork = active
		result[index].ActiveReviewMemory = activeMemory
		result[index].BucketVersions = bucketVersions
	}
	return result, authority.WriterAuthority, nil
}

func legacyRows(rows []db.ListLegacyTechnicalActionsForMigrationRow) []LegacyTechnicalAction {
	result := make([]LegacyTechnicalAction, 0, len(rows))
	for _, row := range rows {
		findingID, _ := uuid.Parse(strings.TrimSpace(row.DoctorFindingID))
		reviewStateID, _ := uuid.Parse(strings.TrimSpace(row.ReviewStateID))
		evidence := jsonValue(row.Evidence)
		legacy := LegacyTechnicalAction{ProjectID: row.ProjectID, OpportunityID: row.OpportunityID,
			ActionID: row.ActionID, DoctorFindingID: findingID, TargetURL: row.TargetUrl,
			ChangeFamily: row.ChangeFamily, OpportunityType: row.OpportunityType,
			OpportunityEvidence: row.OpportunityEvidence, OpportunityQuery: row.Query,
			OpportunityRecommendedAction: row.RecommendedAction, Status: row.Status, Evidence: evidence,
			ApplicationIDs: stableUUIDs(row.ApplicationIds), ReviewDecision: row.ReviewDecision,
			ReviewStateID: reviewStateID, ReviewDecidedBy: row.ReviewDecidedBy,
			ReviewEvidenceFingerprint:    row.ReviewEvidenceFingerprint,
			ReviewMaterialChangeMetadata: row.ReviewMaterialChangeMetadata,
			ReviewHistory:                legacyReviewHistory(row.ReviewHistory),
			ApprovedBy:                   row.ApprovedBy, ApprovalSource: row.ApprovalSource,
			InputSnapshot: row.InputSnapshot, OutputSnapshot: row.OutputSnapshot, DiffSnapshot: row.DiffSnapshot,
			ApplicationStatus: row.ApplicationStatus, HasPullRequest: row.HasPullRequest,
			DeploymentObserved: row.DeploymentObserved, VerificationPassed: row.VerificationPassed,
			ActionLegacyDisposition: row.ActionLegacyDisposition, OpportunityLegacyDisposition: row.OpportunityLegacyDisposition}
		if row.ReviewSnoozedUntil.Valid {
			value := row.ReviewSnoozedUntil.Time.UTC()
			legacy.ReviewSnoozedUntil = &value
		}
		if row.ReviewDecidedAt.Valid {
			legacy.ReviewDecidedAt = row.ReviewDecidedAt.Time.UTC()
		}
		if row.OriginalCreatedAt.Valid {
			legacy.CreatedAt = row.OriginalCreatedAt.Time.UTC()
		}
		if row.OriginalUpdatedAt.Valid {
			legacy.UpdatedAt = row.OriginalUpdatedAt.Time.UTC()
		}
		if row.ApprovedAt.Valid {
			approved := row.ApprovedAt.Time.UTC()
			legacy.ApprovedAt = &approved
		}
		result = append(result, legacy)
	}
	return result
}

func legacyOpportunityRows(rows []db.ListLegacyTechnicalOpportunitiesWithoutActionsRow) []LegacyTechnicalAction {
	result := make([]LegacyTechnicalAction, 0, len(rows))
	for _, row := range rows {
		findingID, _ := uuid.Parse(strings.TrimSpace(row.DoctorFindingID))
		reviewID, _ := uuid.Parse(strings.TrimSpace(row.ReviewStateID))
		target := row.NormalizedPageUrl
		if strings.TrimSpace(target) == "" && row.PageUrl != nil {
			target = *row.PageUrl
		}
		legacy := LegacyTechnicalAction{ProjectID: row.ProjectID, OpportunityID: row.ID, DoctorFindingID: findingID,
			TargetURL: target, ChangeFamily: row.Type, OpportunityType: row.Type, OpportunityEvidence: row.Evidence,
			OpportunityQuery: row.Query, OpportunityRecommendedAction: row.RecommendedAction, Status: row.Status,
			Evidence: row.Evidence, ReviewDecision: row.ReviewDecision, ReviewStateID: reviewID,
			ReviewDecidedBy: row.ReviewDecidedBy, ReviewEvidenceFingerprint: row.ReviewEvidenceFingerprint,
			ReviewMaterialChangeMetadata: row.ReviewMaterialChangeMetadata,
			ReviewHistory:                legacyReviewHistory(row.ReviewHistory),
			OpportunityLegacyDisposition: row.LegacyMigrationDisposition}
		if row.CreatedAt.Valid {
			legacy.CreatedAt = row.CreatedAt.Time.UTC()
		}
		if row.UpdatedAt.Valid {
			legacy.UpdatedAt = row.UpdatedAt.Time.UTC()
		}
		if row.ReviewSnoozedUntil.Valid {
			value := row.ReviewSnoozedUntil.Time.UTC()
			legacy.ReviewSnoozedUntil = &value
		}
		if row.ReviewDecidedAt.Valid {
			legacy.ReviewDecidedAt = row.ReviewDecidedAt.Time.UTC()
		}
		result = append(result, legacy)
	}
	return result
}

func migrationActiveWork(rows []db.ListActiveWorkForLegacyMigrationRow) []MigrationActiveWork {
	result := make([]MigrationActiveWork, 0, len(rows))
	for _, row := range rows {
		var buckets []string
		_ = json.Unmarshal(row.ConflictBucketKeys, &buckets)
		workID := uuid.Nil
		if row.WorkID.Valid {
			workID = uuid.UUID(row.WorkID.Bytes)
		}
		result = append(result, MigrationActiveWork{WorkSignatureID: row.WorkSignatureID,
			ExactSignatureHash: row.ExactSignatureHash, ConflictBucketKeys: buckets, Owner: row.Owner,
			WorkType: row.WorkType, WorkID: workID, Active: row.Active, ActiveApplicationCount: row.ActiveApplicationCount})
	}
	return result
}

func migrationActiveReviewMemory(rows []db.ListActiveReviewMemoryForLegacyMigrationRow) []MigrationActiveReviewMemory {
	result := make([]MigrationActiveReviewMemory, 0, len(rows))
	for _, row := range rows {
		var buckets []string
		_ = json.Unmarshal(row.ConflictBucketKeys, &buckets)
		memory := MigrationActiveReviewMemory{ID: row.ID, ExactSignatureHash: row.MatchSignatureHash, SemanticFingerprint: row.MatchSemanticFingerprint, ConflictBucketKeys: buckets, Decision: row.Decision, EvidenceFingerprint: row.EvidenceFingerprintAtDecision, ViaAlias: row.ViaAlias}
		if row.SnoozedUntil.Valid {
			value := row.SnoozedUntil.Time.UTC()
			memory.SnoozedUntil = &value
		}
		result = append(result, memory)
	}
	return result
}

func migrationPlannedWork(rows []db.ListPlannedWorkForLegacyMigrationRow) []MigrationActiveWork {
	result := make([]MigrationActiveWork, 0, len(rows))
	for _, row := range rows {
		var buckets []string
		_ = json.Unmarshal(row.ConflictBucketKeys, &buckets)
		result = append(result, MigrationActiveWork{WorkSignatureID: row.ID, ExactSignatureHash: row.ExactSignatureHash, ConflictBucketKeys: buckets, Owner: row.Owner, WorkType: "planned_candidate", WorkID: row.ID, Planned: true})
	}
	return result
}

func migrationBucketVersionSnapshot(ctx context.Context, q *db.Queries, projectID uuid.UUID, rows []LegacyTechnicalAction) (map[string]int64, error) {
	versions := map[string]int64{}
	for _, row := range rows {
		_, identity, reason := canonicalLegacyMigrationIdentity(row)
		if reason != "" && identity.ExactSignatureHash == "" {
			continue
		}
		for _, key := range identity.ConflictBucketKeys {
			versions[key] = 0
		}
	}
	if len(versions) == 0 {
		return versions, nil
	}
	keys := make([]string, 0, len(versions))
	for key := range versions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	buckets, err := q.GetConflictBucketSnapshot(ctx, db.GetConflictBucketSnapshotParams{ProjectID: projectID, BucketKeys: keys})
	if err != nil {
		return nil, err
	}
	for _, bucket := range buckets {
		versions[bucket.BucketKey] = bucket.BucketVersion
	}
	return versions, nil
}

func jsonValue(value any) json.RawMessage {
	switch typed := value.(type) {
	case []byte:
		return append(json.RawMessage(nil), typed...)
	case string:
		return json.RawMessage(typed)
	case json.RawMessage:
		return append(json.RawMessage(nil), typed...)
	default:
		raw, _ := json.Marshal(value)
		return raw
	}
}

func legacyReviewHistory(raw json.RawMessage) []LegacyReviewState {
	var history []LegacyReviewState
	_ = json.Unmarshal(raw, &history)
	return history
}

func (s *PostgresMigrationStore) Apply(ctx context.Context, plan MigrationPlan) (MigrationBatchReport, error) {
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		result, err := s.applyOnce(ctx, plan)
		if err == nil || !isRetryableMigrationTx(err) {
			return result, err
		}
		last = err
		if err := waitMigrationRetry(ctx, attempt); err != nil {
			return MigrationBatchReport{}, err
		}
	}
	return MigrationBatchReport{}, last
}

func (s *PostgresMigrationStore) applyOnce(ctx context.Context, plan MigrationPlan) (MigrationBatchReport, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return MigrationBatchReport{}, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	authority, err := q.LockProductWriterAuthority(ctx, db.LockProductWriterAuthorityParams{ProjectID: plan.ProjectID, Product: "doctor"})
	if err != nil {
		return MigrationBatchReport{}, err
	}
	if authority.WriterAuthority != "legacy" || authority.WriteFenced {
		return MigrationBatchReport{}, ErrMigrationSnapshotDrift
	}
	now := time.Now().UTC()
	fenceToken := uuid.New()
	if _, err = q.FenceProductWriterAuthority(ctx, db.FenceProductWriterAuthorityParams{
		FenceToken: pgUUID(fenceToken), FencedBy: &plan.InitiatedBy,
		FencedAt: pgtype.Timestamptz{Time: now, Valid: true}, ProjectID: plan.ProjectID, Product: "doctor",
	}); err != nil {
		return MigrationBatchReport{}, fmt.Errorf("fence Doctor writer: %w", err)
	}
	if _, err = tx.Exec(ctx, "select set_config('citeloop.migration_fence_token', $1, true)", fenceToken.String()); err != nil {
		return MigrationBatchReport{}, fmt.Errorf("bind migration fence token: %w", err)
	}
	rows, err := q.ListLegacyTechnicalActionsForMigration(ctx, plan.ProjectID)
	if err != nil {
		return MigrationBatchReport{}, err
	}
	opportunities, err := q.ListLegacyTechnicalOpportunitiesWithoutActions(ctx, plan.ProjectID)
	if err != nil {
		return MigrationBatchReport{}, err
	}
	plannedBucketKeys := plannedMigrationBucketKeys(plan.Items)
	preexistingBucketKeys := map[string]bool{}
	initialBucketVersions := map[string]int64{}
	lockedBucketVersions := map[string]int64{}
	if len(plannedBucketKeys) > 0 {
		beforeBuckets, snapshotErr := q.GetConflictBucketSnapshot(ctx, db.GetConflictBucketSnapshotParams{ProjectID: plan.ProjectID, BucketKeys: plannedBucketKeys})
		if snapshotErr != nil {
			return MigrationBatchReport{}, snapshotErr
		}
		for _, key := range plannedBucketKeys {
			initialBucketVersions[key] = 0
		}
		for _, bucket := range beforeBuckets {
			preexistingBucketKeys[bucket.BucketKey] = true
			initialBucketVersions[bucket.BucketKey] = bucket.BucketVersion
		}
		if _, materializeErr := q.MaterializeConflictBuckets(ctx, db.MaterializeConflictBucketsParams{ProjectID: plan.ProjectID, BucketKeys: plannedBucketKeys}); materializeErr != nil {
			return MigrationBatchReport{}, materializeErr
		}
		lockedBuckets, lockErr := q.LockConflictBucketsForReserve(ctx, db.LockConflictBucketsForReserveParams{ProjectID: plan.ProjectID, BucketKeys: plannedBucketKeys})
		if lockErr != nil {
			return MigrationBatchReport{}, lockErr
		}
		if len(lockedBuckets) != len(plannedBucketKeys) {
			return MigrationBatchReport{}, ErrMigrationSnapshotDrift
		}
		for _, bucket := range lockedBuckets {
			lockedBucketVersions[bucket.BucketKey] = bucket.BucketVersion
		}
	}
	activeRows, err := q.ListActiveWorkForLegacyMigration(ctx, plan.ProjectID)
	if err != nil {
		return MigrationBatchReport{}, err
	}
	plannedRows, err := q.ListPlannedWorkForLegacyMigration(ctx, plan.ProjectID)
	if err != nil {
		return MigrationBatchReport{}, err
	}
	memoryRows, err := q.ListActiveReviewMemoryForLegacyMigration(ctx, plan.ProjectID)
	if err != nil {
		return MigrationBatchReport{}, err
	}
	sources := append(legacyRows(rows), legacyOpportunityRows(opportunities)...)
	active := append(migrationActiveWork(activeRows), migrationPlannedWork(plannedRows)...)
	activeMemory := migrationActiveReviewMemory(memoryRows)
	for index := range sources {
		sources[index].ActiveWork = active
		sources[index].ActiveReviewMemory = activeMemory
		sources[index].BucketVersions = lockedBucketVersions
	}
	lockedPlan, err := ClassifyLegacyTechnicalActions(plan.ProjectID, authority.WriterAuthority, sources)
	if err != nil {
		return MigrationBatchReport{}, err
	}
	if lockedPlan.SnapshotHash != plan.SnapshotHash {
		return MigrationBatchReport{}, ErrMigrationSnapshotDrift
	}
	plan.MigrationDryRunReport = lockedPlan
	for index := range plan.Items {
		if plan.Items[index].Disposition == MigrationDispositionMigrate {
			plan.Items[index].SiteFixID = uuid.New()
		}
	}

	sourceSnapshot := mustJSON(lockedPlan)
	resultHash := ""
	resultSnapshot := mustJSON(map[string]any{"computed_from": "migration_ledger_current_snapshots", "source_snapshot_hash": plan.SnapshotHash})
	status := "completed"
	if plan.ReviewCount > 0 {
		status = "review_required"
	}
	if _, err = q.CreateMigrationBatch(ctx, db.CreateMigrationBatchParams{
		ID: plan.BatchID, ProjectID: plan.ProjectID, Product: "doctor", BatchKind: "forward", Status: status,
		SchemaVersion: migrationSchemaVersion, SourceCount: int32(plan.SourceCount), MigratedCount: int32(plan.MigratedCount),
		ArchivedDuplicateCount: int32(plan.ArchivedDuplicateCount), ReviewCount: int32(plan.ReviewCount),
		WriterAuthorityBefore: "legacy", WriterAuthorityAfter: "canonical", SourceSnapshot: sourceSnapshot,
		ResultSnapshot: resultSnapshot, InitiatedBy: plan.InitiatedBy,
		StartedAt: pgtype.Timestamptz{Time: now, Valid: true}, FinishedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}); err != nil {
		return MigrationBatchReport{}, fmt.Errorf("create migration batch: %w", err)
	}

	byAction := make(map[uuid.UUID]LegacyTechnicalAction, len(sources))
	for _, row := range sources {
		byAction[legacyMigrationSourceID(row)] = row
	}
	createdFixes := make(map[uuid.UUID]uuid.UUID, plan.MigratedCount)
	aliased := map[string]bool{}
	ledgeredLegacy := map[string]bool{}
	repointedApplications := 0
	createdReviewMemories := 0
	sequence := int32(0)
	appendLedger := func(sourceType string, sourceID uuid.UUID, canonicalType string, canonicalID uuid.UUID, operation string, before, after, inverse any) error {
		sequence++
		beforeRaw, afterRaw, inverseRaw := mustJSON(before), mustJSON(after), mustJSON(inverse)
		if canonicalID != uuid.Nil {
			persisted, snapshotErr := q.GetMigrationCurrentSnapshot(ctx, db.GetMigrationCurrentSnapshotParams{ObjectType: canonicalType, ProjectID: plan.ProjectID, ObjectID: canonicalID})
			if snapshotErr != nil {
				return fmt.Errorf("snapshot migrated %s %s: %w", canonicalType, canonicalID, snapshotErr)
			}
			afterRaw = persisted
		}
		_, ledgerErr := q.AppendMigrationLedger(ctx, db.AppendMigrationLedgerParams{
			ID: uuid.New(), ProjectID: plan.ProjectID, MigrationBatchID: plan.BatchID, SequenceNumber: sequence,
			SourceObjectType: sourceType, SourceObjectID: sourceID, CanonicalObjectType: canonicalType,
			CanonicalObjectID: pgtype.UUID{Bytes: canonicalID, Valid: canonicalID != uuid.Nil}, Operation: operation,
			OperationVersion: operationVersionSiteFixMigrationV1, CutoverPoint: "writer_fenced", RollbackEligibility: "eligible",
			BeforeHash: digestBytes(beforeRaw), AfterHash: digestBytes(afterRaw), BeforeSnapshot: beforeRaw, AfterSnapshot: afterRaw,
			InverseOperationVersion: inverseOperationVersionSiteFixMigrationV1, InverseOperation: inverseRaw,
			AppliedBy: plan.InitiatedBy, AppliedAt: pgtype.Timestamptz{Time: now, Valid: true},
		})
		return ledgerErr
	}
	createAlias := func(legacyType string, legacyID uuid.UUID, canonicalType string, canonicalID uuid.UUID, provenance any) error {
		key := legacyType + ":" + legacyID.String()
		if aliased[key] {
			return nil
		}
		aliased[key] = true
		alias, aliasErr := q.CreateLegacyObjectAlias(ctx, db.CreateLegacyObjectAliasParams{ID: uuid.New(), ProjectID: plan.ProjectID,
			MigrationBatchID: plan.BatchID, LegacyObjectType: legacyType, LegacyObjectID: legacyID,
			CanonicalObjectType: canonicalType, CanonicalObjectID: canonicalID, AliasState: "active", ProvenanceSnapshot: mustJSON(provenance)})
		if aliasErr != nil {
			return aliasErr
		}
		return appendLedger(legacyType, legacyID, "legacy_object_alias", alias.ID, "create", map[string]any{"missing": true}, nil, map[string]any{"operation": "tombstone_alias"})
	}
	createReviewAliases := func(legacy LegacyTechnicalAction, canonicalType string, canonicalID uuid.UUID) error {
		for _, review := range legacy.ReviewHistory {
			if review.ID == uuid.Nil {
				continue
			}
			if err := createAlias("seo_opportunity_review_state", review.ID, canonicalType, canonicalID, review); err != nil {
				return err
			}
		}
		if legacy.ReviewStateID != uuid.Nil {
			if err := createAlias("seo_opportunity_review_state", legacy.ReviewStateID, canonicalType, canonicalID, legacy); err != nil {
				return err
			}
		}
		return nil
	}
	appendLegacyMutations := func(legacy LegacyTechnicalAction, operation string) error {
		if legacy.ActionID != uuid.Nil {
			key := "content_action:" + legacy.ActionID.String()
			if !ledgeredLegacy[key] {
				ledgeredLegacy[key] = true
				disposition := legacy.ActionLegacyDisposition
				if disposition == "" {
					disposition = "none"
				}
				if err := appendLedger("content_action", legacy.ActionID, "content_action", legacy.ActionID, operation, legacy, nil, map[string]any{"operation": "restore_content_action", "legacy_disposition": disposition}); err != nil {
					return err
				}
			}
		}
		key := "seo_opportunity:" + legacy.OpportunityID.String()
		if !ledgeredLegacy[key] {
			ledgeredLegacy[key] = true
			disposition := legacy.OpportunityLegacyDisposition
			if disposition == "" {
				disposition = "none"
			}
			if err := appendLedger("seo_opportunity", legacy.OpportunityID, "seo_opportunity", legacy.OpportunityID, operation, legacy, nil, map[string]any{"operation": "restore_legacy", "legacy_disposition": disposition}); err != nil {
				return err
			}
		}
		return nil
	}
	if err := appendLedger("migration_batch", plan.BatchID, "migration_batch", plan.BatchID, "create", map[string]any{"missing": true}, nil, map[string]any{"operation": "retain_tombstone"}); err != nil {
		return MigrationBatchReport{}, err
	}

	for _, item := range plan.Items {
		legacy := byAction[classificationSourceID(item)]
		sourceID, sourceType := legacyMigrationSourceID(legacy), migrationSourceObjectType(legacy)
		canonicalFixID := uuid.Nil
		switch item.Disposition {
		case MigrationDispositionMigrate:
			artifacts, err := s.createArtifacts(ctx, q, plan, legacy, item, fenceToken, now)
			if err != nil {
				return MigrationBatchReport{}, err
			}
			fixID, findingCreated := artifacts.FixID, artifacts.FindingCreated
			createdFixes[sourceID] = fixID
			canonicalFixID = fixID
			if artifacts.DoctorRunID != uuid.Nil {
				if err := appendLedger(sourceType, sourceID, "seo_doctor_run", artifacts.DoctorRunID, "create", map[string]any{"missing": true}, nil, map[string]any{"operation": "retain_tombstone"}); err != nil {
					return MigrationBatchReport{}, err
				}
			}
			if findingCreated != uuid.Nil {
				if err := appendLedger(sourceType+":finding", sourceID, "seo_doctor_finding", findingCreated, "create", map[string]any{"missing": true}, nil, map[string]any{"operation": "tombstone_finding"}); err != nil {
					return MigrationBatchReport{}, err
				}
			}
			if err := appendLedger(sourceType, sourceID, "discovery_shadow_run", artifacts.ShadowRunID, "create", map[string]any{"missing": true}, nil, map[string]any{"operation": "retain_tombstone"}); err != nil {
				return MigrationBatchReport{}, err
			}
			if err := appendLedger(sourceType, sourceID, "discovery_candidate", artifacts.CandidateID, "create", map[string]any{"missing": true}, nil, map[string]any{"operation": "retain_tombstone"}); err != nil {
				return MigrationBatchReport{}, err
			}
			if err := appendLedger(sourceType, sourceID, "work_signature_registry", artifacts.SignatureID, "create", map[string]any{"missing": true}, nil, map[string]any{"operation": "deactivate_signature"}); err != nil {
				return MigrationBatchReport{}, err
			}
			if err := appendLedger(sourceType, sourceID, "site_fix", fixID, "create", map[string]any{"missing": true}, nil, map[string]any{"operation": "tombstone_site_fix"}); err != nil {
				return MigrationBatchReport{}, err
			}
			if err := appendLegacyMutations(legacy, "mark_canonical_read_only"); err != nil {
				return MigrationBatchReport{}, err
			}
			if legacy.ActionID != uuid.Nil {
				if err := createAlias("content_action", legacy.ActionID, "site_fix", fixID, legacy); err != nil {
					return MigrationBatchReport{}, err
				}
			}
			if err := createAlias("seo_opportunity", legacy.OpportunityID, "site_fix", fixID, legacy); err != nil {
				return MigrationBatchReport{}, err
			}
			apps := []db.SiteChangeApplication{}
			beforeApps := map[uuid.UUID]db.SiteChangeApplication{}
			var appErr error
			if legacy.ActionID != uuid.Nil {
				lockedApps, lockErr := q.ListLegacyApplicationsForMigrationUpdate(ctx, db.ListLegacyApplicationsForMigrationUpdateParams{ProjectID: plan.ProjectID, LegacyActionID: pgUUID(legacy.ActionID)})
				if lockErr != nil {
					return MigrationBatchReport{}, lockErr
				}
				for _, app := range lockedApps {
					beforeApps[app.ID] = app
				}
				apps, appErr = q.RepointLegacyApplicationsToCanonicalSiteFix(ctx, db.RepointLegacyApplicationsToCanonicalSiteFixParams{ProjectID: plan.ProjectID, SiteFixID: pgUUID(fixID), LegacyActionID: pgUUID(legacy.ActionID), MigrationBatchID: pgUUID(plan.BatchID), FenceToken: pgUUID(fenceToken)})
			}
			if appErr != nil {
				return MigrationBatchReport{}, appErr
			}
			if len(apps) != len(beforeApps) {
				return MigrationBatchReport{}, errors.New("legacy application conservation failed during migration")
			}
			repointedApplications += len(apps)
			for _, app := range apps {
				before := beforeApps[app.ID]
				if err := appendLedger("site_change_application", app.ID, "site_change_application", app.ID, "repoint", before, nil, map[string]any{"operation": "restore_application_source", "content_action_id": legacy.ActionID, "application_kind": before.ApplicationKind, "site_fix_id": fixID}); err != nil {
					return MigrationBatchReport{}, err
				}
				if err := createAlias("site_change_application", app.ID, "site_fix", fixID, before); err != nil {
					return MigrationBatchReport{}, err
				}
			}
		case MigrationDispositionArchiveDuplicate:
			fixID := item.ExistingSiteFixID
			winnerID := item.CanonicalSourceID
			if winnerID == uuid.Nil {
				winnerID = item.CanonicalActionID
			}
			if fixID == uuid.Nil {
				fixID = createdFixes[winnerID]
			}
			if fixID == uuid.Nil {
				return MigrationBatchReport{}, errors.New("duplicate winner was not migrated first")
			}
			canonicalFixID = fixID
			if legacy.ActionID != uuid.Nil {
				if _, err := q.MarkLegacyDuplicateCanonicalReadOnly(ctx, db.MarkLegacyDuplicateCanonicalReadOnlyParams{SiteFixID: pgUUID(fixID), MigrationBatchID: pgUUID(plan.BatchID), ProjectID: plan.ProjectID, LegacyActionID: legacy.ActionID}); err != nil {
					return MigrationBatchReport{}, err
				}
			} else if _, err := q.MarkLegacyOpportunityDuplicateReadOnly(ctx, db.MarkLegacyOpportunityDuplicateReadOnlyParams{SiteFixID: pgUUID(fixID), MigrationBatchID: pgUUID(plan.BatchID), ProjectID: plan.ProjectID, LegacyOpportunityID: legacy.OpportunityID}); err != nil {
				return MigrationBatchReport{}, err
			}
			if err := appendLegacyMutations(legacy, "archive_duplicate"); err != nil {
				return MigrationBatchReport{}, err
			}
			if legacy.ActionID != uuid.Nil {
				if err := createAlias("content_action", legacy.ActionID, "site_fix", fixID, legacy); err != nil {
					return MigrationBatchReport{}, err
				}
			}
			if err := createAlias("seo_opportunity", legacy.OpportunityID, "site_fix", fixID, legacy); err != nil {
				return MigrationBatchReport{}, err
			}
			apps := []db.SiteChangeApplication{}
			beforeApps := map[uuid.UUID]db.SiteChangeApplication{}
			var err error
			if legacy.ActionID != uuid.Nil {
				lockedApps, lockErr := q.ListLegacyApplicationsForMigrationUpdate(ctx, db.ListLegacyApplicationsForMigrationUpdateParams{ProjectID: plan.ProjectID, LegacyActionID: pgUUID(legacy.ActionID)})
				if lockErr != nil {
					return MigrationBatchReport{}, lockErr
				}
				for _, app := range lockedApps {
					beforeApps[app.ID] = app
				}
				apps, err = q.RepointLegacyApplicationsToCanonicalSiteFix(ctx, db.RepointLegacyApplicationsToCanonicalSiteFixParams{ProjectID: plan.ProjectID, SiteFixID: pgUUID(fixID), LegacyActionID: pgUUID(legacy.ActionID), MigrationBatchID: pgUUID(plan.BatchID), FenceToken: pgUUID(fenceToken)})
			}
			if err != nil {
				return MigrationBatchReport{}, err
			}
			if len(apps) != len(beforeApps) {
				return MigrationBatchReport{}, errors.New("legacy duplicate application conservation failed during migration")
			}
			repointedApplications += len(apps)
			for _, app := range apps {
				before := beforeApps[app.ID]
				if err := appendLedger("site_change_application", app.ID, "site_change_application", app.ID, "repoint", before, nil, map[string]any{"operation": "restore_application_source", "content_action_id": legacy.ActionID, "application_kind": before.ApplicationKind, "site_fix_id": fixID}); err != nil {
					return MigrationBatchReport{}, err
				}
				if err := createAlias("site_change_application", app.ID, "site_fix", fixID, before); err != nil {
					return MigrationBatchReport{}, err
				}
			}
		case MigrationDispositionReview:
			reviewID := uuid.New()
			if _, err := q.CreateMigrationReviewItem(ctx, db.CreateMigrationReviewItemParams{ID: reviewID, ProjectID: plan.ProjectID, MigrationBatchID: plan.BatchID, SourceObjectType: sourceType, SourceObjectID: sourceID, ReasonCode: item.ReasonCode, Reason: "Legacy technical work requires operator resolution", SourceSnapshot: mustJSON(legacy), ProposedResolution: mustJSON(map[string]any{"action": "resolve_target_or_evidence"})}); err != nil {
				return MigrationBatchReport{}, err
			}
			if legacy.ActionID != uuid.Nil {
				if _, err := q.MarkLegacyMigrationReviewReadOnly(ctx, db.MarkLegacyMigrationReviewReadOnlyParams{MigrationBatchID: pgUUID(plan.BatchID), ProjectID: plan.ProjectID, LegacyActionID: legacy.ActionID}); err != nil {
					return MigrationBatchReport{}, err
				}
			} else if _, err := q.MarkLegacyOpportunityMigrationReviewReadOnly(ctx, db.MarkLegacyOpportunityMigrationReviewReadOnlyParams{MigrationBatchID: pgUUID(plan.BatchID), ProjectID: plan.ProjectID, LegacyOpportunityID: legacy.OpportunityID}); err != nil {
				return MigrationBatchReport{}, err
			}
			if err := appendLedger(sourceType, sourceID, "migration_review_item", reviewID, "create", map[string]any{"missing": true}, nil, map[string]any{"operation": "dismiss_migration_review"}); err != nil {
				return MigrationBatchReport{}, err
			}
			if err := appendLegacyMutations(legacy, "migration_review"); err != nil {
				return MigrationBatchReport{}, err
			}
			if legacy.ActionID != uuid.Nil {
				applications, appErr := q.ListLegacyApplicationsForMigrationUpdate(ctx, db.ListLegacyApplicationsForMigrationUpdateParams{ProjectID: plan.ProjectID, LegacyActionID: pgUUID(legacy.ActionID)})
				if appErr != nil {
					return MigrationBatchReport{}, appErr
				}
				for _, application := range applications {
					if err := createAlias("site_change_application", application.ID, "migration_review_item", reviewID, application); err != nil {
						return MigrationBatchReport{}, err
					}
				}
			}
			if legacy.ActionID != uuid.Nil {
				if err := createAlias("content_action", legacy.ActionID, "migration_review_item", reviewID, legacy); err != nil {
					return MigrationBatchReport{}, err
				}
			}
			if err := createAlias("seo_opportunity", legacy.OpportunityID, "migration_review_item", reviewID, legacy); err != nil {
				return MigrationBatchReport{}, err
			}
			if legacy.ReviewStateID != uuid.Nil {
				if err := createReviewAliases(legacy, "migration_review_item", reviewID); err != nil {
					return MigrationBatchReport{}, err
				}
			}
		}
		if canonicalFixID != uuid.Nil && legacy.ReviewStateID != uuid.Nil && legacy.ReviewDecision != "" {
			fix, err := q.GetCanonicalSiteFix(ctx, db.GetCanonicalSiteFixParams{ID: canonicalFixID, ProjectID: plan.ProjectID})
			if err != nil {
				return MigrationBatchReport{}, err
			}
			payload, buckets := item.SignaturePayload, mustJSON(item.ConflictBucketKeys)
			reviewCandidate, identity, identityReason := canonicalLegacyMigrationIdentity(legacy)
			if identityReason != "" {
				return MigrationBatchReport{}, errors.New("review memory identity is no longer canonical")
			}
			semanticFingerprint, semanticErr := discovery.DeterministicSemanticFingerprint(identity)
			if semanticErr != nil {
				return MigrationBatchReport{}, semanticErr
			}
			evidenceFingerprint := strings.TrimSpace(legacy.ReviewEvidenceFingerprint)
			if evidenceFingerprint == "" {
				evidenceFingerprint = reviewCandidate.EvidenceFingerprint
			}
			decidedAt := legacy.ReviewDecidedAt
			if decidedAt.IsZero() {
				decidedAt = legacy.UpdatedAt
			}
			if decidedAt.IsZero() {
				decidedAt = now
			}
			decidedBy := strings.TrimSpace(legacy.ReviewDecidedBy)
			if decidedBy == "" {
				decidedBy = plan.InitiatedBy
			}
			snoozed := pgtype.Timestamptz{}
			if legacy.ReviewSnoozedUntil != nil {
				snoozed = pgtime(*legacy.ReviewSnoozedUntil)
			}
			memory, err := q.CreateMigrationWorkReviewMemory(ctx, db.CreateMigrationWorkReviewMemoryParams{ID: uuid.New(), ProjectID: plan.ProjectID,
				CandidateID: pgUUID(fix.CandidateID), WorkSignatureID: pgUUID(fix.WorkSignatureID),
				ExactSignatureHashAtDecision: item.IdentityHash, SemanticFingerprintAtDecision: semanticFingerprint,
				SignaturePayload: payload, ConflictBucketKeys: buckets,
				Decision: legacy.ReviewDecision, DecisionScope: mustJSON(map[string]any{"legacy_review_state_id": legacy.ReviewStateID, "material_change_metadata": json.RawMessage(legacy.ReviewMaterialChangeMetadata)}),
				EvidenceFingerprintAtDecision: evidenceFingerprint, SnoozedUntil: snoozed,
				DecidedBy: decidedBy, DecidedAt: pgtime(decidedAt), MigrationBatchID: pgUUID(plan.BatchID), LegacyReviewStateID: pgUUID(legacy.ReviewStateID)})
			createdMemory := err == nil
			if errors.Is(err, pgx.ErrNoRows) {
				memory, err = q.GetActiveWorkReviewMemoryByExactSignature(ctx, db.GetActiveWorkReviewMemoryByExactSignatureParams{ProjectID: plan.ProjectID, ExactSignatureHashAtDecision: item.IdentityHash})
			}
			if err != nil {
				return MigrationBatchReport{}, err
			}
			if !createdMemory && memory.Decision != legacy.ReviewDecision {
				return MigrationBatchReport{}, errors.New("legacy review decision conflicts with active canonical review memory")
			}
			if createdMemory {
				createdReviewMemories++
				if err := appendLedger("seo_opportunity_review_state", legacy.ReviewStateID, "work_review_memory", memory.ID, "decision_migrate", map[string]any{"missing": true}, nil, map[string]any{"operation": "deactivate_migrated_review_memory"}); err != nil {
					return MigrationBatchReport{}, err
				}
			}
			if err := createReviewAliases(legacy, "work_review_memory", memory.ID); err != nil {
				return MigrationBatchReport{}, err
			}
		}
	}
	if len(plannedBucketKeys) > 0 {
		currentBuckets, bucketErr := q.GetConflictBucketSnapshot(ctx, db.GetConflictBucketSnapshotParams{ProjectID: plan.ProjectID, BucketKeys: plannedBucketKeys})
		if bucketErr != nil {
			return MigrationBatchReport{}, bucketErr
		}
		for _, bucket := range currentBuckets {
			beforeVersion, existed := initialBucketVersions[bucket.BucketKey], preexistingBucketKeys[bucket.BucketKey]
			if existed && beforeVersion == bucket.BucketVersion {
				continue
			}
			if err := appendLedger("work_conflict_bucket", bucket.ID, "work_conflict_bucket", bucket.ID, "migration_bucket_mutation", map[string]any{"bucket_version": beforeVersion, "existed": existed}, nil, map[string]any{"operation": "restore_bucket", "bucket_version": beforeVersion, "created_bucket": !existed, "expected_bucket_version": bucket.BucketVersion}); err != nil {
				return MigrationBatchReport{}, err
			}
		}
	}
	conservation, err := q.GetMigrationConservation(ctx, db.GetMigrationConservationParams{ProjectID: plan.ProjectID, MigrationBatchID: plan.BatchID})
	if err != nil {
		return MigrationBatchReport{}, err
	}
	invariants := MigrationInvariantReport{
		ExpectedAliases: len(aliased), ActiveAliases: int(conservation.ActiveAliasCount),
		ExpectedRepointedApplications: repointedApplications, RepointedApplications: int(conservation.RepointedApplicationCount),
		ExpectedSourceLedgers: len(ledgeredLegacy), SourceLedgers: int(conservation.SourceLedgerCount),
		ExpectedSiteFixes: plan.MigratedCount, SiteFixes: int(conservation.SiteFixCount),
		ExpectedReviewItems: plan.ReviewCount, ReviewItems: int(conservation.ReviewItemCount),
		ExpectedReviewMemories: createdReviewMemories, ReviewMemories: int(conservation.ReviewMemoryCount),
		InvalidApplicationSources: int(conservation.InvalidApplicationSourceCount), UnledgeredObjects: int(conservation.UnledgeredObjectCount),
	}
	invariants.Passed = invariants.ExpectedAliases == invariants.ActiveAliases && invariants.ExpectedRepointedApplications == invariants.RepointedApplications && invariants.ExpectedSourceLedgers == invariants.SourceLedgers && invariants.ExpectedSiteFixes == invariants.SiteFixes && invariants.ExpectedReviewItems == invariants.ReviewItems && invariants.ExpectedReviewMemories == invariants.ReviewMemories && invariants.InvalidApplicationSources == 0 && invariants.UnledgeredObjects == 0
	if !invariants.Passed {
		return MigrationBatchReport{}, errors.New("migration conservation gate failed")
	}
	if err := appendLedger("migration_batch", plan.BatchID, "migration_invariant_report", uuid.Nil, "validate_conservation", map[string]any{}, invariants, map[string]any{"operation": "retain_tombstone"}); err != nil {
		return MigrationBatchReport{}, err
	}
	if sequence == 0 && plan.SourceCount > 0 {
		return MigrationBatchReport{}, errors.New("migration ledger is empty")
	}
	if _, err = q.SwitchProductWriterAuthority(ctx, db.SwitchProductWriterAuthorityParams{WriterAuthority: "canonical", AuthorityChangedAt: pgtype.Timestamptz{Time: now, Valid: true}, ProjectID: plan.ProjectID, Product: "doctor", FenceToken: pgUUID(fenceToken), ExpectedWriterAuthority: "legacy"}); err != nil {
		return MigrationBatchReport{}, err
	}
	if err := appendLedger("product_writer_authority", plan.ProjectID, "product_writer_authority", plan.ProjectID, "authority_switch", map[string]any{"writer_authority": "legacy"}, map[string]any{"writer_authority": "canonical"}, map[string]any{"operation": "restore_authority", "writer_authority": "legacy"}); err != nil {
		return MigrationBatchReport{}, err
	}
	ledger, err := q.ListMigrationLedgerForBatch(ctx, db.ListMigrationLedgerForBatchParams{ProjectID: plan.ProjectID, MigrationBatchID: plan.BatchID})
	if err != nil {
		return MigrationBatchReport{}, err
	}
	resultHash, err = currentMigrationSnapshotHash(ctx, q, plan.ProjectID, ledger)
	if err != nil {
		return MigrationBatchReport{}, err
	}
	if _, err = q.ReleaseProductWriterFence(ctx, db.ReleaseProductWriterFenceParams{ProjectID: plan.ProjectID, Product: "doctor", FenceToken: pgUUID(fenceToken)}); err != nil {
		return MigrationBatchReport{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return MigrationBatchReport{}, err
	}
	return MigrationBatchReport{BatchID: plan.BatchID, ProjectID: plan.ProjectID, SourceCount: plan.SourceCount, MigratedCount: plan.MigratedCount, ArchivedDuplicateCount: plan.ArchivedDuplicateCount, ReviewCount: plan.ReviewCount, WriterAuthority: "canonical", ResultSnapshotHash: resultHash, Status: status, SchemaVersion: migrationSchemaVersion, RollbackEligible: true, Invariants: invariants}, nil
}

type migrationArtifactSet struct {
	FixID          uuid.UUID
	FindingCreated uuid.UUID
	DoctorRunID    uuid.UUID
	ShadowRunID    uuid.UUID
	CandidateID    uuid.UUID
	SignatureID    uuid.UUID
}

func (s *PostgresMigrationStore) createArtifacts(ctx context.Context, q *db.Queries, plan MigrationPlan, legacy LegacyTechnicalAction, item MigrationClassification, fenceToken uuid.UUID, now time.Time) (migrationArtifactSet, error) {
	candidate, identity, reason := canonicalLegacyMigrationIdentity(legacy)
	if reason != "" || identity.ExactSignatureHash != item.IdentityHash || string(identity.SignaturePayload) != string(item.SignaturePayload) {
		return migrationArtifactSet{}, errors.New("migration candidate no longer matches canonical projector")
	}
	targets := mustJSON(candidate.NormalizedTargetSet)
	evidence := mustJSON(map[string]any{"legacy_evidence": json.RawMessage(legacy.Evidence), "legacy_opportunity_evidence": json.RawMessage(legacy.OpportunityEvidence),
		"legacy_opportunity_id": legacy.OpportunityID, "legacy_content_action_id": nullUUID(legacy.ActionID),
		"approval_source": legacy.ApprovalSource, "approved_by": legacy.ApprovedBy,
		"input_snapshot": json.RawMessage(legacy.InputSnapshot), "output_snapshot": json.RawMessage(legacy.OutputSnapshot),
		"diff_snapshot": json.RawMessage(legacy.DiffSnapshot), "application_status": legacy.ApplicationStatus})
	acceptance := mustJSON(migrationAcceptanceTests(candidate, legacy))
	mutations := mustJSON(candidate.ProposedMutations)
	payload := item.SignaturePayload
	buckets := mustJSON(item.ConflictBucketKeys)
	findingID := legacy.DoctorFindingID
	createdFinding := uuid.Nil
	if findingID == uuid.Nil {
		findingID = migrationArtifactUUID("finding", plan.BatchID, legacyMigrationSourceID(legacy))
		createdFinding = findingID
	}
	doctorRunID := uuid.Nil
	if createdFinding != uuid.Nil {
		doctorRunID = migrationArtifactUUID("doctor-run", plan.BatchID, legacyMigrationSourceID(legacy))
	}
	shadowRunID := migrationArtifactUUID("shadow-run", plan.BatchID, legacyMigrationSourceID(legacy))
	candidateID := migrationArtifactUUID("candidate", plan.BatchID, legacyMigrationSourceID(legacy))
	signatureID := migrationArtifactUUID("signature", plan.BatchID, legacyMigrationSourceID(legacy))
	status, registry, registryActive := projectMigrationStatus(legacy)
	approvedAt := pgtype.Timestamptz{}
	if legacy.ApprovedAt != nil {
		approvedAt = pgtype.Timestamptz{Time: legacy.ApprovedAt.UTC(), Valid: true}
	}
	createdAt, updatedAt := legacy.CreatedAt, legacy.UpdatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	row, err := q.CreateMigrationDoctorArtifacts(ctx, db.CreateMigrationDoctorArtifactsParams{
		LegacyActionID: pgUUID(legacy.ActionID), ProjectID: plan.ProjectID, FenceToken: pgUUID(fenceToken), MigrationBatchID: plan.BatchID,
		DoctorRunID: migrationArtifactUUID("doctor-run", plan.BatchID, legacyMigrationSourceID(legacy)), FindingEvidence: evidence,
		InitiatedBy: &plan.InitiatedBy, MigratedAt: pgtime(now), ExistingDoctorFindingID: pgUUID(legacy.DoctorFindingID),
		DoctorFindingID: findingID, SourceObjectID: legacyMigrationSourceID(legacy).String(), ChangeFamily: candidate.ChangeFamily,
		TargetUrls: targets, AcceptanceTests: acceptance, LegacyOpportunityID: pgUUID(legacy.OpportunityID),
		OriginalCreatedAt: pgtime(createdAt), OriginalUpdatedAt: pgtime(updatedAt), ShadowRunID: shadowRunID,
		CandidateID: candidateID, ProposedMutations: mutations,
		SourceObjectType: stringPointerValue(migrationSourceObjectType(legacy)), EvidenceFingerprint: candidate.EvidenceFingerprint,
		ExactSignatureHash: &item.IdentityHash, SignaturePayload: payload,
		ConflictBucketKeys: buckets, WorkSignatureID: signatureID,
		RegistryStatus: registry, RegistryActive: registryActive, SiteFixID: pgUUID(item.SiteFixID), SiteFixStatus: status,
		ProposedFix: mustJSON(map[string]any{"mutations": json.RawMessage(mutations), "legacy_action_type": legacy.ChangeFamily,
			"approved_by": legacy.ApprovedBy, "approval_source": legacy.ApprovalSource,
			"input_snapshot": json.RawMessage(legacy.InputSnapshot), "output_snapshot": json.RawMessage(legacy.OutputSnapshot), "diff_snapshot": json.RawMessage(legacy.DiffSnapshot)}), ApprovedAt: approvedAt,
	})
	if err != nil {
		return migrationArtifactSet{}, fmt.Errorf("create migration Doctor artifacts for %s: %w", legacy.ActionID, err)
	}
	return migrationArtifactSet{FixID: row.ID, FindingCreated: createdFinding, DoctorRunID: doctorRunID, ShadowRunID: shadowRunID, CandidateID: candidateID, SignatureID: signatureID}, nil
}

func migrationAcceptanceTests(candidate discovery.Candidate, legacy LegacyTechnicalAction) []map[string]any {
	target := normalizeMigrationTarget(legacy.TargetURL)
	issue := migrationIssueType(legacy)
	for _, mutation := range candidate.ProposedMutations {
		switch mutation.Field {
		case "jsonld":
			test := map[string]any{"type": "json_ld_valid", "target": target}
			if expected := migrationJSONText(legacy.OpportunityEvidence, "schema_type", "expected_type"); expected != "" {
				test["expected_type"] = expected
			}
			return []map[string]any{test}
		case "canonical":
			expected := migrationExpectedText(legacy, "canonical", "canonical_url", "expected_canonical")
			if expected != "" {
				return []map[string]any{{"type": "canonical_equals", "target": target, "expected": expected}}
			}
			if issue == "canonical_missing" || issue == "canonical_multiple" {
				return []map[string]any{{"type": "canonical_present", "target": target}}
			}
			return nil
		case "robots":
			switch issue {
			case "noindex", "noindex_conflict", "robots_noindex":
				return []map[string]any{{"type": "noindex_absent", "target": target}}
			case "geo_crawler_access_blocked", "robots_blocked", "robots_conflict":
				return []map[string]any{{"type": "content_evidence_present", "target": target, "expected": "an authoritative crawler-access check reports the target is allowed by the deployed robots policy"}}
			default:
				return nil
			}
		case "title":
			expected := migrationExpectedText(legacy, "title", "expected_title", "proposed_title")
			if expected != "" {
				return []map[string]any{{"type": "title_equals", "target": target, "expected": expected}}
			}
			if issue == "title_missing" || issue == "missing_title" {
				return []map[string]any{{"type": "content_evidence_present", "target": target, "expected": "the rendered document has exactly one non-empty title element"}}
			}
			if issue == "metadata_readability" || issue == "duplicate_metadata_template" {
				return []map[string]any{{"type": "content_evidence_present", "target": target, "expected": "the rendered title is readable and unique for the existing page intent"}}
			}
			return nil
		case "http_response":
			expected := migrationExpectedText(legacy, "expected_final_url", "final_url", "redirect_target")
			if issue == "soft_404" || (issue == "redirect_chain" && expected == "") {
				return nil
			}
			if issue != "broken_url" && issue != "http_status" && issue != "redirect_loop" && issue != "redirect_chain" {
				return nil
			}
			test := map[string]any{"type": "content_evidence_present", "target": target, "expected": "the fetched production URL has a successful final HTTP status and no redirect loop"}
			if expected != "" {
				test["expected_final_url"] = expected
				test["expected"] = "the fetched production URL has a successful final HTTP status, no redirect loop, and final URL " + expected
			}
			return []map[string]any{test}
		case "internal_link":
			link := migrationExpectedText(legacy, "target_url", "link_url", "internal_link_target")
			if issue == "internal_link_gap" && link != "" {
				test := map[string]any{"type": "content_evidence_present", "target": target}
				test["expected"] = "the rendered page contains a crawlable internal link to " + link
				test["expected_link"] = link
				return []map[string]any{test}
			}
			if issue == "zero_internal_links" {
				return []map[string]any{{"type": "content_evidence_present", "target": target, "expected": "the rendered page contains at least one crawlable same-site internal link"}}
			}
			return nil
		case "meta_description":
			return []map[string]any{{"type": "content_evidence_present", "target": target, "expected": "the rendered document has exactly one non-empty meta description"}}
		case "h1":
			return []map[string]any{{"type": "content_evidence_present", "target": target, "expected": "the rendered document has exactly one non-empty primary heading"}}
		case "sitemap_entry":
			return []map[string]any{{"type": "content_evidence_present", "target": target, "expected": "the exact canonical target URL appears as a loc entry in an authoritative sitemap"}}
		case "unsafe_output":
			return []map[string]any{{"type": "content_evidence_present", "target": target, "expected": "the rendered output contains no unsafe MDX or template script payload"}}
		case "answer_block":
			return []map[string]any{{"type": "content_evidence_present", "target": target, "expected": "the existing preserved propositions are present in a self-contained extractable answer block"}}
		case "source_association":
			return []map[string]any{{"type": "content_evidence_present", "target": target, "expected": "each preserved proposition retains its visible source association without adding facts"}}
		case "entity_name":
			return []map[string]any{{"type": "content_evidence_present", "target": target, "expected": "the established entity name is consistent across the rendered page metadata and content"}}
		default:
			return nil
		}
	}
	return nil
}

func migrationIssueType(legacy LegacyTechnicalAction) string {
	issue := migrationJSONText(legacy.OpportunityEvidence, "issue")
	if issue == "" {
		issue = migrationJSONText(legacy.Evidence, "issue")
	}
	if issue == "" || strings.EqualFold(strings.TrimSpace(issue), "unknown") {
		issue = legacy.OpportunityType
	}
	if strings.TrimSpace(issue) == "" {
		issue = canonicalLegacyOpportunityType(legacy.ChangeFamily, legacy.OpportunityEvidence)
	}
	issue = strings.ToLower(strings.TrimSpace(issue))
	issue = strings.NewReplacer("-", "_", " ", "_").Replace(issue)
	switch issue {
	case "missing_schema":
		return "schema_gap"
	case "duplicate_title":
		return "title_duplicate"
	default:
		return issue
	}
}

func migrationExpectedText(legacy LegacyTechnicalAction, keys ...string) string {
	for _, raw := range []json.RawMessage{legacy.OutputSnapshot, legacy.DiffSnapshot, legacy.OpportunityEvidence, legacy.Evidence} {
		if value := migrationJSONText(raw, keys...); value != "" {
			return value
		}
	}
	return ""
}

func migrationJSONText(raw json.RawMessage, keys ...string) string {
	var object map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &object) != nil {
		return ""
	}
	for _, key := range keys {
		if value, ok := object[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func plannedMigrationBucketKeys(items []MigrationClassification) []string {
	set := map[string]struct{}{}
	for _, item := range items {
		for _, key := range item.ConflictBucketKeys {
			if strings.TrimSpace(key) != "" {
				set[key] = struct{}{}
			}
		}
	}
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func migrationStatuses(status string) (string, string, bool) {
	return projectMigrationStatus(LegacyTechnicalAction{Status: status})
}

func projectMigrationStatus(legacy LegacyTechnicalAction) (string, string, bool) {
	switch strings.ToLower(strings.TrimSpace(legacy.ReviewDecision)) {
	case "dismissed", "snoozed", "watching":
		return "failed_terminal", "failed_terminal", false
	}
	if legacy.VerificationPassed || legacy.ApplicationStatus == "verified" {
		return "verified", "verified", false
	}
	switch legacy.ApplicationStatus {
	case "verification_pending":
		return "verifying", "verifying", true
	case "deployment_pending", "github_pr_merged":
		return "awaiting_deploy", "awaiting_deploy", true
	case "creating_pr", "github_pr_open":
		return "applying", "executing", true
	case "github_pr_closed":
		return "failed_retryable", "failed_retryable", true
	case "ready_for_pr", "draft_ready", "source_mapping_required", "manual_apply_required":
		return "ready_to_apply", "proposed", true
	case "needs_follow_up", "conflict", "failed":
		return "failed_retryable", "failed_retryable", true
	}
	status := legacy.Status
	switch status {
	case "ready_for_review":
		return "proposed", "proposed", true
	case "approved":
		return "approved", "approved", true
	case "drafting":
		return "preparing", "preparing", true
	case "published":
		return "awaiting_deploy", "awaiting_deploy", true
	case "measuring", "verification_pending":
		if legacy.DeploymentObserved {
			return "verifying", "verifying", true
		}
		return "awaiting_deploy", "awaiting_deploy", true
	case "completed":
		return "verified", "verified", false
	case "verification_failed":
		return "failed_retryable", "failed_retryable", true
	case "manual_apply_required":
		return "ready_to_apply", "proposed", true
	case "needs_follow_up":
		return "failed_retryable", "failed_retryable", true
	case "failed", "recovery_required", "returned", "dismissed":
		return "failed_terminal", "failed_terminal", false
	default:
		return "proposed", "proposed", true
	}
}

func (s *PostgresMigrationStore) Rollback(ctx context.Context, projectID, batchID uuid.UUID, expectedSnapshotHash, initiatedBy string) (MigrationRollbackReport, error) {
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		result, err := s.rollbackOnce(ctx, projectID, batchID, expectedSnapshotHash, initiatedBy)
		if err == nil || !isRetryableMigrationTx(err) {
			return result, err
		}
		last = err
		if err := waitMigrationRetry(ctx, attempt); err != nil {
			return MigrationRollbackReport{}, err
		}
	}
	return MigrationRollbackReport{}, last
}

func (s *PostgresMigrationStore) rollbackOnce(ctx context.Context, projectID, batchID uuid.UUID, expectedSnapshotHash, initiatedBy string) (MigrationRollbackReport, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return MigrationRollbackReport{}, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	batch, err := q.GetMigrationBatch(ctx, db.GetMigrationBatchParams{ProjectID: projectID, MigrationBatchID: batchID})
	if err != nil {
		return MigrationRollbackReport{}, err
	}
	if batch.Product != "doctor" || batch.BatchKind != "forward" {
		return MigrationRollbackReport{}, ErrMigrationSnapshotDrift
	}
	authority, err := q.LockProductWriterAuthority(ctx, db.LockProductWriterAuthorityParams{ProjectID: projectID, Product: "doctor"})
	if err != nil {
		return MigrationRollbackReport{}, err
	}
	if authority.WriterAuthority != "canonical" || authority.WriteFenced {
		return MigrationRollbackReport{}, ErrMigrationSnapshotDrift
	}
	now, fenceToken := time.Now().UTC(), uuid.New()
	if _, err := q.FenceProductWriterAuthority(ctx, db.FenceProductWriterAuthorityParams{FenceToken: pgUUID(fenceToken), FencedBy: &initiatedBy, FencedAt: pgtime(now), ProjectID: projectID, Product: "doctor"}); err != nil {
		return MigrationRollbackReport{}, err
	}
	if _, err := tx.Exec(ctx, "select set_config('citeloop.migration_fence_token', $1, true)", fenceToken.String()); err != nil {
		return MigrationRollbackReport{}, err
	}
	if _, err := q.LockMigrationBucketsForBatch(ctx, db.LockMigrationBucketsForBatchParams{ProjectID: projectID, MigrationBatchID: batchID}); err != nil {
		return MigrationRollbackReport{}, err
	}
	ledger, err := q.ListMigrationLedgerForBatch(ctx, db.ListMigrationLedgerForBatchParams{ProjectID: projectID, MigrationBatchID: batchID})
	if err != nil {
		return MigrationRollbackReport{}, err
	}
	currentResultHash, hashErr := currentMigrationSnapshotHash(ctx, q, projectID, ledger)
	if hashErr != nil || currentResultHash != expectedSnapshotHash {
		return MigrationRollbackReport{}, ErrMigrationSnapshotDrift
	}
	blockers, blockerErr := migrationRollbackBlockers(ctx, q, projectID, ledger)
	if blockerErr != nil {
		return MigrationRollbackReport{}, blockerErr
	}
	eligible := len(blockers) == 0
	if !eligible {
		sequence, sequenceErr := q.GetNextMigrationRollbackEventSequence(ctx, db.GetNextMigrationRollbackEventSequenceParams{ProjectID: projectID, MigrationBatchID: batchID})
		if sequenceErr != nil {
			return MigrationRollbackReport{}, sequenceErr
		}
		_, eventErr := q.AppendMigrationRollbackEvent(ctx, db.AppendMigrationRollbackEventParams{ID: uuid.New(), ProjectID: projectID, MigrationBatchID: batchID, EventSequence: sequence, EventType: "rollback_blocked_forward_fix_required", RollbackEligibility: "blocked_forward_fix_required", CutoverPoint: "writer_fenced", Reason: "canonical-only writes cannot be losslessly inverse-projected", EventSnapshot: mustJSON(map[string]any{"batch_id": batchID, "current_snapshot_hash": currentResultHash, "blockers": blockers}), EventVersion: "site-fix-migration-rollback/v1", OccurredAt: pgtime(now)})
		if eventErr != nil {
			return MigrationRollbackReport{}, eventErr
		}
		if _, releaseErr := q.ReleaseProductWriterFence(ctx, db.ReleaseProductWriterFenceParams{ProjectID: projectID, Product: "doctor", FenceToken: pgUUID(fenceToken)}); releaseErr != nil {
			return MigrationRollbackReport{}, releaseErr
		}
		if err := tx.Commit(ctx); err != nil {
			return MigrationRollbackReport{}, err
		}
		return MigrationRollbackReport{}, ErrMigrationRollbackBlocked
	}
	var authorityOperation *db.MigrationLedger
	for index := len(ledger) - 1; index >= 0; index-- {
		if migrationInverseOperationName(ledger[index].InverseOperation) == "restore_authority" {
			copy := ledger[index]
			authorityOperation = &copy
			continue
		}
		if err := applyMigrationInverse(ctx, q, projectID, batchID, fenceToken, initiatedBy, now, ledger[index]); err != nil {
			return MigrationRollbackReport{}, fmt.Errorf("inverse migration ledger sequence %d: %w", ledger[index].SequenceNumber, err)
		}
	}
	if authorityOperation == nil {
		return MigrationRollbackReport{}, ErrMigrationRollbackBlocked
	}
	if err := applyMigrationInverse(ctx, q, projectID, batchID, fenceToken, initiatedBy, now, *authorityOperation); err != nil {
		return MigrationRollbackReport{}, err
	}
	sequence, err := q.GetNextMigrationRollbackEventSequence(ctx, db.GetNextMigrationRollbackEventSequenceParams{ProjectID: projectID, MigrationBatchID: batchID})
	if err != nil {
		return MigrationRollbackReport{}, err
	}
	if _, err := q.AppendMigrationRollbackEvent(ctx, db.AppendMigrationRollbackEventParams{ID: uuid.New(), ProjectID: projectID, MigrationBatchID: batchID, EventSequence: sequence, EventType: "rollback_completed", RollbackEligibility: "eligible", CutoverPoint: "rollback", Reason: "versioned inverse projection completed", EventSnapshot: mustJSON(map[string]any{"batch_id": batchID, "ledger_operations": len(ledger)}), EventVersion: "site-fix-migration-rollback/v1", OccurredAt: pgtime(now), RolledBackAt: pgtime(now)}); err != nil {
		return MigrationRollbackReport{}, err
	}
	if _, err := q.ReleaseProductWriterFence(ctx, db.ReleaseProductWriterFenceParams{ProjectID: projectID, Product: "doctor", FenceToken: pgUUID(fenceToken)}); err != nil {
		return MigrationRollbackReport{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return MigrationRollbackReport{}, err
	}
	return MigrationRollbackReport{BatchID: batchID, ProjectID: projectID, RolledBack: true, WriterAuthority: "legacy"}, nil
}

func (s *PostgresMigrationStore) Report(ctx context.Context, projectID, batchID uuid.UUID) (MigrationBatchReport, error) {
	q := db.New(s.pool)
	batch, err := q.GetMigrationBatch(ctx, db.GetMigrationBatchParams{ProjectID: projectID, MigrationBatchID: batchID})
	if err != nil {
		return MigrationBatchReport{}, err
	}
	authority, err := q.GetProductWriterAuthority(ctx, db.GetProductWriterAuthorityParams{ProjectID: projectID, Product: "doctor"})
	if err != nil {
		return MigrationBatchReport{}, err
	}
	ledger, err := q.ListMigrationLedgerForBatch(ctx, db.ListMigrationLedgerForBatchParams{ProjectID: projectID, MigrationBatchID: batchID})
	if err != nil {
		return MigrationBatchReport{}, err
	}
	resultHash, err := currentMigrationSnapshotHash(ctx, q, projectID, ledger)
	if err != nil {
		return MigrationBatchReport{}, err
	}
	reviewRows, err := q.ListMigrationReviewItemsForBatch(ctx, db.ListMigrationReviewItemsForBatchParams{ProjectID: projectID, MigrationBatchID: batchID})
	if err != nil {
		return MigrationBatchReport{}, err
	}
	events, err := q.ListMigrationRollbackEventsForBatch(ctx, db.ListMigrationRollbackEventsForBatchParams{ProjectID: projectID, MigrationBatchID: batchID})
	if err != nil {
		return MigrationBatchReport{}, err
	}
	status := batch.Status
	for _, event := range events {
		if event.EventType == "rollback_completed" {
			status = "rolled_back"
		}
	}
	blockers := []string(nil)
	if status != "rolled_back" {
		blockers, err = migrationRollbackBlockers(ctx, q, projectID, ledger)
		if err != nil {
			return MigrationBatchReport{}, err
		}
	}
	var source MigrationDryRunReport
	_ = json.Unmarshal(batch.SourceSnapshot, &source)
	ledgerSummary := make([]MigrationLedgerSummary, 0, len(ledger))
	var invariants MigrationInvariantReport
	for _, operation := range ledger {
		id := uuid.Nil
		if operation.CanonicalObjectID.Valid {
			id = operation.CanonicalObjectID.Bytes
		}
		ledgerSummary = append(ledgerSummary, MigrationLedgerSummary{SequenceNumber: operation.SequenceNumber, CanonicalObjectType: operation.CanonicalObjectType, CanonicalObjectID: id, Operation: operation.Operation, OperationVersion: operation.OperationVersion, InverseOperationVersion: operation.InverseOperationVersion, AfterHash: operation.AfterHash})
		if operation.CanonicalObjectType == "migration_invariant_report" {
			_ = json.Unmarshal(operation.AfterSnapshot, &invariants)
		}
	}
	reviewSummary := make([]MigrationReviewSummary, 0, len(reviewRows))
	for _, item := range reviewRows {
		reviewSummary = append(reviewSummary, MigrationReviewSummary{ID: item.ID, SourceObjectType: item.SourceObjectType, SourceObjectID: item.SourceObjectID, ReasonCode: item.ReasonCode, Status: item.Status})
	}
	return MigrationBatchReport{BatchID: batch.ID, ProjectID: projectID, SourceCount: int(batch.SourceCount), MigratedCount: int(batch.MigratedCount), ArchivedDuplicateCount: int(batch.ArchivedDuplicateCount), ReviewCount: int(batch.ReviewCount), WriterAuthority: authority.WriterAuthority, ResultSnapshotHash: resultHash, Status: status, SchemaVersion: batch.SchemaVersion, RollbackEligible: authority.WriterAuthority == "canonical" && len(blockers) == 0, RollbackBlockers: blockers, Items: source.Items, Ledger: ledgerSummary, ReviewItems: reviewSummary, Invariants: invariants}, nil
}

func currentMigrationSnapshotHash(ctx context.Context, q *db.Queries, projectID uuid.UUID, ledger []db.MigrationLedger) (string, error) {
	type objectSnapshot struct {
		Sequence int32           `json:"sequence"`
		Type     string          `json:"type"`
		ID       uuid.UUID       `json:"id"`
		Snapshot json.RawMessage `json:"snapshot"`
	}
	snapshots := make([]objectSnapshot, 0, len(ledger))
	for _, operation := range ledger {
		if !operation.CanonicalObjectID.Valid {
			continue
		}
		snapshot, err := q.GetMigrationCurrentSnapshot(ctx, db.GetMigrationCurrentSnapshotParams{ObjectType: operation.CanonicalObjectType, ProjectID: projectID, ObjectID: operation.CanonicalObjectID.Bytes})
		if err != nil {
			return "", err
		}
		snapshots = append(snapshots, objectSnapshot{Sequence: operation.SequenceNumber, Type: operation.CanonicalObjectType, ID: operation.CanonicalObjectID.Bytes, Snapshot: snapshot})
	}
	return digestBytes(mustJSON(snapshots)), nil
}

func migrationRollbackBlockers(ctx context.Context, q *db.Queries, projectID uuid.UUID, ledger []db.MigrationLedger) ([]string, error) {
	blockers := make([]string, 0)
	for _, operation := range ledger {
		if operation.OperationVersion != operationVersionSiteFixMigrationV1 || operation.InverseOperationVersion != inverseOperationVersionSiteFixMigrationV1 {
			blockers = append(blockers, fmt.Sprintf("sequence %d has unsupported operation version", operation.SequenceNumber))
			continue
		}
		if !operation.CanonicalObjectID.Valid {
			continue
		}
		current, err := q.GetMigrationCurrentSnapshot(ctx, db.GetMigrationCurrentSnapshotParams{ObjectType: operation.CanonicalObjectType, ProjectID: projectID, ObjectID: operation.CanonicalObjectID.Bytes})
		if err != nil {
			return nil, err
		}
		if currentHash := digestBytes(current); currentHash != operation.AfterHash {
			blockers = append(blockers, fmt.Sprintf("sequence %d %s/%s drifted", operation.SequenceNumber, operation.CanonicalObjectType, operation.CanonicalObjectID.Bytes))
		}
	}
	if len(ledger) > 0 {
		count, err := q.CountMigrationRollbackRelationBlockers(ctx, db.CountMigrationRollbackRelationBlockersParams{ProjectID: projectID, MigrationBatchID: ledger[0].MigrationBatchID})
		if err != nil {
			return nil, err
		}
		if count > 0 {
			blockers = append(blockers, fmt.Sprintf("%d canonical-only relations were created after cutover", count))
		}
	}
	return blockers, nil
}

func applyMigrationInverse(ctx context.Context, q *db.Queries, projectID, batchID, fenceToken uuid.UUID, initiatedBy string, now time.Time, operation db.MigrationLedger) error {
	if operation.OperationVersion != operationVersionSiteFixMigrationV1 || operation.InverseOperationVersion != inverseOperationVersionSiteFixMigrationV1 {
		return ErrMigrationRollbackBlocked
	}
	var inverse struct {
		Operation             string    `json:"operation"`
		ContentActionID       uuid.UUID `json:"content_action_id"`
		SiteFixID             uuid.UUID `json:"site_fix_id"`
		ApplicationKind       string    `json:"application_kind"`
		LegacyDisposition     string    `json:"legacy_disposition"`
		BucketVersion         int64     `json:"bucket_version"`
		ExpectedBucketVersion int64     `json:"expected_bucket_version"`
		CreatedBucket         bool      `json:"created_bucket"`
	}
	if err := json.Unmarshal(operation.InverseOperation, &inverse); err != nil {
		return err
	}
	objectID := operation.CanonicalObjectID.Bytes
	switch inverse.Operation {
	case "restore_content_action", "restore_legacy", "restore_duplicate", "restore_review_pending":
		if operation.CanonicalObjectType == "content_action" {
			_, err := q.RestoreLegacyContentActionFromLedger(ctx, db.RestoreLegacyContentActionFromLedgerParams{LegacyDisposition: inverse.LegacyDisposition, ProjectID: projectID, ObjectID: objectID, MigrationBatchID: pgUUID(batchID)})
			return err
		}
		if operation.CanonicalObjectType == "seo_opportunity" {
			_, err := q.RestoreLegacyOpportunityFromLedger(ctx, db.RestoreLegacyOpportunityFromLedgerParams{LegacyDisposition: inverse.LegacyDisposition, ProjectID: projectID, ObjectID: objectID, MigrationBatchID: pgUUID(batchID)})
			return err
		}
		return nil
	case "restore_application_source":
		kind := inverse.ApplicationKind
		if kind == "" {
			kind = "site_fix"
		}
		_, err := q.RestoreLegacyApplicationFromLedger(ctx, db.RestoreLegacyApplicationFromLedgerParams{ContentActionID: pgUUID(inverse.ContentActionID), ApplicationKind: kind, ProjectID: projectID, ObjectID: objectID, SiteFixID: pgUUID(inverse.SiteFixID)})
		return err
	case "tombstone_alias":
		_, err := q.TombstoneLegacyMigrationAlias(ctx, db.TombstoneLegacyMigrationAliasParams{ProjectID: projectID, ObjectID: objectID, MigrationBatchID: batchID})
		return err
	case "tombstone_site_fix":
		_, err := q.TombstoneMigrationSiteFix(ctx, db.TombstoneMigrationSiteFixParams{ProjectID: projectID, ObjectID: objectID, MigrationBatchID: pgUUID(batchID)})
		return err
	case "tombstone_finding":
		_, err := q.TombstoneMigrationFinding(ctx, db.TombstoneMigrationFindingParams{ProjectID: projectID, ObjectID: objectID})
		return err
	case "deactivate_signature":
		_, err := q.DeactivateMigrationSignature(ctx, db.DeactivateMigrationSignatureParams{ProjectID: projectID, ObjectID: objectID})
		return err
	case "restore_bucket":
		if inverse.CreatedBucket {
			rows, err := q.DeleteCreatedMigrationBucket(ctx, db.DeleteCreatedMigrationBucketParams{ProjectID: projectID, ObjectID: objectID, ExpectedBucketVersion: inverse.ExpectedBucketVersion})
			if err != nil {
				return err
			}
			if rows != 1 {
				return ErrMigrationRollbackBlocked
			}
			return nil
		}
		_, err := q.RestoreMigrationBucketVersion(ctx, db.RestoreMigrationBucketVersionParams{BucketVersion: inverse.BucketVersion, ProjectID: projectID, ObjectID: objectID})
		return err
	case "deactivate_migrated_review_memory":
		_, err := q.DeactivateMigrationReviewMemory(ctx, db.DeactivateMigrationReviewMemoryParams{ProjectID: projectID, ObjectID: objectID, MigrationBatchID: pgUUID(batchID)})
		return err
	case "dismiss_migration_review":
		_, err := q.DismissMigrationReviewItem(ctx, db.DismissMigrationReviewItemParams{ResolutionSnapshot: mustJSON(map[string]any{"reason": "migration_rolled_back"}), ResolvedBy: &initiatedBy, ResolvedAt: pgtime(now), ProjectID: projectID, ObjectID: objectID})
		return err
	case "restore_authority":
		_, err := q.SwitchProductWriterAuthority(ctx, db.SwitchProductWriterAuthorityParams{WriterAuthority: "legacy", AuthorityChangedAt: pgtime(now), ProjectID: projectID, Product: "doctor", FenceToken: pgUUID(fenceToken), ExpectedWriterAuthority: "canonical"})
		return err
	case "retain_tombstone":
		return nil
	default:
		return fmt.Errorf("unsupported migration inverse %q", inverse.Operation)
	}
}

func migrationInverseOperationName(raw json.RawMessage) string {
	var inverse struct {
		Operation string `json:"operation"`
	}
	_ = json.Unmarshal(raw, &inverse)
	return inverse.Operation
}

func isRetryableMigrationTx(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "40001" || pgErr.Code == "40P01")
}

func waitMigrationRetry(ctx context.Context, attempt int) error {
	timer := time.NewTimer(time.Duration(attempt+1) * 20 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func mustJSON(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}
func pgUUID(value uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: value, Valid: value != uuid.Nil} }
func pgtime(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: !value.IsZero()}
}
