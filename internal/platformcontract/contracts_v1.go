package platformcontract

import "time"

const canonicalURLPlaceholder = "{{CANONICAL_URL}}"

type ResolvedContract struct {
	Platform       string              `json:"platform"`
	Version        string              `json:"version"`
	AssetType      string              `json:"asset_type"`
	OutputType     string              `json:"output_type"`
	Prompt         string              `json:"prompt"`
	RequiredFields []string            `json:"required_fields"`
	Rules          RuleSet             `json:"rules"`
	TargetContext  *TargetContextRules `json:"target_context,omitempty"`
}

type RuleSet struct {
	RequiresCanonical  bool `json:"requires_canonical"`
	RequiresSourceLink bool `json:"requires_source_link"`
	ForbidMDX          bool `json:"forbid_mdx"`
	LinkOnly           bool `json:"link_only"`
}

type TargetContextRules struct {
	TargetKey        string    `json:"target_key"`
	Status           string    `json:"status"`
	ExpiresAt        time.Time `json:"expires_at"`
	AllowedPostTypes []string  `json:"allowed_post_types"`
	RequiredFlair    string    `json:"required_flair,omitempty"`
	LinkPolicy       string    `json:"link_policy"`
}

func ContractsV1() map[string]ResolvedContract {
	return map[string]ResolvedContract{
		"blog": {
			Platform: "blog", Version: "platform-contract-v1", OutputType: "long_form_article",
			Prompt:         "Write the canonical CiteLoop blog article as clean Markdown/MDX with SEO metadata, evidence-led sections, and useful visual insertion points.",
			RequiredFields: []string{"title", "slug"},
		},
		"dev_to": {
			Platform: "dev_to", Version: "platform-contract-v1", OutputType: "long_form_article",
			Prompt:         "Write a Dev.to-native Markdown article for developers: concise opening, practical sections, code-safe formatting, tags, and rel-canonical metadata. Do not use MDX components.",
			RequiredFields: []string{"title", "canonical_url"}, Rules: RuleSet{RequiresCanonical: true, ForbidMDX: true},
		},
		"hashnode": {
			Platform: "hashnode", Version: "platform-contract-v1", OutputType: "long_form_article",
			Prompt:         "Write a Hashnode publication article with developer-focused Markdown, publication context, tags, subtitle, and original-article canonical URL.",
			RequiredFields: []string{"title", "canonical_url", "publication"}, Rules: RuleSet{RequiresCanonical: true, ForbidMDX: true},
		},
		"medium": {
			Platform: "medium", Version: "platform-contract-v1", OutputType: "long_form_article",
			Prompt:         "Write a Medium-native story with a narrative opening, readable short sections, title/subtitle, and canonical link to the original article.",
			RequiredFields: []string{"title", "canonical_url"}, Rules: RuleSet{RequiresCanonical: true, ForbidMDX: true},
		},
		"linkedin": {
			Platform: "linkedin", Version: "platform-contract-v1", OutputType: "long_form_article",
			Prompt:         "Write a LinkedIn article for a professional audience with title, description, skimmable body, practical takeaways, and an explicit source link.",
			RequiredFields: []string{"title", "description"}, Rules: RuleSet{RequiresSourceLink: true, ForbidMDX: true},
		},
		"reddit": {
			Platform: "reddit", Version: "platform-contract-v1", OutputType: "community_post",
			Prompt:         "Write a subreddit-specific Reddit post that follows the pinned rules revision, uses the allowed post type and flair, discloses affiliation, avoids generic promotion, and links to the source only as permitted.",
			RequiredFields: []string{"title", "post_type", "subreddit"}, Rules: RuleSet{RequiresSourceLink: true, ForbidMDX: true},
		},
		"hacker_news": {
			Platform: "hacker_news", Version: "platform-contract-v1", OutputType: "link_submission",
			Prompt:         "Create only a Hacker News link-submission package: a factual, non-promotional title and the canonical URL. Do not generate a comment or article body.",
			RequiredFields: []string{"title", "url"}, Rules: RuleSet{LinkOnly: true},
		},
	}
}
