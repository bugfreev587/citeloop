// Package discovery implements the internal candidate and work-identity layer
// shared by Doctor and Opportunities. Phase 1A is shadow-only: it observes
// collisions without changing legacy queues.
package discovery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
)

const (
	CandidateSchemaVersionV1 = "discovery-candidate-v1"
	SignatureVersionV1       = "work-signature-v1"
)

var ErrNeedsSpecification = errors.New("candidate needs specification")

type SourceKind string

const (
	SourceDoctor      SourceKind = "doctor"
	SourceSignalScan  SourceKind = "signal_scan"
	SourceAIDiscovery SourceKind = "ai_discovery"
	SourceMigration   SourceKind = "migration"
)

type Owner string

const (
	OwnerDoctor        Owner = "doctor"
	OwnerOpportunities Owner = "opportunities"
)

type VerificationMode string

const (
	VerificationImmediate VerificationMode = "immediate"
	VerificationDelayed   VerificationMode = "delayed"
)

type ArtifactIntent string

const (
	ArtifactRepairExistingSurface ArtifactIntent = "repair_existing_surface"
	ArtifactUpdateExistingContent ArtifactIntent = "update_existing_content"
	ArtifactCreateNewAsset        ArtifactIntent = "create_new_asset"
	ArtifactConsolidateAssets     ArtifactIntent = "consolidate_assets"
	ArtifactMeasurementOnly       ArtifactIntent = "measurement_only"
)

type CandidateStatus string

const (
	StatusIdentityReady      CandidateStatus = "identity_ready"
	StatusNeedsSpecification CandidateStatus = "needs_specification"
	StatusNeedsEvidence      CandidateStatus = "needs_evidence"
	StatusNeedsArbitration   CandidateStatus = "needs_arbitration_review"
)

type Mutation struct {
	Operation string `json:"operation"`
	Field     string `json:"field"`
	Selector  string `json:"selector,omitempty"`
	Source    string `json:"source,omitempty"`
	Target    string `json:"target,omitempty"`
}

type Candidate struct {
	ProjectID               uuid.UUID
	SourceKind              SourceKind
	SourceObjectType        string
	SourceObjectID          string
	TargetKind              string
	NormalizedTargetSet     []string
	IssueOrHypothesisFamily string
	ChangeFamily            string
	ProposedMutations       []Mutation
	ArtifactIntent          ArtifactIntent
	IntendedSlugOrCanonical string
	TopicEntityIdentity     []string
	AudienceIdentity        []string
	PrimarySuccessMetric    string
	VerificationMode        VerificationMode
	EvidenceIDs             []string
	EvidenceFingerprint     string
	SuggestedOwner          Owner
	Confidence              float64
	CandidateSchemaVersion  string
	SignatureVersion        string
	Status                  CandidateStatus
	HoldReason              string
}

type Identity struct {
	ExactSignatureHash string
	SignaturePayload   json.RawMessage
	ConflictBucketKeys []string
}

type signaturePayload struct {
	ProjectID               string         `json:"project_id"`
	NormalizedTargetSet     []string       `json:"normalized_target_set"`
	ChangeFamily            string         `json:"change_family"`
	NormalizedMutations     []Mutation     `json:"normalized_proposed_mutations"`
	ArtifactIntent          ArtifactIntent `json:"artifact_intent"`
	IntendedSlugOrCanonical string         `json:"intended_slug_or_canonical,omitempty"`
	TopicEntityIdentity     []string       `json:"topic_entity_identity"`
	AudienceIdentity        []string       `json:"audience_identity"`
	SignatureVersion        string         `json:"signature_version"`
}

func BuildIdentity(candidate Candidate) (Identity, error) {
	if candidate.ProjectID == uuid.Nil {
		return Identity{}, fmt.Errorf("%w: project_id is required", ErrNeedsSpecification)
	}
	if !supportedArtifactIntent(candidate.ArtifactIntent) {
		return Identity{}, fmt.Errorf("%w: unsupported artifact intent %q", ErrNeedsSpecification, candidate.ArtifactIntent)
	}
	for _, mutation := range candidate.ProposedMutations {
		operation := normalizeToken(mutation.Operation)
		if operation == "" || normalizeToken(mutation.Field) == "" {
			return Identity{}, fmt.Errorf("%w: every mutation requires operation and field", ErrNeedsSpecification)
		}
		if !supportedMutationOperation(operation) {
			return Identity{}, fmt.Errorf("%w: unsupported mutation operation %q", ErrNeedsSpecification, mutation.Operation)
		}
	}
	targets := normalizeTargetSet(candidate.NormalizedTargetSet)
	mutations := normalizeMutations(candidate.ProposedMutations)
	changeFamily := normalizeToken(candidate.ChangeFamily)
	intent := ArtifactIntent(normalizeToken(string(candidate.ArtifactIntent)))
	intended := normalizeTarget(candidate.IntendedSlugOrCanonical)
	version := strings.TrimSpace(candidate.SignatureVersion)
	if version == "" {
		version = SignatureVersionV1
	}
	if len(targets) == 0 || changeFamily == "" || len(mutations) == 0 || intent == "" {
		return Identity{}, fmt.Errorf("%w: target, change family, mutation, and artifact intent are required", ErrNeedsSpecification)
	}
	if intent == ArtifactCreateNewAsset && intended == "" {
		return Identity{}, fmt.Errorf("%w: create_new_asset requires intended slug or canonical", ErrNeedsSpecification)
	}
	payload := signaturePayload{
		ProjectID:               candidate.ProjectID.String(),
		NormalizedTargetSet:     targets,
		ChangeFamily:            changeFamily,
		NormalizedMutations:     mutations,
		ArtifactIntent:          intent,
		IntendedSlugOrCanonical: intended,
		TopicEntityIdentity:     normalizeTokenSet(candidate.TopicEntityIdentity),
		AudienceIdentity:        normalizeTokenSet(candidate.AudienceIdentity),
		SignatureVersion:        version,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return Identity{}, fmt.Errorf("marshal signature payload: %w", err)
	}
	sum := sha256.Sum256(raw)
	return Identity{
		ExactSignatureHash: hex.EncodeToString(sum[:]),
		SignaturePayload:   raw,
		ConflictBucketKeys: buildConflictBucketKeys(candidate.ProjectID, payload),
	}, nil
}

func supportedArtifactIntent(intent ArtifactIntent) bool {
	switch ArtifactIntent(normalizeToken(string(intent))) {
	case ArtifactRepairExistingSurface, ArtifactUpdateExistingContent, ArtifactCreateNewAsset, ArtifactConsolidateAssets, ArtifactMeasurementOnly:
		return true
	default:
		return false
	}
}

func supportedMutationOperation(operation string) bool {
	switch operation {
	case "add", "remove", "update", "move", "redirect", "link", "create", "consolidate":
		return true
	default:
		return false
	}
}

func normalizeMutations(mutations []Mutation) []Mutation {
	byKey := make(map[string]Mutation, len(mutations))
	for _, mutation := range mutations {
		normalized := Mutation{
			Operation: normalizeToken(mutation.Operation),
			Field:     normalizeToken(mutation.Field),
			Selector:  normalizeToken(mutation.Selector),
			Source:    normalizeTarget(mutation.Source),
			Target:    normalizeTarget(mutation.Target),
		}
		if normalized.Operation == "" || normalized.Field == "" {
			continue
		}
		raw, _ := json.Marshal(normalized)
		byKey[string(raw)] = normalized
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]Mutation, 0, len(keys))
	for _, key := range keys {
		out = append(out, byKey[key])
	}
	return out
}

func buildConflictBucketKeys(projectID uuid.UUID, payload signaturePayload) []string {
	prefix := "project:" + projectID.String() + "|"
	coarseFamily := payload.ChangeFamily
	if before, _, ok := strings.Cut(coarseFamily, "."); ok {
		coarseFamily = before
	}
	keys := make([]string, 0, len(payload.NormalizedTargetSet)+len(payload.TopicEntityIdentity)+1)
	if payload.IntendedSlugOrCanonical != "" {
		keys = append(keys, prefix+"slug:"+payload.IntendedSlugOrCanonical+"|change:"+coarseFamily)
	}
	for _, target := range payload.NormalizedTargetSet {
		keys = append(keys, prefix+"target:"+target+"|change:"+coarseFamily)
	}
	for _, topic := range payload.TopicEntityIdentity {
		keys = append(keys, prefix+"topic:"+topic+"|change:"+coarseFamily)
	}
	return normalizeExactSet(keys)
}

func normalizeTargetSet(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if normalized := normalizeTarget(value); normalized != "" {
			out = append(out, normalized)
		}
	}
	return normalizeExactSet(out)
}

func normalizeTokenSet(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if normalized := normalizeToken(value); normalized != "" {
			out = append(out, normalized)
		}
	}
	return normalizeExactSet(out)
}

func normalizeExactSet(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			seen[value] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizeToken(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func normalizeTarget(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasSuffix(value, "/") && !strings.HasSuffix(value, "://") {
		value = strings.TrimRight(value, "/")
	}
	return value
}
