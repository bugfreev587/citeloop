package discovery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strings"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/jackc/pgx/v5/pgtype"
)

type legacyWorkSpec struct {
	changeFamily      string
	operation         string
	field             string
	artifactIntent    ArtifactIntent
	owner             Owner
	verificationMode  VerificationMode
	primaryMetric     string
	usesQueryIdentity bool
}

func ProjectDoctorFinding(finding db.SeoDoctorFinding) Candidate {
	targets := rawStringList(finding.NormalizedUrls)
	if len(targets) == 0 {
		targets = rawStringList(finding.AffectedUrls)
	}
	spec, ok := doctorWorkSpec(finding.IssueType)
	candidate := Candidate{
		ProjectID:               finding.ProjectID,
		SourceKind:              SourceDoctor,
		SourceObjectType:        "seo_doctor_finding",
		SourceObjectID:          finding.ID.String(),
		TargetKind:              "page",
		NormalizedTargetSet:     targets,
		IssueOrHypothesisFamily: finding.IssueType,
		ArtifactIntent:          ArtifactRepairExistingSurface,
		SuggestedOwner:          OwnerDoctor,
		VerificationMode:        VerificationImmediate,
		EvidenceFingerprint:     fingerprintJSON(finding.Evidence),
		EvidenceIDs:             nonZeroUUIDs(finding.RunID.String()),
		Confidence:              1,
		CandidateSchemaVersion:  CandidateSchemaVersionV1,
		SignatureVersion:        SignatureVersionV1,
	}
	if !ok {
		return holdCandidate(candidate, "Doctor issue type does not yet provide a deterministic mutation specification")
	}
	applyLegacySpec(&candidate, spec)
	return finalizeProjectedCandidate(candidate)
}

func ProjectSEOOpportunity(opportunity db.SeoOpportunity) Candidate {
	targets := normalizeTargetSet([]string{opportunity.NormalizedPageUrl})
	if len(targets) == 0 && opportunity.PageUrl != nil {
		targets = normalizeTargetSet([]string{*opportunity.PageUrl})
	}
	spec, ok := opportunityWorkSpec(opportunity.Type)
	if normalizeIssueType(opportunity.Type) == "technical_visibility_issue" {
		spec, ok = technicalVisibilityWorkSpec(opportunity.Evidence)
	}
	candidate := Candidate{
		ProjectID:               opportunity.ProjectID,
		SourceKind:              opportunitySourceKind(opportunity),
		SourceObjectType:        "seo_opportunity",
		SourceObjectID:          opportunity.ID.String(),
		TargetKind:              "page",
		NormalizedTargetSet:     targets,
		IssueOrHypothesisFamily: opportunity.Type,
		ArtifactIntent:          ArtifactUpdateExistingContent,
		SuggestedOwner:          OwnerOpportunities,
		VerificationMode:        VerificationDelayed,
		EvidenceFingerprint:     firstNonEmpty(opportunity.EvidenceFingerprint, fingerprintJSON(opportunity.Evidence)),
		Confidence:              normalizeLegacyConfidence(numericFloat(opportunity.Confidence)),
		CandidateSchemaVersion:  CandidateSchemaVersionV1,
		SignatureVersion:        SignatureVersionV1,
	}
	if ok && spec.usesQueryIdentity && opportunity.Query != nil {
		candidate.TopicEntityIdentity = []string{*opportunity.Query}
	}
	if !ok {
		return holdCandidate(candidate, "Opportunity type does not yet provide a deterministic mutation specification")
	}
	applyLegacySpec(&candidate, spec)
	if normalizeIssueType(opportunity.Type) == "gsc_query_cannibalization" {
		candidate.NormalizedTargetSet = normalizeTargetSet(append(candidate.NormalizedTargetSet, evidencePageTargets(opportunity.Evidence)...))
	}
	if spec.artifactIntent == ArtifactCreateNewAsset {
		candidate.IntendedSlugOrCanonical = evidenceString(opportunity.Evidence, "intended_slug_or_canonical")
	}
	return finalizeProjectedCandidate(candidate)
}

func applyLegacySpec(candidate *Candidate, spec legacyWorkSpec) {
	candidate.ChangeFamily = spec.changeFamily
	candidate.ProposedMutations = []Mutation{{
		Operation: spec.operation,
		Field:     spec.field,
	}}
	candidate.ArtifactIntent = spec.artifactIntent
	candidate.SuggestedOwner = spec.owner
	candidate.VerificationMode = spec.verificationMode
	candidate.PrimarySuccessMetric = spec.primaryMetric
}

func finalizeProjectedCandidate(candidate Candidate) Candidate {
	if _, err := BuildIdentity(candidate); err != nil {
		if errors.Is(err, ErrNeedsSpecification) {
			return holdCandidate(candidate, err.Error())
		}
		return holdCandidate(candidate, "candidate identity could not be calculated")
	}
	candidate.Status = StatusIdentityReady
	candidate.HoldReason = ""
	return candidate
}

func holdCandidate(candidate Candidate, reason string) Candidate {
	candidate.Status = StatusNeedsSpecification
	candidate.HoldReason = reason
	return candidate
}

func doctorWorkSpec(issueType string) (legacyWorkSpec, bool) {
	issue := normalizeIssueType(issueType)
	switch issue {
	case "structured_data_missing", "schema_gap", "json_ld_missing", "schema_missing":
		return immediateSpec("schema.jsonld", "add", "jsonld"), true
	case "title_missing", "missing_title":
		return immediateSpec("metadata.title", "add", "title"), true
	case "title_duplicate", "duplicate_title", "title_too_long", "title_invalid", "metadata_title":
		return immediateSpec("metadata.title", "update", "title"), true
	case "meta_description_missing", "metadata_description":
		return immediateSpec("metadata.description", "add", "meta_description"), true
	case "h1_missing":
		return immediateSpec("content.heading", "add", "h1"), true
	case "canonical_missing":
		return immediateSpec("url.canonical", "add", "canonical"), true
	case "canonical_mismatch", "canonical_invalid", "canonical_multiple":
		return immediateSpec("url.canonical", "update", "canonical"), true
	case "robots_blocked", "robots_conflict", "noindex", "noindex_conflict":
		return immediateSpec("indexability.robots", "update", "robots"), true
	case "broken_url", "soft_404", "redirect_loop", "redirect_chain":
		return immediateSpec("availability.http", "update", "http_response"), true
	case "internal_link_gap", "zero_internal_links", "broken_internal_link", "orphan_page":
		return immediateSpec("links.internal", "add", "internal_link"), true
	case "important_page_missing_from_sitemap", "sitemap_update":
		return immediateSpec("discovery.sitemap", "add", "sitemap_entry"), true
	case "geo_crawler_access_blocked":
		return immediateSpec("indexability.ai_crawler", "update", "robots"), true
	case "unsafe_mdx_detected":
		return immediateSpec("rendering.template", "update", "unsafe_output"), true
	case "metadata_readability", "duplicate_metadata_template":
		return immediateSpec("metadata.title", "update", "title"), true
	case "supported_fact_extractability", "citation_readiness_structure":
		return immediateSpec("content.evidence", "move", "answer_block"), true
	case "source_association":
		return immediateSpec("content.evidence", "update", "source_association"), true
	case "entity_naming_consistency":
		return immediateSpec("content.entity", "update", "entity_name"), true
	case "ga4_missing", "tracking_missing", "measurement_readiness":
		return immediateSpec("measurement.instrumentation", "update", "tracking"), true
	default:
		return legacyWorkSpec{}, false
	}
}

func opportunityWorkSpec(opportunityType string) (legacyWorkSpec, bool) {
	if spec, ok := doctorWorkSpec(opportunityType); ok {
		return spec, true
	}
	issue := normalizeIssueType(opportunityType)
	switch issue {
	case "low_ctr", "low_ctr_snippet", "gsc_low_ctr", "gsc_low_ctr_query":
		return delayedSpec("metadata.title", "update", "title", ArtifactUpdateExistingContent, "ctr"), true
	case "geo_project_mentioned_without_citation", "geo_competitor_cited_project_absent", "ai_citation_gap", "weak_citation_surface":
		return delayedSpec("content.evidence", "update", "evidence_block", ArtifactUpdateExistingContent, "ai_citation"), true
	case "thin_evidence_page":
		return delayedSpec("content.evidence", "add", "evidence_block", ArtifactUpdateExistingContent, "ai_citation"), true
	case "citation_fact_expansion":
		return delayedSpec("content.evidence", "add", "supported_fact", ArtifactUpdateExistingContent, "ai_citation"), true
	case "gsc_query_gap", "query_gap", "striking_distance", "gsc_striking_distance_query":
		return delayedQuerySpec("content.query", "update", "page_content", ArtifactUpdateExistingContent, "search_visibility"), true
	case "content_decay", "content_decay_refresh", "gsc_content_decay":
		return delayedSpec("content.refresh", "update", "page_content", ArtifactUpdateExistingContent, "search_traffic"), true
	case "gsc_query_cannibalization":
		return delayedQuerySpec("content.consolidation", "consolidate", "pages", ArtifactConsolidateAssets, "search_visibility"), true
	case "cold_start_competitive_gap", "comparison_page", "alternative_page", "missing_use_case":
		return delayedSpec("content.new_asset", "create", "page", ArtifactCreateNewAsset, "search_and_citation"), true
	case "ranking_cluster_opportunity", "internal_link_strategy":
		return delayedSpec("links.strategy", "update", "internal_link_plan", ArtifactUpdateExistingContent, "rankings"), true
	default:
		return legacyWorkSpec{}, false
	}
}

func technicalVisibilityWorkSpec(evidence json.RawMessage) (legacyWorkSpec, bool) {
	switch normalizeIssueType(evidenceString(evidence, "issue")) {
	case "http_status":
		return immediateSpec("availability.http", "update", "http_response"), true
	case "robots_noindex", "robots_disallowed", "robots_blocked":
		return immediateSpec("indexability.robots", "update", "robots"), true
	case "canonical_missing":
		return immediateSpec("url.canonical", "add", "canonical"), true
	default:
		return legacyWorkSpec{}, false
	}
}

func immediateSpec(changeFamily, operation, field string) legacyWorkSpec {
	return legacyWorkSpec{
		changeFamily:     changeFamily,
		operation:        operation,
		field:            field,
		artifactIntent:   ArtifactRepairExistingSurface,
		owner:            OwnerDoctor,
		verificationMode: VerificationImmediate,
		primaryMetric:    "acceptance_test_pass",
	}
}

func delayedSpec(changeFamily, operation, field string, intent ArtifactIntent, metric string) legacyWorkSpec {
	return legacyWorkSpec{
		changeFamily:     changeFamily,
		operation:        operation,
		field:            field,
		artifactIntent:   intent,
		owner:            OwnerOpportunities,
		verificationMode: VerificationDelayed,
		primaryMetric:    metric,
	}
}

func delayedQuerySpec(changeFamily, operation, field string, intent ArtifactIntent, metric string) legacyWorkSpec {
	spec := delayedSpec(changeFamily, operation, field, intent, metric)
	spec.usesQueryIdentity = true
	return spec
}

func opportunitySourceKind(opportunity db.SeoOpportunity) SourceKind {
	if strings.HasPrefix(normalizeIssueType(opportunity.Type), "geo_") || evidenceString(opportunity.Evidence, "engine") != "" {
		return SourceAIDiscovery
	}
	return SourceSignalScan
}

func normalizeIssueType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("-", "_", " ", "_").Replace(value)
	return value
}

func rawStringList(raw json.RawMessage) []string {
	var values []string
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil {
		return nil
	}
	return normalizeTargetSet(values)
}

func evidenceString(raw json.RawMessage, key string) string {
	var value map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return ""
	}
	text, _ := value[key].(string)
	return strings.TrimSpace(text)
}

func fingerprintJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func numericFloat(value pgtype.Numeric) float64 {
	converted, err := value.Float64Value()
	if err != nil || !converted.Valid {
		return 0
	}
	return converted.Float64
}

func normalizeLegacyConfidence(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return 0
	}
	if value <= 1 {
		return value
	}
	if value <= 100 {
		return value / 100
	}
	return 1
}

func evidencePageTargets(raw json.RawMessage) []string {
	var value struct {
		CompetingPages []struct {
			NormalizedPageURL string `json:"normalized_page_url"`
			PageURL           string `json:"page_url"`
		} `json:"competing_pages"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return nil
	}
	targets := make([]string, 0, len(value.CompetingPages))
	for _, page := range value.CompetingPages {
		targets = append(targets, firstNonEmpty(page.NormalizedPageURL, page.PageURL))
	}
	return normalizeTargetSet(targets)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func nonZeroUUIDs(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && value != "00000000-0000-0000-0000-000000000000" {
			out = append(out, value)
		}
	}
	return out
}
