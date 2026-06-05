// Package config holds both process-level config (env) and the per-project
// config stored in projects.config jsonb (PRD §3).
package config

import (
	"encoding/json"
	"os"
)

// Env is process-level configuration sourced from the environment.
type Env struct {
	DatabaseURL     string
	Port            string
	AnthropicAPIKey string
	AnthropicModel  string
	SearchAPIKey    string // Brave Search API key (or swapped provider)
	GitHubToken     string // for BlogPublisher auto-commit (§5.6, option A)
	BlogRepo        string // "owner/name" of the blog repo
	BlogBranch      string // publish branch
	BlogBaseURL     string // public base for published canonical URLs
}

func FromEnv() Env {
	return Env{
		DatabaseURL:     getenv("DATABASE_URL", "postgres://localhost:5432/citeloop?sslmode=disable"),
		Port:            getenv("PORT", "8080"),
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		AnthropicModel:  getenv("ANTHROPIC_MODEL", "claude-opus-4-8"),
		SearchAPIKey:    os.Getenv("SEARCH_API_KEY"),
		GitHubToken:     os.Getenv("GITHUB_TOKEN"),
		BlogRepo:        os.Getenv("BLOG_REPO"),
		BlogBranch:      getenv("BLOG_BRANCH", "content-publish"),
		BlogBaseURL:     getenv("BLOG_BASE_URL", "https://unipost.example/blog"),
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
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
	CadencePerWeek   int         `json:"cadence_per_week"`
	BufferDays       int         `json:"buffer_days"`
	ChannelMix       ChannelMix  `json:"channel_mix"`
	BrandVoice       string      `json:"brand_voice"`
	MonthlyBudgetUSD float64     `json:"monthly_budget_usd"`
	Crawl            CrawlConfig `json:"crawl"`
}

// Default returns the PRD §3 example config values.
func Default() ProjectConfig {
	return ProjectConfig{
		CadencePerWeek:   3,
		BufferDays:       5,
		ChannelMix:       ChannelMix{Blog: 0.6, Syndication: 0.4},
		MonthlyBudgetUSD: 50,
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
	return c, nil
}

func (c ProjectConfig) JSON() json.RawMessage {
	b, _ := json.Marshal(c)
	return b
}
