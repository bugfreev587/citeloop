package sitefix

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/discovery"
	"github.com/google/uuid"
)

// CanonicalCandidate is the persisted, owner-neutral input to arbitration.
// Materialization deliberately has no semantic-provider dependency; provider
// comparison remains in arbitration Phase A, outside database transactions.
type CanonicalCandidate struct {
	ID                  uuid.UUID
	RunID               uuid.UUID
	SnapshotFingerprint string
	Candidate           discovery.Candidate
	Identity            discovery.Identity
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
	if !meaningfulJSON(finding.Evidence) || finding.RunID == uuid.Nil {
		return CanonicalCandidate{}, ErrIncompleteCandidate
	}
	candidate := discovery.ProjectDoctorFinding(finding)
	if candidate.Status != discovery.StatusIdentityReady || candidate.SuggestedOwner != discovery.OwnerDoctor || len(candidate.EvidenceIDs) == 0 {
		return CanonicalCandidate{}, ErrIncompleteCandidate
	}
	identity, err := discovery.BuildIdentity(candidate)
	if err != nil || identity.ExactSignatureHash == "" || len(identity.ConflictBucketKeys) == 0 {
		return CanonicalCandidate{}, fmt.Errorf("%w: %v", ErrIncompleteCandidate, err)
	}
	snapshotFingerprint, err := doctorFindingSnapshotFingerprint(finding, candidate, identity)
	if err != nil {
		return CanonicalCandidate{}, fmt.Errorf("fingerprint Doctor candidate snapshot: %w", err)
	}
	runID := canonicalRunID(finding.ProjectID, finding.ID, snapshotFingerprint)
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
	return CanonicalCandidate{
		ID: candidateID, RunID: runID, SnapshotFingerprint: snapshotFingerprint,
		Candidate: candidate, Identity: identity,
	}, nil
}

var errNilCandidateID = fmt.Errorf("canonical candidate persistence returned a nil id")

func canonicalRunID(projectID, findingID uuid.UUID, snapshotFingerprint string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("citeloop:canonical-doctor-candidate:"+projectID.String()+":"+findingID.String()+":"+discovery.CandidateSchemaVersionV1+":"+discovery.SignatureVersionV1+":"+snapshotFingerprint))
}

func doctorFindingSnapshotFingerprint(finding db.SeoDoctorFinding, candidate discovery.Candidate, identity discovery.Identity) (string, error) {
	payload := struct {
		ProjectID             uuid.UUID
		FindingID             uuid.UUID
		FindingRunID          uuid.UUID
		FindingStatus         string
		FindingKind           string
		Evidence              json.RawMessage
		FixIntent             string
		DeveloperInstructions string
		LikelyFilesOrSurfaces json.RawMessage
		AcceptanceTests       json.RawMessage
		Candidate             discovery.Candidate
		ExactSignatureHash    string
		SignaturePayload      json.RawMessage
		ConflictBucketKeys    []string
	}{
		ProjectID: finding.ProjectID, FindingID: finding.ID, FindingRunID: finding.RunID,
		FindingStatus: finding.Status, FindingKind: finding.FindingKind,
		Evidence: finding.Evidence, FixIntent: finding.FixIntent,
		DeveloperInstructions: finding.DeveloperInstructions,
		LikelyFilesOrSurfaces: finding.LikelyFilesOrSurfaces,
		AcceptanceTests:       finding.AcceptanceTests, Candidate: candidate,
		ExactSignatureHash: identity.ExactSignatureHash,
		SignaturePayload:   identity.SignaturePayload,
		ConflictBucketKeys: identity.ConflictBucketKeys,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func meaningfulJSON(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	switch typed := value.(type) {
	case nil:
		return false
	case map[string]any:
		return len(typed) > 0
	case []any:
		return len(typed) > 0
	case string:
		return strings.TrimSpace(typed) != ""
	default:
		return true
	}
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
