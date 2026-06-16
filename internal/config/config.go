// Package config holds both process-level config (env) and the per-project
// config stored in projects.config jsonb (PRD §3).
package config

import (
	"encoding/json"
	"os"
	"strconv"
	"time"
)

// Env is process-level configuration sourced from the environment.
type Env struct {
	DatabaseURL              string
	Port                     string
	TokenGateAPIKey          string
	TokenGateBaseURL         string
	TokenGateModel           string
	AnthropicAPIKey          string
	AnthropicModel           string
	ClerkSecretKey           string
	AdminUserIDs             string // comma-separated Clerk user IDs allowed to manage global admin settings
	SearchAPIKey             string // Brave Search API key (or swapped provider)
	GitHubToken              string // for BlogPublisher auto-commit (§5.6, option A)
	BlogRepo                 string // "owner/name" of the blog repo
	BlogBranch               string // publish branch
	BlogContentDir           string // generated MDX root inside the blog repo
	BlogBaseURL              string // public base for published canonical URLs
	UniPostDeployHookURL     string // Vercel deploy hook for UniPost build-time content fetch
	NotificationSecretKey    string // AEAD key material for webhook URL encryption
	GoogleServiceAccountJSON string // service account JSON for GSC/GA4 read-only ingestion
	PerplexityAPIKey         string // Perplexity Sonar API key for legal answer-engine observation
	PerplexityBaseURL        string // Perplexity API base URL
	PerplexityModel          string // Perplexity Sonar model
	GEOProviderRunBudgetUSD  float64
}

func FromEnv() Env {
	return Env{
		DatabaseURL:              getenv("DATABASE_URL", "postgres://localhost:5432/citeloop?sslmode=disable"),
		Port:                     getenv("PORT", "8080"),
		TokenGateAPIKey:          os.Getenv("TOKENGATE_API_KEY"),
		TokenGateBaseURL:         getenv("TOKENGATE_BASE_URL", "https://tokengate-production.up.railway.app/v1"),
		TokenGateModel:           getenv("TOKENGATE_MODEL", "claude-haiku-4-5-20251001"),
		AnthropicAPIKey:          os.Getenv("ANTHROPIC_API_KEY"),
		AnthropicModel:           getenv("ANTHROPIC_MODEL", "claude-opus-4-8"),
		ClerkSecretKey:           os.Getenv("CLERK_SECRET_KEY"),
		AdminUserIDs:             os.Getenv("CITELOOP_ADMIN_USER_IDS"),
		SearchAPIKey:             os.Getenv("SEARCH_API_KEY"),
		GitHubToken:              os.Getenv("GITHUB_TOKEN"),
		BlogRepo:                 os.Getenv("BLOG_REPO"),
		BlogBranch:               getenv("BLOG_BRANCH", "citeloop-content"),
		BlogContentDir:           getenv("BLOG_CONTENT_DIR", "content/citeloop/blog"),
		BlogBaseURL:              getenv("BLOG_BASE_URL", "https://unipost.example/blog"),
		UniPostDeployHookURL:     os.Getenv("UNIPOST_DEPLOY_HOOK_URL"),
		NotificationSecretKey:    os.Getenv("NOTIFICATION_SECRET_KEY"),
		GoogleServiceAccountJSON: os.Getenv("GOOGLE_SERVICE_ACCOUNT_JSON"),
		PerplexityAPIKey:         os.Getenv("PERPLEXITY_API_KEY"),
		PerplexityBaseURL:        getenv("PERPLEXITY_BASE_URL", "https://api.perplexity.ai"),
		PerplexityModel:          getenv("PERPLEXITY_MODEL", "sonar-pro"),
		GEOProviderRunBudgetUSD:  getenvFloat("GEO_PROVIDER_RUN_BUDGET_USD", 1),
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getenvFloat(k string, def float64) float64 {
	if v := os.Getenv(k); v != "" {
		parsed, err := strconv.ParseFloat(v, 64)
		if err == nil {
			return parsed
		}
	}
	return def
}

// CrawlConfig mirrors projects.config.crawl (PRD §3 / §5.1).
type CrawlConfig struct {
	SameOriginOnly   bool `json:"same_origin_only"`
	MaxPages         int  `json:"max_pages"`
	MaxDepth         int  `json:"max_depth"`
	RequestTimeoutMs int  `json:"request_timeout_ms"`
	RateLimitRPS     int  `json:"rate_limit_rps"`
	RespectRobots    bool `json:"respect_robots"`
	SitemapURLCap    int  `json:"sitemap_url_cap"`
}

// ChannelMix is the blog/syndication split used by the Strategist (§3).
type ChannelMix struct {
	Blog        float64 `json:"blog"`
	Syndication float64 `json:"syndication"`
}

// ProjectConfig mirrors projects.config (PRD §3).
type ProjectConfig struct {
	SiteURL            string      `json:"site_url,omitempty"`
	CadencePerWeek     int         `json:"cadence_per_week"`
	BufferDays         int         `json:"buffer_days"`
	ChannelMix         ChannelMix  `json:"channel_mix"`
	BrandVoice         string      `json:"brand_voice"`
	MonthlyBudgetUSD   float64     `json:"monthly_budget_usd"`
	AutoAdvanceEnabled bool        `json:"auto_advance_enabled"`
	// PublishMode controls how approved canonicals reach the live blog:
	//   "scheduled" (default) — staggered one every PublishIntervalDays so a batch
	//     of approvals does not publish all at once;
	//   "auto" — publish as soon as due;
	//   "manual" — wait on the Publish page until the operator publishes/schedules.
	PublishMode         string      `json:"publish_mode"`
	PublishIntervalDays int         `json:"publish_interval_days"`
	Crawl               CrawlConfig `json:"crawl"`
}

// Publish mode values.
const (
	PublishModeScheduled = "scheduled"
	PublishModeAuto      = "auto"
	PublishModeManual    = "manual"
)

// Default returns the PRD §3 example config values.
func Default() ProjectConfig {
	return ProjectConfig{
		CadencePerWeek:     3,
		BufferDays:         5,
		ChannelMix:         ChannelMix{Blog: 0.6, Syndication: 0.4},
		MonthlyBudgetUSD:    50,
		AutoAdvanceEnabled:  true,
		PublishMode:         PublishModeScheduled,
		PublishIntervalDays: 2,
		Crawl: CrawlConfig{
			SameOriginOnly:   true,
			MaxPages:         200,
			MaxDepth:         3,
			RequestTimeoutMs: 8000,
			RateLimitRPS:     1,
			RespectRobots:    true,
			SitemapURLCap:    2000,
		},
	}
}

// Parse decodes a projects.config jsonb payload, filling defaults for zero values.
func Parse(raw json.RawMessage) (ProjectConfig, error) {
	// Start from defaults, then unmarshal the stored config on top. Go's JSON
	// decoder only overwrites fields present in the payload (including nested
	// struct fields), so absent fields keep their default while an explicit 0
	// (e.g. buffer_days:0 = publish immediately) is honored.
	c := Default()
	if len(raw) == 0 || string(raw) == "{}" {
		return c, nil
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, err
	}
	switch c.PublishMode {
	case PublishModeScheduled, PublishModeAuto, PublishModeManual:
	default:
		c.PublishMode = PublishModeScheduled
	}
	if c.PublishIntervalDays <= 0 {
		c.PublishIntervalDays = 2
	}
	return c, nil
}

func (c ProjectConfig) JSON() json.RawMessage {
	b, _ := json.Marshal(c)
	return b
}

// NextPublishSlot decides when a freshly approved canonical should publish, given
// the latest slot already taken by the project's other canonicals. It is the one
// place the publish cadence lives, used by both the auto-approve loop and the
// manual approve handler so a batch of approvals never publishes all at once.
//   - manual: no automatic schedule (ok=false) — the operator publishes/schedules;
//   - auto:   publish now;
//   - scheduled (default): one every PublishIntervalDays after the last slot,
//     never in the past.
func (c ProjectConfig) NextPublishSlot(latest, now time.Time) (time.Time, bool) {
	switch c.PublishMode {
	case PublishModeManual:
		return time.Time{}, false
	case PublishModeAuto:
		return now, true
	default:
		interval := c.PublishIntervalDays
		if interval <= 0 {
			interval = 2
		}
		candidate := latest.AddDate(0, 0, interval)
		if candidate.Before(now) {
			candidate = now
		}
		return candidate, true
	}
}
