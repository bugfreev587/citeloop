package sitefix

import (
	"context"
	"fmt"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/discovery"
	"github.com/google/uuid"
)

// CanonicalCandidate is the persisted, owner-neutral input to arbitration.
// Materialization deliberately has no semantic-provider dependency; provider
// comparison remains in arbitration Phase A, outside database transactions.
type CanonicalCandidate struct {
	ID        uuid.UUID
	RunID     uuid.UUID
	Candidate discovery.Candidate
	Identity  discovery.Identity
}

type candidateStore interface {
	EnsureRun(context.Context, uuid.UUID, uuid.UUID, string) error
	SaveCandidate(context.Context, uuid.UUID, discovery.Candidate, discovery.Identity) (uuid.UUID, error)
	EnsureBucket(context.Context, uuid.UUID, string) error
	CompleteRun(context.Context, uuid.UUID, uuid.UUID) error
}

type CandidateMaterializer struct {
	store candidateStore
}

func NewCandidateMaterializer(q *db.Queries) *CandidateMaterializer {
	return newCandidateMaterializer(postgresCandidateStore{q: q})
}

func newCandidateMaterializer(store candidateStore) *CandidateMaterializer {
	return &CandidateMaterializer{store: store}
}

func (m *CandidateMaterializer) Materialize(ctx context.Context, finding db.SeoDoctorFinding) (CanonicalCandidate, error) {
	if m == nil || m.store == nil {
		return CanonicalCandidate{}, fmt.Errorf("candidate store is required")
	}
	if finding.ID == uuid.Nil || finding.ProjectID == uuid.Nil {
		return CanonicalCandidate{}, ErrProjectMismatch
	}
	if finding.FindingKind == "healthy" {
		return CanonicalCandidate{}, ErrHealthyFinding
	}
	candidate := discovery.ProjectDoctorFinding(finding)
	if candidate.Status != discovery.StatusIdentityReady || candidate.SuggestedOwner != discovery.OwnerDoctor {
		return CanonicalCandidate{}, ErrIncompleteCandidate
	}
	identity, err := discovery.BuildIdentity(candidate)
	if err != nil || identity.ExactSignatureHash == "" || len(identity.ConflictBucketKeys) == 0 {
		return CanonicalCandidate{}, fmt.Errorf("%w: %v", ErrIncompleteCandidate, err)
	}
	runID := canonicalRunID(finding.ProjectID, finding.ID)
	if err := m.store.EnsureRun(ctx, runID, finding.ProjectID, "canonical"); err != nil {
		return CanonicalCandidate{}, fmt.Errorf("ensure canonical discovery run: %w", err)
	}
	for _, bucket := range identity.ConflictBucketKeys {
		if err := m.store.EnsureBucket(ctx, finding.ProjectID, bucket); err != nil {
			return CanonicalCandidate{}, fmt.Errorf("materialize conflict bucket %q: %w", bucket, err)
		}
	}
	candidateID, err := m.store.SaveCandidate(ctx, runID, candidate, identity)
	if err != nil {
		return CanonicalCandidate{}, fmt.Errorf("save canonical Doctor candidate: %w", err)
	}
	if candidateID == uuid.Nil {
		return CanonicalCandidate{}, errNilCandidateID
	}
	if err := m.store.CompleteRun(ctx, runID, finding.ProjectID); err != nil {
		return CanonicalCandidate{}, fmt.Errorf("complete canonical discovery run: %w", err)
	}
	return CanonicalCandidate{ID: candidateID, RunID: runID, Candidate: candidate, Identity: identity}, nil
}

var errNilCandidateID = fmt.Errorf("canonical candidate persistence returned a nil id")

func canonicalRunID(projectID, findingID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("citeloop:canonical-doctor-candidate:"+projectID.String()+":"+findingID.String()+":"+discovery.CandidateSchemaVersionV1+":"+discovery.SignatureVersionV1))
}

type postgresCandidateStore struct {
	q *db.Queries
}

func (s postgresCandidateStore) EnsureRun(ctx context.Context, runID, projectID uuid.UUID, mode string) error {
	if s.q == nil {
		return fmt.Errorf("database unavailable")
	}
	if mode != "canonical" {
		return fmt.Errorf("unsupported discovery run mode %q", mode)
	}
	_, err := s.q.EnsureCanonicalDiscoveryRun(ctx, db.EnsureCanonicalDiscoveryRunParams{
		ID: runID, ProjectID: projectID,
		CandidateSchemaVersion: discovery.CandidateSchemaVersionV1,
		SignatureVersion:       discovery.SignatureVersionV1,
	})
	return err
}

func (s postgresCandidateStore) SaveCandidate(ctx context.Context, runID uuid.UUID, candidate discovery.Candidate, identity discovery.Identity) (uuid.UUID, error) {
	if s.q == nil {
		return uuid.Nil, fmt.Errorf("database unavailable")
	}
	return discovery.NewPostgresRepository(s.q).SaveCandidate(ctx, runID, candidate, &identity)
}

func (s postgresCandidateStore) EnsureBucket(ctx context.Context, projectID uuid.UUID, bucket string) error {
	if s.q == nil {
		return fmt.Errorf("database unavailable")
	}
	_, err := s.q.EnsureWorkConflictBucket(ctx, db.EnsureWorkConflictBucketParams{ProjectID: projectID, BucketKey: bucket})
	return err
}

func (s postgresCandidateStore) CompleteRun(ctx context.Context, runID, projectID uuid.UUID) error {
	if s.q == nil {
		return fmt.Errorf("database unavailable")
	}
	_, err := s.q.CompleteCanonicalDiscoveryRun(ctx, db.CompleteCanonicalDiscoveryRunParams{ID: runID, ProjectID: projectID})
	return err
}
