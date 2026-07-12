package growthwork

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/discovery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func revalidateLegacyGrowthTarget(ctx context.Context, q *db.Queries, projectID, opportunityID uuid.UUID, expectedSnapshot string) error {
	if expectedSnapshot == "" {
		return discovery.ErrSnapshotStale
	}
	locked, err := q.LockLegacyGrowthIntendedTarget(ctx, db.LockLegacyGrowthIntendedTargetParams{
		ProjectID: projectID, OpportunityID: opportunityID,
	})
	if err != nil {
		return err
	}
	if lockedLegacyGrowthTargetSnapshot(locked) != expectedSnapshot {
		return discovery.ErrSnapshotStale
	}
	return nil
}

func resolveLegacyGrowthIntendedTarget(row db.GetLegacyGrowthIntendedTargetRow) string {
	return resolveLegacyGrowthTarget(legacyTargetStateFromGet(row))
}

func resolveLockedLegacyGrowthIntendedTarget(row db.LockLegacyGrowthIntendedTargetRow) string {
	return resolveLegacyGrowthTarget(legacyTargetStateFromLock(row))
}

type legacyTargetState struct {
	EvidenceIntended          string `json:"evidence_intended"`
	OpportunityPageURL        string `json:"opportunity_page_url"`
	ActionID                  string `json:"action_id"`
	ActionUpdatedAt           string `json:"action_updated_at"`
	ActionTargetURL           string `json:"action_target_url"`
	ActionNormalizedTargetURL string `json:"action_normalized_target_url"`
	ArticleID                 string `json:"article_id"`
	ArticleContentHash        string `json:"article_content_hash"`
	ArticlePublishedAt        string `json:"article_published_at"`
	ArticleCanonicalURL       string `json:"article_canonical_url"`
	ArticleExternalURL        string `json:"article_external_url"`
	SEOCanonicalURL           string `json:"seo_canonical_url"`
	SEOSlug                   string `json:"seo_slug"`
}

func legacyTargetStateFromGet(row db.GetLegacyGrowthIntendedTargetRow) legacyTargetState {
	return newLegacyTargetState(
		row.EvidenceIntended, row.OpportunityPageUrl, uuidText(row.ActionID), row.ActionUpdatedAt,
		row.ActionTargetUrl, row.ActionNormalizedTargetUrl, row.ArticleID, row.ArticleContentHash,
		row.ArticlePublishedAt, row.ArticleCanonicalUrl, row.ArticleExternalUrl, row.SeoCanonicalUrl, row.SeoSlug,
	)
}

func legacyTargetStateFromLock(row db.LockLegacyGrowthIntendedTargetRow) legacyTargetState {
	return newLegacyTargetState(
		row.EvidenceIntended, row.OpportunityPageUrl, uuidText(row.ActionID), row.ActionUpdatedAt,
		row.ActionTargetUrl, row.ActionNormalizedTargetUrl, row.ArticleID, row.ArticleContentHash,
		row.ArticlePublishedAt, row.ArticleCanonicalUrl, row.ArticleExternalUrl, row.SeoCanonicalUrl, row.SeoSlug,
	)
}

func newLegacyTargetState(
	evidence string, opportunityURL *string, actionID string, actionUpdated pgtype.Timestamptz,
	actionURL, normalizedActionURL *string, articleID uuid.UUID, articleContentHash *string,
	articlePublished pgtype.Timestamptz, articleCanonicalURL, articleExternalURL *string, seoCanonicalURL, seoSlug string,
) legacyTargetState {
	return legacyTargetState{
		EvidenceIntended: strings.TrimSpace(evidence), OpportunityPageURL: pointerText(opportunityURL),
		ActionID: actionID, ActionUpdatedAt: pgTimeString(actionUpdated),
		ActionTargetURL: pointerText(actionURL), ActionNormalizedTargetURL: pointerText(normalizedActionURL),
		ArticleID: uuidText(articleID), ArticleContentHash: pointerText(articleContentHash),
		ArticlePublishedAt: pgTimeString(articlePublished), ArticleCanonicalURL: pointerText(articleCanonicalURL),
		ArticleExternalURL: pointerText(articleExternalURL), SEOCanonicalURL: strings.TrimSpace(seoCanonicalURL), SEOSlug: strings.TrimSpace(seoSlug),
	}
}

func resolveLegacyGrowthTarget(state legacyTargetState) string {
	if value := state.EvidenceIntended; value != "" {
		return value
	}
	base := firstNonEmpty(state.ActionNormalizedTargetURL, state.ActionTargetURL, state.OpportunityPageURL)
	for _, value := range []string{
		state.ArticleCanonicalURL,
		state.ArticleExternalURL,
		state.SEOCanonicalURL,
		state.SEOSlug,
	} {
		if resolved := resolveLegacyGrowthTargetAgainstBase(value, base); resolved != "" {
			return resolved
		}
	}
	return ""
}

func legacyGrowthTargetSnapshot(state legacyTargetState) string {
	raw, _ := json.Marshal(state)
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum[:])
}

func getLegacyGrowthTargetSnapshot(row db.GetLegacyGrowthIntendedTargetRow) string {
	return legacyGrowthTargetSnapshot(legacyTargetStateFromGet(row))
}

func lockedLegacyGrowthTargetSnapshot(row db.LockLegacyGrowthIntendedTargetRow) string {
	return legacyGrowthTargetSnapshot(legacyTargetStateFromLock(row))
}

func resolveLegacyGrowthTargetAgainstBase(value, base string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.IsAbs() && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		return parsed.String()
	}
	baseURL, err := url.Parse(strings.TrimSpace(base))
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return ""
	}
	path := "/" + strings.TrimLeft(value, "/")
	return (&url.URL{Scheme: baseURL.Scheme, Host: baseURL.Host, Path: path}).String()
}

func enrichLegacyGrowthEvidence(raw json.RawMessage, intendedTarget string) (json.RawMessage, error) {
	intendedTarget = strings.TrimSpace(intendedTarget)
	if intendedTarget == "" {
		return raw, nil
	}
	evidence := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &evidence); err != nil {
			return nil, err
		}
	}
	if current, _ := evidence["intended_slug_or_canonical"].(string); strings.TrimSpace(current) != "" {
		return raw, nil
	}
	evidence["intended_slug_or_canonical"] = intendedTarget
	evidence["intended_target_provenance"] = "legacy_execution_artifact"
	return json.Marshal(evidence)
}

func firstNonEmptyPointer(values ...*string) string {
	for _, value := range values {
		if text := pointerText(value); text != "" {
			return text
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func uuidText(value uuid.UUID) string {
	if value == uuid.Nil {
		return ""
	}
	return value.String()
}

func pgTimeString(value pgtype.Timestamptz) string {
	if !value.Valid {
		return ""
	}
	return value.Time.UTC().Format(time.RFC3339Nano)
}

func pointerText(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
