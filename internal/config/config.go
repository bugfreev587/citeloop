// Package config holds both process-level config (env) and the per-project
// config stored in projects.config jsonb (PRD §3).
package config

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"
)

// Env is process-level configuration sourced from the environment.
type Env struct {
	DatabaseURL              string
	Environment              string
	Port                     string
	TokenGateAPIKey          string
	TokenGateBaseURL         string
	TokenGateModel           string
	ClerkSecretKey           string
	AdminEmails              string // comma-separated admin email addresses (ADMINS env) allowed to manage global admin settings
	SearchAPIKey             string // Brave Search API key (or swapped provider)
	GitHubToken              string // for BlogPublisher auto-commit (§5.6, option A)
	BlogRepo                 string // "owner/name" of the blog repo
	BlogBranch               string // publish branch
	BlogContentDir           string // generated MDX root inside the blog repo
	BlogBaseURL              string // public base for published canonical URLs
	UniPostDeployHookURL     string // Vercel deploy hook for UniPost build-time content fetch
	NotificationSecretKey    string // AEAD key material for webhook URL encryption
	ResendAPIKey             string // Resend API key for platform-owned Email notifications
	NotificationEmailFrom    string // verified sender for platform-owned Email notifications
	NotificationEmailReplyTo string // optional reply-to for Email notifications
	GoogleServiceAccountJSON string // service account JSON for GSC/GA4 read-only ingestion
	GoogleOAuthClientID      string // Google OAuth client ID for customer-owned GSC connections
	GoogleOAuthClientSecret  string // Google OAuth client secret for customer-owned GSC connections
	PublicAppURL             string // public web app URL used for OAuth redirect validation
	PublicAPIURL             string // public API origin used for stable article asset URLs
	PerplexityAPIKey         string // Perplexity Sonar API key for legal answer-engine observation
	PerplexityBaseURL        string // Perplexity API base URL
	PerplexityModel          string // Perplexity Sonar model
	GEOProviderRunBudgetUSD  float64
	// GitHub App (Railway-style publish connect). Inert until all are set.
	GitHubAppID           string
	GitHubAppSlug         string
	GitHubAppClientID     string
	GitHubAppClientSecret string
	GitHubAppPrivateKey   string
}

func FromEnv() Env {
	return Env{
		DatabaseURL:              getenv("DATABASE_URL", "postgres://localhost:5432/citeloop?sslmode=disable"),
		Environment:              firstEnv("RAILWAY_ENVIRONMENT_NAME", "RAILWAY_ENVIRONMENT", "APP_ENV", "GO_ENV"),
		Port:                     getenv("PORT", "8080"),
		TokenGateAPIKey:          os.Getenv("TOKENGATE_API_KEY"),
		TokenGateBaseURL:         getenv("TOKENGATE_BASE_URL", "https://tokengate-production.up.railway.app/v1"),
		TokenGateModel:           getenv("TOKENGATE_MODEL", "claude-sonnet-4-6"),
		ClerkSecretKey:           os.Getenv("CLERK_SECRET_KEY"),
		AdminEmails:              os.Getenv("ADMINS"),
		SearchAPIKey:             os.Getenv("SEARCH_API_KEY"),
		GitHubToken:              os.Getenv("GITHUB_TOKEN"),
		BlogRepo:                 os.Getenv("BLOG_REPO"),
		BlogBranch:               getenv("BLOG_BRANCH", "citeloop-content"),
		BlogContentDir:           getenv("BLOG_CONTENT_DIR", "content/citeloop/blog"),
		BlogBaseURL:              getenv("BLOG_BASE_URL", "https://unipost.example/blog"),
		UniPostDeployHookURL:     os.Getenv("UNIPOST_DEPLOY_HOOK_URL"),
		NotificationSecretKey:    os.Getenv("NOTIFICATION_SECRET_KEY"),
		ResendAPIKey:             os.Getenv("RESEND_API_KEY"),
		NotificationEmailFrom:    os.Getenv("NOTIFICATION_EMAIL_FROM"),
		NotificationEmailReplyTo: os.Getenv("NOTIFICATION_EMAIL_REPLY_TO"),
		GoogleServiceAccountJSON: os.Getenv("GOOGLE_SERVICE_ACCOUNT_JSON"),
		GoogleOAuthClientID:      os.Getenv("GOOGLE_OAUTH_CLIENT_ID"),
		GoogleOAuthClientSecret:  os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET"),
		PublicAppURL:             os.Getenv("PUBLIC_APP_URL"),
		PublicAPIURL:             publicAPIURL(),
		PerplexityAPIKey:         os.Getenv("PERPLEXITY_API_KEY"),
		PerplexityBaseURL:        getenv("PERPLEXITY_BASE_URL", "https://api.perplexity.ai"),
		PerplexityModel:          getenv("PERPLEXITY_MODEL", "sonar-pro"),
		GEOProviderRunBudgetUSD:  getenvFloat("GEO_PROVIDER_RUN_BUDGET_USD", 1),
		GitHubAppID:              os.Getenv("GITHUB_APP_ID"),
		GitHubAppSlug:            os.Getenv("GITHUB_APP_SLUG"),
		GitHubAppClientID:        os.Getenv("GITHUB_APP_CLIENT_ID"),
		GitHubAppClientSecret:    os.Getenv("GITHUB_APP_CLIENT_SECRET"),
		GitHubAppPrivateKey:      os.Getenv("GITHUB_APP_PRIVATE_KEY"),
	}
}

func publicAPIURL() string {
	if value := strings.TrimRight(os.Getenv("PUBLIC_API_URL"), "/"); value != "" {
		return value
	}
	if domain := strings.TrimSpace(os.Getenv("RAILWAY_PUBLIC_DOMAIN")); domain != "" {
		return "https://" + domain
	}
	return ""
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
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
	SiteURL            string     `json:"site_url,omitempty"`
	CadencePerWeek     int        `json:"cadence_per_week"`
	BufferDays         int        `json:"buffer_days"`
	ChannelMix         ChannelMix `json:"channel_mix"`
	BrandVoice         string     `json:"brand_voice"`
	MonthlyBudgetUSD   float64    `json:"monthly_budget_usd"`
	AutoAdvanceEnabled bool       `json:"auto_advance_enabled"`
	// CapabilityPolicyVersion marks configs that use independent Doctor and
	// Opportunities execution authority.
	CapabilityPolicyVersion int             `json:"capability_policy_version"`
	GrowthSignalEnabled     bool            `json:"growth_signal_enabled"`
	GrowthAIEnabled         bool            `json:"growth_ai_enabled"`
	GrowthAIRunPolicy       string          `json:"growth_ai_run_policy"`
	GrowthRadarMode         GrowthRadarMode `json:"growth_radar_mode"`
	DoctorAIEnabled         bool            `json:"doctor_ai_enabled"`
	DoctorAIRunPolicy       string          `json:"doctor_ai_run_policy"`
	// PublishMode controls how approved canonicals reach the live blog:
	//   "manual" (default) — wait on the Publish page until the operator publishes/schedules;
	//   "scheduled" — staggered one every PublishIntervalDays so a batch
	//     of approvals does not publish all at once;
	//   "auto" — legacy value normalized to scheduled.
	PublishMode             string      `json:"publish_mode"`
	PublishIntervalDays     int         `json:"publish_interval_days"`
	ImageDailyCountBudget   int         `json:"image_daily_count_budget"`
	ImageDailyCostBudgetUSD float64     `json:"image_daily_cost_budget_usd"`
	Crawl                   CrawlConfig `json:"crawl"`
}

type GrowthRadarMode string

const (
	GrowthRadarOff     GrowthRadarMode = "off"
	GrowthRadarObserve GrowthRadarMode = "observe_only"
	GrowthRadarCreate  GrowthRadarMode = "create_opportunities"
)

// Publish mode values.
const (
	PublishModeScheduled = "scheduled"
	PublishModeAuto      = "auto"
	PublishModeManual    = "manual"
)

const (
	CapabilityPolicyVersionV1 = 1

	DoctorAIRunPolicyManualOnly = "manual_only"
	DoctorAIRunPolicyOnDemand   = "on_demand"
	DoctorAIRunPolicyAutomatic  = "automatic"
)

const (
	GrowthAIRunPolicyScheduledOnly       = "scheduled_only"
	GrowthAIRunPolicyScheduledAndEvent   = "scheduled_and_event"
	GrowthAIRunPolicyOnDemandRecommended = "on_demand_recommended"
	GrowthAIRunPolicyManualOnly          = "manual_only"
)

type DoctorAITrigger string

const (
	DoctorAITriggerDiagnosisUser         DoctorAITrigger = "diagnosis_user"
	DoctorAITriggerDiagnosisScheduler    DoctorAITrigger = "diagnosis_scheduler"
	DoctorAITriggerApplyUser             DoctorAITrigger = "apply_user"
	DoctorAITriggerVerificationUser      DoctorAITrigger = "verification_user"
	DoctorAITriggerVerificationScheduler DoctorAITrigger = "verification_scheduler"
)

type GrowthAITrigger string

const (
	GrowthAITriggerManual    GrowthAITrigger = "manual"
	GrowthAITriggerScheduled GrowthAITrigger = "scheduled"
	GrowthAITriggerEvent     GrowthAITrigger = "event"
)

type OpportunityFindingStages struct {
	SignalScan  bool
	AIDiscovery bool
}

// Default returns the PRD §3 example config values.
func Default() ProjectConfig {
	return ProjectConfig{
		CadencePerWeek:          3,
		BufferDays:              5,
		ChannelMix:              ChannelMix{Blog: 0.6, Syndication: 0.4},
		MonthlyBudgetUSD:        50,
		AutoAdvanceEnabled:      false,
		CapabilityPolicyVersion: CapabilityPolicyVersionV1,
		GrowthSignalEnabled:     true,
		GrowthAIEnabled:         true,
		GrowthAIRunPolicy:       GrowthAIRunPolicyOnDemandRecommended,
		GrowthRadarMode:         GrowthRadarObserve,
		DoctorAIEnabled:         false,
		DoctorAIRunPolicy:       DoctorAIRunPolicyManualOnly,
		PublishMode:             PublishModeManual,
		PublishIntervalDays:     2,
		ImageDailyCountBudget:   2,
		ImageDailyCostBudgetUSD: 0.20,
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

func (c ProjectConfig) AllowsGrowthAI(trigger GrowthAITrigger) bool {
	if !c.GrowthAIEnabled {
		return false
	}
	switch trigger {
	case GrowthAITriggerManual:
		return c.GrowthAIRunPolicy == GrowthAIRunPolicyManualOnly ||
			c.GrowthAIRunPolicy == GrowthAIRunPolicyOnDemandRecommended ||
			c.GrowthAIRunPolicy == GrowthAIRunPolicyScheduledOnly ||
			c.GrowthAIRunPolicy == GrowthAIRunPolicyScheduledAndEvent
	case GrowthAITriggerScheduled:
		return c.GrowthAIRunPolicy == GrowthAIRunPolicyScheduledOnly || c.GrowthAIRunPolicy == GrowthAIRunPolicyScheduledAndEvent
	case GrowthAITriggerEvent:
		return c.GrowthAIRunPolicy == GrowthAIRunPolicyScheduledAndEvent
	default:
		return false
	}
}

func (c ProjectConfig) AllowsDoctorAI(trigger DoctorAITrigger) bool {
	if !c.DoctorAIEnabled {
		return false
	}
	switch trigger {
	case DoctorAITriggerDiagnosisUser, DoctorAITriggerApplyUser:
		return c.DoctorAIRunPolicy == DoctorAIRunPolicyManualOnly || c.DoctorAIRunPolicy == DoctorAIRunPolicyOnDemand || c.DoctorAIRunPolicy == DoctorAIRunPolicyAutomatic
	case DoctorAITriggerVerificationUser:
		return c.DoctorAIRunPolicy == DoctorAIRunPolicyManualOnly || c.DoctorAIRunPolicy == DoctorAIRunPolicyOnDemand || c.DoctorAIRunPolicy == DoctorAIRunPolicyAutomatic
	case DoctorAITriggerDiagnosisScheduler, DoctorAITriggerVerificationScheduler:
		return c.DoctorAIRunPolicy == DoctorAIRunPolicyAutomatic
	default:
		return false
	}
}

func (c ProjectConfig) OpportunityFindingStages(automatic bool) OpportunityFindingStages {
	trigger := GrowthAITriggerManual
	if automatic {
		trigger = GrowthAITriggerScheduled
	}
	return c.OpportunityFindingStagesForTrigger(trigger)
}

func (c ProjectConfig) OpportunityFindingStagesForTrigger(trigger GrowthAITrigger) OpportunityFindingStages {
	return OpportunityFindingStages{
		SignalScan:  c.GrowthSignalEnabled,
		AIDiscovery: c.AllowsGrowthAI(trigger),
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
	stored := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &stored); err != nil {
		return c, err
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, err
	}
	legacyAuto := c.PublishMode == PublishModeAuto
	switch c.PublishMode {
	case PublishModeScheduled, PublishModeManual:
	case PublishModeAuto:
		c.PublishMode = PublishModeScheduled
	default:
		c.PublishMode = PublishModeManual
	}
	if c.PublishIntervalDays <= 0 {
		if legacyAuto {
			c.PublishIntervalDays = 1
		} else {
			c.PublishIntervalDays = 2
		}
	}
	c.CapabilityPolicyVersion = CapabilityPolicyVersionV1
	// Incomplete post-migration payloads keep deterministic evidence enabled but
	// fail closed for provider calls. Normal settings saves merge over the full
	// stored config and therefore retain explicit authority fields.
	if _, ok := stored["growth_ai_enabled"]; !ok {
		c.GrowthAIEnabled = false
	}
	if _, ok := stored["doctor_ai_enabled"]; !ok {
		c.DoctorAIEnabled = false
	}
	if _, ok := stored["growth_ai_run_policy"]; !ok {
		c.GrowthAIRunPolicy = GrowthAIRunPolicyManualOnly
	}
	switch c.GrowthRadarMode {
	case GrowthRadarOff, GrowthRadarObserve, GrowthRadarCreate:
	default:
		c.GrowthRadarMode = GrowthRadarObserve
	}
	if _, ok := stored["doctor_ai_run_policy"]; !ok {
		c.DoctorAIRunPolicy = DoctorAIRunPolicyManualOnly
	}
	switch c.GrowthAIRunPolicy {
	case GrowthAIRunPolicyScheduledOnly, GrowthAIRunPolicyScheduledAndEvent, GrowthAIRunPolicyOnDemandRecommended, GrowthAIRunPolicyManualOnly:
	default:
		c.GrowthAIEnabled = false
		c.GrowthAIRunPolicy = GrowthAIRunPolicyManualOnly
	}
	switch c.DoctorAIRunPolicy {
	case DoctorAIRunPolicyManualOnly, DoctorAIRunPolicyOnDemand, DoctorAIRunPolicyAutomatic:
	default:
		c.DoctorAIEnabled = false
		c.DoctorAIRunPolicy = DoctorAIRunPolicyManualOnly
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
//   - auto: legacy value treated as scheduled;
//   - scheduled: one every PublishIntervalDays after the last slot, never in the past.
func (c ProjectConfig) NextPublishSlot(latest, now time.Time) (time.Time, bool) {
	switch c.PublishMode {
	case PublishModeManual:
		return time.Time{}, false
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
