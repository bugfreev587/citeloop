package sitefix

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/discovery"
	"github.com/google/uuid"
)

const (
	MigrationDispositionMigrate               = "migrate"
	MigrationDispositionArchiveDuplicate      = "archive_duplicate"
	MigrationDispositionReview                = "migration_review"
	migrationSchemaVersion                    = "site-fix-migration/v1"
	operationVersionSiteFixMigrationV1        = "site-fix-migration/v1"
	inverseOperationVersionSiteFixMigrationV1 = "site-fix-migration-inverse/v1"
)

var (
	ErrMigrationSnapshotDrift   = errors.New("migration source snapshot drifted")
	ErrMigrationRollbackBlocked = errors.New("migration rollback blocked: canonical writes cannot be losslessly inverse-projected")
)

// LegacyTechnicalAction is the owner-neutral migration input. It deliberately
// contains only persisted legacy identity/provenance and never an inferred owner.
type LegacyTechnicalAction struct {
	ProjectID                    uuid.UUID                     `json:"project_id"`
	OpportunityID                uuid.UUID                     `json:"opportunity_id"`
	ActionID                     uuid.UUID                     `json:"action_id"`
	DoctorFindingID              uuid.UUID                     `json:"doctor_finding_id,omitempty"`
	TargetURL                    string                        `json:"target_url"`
	ChangeFamily                 string                        `json:"change_family"`
	OpportunityType              string                        `json:"opportunity_type"`
	OpportunityEvidence          json.RawMessage               `json:"opportunity_evidence"`
	OpportunityQuery             *string                       `json:"opportunity_query,omitempty"`
	OpportunityRecommendedAction *string                       `json:"opportunity_recommended_action,omitempty"`
	Status                       string                        `json:"status"`
	Evidence                     json.RawMessage               `json:"evidence"`
	ApplicationIDs               []uuid.UUID                   `json:"application_ids"`
	ReviewDecision               string                        `json:"review_decision,omitempty"`
	ReviewStateID                uuid.UUID                     `json:"review_state_id,omitempty"`
	ReviewSnoozedUntil           *time.Time                    `json:"review_snoozed_until,omitempty"`
	ReviewDecidedBy              string                        `json:"review_decided_by,omitempty"`
	ReviewDecidedAt              time.Time                     `json:"review_decided_at,omitempty"`
	ReviewEvidenceFingerprint    string                        `json:"review_evidence_fingerprint,omitempty"`
	ReviewMaterialChangeMetadata json.RawMessage               `json:"review_material_change_metadata,omitempty"`
	ReviewHistory                []LegacyReviewState           `json:"review_history,omitempty"`
	CreatedAt                    time.Time                     `json:"created_at"`
	UpdatedAt                    time.Time                     `json:"updated_at"`
	ApprovedAt                   *time.Time                    `json:"approved_at,omitempty"`
	ApprovedBy                   string                        `json:"approved_by,omitempty"`
	ApprovalSource               string                        `json:"approval_source,omitempty"`
	InputSnapshot                json.RawMessage               `json:"input_snapshot,omitempty"`
	OutputSnapshot               json.RawMessage               `json:"output_snapshot,omitempty"`
	DiffSnapshot                 json.RawMessage               `json:"diff_snapshot,omitempty"`
	ApplicationStatus            string                        `json:"application_status,omitempty"`
	HasPullRequest               bool                          `json:"has_pull_request,omitempty"`
	DeploymentObserved           bool                          `json:"deployment_observed,omitempty"`
	VerificationPassed           bool                          `json:"verification_passed,omitempty"`
	ActionLegacyDisposition      string                        `json:"action_legacy_disposition,omitempty"`
	OpportunityLegacyDisposition string                        `json:"opportunity_legacy_disposition,omitempty"`
	ActiveWork                   []MigrationActiveWork         `json:"active_work,omitempty"`
	ActiveReviewMemory           []MigrationActiveReviewMemory `json:"active_review_memory,omitempty"`
	BucketVersions               map[string]int64              `json:"bucket_versions,omitempty"`
}

type MigrationActiveReviewMemory struct {
	ID                  uuid.UUID  `json:"id"`
	ExactSignatureHash  string     `json:"exact_signature_hash"`
	SemanticFingerprint string     `json:"semantic_fingerprint,omitempty"`
	ConflictBucketKeys  []string   `json:"conflict_bucket_keys,omitempty"`
	Decision            string     `json:"decision"`
	EvidenceFingerprint string     `json:"evidence_fingerprint,omitempty"`
	SnoozedUntil        *time.Time `json:"snoozed_until,omitempty"`
	ViaAlias            bool       `json:"via_alias,omitempty"`
}

type LegacyReviewState struct {
	ID                     uuid.UUID       `json:"id"`
	Decision               string          `json:"decision"`
	DecidedBy              string          `json:"decided_by,omitempty"`
	DecidedAt              *time.Time      `json:"decided_at,omitempty"`
	SnoozedUntil           *time.Time      `json:"snoozed_until,omitempty"`
	EvidenceFingerprint    string          `json:"evidence_fingerprint,omitempty"`
	MaterialChangeMetadata json.RawMessage `json:"material_change_metadata,omitempty"`
}

type MigrationActiveWork struct {
	WorkSignatureID        uuid.UUID `json:"work_signature_id"`
	ExactSignatureHash     string    `json:"exact_signature_hash"`
	ConflictBucketKeys     []string  `json:"conflict_bucket_keys"`
	Owner                  string    `json:"owner"`
	WorkType               string    `json:"work_type"`
	WorkID                 uuid.UUID `json:"work_id"`
	Active                 bool      `json:"active"`
	ActiveApplicationCount int32     `json:"active_application_count,omitempty"`
	Planned                bool      `json:"planned,omitempty"`
}

type MigrationClassification struct {
	LegacyOpportunityID    uuid.UUID       `json:"legacy_opportunity_id"`
	LegacyActionID         uuid.UUID       `json:"legacy_action_id"`
	DoctorFindingID        uuid.UUID       `json:"doctor_finding_id,omitempty"`
	SiteFixID              uuid.UUID       `json:"site_fix_id,omitempty"`
	CanonicalActionID      uuid.UUID       `json:"canonical_action_id,omitempty"`
	Disposition            string          `json:"disposition"`
	ReasonCode             string          `json:"reason_code,omitempty"`
	IdentityHash           string          `json:"identity_hash,omitempty"`
	SignaturePayload       json.RawMessage `json:"signature_payload,omitempty"`
	ConflictBucketKeys     []string        `json:"conflict_bucket_keys,omitempty"`
	CandidateSchemaVersion string          `json:"candidate_schema_version,omitempty"`
	SignatureVersion       string          `json:"signature_version,omitempty"`
	ExistingSiteFixID      uuid.UUID       `json:"existing_site_fix_id,omitempty"`
	CanonicalSourceID      uuid.UUID       `json:"canonical_source_id,omitempty"`
}

type MigrationDryRunReport struct {
	ProjectID              uuid.UUID                 `json:"project_id"`
	WriterAuthority        string                    `json:"writer_authority"`
	SnapshotHash           string                    `json:"snapshot_hash"`
	SourceCount            int                       `json:"source_count"`
	MigratedCount          int                       `json:"migrated_count"`
	ArchivedDuplicateCount int                       `json:"archived_duplicate_count"`
	ReviewCount            int                       `json:"review_count"`
	Items                  []MigrationClassification `json:"items"`
}

type MigrationPlan struct {
	MigrationDryRunReport
	BatchID     uuid.UUID `json:"batch_id"`
	InitiatedBy string    `json:"initiated_by"`
}

type MigrationBatchReport struct {
	BatchID                uuid.UUID                 `json:"batch_id"`
	ProjectID              uuid.UUID                 `json:"project_id"`
	SourceCount            int                       `json:"source_count"`
	MigratedCount          int                       `json:"migrated_count"`
	ArchivedDuplicateCount int                       `json:"archived_duplicate_count"`
	ReviewCount            int                       `json:"review_count"`
	WriterAuthority        string                    `json:"writer_authority"`
	ResultSnapshotHash     string                    `json:"result_snapshot_hash"`
	Status                 string                    `json:"status,omitempty"`
	SchemaVersion          string                    `json:"schema_version,omitempty"`
	RollbackEligible       bool                      `json:"rollback_eligible"`
	RollbackBlockers       []string                  `json:"rollback_blockers,omitempty"`
	Items                  []MigrationClassification `json:"items,omitempty"`
	Ledger                 []MigrationLedgerSummary  `json:"ledger,omitempty"`
	ReviewItems            []MigrationReviewSummary  `json:"review_items,omitempty"`
	Invariants             MigrationInvariantReport  `json:"invariants"`
}

type MigrationInvariantReport struct {
	ExpectedAliases               int  `json:"expected_aliases"`
	ActiveAliases                 int  `json:"active_aliases"`
	ExpectedRepointedApplications int  `json:"expected_repointed_applications"`
	RepointedApplications         int  `json:"repointed_applications"`
	ExpectedSourceLedgers         int  `json:"expected_source_ledgers"`
	SourceLedgers                 int  `json:"source_ledgers"`
	ExpectedSiteFixes             int  `json:"expected_site_fixes"`
	SiteFixes                     int  `json:"site_fixes"`
	ExpectedReviewItems           int  `json:"expected_review_items"`
	ReviewItems                   int  `json:"review_items"`
	ExpectedReviewMemories        int  `json:"expected_review_memories"`
	ReviewMemories                int  `json:"review_memories"`
	InvalidApplicationSources     int  `json:"invalid_application_sources"`
	UnledgeredObjects             int  `json:"unledgered_objects"`
	Passed                        bool `json:"passed"`
}

type MigrationLedgerSummary struct {
	SequenceNumber          int32     `json:"sequence_number"`
	CanonicalObjectType     string    `json:"canonical_object_type"`
	CanonicalObjectID       uuid.UUID `json:"canonical_object_id,omitempty"`
	Operation               string    `json:"operation"`
	OperationVersion        string    `json:"operation_version"`
	InverseOperationVersion string    `json:"inverse_operation_version"`
	AfterHash               string    `json:"after_hash"`
}

type MigrationReviewSummary struct {
	ID               uuid.UUID `json:"id"`
	SourceObjectType string    `json:"source_object_type"`
	SourceObjectID   uuid.UUID `json:"source_object_id"`
	ReasonCode       string    `json:"reason_code"`
	Status           string    `json:"status"`
}

type MigrationRollbackReport struct {
	BatchID         uuid.UUID `json:"batch_id"`
	ProjectID       uuid.UUID `json:"project_id"`
	RolledBack      bool      `json:"rolled_back"`
	WriterAuthority string    `json:"writer_authority"`
}

type MigrationLedgerOperation struct {
	OperationVersion        string          `json:"operation_version"`
	InverseOperationVersion string          `json:"inverse_operation_version"`
	InverseOperation        json.RawMessage `json:"inverse_operation"`
}

// MigrationFixtureState is exported only as a small invariant view useful to
// repository contract tests; production persistence remains database-owned.
type MigrationFixtureState struct {
	FindingID         uuid.UUID
	SignatureID       uuid.UUID
	SiteFixID         uuid.UUID
	Status            string
	LegacyReadOnly    bool
	AliasState        string
	ApplicationSource string
}

type MigrationStore interface {
	Snapshot(context.Context, uuid.UUID) ([]LegacyTechnicalAction, string, error)
	Apply(context.Context, MigrationPlan) (MigrationBatchReport, error)
	Rollback(context.Context, uuid.UUID, uuid.UUID, string, string) (MigrationRollbackReport, error)
	Report(context.Context, uuid.UUID, uuid.UUID) (MigrationBatchReport, error)
}

type MigrationService struct{ store MigrationStore }

func NewMigrationService(store MigrationStore) *MigrationService {
	return &MigrationService{store: store}
}

func (s *MigrationService) DryRun(ctx context.Context, projectID uuid.UUID, _ string) (MigrationDryRunReport, error) {
	if s == nil || s.store == nil {
		return MigrationDryRunReport{}, errors.New("migration store is required")
	}
	rows, authority, err := s.store.Snapshot(ctx, projectID)
	if err != nil {
		return MigrationDryRunReport{}, err
	}
	return ClassifyLegacyTechnicalActions(projectID, authority, rows)
}

func (s *MigrationService) Apply(ctx context.Context, projectID uuid.UUID, expectedSnapshotHash, initiatedBy string) (MigrationBatchReport, error) {
	dry, err := s.DryRun(ctx, projectID, initiatedBy)
	if err != nil {
		return MigrationBatchReport{}, err
	}
	if expectedSnapshotHash == "" || expectedSnapshotHash != dry.SnapshotHash {
		return MigrationBatchReport{}, ErrMigrationSnapshotDrift
	}
	return s.store.Apply(ctx, MigrationPlan{MigrationDryRunReport: dry, BatchID: uuid.New(), InitiatedBy: initiatedBy})
}

func (s *MigrationService) Rollback(ctx context.Context, projectID, batchID uuid.UUID, expectedSnapshotHash, initiatedBy string) (MigrationRollbackReport, error) {
	if expectedSnapshotHash == "" {
		return MigrationRollbackReport{}, ErrMigrationSnapshotDrift
	}
	return s.store.Rollback(ctx, projectID, batchID, expectedSnapshotHash, initiatedBy)
}

func (s *MigrationService) Report(ctx context.Context, projectID, batchID uuid.UUID) (MigrationBatchReport, error) {
	return s.store.Report(ctx, projectID, batchID)
}

func ClassifyLegacyTechnicalActions(projectID uuid.UUID, authority string, rows []LegacyTechnicalAction) (MigrationDryRunReport, error) {
	if projectID == uuid.Nil {
		return MigrationDryRunReport{}, errors.New("project id is required")
	}
	if authority != "legacy" && authority != "canonical" {
		return MigrationDryRunReport{}, fmt.Errorf("unsupported writer authority %q", authority)
	}
	report := MigrationDryRunReport{ProjectID: projectID, WriterAuthority: authority, SourceCount: len(rows), Items: make([]MigrationClassification, 0, len(rows))}
	type projected struct {
		candidate discovery.Candidate
		identity  discovery.Identity
		reason    string
	}
	projections := make([]projected, len(rows))
	opportunityIdentities := make(map[uuid.UUID]map[string]struct{}, len(rows))
	identityActiveApplicationCount := make(map[string]int, len(rows))
	identityLegacyReviewStates := make(map[string]map[string]struct{}, len(rows))
	bucketIdentities := make(map[string]map[string]struct{}, len(rows))
	for index, row := range rows {
		candidate, identity, reason := canonicalLegacyMigrationIdentity(row)
		projections[index] = projected{candidate: candidate, identity: identity, reason: reason}
		if reason != "" {
			continue
		}
		if opportunityIdentities[row.OpportunityID] == nil {
			opportunityIdentities[row.OpportunityID] = map[string]struct{}{}
		}
		opportunityIdentities[row.OpportunityID][identity.ExactSignatureHash] = struct{}{}
		if len(row.ApplicationIDs) > 0 {
			identityActiveApplicationCount[identity.ExactSignatureHash] += len(row.ApplicationIDs)
		}
		if strings.TrimSpace(row.ReviewDecision) != "" {
			if identityLegacyReviewStates[identity.ExactSignatureHash] == nil {
				identityLegacyReviewStates[identity.ExactSignatureHash] = map[string]struct{}{}
			}
			identityLegacyReviewStates[identity.ExactSignatureHash][legacyReviewStateKey(row)] = struct{}{}
		}
		for _, bucket := range identity.ConflictBucketKeys {
			if bucketIdentities[bucket] == nil {
				bucketIdentities[bucket] = map[string]struct{}{}
			}
			bucketIdentities[bucket][identity.ExactSignatureHash] = struct{}{}
		}
	}
	winners := make(map[string]uuid.UUID, len(rows))
	for _, row := range rows {
		_ = row
	}
	for index, row := range rows {
		if row.ProjectID != projectID || row.OpportunityID == uuid.Nil {
			return MigrationDryRunReport{}, errors.New("legacy migration row has invalid project provenance")
		}
		item := MigrationClassification{LegacyOpportunityID: row.OpportunityID, LegacyActionID: row.ActionID, DoctorFindingID: row.DoctorFindingID}
		projection := projections[index]
		if projection.reason != "" {
			if projection.identity.ExactSignatureHash != "" {
				item.IdentityHash = projection.identity.ExactSignatureHash
				item.SignaturePayload = projection.identity.SignaturePayload
				item.ConflictBucketKeys = projection.identity.ConflictBucketKeys
				item.CandidateSchemaVersion = projection.candidate.CandidateSchemaVersion
				item.SignatureVersion = projection.candidate.SignatureVersion
			}
			item.Disposition, item.ReasonCode = MigrationDispositionReview, projection.reason
			report.ReviewCount++
			report.Items = append(report.Items, item)
			continue
		}
		item.IdentityHash = projection.identity.ExactSignatureHash
		item.SignaturePayload = projection.identity.SignaturePayload
		item.ConflictBucketKeys = projection.identity.ConflictBucketKeys
		item.CandidateSchemaVersion = projection.candidate.CandidateSchemaVersion
		item.SignatureVersion = projection.candidate.SignatureVersion
		if len(opportunityIdentities[row.OpportunityID]) > 1 {
			item.Disposition, item.ReasonCode = MigrationDispositionReview, "ambiguous_legacy_group"
			report.ReviewCount++
			report.Items = append(report.Items, item)
			continue
		}
		if identityActiveApplicationCount[item.IdentityHash] > 1 {
			item.Disposition, item.ReasonCode = MigrationDispositionReview, "duplicate_active_applications"
			report.ReviewCount++
			report.Items = append(report.Items, item)
			continue
		}
		if len(row.ApplicationIDs) > 0 && isLegacySuppressingReviewDecision(row.ReviewDecision) {
			item.Disposition, item.ReasonCode = MigrationDispositionReview, "review_decision_application_conflict"
			report.ReviewCount++
			report.Items = append(report.Items, item)
			continue
		}
		if len(identityLegacyReviewStates[item.IdentityHash]) > 1 {
			item.Disposition, item.ReasonCode = MigrationDispositionReview, "conflicting_legacy_review_decisions"
			report.ReviewCount++
			report.Items = append(report.Items, item)
			continue
		}
		reviewMemoryReason := ""
		for _, memory := range row.ActiveReviewMemory {
			exactMemory := memory.ExactSignatureHash == item.IdentityHash
			if exactMemory {
				if strings.TrimSpace(row.ReviewDecision) != "" && memory.Decision != row.ReviewDecision {
					reviewMemoryReason = "review_memory_conflict"
				} else if migrationReviewMemoryApplies(memory, projection.candidate) {
					reviewMemoryReason = "review_memory_applied"
				} else {
					reviewMemoryReason = "review_memory_bucket_overlap"
				}
				break
			}
			if migrationBucketsOverlap(memory.ConflictBucketKeys, item.ConflictBucketKeys) {
				reviewMemoryReason = "review_memory_bucket_overlap"
			}
		}
		if reviewMemoryReason != "" {
			item.Disposition, item.ReasonCode = MigrationDispositionReview, reviewMemoryReason
			report.ReviewCount++
			report.Items = append(report.Items, item)
			continue
		}
		batchBucketConflict := false
		for _, bucket := range item.ConflictBucketKeys {
			if len(bucketIdentities[bucket]) > 1 {
				batchBucketConflict = true
				break
			}
		}
		if batchBucketConflict {
			item.Disposition, item.ReasonCode = MigrationDispositionReview, "migration_batch_bucket_collision"
			report.ReviewCount++
			report.Items = append(report.Items, item)
			continue
		}
		exactMatches, bucketConflict := activeWorkCollisions(row.ActiveWork, projection.identity)
		if len(exactMatches) > 0 {
			if exactMatches[0].Planned {
				item.Disposition, item.ReasonCode = MigrationDispositionReview, "planned_work_collision"
				report.ReviewCount++
			} else if len(exactMatches) == 1 && exactMatches[0].Owner == string(discovery.OwnerDoctor) && exactMatches[0].WorkType == "site_fix" && exactMatches[0].WorkID != uuid.Nil {
				if len(row.ApplicationIDs) > 0 && exactMatches[0].ActiveApplicationCount > 0 {
					item.Disposition, item.ReasonCode = MigrationDispositionReview, "duplicate_active_applications"
					report.ReviewCount++
				} else {
					item.Disposition, item.ExistingSiteFixID = MigrationDispositionArchiveDuplicate, exactMatches[0].WorkID
					report.ArchivedDuplicateCount++
				}
			} else {
				item.Disposition, item.ReasonCode = MigrationDispositionReview, "active_cross_line_collision"
				report.ReviewCount++
			}
			report.Items = append(report.Items, item)
			continue
		}
		if bucketConflict {
			item.Disposition, item.ReasonCode = MigrationDispositionReview, "active_bucket_collision"
			report.ReviewCount++
			report.Items = append(report.Items, item)
			continue
		}
		sourceID := legacyMigrationSourceID(row)
		if winner, duplicate := winners[item.IdentityHash]; duplicate {
			item.Disposition, item.CanonicalSourceID = MigrationDispositionArchiveDuplicate, winner
			if row.ActionID != uuid.Nil {
				item.CanonicalActionID = winner
			}
			report.ArchivedDuplicateCount++
		} else {
			item.Disposition = MigrationDispositionMigrate
			winners[item.IdentityHash] = sourceID
			report.MigratedCount++
		}
		report.Items = append(report.Items, item)
	}
	if report.SourceCount != report.MigratedCount+report.ArchivedDuplicateCount+report.ReviewCount {
		return MigrationDryRunReport{}, errors.New("legacy row conservation failed")
	}
	snapshot := struct {
		SchemaVersion string                    `json:"schema_version"`
		ProjectID     uuid.UUID                 `json:"project_id"`
		Authority     string                    `json:"authority"`
		Rows          []LegacyTechnicalAction   `json:"rows"`
		Items         []MigrationClassification `json:"items"`
	}{migrationSchemaVersion, projectID, authority, rows, report.Items}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return MigrationDryRunReport{}, err
	}
	report.SnapshotHash = digestBytes(raw)
	return report, nil
}

func legacyReviewStateKey(row LegacyTechnicalAction) string {
	snoozed := ""
	if row.ReviewSnoozedUntil != nil {
		snoozed = row.ReviewSnoozedUntil.UTC().Format(time.RFC3339Nano)
	}
	return strings.TrimSpace(row.ReviewDecision) + "|" + snoozed + "|" + digestBytes(row.ReviewMaterialChangeMetadata)
}

func isLegacySuppressingReviewDecision(decision string) bool {
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "dismissed", "snoozed", "watching":
		return true
	default:
		return false
	}
}

func canonicalLegacyMigrationIdentity(row LegacyTechnicalAction) (discovery.Candidate, discovery.Identity, string) {
	if normalizeMigrationTarget(row.TargetURL) == "" {
		return discovery.Candidate{}, discovery.Identity{}, "ambiguous_target"
	}
	evidence := row.OpportunityEvidence
	if len(evidence) == 0 {
		evidence = row.Evidence
	}
	if _, meaningful := canonicalEvidence(evidence); !meaningful {
		return discovery.Candidate{}, discovery.Identity{}, "insufficient_evidence"
	}
	opportunityType := strings.TrimSpace(row.OpportunityType)
	if opportunityType == "" {
		opportunityType = canonicalLegacyOpportunityType(row.ChangeFamily, evidence)
	}
	projected := discovery.ProjectSEOOpportunity(db.SeoOpportunity{ID: row.OpportunityID, ProjectID: row.ProjectID,
		Type: opportunityType, NormalizedPageUrl: row.TargetURL, Query: row.OpportunityQuery,
		Evidence: evidence, RecommendedAction: row.OpportunityRecommendedAction})
	if projected.Status != discovery.StatusIdentityReady {
		return projected, discovery.Identity{}, "needs_specification"
	}
	identity, err := discovery.BuildIdentity(projected)
	if err != nil {
		return projected, discovery.Identity{}, "needs_specification"
	}
	if projected.SuggestedOwner != discovery.OwnerDoctor || projected.VerificationMode != discovery.VerificationImmediate || projected.ArtifactIntent != discovery.ArtifactRepairExistingSurface {
		return projected, identity, "owner_or_verification_ambiguous"
	}
	if len(migrationAcceptanceTests(projected, row)) == 0 {
		return projected, identity, "acceptance_criterion_incomplete"
	}
	return projected, identity, ""
}

func canonicalLegacyOpportunityType(family string, evidence json.RawMessage) string {
	family = strings.ToLower(strings.TrimSpace(family))
	switch family {
	case "schema_patch", "schema_gap", "structured_data_missing":
		return "schema_gap"
	case "sitemap_update":
		return "important_page_missing_from_sitemap"
	case "metadata_rewrite":
		return "metadata_title"
	case "internal_link_patch", "internal_link_gap":
		return "internal_link_gap"
	case "technical_fix", "technical seo fix task":
		var value map[string]any
		_ = json.Unmarshal(evidence, &value)
		issue, _ := value["issue"].(string)
		issue = strings.ToLower(strings.TrimSpace(issue))
		switch issue {
		case "missing_schema":
			return "schema_gap"
		case "unknown":
			return ""
		default:
			return issue
		}
	default:
		return family
	}
}

func legacyMigrationSourceID(row LegacyTechnicalAction) uuid.UUID {
	if row.ActionID != uuid.Nil {
		return row.ActionID
	}
	return row.OpportunityID
}

func classificationSourceID(item MigrationClassification) uuid.UUID {
	if item.LegacyActionID != uuid.Nil {
		return item.LegacyActionID
	}
	return item.LegacyOpportunityID
}

func activeWorkCollisions(active []MigrationActiveWork, identity discovery.Identity) ([]MigrationActiveWork, bool) {
	var exact []MigrationActiveWork
	bucketConflict := false
	wanted := map[string]struct{}{}
	for _, key := range identity.ConflictBucketKeys {
		wanted[key] = struct{}{}
	}
	for _, work := range active {
		if !work.Active && !work.Planned {
			continue
		}
		if work.ExactSignatureHash == identity.ExactSignatureHash {
			exact = append(exact, work)
			continue
		}
		for _, key := range work.ConflictBucketKeys {
			if _, ok := wanted[key]; ok {
				bucketConflict = true
				break
			}
		}
	}
	return exact, bucketConflict
}

func migrationReviewMemoryApplies(memory MigrationActiveReviewMemory, candidate discovery.Candidate) bool {
	if memory.Decision == "snoozed" {
		return memory.SnoozedUntil != nil && time.Now().UTC().Before(memory.SnoozedUntil.UTC())
	}
	return memory.EvidenceFingerprint != "" && memory.EvidenceFingerprint == candidate.EvidenceFingerprint
}

func migrationBucketsOverlap(left, right []string) bool {
	set := make(map[string]struct{}, len(left))
	for _, key := range left {
		set[key] = struct{}{}
	}
	for _, key := range right {
		if _, ok := set[key]; ok {
			return true
		}
	}
	return false
}

func normalizeMigrationTarget(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String()
}

func canonicalEvidence(raw json.RawMessage) ([]byte, bool) {
	var value any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return nil, false
	}
	obj, ok := value.(map[string]any)
	if !ok || len(obj) == 0 {
		return nil, false
	}
	canonical, err := json.Marshal(obj)
	return canonical, err == nil
}

func digestString(value string) string { return digestBytes([]byte(value)) }
func digestBytes(value []byte) string  { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }
func deterministicMigrationUUID(kind string, id uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(kind+":"+id.String()))
}

func migrationArtifactUUID(kind string, batchID, sourceID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(batchID, []byte(kind+":"+sourceID.String()))
}

func migrationSourceObjectType(row LegacyTechnicalAction) string {
	if row.ActionID != uuid.Nil {
		return "content_action"
	}
	return "seo_opportunity"
}

func nullUUID(value uuid.UUID) any {
	if value == uuid.Nil {
		return nil
	}
	return value
}
func stringPointerValue(value string) *string { return &value }

// stableUUIDs is used when serializing application provenance supplied by SQL.
func stableUUIDs(values []uuid.UUID) []uuid.UUID {
	copyValues := append([]uuid.UUID(nil), values...)
	sort.Slice(copyValues, func(i, j int) bool { return copyValues[i].String() < copyValues[j].String() })
	return copyValues
}
