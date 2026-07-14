package platformcontract

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type Artifact struct {
	ContentMD string         `json:"content_md"`
	Metadata  map[string]any `json:"platform_metadata"`
}

type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ValidationReport struct {
	Passed   bool      `json:"passed"`
	Failures []Failure `json:"failures"`
	Warnings []Failure `json:"warnings"`
}

var mdxComponentPattern = regexp.MustCompile(`(?m)</?[A-Z][A-Za-z0-9_.-]*(?:\s[^>]*)?>`)

func Validate(contract ResolvedContract, artifact Artifact) ValidationReport {
	return ValidateAt(contract, artifact, time.Now().UTC())
}

func ValidateAt(contract ResolvedContract, artifact Artifact, now time.Time) ValidationReport {
	report := ValidationReport{Failures: []Failure{}, Warnings: []Failure{}}
	fail := func(code, message string) {
		report.Failures = append(report.Failures, Failure{Code: code, Message: message})
	}
	for _, field := range contract.RequiredFields {
		if strings.TrimSpace(metadataString(artifact.Metadata, field)) == "" {
			fail("missing_"+field, fmt.Sprintf("%s metadata is required", field))
		}
	}
	if contract.Rules.ForbidMDX && mdxComponentPattern.MatchString(artifact.ContentMD) {
		fail("mdx_not_supported", "platform output must be portable Markdown, not MDX")
	}
	if contract.Rules.RequiresCanonical && metadataString(artifact.Metadata, "canonical_url") != canonicalURLPlaceholder {
		fail("canonical_required", "canonical_url must reference the canonical article placeholder")
	}
	if contract.Rules.RequiresSourceLink && !strings.Contains(artifact.ContentMD, canonicalURLPlaceholder) {
		fail("source_link_required", "body must contain the canonical source placeholder")
	}
	if contract.Rules.LinkOnly && strings.TrimSpace(artifact.ContentMD) != "" {
		fail("link_only", "link submission must not include an article body or generated comment")
	}
	if contract.Platform == "hacker_news" {
		title := strings.ToLower(metadataString(artifact.Metadata, "title"))
		for _, promotional := range []string{"best", "amazing", "revolutionary", "game-changing", "#1"} {
			if strings.Contains(title, promotional) {
				fail("promotional_title", "Hacker News title must be factual and non-promotional")
				break
			}
		}
		if metadataString(artifact.Metadata, "url") != canonicalURLPlaceholder {
			fail("canonical_source_required", "Hacker News link must use the canonical URL placeholder")
		}
	}
	if contract.Platform == "reddit" {
		context := contract.TargetContext
		if context == nil || !TargetContextCurrent(context.Status, context.ExpiresAt, now) {
			fail("target_context_stale", "a current confirmed subreddit rules revision is required")
		} else {
			if metadataString(artifact.Metadata, "subreddit") != context.TargetKey {
				fail("subreddit_mismatch", "artifact subreddit must match the pinned target context")
			}
			postType := metadataString(artifact.Metadata, "post_type")
			if !contains(context.AllowedPostTypes, postType) {
				fail("post_type_not_allowed", "post type is not allowed by the pinned subreddit rules")
			}
			if context.RequiredFlair != "" && metadataString(artifact.Metadata, "flair") != context.RequiredFlair {
				fail("flair_required", "required subreddit flair is missing")
			}
		}
	}
	report.Passed = len(report.Failures) == 0
	return report
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}
