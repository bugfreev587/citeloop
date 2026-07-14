package platformcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

const targetContextValidity = 30 * 24 * time.Hour

type ConfirmTargetContextInput struct {
	Platform               string   `json:"platform"`
	TargetKey              string   `json:"target_key"`
	SourceURL              string   `json:"source_url"`
	RulesURL               string   `json:"rules_url"`
	RulesText              string   `json:"rules_text"`
	AllowedPostTypes       []string `json:"allowed_post_types"`
	RequiredFlair          string   `json:"required_flair"`
	LinkPolicy             string   `json:"link_policy"`
	SelfPromotionPolicy    string   `json:"self_promotion_policy"`
	DisclosureRequirements string   `json:"disclosure_requirements"`
	Notes                  string   `json:"notes"`
	Verified               bool     `json:"verified"`
}

type PreparedTargetContext struct {
	ConfirmTargetContextInput
	Version     int32
	SourceKind  string
	ContentHash string
	ConfirmedAt time.Time
	ExpiresAt   time.Time
}

func PrepareTargetContext(input ConfirmTargetContextInput, priorVersion int32, now time.Time) (PreparedTargetContext, error) {
	input.Platform = strings.ToLower(strings.TrimSpace(input.Platform))
	if !input.Verified {
		return PreparedTargetContext{}, fmt.Errorf("target context must be explicitly verified")
	}
	sourceKind := "user_pasted_rules"
	switch input.Platform {
	case "reddit":
		input.TargetKey = normalizeRedditTarget(input.TargetKey)
		if input.TargetKey == "" {
			return PreparedTargetContext{}, fmt.Errorf("reddit target is required")
		}
	case "hashnode":
		input.TargetKey = strings.TrimSpace(input.TargetKey)
		input.SourceURL = strings.TrimSpace(input.SourceURL)
		if input.TargetKey == "" || input.SourceURL == "" {
			return PreparedTargetContext{}, fmt.Errorf("hashnode publication key and URL are required")
		}
		parsed, err := url.Parse(input.SourceURL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return PreparedTargetContext{}, fmt.Errorf("hashnode publication URL must be an https URL")
		}
		input.RulesText = "User-confirmed Hashnode publication: " + input.TargetKey
		input.AllowedPostTypes = []string{"long_form_article"}
		input.LinkPolicy = "rel=canonical required"
		input.SelfPromotionPolicy = "publication owner confirmed"
		sourceKind = "user_confirmed_rules"
	default:
		return PreparedTargetContext{}, fmt.Errorf("target context is only supported for hashnode and reddit")
	}
	input.RulesText = strings.TrimSpace(input.RulesText)
	input.LinkPolicy = strings.TrimSpace(input.LinkPolicy)
	input.SelfPromotionPolicy = strings.TrimSpace(input.SelfPromotionPolicy)
	if input.RulesText == "" || input.LinkPolicy == "" || input.SelfPromotionPolicy == "" {
		return PreparedTargetContext{}, fmt.Errorf("rules, link policy, and self-promotion policy are required")
	}
	input.AllowedPostTypes = normalizePostTypes(input.Platform, input.AllowedPostTypes)
	if len(input.AllowedPostTypes) == 0 {
		return PreparedTargetContext{}, fmt.Errorf("at least one allowed post type is required")
	}
	input.SourceURL = strings.TrimSpace(input.SourceURL)
	input.RulesURL = strings.TrimSpace(input.RulesURL)
	input.RequiredFlair = strings.TrimSpace(input.RequiredFlair)
	input.DisclosureRequirements = strings.TrimSpace(input.DisclosureRequirements)
	input.Notes = strings.TrimSpace(input.Notes)
	now = now.UTC()

	hashInput, err := json.Marshal(input)
	if err != nil {
		return PreparedTargetContext{}, fmt.Errorf("encode target context: %w", err)
	}
	sum := sha256.Sum256(hashInput)
	return PreparedTargetContext{
		ConfirmTargetContextInput: input,
		Version:                   priorVersion + 1,
		SourceKind:                sourceKind,
		ContentHash:               hex.EncodeToString(sum[:]),
		ConfirmedAt:               now,
		ExpiresAt:                 now.Add(targetContextValidity),
	}, nil
}

func TargetContextCurrent(status string, expiresAt, now time.Time) bool {
	return status == "confirmed" && expiresAt.After(now)
}

func normalizeRedditTarget(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if parsed, err := url.Parse(raw); err == nil && parsed.Host != "" {
		raw = parsed.Path
	}
	raw = strings.Trim(raw, "/")
	parts := strings.Split(raw, "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "r" && strings.TrimSpace(parts[i+1]) != "" {
			return "r/" + strings.TrimSpace(parts[i+1])
		}
	}
	if raw != "" && !strings.Contains(raw, "/") {
		return "r/" + raw
	}
	return ""
}

func normalizeValues(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			seen[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func normalizePostTypes(platform string, values []string) []string {
	if platform == "reddit" {
		for i, value := range values {
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "text":
				values[i] = "community_post"
			case "link":
				values[i] = "link_submission"
			}
		}
	}
	return normalizeValues(values)
}
