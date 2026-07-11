package discovery

import (
	"context"
	"fmt"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
)

type Report struct {
	RunID                  uuid.UUID `json:"run_id"`
	ProjectID              uuid.UUID `json:"project_id"`
	Mode                   string    `json:"mode"`
	Status                 string    `json:"status"`
	DoctorCandidates       int       `json:"doctor_candidates"`
	OpportunityCandidates  int       `json:"opportunity_candidates"`
	IdentityReady          int       `json:"identity_ready"`
	NeedsSpecification     int       `json:"needs_specification"`
	ExactDuplicateGroups   int       `json:"exact_duplicate_groups"`
	PossibleConflictGroups int       `json:"possible_conflict_groups"`
	Error                  string    `json:"error,omitempty"`
	CreatedAt              time.Time `json:"created_at"`
	FinishedAt             time.Time `json:"finished_at,omitempty"`
}

type ShadowSignature struct {
	ProjectID          uuid.UUID
	CandidateID        uuid.UUID
	RunID              uuid.UUID
	SourceKind         SourceKind
	SourceObjectType   string
	SourceObjectID     string
	Mode               string
	Active             bool
	Owner              Owner
	ExactSignatureHash string
	SignaturePayload   []byte
	ConflictBucketKeys []string
	SignatureVersion   string
}

type Repository interface {
	CreateRun(context.Context, uuid.UUID) (Report, error)
	ListDoctorFindings(context.Context, uuid.UUID) ([]db.SeoDoctorFinding, error)
	ListOpportunities(context.Context, uuid.UUID) ([]db.SeoOpportunity, error)
	SaveCandidate(context.Context, uuid.UUID, Candidate, *Identity) (uuid.UUID, error)
	SaveShadowSignature(context.Context, ShadowSignature) error
	CompleteRun(context.Context, Report) (Report, error)
	FailRun(context.Context, Report, error) error
	LatestRun(context.Context, uuid.UUID) (Report, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) RunProject(ctx context.Context, projectID uuid.UUID) (Report, error) {
	if s == nil || s.repo == nil {
		return Report{}, fmt.Errorf("discovery repository is required")
	}
	report, err := s.repo.CreateRun(ctx, projectID)
	if err != nil {
		return Report{}, fmt.Errorf("create discovery shadow run: %w", err)
	}
	doctorFindings, err := s.repo.ListDoctorFindings(ctx, projectID)
	if err != nil {
		return Report{}, s.fail(ctx, report, fmt.Errorf("list Doctor findings: %w", err))
	}
	opportunities, err := s.repo.ListOpportunities(ctx, projectID)
	if err != nil {
		return Report{}, s.fail(ctx, report, fmt.Errorf("list opportunities: %w", err))
	}

	candidates := make([]Candidate, 0, len(doctorFindings)+len(opportunities))
	for _, finding := range doctorFindings {
		candidates = append(candidates, ProjectDoctorFinding(finding))
		report.DoctorCandidates++
	}
	for _, opportunity := range opportunities {
		candidates = append(candidates, ProjectSEOOpportunity(opportunity))
		report.OpportunityCandidates++
	}

	signatures := make([]ShadowSignature, 0, len(candidates))
	for _, candidate := range candidates {
		var identity *Identity
		if candidate.Status == StatusIdentityReady {
			built, buildErr := BuildIdentity(candidate)
			if buildErr != nil {
				candidate = holdCandidate(candidate, buildErr.Error())
			} else {
				identity = &built
				report.IdentityReady++
			}
		}
		if candidate.Status == StatusNeedsSpecification {
			report.NeedsSpecification++
		}
		candidateID, saveErr := s.repo.SaveCandidate(ctx, report.RunID, candidate, identity)
		if saveErr != nil {
			return Report{}, s.fail(ctx, report, fmt.Errorf("save candidate %s/%s: %w", candidate.SourceObjectType, candidate.SourceObjectID, saveErr))
		}
		if identity == nil {
			continue
		}
		signature := ShadowSignature{
			ProjectID:          projectID,
			CandidateID:        candidateID,
			RunID:              report.RunID,
			SourceKind:         candidate.SourceKind,
			SourceObjectType:   candidate.SourceObjectType,
			SourceObjectID:     candidate.SourceObjectID,
			Mode:               "shadow",
			Active:             false,
			Owner:              candidate.SuggestedOwner,
			ExactSignatureHash: identity.ExactSignatureHash,
			SignaturePayload:   identity.SignaturePayload,
			ConflictBucketKeys: identity.ConflictBucketKeys,
			SignatureVersion:   candidate.SignatureVersion,
		}
		if saveErr := s.repo.SaveShadowSignature(ctx, signature); saveErr != nil {
			return Report{}, s.fail(ctx, report, fmt.Errorf("save shadow signature for candidate %s: %w", candidateID, saveErr))
		}
		signatures = append(signatures, signature)
	}
	report.ExactDuplicateGroups, report.PossibleConflictGroups = aggregateShadowConflicts(signatures)
	completed, err := s.repo.CompleteRun(ctx, report)
	if err != nil {
		return Report{}, fmt.Errorf("complete discovery shadow run: %w", err)
	}
	return completed, nil
}

func (s *Service) LatestReport(ctx context.Context, projectID uuid.UUID) (Report, error) {
	if s == nil || s.repo == nil {
		return Report{}, fmt.Errorf("discovery repository is required")
	}
	return s.repo.LatestRun(ctx, projectID)
}

func (s *Service) fail(ctx context.Context, report Report, runErr error) error {
	if err := s.repo.FailRun(ctx, report, runErr); err != nil {
		return fmt.Errorf("%v; mark discovery shadow run failed: %w", runErr, err)
	}
	return runErr
}

func aggregateShadowConflicts(signatures []ShadowSignature) (exactGroups, possibleGroups int) {
	exactCounts := make(map[string]int)
	bucketHashes := make(map[string]map[string]struct{})
	for _, signature := range signatures {
		exactCounts[signature.ExactSignatureHash]++
		for _, bucket := range signature.ConflictBucketKeys {
			if bucketHashes[bucket] == nil {
				bucketHashes[bucket] = make(map[string]struct{})
			}
			bucketHashes[bucket][signature.ExactSignatureHash] = struct{}{}
		}
	}
	for _, count := range exactCounts {
		if count > 1 {
			exactGroups++
		}
	}
	for _, hashes := range bucketHashes {
		if len(hashes) > 1 {
			possibleGroups++
		}
	}
	return exactGroups, possibleGroups
}
