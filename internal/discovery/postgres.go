package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type PostgresRepository struct {
	q *db.Queries
}

func NewPostgresRepository(q *db.Queries) *PostgresRepository {
	return &PostgresRepository{q: q}
}

func (r *PostgresRepository) CreateRun(ctx context.Context, projectID uuid.UUID) (Report, error) {
	if r == nil || r.q == nil {
		return Report{}, fmt.Errorf("database unavailable")
	}
	run, err := r.q.CreateDiscoveryShadowRun(ctx, db.CreateDiscoveryShadowRunParams{
		ProjectID:              projectID,
		CandidateSchemaVersion: CandidateSchemaVersionV1,
		SignatureVersion:       SignatureVersionV1,
	})
	if err != nil {
		return Report{}, err
	}
	return reportFromDB(run), nil
}

func (r *PostgresRepository) ListDoctorFindings(ctx context.Context, projectID uuid.UUID) ([]db.SeoDoctorFinding, error) {
	return r.q.ListActiveDoctorFindingsForDiscoveryShadow(ctx, projectID)
}

func (r *PostgresRepository) ListOpportunities(ctx context.Context, projectID uuid.UUID) ([]db.SeoOpportunity, error) {
	return r.q.ListActiveSEOOpportunitiesForDiscoveryShadow(ctx, projectID)
}

func (r *PostgresRepository) SaveCandidate(ctx context.Context, runID uuid.UUID, candidate Candidate, identity *Identity) (uuid.UUID, error) {
	confidence, err := numericFromFloat(candidate.Confidence)
	if err != nil {
		return uuid.Nil, err
	}
	targets, err := json.Marshal(normalizeTargetSet(candidate.NormalizedTargetSet))
	if err != nil {
		return uuid.Nil, err
	}
	mutations, err := json.Marshal(candidate.ProposedMutations)
	if err != nil {
		return uuid.Nil, err
	}
	topics, err := json.Marshal(normalizeTokenSet(candidate.TopicEntityIdentity))
	if err != nil {
		return uuid.Nil, err
	}
	audience, err := json.Marshal(normalizeTokenSet(candidate.AudienceIdentity))
	if err != nil {
		return uuid.Nil, err
	}
	evidenceIDs, err := json.Marshal(normalizeExactSet(candidate.EvidenceIDs))
	if err != nil {
		return uuid.Nil, err
	}
	bucketKeys := json.RawMessage(`[]`)
	var exactHash *string
	var signaturePayload []byte
	if identity != nil {
		exactHash = stringPtr(identity.ExactSignatureHash)
		signaturePayload = identity.SignaturePayload
		bucketKeys, err = json.Marshal(identity.ConflictBucketKeys)
		if err != nil {
			return uuid.Nil, err
		}
	}
	row, err := r.q.UpsertDiscoveryCandidate(ctx, db.UpsertDiscoveryCandidateParams{
		ProjectID:               candidate.ProjectID,
		ShadowRunID:             runID,
		SourceKind:              string(candidate.SourceKind),
		SourceObjectType:        candidate.SourceObjectType,
		SourceObjectID:          candidate.SourceObjectID,
		TargetKind:              candidate.TargetKind,
		NormalizedTargetSet:     targets,
		IssueOrHypothesisFamily: candidate.IssueOrHypothesisFamily,
		ChangeFamily:            candidate.ChangeFamily,
		ProposedMutations:       mutations,
		ArtifactIntent:          string(candidate.ArtifactIntent),
		IntendedSlugOrCanonical: optionalString(candidate.IntendedSlugOrCanonical),
		TopicEntityIdentity:     topics,
		AudienceIdentity:        audience,
		PrimarySuccessMetric:    candidate.PrimarySuccessMetric,
		VerificationMode:        string(candidate.VerificationMode),
		EvidenceIds:             evidenceIDs,
		EvidenceFingerprint:     candidate.EvidenceFingerprint,
		SuggestedOwner:          string(candidate.SuggestedOwner),
		Confidence:              confidence,
		CandidateSchemaVersion:  candidate.CandidateSchemaVersion,
		Status:                  string(candidate.Status),
		HoldReason:              optionalString(candidate.HoldReason),
		ExactSignatureHash:      exactHash,
		SignaturePayload:        signaturePayload,
		ConflictBucketKeys:      bucketKeys,
	})
	if err != nil {
		return uuid.Nil, err
	}
	if identity == nil {
		if err := r.q.DeleteShadowWorkSignatureForCandidate(ctx, db.DeleteShadowWorkSignatureForCandidateParams{
			ProjectID:   candidate.ProjectID,
			CandidateID: row.ID,
		}); err != nil {
			return uuid.Nil, err
		}
	}
	return row.ID, nil
}

func (r *PostgresRepository) SaveShadowSignature(ctx context.Context, signature ShadowSignature) error {
	for _, bucketKey := range signature.ConflictBucketKeys {
		if _, err := r.q.EnsureWorkConflictBucket(ctx, db.EnsureWorkConflictBucketParams{
			ProjectID: signature.ProjectID,
			BucketKey: bucketKey,
		}); err != nil {
			return err
		}
	}
	bucketKeys, err := json.Marshal(signature.ConflictBucketKeys)
	if err != nil {
		return err
	}
	owner := string(signature.Owner)
	_, err = r.q.UpsertShadowWorkSignature(ctx, db.UpsertShadowWorkSignatureParams{
		ProjectID:          signature.ProjectID,
		CandidateID:        signature.CandidateID,
		ShadowRunID:        signature.RunID,
		ExactSignatureHash: signature.ExactSignatureHash,
		SignaturePayload:   signature.SignaturePayload,
		ConflictBucketKeys: bucketKeys,
		SignatureVersion:   signature.SignatureVersion,
		Owner:              &owner,
		SourceObjectType:   signature.SourceObjectType,
		SourceObjectID:     signature.SourceObjectID,
	})
	return err
}

func (r *PostgresRepository) CompleteRun(ctx context.Context, report Report) (Report, error) {
	run, err := r.q.CompleteDiscoveryShadowRun(ctx, db.CompleteDiscoveryShadowRunParams{
		DoctorCandidates:       int32(report.DoctorCandidates),
		OpportunityCandidates:  int32(report.OpportunityCandidates),
		IdentityReady:          int32(report.IdentityReady),
		NeedsSpecification:     int32(report.NeedsSpecification),
		ExactDuplicateGroups:   int32(report.ExactDuplicateGroups),
		PossibleConflictGroups: int32(report.PossibleConflictGroups),
		ID:                     report.RunID,
		ProjectID:              report.ProjectID,
	})
	if err != nil {
		return Report{}, err
	}
	return reportFromDB(run), nil
}

func (r *PostgresRepository) FailRun(ctx context.Context, report Report, runErr error) error {
	message := runErr.Error()
	_, err := r.q.FailDiscoveryShadowRun(ctx, db.FailDiscoveryShadowRunParams{
		Error:     &message,
		ID:        report.RunID,
		ProjectID: report.ProjectID,
	})
	return err
}

func (r *PostgresRepository) LatestRun(ctx context.Context, projectID uuid.UUID) (Report, error) {
	run, err := r.q.GetLatestDiscoveryShadowRun(ctx, projectID)
	if err != nil {
		return Report{}, err
	}
	return reportFromDB(run), nil
}

func reportFromDB(run db.DiscoveryShadowRun) Report {
	report := Report{
		RunID:                  run.ID,
		ProjectID:              run.ProjectID,
		Mode:                   run.Mode,
		Status:                 run.Status,
		DoctorCandidates:       int(run.DoctorCandidates),
		OpportunityCandidates:  int(run.OpportunityCandidates),
		IdentityReady:          int(run.IdentityReady),
		NeedsSpecification:     int(run.NeedsSpecification),
		ExactDuplicateGroups:   int(run.ExactDuplicateGroups),
		PossibleConflictGroups: int(run.PossibleConflictGroups),
		CreatedAt:              timestamp(run.CreatedAt),
		FinishedAt:             timestamp(run.FinishedAt),
	}
	if run.Error != nil {
		report.Error = *run.Error
	}
	return report
}

func numericFromFloat(value float64) (pgtype.Numeric, error) {
	var numeric pgtype.Numeric
	if err := numeric.Scan(fmt.Sprintf("%.4f", value)); err != nil {
		return pgtype.Numeric{}, fmt.Errorf("convert confidence: %w", err)
	}
	return numeric, nil
}

func timestamp(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func stringPtr(value string) *string { return &value }
