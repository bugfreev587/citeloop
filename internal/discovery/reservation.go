package discovery

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
)

var ErrSnapshotStale = errors.New("arbitration snapshot stale")

type ReservedWork struct {
	ProjectID   uuid.UUID
	CandidateID uuid.UUID
	DecisionID  uuid.UUID
	Owner       Owner
}

type WorkReference struct {
	Type string
	ID   uuid.UUID
}

type WorkCreator interface {
	CreateInTransaction(context.Context, *db.Queries, ReservedWork) (WorkReference, error)
}

type ReservationResult struct {
	SignatureID uuid.UUID
	Work        WorkReference
}

type ReservationStore interface {
	LoadPreparedDecision(context.Context, uuid.UUID, uuid.UUID) (PreparedDecision, error)
	LoadReservationConfig(context.Context, uuid.UUID) (ArbitrationConfig, error)
	ReserveAtomically(context.Context, PreparedDecision, WorkCreator) (ReservationResult, error)
}

type ReservationService struct {
	store ReservationStore
}

func NewReservationService(store ReservationStore) *ReservationService {
	return &ReservationService{store: store}
}

// ReservePrepared deliberately has no comparator or provider dependency. All
// semantic work must already be persisted by Phase A before this method can
// open the short compare-and-reserve transaction.
func (s *ReservationService) ReservePrepared(ctx context.Context, projectID, decisionID uuid.UUID, creator WorkCreator) (ReservationResult, error) {
	if s == nil || s.store == nil {
		return ReservationResult{}, errors.New("reservation store is required")
	}
	if projectID == uuid.Nil || decisionID == uuid.Nil {
		return ReservationResult{}, errors.New("project and arbitration decision ids are required")
	}
	if creator == nil {
		return ReservationResult{}, errors.New("work creator is required")
	}
	decision, err := s.store.LoadPreparedDecision(ctx, projectID, decisionID)
	if err != nil {
		return ReservationResult{}, fmt.Errorf("load prepared decision: %w", err)
	}
	if decision.ID != decisionID || decision.ProjectID != projectID || decision.CandidateID == uuid.Nil || decision.CandidateVersion < 1 {
		return ReservationResult{}, errors.New("prepared decision identity is invalid")
	}
	config, err := s.store.LoadReservationConfig(ctx, decision.ProjectID)
	if err != nil {
		return ReservationResult{}, fmt.Errorf("load reservation config: %w", err)
	}
	config = normalizeArbitrationConfig(config)
	if decision.Status != ArbitrationStatusPrepared {
		return ReservationResult{}, fmt.Errorf("decision status %q cannot reserve", decision.Status)
	}
	if decision.Decision != DecisionCreate {
		return ReservationResult{}, fmt.Errorf("decision %q does not create new work", decision.Decision)
	}
	if decision.Confidence < config.ConfidenceThreshold {
		return ReservationResult{}, fmt.Errorf("decision confidence %.4f is below threshold %.4f", decision.Confidence, config.ConfidenceThreshold)
	}
	if len(decision.ExpectedBucketVersions) == 0 || strings.TrimSpace(decision.SnapshotFingerprint) == "" {
		return ReservationResult{}, errors.New("prepared decision is missing its bucket snapshot")
	}
	if strings.TrimSpace(decision.ExactSignatureHash) == "" || strings.TrimSpace(decision.SemanticFingerprint) == "" {
		return ReservationResult{}, errors.New("prepared decision is missing work identity")
	}
	if decision.Owner != OwnerDoctor && decision.Owner != OwnerOpportunities {
		return ReservationResult{}, errors.New("prepared decision owner is invalid")
	}
	result, err := s.store.ReserveAtomically(ctx, decision, creator)
	if err != nil {
		return ReservationResult{}, err
	}
	if result.SignatureID == uuid.Nil || result.Work.ID == uuid.Nil || strings.TrimSpace(result.Work.Type) == "" {
		return ReservationResult{}, errors.New("atomic reservation returned an invalid work reference")
	}
	return result, nil
}
