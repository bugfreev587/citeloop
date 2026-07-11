package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	DefaultArbitrationConfidenceThreshold = 0.80
	ArbitrationRulesVersionV1             = "discovery-arbitration-v1"
)

const DecisionHold DecisionKind = "hold"

type ArbitrationDisposition string

const (
	DispositionDeterministicSafe       ArbitrationDisposition = "deterministic_safe"
	DispositionExactMerge              ArbitrationDisposition = "exact_merge"
	DispositionSemanticComparison      ArbitrationDisposition = "semantic_comparison"
	DispositionReviewMemory            ArbitrationDisposition = "review_memory"
	DispositionProviderFailure         ArbitrationDisposition = "provider_failure"
	DispositionIncompleteSpecification ArbitrationDisposition = "incomplete_specification"
	DispositionManualResolution        ArbitrationDisposition = "manual_resolution"
)

type ArbitrationStatus string

const (
	ArbitrationStatusPrepared ArbitrationStatus = "prepared"
	ArbitrationStatusHeld     ArbitrationStatus = "held"
	ArbitrationStatusResolved ArbitrationStatus = "resolved"
)

type ArbitrationCandidate struct {
	ID        uuid.UUID
	RunID     uuid.UUID
	Version   int64
	Candidate Candidate
	Identity  Identity
}

type SnapshotWork struct {
	ID                  uuid.UUID
	Owner               Owner
	ExactSignatureHash  string
	SignaturePayload    json.RawMessage
	SemanticFingerprint string
	EvidenceFingerprint string
	SignatureVersion    string
}

type ReviewMemorySnapshot struct {
	ID                  uuid.UUID
	Decision            string
	ExactSignatureHash  string
	SemanticFingerprint string
	SignaturePayload    json.RawMessage
	EvidenceFingerprint string
	SignatureVersion    string
	SnoozedUntil        time.Time
	Active              bool
}

type ReviewMemoryAliasSnapshot struct {
	ReviewMemoryID      uuid.UUID
	ExactSignatureHash  string
	SemanticFingerprint string
	SignatureVersion    string
}

type BucketSnapshot struct {
	Versions      map[string]int64
	ActiveWorks   []SnapshotWork
	ReviewMemory  []ReviewMemorySnapshot
	ReviewAliases []ReviewMemoryAliasSnapshot
}

type ArbitrationConfig struct {
	ConfidenceThreshold         float64
	LaunchReady                 bool
	AutomaticSuppressionEnabled bool
	RulesVersion                string
	Provider                    string
	Model                       string
}

type AICallStart struct {
	ProjectID          uuid.UUID
	RunID              uuid.UUID
	CandidateID        uuid.UUID
	Provider           string
	Model              string
	PromptVersion      string
	RequestFingerprint string
	Status             string
}

type AICallFinish struct {
	ID               uuid.UUID
	ProjectID        uuid.UUID
	Status           string
	ErrorCode        string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CostUSD          float64
}

type ReviewHold struct {
	ProjectID              uuid.UUID
	CandidateID            uuid.UUID
	CandidateVersion       int64
	State                  CandidateStatus
	Reason                 string
	ExpectedBucketVersions map[string]int64
	ArbitrationDecisionID  uuid.UUID
	DueAt                  time.Time
}

type PreparedDecision struct {
	ID                     uuid.UUID              `json:"id"`
	ProjectID              uuid.UUID              `json:"project_id"`
	CandidateID            uuid.UUID              `json:"candidate_id"`
	CandidateVersion       int64                  `json:"candidate_version"`
	AICallID               uuid.UUID              `json:"ai_call_id,omitempty"`
	Disposition            ArbitrationDisposition `json:"disposition"`
	Decision               DecisionKind           `json:"decision"`
	Owner                  Owner                  `json:"owner"`
	OverlapWorkIDs         []uuid.UUID            `json:"overlap_work_ids"`
	Reason                 string                 `json:"reason"`
	Confidence             float64                `json:"confidence"`
	SemanticFingerprint    string                 `json:"semantic_fingerprint"`
	ComparedWorkIDs        []uuid.UUID            `json:"compared_work_ids"`
	ExpectedBucketVersions map[string]int64       `json:"expected_bucket_versions"`
	SnapshotFingerprint    string                 `json:"snapshot_fingerprint"`
	ExactSignatureHash     string                 `json:"exact_signature_hash"`
	SignatureVersion       string                 `json:"signature_version"`
	EvidenceFingerprint    string                 `json:"evidence_fingerprint"`
	RulesVersion           string                 `json:"rules_version"`
	PromptVersion          string                 `json:"prompt_version"`
	Provider               string                 `json:"provider"`
	Model                  string                 `json:"model"`
	Status                 ArbitrationStatus      `json:"status"`
}

type ArbitrationStore interface {
	LoadCandidate(context.Context, uuid.UUID, uuid.UUID) (ArbitrationCandidate, error)
	MaterializeBuckets(context.Context, uuid.UUID, []string) error
	ReadSnapshot(context.Context, uuid.UUID, []string) (BucketSnapshot, error)
	LoadConfig(context.Context, uuid.UUID) (ArbitrationConfig, error)
	StartAICall(context.Context, AICallStart) (uuid.UUID, error)
	FinishAICall(context.Context, AICallFinish) error
	SavePreparedDecision(context.Context, PreparedDecision) (PreparedDecision, error)
	SaveReviewHold(context.Context, ReviewHold) error
}

type ArbitrationService struct {
	store      ArbitrationStore
	comparator SemanticComparator
	now        func() time.Time
}

func NewArbitrationService(store ArbitrationStore, comparator SemanticComparator) *ArbitrationService {
	return &ArbitrationService{store: store, comparator: comparator, now: time.Now}
}

func (s *ArbitrationService) Prepare(ctx context.Context, projectID, candidateID uuid.UUID) (PreparedDecision, error) {
	if s == nil || s.store == nil {
		return PreparedDecision{}, errors.New("arbitration store is required")
	}
	if projectID == uuid.Nil || candidateID == uuid.Nil {
		return PreparedDecision{}, errors.New("project and candidate ids are required")
	}
	candidate, err := s.store.LoadCandidate(ctx, projectID, candidateID)
	if err != nil {
		return PreparedDecision{}, fmt.Errorf("load arbitration candidate: %w", err)
	}
	if candidate.ID == uuid.Nil || candidate.Candidate.ProjectID == uuid.Nil || candidate.Version < 1 {
		return PreparedDecision{}, errors.New("invalid arbitration candidate")
	}
	if candidate.Candidate.ProjectID != projectID {
		return PreparedDecision{}, errors.New("arbitration candidate does not belong to project")
	}
	if candidate.Candidate.Status != StatusIdentityReady || candidate.Identity.ExactSignatureHash == "" || len(candidate.Identity.ConflictBucketKeys) == 0 {
		return s.persistHold(ctx, candidate, BucketSnapshot{Versions: map[string]int64{}}, PreparedDecision{
			Disposition: DispositionIncompleteSpecification,
			Decision:    DecisionHold,
			Reason:      "candidate does not have a complete work identity",
			Confidence:  0,
			Status:      ArbitrationStatusHeld,
		})
	}
	if err := s.store.MaterializeBuckets(ctx, projectID, candidate.Identity.ConflictBucketKeys); err != nil {
		return PreparedDecision{}, fmt.Errorf("materialize arbitration buckets: %w", err)
	}
	snapshot, err := s.store.ReadSnapshot(ctx, projectID, candidate.Identity.ConflictBucketKeys)
	if err != nil {
		return PreparedDecision{}, fmt.Errorf("read arbitration snapshot: %w", err)
	}
	config, err := s.store.LoadConfig(ctx, projectID)
	if err != nil {
		return PreparedDecision{}, fmt.Errorf("load arbitration config: %w", err)
	}
	config = normalizeArbitrationConfig(config)
	snapshotFingerprint, err := buildSnapshotFingerprint(snapshot)
	if err != nil {
		return PreparedDecision{}, err
	}
	base := PreparedDecision{
		ProjectID:              projectID,
		CandidateID:            candidate.ID,
		CandidateVersion:       candidate.Version,
		Owner:                  ownerForCandidate(candidate.Candidate),
		ExpectedBucketVersions: cloneVersions(snapshot.Versions),
		SnapshotFingerprint:    snapshotFingerprint,
		ExactSignatureHash:     candidate.Identity.ExactSignatureHash,
		SignatureVersion:       firstNonEmpty(candidate.Candidate.SignatureVersion, SignatureVersionV1),
		EvidenceFingerprint:    candidate.Candidate.EvidenceFingerprint,
		RulesVersion:           config.RulesVersion,
		PromptVersion:          SemanticPromptVersionV1,
		Provider:               firstNonEmpty(config.Provider, "deterministic"),
		Model:                  firstNonEmpty(config.Model, "deterministic"),
	}

	if exact := exactActiveWork(snapshot.ActiveWorks, candidate.Identity.ExactSignatureHash); exact != nil {
		base.Disposition = DispositionExactMerge
		base.Decision = DecisionMergeEvidence
		base.Owner = firstValidOwner(exact.Owner, base.Owner)
		base.OverlapWorkIDs = []uuid.UUID{exact.ID}
		base.ComparedWorkIDs = []uuid.UUID{exact.ID}
		base.Reason = "an active reservation already owns the exact work signature"
		base.Confidence = 1
		base.Status = ArbitrationStatusPrepared
		base.SemanticFingerprint, _ = semanticFingerprint(candidate.Identity, "deterministic")
		return s.store.SavePreparedDecision(ctx, base)
	}

	if memory := exactReviewMemory(snapshot.ReviewMemory, candidate, s.now().UTC()); memory != nil {
		base.Disposition = DispositionReviewMemory
		base.Decision = DecisionSuppress
		base.OverlapWorkIDs = []uuid.UUID{memory.ID}
		base.ComparedWorkIDs = []uuid.UUID{memory.ID}
		base.Reason = "active review memory applies to the unchanged work evidence"
		base.Confidence = 1
		base.SemanticFingerprint = memory.SemanticFingerprint
		base.Status = ArbitrationStatusResolved
		return s.store.SavePreparedDecision(ctx, base)
	}

	if memory := exactReviewMemoryAlias(snapshot, candidate, s.now().UTC()); memory != nil {
		base.Disposition = DispositionReviewMemory
		base.Decision = DecisionSuppress
		base.OverlapWorkIDs = []uuid.UUID{memory.ID}
		base.ComparedWorkIDs = []uuid.UUID{memory.ID}
		base.Reason = "review memory alias applies to the unchanged work evidence"
		base.Confidence = 1
		base.SemanticFingerprint = memory.SemanticFingerprint
		base.Status = ArbitrationStatusResolved
		return s.store.SavePreparedDecision(ctx, base)
	}

	possible, incompleteMemory := semanticOverlapSet(snapshot)
	if len(possible) == 0 && !incompleteMemory {
		base.Disposition = DispositionDeterministicSafe
		base.Decision = DecisionCreate
		base.Reason = "no active work or review memory shares a deterministic conflict bucket"
		base.Confidence = 1
		base.Status = ArbitrationStatusPrepared
		base.SemanticFingerprint, _ = semanticFingerprint(candidate.Identity, "deterministic")
		return s.store.SavePreparedDecision(ctx, base)
	}
	if incompleteMemory {
		base.Disposition = DispositionReviewMemory
		base.Decision = DecisionHold
		base.Reason = "overlapping review memory lacks semantic comparison material"
		base.Confidence = 0
		base.Status = ArbitrationStatusHeld
		return s.persistHold(ctx, candidate, snapshot, base)
	}
	if s.comparator == nil {
		base.Disposition = DispositionProviderFailure
		base.Decision = DecisionHold
		base.Reason = "semantic comparison provider is unavailable"
		base.Status = ArbitrationStatusHeld
		return s.persistHold(ctx, candidate, snapshot, base)
	}
	semanticRequest := SemanticRequest{
		CandidateID:      candidate.ID,
		Candidate:        candidate.Candidate,
		Identity:         candidate.Identity,
		PossibleOverlaps: possible,
	}
	requestFingerprint := arbitrationRequestFingerprint(candidate, snapshotFingerprint)
	callID, err := s.store.StartAICall(ctx, AICallStart{
		ProjectID:          projectID,
		RunID:              candidate.CandidateRunID(),
		CandidateID:        candidate.ID,
		Provider:           firstNonEmpty(config.Provider, "tokengate"),
		Model:              config.Model,
		PromptVersion:      SemanticPromptVersionV1,
		RequestFingerprint: requestFingerprint,
		Status:             "running",
	})
	if err != nil {
		return PreparedDecision{}, fmt.Errorf("start arbitration AI call: %w", err)
	}
	decision, usage, compareErr := s.comparator.Compare(ctx, semanticRequest)
	if compareErr != nil {
		_ = s.store.FinishAICall(context.WithoutCancel(ctx), AICallFinish{
			ID: callID, ProjectID: projectID, Status: "failed", ErrorCode: "provider_failure",
		})
		base.AICallID = callID
		base.Disposition = DispositionProviderFailure
		base.Decision = DecisionHold
		base.Reason = "semantic provider failed: " + compareErr.Error()
		base.Status = ArbitrationStatusHeld
		return s.persistHold(ctx, candidate, snapshot, base)
	}
	if err := s.store.FinishAICall(ctx, AICallFinish{
		ID: callID, ProjectID: projectID, Status: "ok",
		TotalTokens: usage.TotalTokens, CostUSD: usage.CostUSD,
	}); err != nil {
		return PreparedDecision{}, fmt.Errorf("finish arbitration AI call: %w", err)
	}
	base.AICallID = callID
	base.Disposition = DispositionSemanticComparison
	base.Decision = decision.Decision
	base.Owner = decision.Owner
	base.OverlapWorkIDs = append([]uuid.UUID(nil), decision.Overlaps...)
	base.ComparedWorkIDs = semanticWorkIDs(possible)
	base.Reason = decision.Reason
	base.Confidence = decision.Confidence
	base.SemanticFingerprint = decision.SemanticFingerprint
	base.Provider = firstNonEmpty(usage.Provider, config.Provider)
	base.Model = firstNonEmpty(usage.Model, config.Model)
	base.PromptVersion = firstNonEmpty(usage.PromptVersion, SemanticPromptVersionV1)
	if decision.Confidence < config.ConfidenceThreshold {
		base.Decision = DecisionHold
		base.Reason = fmt.Sprintf("semantic confidence %.4f is below threshold %.4f", decision.Confidence, config.ConfidenceThreshold)
		base.Status = ArbitrationStatusHeld
		return s.persistHold(ctx, candidate, snapshot, base)
	}
	if decision.Decision == DecisionSuppress {
		hasMemory, onlyMemory, allApply := reviewMemoryOverlapPolicy(snapshot, decision.Overlaps, candidate, s.now().UTC())
		if hasMemory && onlyMemory && allApply {
			base.Disposition = DispositionReviewMemory
			base.Status = ArbitrationStatusResolved
			return s.store.SavePreparedDecision(ctx, base)
		}
		if hasMemory && onlyMemory {
			base.Disposition = DispositionReviewMemory
			base.Decision = DecisionCreate
			base.Reason = "versioned material evidence change reopens previously reviewed work"
			base.Status = ArbitrationStatusPrepared
			return s.store.SavePreparedDecision(ctx, base)
		}
	}
	if decision.Decision == DecisionSuppress && (!config.LaunchReady || !config.AutomaticSuppressionEnabled) {
		base.Decision = DecisionHold
		base.Reason = "semantic suppression is disabled until the launch evaluation gate passes"
		base.Status = ArbitrationStatusHeld
		return s.persistHold(ctx, candidate, snapshot, base)
	}
	base.Status = ArbitrationStatusPrepared
	return s.store.SavePreparedDecision(ctx, base)
}

func (s *ArbitrationService) persistHold(ctx context.Context, candidate ArbitrationCandidate, snapshot BucketSnapshot, prepared PreparedDecision) (PreparedDecision, error) {
	if prepared.ProjectID == uuid.Nil {
		prepared.ProjectID = candidate.Candidate.ProjectID
		prepared.CandidateID = candidate.ID
		prepared.CandidateVersion = candidate.Version
		prepared.ExpectedBucketVersions = cloneVersions(snapshot.Versions)
		prepared.ExactSignatureHash = candidate.Identity.ExactSignatureHash
		prepared.SignatureVersion = firstNonEmpty(candidate.Candidate.SignatureVersion, SignatureVersionV1)
		prepared.EvidenceFingerprint = candidate.Candidate.EvidenceFingerprint
		prepared.RulesVersion = ArbitrationRulesVersionV1
		prepared.PromptVersion = SemanticPromptVersionV1
		prepared.Provider = "deterministic"
		prepared.Model = "deterministic"
		prepared.SnapshotFingerprint, _ = buildSnapshotFingerprint(snapshot)
	}
	prepared.Status = ArbitrationStatusHeld
	saved, err := s.store.SavePreparedDecision(ctx, prepared)
	if err != nil {
		return PreparedDecision{}, err
	}
	if err := s.store.SaveReviewHold(ctx, ReviewHold{
		ProjectID:              saved.ProjectID,
		CandidateID:            saved.CandidateID,
		CandidateVersion:       saved.CandidateVersion,
		State:                  reviewStateForHold(candidate.Candidate.Status, saved.Disposition),
		Reason:                 saved.Reason,
		ExpectedBucketVersions: cloneVersions(saved.ExpectedBucketVersions),
		ArbitrationDecisionID:  saved.ID,
		DueAt:                  s.now().UTC().Add(48 * time.Hour),
	}); err != nil {
		return PreparedDecision{}, err
	}
	return saved, nil
}

func reviewStateForHold(candidateStatus CandidateStatus, disposition ArbitrationDisposition) CandidateStatus {
	if disposition != DispositionIncompleteSpecification {
		return StatusNeedsArbitration
	}
	if candidateStatus == StatusNeedsEvidence {
		return StatusNeedsEvidence
	}
	return StatusNeedsSpecification
}

func normalizeArbitrationConfig(config ArbitrationConfig) ArbitrationConfig {
	if config.ConfidenceThreshold <= 0 || config.ConfidenceThreshold > 1 {
		config.ConfidenceThreshold = DefaultArbitrationConfidenceThreshold
	}
	if strings.TrimSpace(config.RulesVersion) == "" {
		config.RulesVersion = ArbitrationRulesVersionV1
	}
	return config
}

func ownerForCandidate(candidate Candidate) Owner {
	if candidate.VerificationMode == VerificationImmediate {
		return OwnerDoctor
	}
	if candidate.SuggestedOwner == OwnerDoctor || candidate.SuggestedOwner == OwnerOpportunities {
		return candidate.SuggestedOwner
	}
	return OwnerOpportunities
}

func firstValidOwner(values ...Owner) Owner {
	for _, value := range values {
		if value == OwnerDoctor || value == OwnerOpportunities {
			return value
		}
	}
	return OwnerOpportunities
}

func exactActiveWork(works []SnapshotWork, hash string) *SnapshotWork {
	for i := range works {
		if works[i].ExactSignatureHash == hash {
			return &works[i]
		}
	}
	return nil
}

func exactReviewMemory(memories []ReviewMemorySnapshot, candidate ArbitrationCandidate, now time.Time) *ReviewMemorySnapshot {
	for i := range memories {
		memory := &memories[i]
		if !memory.Active || memory.ExactSignatureHash != candidate.Identity.ExactSignatureHash {
			continue
		}
		if !reviewMemoryApplies(*memory, candidate, now) {
			continue
		}
		return memory
	}
	return nil
}

func exactReviewMemoryAlias(snapshot BucketSnapshot, candidate ArbitrationCandidate, now time.Time) *ReviewMemorySnapshot {
	for _, alias := range snapshot.ReviewAliases {
		if alias.ExactSignatureHash != candidate.Identity.ExactSignatureHash {
			continue
		}
		for i := range snapshot.ReviewMemory {
			memory := &snapshot.ReviewMemory[i]
			if memory.ID != alias.ReviewMemoryID || !memory.Active {
				continue
			}
			if !reviewMemoryApplies(*memory, candidate, now) {
				continue
			}
			return memory
		}
	}
	return nil
}

func reviewMemoryApplies(memory ReviewMemorySnapshot, candidate ArbitrationCandidate, now time.Time) bool {
	if !memory.Active {
		return false
	}
	if memory.Decision == "snoozed" {
		return !memory.SnoozedUntil.IsZero() && now.Before(memory.SnoozedUntil)
	}
	return memory.EvidenceFingerprint == candidate.Candidate.EvidenceFingerprint
}

func reviewMemoryOverlapPolicy(snapshot BucketSnapshot, overlaps []uuid.UUID, candidate ArbitrationCandidate, now time.Time) (hasMemory, onlyMemory, allApply bool) {
	byID := make(map[uuid.UUID]ReviewMemorySnapshot, len(snapshot.ReviewMemory))
	for _, memory := range snapshot.ReviewMemory {
		if memory.Active {
			byID[memory.ID] = memory
		}
	}
	onlyMemory = len(overlaps) > 0
	allApply = true
	for _, id := range overlaps {
		memory, ok := byID[id]
		if !ok {
			onlyMemory = false
			continue
		}
		hasMemory = true
		if !reviewMemoryApplies(memory, candidate, now) {
			allApply = false
		}
	}
	return hasMemory, onlyMemory, allApply
}

func semanticOverlapSet(snapshot BucketSnapshot) ([]SemanticWork, bool) {
	out := make([]SemanticWork, 0, len(snapshot.ActiveWorks)+len(snapshot.ReviewMemory))
	incompleteOverlap := false
	for _, work := range snapshot.ActiveWorks {
		if work.ID == uuid.Nil || work.ExactSignatureHash == "" || len(work.SignaturePayload) == 0 {
			incompleteOverlap = true
			continue
		}
		out = append(out, SemanticWork{
			ID: work.ID, ExactSignatureHash: work.ExactSignatureHash,
			SignaturePayload: work.SignaturePayload, SemanticFingerprint: work.SemanticFingerprint,
		})
	}
	for _, memory := range snapshot.ReviewMemory {
		if !memory.Active {
			continue
		}
		if memory.ID == uuid.Nil || memory.ExactSignatureHash == "" || len(memory.SignaturePayload) == 0 {
			incompleteOverlap = true
			continue
		}
		out = append(out, SemanticWork{
			ID: memory.ID, ExactSignatureHash: memory.ExactSignatureHash,
			SignaturePayload: memory.SignaturePayload, SemanticFingerprint: memory.SemanticFingerprint,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID.String() < out[j].ID.String() })
	return out, incompleteOverlap
}

func semanticWorkIDs(works []SemanticWork) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(works))
	for _, work := range works {
		ids = append(ids, work.ID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	return ids
}

func buildSnapshotFingerprint(snapshot BucketSnapshot) (string, error) {
	type bucketVersion struct {
		Key     string `json:"key"`
		Version int64  `json:"version"`
	}
	type active struct {
		ID          string `json:"id"`
		Exact       string `json:"exact"`
		Semantic    string `json:"semantic"`
		Evidence    string `json:"evidence"`
		Version     string `json:"version"`
		PayloadHash string `json:"payload_hash"`
	}
	type memory struct {
		ID           string `json:"id"`
		Decision     string `json:"decision"`
		Exact        string `json:"exact"`
		Semantic     string `json:"semantic"`
		Evidence     string `json:"evidence"`
		Version      string `json:"version"`
		PayloadHash  string `json:"payload_hash"`
		SnoozedUntil string `json:"snoozed_until,omitempty"`
	}
	type alias struct {
		MemoryID string `json:"memory_id"`
		Exact    string `json:"exact"`
		Semantic string `json:"semantic"`
		Version  string `json:"version"`
	}
	buckets := make([]bucketVersion, 0, len(snapshot.Versions))
	for key, version := range snapshot.Versions {
		buckets = append(buckets, bucketVersion{Key: key, Version: version})
	}
	sort.Slice(buckets, func(i, j int) bool { return buckets[i].Key < buckets[j].Key })
	activeRows := make([]active, 0, len(snapshot.ActiveWorks))
	for _, work := range snapshot.ActiveWorks {
		activeRows = append(activeRows, active{ID: work.ID.String(), Exact: work.ExactSignatureHash, Semantic: work.SemanticFingerprint, Evidence: work.EvidenceFingerprint, Version: work.SignatureVersion, PayloadHash: rawFingerprint(work.SignaturePayload)})
	}
	sort.Slice(activeRows, func(i, j int) bool { return activeRows[i].ID < activeRows[j].ID })
	memoryRows := make([]memory, 0, len(snapshot.ReviewMemory))
	for _, row := range snapshot.ReviewMemory {
		snoozedUntil := ""
		if !row.SnoozedUntil.IsZero() {
			snoozedUntil = row.SnoozedUntil.UTC().Format(time.RFC3339Nano)
		}
		memoryRows = append(memoryRows, memory{ID: row.ID.String(), Decision: row.Decision, Exact: row.ExactSignatureHash, Semantic: row.SemanticFingerprint, Evidence: row.EvidenceFingerprint, Version: row.SignatureVersion, PayloadHash: rawFingerprint(row.SignaturePayload), SnoozedUntil: snoozedUntil})
	}
	sort.Slice(memoryRows, func(i, j int) bool { return memoryRows[i].ID < memoryRows[j].ID })
	aliasRows := make([]alias, 0, len(snapshot.ReviewAliases))
	for _, row := range snapshot.ReviewAliases {
		aliasRows = append(aliasRows, alias{MemoryID: row.ReviewMemoryID.String(), Exact: row.ExactSignatureHash, Semantic: row.SemanticFingerprint, Version: row.SignatureVersion})
	}
	sort.Slice(aliasRows, func(i, j int) bool {
		if aliasRows[i].MemoryID == aliasRows[j].MemoryID {
			if aliasRows[i].Version == aliasRows[j].Version {
				return aliasRows[i].Exact < aliasRows[j].Exact
			}
			return aliasRows[i].Version < aliasRows[j].Version
		}
		return aliasRows[i].MemoryID < aliasRows[j].MemoryID
	})
	raw, err := json.Marshal(struct {
		Buckets []bucketVersion `json:"buckets"`
		Active  []active        `json:"active"`
		Memory  []memory        `json:"memory"`
		Aliases []alias         `json:"aliases"`
	}{Buckets: buckets, Active: activeRows, Memory: memoryRows, Aliases: aliasRows})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func rawFingerprint(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func cloneVersions(values map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func arbitrationRequestFingerprint(candidate ArbitrationCandidate, snapshotFingerprint string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		candidate.Identity.ExactSignatureHash,
		snapshotFingerprint,
		SemanticPromptVersionV1,
	}, "|")))
	return hex.EncodeToString(sum[:])
}

func (candidate ArbitrationCandidate) CandidateRunID() uuid.UUID { return candidate.RunID }
